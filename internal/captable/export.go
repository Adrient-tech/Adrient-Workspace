package captable

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// safeCell stops a spreadsheet from running a cell as a formula (CSV injection).
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// Money formats minor units as a decimal with 2 places ("1234567" -> "12345.67").
func Money(c int64) string {
	neg := ""
	if c < 0 {
		neg, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", neg, c/100, c%100)
}

// ExportCSV builds a CSV for captable, ledger, vesting or transactions. The result is an audited read.
func (r *Repository) ExportCSV(ctx context.Context, a knowkit.Actor, what, asOf string) ([]byte, string, error) {
	if asOf == "" {
		asOf = r.today()
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	row := func(cols ...string) {
		for i := range cols {
			cols[i] = safeCell(cols[i])
		}
		_ = w.Write(cols)
	}
	switch what {
	case "captable":
		tb, err := r.TableOf(ctx, a.Org, asOf)
		if err != nil {
			return nil, "", err
		}
		row("Holder", "Type", "Shares", "Options", "Warrants", "Fully diluted", "% outstanding", "% fully diluted")
		for _, x := range tb.Rows {
			row(x.Name, x.Kind, i(x.Stock), i(x.Options), i(x.Warrants), i(x.FullyDiluted), Pct(x.PctOut), Pct(x.PctFD))
		}
		row("Option pool (unallocated)", "pool", "", "", "", i(tb.Pool), "", Pct(tb.PoolPctFD))
		row("Total", "", i(tb.Outstanding), "", "", i(tb.FullyDil), "100.0000", "100.0000")
	case "ledger":
		recs, err := r.Securities(ctx, a.Org, "", "", asOf)
		if err != nil {
			return nil, "", err
		}
		row("Label", "Holder", "Kind", "Class", "Issued", "Quantity", "Outstanding", "Vested", "Price per share", "Principal", "Cap", "Discount %", "Expires")
		for _, s := range recs {
			row(s.Label, s.Stakeholder, s.Kind, s.Class, s.IssueDate, i(s.Quantity), i(s.Outstanding), i(s.Vested), s.Price, Money(s.Principal), Money(s.Cap), fmt.Sprintf("%.2f", float64(s.DiscountBp)/100), s.Expires)
		}
	case "vesting":
		recs, err := r.Securities(ctx, a.Org, "", "", asOf)
		if err != nil {
			return nil, "", err
		}
		row("Label", "Holder", "Kind", "Date", "Vested to date", "Newly vested")
		for _, s := range recs {
			if s.VestMonths == 0 || (s.Kind != "option" && s.Kind != "stock") {
				continue
			}
			var prev int64
			for _, p := range Schedule(s.toSecurity().Vesting, s.Quantity) {
				row(s.Label, s.Stakeholder, s.Kind, p.Date, i(p.Vested), i(p.Vested-prev))
				prev = p.Vested
			}
		}
	case "transactions":
		evs, err := r.Events(ctx, a.Org)
		if err != nil {
			return nil, "", err
		}
		recs, _ := r.Securities(ctx, a.Org, "", "", asOf)
		row("Date", "Event", "Security", "Quantity", "Price per share", "To", "Note")
		for _, e := range evs {
			row(e.Date, e.Kind, e.Label, i(e.Quantity), e.Price, e.ToName, e.Note)
		}
		for _, s := range recs {
			row(s.IssueDate, "issue", s.Label, i(s.Quantity), s.Price, s.Stakeholder, "")
		}
	default:
		return nil, "", knowkit.Invalid("export one of captable, ledger, vesting, transactions")
	}
	w.Flush()
	r.audit(ctx, "export_"+what, "captable_export", asOf)
	return buf.Bytes(), "captable-" + what + "-" + asOf + ".csv", w.Error()
}

func i(v int64) string { return strconv.FormatInt(v, 10) }

var _ = big.NewRat
