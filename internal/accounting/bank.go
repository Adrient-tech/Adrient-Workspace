package accounting

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- CSV statement parsing (pure) ----

// StatementLine is one parsed bank statement row; Amount is paise, + money in.
type StatementLine struct {
	Date        string `json:"date"`
	Description string `json:"description"`
	Amount      int64  `json:"amount"`
}

var dateLayouts = []string{"2006-01-02", "02/01/2006", "02-01-2006", "2/1/2006", "02-Jan-2006", "2-Jan-2006", "02 Jan 2006", "2 Jan 2006",
	"02/01/06", "02-01-06", "02-Jan-06", "Jan 2, 2006", "02.01.2006"}

// ParseDate accepts the date formats Indian bank statements use (day first).
func ParseDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// ParseAmount reads "1,23,456.78", "(500)", "-500", "500 Dr", "500 Cr" into
// signed paise (Dr = negative). ok is false for blanks.
func ParseAmount(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	neg := false
	up := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(up, "DR"):
		neg, s = true, strings.TrimSpace(s[:len(s)-2])
	case strings.HasSuffix(up, "CR"):
		s = strings.TrimSpace(s[:len(s)-2])
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, s[1:len(s)-1]
	}
	s = strings.NewReplacer(",", "", "₹", "", "INR", "", "Rs.", "", "Rs", "", " ", "").Replace(s)
	if strings.HasPrefix(s, "-") {
		neg, s = !neg, s[1:]
	}
	s = strings.TrimPrefix(s, "+")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	p := int64(math.Round(f * 100))
	if neg {
		p = -p
	}
	return p, true
}

// ParseStatementCSV maps the header row to date / description / debit / credit
// (or a single signed amount) and returns the lines, newest order preserved.
func ParseStatementCSV(data string) ([]StatementLine, error) {
	rd := csv.NewReader(strings.NewReader(strings.TrimPrefix(data, "\ufeff")))
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, invalid("could not read CSV: %v", err)
	}
	hdr := -1
	var di, ni, wi, ci, ai = -1, -1, -1, -1, -1
	for i, row := range rows {
		di, ni, wi, ci, ai = -1, -1, -1, -1, -1
		for j, h := range row {
			h = strings.ToLower(strings.TrimSpace(h))
			switch {
			case di < 0 && (h == "date" || strings.Contains(h, "txn date") || strings.Contains(h, "transaction date") || strings.Contains(h, "value date") || h == "posting date"):
				di = j
			case ni < 0 && (strings.Contains(h, "narration") || strings.Contains(h, "description") || strings.Contains(h, "particulars") || strings.Contains(h, "details") || strings.Contains(h, "remarks")):
				ni = j
			case wi < 0 && (strings.Contains(h, "withdraw") || strings.Contains(h, "debit") || h == "dr"):
				wi = j
			case ci < 0 && (strings.Contains(h, "deposit") || strings.Contains(h, "credit") || h == "cr"):
				ci = j
			case ai < 0 && (h == "amount" || strings.HasPrefix(h, "amount")):
				ai = j
			}
		}
		if di >= 0 && ni >= 0 && (ai >= 0 || wi >= 0 || ci >= 0) {
			hdr = i
			break
		}
	}
	if hdr < 0 {
		return nil, invalid("CSV needs a header row with Date, Description (or Narration) and Debit/Credit or Amount columns")
	}
	out := []StatementLine{}
	for _, row := range rows[hdr+1:] {
		get := func(i int) string {
			if i >= 0 && i < len(row) {
				return row[i]
			}
			return ""
		}
		d, ok := ParseDate(get(di))
		if !ok {
			continue
		}
		var amt int64
		if wi >= 0 || ci >= 0 {
			w, _ := ParseAmount(get(wi))
			c, _ := ParseAmount(get(ci))
			amt = c - abs64(w)
			if ai >= 0 && w == 0 && c == 0 {
				amt, _ = ParseAmount(get(ai))
			}
		} else {
			amt, _ = ParseAmount(get(ai))
		}
		if amt == 0 {
			continue
		}
		out = append(out, StatementLine{Date: d, Description: strings.TrimSpace(get(ni)), Amount: amt})
	}
	if len(out) == 0 {
		return nil, invalid("no transactions found in the CSV")
	}
	return out, nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// ---- bank rules ----

// Rule categorizes statement lines whose description contains Contains.
type Rule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AccountID  string `json:"account_id"`
	Direction  string `json:"direction"` // in | out | any
	Contains   string `json:"contains"`
	CategoryID string `json:"category_id"`
	Priority   int    `json:"priority"`
	AutoAdd    bool   `json:"auto_add"`
}

// MatchRule returns the first rule (lowest priority number first) that applies.
func MatchRule(rules []Rule, bankAccountID, desc string, amount int64) *Rule {
	d := strings.ToLower(desc)
	var best *Rule
	for i := range rules {
		r := &rules[i]
		if r.AccountID != "" && r.AccountID != bankAccountID {
			continue
		}
		if r.Direction == "in" && amount <= 0 || r.Direction == "out" && amount >= 0 {
			continue
		}
		if c := strings.ToLower(strings.TrimSpace(r.Contains)); c == "" || !strings.Contains(d, c) {
			continue
		}
		if best == nil || r.Priority < best.Priority {
			best = r
		}
	}
	return best
}

func (r *Repository) ListRules(ctx context.Context, orgID string) ([]Rule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, coalesce(account_id::text,''), direction, contains, category_id::text, priority, auto_add
		FROM adrient.acct_bank_rules WHERE org_id=$1 ORDER BY priority, created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var x Rule
		if err := rows.Scan(&x.ID, &x.Name, &x.AccountID, &x.Direction, &x.Contains, &x.CategoryID, &x.Priority, &x.AutoAdd); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) SaveRule(ctx context.Context, orgID, userID, id string, x Rule) (Rule, error) {
	x.Contains = strings.TrimSpace(x.Contains)
	if x.Contains == "" {
		return Rule{}, invalid("a rule needs text to match")
	}
	if !isUUID(x.CategoryID) {
		return Rule{}, invalid("choose the account to assign")
	}
	switch x.Direction {
	case "in", "out", "any":
	default:
		x.Direction = "any"
	}
	if x.Priority == 0 {
		x.Priority = 100
	}
	if x.Name == "" {
		x.Name = x.Contains
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_bank_rules (org_id, name, account_id, direction, contains, category_id, priority, auto_add)
			VALUES ($1,$2,NULLIF($3,'')::uuid,$4,$5,$6::uuid,$7,$8) RETURNING id::text`,
			orgID, x.Name, x.AccountID, x.Direction, x.Contains, x.CategoryID, x.Priority, x.AutoAdd).Scan(&x.ID)
	} else {
		var tag int64
		t, e := r.pool.Exec(ctx, `UPDATE adrient.acct_bank_rules SET name=$3, account_id=NULLIF($4,'')::uuid, direction=$5, contains=$6,
			category_id=$7::uuid, priority=$8, auto_add=$9 WHERE org_id=$1 AND id=$2::uuid`,
			orgID, id, x.Name, x.AccountID, x.Direction, x.Contains, x.CategoryID, x.Priority, x.AutoAdd)
		err, tag, x.ID = e, 0, id
		if e == nil {
			tag = t.RowsAffected()
			if tag == 0 {
				return Rule{}, ErrNotFound
			}
		}
	}
	if err != nil {
		return Rule{}, err
	}
	r.audit(ctx, orgID, userID, "save", "bank_rule", x.ID, x.Contains)
	return x, nil
}

func (r *Repository) DeleteRule(ctx context.Context, orgID, userID, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_bank_rules WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err == nil {
		r.audit(ctx, orgID, userID, "delete", "bank_rule", id, "")
	}
	return err
}

// ---- bank lines ----

type BankLine struct {
	ID          string  `json:"id"`
	AccountID   string  `json:"account_id"`
	Date        string  `json:"date"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Status      string  `json:"status"`
	CategoryID  string  `json:"category_id"`
	JournalID   string  `json:"journal_id"`
}

type ImportResult struct {
	Imported    int `json:"imported"`
	Duplicates  int `json:"duplicates"`
	Categorized int `json:"categorized"`
	Added       int `json:"added"`
}

func (r *Repository) bankAccount(ctx context.Context, orgID, id string) (Account, error) {
	var a Account
	err := r.pool.QueryRow(ctx, `SELECT id::text, name, type, subtype FROM adrient.acct_accounts WHERE org_id=$1 AND id=$2::uuid`, orgID, id).
		Scan(&a.ID, &a.Name, &a.Type, &a.Subtype)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, invalid("choose a bank account")
	}
	if err == nil && a.Subtype != "bank" && a.Subtype != "cash" {
		return a, invalid("%s is not a bank or cash account", a.Name)
	}
	return a, err
}

// ImportCSV stores statement lines (skipping ones imported before), then applies
// the bank rules to the new lines. Identical rows inside one file stay distinct
// through an occurrence counter, so re-importing the same file adds nothing.
func (r *Repository) ImportCSV(ctx context.Context, orgID, userID, accountID, csvText string) (ImportResult, error) {
	var res ImportResult
	if _, err := r.bankAccount(ctx, orgID, accountID); err != nil {
		return res, err
	}
	lines, err := ParseStatementCSV(csvText)
	if err != nil {
		return res, err
	}
	return r.importLines(ctx, orgID, userID, accountID, lines, "csv")
}

// importLines stores parsed statement lines, skipping ones already imported.
func (r *Repository) importLines(ctx context.Context, orgID, userID, accountID string, lines []StatementLine, format string) (ImportResult, error) {
	var res ImportResult
	seen := map[string]int{}
	for _, l := range lines {
		base := fmt.Sprintf("%s|%s|%d", l.Date, strings.ToLower(l.Description), l.Amount)
		seen[base]++
		tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.acct_bank_lines (org_id, account_id, line_date, description, amount, hash)
			VALUES ($1,$2::uuid,$3::date,$4,$5,$6) ON CONFLICT (org_id, account_id, hash) DO NOTHING`,
			orgID, accountID, l.Date, l.Description, rupees(l.Amount), hashOf(base, seen[base]))
		if err != nil {
			return res, err
		}
		if tag.RowsAffected() == 0 {
			res.Duplicates++
		} else {
			res.Imported++
		}
	}
	cat, added, err := r.ApplyRules(ctx, orgID, userID)
	res.Categorized, res.Added = cat, added
	r.audit(ctx, orgID, userID, "import", "bank", accountID, fmt.Sprintf("%s: %d lines, %d duplicates", format, res.Imported, res.Duplicates))
	return res, err
}

// ApplyRules categorizes pending lines that have no category yet and posts the
// ones whose rule says auto-add.
func (r *Repository) ApplyRules(ctx context.Context, orgID, userID string) (categorized, added int, err error) {
	rules, err := r.ListRules(ctx, orgID)
	if err != nil || len(rules) == 0 {
		return 0, 0, err
	}
	lines, err := r.ListBankLines(ctx, orgID, "", "pending")
	if err != nil {
		return 0, 0, err
	}
	for _, l := range lines {
		if l.CategoryID != "" {
			continue
		}
		m := MatchRule(rules, l.AccountID, l.Description, paise(l.Amount))
		if m == nil {
			continue
		}
		if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_bank_lines SET category_id=$3::uuid WHERE org_id=$1 AND id=$2::uuid`, orgID, l.ID, m.CategoryID); err != nil {
			return categorized, added, err
		}
		categorized++
		if m.AutoAdd {
			if _, err := r.AddBankLine(ctx, orgID, userID, l.ID, m.CategoryID, ""); err == nil {
				added++
			}
		}
	}
	return categorized, added, nil
}

func (r *Repository) ListBankLines(ctx context.Context, orgID, accountID, status string) ([]BankLine, error) {
	return r.ListBankLinesPage(ctx, orgID, accountID, status, 2000, 0)
}

// ListBankLinesPage is ListBankLines with a page window (newest first).
func (r *Repository) ListBankLinesPage(ctx context.Context, orgID, accountID, status string, limit, offset int) ([]BankLine, error) {
	if limit <= 0 || limit > 2000 {
		limit = 2000
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, account_id::text, to_char(line_date,'YYYY-MM-DD'), description, amount::float8, status,
		coalesce(category_id::text,''), coalesce(journal_id::text,'') FROM adrient.acct_bank_lines
		WHERE org_id=$1 AND ($2='' OR account_id=$2::uuid) AND ($3='' OR status=$3) ORDER BY line_date DESC, created_at DESC, id DESC LIMIT $4 OFFSET $5`,
		orgID, accountID, status, limit, max(offset, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BankLine{}
	for rows.Next() {
		var l BankLine
		if err := rows.Scan(&l.ID, &l.AccountID, &l.Date, &l.Description, &l.Amount, &l.Status, &l.CategoryID, &l.JournalID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) getBankLine(ctx context.Context, orgID, id string) (BankLine, error) {
	if !isUUID(id) {
		return BankLine{}, ErrNotFound
	}
	var l BankLine
	err := r.pool.QueryRow(ctx, `SELECT id::text, account_id::text, to_char(line_date,'YYYY-MM-DD'), description, amount::float8, status,
		coalesce(category_id::text,''), coalesce(journal_id::text,'') FROM adrient.acct_bank_lines WHERE org_id=$1 AND id=$2::uuid`, orgID, id).
		Scan(&l.ID, &l.AccountID, &l.Date, &l.Description, &l.Amount, &l.Status, &l.CategoryID, &l.JournalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// AddBankLine posts a statement line against a category account.
func (r *Repository) AddBankLine(ctx context.Context, orgID, userID, id, categoryID, memo string) (BankLine, error) {
	l, err := r.getBankLine(ctx, orgID, id)
	if err != nil {
		return l, err
	}
	if l.Status == "added" {
		return l, invalid("this line is already in the books")
	}
	if !isUUID(categoryID) {
		return l, invalid("choose an account for this line")
	}
	if categoryID == l.AccountID {
		return l, invalid("the category cannot be the bank account itself")
	}
	amt := paise(l.Amount)
	m := orDefault(memo, l.Description)
	ln := []Line{{AccountID: l.AccountID, Memo: m}, {AccountID: categoryID, Memo: m}}
	if amt > 0 {
		ln[0].Debit, ln[1].Credit = rupees(amt), rupees(amt)
	} else {
		ln[0].Credit, ln[1].Debit = rupees(-amt), rupees(-amt)
	}
	j, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: l.Date, Memo: m, Kind: "bank", Source: "bank_line", SourceID: l.ID, SourceHash: hashOf(amt, categoryID, l.Date), Lines: ln})
	if err != nil {
		return l, err
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.acct_bank_lines SET status='added', category_id=$3::uuid, journal_id=$4::uuid WHERE org_id=$1 AND id=$2::uuid`,
		orgID, id, categoryID, j.ID)
	if err != nil {
		return l, err
	}
	return r.getBankLine(ctx, orgID, id)
}

func (r *Repository) ExcludeBankLine(ctx context.Context, orgID, userID, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.acct_bank_lines SET status='excluded' WHERE org_id=$1 AND id=$2::uuid AND status='pending'`, orgID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return invalid("only pending lines can be excluded")
	}
	return err
}

// UndoBankLine removes the journal an added line created, or restores an excluded line.
func (r *Repository) UndoBankLine(ctx context.Context, orgID, userID, id string) error {
	l, err := r.getBankLine(ctx, orgID, id)
	if err != nil {
		return err
	}
	if l.Status == "added" {
		if err := r.deleteBySource(ctx, orgID, userID, "bank_line", l.ID); err != nil {
			return err
		}
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.acct_bank_lines SET status='pending', journal_id=NULL WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	return err
}

// ---- reconciliation ----

type ReconLine struct {
	LineID    string  `json:"line_id"`
	JournalID string  `json:"journal_id"`
	Date      string  `json:"date"`
	Memo      string  `json:"memo"`
	Amount    float64 `json:"amount"` // + deposit / - payment
}

type ReconDraft struct {
	AccountID      string      `json:"account_id"`
	StatementDate  string      `json:"statement_date"`
	OpeningBalance float64     `json:"opening_balance"` // already-reconciled balance
	Lines          []ReconLine `json:"lines"`
}

func (r *Repository) ReconcileDraft(ctx context.Context, orgID, accountID, statementDate string) (ReconDraft, error) {
	d := ReconDraft{AccountID: accountID, StatementDate: statementDate, Lines: []ReconLine{}}
	if _, err := r.bankAccount(ctx, orgID, accountID); err != nil {
		return d, err
	}
	sd, err := validDate(statementDate)
	if err != nil {
		return d, err
	}
	var open int64
	if err := r.pool.QueryRow(ctx, `SELECT coalesce(sum(round((l.debit-l.credit)*100)),0)::bigint FROM adrient.acct_lines l
		JOIN adrient.acct_journals j ON j.id=l.journal_id AND NOT j.voided WHERE l.org_id=$1 AND l.account_id=$2::uuid AND l.reconciliation_id IS NOT NULL`,
		orgID, accountID).Scan(&open); err != nil {
		return d, err
	}
	d.OpeningBalance = rupees(open)
	rows, err := r.pool.Query(ctx, `SELECT l.id::text, j.id::text, to_char(j.entry_date,'YYYY-MM-DD'), coalesce(nullif(l.memo,''), j.memo), (l.debit-l.credit)::float8
		FROM adrient.acct_lines l JOIN adrient.acct_journals j ON j.id=l.journal_id AND NOT j.voided
		WHERE l.org_id=$1 AND l.account_id=$2::uuid AND l.reconciliation_id IS NULL AND j.entry_date <= $3::date ORDER BY j.entry_date, j.number`, orgID, accountID, sd)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var x ReconLine
		if err := rows.Scan(&x.LineID, &x.JournalID, &x.Date, &x.Memo, &x.Amount); err != nil {
			return d, err
		}
		d.Lines = append(d.Lines, x)
	}
	return d, rows.Err()
}

type Reconciliation struct {
	ID            string  `json:"id"`
	AccountID     string  `json:"account_id"`
	StatementDate string  `json:"statement_date"`
	EndingBalance float64 `json:"ending_balance"`
	Cleared       int     `json:"cleared"`
	CreatedAt     string  `json:"created_at"`
}

// FinishReconciliation marks the ticked lines reconciled when
// opening + cleared == the statement's ending balance.
func (r *Repository) FinishReconciliation(ctx context.Context, orgID, userID, accountID, statementDate string, ending float64, lineIDs []string) (Reconciliation, error) {
	d, err := r.ReconcileDraft(ctx, orgID, accountID, statementDate)
	if err != nil {
		return Reconciliation{}, err
	}
	avail := map[string]int64{}
	for _, l := range d.Lines {
		avail[l.LineID] = paise(l.Amount)
	}
	sum := paise(d.OpeningBalance)
	for _, id := range lineIDs {
		v, ok := avail[id]
		if !ok {
			return Reconciliation{}, invalid("a ticked line is not available to reconcile")
		}
		sum += v
	}
	if diff := paise(ending) - sum; diff != 0 {
		return Reconciliation{}, invalid("difference of %.2f: the ticked lines do not reach the statement balance", rupees(diff))
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Reconciliation{}, err
	}
	defer tx.Rollback(ctx)
	var rec Reconciliation
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.acct_reconciliations (org_id, account_id, statement_date, ending_balance, created_by)
		VALUES ($1,$2::uuid,$3::date,$4,NULLIF($5,'')::uuid) RETURNING id::text`, orgID, accountID, statementDate, rupees(paise(ending)), userID).Scan(&rec.ID); err != nil {
		return rec, err
	}
	if len(lineIDs) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE adrient.acct_lines SET reconciliation_id=$2::uuid WHERE org_id=$1 AND id = ANY($3::uuid[])`, orgID, rec.ID, lineIDs); err != nil {
			return rec, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return rec, err
	}
	r.audit(ctx, orgID, userID, "reconcile", "account", accountID, fmt.Sprintf("%d lines to %s", len(lineIDs), statementDate))
	rec.AccountID, rec.StatementDate, rec.EndingBalance, rec.Cleared = accountID, statementDate, rupees(paise(ending)), len(lineIDs)
	return rec, nil
}

func (r *Repository) ReconciliationHistory(ctx context.Context, orgID, accountID string) ([]Reconciliation, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.id::text, c.account_id::text, to_char(c.statement_date,'YYYY-MM-DD'), c.ending_balance::float8,
		(SELECT count(*) FROM adrient.acct_lines l WHERE l.reconciliation_id=c.id), to_char(c.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM adrient.acct_reconciliations c WHERE c.org_id=$1 AND ($2='' OR c.account_id=$2::uuid) ORDER BY c.created_at DESC LIMIT 100`, orgID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reconciliation{}
	for rows.Next() {
		var x Reconciliation
		if err := rows.Scan(&x.ID, &x.AccountID, &x.StatementDate, &x.EndingBalance, &x.Cleared, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// UndoReconciliation releases the lines of the most recent reconciliation for its account.
func (r *Repository) UndoReconciliation(ctx context.Context, orgID, userID, id string) error {
	var acct string
	var latest bool
	err := r.pool.QueryRow(ctx, `SELECT c.account_id::text, c.id = (SELECT id FROM adrient.acct_reconciliations WHERE org_id=$1 AND account_id=c.account_id ORDER BY created_at DESC LIMIT 1)
		FROM adrient.acct_reconciliations c WHERE c.org_id=$1 AND c.id=$2::uuid`, orgID, id).Scan(&acct, &latest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !latest {
		return invalid("only the latest reconciliation of an account can be undone")
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_lines SET reconciliation_id=NULL WHERE org_id=$1 AND reconciliation_id=$2::uuid`, orgID, id); err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.acct_reconciliations WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err == nil {
		r.audit(ctx, orgID, userID, "undo", "reconciliation", id, "")
	}
	return err
}
