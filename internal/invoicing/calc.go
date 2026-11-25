// Package invoicing implements estimates, invoices, payments, recurring
// invoices, a price list, org branding and a public share page.
package invoicing

import (
	"math"
	"time"
)

// Line is one invoice row. Price is per unit, DiscountPct is a per-line percent.
type Line struct {
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	Unit        string  `json:"unit"`
	Price       float64 `json:"price"`
	DiscountPct float64 `json:"discount_pct"`
	TaxRate     float64 `json:"tax_rate"`
	// TaxInclusive means Price already contains the tax (how the EU, UK and Australia quote).
	// Stored inside the line JSON, so no migration; absent means tax is added on top.
	TaxInclusive bool   `json:"tax_inclusive,omitempty"`
	TaxName      string `json:"tax_name"`
	ProductID    string `json:"product_id,omitempty"`
	HSN          string `json:"hsn,omitempty"`     // HSN/SAC code for GST returns
	HSNSAC       string `json:"hsn_sac,omitempty"` // HSN (goods) or SAC (services) code
	Ref          string `json:"ref,omitempty"`     // stable line key on sales orders
	SOLine       string `json:"so_line,omitempty"` // sales order line this row bills
}

// Totals are the computed amounts of a document, rounded to 2 places.
type Totals struct {
	Subtotal      float64 `json:"subtotal"`
	DiscountTotal float64 `json:"discount_total"`
	TaxTotal      float64 `json:"tax_total"`
	Total         float64 `json:"total"`
}

// Round2 rounds half away from zero to cents.
func Round2(v float64) float64 { return math.Round(v*100) / 100 }

// Compute totals. Line discounts apply first, then the document discount
// (percent or amount) is spread proportionally over lines so tax is charged on
// the discounted base. Taxes are exclusive.
func Compute(lines []Line, discountKind string, discountValue float64) Totals {
	var sub float64
	nets := make([]float64, len(lines))
	inclusive := false
	for i, l := range lines {
		n := lineNet(l)
		nets[i] = n
		sub += n
		inclusive = inclusive || (l.TaxInclusive && l.TaxRate > 0)
	}
	var disc float64
	if discountKind == "amount" {
		disc = math.Min(math.Max(discountValue, 0), sub)
	} else {
		disc = sub * clampPct(discountValue) / 100
	}
	var tax, gross float64
	if sub > 0 {
		for i, l := range lines {
			share := nets[i] - disc*nets[i]/sub
			tax += share * math.Max(l.TaxRate, 0) / 100
			gross += share * (1 + math.Max(l.TaxRate, 0)/100)
		}
	}
	t := Totals{Subtotal: Round2(sub), DiscountTotal: Round2(disc), TaxTotal: Round2(tax)}
	t.Total = Round2(t.Subtotal - t.DiscountTotal + t.TaxTotal)
	if inclusive {
		// One rounding of the whole: the total is the quoted gross, and the parts add up to it.
		t.Total = Round2(gross)
		t.TaxTotal = Round2(t.Total - t.Subtotal + t.DiscountTotal)
	}
	return t
}

// lineNet is a line's value without tax after its own discount. For a tax-inclusive line the
// quoted price contains the tax, so it is taken out: net = gross / (1 + rate).
func lineNet(l Line) float64 {
	g := l.Qty * l.Price * (1 - clampPct(l.DiscountPct)/100)
	if l.TaxInclusive && l.TaxRate > 0 {
		return g / (1 + l.TaxRate/100)
	}
	return g
}

func clampPct(p float64) float64 { return math.Min(math.Max(p, 0), 100) }

// Standing is the payment state of an invoice.
type Standing struct {
	Paid       float64 `json:"paid"`        // payments minus refunds
	WrittenOff float64 `json:"written_off"` // forgiven balance
	Credited   float64 `json:"credited"`    // credit notes applied
	Balance    float64 `json:"balance"`     // total - paid - credited - written_off
}

// Stand folds payment rows into a Standing.
func Stand(total float64, pays []Payment) Standing {
	var s Standing
	for _, p := range pays {
		switch p.Kind {
		case "refund":
			s.Paid -= p.Amount
		case "writeoff":
			s.WrittenOff += p.Amount
		case "credit":
			s.Credited += p.Amount
		default:
			s.Paid += p.Amount
		}
	}
	s.Paid, s.WrittenOff, s.Credited = Round2(s.Paid), Round2(s.WrittenOff), Round2(s.Credited)
	s.Balance = Round2(total - s.Paid - s.WrittenOff - s.Credited)
	return s
}

// DisplayStatus derives the status shown to users from the stored status, the
// payment standing and today's date.
func DisplayStatus(kind, stored string, due *time.Time, total float64, s Standing, today time.Time) string {
	if kind == "estimate" || kind == "credit_note" {
		return stored
	}
	if stored == "draft" {
		return "draft"
	}
	if s.Balance <= 0.004 && total > 0 {
		if s.WrittenOff > 0 && s.Paid <= 0.004 && s.Credited <= 0.004 {
			return "written_off"
		}
		if s.Credited > 0 && s.Paid <= 0.004 {
			return "credited"
		}
		return "paid"
	}
	if s.Paid > 0.004 {
		if due != nil && due.Before(today) {
			return "overdue"
		}
		return "partial"
	}
	if due != nil && due.Before(today) {
		return "overdue"
	}
	return stored // sent
}

// NextDate advances d by a recurrence frequency.
func NextDate(d time.Time, freq string) time.Time {
	switch freq {
	case "weekly":
		return d.AddDate(0, 0, 7)
	case "quarterly":
		return addMonths(d, 3)
	case "yearly":
		return addMonths(d, 12)
	}
	return addMonths(d, 1)
}

// addMonths clamps the day so Jan 31 + 1 month is Feb 28/29, not March.
func addMonths(d time.Time, n int) time.Time {
	y, m, day := d.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, d.Location())
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, d.Location())
}

// ToBase converts amount in currency to the base currency using rates
// (1 unit of currency = rates[currency] base units). Unknown currency => 1.
func ToBase(amount float64, currency, base string, rates map[string]float64) float64 {
	if currency == "" || currency == base {
		return amount
	}
	if r, ok := rates[currency]; ok && r > 0 {
		return amount * r
	}
	return amount
}
