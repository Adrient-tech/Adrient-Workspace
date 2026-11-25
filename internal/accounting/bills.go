package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// PermApprove is the permission needed to approve bills over the approval limit.
// Owners and admins hold it through "*"; roles with "accounting:*" hold it too.
const PermApprove = "accounting:bills:approve"

// BillLineIn is one line of a bill or vendor credit.
type BillLineIn struct {
	AccountID   string  `json:"account_id"`
	Description string  `json:"description"`
	HSN         string  `json:"hsn"`
	Amount      float64 `json:"amount"` // before tax, document currency
	TaxRate     float64 `json:"tax_rate"`
	Class       string  `json:"class"`
}

// BillInput creates a bill (kind "bill") or a vendor credit / debit note (kind "vendor_credit").
type BillInput struct {
	Kind        string       `json:"kind"`
	Number      string       `json:"number"`
	VendorID    string       `json:"vendor_id"`
	VendorName  string       `json:"vendor_name"`
	BillDate    string       `json:"bill_date"`
	DueDate     string       `json:"due_date"`
	Currency    string       `json:"currency"`
	Rate        float64      `json:"rate"` // 0 = from the rate table
	Lines       []BillLineIn `json:"lines"`
	Notes       string       `json:"notes"`
	Attachments []Attachment `json:"attachments"`
	Draft       bool         `json:"draft"`
	// SkipApproval is set by imports, which bring in already-approved history.
	SkipApproval bool `json:"-"`
}

// Attachment is a file kept in Drive and linked from an accounting record.
type Attachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BillPayment is a payment (or an applied credit) against a bill.
type BillPayment struct {
	ID        string  `json:"id"`
	Date      string  `json:"date"`
	Amount    float64 `json:"amount"`
	AccountID string  `json:"account_id"`
	CreditID  string  `json:"credit_id"`
	Rate      float64 `json:"rate"`
	Reference string  `json:"reference"`
}

// Bill is a stored bill or vendor credit.
type Bill struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	Number       string        `json:"number"`
	VendorID     string        `json:"vendor_id"`
	VendorName   string        `json:"vendor_name"`
	BillDate     string        `json:"bill_date"`
	DueDate      string        `json:"due_date"`
	Currency     string        `json:"currency"`
	Rate         float64       `json:"rate"`
	Subtotal     float64       `json:"subtotal"`
	TaxTotal     float64       `json:"tax_total"`
	Total        float64       `json:"total"`
	Paid         float64       `json:"paid"`
	Balance      float64       `json:"balance"`
	Status       string        `json:"status"`
	ApprovedBy   string        `json:"approved_by"`
	ApprovalNote string        `json:"approval_note"`
	Notes        string        `json:"notes"`
	Attachments  []Attachment  `json:"attachments"`
	Lines        []BillLineIn  `json:"lines"`
	Payments     []BillPayment `json:"payments"`
	CreatedBy    string        `json:"created_by"`
}

// computeBill derives line tax and totals in paise.
func computeBill(lines []BillLineIn) (sub, tax int64, taxes []int64, err error) {
	if len(lines) == 0 {
		return 0, 0, nil, invalid("add at least one line")
	}
	for _, l := range lines {
		a := paise(l.Amount)
		if a <= 0 {
			return 0, 0, nil, invalid("line amounts must be greater than zero")
		}
		if l.TaxRate < 0 || l.TaxRate > 100 {
			return 0, 0, nil, invalid("tax rate must be between 0 and 100")
		}
		t := int64(float64(a)*l.TaxRate/100 + 0.5)
		sub += a
		tax += t
		taxes = append(taxes, t)
	}
	return sub, tax, taxes, nil
}

func (r *Repository) loadBill(ctx context.Context, orgID, id string) (Bill, error) {
	if !isUUID(id) {
		return Bill{}, ErrNotFound
	}
	var b Bill
	var att []byte
	err := r.pool.QueryRow(ctx, `SELECT id::text, kind, number, vendor_id, vendor_name, to_char(bill_date,'YYYY-MM-DD'), coalesce(to_char(due_date,'YYYY-MM-DD'),''),
		currency, fx_rate::float8, subtotal::float8, tax_total::float8, total::float8, paid::float8, status, approved_by, approval_note, notes, attachments, created_by
		FROM adrient.acct_bills WHERE org_id=$1 AND id=$2::uuid`, orgID, id).
		Scan(&b.ID, &b.Kind, &b.Number, &b.VendorID, &b.VendorName, &b.BillDate, &b.DueDate, &b.Currency, &b.Rate, &b.Subtotal, &b.TaxTotal, &b.Total, &b.Paid,
			&b.Status, &b.ApprovedBy, &b.ApprovalNote, &b.Notes, &att, &b.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return Bill{}, ErrNotFound
	}
	if err != nil {
		return Bill{}, err
	}
	b.Attachments = []Attachment{}
	_ = json.Unmarshal(att, &b.Attachments)
	b.Balance = rupees(paise(b.Total) - paise(b.Paid))
	b.Lines, b.Payments = []BillLineIn{}, []BillPayment{}
	rows, err := r.pool.Query(ctx, `SELECT account_id::text, description, hsn, amount::float8, tax_rate::float8, class FROM adrient.acct_bill_lines WHERE bill_id=$1::uuid ORDER BY position`, id)
	if err != nil {
		return b, err
	}
	for rows.Next() {
		var l BillLineIn
		if err := rows.Scan(&l.AccountID, &l.Description, &l.HSN, &l.Amount, &l.TaxRate, &l.Class); err != nil {
			rows.Close()
			return b, err
		}
		b.Lines = append(b.Lines, l)
	}
	rows.Close()
	prow, err := r.pool.Query(ctx, `SELECT id::text, to_char(pay_date,'YYYY-MM-DD'), amount::float8, coalesce(account_id::text,''), coalesce(credit_id::text,''), fx_rate::float8, reference
		FROM adrient.acct_bill_payments WHERE bill_id=$1::uuid ORDER BY pay_date, created_at`, id)
	if err != nil {
		return b, err
	}
	defer prow.Close()
	for prow.Next() {
		var p BillPayment
		if err := prow.Scan(&p.ID, &p.Date, &p.Amount, &p.AccountID, &p.CreditID, &p.Rate, &p.Reference); err != nil {
			return b, err
		}
		b.Payments = append(b.Payments, p)
	}
	return b, prow.Err()
}

// GetBill returns one bill.
func (r *Repository) GetBill(ctx context.Context, orgID, id string) (Bill, error) {
	return r.loadBill(ctx, orgID, id)
}

// ListBills returns bills, newest first. status "" = all, "unpaid" = open with a balance.
func (r *Repository) ListBills(ctx context.Context, orgID, kind, status, vendorID string) ([]Bill, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM adrient.acct_bills WHERE org_id=$1 AND ($2='' OR kind=$2)
		AND ($3='' OR status=$3 OR ($3='unpaid' AND status='open')) AND ($4='' OR vendor_id=$4) ORDER BY bill_date DESC, created_at DESC LIMIT 500`, orgID, kind, status, vendorID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	out := []Bill{}
	for _, id := range ids {
		b, err := r.loadBill(ctx, orgID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// SaveBill creates a bill or vendor credit (id "") or replaces a draft/pending one.
// A bill above the approval limit waits in pending_approval and is not posted.
func (r *Repository) SaveBill(ctx context.Context, orgID, userID, id string, in BillInput) (Bill, error) {
	if in.Kind == "" {
		in.Kind = "bill"
	}
	if in.Kind != "bill" && in.Kind != "vendor_credit" {
		return Bill{}, invalid("kind must be bill or vendor_credit")
	}
	in.VendorName = strings.TrimSpace(in.VendorName)
	if in.VendorName == "" && in.VendorID == "" {
		return Bill{}, invalid("choose a vendor")
	}
	bd, err := validDate(in.BillDate)
	if err != nil {
		return Bill{}, err
	}
	if in.DueDate != "" {
		if in.DueDate, err = validDate(in.DueDate); err != nil {
			return Bill{}, err
		}
	}
	sub, tax, _, err := computeBill(in.Lines)
	if err != nil {
		return Bill{}, err
	}
	for _, l := range in.Lines {
		if !isUUID(l.AccountID) {
			return Bill{}, invalid("every line needs an account")
		}
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Bill{}, err
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	rate := 1.0
	if cur == st.Currency {
		cur = ""
	}
	if cur != "" {
		rate = in.Rate
		if rate <= 0 {
			if rate, err = r.RateOn(ctx, orgID, cur, bd); err != nil {
				return Bill{}, err
			}
		}
	}
	status := "open"
	switch {
	case in.Draft:
		status = "draft"
	case in.Kind == "bill" && !in.SkipApproval && st.ApprovalLimit > 0 && float64(sub+tax)/100*rate > st.ApprovalLimit:
		status = "pending_approval"
	}
	attJSON, _ := json.Marshal(in.Attachments)
	if in.Attachments == nil {
		attJSON = []byte("[]")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Bill{}, err
	}
	defer tx.Rollback(ctx)
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO adrient.acct_bills (org_id, kind, number, vendor_id, vendor_name, bill_date, due_date, currency, fx_rate, subtotal, tax_total, total,
			status, notes, attachments, created_by) VALUES ($1,$2,$3,$4,$5,$6::date,NULLIF($7,'')::date,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16) RETURNING id::text`,
			orgID, in.Kind, strings.TrimSpace(in.Number), in.VendorID, in.VendorName, bd, in.DueDate, cur, rate, rupees(sub), rupees(tax), rupees(sub+tax), status, in.Notes, string(attJSON), userID).Scan(&id)
	} else {
		var cur0 Bill
		if cur0, err = r.loadBill(ctx, orgID, id); err != nil {
			return Bill{}, err
		}
		if cur0.Status != "draft" && cur0.Status != "pending_approval" && cur0.Status != "rejected" {
			return Bill{}, invalid("a posted bill cannot be edited; void it and create a new one")
		}
		_, err = tx.Exec(ctx, `UPDATE adrient.acct_bills SET number=$3, vendor_id=$4, vendor_name=$5, bill_date=$6::date, due_date=NULLIF($7,'')::date, currency=$8, fx_rate=$9,
			subtotal=$10, tax_total=$11, total=$12, status=$13, notes=$14, attachments=$15::jsonb, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`,
			orgID, id, strings.TrimSpace(in.Number), in.VendorID, in.VendorName, bd, in.DueDate, cur, rate, rupees(sub), rupees(tax), rupees(sub+tax), status, in.Notes, string(attJSON))
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM adrient.acct_bill_lines WHERE bill_id=$1::uuid`, id)
		}
	}
	if err != nil {
		return Bill{}, err
	}
	for i, l := range in.Lines {
		t := int64(float64(paise(l.Amount))*l.TaxRate/100 + 0.5)
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.acct_bill_lines (bill_id, org_id, account_id, description, hsn, amount, tax_rate, tax_amount, class, position)
			VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8,$9,$10)`, id, orgID, l.AccountID, l.Description, l.HSN, rupees(paise(l.Amount)), l.TaxRate, rupees(t), strings.TrimSpace(l.Class), i); err != nil {
			return Bill{}, invalid("a line uses an account that does not exist")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Bill{}, err
	}
	r.audit(ctx, orgID, userID, "save", "bill", id, fmt.Sprintf("%s %s %s %.2f", in.Kind, in.Number, in.VendorName, rupees(sub+tax)))
	if status == "open" {
		if err := r.postBill(ctx, orgID, userID, id); err != nil {
			// A closed period or similar: do not leave a posted-looking bill without a journal.
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET status='draft' WHERE id=$1::uuid`, id)
			return Bill{}, err
		}
	}
	return r.loadBill(ctx, orgID, id)
}

// postBill writes (or rewrites) the bill's journal.
func (r *Repository) postBill(ctx context.Context, orgID, userID, id string) error {
	b, err := r.loadBill(ctx, orgID, id)
	if err != nil {
		return err
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return err
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return err
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return err
	}
	credit := b.Kind == "vendor_credit"
	memo := map[bool]string{false: "Bill ", true: "Vendor credit "}[credit] + orDefault(b.Number, b.VendorName)
	var lines []Line
	var taxTotal int64
	for _, l := range b.Lines {
		t := int64(float64(paise(l.Amount))*l.TaxRate/100 + 0.5)
		taxTotal += t
		ln := Line{AccountID: l.AccountID, Memo: orDefault(l.Description, memo), Class: l.Class}
		if credit {
			ln.Credit = l.Amount
		} else {
			ln.Debit = l.Amount
		}
		lines = append(lines, ln)
	}
	for _, tl := range taxLines(keys, taxTotal, st.Intrastate, false, memo) { // input GST debit lines
		if credit {
			tl.Debit, tl.Credit = 0, tl.Debit
		}
		lines = append(lines, tl)
	}
	ap := Line{AccountID: keys["ap"], Memo: memo}
	total := rupees(paise(b.Subtotal) + taxTotal)
	if credit {
		ap.Debit = total
	} else {
		ap.Credit = total
	}
	lines = append(lines, ap)
	in := JournalInput{Date: b.BillDate, DueDate: b.DueDate, Memo: memo, Ref: b.Number, Kind: "bill", Source: SourceBill, SourceID: b.ID,
		ContactID: b.VendorID, ContactName: b.VendorName, Currency: b.Currency, Rate: b.Rate, Lines: lines}
	if credit {
		in.Source = SourceVendorCredit
	}
	if b.Currency != "" {
		in.Lines = toBase(in.Lines, b.Rate)
	}
	in.SourceHash = hashOf(b.BillDate, b.DueDate, b.Total, b.Currency, b.Rate, len(b.Lines), taxTotal)
	_, _, err = r.PostJournal(ctx, orgID, userID, in)
	return err
}

// ApproveBill approves (or rejects) a bill waiting for approval and posts it.
// The caller must already have been checked for PermApprove.
func (r *Repository) ApproveBill(ctx context.Context, orgID, userID, approverName, id string, approve bool, note string) (Bill, error) {
	b, err := r.loadBill(ctx, orgID, id)
	if err != nil {
		return Bill{}, err
	}
	if b.Status != "pending_approval" {
		return Bill{}, invalid("this bill is not waiting for approval")
	}
	if !approve {
		_, err = r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET status='rejected', approved_by=$3, approval_note=$4, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`, orgID, id, approverName, note)
		if err == nil {
			r.audit(ctx, orgID, userID, "reject", "bill", id, note)
		}
		if err != nil {
			return Bill{}, err
		}
		return r.loadBill(ctx, orgID, id)
	}
	if userID != "" && b.CreatedBy == userID {
		// separation of duties: the creator cannot approve their own bill
		return Bill{}, invalid("a bill must be approved by someone other than its creator")
	}
	if _, err = r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET status='open', approved_by=$3, approval_note=$4, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`, orgID, id, approverName, note); err != nil {
		return Bill{}, err
	}
	if err := r.postBill(ctx, orgID, userID, id); err != nil {
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET status='pending_approval' WHERE id=$1::uuid`, id)
		return Bill{}, err
	}
	r.audit(ctx, orgID, userID, "approve", "bill", id, note)
	return r.loadBill(ctx, orgID, id)
}

// PostDraftBill posts a draft bill (applying the approval limit again).
func (r *Repository) PostDraftBill(ctx context.Context, orgID, userID, id string) (Bill, error) {
	b, err := r.loadBill(ctx, orgID, id)
	if err != nil {
		return Bill{}, err
	}
	if b.Status != "draft" && b.Status != "rejected" {
		return Bill{}, invalid("only a draft bill can be posted")
	}
	in := BillInput{Kind: b.Kind, Number: b.Number, VendorID: b.VendorID, VendorName: b.VendorName, BillDate: b.BillDate, DueDate: b.DueDate,
		Currency: b.Currency, Rate: b.Rate, Lines: b.Lines, Notes: b.Notes, Attachments: b.Attachments}
	return r.SaveBill(ctx, orgID, userID, id, in)
}

func (r *Repository) refreshBillStatus(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET status = CASE WHEN paid >= total THEN 'paid' ELSE 'open' END, updated_at=now()
		WHERE id=$1::uuid AND status IN ('open','paid')`, id)
	return err
}

// PayVendorBill records a payment on a bill (cash out) or, for a vendor credit, a refund
// received from the vendor. Foreign bills settle at the payment-date rate and book
// the realised exchange gain or loss.
func (r *Repository) PayVendorBill(ctx context.Context, orgID, userID, id string, p BillPayment) (Bill, error) {
	b, err := r.loadBill(ctx, orgID, id)
	if err != nil {
		return Bill{}, err
	}
	if b.Status != "open" {
		return Bill{}, invalid("only a posted, unpaid bill can take a payment")
	}
	d, err := validDate(p.Date)
	if err != nil {
		return Bill{}, err
	}
	a := paise(p.Amount)
	if a <= 0 {
		return Bill{}, invalid("amount must be greater than zero")
	}
	if a > paise(b.Total)-paise(b.Paid) {
		return Bill{}, invalid("amount exceeds the balance %.2f", b.Balance)
	}
	if !isUUID(p.AccountID) {
		return Bill{}, invalid("choose the bank or cash account")
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return Bill{}, err
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return Bill{}, err
	}
	payRate := 1.0
	if b.Currency != "" {
		payRate = p.Rate
		if payRate <= 0 {
			if payRate, err = r.RateOn(ctx, orgID, b.Currency, d); err != nil {
				return Bill{}, err
			}
		}
	}
	var pid string
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Bill{}, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.acct_bill_payments (org_id, bill_id, pay_date, amount, account_id, fx_rate, reference)
		VALUES ($1,$2::uuid,$3::date,$4,$5::uuid,$6,$7) RETURNING id::text`, orgID, id, d, rupees(a), p.AccountID, payRate, p.Reference).Scan(&pid); err != nil {
		return Bill{}, invalid("unknown account")
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.acct_bills SET paid = paid + $2, updated_at=now() WHERE id=$1::uuid`, id, rupees(a)); err != nil {
		return Bill{}, err
	}
	memo := "Payment " + orDefault(b.Number, b.VendorName)
	receipt := b.Kind == "vendor_credit" // vendor refunds us
	docRate := b.Rate
	if docRate <= 0 {
		docRate = 1
	}
	lines := settlementLines(p.AccountID, keys["ap"], keys["fx_gain_loss"], rupees(a), docRate, payRate, receipt, memo)
	in := JournalInput{Date: d, Memo: memo, Ref: p.Reference, Kind: "payment", Source: SourceBillPayment, SourceID: pid,
		ContactID: b.VendorID, ContactName: b.VendorName, Currency: b.Currency, Rate: payRate, Lines: lines,
		SourceHash: hashOf(d, a, docRate, payRate)}
	if _, _, err := r.postJournalTx(ctx, tx, orgID, userID, in); err != nil {
		return Bill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Bill{}, err
	}
	_ = r.refreshBillStatus(ctx, id)
	r.audit(ctx, orgID, userID, "pay", "bill", id, fmt.Sprintf("%.2f", rupees(a)))
	return r.loadBill(ctx, orgID, id)
}

// postJournalTx posts using the repository, outside the transaction (journals take
// their own lock). The payment row is only committed if the journal posts: callers
// invoke this before Commit, and a failure rolls the payment row back.
func (r *Repository) postJournalTx(ctx context.Context, _ pgx.Tx, orgID, userID string, in JournalInput) (Journal, bool, error) {
	return r.PostJournal(ctx, orgID, userID, in)
}

// ApplyCredit uses a vendor credit to settle part of a bill (same vendor and currency).
func (r *Repository) ApplyCredit(ctx context.Context, orgID, userID, billID, creditID, date string, amount float64) (Bill, error) {
	b, err := r.loadBill(ctx, orgID, billID)
	if err != nil {
		return Bill{}, err
	}
	c, err := r.loadBill(ctx, orgID, creditID)
	if err != nil {
		return Bill{}, err
	}
	d, err := validDate(date)
	if err != nil {
		return Bill{}, err
	}
	a := paise(amount)
	switch {
	case b.Kind != "bill" || c.Kind != "vendor_credit":
		return Bill{}, invalid("apply a vendor credit to a bill")
	case b.Status != "open" || c.Status != "open":
		return Bill{}, invalid("both documents must be posted and open")
	case b.Currency != c.Currency:
		return Bill{}, invalid("credit and bill must be in the same currency")
	case b.VendorID != c.VendorID || b.VendorName != c.VendorName:
		return Bill{}, invalid("credit and bill must be for the same vendor")
	case a <= 0 || a > paise(b.Total)-paise(b.Paid) || a > paise(c.Total)-paise(c.Paid):
		return Bill{}, invalid("amount must be positive and within both balances")
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return Bill{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Bill{}, err
	}
	defer tx.Rollback(ctx)
	var pid string
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.acct_bill_payments (org_id, bill_id, credit_id, pay_date, amount, fx_rate, reference)
		VALUES ($1,$2::uuid,$3::uuid,$4::date,$5,$6,$7) RETURNING id::text`, orgID, billID, creditID, d, rupees(a), b.Rate, "credit "+c.Number).Scan(&pid); err != nil {
		return Bill{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.acct_bills SET paid = paid + $2, updated_at=now() WHERE id = ANY($1::uuid[])`, []string{billID, creditID}, rupees(a)); err != nil {
		return Bill{}, err
	}
	// Both sides sit in Accounts Payable. Only an exchange-rate difference between the two bookings moves money.
	if b.Currency != "" && b.Rate != c.Rate {
		lines := []Line{
			{AccountID: keys["ap"], Debit: rupees(int64(float64(a)*b.Rate + 0.5)), FcyDebit: rupees(a), Memo: "Credit applied"},
			{AccountID: keys["ap"], Credit: rupees(int64(float64(a)*c.Rate + 0.5)), FcyCredit: rupees(a), Memo: "Credit applied"},
		}
		diff := paise(lines[0].Debit) - paise(lines[1].Credit)
		if diff > 0 {
			lines = append(lines, Line{AccountID: keys["fx_gain_loss"], Credit: rupees(diff), Memo: "Realised exchange gain"})
		} else if diff < 0 {
			lines = append(lines, Line{AccountID: keys["fx_gain_loss"], Debit: rupees(-diff), Memo: "Realised exchange loss"})
		}
		if _, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: d, Memo: "Vendor credit applied to " + b.Number, Kind: "journal", Source: SourceBillPayment,
			SourceID: pid, ContactID: b.VendorID, ContactName: b.VendorName, Currency: b.Currency, Rate: b.Rate, Lines: lines, SourceHash: hashOf(pid, a)}); err != nil {
			return Bill{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Bill{}, err
	}
	_ = r.refreshBillStatus(ctx, billID)
	_ = r.refreshBillStatus(ctx, creditID)
	r.audit(ctx, orgID, userID, "apply_credit", "bill", billID, fmt.Sprintf("%.2f from %s", rupees(a), c.Number))
	return r.loadBill(ctx, orgID, billID)
}

// VoidBill voids an unpaid bill and its journal.
func (r *Repository) VoidBill(ctx context.Context, orgID, userID, id string) (Bill, error) {
	b, err := r.loadBill(ctx, orgID, id)
	if err != nil {
		return Bill{}, err
	}
	if len(b.Payments) > 0 || b.Paid > 0 {
		return Bill{}, invalid("this bill has payments; delete them first")
	}
	if b.Status == "void" {
		return b, nil
	}
	src := SourceBill
	if b.Kind == "vendor_credit" {
		src = SourceVendorCredit
	}
	if _, err := r.voidBySource(ctx, orgID, userID, src, id); err != nil {
		return Bill{}, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET status='void', updated_at=now() WHERE org_id=$1 AND id=$2::uuid`, orgID, id); err != nil {
		return Bill{}, err
	}
	r.audit(ctx, orgID, userID, "void", "bill", id, "")
	return r.loadBill(ctx, orgID, id)
}

// DeleteBillPayment removes a payment and voids its journal.
func (r *Repository) DeleteBillPayment(ctx context.Context, orgID, userID, billID, payID string) (Bill, error) {
	b, err := r.loadBill(ctx, orgID, billID)
	if err != nil {
		return Bill{}, err
	}
	var amt float64
	var creditID string
	if !isUUID(payID) {
		return Bill{}, ErrNotFound
	}
	err = r.pool.QueryRow(ctx, `SELECT amount::float8, coalesce(credit_id::text,'') FROM adrient.acct_bill_payments WHERE id=$1::uuid AND bill_id=$2::uuid AND org_id=$3`, payID, billID, orgID).Scan(&amt, &creditID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Bill{}, ErrNotFound
	}
	if err != nil {
		return Bill{}, err
	}
	if _, err := r.voidBySource(ctx, orgID, userID, SourceBillPayment, payID); err != nil {
		return Bill{}, err
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_bill_payments WHERE id=$1::uuid`, payID); err != nil {
		return Bill{}, err
	}
	ids := []string{billID}
	if creditID != "" {
		ids = append(ids, creditID)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET paid = greatest(paid - $2,0), updated_at=now() WHERE id = ANY($1::uuid[])`, ids, amt); err != nil {
		return Bill{}, err
	}
	for _, i := range ids {
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET status='open' WHERE id=$1::uuid AND status='paid' AND paid < total`, i)
	}
	r.audit(ctx, orgID, userID, "delete_payment", "bill", billID, b.Number)
	return r.loadBill(ctx, orgID, billID)
}

// WriteOffBill clears the remaining balance of a bill to the Write-offs account
// (for example a small amount the vendor forgave).
func (r *Repository) WriteOffBill(ctx context.Context, orgID, userID, id, date, note string) (Bill, error) {
	b, err := r.loadBill(ctx, orgID, id)
	if err != nil {
		return Bill{}, err
	}
	if b.Status != "open" || b.Balance <= 0 {
		return Bill{}, invalid("nothing to write off")
	}
	d, err := validDate(date)
	if err != nil {
		return Bill{}, err
	}
	rate := b.Rate
	if rate <= 0 {
		rate = 1
	}
	var pid string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_bill_payments (org_id, bill_id, pay_date, amount, fx_rate, reference) VALUES ($1,$2::uuid,$3::date,$4,$5,'write-off') RETURNING id::text`,
		orgID, id, d, b.Balance, rate).Scan(&pid); err != nil {
		return Bill{}, err
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return Bill{}, err
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	memo := "Write-off " + orDefault(b.Number, b.VendorName) + " " + note
	ap := Line{AccountID: keys["ap"], Memo: memo}
	wo := Line{AccountID: keys["write_off"], Memo: memo}
	if b.Kind == "vendor_credit" {
		ap.Credit, wo.Debit = b.Balance, b.Balance
	} else {
		ap.Debit, wo.Credit = b.Balance, b.Balance
	}
	in := JournalInput{Date: d, Memo: memo, Kind: "journal", Source: SourceBillPayment, SourceID: pid, ContactID: b.VendorID, ContactName: b.VendorName,
		Currency: b.Currency, Rate: rate, Lines: []Line{ap, wo}, SourceHash: hashOf(pid)}
	if b.Currency != "" {
		in.Lines = toBase(in.Lines, rate)
	}
	if _, _, err := r.PostJournal(ctx, orgID, userID, in); err != nil {
		_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.acct_bill_payments WHERE id=$1::uuid`, pid)
		return Bill{}, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_bills SET paid=total, status='paid', updated_at=now() WHERE id=$1::uuid`, id); err != nil {
		return Bill{}, err
	}
	r.audit(ctx, orgID, userID, "write_off", "bill", id, fmt.Sprintf("%.2f", b.Balance))
	return r.loadBill(ctx, orgID, id)
}
