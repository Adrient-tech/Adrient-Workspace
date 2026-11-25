// Package pos implements the point of sale: registers, shifts, sales,
// payments, refunds, taxes and customer balances. Products and stock belong to
// the inventory module.
package pos

import "math"

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// CalcLine is the input of one cart line for pricing.
type CalcLine struct {
	Qty           float64
	UnitPrice     float64
	DiscountKind  string // percent | amount | ""
	DiscountValue float64
	TaxRate       float64 // percent
	TaxInclusive  bool
}

// CalcLineResult is the priced line.
type CalcLineResult struct {
	Gross          float64 // qty * price
	DiscountAmount float64 // line discount + allocated order discount
	TaxAmount      float64
	LineTotal      float64 // amount payable for the line
}

// CalcTotals is the priced cart.
type CalcTotals struct {
	Lines         []CalcLineResult
	Subtotal      float64
	DiscountTotal float64
	TaxTotal      float64
	Total         float64
}

func discountOf(kind string, value, base float64) float64 {
	if base <= 0 || value <= 0 {
		return 0
	}
	var d float64
	switch kind {
	case "percent":
		if value > 100 {
			value = 100
		}
		d = round2(base * value / 100)
	case "amount":
		d = round2(value)
	default:
		return 0
	}
	if d > base {
		d = base
	}
	return d
}

// ComputeTotals prices a cart: line discounts first, then the order discount
// is spread over the lines in proportion to their value (the last line takes
// the rounding remainder), then tax per line (exclusive tax is added,
// inclusive tax is extracted from the price).
func ComputeTotals(lines []CalcLine, orderKind string, orderValue float64) CalcTotals {
	out := CalcTotals{Lines: make([]CalcLineResult, len(lines))}
	after := make([]float64, len(lines))
	var afterSum float64
	for i, l := range lines {
		g := round2(l.Qty * l.UnitPrice)
		ld := discountOf(l.DiscountKind, l.DiscountValue, g)
		out.Lines[i].Gross = g
		out.Lines[i].DiscountAmount = ld
		after[i] = round2(g - ld)
		afterSum += after[i]
		out.Subtotal += g
	}
	od := discountOf(orderKind, orderValue, round2(afterSum))
	remaining := od
	for i := range lines {
		share := 0.0
		if afterSum > 0 {
			share = round2(od * after[i] / afterSum)
		}
		if i == lastNonZero(after) {
			share = remaining
		}
		if share > after[i] {
			share = after[i]
		}
		remaining = round2(remaining - share)
		out.Lines[i].DiscountAmount = round2(out.Lines[i].DiscountAmount + share)
		base := round2(after[i] - share)
		var tax, total float64
		if lines[i].TaxRate > 0 {
			if lines[i].TaxInclusive {
				tax = round2(base - base/(1+lines[i].TaxRate/100))
				total = base
			} else {
				tax = round2(base * lines[i].TaxRate / 100)
				total = round2(base + tax)
			}
		} else {
			total = base
		}
		out.Lines[i].TaxAmount = tax
		out.Lines[i].LineTotal = total
		out.DiscountTotal += out.Lines[i].DiscountAmount
		out.TaxTotal += tax
		out.Total += total
	}
	out.Subtotal = round2(out.Subtotal)
	out.DiscountTotal = round2(out.DiscountTotal)
	out.TaxTotal = round2(out.TaxTotal)
	out.Total = round2(out.Total)
	return out
}

func lastNonZero(a []float64) int {
	idx := -1
	for i, v := range a {
		if v > 0 {
			idx = i
		}
	}
	return idx
}

// Tender is one payment offered at checkout.
type Tender struct {
	Method string
	Amount float64 // handed over
}

// Applied is a tender after allocation.
type Applied struct {
	Method   string
	Amount   float64 // applied to the sale
	Tendered float64
}

// AllocateTenders spreads the tenders over total in the given order. Only cash
// may exceed what is still due (the excess is change); every other method is
// capped to the remainder and is rejected when it would overpay. Returns the
// applied payments, the change due and ok=false if the total is not covered.
func AllocateTenders(total float64, tenders []Tender) (applied []Applied, change float64, ok bool) {
	due := round2(total)
	// Cash goes last so that exact amounts on other methods are honoured first.
	var ordered []Tender
	for _, t := range tenders {
		if t.Method != "cash" {
			ordered = append(ordered, t)
		}
	}
	for _, t := range tenders {
		if t.Method == "cash" {
			ordered = append(ordered, t)
		}
	}
	for _, t := range ordered {
		amt := round2(t.Amount)
		if amt <= 0 {
			continue
		}
		if t.Method == "cash" {
			use := amt
			if use > due {
				use = due
			}
			applied = append(applied, Applied{Method: t.Method, Amount: use, Tendered: amt})
			change = round2(change + amt - use)
			due = round2(due - use)
			continue
		}
		if amt > due+0.004 {
			return nil, 0, false
		}
		use := amt
		if use > due {
			use = due
		}
		applied = append(applied, Applied{Method: t.Method, Amount: use, Tendered: use})
		due = round2(due - use)
	}
	return applied, change, due <= 0.004
}

// RefundAmount is the share of a line's total owed for returning qty units.
// Returning the last remaining units pays out the remainder so the refunds
// always add up to the line total.
func RefundAmount(lineTotal, lineQty, alreadyQty, alreadyAmount, qty float64) float64 {
	if lineQty <= 0 || qty <= 0 {
		return 0
	}
	if qty >= round3(lineQty-alreadyQty) {
		return round2(lineTotal - alreadyAmount)
	}
	return round2(lineTotal * qty / lineQty)
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
