package inventory

// Pure business rules for valuation, lot picking, purchasing and planning.
// Nothing here touches the database so every rule can be tested exhaustively.

import (
	"math"
	"sort"
	"strconv"
	"time"
)

func round2(f float64) float64 { return math.Round(f*100) / 100 }
func round4(f float64) float64 { return math.Round(f*10000) / 10000 }

// ------------------------------------------------------------ cost layers

// Layer is the open part of one receipt: remaining quantity at its unit cost.
type Layer struct {
	ID   string
	Qty  float64
	Cost float64
}

// Consumption is quantity taken from one layer. Cost is the unit cost the
// movement is valued at (the layer's own cost for FIFO, the pool average for
// weighted average). LayerID is empty when stock ran out and the shortfall was
// valued at the fallback cost.
type Consumption struct {
	LayerID string
	Qty     float64
	Cost    float64
}

// ConsumeLayers takes qty out of layers (oldest first, as supplied) using the
// valuation method "fifo" or "avg". Layers whose ID is in prefer are taken first
// at their own cost (used for returns to the original receipt). Quantity beyond
// the open layers is valued at fallback and has no LayerID.
func ConsumeLayers(method string, layers []Layer, qty float64, prefer map[string]bool, fallback float64) []Consumption {
	var out []Consumption
	left := round3(qty)
	var rest []Layer
	for _, l := range layers {
		if l.Qty <= 0 {
			continue
		}
		if prefer[l.ID] && left > 0 {
			take := math.Min(left, l.Qty)
			out = append(out, Consumption{LayerID: l.ID, Qty: round3(take), Cost: l.Cost})
			left = round3(left - take)
			if rem := round3(l.Qty - take); rem > 0 {
				rest = append(rest, Layer{ID: l.ID, Qty: rem, Cost: l.Cost})
			}
			continue
		}
		rest = append(rest, l)
	}
	if left > 0 && len(rest) > 0 {
		if method == "fifo" {
			for _, l := range rest {
				if left <= 0 {
					break
				}
				take := math.Min(left, l.Qty)
				out = append(out, Consumption{LayerID: l.ID, Qty: round3(take), Cost: l.Cost})
				left = round3(left - take)
			}
		} else {
			var tq, tv float64
			for _, l := range rest {
				tq += l.Qty
				tv += l.Qty * l.Cost
			}
			avg := round4(tv / tq)
			take := math.Min(left, tq)
			given := 0.0
			for i, l := range rest {
				share := round3(take * l.Qty / tq)
				if i == len(rest)-1 {
					share = round3(take - given)
				}
				if share > l.Qty {
					share = l.Qty
				}
				if share <= 0 {
					continue
				}
				given = round3(given + share)
				out = append(out, Consumption{LayerID: l.ID, Qty: share, Cost: avg})
			}
			left = round3(left - take)
		}
	}
	if left > 0 {
		out = append(out, Consumption{Qty: left, Cost: fallback})
	}
	return out
}

// ConsumedValue is the money value of a consumption list.
func ConsumedValue(cs []Consumption) float64 {
	var v float64
	for _, c := range cs {
		v += c.Qty * c.Cost
	}
	return round2(v)
}

// AverageCost is the weighted average unit cost of open layers (fallback when none).
func AverageCost(layers []Layer, fallback float64) float64 {
	var tq, tv float64
	for _, l := range layers {
		if l.Qty > 0 {
			tq += l.Qty
			tv += l.Qty * l.Cost
		}
	}
	if tq <= 0 {
		return fallback
	}
	return round4(tv / tq)
}

// AllocateLanded spreads a landed cost over receipt lines. method "value"
// weights by line value (qty*unit cost), "qty" by quantity. It returns the extra
// cost per unit for each line; the sum of extra*qty equals total (to the paisa).
func AllocateLanded(total float64, method string, qty, unit []float64) []float64 {
	out := make([]float64, len(qty))
	if total == 0 || len(qty) == 0 {
		return out
	}
	weights := make([]float64, len(qty))
	var sum float64
	for i := range qty {
		w := qty[i]
		if method == "value" {
			w = qty[i] * unit[i]
		}
		weights[i] = w
		sum += w
	}
	if sum <= 0 { // all-zero prices: fall back to quantity
		sum = 0
		for i := range qty {
			weights[i] = qty[i]
			sum += qty[i]
		}
	}
	if sum <= 0 {
		return out
	}
	for i := range qty {
		if qty[i] > 0 {
			out[i] = round4(total * weights[i] / sum / qty[i])
		}
	}
	return out
}

// ------------------------------------------------------------------ lots

// LotStock is on-hand quantity of one lot at a location.
type LotStock struct {
	LotID  string
	LotNo  string
	Expiry *time.Time
	Qty    float64
}

// LotPick is quantity taken from (or put into) one lot.
type LotPick struct {
	LotID  string  `json:"lot_id,omitempty"`
	LotNo  string  `json:"lot_no"`
	Expiry string  `json:"expiry,omitempty"`
	Qty    float64 `json:"qty"`
}

// PickFEFO chooses lots first-expiry-first-out. Lots already expired on asOf are
// skipped unless allowExpired. Lots with no expiry are used last. It returns the
// picks and any shortfall.
func PickFEFO(stock []LotStock, qty float64, asOf time.Time, allowExpired bool) ([]LotPick, float64) {
	cand := make([]LotStock, 0, len(stock))
	today := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, time.UTC)
	for _, s := range stock {
		if s.Qty <= 0 {
			continue
		}
		if s.Expiry != nil && s.Expiry.Before(today) && !allowExpired {
			continue
		}
		cand = append(cand, s)
	}
	sort.SliceStable(cand, func(i, j int) bool {
		a, b := cand[i].Expiry, cand[j].Expiry
		switch {
		case a == nil && b == nil:
			return cand[i].LotNo < cand[j].LotNo
		case a == nil:
			return false
		case b == nil:
			return true
		case a.Equal(*b):
			return cand[i].LotNo < cand[j].LotNo
		}
		return a.Before(*b)
	})
	left := round3(qty)
	var picks []LotPick
	for _, c := range cand {
		if left <= 0 {
			break
		}
		take := math.Min(left, c.Qty)
		p := LotPick{LotID: c.LotID, LotNo: c.LotNo, Qty: round3(take)}
		if c.Expiry != nil {
			p.Expiry = c.Expiry.Format("2006-01-02")
		}
		picks = append(picks, p)
		left = round3(left - take)
	}
	return picks, left
}

// ---------------------------------------------------------- purchase orders

// PO states.
const (
	POdraft        = "draft"
	POapproved     = "approved"
	POsent         = "sent"
	POacknowledged = "acknowledged"
	POpartial      = "partial"
	POreceived     = "received"
	POclosed       = "closed"
	POcancelled    = "cancelled"
)

var poTransitions = map[string][]string{
	POdraft:        {POapproved, POcancelled},
	POapproved:     {POsent, POacknowledged, POpartial, POreceived, POclosed, POcancelled, POdraft},
	POsent:         {POacknowledged, POpartial, POreceived, POclosed, POcancelled},
	POacknowledged: {POpartial, POreceived, POclosed, POcancelled},
	POpartial:      {POpartial, POreceived, POclosed},
	POreceived:     {POclosed, POpartial},
	POclosed:       {},
	POcancelled:    {},
}

// POCanMove reports whether a purchase order may change from one state to another.
func POCanMove(from, to string) bool {
	for _, s := range poTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// POCanReceive reports whether goods may be received against a PO in this state.
func POCanReceive(state string) bool {
	return state == POapproved || state == POsent || state == POacknowledged || state == POpartial
}

// POCanEdit reports whether lines can still be edited.
func POCanEdit(state string) bool { return state == POdraft }

// ReceiptLimit returns the most that may be received on a line given what was
// ordered, what was already received and the over-receipt tolerance percentage.
func ReceiptLimit(ordered, received, overPct float64) float64 {
	max := ordered * (1 + overPct/100)
	return math.Max(0, round3(max-received))
}

// POStateAfterReceipt derives the state after a receipt. lines are (ordered,
// received) pairs. With underClose true, a receipt that leaves lines short still
// closes the order.
func POStateAfterReceipt(lines [][2]float64, closeShort bool) string {
	allDone, any := true, false
	for _, l := range lines {
		if l[1] > 0 {
			any = true
		}
		if round3(l[1]) < round3(l[0]) {
			allDone = false
		}
	}
	switch {
	case allDone:
		return POreceived
	case any && closeShort:
		return POclosed
	case any:
		return POpartial
	}
	return POsent
}

// ----------------------------------------------------------- three way match

// MatchLine compares one PO line with what was received and what is billed.
type MatchLine struct {
	LineID       string
	Name         string
	OrderedQty   float64
	ReceivedQty  float64 // net of returns
	BilledBefore float64 // quantity on earlier posted bills
	BillQty      float64
	POCost       float64
	BillCost     float64
}

// MatchIssue describes one mismatch.
type MatchIssue struct {
	LineID string `json:"line_id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"` // qty_over_received, price_variance, no_po_line
	Detail string `json:"detail"`
}

// ThreeWayMatch checks bill lines against PO and goods receipts. Billing more
// than was received (beyond qtyTolPct) or at a price above the PO beyond
// priceTolPct is an issue. Lower prices are not blocked.
func ThreeWayMatch(lines []MatchLine, qtyTolPct, priceTolPct float64) []MatchIssue {
	var issues []MatchIssue
	for _, l := range lines {
		if l.LineID == "" {
			issues = append(issues, MatchIssue{Name: l.Name, Kind: "no_po_line", Detail: "not on the purchase order"})
			continue
		}
		allowed := l.ReceivedQty * (1 + qtyTolPct/100)
		if round3(l.BilledBefore+l.BillQty) > round3(allowed) {
			issues = append(issues, MatchIssue{LineID: l.LineID, Name: l.Name, Kind: "qty_over_received",
				Detail: "billing " + trimFloat(l.BilledBefore+l.BillQty) + " but only " + trimFloat(l.ReceivedQty) + " received"})
		}
		if l.POCost > 0 && l.BillCost > l.POCost*(1+priceTolPct/100)+0.00001 {
			issues = append(issues, MatchIssue{LineID: l.LineID, Name: l.Name, Kind: "price_variance",
				Detail: "billed at " + trimFloat(l.BillCost) + " against PO price " + trimFloat(l.POCost)})
		}
	}
	return issues
}

func trimFloat(f float64) string { return strconv.FormatFloat(math.Round(f*1000)/1000, 'f', -1, 64) }

// ----------------------------------------------------------- planning maths

// ABCClass assigns A/B/C by cumulative share of consumption value: A covers the
// first 80%, B the next 15%, C the rest. Items with zero value are C.
func ABCClass(values []float64) []string {
	idx := make([]int, len(values))
	var total float64
	for i, v := range values {
		idx[i] = i
		total += v
	}
	sort.SliceStable(idx, func(a, b int) bool { return values[idx[a]] > values[idx[b]] })
	out := make([]string, len(values))
	var cum float64
	for _, i := range idx {
		if total <= 0 || values[i] <= 0 {
			out[i] = "C"
			continue
		}
		before := cum / total
		cum += values[i]
		switch {
		case before < 0.80:
			out[i] = "A"
		case before < 0.95:
			out[i] = "B"
		default:
			out[i] = "C"
		}
	}
	return out
}

// MovingAverage is the mean of the last n values (all of them when fewer).
func MovingAverage(vals []float64, n int) float64 {
	if len(vals) == 0 || n <= 0 {
		return 0
	}
	if len(vals) > n {
		vals = vals[len(vals)-n:]
	}
	var s float64
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

// DaysOfCover is on-hand divided by average daily demand; -1 means no demand.
func DaysOfCover(onHand, dailyDemand float64) float64 {
	if dailyDemand <= 0 {
		return -1
	}
	if onHand <= 0 {
		return 0
	}
	return round2(onHand / dailyDemand)
}

// RoundUpToMultiple rounds q up to a multiple of m (m<=1 leaves q as is).
func RoundUpToMultiple(q, m float64) float64 {
	if m <= 1 {
		return q
	}
	return math.Ceil(q/m-1e-9) * m
}

// ReorderQty is how much to order for a min/max rule given the forecast
// position (on hand + incoming - reserved). Zero when above min.
func ReorderQty(position, min, max, multiple float64) float64 {
	if position > min {
		return 0
	}
	target := max
	if target <= min {
		target = min * 2
	}
	q := target - position
	if q <= 0 {
		return 0
	}
	return round3(RoundUpToMultiple(q, multiple))
}

// BestPrice picks the unit cost from price tiers: the lowest cost among tiers
// whose minimum quantity is met (and valid on the date).
type PriceTier struct {
	MinQty    float64
	Cost      float64
	ValidFrom *time.Time
	ValidTo   *time.Time
	LeadDays  int
}

// PickTier returns the best applicable tier.
func PickTier(tiers []PriceTier, qty float64, on time.Time) (PriceTier, bool) {
	var best PriceTier
	found := false
	for _, t := range tiers {
		if qty < t.MinQty {
			continue
		}
		if t.ValidFrom != nil && on.Before(*t.ValidFrom) {
			continue
		}
		if t.ValidTo != nil && on.After(*t.ValidTo) {
			continue
		}
		if !found || t.Cost < best.Cost {
			best, found = t, true
		}
	}
	return best, found
}
