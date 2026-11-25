package accounting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
)

// PosSource is the slice of the POS repository Accounting reads.
type PosSource interface {
	ListSales(ctx context.Context, orgID string, req *adrientv1.ListPosSalesRequest) ([]*adrientv1.PosSale, error)
}

// InvoiceSource is the slice of the Invoicing repository Accounting reads.
type InvoiceSource interface {
	ListDocuments(ctx context.Context, orgID string, f invoicing.DocFilter) ([]invoicing.Document, error)
	ListPayments(ctx context.Context, orgID, from, to string) ([]invoicing.Payment, error)
}

// ---- pure posting builders (unit-tested) ----

// taxSplit divides GST between the CGST/SGST or IGST accounts.
func taxLines(keys map[string]string, tax int64, intrastate, credit bool, memo string) []Line {
	if tax <= 0 {
		return nil
	}
	mk := func(key string, amt int64) Line {
		l := Line{AccountID: keys[key], Memo: memo}
		if credit {
			l.Credit = rupees(amt)
		} else {
			l.Debit = rupees(amt)
		}
		return l
	}
	out := func(c, s, i string) []Line {
		if intrastate {
			half := tax / 2
			return []Line{mk(c, half), mk(s, tax-half)}
		}
		return []Line{mk(i, tax)}
	}
	if credit {
		return out("gst_out_cgst", "gst_out_sgst", "gst_out_igst")
	}
	return out("gst_in_cgst", "gst_in_sgst", "gst_in_igst")
}

func taxLinesOut(keys map[string]string, tax int64, intrastate, credit bool, memo string) []Line {
	// credit=true: output tax raised (sale); credit=false: output tax reversed (refund).
	if tax <= 0 {
		return nil
	}
	ls := taxLines(keys, tax, intrastate, true, memo)
	if !credit {
		for i := range ls {
			ls[i].Debit, ls[i].Credit = ls[i].Credit, 0
		}
	}
	return ls
}

func methodKey(m string) string {
	switch m {
	case "cash":
		return "cash"
	case "store_credit", "loyalty":
		return "store_credit"
	}
	return "bank"
}

func dateOf(rfc string) string {
	if t, err := time.Parse(time.RFC3339, rfc); err == nil {
		return t.In(time.FixedZone("IST", 5*3600+1800)).Format("2006-01-02")
	}
	if len(rfc) >= 10 {
		return rfc[:10]
	}
	return time.Now().Format("2006-01-02")
}

// BuildPosSale turns a completed POS sale into a journal: Dr cash/bank per
// payment, Cr Sales (net of GST), Cr GST output.
func BuildPosSale(keys map[string]string, s *adrientv1.PosSale, intrastate bool) JournalInput {
	total, tax := paise(s.GetTotal()), paise(s.GetTaxTotal())
	if tax > total {
		tax = 0
	}
	memo := fmt.Sprintf("POS sale #%d", s.GetNumber())
	in := JournalInput{Date: dateOf(orDefault(s.GetCompletedAt(), s.GetCreatedAt())), Memo: memo, Ref: fmt.Sprintf("POS-%d", s.GetNumber()),
		Kind: "sale", Source: "pos_sale", SourceID: s.GetId(), ContactID: s.GetCustomerId(), ContactName: s.GetCustomerName()}
	var paid int64
	debit := map[string]int64{}
	for _, p := range s.GetPayments() {
		a := paise(p.GetAmount())
		debit[methodKey(p.GetMethod())] += a
		paid += a
	}
	debit["cash"] += total - paid // rounding or an unrecorded tender settles to the till
	for _, k := range []string{"cash", "bank", "store_credit"} {
		v := debit[k]
		switch {
		case v > 0:
			in.Lines = append(in.Lines, Line{AccountID: keys[k], Debit: rupees(v), Memo: memo})
		case v < 0:
			in.Lines = append(in.Lines, Line{AccountID: keys[k], Credit: rupees(-v), Memo: memo})
		}
	}
	in.Lines = append(in.Lines, Line{AccountID: keys["sales"], Credit: rupees(total - tax), Memo: memo})
	in.Lines = append(in.Lines, taxLines(keys, tax, intrastate, true, memo)...)
	in.SourceHash = hashOf(total, tax, paid, len(s.GetPayments()), in.Date, s.GetCustomerId())
	return in
}

// BuildPosRefund reverses part of a sale in proportion to the refund.
func BuildPosRefund(keys map[string]string, s *adrientv1.PosSale, f *adrientv1.PosRefund, intrastate bool) JournalInput {
	rt := paise(f.GetTotal())
	var tax int64
	if st := paise(s.GetTotal()); st > 0 {
		tax = int64(float64(paise(s.GetTaxTotal()))*float64(rt)/float64(st) + 0.5)
	}
	if tax > rt {
		tax = 0
	}
	memo := fmt.Sprintf("POS refund %s", f.GetNumber())
	in := JournalInput{Date: dateOf(f.GetCreatedAt()), Memo: memo, Ref: f.GetNumber(), Kind: "sale", Source: "pos_refund", SourceID: f.GetId(),
		ContactID: s.GetCustomerId(), ContactName: s.GetCustomerName()}
	in.Lines = append(in.Lines, Line{AccountID: keys["sales_returns"], Debit: rupees(rt - tax), Memo: memo})
	in.Lines = append(in.Lines, taxLinesOut(keys, tax, intrastate, false, memo)...)
	in.Lines = append(in.Lines, Line{AccountID: keys[methodKey(f.GetMethod())], Credit: rupees(rt), Memo: memo})
	in.SourceHash = hashOf(rt, tax, f.GetMethod(), in.Date)
	return in
}

// BuildInvoice posts a sent invoice: Dr Receivable, Cr Sales, Cr GST output.
func BuildInvoice(keys map[string]string, d invoicing.Document, intrastate bool) JournalInput {
	total, tax := paise(d.Total), paise(d.TaxTotal)
	if tax > total {
		tax = 0
	}
	memo := "Invoice " + d.Number
	in := JournalInput{Date: d.IssueDate, DueDate: d.DueDate, Memo: memo, Ref: d.Number, Kind: "invoice", Source: "invoice", SourceID: d.ID,
		ContactID: d.ContactID, ContactName: d.ClientName}
	in.Lines = []Line{{AccountID: keys["ar"], Debit: rupees(total), Memo: memo}, {AccountID: keys["sales"], Credit: rupees(total - tax), Memo: memo}}
	in.Lines = append(in.Lines, taxLines(keys, tax, intrastate, true, memo)...)
	in.SourceHash = hashOf(total, tax, d.IssueDate, d.DueDate, d.ContactID, d.ClientName)
	return in
}

// BuildInvoicePayment posts a payment, refund or write-off against an invoice.
func BuildInvoicePayment(keys map[string]string, p invoicing.Payment, d invoicing.Document) JournalInput {
	a := paise(p.Amount)
	label := map[string]string{"payment": "Payment", "refund": "Refund", "writeoff": "Write-off"}[p.Kind]
	memo := label + " " + p.DocNumber
	in := JournalInput{Date: p.PaidAt, Memo: memo, Ref: orDefault(p.Reference, p.DocNumber), Kind: "payment", Source: "invoice_payment", SourceID: p.ID,
		ContactID: d.ContactID, ContactName: orDefault(p.ClientName, d.ClientName)}
	acct := keys["bank"]
	if p.Method == "cash" {
		acct = keys["cash"]
	}
	switch p.Kind {
	case "refund":
		in.Lines = []Line{{AccountID: keys["ar"], Debit: rupees(a), Memo: memo}, {AccountID: acct, Credit: rupees(a), Memo: memo}}
	case "writeoff":
		in.Lines = []Line{{AccountID: keys["bad_debt"], Debit: rupees(a), Memo: memo}, {AccountID: keys["ar"], Credit: rupees(a), Memo: memo}}
	default:
		in.Lines = []Line{{AccountID: acct, Debit: rupees(a), Memo: memo}, {AccountID: keys["ar"], Credit: rupees(a), Memo: memo}}
	}
	in.SourceHash = hashOf(a, p.Kind, p.Method, p.PaidAt)
	return in
}

// ---- sync orchestration ----

// SyncResult counts what a sync did, per source.
type SyncResult struct {
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"` // already posted and unchanged
	Voided   int      `json:"voided"`
	Locked   int      `json:"locked"`  // inside the closed period
	Foreign  int      `json:"foreign"` // not in the base currency
	Warnings []string `json:"warnings"`
}

func (s *SyncResult) note(created, locked bool, err error, what string) {
	switch {
	case err == nil && created:
		s.Created++
	case err == nil:
		s.Skipped++
	case errors.Is(err, ErrLocked):
		s.Locked++
	default:
		if len(s.Warnings) < 10 {
			s.Warnings = append(s.Warnings, what+": "+err.Error())
		}
	}
}

// post wraps PostJournal to distinguish created, updated and unchanged posts.
func (r *Repository) post(ctx context.Context, res *SyncResult, orgID, userID string, in JournalInput) {
	var before string
	_ = r.pool.QueryRow(ctx, `SELECT source_hash FROM adrient.acct_journals WHERE org_id=$1 AND source=$2 AND source_id=$3 AND NOT voided`,
		orgID, in.Source, in.SourceID).Scan(&before)
	_, created, err := r.PostJournal(ctx, orgID, userID, in)
	switch {
	case err == nil && !created && before != "" && before != in.SourceHash:
		res.Updated++
	default:
		res.note(created, false, err, in.Memo)
	}
}

// Sync reads POS and Invoicing and posts what is missing. It is idempotent: each
// source row owns one journal keyed by (source, source id), so running it again
// never double-posts. full re-reads all history instead of the recent window.
func (r *Repository) Sync(ctx context.Context, orgID, userID string, pos PosSource, inv InvoiceSource, full bool) (map[string]*SyncResult, error) {
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return nil, err
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return nil, err
	}
	out := map[string]*SyncResult{"pos": {}, "invoicing": {}}
	if pos != nil {
		if err := r.syncPos(ctx, out["pos"], orgID, userID, keys, st, pos, full); err != nil {
			return out, err
		}
	}
	if inv != nil {
		if err := r.syncInvoicing(ctx, out["invoicing"], orgID, userID, keys, st, inv); err != nil {
			return out, err
		}
	}
	r.audit(ctx, orgID, userID, "sync", "apps", "", fmt.Sprintf("pos +%d, invoicing +%d", out["pos"].Created, out["invoicing"].Created))
	return out, nil
}

func (r *Repository) syncPos(ctx context.Context, res *SyncResult, orgID, userID string, keys map[string]string, st Settings, pos PosSource, full bool) error {
	since := ""
	if !full && st.PosSyncedAt != "" {
		if t, err := time.Parse(time.RFC3339, st.PosSyncedAt); err == nil {
			since = t.AddDate(0, 0, -30).Format(time.RFC3339)
		}
	}
	until := ""
	started := time.Now().UTC()
	for page := 0; page < 200; page++ {
		sales, err := pos.ListSales(ctx, orgID, &adrientv1.ListPosSalesRequest{Since: since, Until: until, Limit: 500, Detail: true})
		if err != nil {
			return err
		}
		for _, s := range sales {
			switch s.GetStatus() {
			case "completed", "refunded", "partially_refunded":
			default:
				continue
			}
			r.post(ctx, res, orgID, userID, BuildPosSale(keys, s, st.Intrastate))
			for _, f := range s.GetRefunds() {
				r.post(ctx, res, orgID, userID, BuildPosRefund(keys, s, f, st.Intrastate))
			}
		}
		if len(sales) < 500 {
			break
		}
		last := sales[len(sales)-1]
		until = orDefault(last.GetCompletedAt(), last.GetCreatedAt())
	}
	_, err := r.pool.Exec(ctx, `UPDATE adrient.acct_settings SET pos_synced_at=$2 WHERE org_id=$1`, orgID, started)
	return err
}

func (r *Repository) syncInvoicing(ctx context.Context, res *SyncResult, orgID, userID string, keys map[string]string, st Settings, inv InvoiceSource) error {
	docs, err := inv.ListDocuments(ctx, orgID, invoicing.DocFilter{Kind: "invoice"})
	if err != nil {
		return err
	}
	byID := map[string]invoicing.Document{}
	for _, d := range docs {
		byID[d.ID] = d
		if d.Status == "draft" || d.Deleted {
			if ok, err := r.voidBySource(ctx, orgID, userID, "invoice", d.ID); err == nil && ok {
				res.Voided++
			}
			continue
		}
		in := BuildInvoice(keys, d, st.Intrastate)
		if cur := strings.ToUpper(d.Currency); cur != "" && cur != strings.ToUpper(st.Currency) {
			rate, err := r.RateOn(ctx, orgID, cur, d.IssueDate)
			if err != nil {
				res.Foreign++
				if len(res.Warnings) < 10 {
					res.Warnings = append(res.Warnings, "Invoice "+d.Number+": "+err.Error())
				}
				continue
			}
			in.Currency, in.Rate = cur, rate
			in.Lines = toBase(in.Lines, rate)
			in.SourceHash = hashOf(in.SourceHash, cur, rate)
		}
		r.post(ctx, res, orgID, userID, in)
	}
	// Trashed invoices are not listed by default; fetch them to void their journals.
	if trashed, err := inv.ListDocuments(ctx, orgID, invoicing.DocFilter{Kind: "invoice", Trash: true}); err == nil {
		for _, d := range trashed {
			if ok, err := r.voidBySource(ctx, orgID, userID, "invoice", d.ID); err == nil && ok {
				res.Voided++
			}
		}
	}
	pays, err := inv.ListPayments(ctx, orgID, "", "")
	if err != nil {
		return err
	}
	for _, p := range pays {
		d, ok := byID[p.DocumentID]
		if !ok || d.Status == "draft" || d.Deleted {
			continue
		}
		in := BuildInvoicePayment(keys, p, d)
		if st.UseUndeposited && p.Kind == "payment" && p.Method != "cash" && keys["undeposited_funds"] != "" {
			in.Lines[0].AccountID = keys["undeposited_funds"]
			in.SourceHash = hashOf(in.SourceHash, "undeposited")
		}
		if cur := strings.ToUpper(firstNonEmpty(p.Currency, d.Currency)); cur != "" && cur != strings.ToUpper(st.Currency) {
			if err := r.foreignPayment(ctx, orgID, keys, &in, p, d, cur); err != nil {
				res.Foreign++
				if len(res.Warnings) < 10 {
					res.Warnings = append(res.Warnings, "Payment "+p.DocNumber+": "+err.Error())
				}
				continue
			}
		}
		r.post(ctx, res, orgID, userID, in)
	}
	// Payments deleted in Invoicing: void the journals whose payment id is gone.
	live := map[string]bool{}
	for _, p := range pays {
		live[p.ID] = true
	}
	rows, err := r.pool.Query(ctx, `SELECT source_id FROM adrient.acct_journals WHERE org_id=$1 AND source='invoice_payment' AND NOT voided`, orgID)
	if err != nil {
		return err
	}
	gone := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && !live[id] {
			gone = append(gone, id)
		}
	}
	rows.Close()
	for _, id := range gone {
		if ok, err := r.voidBySource(ctx, orgID, userID, "invoice_payment", id); err == nil && ok {
			res.Voided++
		}
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// foreignPayment rewrites a payment/refund/write-off journal for a foreign-currency
// invoice: the receivable is cleared at the rate the invoice was booked at, cash
// moves at the payment-date rate, and the difference is the realised gain or loss.
func (r *Repository) foreignPayment(ctx context.Context, orgID string, _ map[string]string, in *JournalInput, p invoicing.Payment, d invoicing.Document, cur string) error {
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return err
	}
	ks, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return err
	}
	docRate := 0.0
	if err := r.pool.QueryRow(ctx, `SELECT fx_rate::float8 FROM adrient.acct_journals WHERE org_id=$1 AND source='invoice' AND source_id=$2 AND NOT voided`,
		orgID, d.ID).Scan(&docRate); err != nil || docRate <= 0 {
		return fmt.Errorf("invoice %s is not posted in %s yet (no exchange rate on its issue date)", d.Number, cur)
	}
	payRate, err := r.RateOn(ctx, orgID, cur, p.PaidAt)
	if err != nil {
		return err
	}
	cash := in.Lines[0].AccountID // bank/cash/undeposited chosen by the builder for payments
	if p.Kind != "payment" {
		cash = ks["bank"]
		if p.Method == "cash" {
			cash = ks["cash"]
		}
	}
	switch p.Kind {
	case "writeoff":
		in.Lines = toBase([]Line{{AccountID: ks["bad_debt"], Debit: p.Amount, Memo: in.Memo}, {AccountID: ks["ar"], Credit: p.Amount, Memo: in.Memo}}, docRate)
	case "refund":
		ls := settlementLines(cash, ks["ar"], ks["fx_gain_loss"], p.Amount, docRate, payRate, true, in.Memo)
		for i := range ls {
			ls[i].Debit, ls[i].Credit = ls[i].Credit, ls[i].Debit
			ls[i].FcyDebit, ls[i].FcyCredit = ls[i].FcyCredit, ls[i].FcyDebit
		}
		in.Lines = ls
	default:
		in.Lines = settlementLines(cash, ks["ar"], ks["fx_gain_loss"], p.Amount, docRate, payRate, true, in.Memo)
	}
	in.Currency, in.Rate = cur, payRate
	in.SourceHash = hashOf(in.SourceHash, cur, docRate, payRate)
	return nil
}
