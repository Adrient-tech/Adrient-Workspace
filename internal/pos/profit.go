// Profit summary and margin: cost is frozen from Adrient's stock layers, money is integer paise.

package pos

import (
	"context"
	"math"
	"sort"
	"time"
)

// ProfitLine is one sold line, in paise. Cost is nil when it was never recorded.
type ProfitLine struct {
	Key       string
	Name      string
	Net       int64  // revenue with tax taken out, after discounts
	Cost      *int64 // cost of goods, nil when unknown
	QtyKept   float64
	KeptShare float64 // 0..1, what is left after refunds
}

// ProfitSummary is gross profit over the lines whose cost is known. Coverage says how much of the
// trade (by money) that is, so a shop that keeps no costs sees none instead of a made-up margin.
type ProfitSummary struct {
	Net       float64     `json:"net"`       // revenue net of tax over every line
	KnownNet  float64     `json:"known_net"` // revenue on lines with a cost
	Cost      float64     `json:"cost"`      // cost of those lines
	Profit    float64     `json:"profit"`    // known_net - cost
	Margin    *float64    `json:"margin"`    // percent of known_net, nil when nothing is costed
	Coverage  float64     `json:"coverage"`  // 0..1
	Known     int         `json:"known_lines"`
	Unknown   int         `json:"unknown_lines"`
	Any       bool        `json:"any"`
	TopProfit []ProfitRow `json:"top_by_profit"`
	From      string      `json:"from"`
	To        string      `json:"to"`
}

// ProfitRow ranks a product by what it earned.
type ProfitRow struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Net    float64 `json:"net"`
	Cost   float64 `json:"cost"`
	Profit float64 `json:"profit"`
}

func paiseF(v int64) float64 { return float64(v) / 100 }

// SummariseProfit adds up lines. Refunds are netted by KeptShare: a line refunded by a quarter
// keeps three quarters of its revenue and cost.
func SummariseProfit(lines []ProfitLine) ProfitSummary {
	var net, knownNet, cost, coveredAbs, totalAbs int64
	var s ProfitSummary
	per := map[string]*[3]int64{}
	names := map[string]string{}
	for _, l := range lines {
		share := math.Min(math.Max(l.KeptShare, 0), 1)
		n := int64(math.Round(float64(l.Net) * share))
		net += n
		totalAbs += abs64(n)
		if l.Cost == nil {
			s.Unknown++
			continue
		}
		c := int64(math.Round(float64(*l.Cost) * share))
		s.Known++
		knownNet += n
		cost += c
		coveredAbs += abs64(n)
		row := per[l.Key]
		if row == nil {
			row = &[3]int64{}
			per[l.Key] = row
			names[l.Key] = l.Name
		}
		row[0] += n
		row[1] += c
	}
	profit := knownNet - cost
	s.Net, s.KnownNet, s.Cost, s.Profit = paiseF(net), paiseF(knownNet), paiseF(cost), paiseF(profit)
	if knownNet != 0 {
		m := math.Round(float64(profit)/float64(knownNet)*10000) / 100
		s.Margin = &m
	}
	if totalAbs > 0 {
		s.Coverage = float64(coveredAbs) / float64(totalAbs)
	}
	s.Any = s.Known > 0
	for k, row := range per {
		s.TopProfit = append(s.TopProfit, ProfitRow{Key: k, Name: names[k], Net: paiseF(row[0]), Cost: paiseF(row[1]), Profit: paiseF(row[0] - row[1])})
	}
	sort.Slice(s.TopProfit, func(i, j int) bool {
		if s.TopProfit[i].Profit != s.TopProfit[j].Profit {
			return s.TopProfit[i].Profit > s.TopProfit[j].Profit
		}
		return s.TopProfit[i].Name < s.TopProfit[j].Name
	})
	if len(s.TopProfit) > 10 {
		s.TopProfit = s.TopProfit[:10]
	}
	return s
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// MarginPercent is (price - cost) / price in percent to two places; ok is false without a price.
func MarginPercent(price, cost float64) (float64, bool) {
	p := int64(math.Round(price * 100))
	if p <= 0 {
		return 0, false
	}
	return math.Round(float64(p-int64(math.Round(cost*100)))/float64(p)*10000) / 100, true
}

// ProfitReport reads sale lines of completed sales in the period and summarises them.
func (r *Repository) ProfitReport(ctx context.Context, orgID, since, until string) (ProfitSummary, error) {
	from, ok := parseTS(since)
	if !ok {
		from = time.Now().AddDate(0, 0, -30)
	}
	to, ok := parseTS(until)
	if !ok {
		to = time.Now().Add(time.Hour)
	}
	rows, err := r.pool.Query(ctx, `SELECT coalesce(l.product_id::text, l.name), l.name, round((l.line_total - l.tax_amount)*100)::bigint,
		CASE WHEN l.cost_total IS NULL THEN NULL ELSE round(l.cost_total*100)::bigint END,
		CASE WHEN l.qty > 0 THEN greatest(l.qty - l.refunded_qty, 0)/l.qty ELSE 1 END::float8
		FROM adrient.pos_sale_lines l JOIN adrient.pos_sales s ON s.id=l.sale_id
		WHERE s.org_id=$1 AND s.status<>'held' AND s.completed_at >= $2 AND s.completed_at < $3`, orgID, from, to)
	if err != nil {
		return ProfitSummary{}, err
	}
	defer rows.Close()
	var lines []ProfitLine
	for rows.Next() {
		var l ProfitLine
		if err := rows.Scan(&l.Key, &l.Name, &l.Net, &l.Cost, &l.KeptShare); err != nil {
			return ProfitSummary{}, err
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		return ProfitSummary{}, err
	}
	s := SummariseProfit(lines)
	s.From, s.To = from.Format("2006-01-02"), to.Format("2006-01-02")
	return s, nil
}
