package accounting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CreditMemoInput creates a customer credit memo.
type CreditMemoInput struct {
	Number       string  `json:"number"`
	CustomerID   string  `json:"customer_id"`
	CustomerName string  `json:"customer_name"`
	Date         string  `json:"date"`
	Currency     string  `json:"currency"`
	Rate         float64 `json:"rate"`
	AccountID    string  `json:"account_id"` // revenue account to reduce, default Sales Returns
	Amount       float64 `json:"amount"`     // before GST
	GSTRate      float64 `json:"gst_rate"`
	Reason       string  `json:"reason"`
}

// CreditMemo is a stored credit memo.
type CreditMemo struct {
	ID           string  `json:"id"`
	Number       string  `json:"number"`
	CustomerID   string  `json:"customer_id"`
	CustomerName string  `json:"customer_name"`
	Date         string  `json:"date"`
	Currency     string  `json:"currency"`
	Rate         float64 `json:"rate"`
	AccountID    string  `json:"account_id"`
	Amount       float64 `json:"amount"`
	GSTRate      float64 `json:"gst_rate"`
	TaxTotal     float64 `json:"tax_total"`
	Total        float64 `json:"total"`
	Refunded     float64 `json:"refunded"`
	Balance      float64 `json:"balance"`
	Reason       string  `json:"reason"`
	Status       string  `json:"status"`
}

const memoCols = `id::text, number, customer_id, customer_name, to_char(memo_date,'YYYY-MM-DD'), currency, fx_rate::float8, account_id::text, amount::float8, gst_rate::float8,
	tax_total::float8, total::float8, refunded::float8, reason, status`

func scanMemo(row pgx.Row) (CreditMemo, error) {
	var m CreditMemo
	err := row.Scan(&m.ID, &m.Number, &m.CustomerID, &m.CustomerName, &m.Date, &m.Currency, &m.Rate, &m.AccountID, &m.Amount, &m.GSTRate, &m.TaxTotal, &m.Total, &m.Refunded, &m.Reason, &m.Status)
	m.Balance = rupees(paise(m.Total) - paise(m.Refunded))
	return m, err
}

func (r *Repository) GetCreditMemo(ctx context.Context, orgID, id string) (CreditMemo, error) {
	if !isUUID(id) {
		return CreditMemo{}, ErrNotFound
	}
	m, err := scanMemo(r.pool.QueryRow(ctx, `SELECT `+memoCols+` FROM adrient.acct_credit_memos WHERE org_id=$1 AND id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

func (r *Repository) ListCreditMemos(ctx context.Context, orgID string) ([]CreditMemo, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+memoCols+` FROM adrient.acct_credit_memos WHERE org_id=$1 ORDER BY memo_date DESC, created_at DESC LIMIT 500`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CreditMemo{}
	for rows.Next() {
		m, err := scanMemo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateCreditMemo records a credit to a customer: Dr Sales Returns (or the chosen
// revenue account), Dr GST output (reversed), Cr Accounts Receivable. The credit
// stays on the customer's receivable account and nets against their open invoices
// in Aged Receivables.
func (r *Repository) CreateCreditMemo(ctx context.Context, orgID, userID string, in CreditMemoInput) (CreditMemo, error) {
	d, err := validDate(in.Date)
	if err != nil {
		return CreditMemo{}, err
	}
	a := paise(in.Amount)
	if a <= 0 {
		return CreditMemo{}, invalid("amount must be greater than zero")
	}
	if in.GSTRate < 0 || in.GSTRate > 100 {
		return CreditMemo{}, invalid("GST rate must be between 0 and 100")
	}
	if strings.TrimSpace(in.CustomerName) == "" && in.CustomerID == "" {
		return CreditMemo{}, invalid("choose a customer")
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return CreditMemo{}, err
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return CreditMemo{}, err
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return CreditMemo{}, err
	}
	if in.AccountID == "" {
		in.AccountID = keys["sales_returns"]
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	rate := 1.0
	if cur == st.Currency {
		cur = ""
	}
	if cur != "" {
		if rate = in.Rate; rate <= 0 {
			if rate, err = r.RateOn(ctx, orgID, cur, d); err != nil {
				return CreditMemo{}, err
			}
		}
	}
	tax := int64(float64(a)*in.GSTRate/100 + 0.5)
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_credit_memos (org_id, number, customer_id, customer_name, memo_date, currency, fx_rate, account_id, amount, gst_rate, tax_total, total, reason, created_by)
		VALUES ($1,$2,$3,$4,$5::date,$6,$7,$8::uuid,$9,$10,$11,$12,$13,$14) RETURNING id::text`,
		orgID, strings.TrimSpace(in.Number), in.CustomerID, strings.TrimSpace(in.CustomerName), d, cur, rate, in.AccountID, rupees(a), in.GSTRate, rupees(tax), rupees(a+tax), in.Reason, userID).Scan(&id); err != nil {
		return CreditMemo{}, invalid("unknown account")
	}
	memo := "Credit memo " + orDefault(in.Number, in.CustomerName)
	lines := []Line{{AccountID: in.AccountID, Debit: rupees(a), Memo: memo}}
	if tax > 0 {
		for _, l := range taxLinesOut(keys, tax, st.Intrastate, false, memo) {
			lines = append(lines, l)
		}
	}
	lines = append(lines, Line{AccountID: keys["ar"], Credit: rupees(a + tax), Memo: memo})
	j := JournalInput{Date: d, Memo: memo, Ref: in.Number, Kind: "invoice", Source: SourceCreditMemo, SourceID: id, ContactID: in.CustomerID, ContactName: in.CustomerName,
		Currency: cur, Rate: rate, Lines: lines, SourceHash: hashOf(id, a, tax, cur, rate)}
	if cur != "" {
		j.Lines = toBase(lines, rate)
	}
	if _, _, err := r.PostJournal(ctx, orgID, userID, j); err != nil {
		_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.acct_credit_memos WHERE id=$1::uuid`, id)
		return CreditMemo{}, err
	}
	r.audit(ctx, orgID, userID, "create", "credit_memo", id, fmt.Sprintf("%s %.2f", in.CustomerName, rupees(a+tax)))
	return r.GetCreditMemo(ctx, orgID, id)
}

// RefundCreditMemo pays cash back to the customer against a credit memo.
func (r *Repository) RefundCreditMemo(ctx context.Context, orgID, userID, id, date, accountID string, amount float64) (CreditMemo, error) {
	m, err := r.GetCreditMemo(ctx, orgID, id)
	if err != nil {
		return m, err
	}
	d, err := validDate(date)
	if err != nil {
		return m, err
	}
	a := paise(amount)
	if m.Status != "open" || a <= 0 || a > paise(m.Total)-paise(m.Refunded) {
		return m, invalid("refund must be positive and within the open credit %.2f", m.Balance)
	}
	if !isUUID(accountID) {
		return m, invalid("choose the bank or cash account")
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return m, err
	}
	payRate := 1.0
	if m.Currency != "" {
		if payRate, err = r.RateOn(ctx, orgID, m.Currency, d); err != nil {
			return m, err
		}
	}
	docRate := m.Rate
	if docRate <= 0 {
		docRate = 1
	}
	var rid string
	if err := r.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&rid); err != nil {
		return m, err
	}
	memo := "Refund of credit memo " + orDefault(m.Number, m.CustomerName)
	// Dr AR (clear the credit), Cr bank. Mirror image of a customer receipt.
	ls := settlementLines(accountID, keys["ar"], keys["fx_gain_loss"], rupees(a), docRate, payRate, true, memo)
	for i := range ls {
		ls[i].Debit, ls[i].Credit = ls[i].Credit, ls[i].Debit
		ls[i].FcyDebit, ls[i].FcyCredit = ls[i].FcyCredit, ls[i].FcyDebit
	}
	if _, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: d, Memo: memo, Kind: "payment", Source: SourceCreditMemo, SourceID: "refund:" + id + ":" + rid,
		ContactID: m.CustomerID, ContactName: m.CustomerName, Currency: m.Currency, Rate: payRate, Lines: ls, SourceHash: hashOf(rid, a)}); err != nil {
		return m, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_credit_memos SET refunded = refunded + $3 WHERE org_id=$1 AND id=$2::uuid`, orgID, id, rupees(a)); err != nil {
		return m, err
	}
	r.audit(ctx, orgID, userID, "refund", "credit_memo", id, fmt.Sprintf("%.2f", rupees(a)))
	return r.GetCreditMemo(ctx, orgID, id)
}

// VoidCreditMemo voids an unrefunded credit memo.
func (r *Repository) VoidCreditMemo(ctx context.Context, orgID, userID, id string) (CreditMemo, error) {
	m, err := r.GetCreditMemo(ctx, orgID, id)
	if err != nil {
		return m, err
	}
	if m.Refunded > 0 {
		return m, invalid("this credit memo has refunds")
	}
	if _, err := r.voidBySource(ctx, orgID, userID, SourceCreditMemo, id); err != nil {
		return m, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_credit_memos SET status='void' WHERE org_id=$1 AND id=$2::uuid`, orgID, id); err != nil {
		return m, err
	}
	r.audit(ctx, orgID, userID, "void", "credit_memo", id, "")
	return r.GetCreditMemo(ctx, orgID, id)
}

// WriteOffReceivable writes a customer balance off as a bad debt (Dr Bad Debts, Cr Receivable).
func (r *Repository) WriteOffReceivable(ctx context.Context, orgID, userID, customerID, customerName, date string, amount float64, note string) (Journal, error) {
	d, err := validDate(date)
	if err != nil {
		return Journal{}, err
	}
	if paise(amount) <= 0 {
		return Journal{}, invalid("amount must be greater than zero")
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return Journal{}, err
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	var sid string
	_ = r.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&sid)
	memo := "Write-off " + orDefault(customerName, customerID) + " " + note
	j, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: d, Memo: strings.TrimSpace(memo), Kind: "journal", Source: SourceWriteOff, SourceID: sid,
		ContactID: customerID, ContactName: customerName, SourceHash: hashOf(sid, amount),
		Lines: []Line{{AccountID: keys["bad_debt"], Debit: amount}, {AccountID: keys["ar"], Credit: amount}}})
	return j, err
}

// ---- deposits (Undeposited Funds) ----

// Undeposited is one receipt waiting in Undeposited Funds.
type Undeposited struct {
	LineID      string  `json:"line_id"`
	Date        string  `json:"date"`
	Memo        string  `json:"memo"`
	ContactName string  `json:"contact_name"`
	Amount      float64 `json:"amount"`
	JournalID   string  `json:"journal_id"`
}

// ListUndeposited returns receipts in Undeposited Funds not yet put in a deposit.
func (r *Repository) ListUndeposited(ctx context.Context, orgID string) ([]Undeposited, error) {
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT l.id::text, to_char(j.entry_date,'YYYY-MM-DD'), j.memo, j.contact_name, (l.debit-l.credit)::float8, j.id::text
		FROM adrient.acct_lines l JOIN adrient.acct_journals j ON j.id=l.journal_id AND NOT j.voided
		JOIN adrient.acct_accounts a ON a.id=l.account_id AND a.system_key='undeposited_funds'
		WHERE l.org_id=$1 AND l.debit>l.credit AND NOT EXISTS (SELECT 1 FROM adrient.acct_deposit_items di WHERE di.line_id=l.id)
		ORDER BY j.entry_date, j.number`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Undeposited{}
	for rows.Next() {
		var u Undeposited
		if err := rows.Scan(&u.LineID, &u.Date, &u.Memo, &u.ContactName, &u.Amount, &u.JournalID); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Deposit is a bank deposit of several receipts.
type Deposit struct {
	ID        string  `json:"id"`
	Date      string  `json:"date"`
	AccountID string  `json:"account_id"`
	Total     float64 `json:"total"`
	Reference string  `json:"reference"`
	Items     int     `json:"items"`
}

// CreateDeposit moves the chosen Undeposited Funds receipts into a bank account:
// Dr Bank, Cr Undeposited Funds. A receipt can be deposited only once.
func (r *Repository) CreateDeposit(ctx context.Context, orgID, userID, date, accountID, reference string, lineIDs []string) (Deposit, error) {
	d, err := validDate(date)
	if err != nil {
		return Deposit{}, err
	}
	if len(lineIDs) == 0 {
		return Deposit{}, invalid("choose at least one receipt")
	}
	if !isUUID(accountID) {
		return Deposit{}, invalid("choose the bank account")
	}
	avail, err := r.ListUndeposited(ctx, orgID)
	if err != nil {
		return Deposit{}, err
	}
	byLine := map[string]Undeposited{}
	for _, u := range avail {
		byLine[u.LineID] = u
	}
	var total int64
	seen := map[string]bool{}
	for _, id := range lineIDs {
		u, ok := byLine[id]
		if !ok || seen[id] {
			return Deposit{}, invalid("a receipt is not in Undeposited Funds or was already deposited")
		}
		seen[id] = true
		total += paise(u.Amount)
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	var depID string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_deposits (org_id, deposit_date, account_id, total, reference) VALUES ($1,$2::date,$3::uuid,$4,$5) RETURNING id::text`,
		orgID, d, accountID, rupees(total), reference).Scan(&depID); err != nil {
		return Deposit{}, invalid("unknown account")
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.acct_deposit_items (deposit_id, line_id) SELECT $1::uuid, unnest($2::uuid[])`, depID, lineIDs); err != nil {
		_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.acct_deposits WHERE id=$1::uuid`, depID)
		return Deposit{}, err
	}
	memo := "Deposit " + reference
	if _, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: d, Memo: strings.TrimSpace(memo), Ref: reference, Kind: "transfer", Source: SourceDeposit, SourceID: depID,
		SourceHash: hashOf(depID, total), Lines: []Line{{AccountID: accountID, Debit: rupees(total)}, {AccountID: keys["undeposited_funds"], Credit: rupees(total)}}}); err != nil {
		_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.acct_deposits WHERE id=$1::uuid`, depID)
		return Deposit{}, err
	}
	r.audit(ctx, orgID, userID, "create", "deposit", depID, fmt.Sprintf("%.2f in %d receipts", rupees(total), len(lineIDs)))
	return Deposit{ID: depID, Date: d, AccountID: accountID, Total: rupees(total), Reference: reference, Items: len(lineIDs)}, nil
}

func (r *Repository) ListDeposits(ctx context.Context, orgID string) ([]Deposit, error) {
	rows, err := r.pool.Query(ctx, `SELECT d.id::text, to_char(d.deposit_date,'YYYY-MM-DD'), d.account_id::text, d.total::float8, d.reference,
		(SELECT count(*) FROM adrient.acct_deposit_items i WHERE i.deposit_id=d.id) FROM adrient.acct_deposits d WHERE d.org_id=$1 ORDER BY d.deposit_date DESC, d.created_at DESC LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deposit{}
	for rows.Next() {
		var x Deposit
		if err := rows.Scan(&x.ID, &x.Date, &x.AccountID, &x.Total, &x.Reference, &x.Items); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// DeleteDeposit voids a deposit and frees its receipts.
func (r *Repository) DeleteDeposit(ctx context.Context, orgID, userID, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	if _, err := r.voidBySource(ctx, orgID, userID, SourceDeposit, id); err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_deposits WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.audit(ctx, orgID, userID, "delete", "deposit", id, "")
	return nil
}
