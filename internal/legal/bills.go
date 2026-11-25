package legal

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/jackc/pgx/v5"
)

// Bill is a statement for a matter, built from unbilled entries. Issuing it creates a draft invoice in Invoicing.
type Bill struct {
	ID            string `json:"id"`
	MatterID      string `json:"matter_id"`
	MatterNo      string `json:"matter_number,omitempty"`
	MatterTitle   string `json:"matter_title,omitempty"`
	ClientName    string `json:"client_name,omitempty"`
	Number        string `json:"number"`
	Status        string `json:"status"`
	IssueDate     string `json:"issue_date"`
	DueDate       string `json:"due_date"`
	PeriodFrom    string `json:"period_from"`
	PeriodTo      string `json:"period_to"`
	Fees          int64  `json:"fees"`
	Expenses      int64  `json:"expenses"`
	Discount      int64  `json:"discount"`
	Tax           int64  `json:"tax"`
	Total         int64  `json:"total"`
	InvoiceID     string `json:"invoice_id,omitempty"`
	InvoiceNumber string `json:"invoice_number,omitempty"`
	Lines         []Line `json:"lines,omitempty"`
	Currency      string `json:"currency,omitempty"`
	Note          string `json:"note,omitempty"`
}

// BillInput drafts a bill.
type BillInput struct {
	From       string `json:"from"`
	To         string `json:"to"`
	DiscountBp int    `json:"discount_bp"`
	Preview    bool   `json:"preview"` // compute only, change nothing
}

const billCols = `b.id::text, b.matter_id::text, m.number, m.title, COALESCE(c.name,''), b.number, b.status, to_char(b.issue_date,'YYYY-MM-DD'), to_char(b.due_date,'YYYY-MM-DD'),
	COALESCE(to_char(b.period_from,'YYYY-MM-DD'),''), COALESCE(to_char(b.period_to,'YYYY-MM-DD'),''), b.fees, b.expenses, b.discount, b.tax, b.total, COALESCE(b.invoice_id::text,''), b.invoice_number`
const billFrom = ` FROM adrient.legal_bills b JOIN adrient.legal_matters m ON m.id=b.matter_id LEFT JOIN adrient.legal_clients c ON c.id=m.client_id `

func scanBill(row pgx.Row) (Bill, error) {
	var b Bill
	err := row.Scan(&b.ID, &b.MatterID, &b.MatterNo, &b.MatterTitle, &b.ClientName, &b.Number, &b.Status, &b.IssueDate, &b.DueDate, &b.PeriodFrom, &b.PeriodTo,
		&b.Fees, &b.Expenses, &b.Discount, &b.Tax, &b.Total, &b.InvoiceID, &b.InvoiceNumber)
	return b, notFound(err)
}

func lineOf(e Entry) Line {
	l := Line{EntryID: e.ID, Kind: e.Kind, Date: e.Date, Description: e.Description, Amount: e.Amount}
	switch e.Kind {
	case "time":
		l.Minutes, l.Rate = e.Minutes, e.Rate
	case "expense":
		l.Qty, l.Rate = e.Qty, e.Rate
	}
	return l
}

// Bills lists bills the actor may see, newest first.
func (r *Repository) Bills(ctx context.Context, a knowkit.Actor, matterID string) ([]Bill, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+billCols+`, m.access, COALESCE(m.shared_with::text[],'{}'), COALESCE(m.created_by::text,''), COALESCE(m.responsible_id::text,''), COALESCE(m.originating_id::text,'')`+billFrom+`
		WHERE b.org_id=$1 AND m.deleted_at IS NULL AND ($2='' OR b.matter_id::text=$2) ORDER BY b.created_at DESC LIMIT 1000`, a.Org, matterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bill{}
	for rows.Next() {
		var b Bill
		var m Matter
		if err := rows.Scan(&b.ID, &b.MatterID, &b.MatterNo, &b.MatterTitle, &b.ClientName, &b.Number, &b.Status, &b.IssueDate, &b.DueDate, &b.PeriodFrom, &b.PeriodTo,
			&b.Fees, &b.Expenses, &b.Discount, &b.Tax, &b.Total, &b.InvoiceID, &b.InvoiceNumber, &m.Access, &m.SharedWith, &m.CreatedBy, &m.ResponsibleID, &m.OriginatingID); err != nil {
			return nil, err
		}
		if r.canView(a, m) {
			out = append(out, b)
		}
	}
	return out, rows.Err()
}

// Bill returns one bill with its lines.
func (r *Repository) Bill(ctx context.Context, a knowkit.Actor, id string) (Bill, error) {
	if !knowkit.IsUUID(id) {
		return Bill{}, knowkit.ErrNotFound
	}
	b, err := scanBill(r.pool.QueryRow(ctx, `SELECT `+billCols+billFrom+`WHERE b.org_id=$1 AND b.id=$2`, a.Org, id))
	if err != nil {
		return b, err
	}
	if _, err := r.loadMatter(ctx, a, b.MatterID); err != nil {
		return Bill{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+entryCols+entryFrom+`WHERE e.bill_id=$1 ORDER BY e.entry_date, e.created_at`, id)
	if err != nil {
		return b, err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return b, err
		}
		b.Lines = append(b.Lines, lineOf(e))
	}
	return b, rows.Err()
}

func (r *Repository) unbilledEntries(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, org, matterID, from, to string) ([]Entry, error) {
	rows, err := q.Query(ctx, `SELECT `+entryCols+entryFrom+`WHERE e.org_id=$1 AND e.matter_id=$2 AND e.billable AND e.bill_id IS NULL
		AND ($3='' OR e.entry_date >= $3::date) AND ($4='' OR e.entry_date <= $4::date) ORDER BY e.entry_date, e.created_at FOR UPDATE OF e`, org, matterID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DraftBill collects a matter's unbilled work in [from, to] into a draft bill. A flat-fee matter whose fee has
// not been billed gets one line for the agreed fee. With Preview nothing is changed.
func (r *Repository) DraftBill(ctx context.Context, a knowkit.Actor, matterID string, in BillInput) (Bill, error) {
	m, err := r.loadMatter(ctx, a, matterID)
	if err != nil {
		return Bill{}, err
	}
	if (in.From != "" && !ValidDate(in.From)) || (in.To != "" && !ValidDate(in.To)) {
		return Bill{}, knowkit.Invalid("from and to must be dates")
	}
	if in.DiscountBp < 0 || in.DiscountBp > 10000 {
		return Bill{}, knowkit.Invalid("the discount must be between 0 and 100 percent")
	}
	s, err := r.GetSettings(ctx, a.Org)
	if err != nil {
		return Bill{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Bill{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Lock the matter row so two people drafting at once do not bill the same work twice.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM adrient.legal_matters WHERE id=$1 FOR UPDATE`, matterID); err != nil {
		return Bill{}, err
	}
	entries, err := r.unbilledEntries(ctx, tx, a.Org, matterID, in.From, in.To)
	if err != nil {
		return Bill{}, err
	}
	var flatID string
	today := r.today()
	if m.BillingMethod == "flat" && m.FlatFee > 0 && !m.FlatFeeBilled {
		var has bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.legal_entries WHERE matter_id=$1 AND kind='flat' AND billable)`, matterID).Scan(&has)
		if !has && !in.Preview {
			day := today
			if in.To != "" && day > in.To {
				day = in.To
			}
			if in.From != "" && day < in.From {
				day = in.From
			}
			if err := tx.QueryRow(ctx, `INSERT INTO adrient.legal_entries (org_id, matter_id, kind, user_id, entry_date, amount, description, source_ref)
				VALUES ($1,$2,'flat',$3,$4,$5,$6,$7) RETURNING id::text`, a.Org, matterID, a.User, day, m.FlatFee, "Flat fee: "+m.Title, "flat:"+matterID).Scan(&flatID); err != nil {
				return Bill{}, err
			}
			entries, err = r.unbilledEntries(ctx, tx, a.Org, matterID, in.From, in.To)
			if err != nil {
				return Bill{}, err
			}
		} else if !has {
			entries = append([]Entry{{Kind: "flat", Date: today, Description: "Flat fee: " + m.Title, Amount: m.FlatFee, Billable: true}}, entries...)
		}
	}
	if len(entries) == 0 {
		return Bill{}, knowkit.Invalid("there is no unbilled work in that period")
	}
	lines := make([]Line, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, lineOf(e))
	}
	t := BillTotals(lines, in.DiscountBp, s.TaxRateBp, false)
	b := Bill{MatterID: matterID, MatterNo: m.Number, MatterTitle: m.Title, ClientName: m.ClientName, Status: "draft", IssueDate: today,
		DueDate: AddDays(today, s.TermsDays), PeriodFrom: in.From, PeriodTo: in.To, Fees: t.Fees, Expenses: t.Expenses, Discount: t.Discount, Tax: t.Tax, Total: t.Total,
		Lines: lines, Currency: s.Currency}
	if in.Preview {
		b.Number = "(preview)"
		return b, nil
	}
	var n int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM adrient.legal_bills WHERE matter_id=$1`, matterID).Scan(&n)
	b.Number = fmt.Sprintf("%s/B%02d", m.Number, n+1)
	var from, to any
	if in.From != "" {
		from = in.From
	}
	if in.To != "" {
		to = in.To
	}
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.legal_bills (org_id, matter_id, number, issue_date, due_date, period_from, period_to, fees, expenses, discount, tax, total, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text`, a.Org, matterID, b.Number, b.IssueDate, b.DueDate, from, to, t.Fees, t.Expenses, t.Discount, t.Tax, t.Total, a.User).Scan(&b.ID); err != nil {
		return Bill{}, err
	}
	for _, e := range entries {
		id := e.ID
		if id == "" {
			id = flatID
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.legal_entries SET bill_id=$2 WHERE id=$1`, id, b.ID); err != nil {
			return Bill{}, err
		}
	}
	if flatID != "" {
		if _, err := tx.Exec(ctx, `UPDATE adrient.legal_matters SET flat_fee_billed=true WHERE id=$1`, matterID); err != nil {
			return Bill{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Bill{}, err
	}
	r.audit(ctx, "bill_draft", "legal_bill", b.ID)
	return r.Bill(ctx, a, b.ID)
}

// IssueBill creates the draft invoice in Invoicing, links it and marks the bill issued. The invoice's own total
// (Invoicing applies the tax per line) becomes the bill total.
func (r *Repository) IssueBill(ctx context.Context, a knowkit.Actor, id string) (Bill, error) {
	b, err := r.Bill(ctx, a, id)
	if err != nil {
		return b, err
	}
	if b.Status != "draft" {
		return b, knowkit.Invalid("only a draft bill can be issued")
	}
	if r.Deps.Invoices == nil {
		return b, knowkit.ErrUnavailable
	}
	m, err := r.loadMatter(ctx, a, b.MatterID)
	if err != nil {
		return b, err
	}
	s, err := r.GetSettings(ctx, a.Org)
	if err != nil {
		return b, err
	}
	var cl Client
	if m.ClientID != "" {
		cl, _ = r.GetClient(ctx, a, m.ClientID)
	}
	if cl.Name == "" {
		return b, knowkit.Invalid("add a client to the matter before issuing a bill")
	}
	d := opshttp.InvoiceDraft{Kind: "invoice", ContactID: cl.ContactID, ClientName: cl.Name, ClientEmail: cl.Email, Currency: s.Currency,
		IssueDate: b.IssueDate, DueDate: b.DueDate, Notes: fmt.Sprintf("Matter %s: %s", m.Number, m.Title), SourceRef: "legal:bill:" + b.ID, DiscountKind: "percent"}
	if b.Fees > 0 && b.Discount > 0 {
		d.DiscountValue = math.Round(float64(b.Discount)/float64(b.Fees)*10000) / 100
	}
	taxPct := float64(s.TaxRateBp) / 100
	for _, l := range b.Lines {
		desc := l.Description
		switch l.Kind {
		case "time":
			desc = fmt.Sprintf("%s: %s (%.2f h)", l.Date, nonEmpty(l.Description, "Professional services"), float64(l.Minutes)/60)
		case "expense":
			desc = fmt.Sprintf("%s: %s (expense)", l.Date, nonEmpty(l.Description, "Disbursement"))
		}
		il := opshttp.InvLine{Description: desc, Qty: 1, Unit: "", Price: float64(l.Amount) / 100}
		if l.Kind != "expense" {
			il.TaxRate = taxPct
		}
		d.Lines = append(d.Lines, il)
	}
	doc, err := r.Deps.Invoices.Create(ctx, a.Org, a.User, d)
	if err != nil {
		return b, err
	}
	total := int64(math.Round(doc.Total * 100))
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.legal_bills SET status='issued', invoice_id=$3, invoice_number=$4, total=$5, tax=GREATEST($5 - fees - expenses + discount, 0)
		WHERE org_id=$1 AND id=$2 AND status='draft'`, a.Org, id, doc.ID, doc.Number, total); err != nil {
		return b, err
	}
	r.audit(ctx, "bill_issue", "legal_bill", id)
	return r.Bill(ctx, a, id)
}

// VoidBill cancels a bill and returns its work to unbilled. An issued bill keeps its Invoicing document, which
// the person must cancel there; the reply says so.
func (r *Repository) VoidBill(ctx context.Context, a knowkit.Actor, id string) (Bill, error) {
	b, err := r.Bill(ctx, a, id)
	if err != nil {
		return b, err
	}
	if b.Status == "void" {
		return b, knowkit.Invalid("this bill is already void")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return b, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// A flat-fee entry made for this bill goes away so the fee can be billed again.
	var flats int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM adrient.legal_entries WHERE bill_id=$1 AND kind='flat' AND source_ref LIKE 'flat:%'`, id).Scan(&flats)
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.legal_entries WHERE bill_id=$1 AND kind='flat' AND source_ref LIKE 'flat:%'`, id); err != nil {
		return b, err
	}
	if flats > 0 {
		if _, err := tx.Exec(ctx, `UPDATE adrient.legal_matters SET flat_fee_billed=false WHERE id=$1`, b.MatterID); err != nil {
			return b, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.legal_entries SET bill_id=NULL WHERE bill_id=$1`, id); err != nil {
		return b, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.legal_bills SET status='void' WHERE id=$1`, id); err != nil {
		return b, err
	}
	if err := tx.Commit(ctx); err != nil {
		return b, err
	}
	r.audit(ctx, "bill_void", "legal_bill", id)
	out, err := r.Bill(ctx, a, id)
	if b.InvoiceNumber != "" {
		out.Note = "The work is unbilled again. Invoice " + b.InvoiceNumber + " still exists in Invoicing; cancel it there."
	}
	return out, err
}

func nonEmpty(a, b string) string {
	if strings.TrimSpace(a) == "" {
		return b
	}
	return a
}

var _ = time.Now
