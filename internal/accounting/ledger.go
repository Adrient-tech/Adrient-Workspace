// Package accounting is the double-entry ledger behind the Accounting module:
// chart of accounts, journals, auto-posting from POS and Invoicing, bank
// statements, reconciliation, expenses and reports. Money is integer paise inside
// the engine and NUMERIC(16,2) in Postgres.
package accounting

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/audit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
	ErrLocked   = errors.New("locked")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Repository owns the accounting tables.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository constructs the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// paise converts a rupee amount to integer paise.
func paise(f float64) int64 { return int64(math.Round(f * 100)) }

// rupees converts paise back for JSON and NUMERIC columns.
func rupees(p int64) float64 { return float64(p) / 100 }

func validDate(s string) (string, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return "", invalid("date must be YYYY-MM-DD")
	}
	return t.Format("2006-01-02"), nil
}

// ---- settings ----

// Settings is the per-org accounting configuration.
type Settings struct {
	BusinessName string `json:"business_name"`
	GSTIN        string `json:"gstin"`
	Currency     string `json:"currency"`
	FYStartMonth int    `json:"fy_start_month"`
	Intrastate   bool   `json:"intrastate"`
	ClosingDate  string `json:"closing_date"`
	PosSyncedAt  string `json:"pos_synced_at"`
	// UseUndeposited posts non-cash receipts to Undeposited Funds until a deposit moves them to a bank account.
	UseUndeposited bool `json:"use_undeposited"`
	// ApprovalLimit: bills above this amount (base currency) need approval. 0 = no approval step.
	ApprovalLimit float64 `json:"approval_limit"`
	FXAutoFetch   bool    `json:"fx_auto_fetch"`
}

func (r *Repository) GetSettings(ctx context.Context, orgID string) (Settings, error) {
	if err := r.EnsureSeeded(ctx, orgID); err != nil {
		return Settings{}, err
	}
	var s Settings
	err := r.pool.QueryRow(ctx, `SELECT business_name, gstin, currency, fy_start_month, intrastate,
		coalesce(to_char(closing_date,'YYYY-MM-DD'),''), coalesce(to_char(pos_synced_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		use_undeposited, approval_limit::float8, fx_auto_fetch
		FROM adrient.acct_settings WHERE org_id=$1`, orgID).
		Scan(&s.BusinessName, &s.GSTIN, &s.Currency, &s.FYStartMonth, &s.Intrastate, &s.ClosingDate, &s.PosSyncedAt, &s.UseUndeposited, &s.ApprovalLimit, &s.FXAutoFetch)
	return s, err
}

func (r *Repository) SaveSettings(ctx context.Context, orgID, userID string, s Settings) (Settings, error) {
	if err := r.EnsureSeeded(ctx, orgID); err != nil {
		return Settings{}, err
	}
	if s.FYStartMonth < 1 || s.FYStartMonth > 12 {
		return Settings{}, invalid("financial year start must be a month 1-12")
	}
	if s.ClosingDate != "" {
		if _, err := validDate(s.ClosingDate); err != nil {
			return Settings{}, err
		}
	}
	cur := strings.ToUpper(strings.TrimSpace(s.Currency))
	if cur == "" {
		cur = "INR"
	}
	beforeSettings, _ := r.GetSettings(ctx, orgID)
	_, err := r.pool.Exec(ctx, `UPDATE adrient.acct_settings SET business_name=$2, gstin=$3, currency=$4, fy_start_month=$5,
		intrastate=$6, closing_date=NULLIF($7,'')::date, use_undeposited=$8, approval_limit=$9, fx_auto_fetch=$10, updated_at=now() WHERE org_id=$1`,
		orgID, strings.TrimSpace(s.BusinessName), strings.TrimSpace(s.GSTIN), cur, s.FYStartMonth, s.Intrastate, s.ClosingDate, s.UseUndeposited, math.Max(s.ApprovalLimit, 0), s.FXAutoFetch)
	if err != nil {
		return Settings{}, err
	}
	afterSettings, gerr := r.GetSettings(ctx, orgID)
	r.auditChange(ctx, orgID, userID, "update", "settings", "", "closing date "+s.ClosingDate, beforeSettings, afterSettings)
	return afterSettings, gerr
}

// ---- audit ----

func (r *Repository) audit(ctx context.Context, orgID, userID, action, entity, id, detail string) {
	// Enrich the audit_events row written by the HTTP wrapper (one row per call,
	// visible in the Admin audit viewer) with this ledger-level entity/detail.
	audit.Annotate(ctx, entity, id, action+" "+entity+" "+detail)
	_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.acct_audit (org_id, user_id, action, entity, entity_id, detail) VALUES ($1,$2,$3,$4,$5,$6)`,
		orgID, userID, action, entity, id, detail)
}

// auditChange records an audit entry with the record's state before and after
// (JSON snapshots; either may be nil for create/delete).
func (r *Repository) auditChange(ctx context.Context, orgID, userID, action, entity, id, detail string, before, after any) {
	audit.Annotate(ctx, entity, id, action+" "+entity+" "+detail)
	enc := func(v any) any {
		if v == nil {
			return nil
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		return string(b)
	}
	_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.acct_audit (org_id, user_id, action, entity, entity_id, detail, before, after) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb)`,
		orgID, userID, action, entity, id, detail, enc(before), enc(after))
}

// AuditRow is one audit-log entry.
type AuditRow struct {
	ID     int64           `json:"id"`
	UserID string          `json:"user_id"`
	Action string          `json:"action"`
	Entity string          `json:"entity"`
	Ref    string          `json:"entity_id"`
	Detail string          `json:"detail"`
	At     string          `json:"at"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
}

func (r *Repository) ListAudit(ctx context.Context, orgID string, limit int) ([]AuditRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `SELECT id, user_id, action, entity, entity_id, detail, to_char(at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'), coalesce(before::text,''), coalesce(after::text,'')
		FROM adrient.acct_audit WHERE org_id=$1 ORDER BY id DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditRow{}
	for rows.Next() {
		var a AuditRow
		var bf, af string
		if err := rows.Scan(&a.ID, &a.UserID, &a.Action, &a.Entity, &a.Ref, &a.Detail, &a.At, &bf, &af); err != nil {
			return nil, err
		}
		if bf != "" {
			a.Before = json.RawMessage(bf)
		}
		if af != "" {
			a.After = json.RawMessage(af)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- accounts ----

// Account is one chart-of-accounts row.
type Account struct {
	ID        string  `json:"id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype"`
	SystemKey string  `json:"system_key"`
	ParentID  string  `json:"parent_id"`
	Active    bool    `json:"active"`
	Balance   float64 `json:"balance"` // natural-side balance (debit-normal for assets/expenses)
}

var accountTypes = map[string]bool{"asset": true, "liability": true, "equity": true, "income": true, "expense": true}

func debitNormal(t string) bool { return t == "asset" || t == "expense" }

func (r *Repository) ListAccounts(ctx context.Context, orgID string) ([]Account, error) {
	if err := r.EnsureSeeded(ctx, orgID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT a.id::text, a.code, a.name, a.type, a.subtype, a.system_key, coalesce(a.parent_id::text,''), a.active,
		coalesce(sum(l.debit - l.credit),0)::float8
		FROM adrient.acct_accounts a
		LEFT JOIN (SELECT l.account_id, l.debit, l.credit FROM adrient.acct_lines l JOIN adrient.acct_journals j ON j.id=l.journal_id AND NOT j.voided WHERE l.org_id=$1) l
		  ON l.account_id=a.id
		WHERE a.org_id=$1 GROUP BY a.id ORDER BY a.code, a.name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		var a Account
		var net float64
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.Subtype, &a.SystemKey, &a.ParentID, &a.Active, &net); err != nil {
			return nil, err
		}
		if !debitNormal(a.Type) {
			net = -net
		}
		a.Balance = rupees(paise(net))
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) SaveAccount(ctx context.Context, orgID, userID, id string, a Account) (Account, error) {
	a.Name = strings.TrimSpace(a.Name)
	a.Code = strings.TrimSpace(a.Code)
	if a.Name == "" {
		return Account{}, invalid("account name is required")
	}
	if !accountTypes[a.Type] {
		return Account{}, invalid("account type must be asset, liability, equity, income or expense")
	}
	if err := r.EnsureSeeded(ctx, orgID); err != nil {
		return Account{}, err
	}
	var out Account
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_accounts (org_id, code, name, type, subtype, parent_id, active)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7) RETURNING id::text`, orgID, a.Code, a.Name, a.Type, a.Subtype, a.ParentID, true).Scan(&out.ID)
	} else {
		// The type of an account that already has postings cannot change.
		var used bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.acct_lines WHERE org_id=$1 AND account_id=$2::uuid)`, orgID, id).Scan(&used)
		if used {
			var cur string
			if e := r.pool.QueryRow(ctx, `SELECT type FROM adrient.acct_accounts WHERE org_id=$1 AND id=$2::uuid`, orgID, id).Scan(&cur); e == nil && cur != a.Type {
				return Account{}, invalid("the type of an account with postings cannot change")
			}
		}
		tag, e := r.pool.Exec(ctx, `UPDATE adrient.acct_accounts SET code=$3, name=$4, type=$5, subtype=$6, parent_id=NULLIF($7,'')::uuid, active=$8
			WHERE org_id=$1 AND id=$2::uuid`, orgID, id, a.Code, a.Name, a.Type, a.Subtype, a.ParentID, a.Active)
		if e == nil && tag.RowsAffected() == 0 {
			return Account{}, ErrNotFound
		}
		err, out.ID = e, id
	}
	if err != nil {
		return Account{}, err
	}
	r.audit(ctx, orgID, userID, "save", "account", out.ID, a.Code+" "+a.Name)
	all, err := r.ListAccounts(ctx, orgID)
	if err != nil {
		return Account{}, err
	}
	for _, x := range all {
		if x.ID == out.ID {
			return x, nil
		}
	}
	return Account{}, ErrNotFound
}

func (r *Repository) DeleteAccount(ctx context.Context, orgID, userID, id string) error {
	var key string
	var used bool
	if err := r.pool.QueryRow(ctx, `SELECT a.system_key, EXISTS(SELECT 1 FROM adrient.acct_lines l WHERE l.account_id=a.id)
		FROM adrient.acct_accounts a WHERE a.org_id=$1 AND a.id=$2::uuid`, orgID, id).Scan(&key, &used); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if key != "" {
		return invalid("system accounts cannot be deleted; mark them inactive instead")
	}
	if used {
		return invalid("this account has postings; mark it inactive instead")
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_accounts WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err == nil {
		r.audit(ctx, orgID, userID, "delete", "account", id, "")
	}
	return err
}

// accountKeys maps system_key to account id.
func (r *Repository) accountKeys(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, orgID string) (map[string]string, error) {
	rows, err := q.Query(ctx, `SELECT system_key, id::text FROM adrient.acct_accounts WHERE org_id=$1 AND system_key<>''`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, id string
		if err := rows.Scan(&k, &id); err != nil {
			return nil, err
		}
		m[k] = id
	}
	return m, rows.Err()
}

// ---- journals ----

// Line is one debit or credit of a journal.
type Line struct {
	ID          string  `json:"id"`
	AccountID   string  `json:"account_id"`
	AccountName string  `json:"account_name"`
	Debit       float64 `json:"debit"`
	Credit      float64 `json:"credit"`
	Memo        string  `json:"memo"`
	Class       string  `json:"class"`     // cost center / class / tag
	FcyDebit    float64 `json:"fcy_debit"` // document-currency amounts (multi-currency journals)
	FcyCredit   float64 `json:"fcy_credit"`
}

// Journal is a balanced set of lines on one date.
type Journal struct {
	ID          string       `json:"id"`
	Number      int64        `json:"number"`
	Date        string       `json:"date"`
	DueDate     string       `json:"due_date"`
	Memo        string       `json:"memo"`
	Ref         string       `json:"ref"`
	Kind        string       `json:"kind"`
	Source      string       `json:"source"`
	SourceID    string       `json:"source_id"`
	ContactID   string       `json:"contact_id"`
	ContactName string       `json:"contact_name"`
	Voided      bool         `json:"voided"`
	Currency    string       `json:"currency"` // '' = base currency
	Rate        float64      `json:"rate"`     // base per 1 foreign unit
	Attachments []Attachment `json:"attachments"`
	Total       float64      `json:"total"`
	Lines       []Line       `json:"lines"`
}

// JournalInput creates or replaces a journal. SourceHash lets auto-posting skip
// unchanged sources.
type JournalInput struct {
	Date        string  `json:"date"`
	DueDate     string  `json:"due_date"`
	Memo        string  `json:"memo"`
	Ref         string  `json:"ref"`
	Kind        string  `json:"kind"`
	Source      string  `json:"-"`
	SourceID    string  `json:"-"`
	SourceHash  string  `json:"-"`
	ContactID   string  `json:"contact_id"`
	ContactName string  `json:"contact_name"`
	Currency    string  `json:"currency"`
	Rate        float64 `json:"rate"`
	Lines       []Line  `json:"lines"`
}

// checkBalanced validates lines and returns the total in paise.
func checkBalanced(lines []Line) (int64, error) {
	var d, c int64
	n := 0
	for _, l := range lines {
		dp, cp := paise(l.Debit), paise(l.Credit)
		if dp < 0 || cp < 0 {
			return 0, invalid("amounts cannot be negative")
		}
		if dp > 0 && cp > 0 {
			return 0, invalid("a line is either a debit or a credit, not both")
		}
		if dp == 0 && cp == 0 {
			continue
		}
		if l.AccountID == "" {
			return 0, invalid("every line needs an account")
		}
		d += dp
		c += cp
		n++
	}
	if n < 2 {
		return 0, invalid("a journal needs at least two lines")
	}
	if d != c {
		return 0, invalid("debits (%.2f) and credits (%.2f) must be equal", rupees(d), rupees(c))
	}
	if d == 0 {
		return 0, invalid("a journal cannot be zero")
	}
	return d, nil
}

func (r *Repository) closingDate(ctx context.Context, q pgx.Tx, orgID string) string {
	var s string
	_ = q.QueryRow(ctx, `SELECT coalesce(to_char(closing_date,'YYYY-MM-DD'),'') FROM adrient.acct_settings WHERE org_id=$1`, orgID).Scan(&s)
	return s
}

func lockedErr(date, closing string) error {
	return fmt.Errorf("%w: books are closed through %s (entry dated %s)", ErrLocked, closing, date)
}

// PostJournal creates a journal. When Source and SourceID are set it is an
// upsert keyed by (source, source_id): an unchanged hash is a no-op, a changed
// one replaces the lines. created reports whether a new journal was inserted.
func (r *Repository) PostJournal(ctx context.Context, orgID, userID string, in JournalInput) (Journal, bool, error) {
	date, err := validDate(in.Date)
	if err != nil {
		return Journal{}, false, err
	}
	if in.DueDate != "" {
		if in.DueDate, err = validDate(in.DueDate); err != nil {
			return Journal{}, false, err
		}
	}
	if _, err := checkBalanced(in.Lines); err != nil {
		return Journal{}, false, err
	}
	if in.Source == "" {
		in.Source = "manual"
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	rate := in.Rate
	if in.Currency == "" || rate <= 0 {
		rate = 1
	}
	if in.Kind == "" {
		in.Kind = "journal"
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Journal{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "acct:"+orgID); err != nil {
		return Journal{}, false, err
	}
	closing := r.closingDate(ctx, tx, orgID)
	ids := []string{}
	for _, l := range in.Lines {
		ids = append(ids, l.AccountID)
	}
	var okCount int
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT id) FROM adrient.acct_accounts WHERE org_id=$1 AND id = ANY($2::uuid[])`, orgID, ids).Scan(&okCount); err != nil {
		return Journal{}, false, invalid("unknown account")
	}
	if want := distinct(ids); okCount != want {
		return Journal{}, false, invalid("a line uses an account that does not exist")
	}

	var jid, oldDate, oldHash string
	var oldVoided bool
	created := false
	if in.SourceID != "" {
		err = tx.QueryRow(ctx, `SELECT id::text, to_char(entry_date,'YYYY-MM-DD'), source_hash, voided FROM adrient.acct_journals
			WHERE org_id=$1 AND source=$2 AND source_id=$3 FOR UPDATE`, orgID, in.Source, in.SourceID).Scan(&jid, &oldDate, &oldHash, &oldVoided)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Journal{}, false, err
		}
	}
	if jid != "" && !oldVoided && oldHash == in.SourceHash && in.SourceHash != "" {
		_ = tx.Commit(ctx)
		j, e := r.GetJournal(ctx, orgID, jid)
		return j, false, e
	}
	if closing != "" && date <= closing {
		return Journal{}, false, lockedErr(date, closing)
	}
	var beforeJ *Journal
	if jid != "" {
		if bj, e := r.GetJournal(ctx, orgID, jid); e == nil {
			beforeJ = &bj
		}
		if closing != "" && oldDate <= closing {
			return Journal{}, false, lockedErr(oldDate, closing)
		}
		if err := guardReconciled(ctx, tx, jid); err != nil {
			return Journal{}, false, err
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.acct_journals SET entry_date=$3::date, due_date=NULLIF($4,'')::date, memo=$5, ref=$6, kind=$7,
			contact_id=$8, contact_name=$9, source_hash=$10, voided=false, currency=$11, fx_rate=$12, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`,
			orgID, jid, date, in.DueDate, in.Memo, in.Ref, in.Kind, in.ContactID, in.ContactName, in.SourceHash, in.Currency, rate); err != nil {
			return Journal{}, false, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM adrient.acct_lines WHERE journal_id=$1::uuid`, jid); err != nil {
			return Journal{}, false, err
		}
	} else {
		var num int64
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(number),0)+1 FROM adrient.acct_journals WHERE org_id=$1`, orgID).Scan(&num); err != nil {
			return Journal{}, false, err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO adrient.acct_journals (org_id, number, entry_date, due_date, memo, ref, kind, source, source_id, source_hash,
			contact_id, contact_name, created_by, currency, fx_rate) VALUES ($1,$2,$3::date,NULLIF($4,'')::date,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,'')::uuid,$14,$15) RETURNING id::text`,
			orgID, num, date, in.DueDate, in.Memo, in.Ref, in.Kind, in.Source, in.SourceID, in.SourceHash, in.ContactID, in.ContactName, userID, in.Currency, rate).Scan(&jid); err != nil {
			return Journal{}, false, err
		}
		created = true
	}
	pos := 0
	for _, l := range in.Lines {
		if paise(l.Debit) == 0 && paise(l.Credit) == 0 {
			continue
		}
		if err := insertLine(ctx, tx, jid, orgID, l, pos); err != nil {
			return Journal{}, false, err
		}
		pos++
	}
	if err := tx.Commit(ctx); err != nil {
		return Journal{}, false, err
	}
	act := "create"
	if !created {
		act = "update"
	}
	j, err := r.GetJournal(ctx, orgID, jid)
	if beforeJ != nil {
		r.auditChange(ctx, orgID, userID, act, "journal", jid, in.Source+" "+in.Memo, beforeJ, j)
	} else {
		r.auditChange(ctx, orgID, userID, act, "journal", jid, in.Source+" "+in.Memo, nil, j)
	}
	return j, created, err
}

func insertLine(ctx context.Context, tx pgx.Tx, jid, orgID string, l Line, pos int) error {
	_, err := tx.Exec(ctx, `INSERT INTO adrient.acct_lines (journal_id, org_id, account_id, debit, credit, memo, position, class, fcy_debit, fcy_credit)
		VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8,$9,$10)`, jid, orgID, l.AccountID, rupees(paise(l.Debit)), rupees(paise(l.Credit)), l.Memo, pos,
		strings.TrimSpace(l.Class), rupees(paise(l.FcyDebit)), rupees(paise(l.FcyCredit)))
	return err
}

func distinct(ids []string) int {
	m := map[string]bool{}
	for _, i := range ids {
		m[i] = true
	}
	return len(m)
}

const journalCols = `j.id::text, j.number, to_char(j.entry_date,'YYYY-MM-DD'), coalesce(to_char(j.due_date,'YYYY-MM-DD'),''), j.memo, j.ref, j.kind,
	j.source, j.source_id, j.contact_id, j.contact_name, j.voided, j.currency, j.fx_rate::float8`

func (r *Repository) loadLines(ctx context.Context, orgID string, js []Journal) error {
	if len(js) == 0 {
		return nil
	}
	idx := map[string]int{}
	ids := make([]string, len(js))
	for i := range js {
		idx[js[i].ID] = i
		ids[i] = js[i].ID
		js[i].Lines = []Line{}
	}
	rows, err := r.pool.Query(ctx, `SELECT l.journal_id::text, l.id::text, l.account_id::text, a.code||' '||a.name, l.debit::float8, l.credit::float8, l.memo, l.class, l.fcy_debit::float8, l.fcy_credit::float8
		FROM adrient.acct_lines l JOIN adrient.acct_accounts a ON a.id=l.account_id
		WHERE l.org_id=$1 AND l.journal_id = ANY($2::uuid[]) ORDER BY l.journal_id, l.position`, orgID, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var jid string
		var l Line
		if err := rows.Scan(&jid, &l.ID, &l.AccountID, &l.AccountName, &l.Debit, &l.Credit, &l.Memo, &l.Class, &l.FcyDebit, &l.FcyCredit); err != nil {
			return err
		}
		i := idx[jid]
		js[i].Lines = append(js[i].Lines, l)
		js[i].Total = rupees(paise(js[i].Total) + paise(l.Debit))
	}
	return rows.Err()
}

func scanJournal(row pgx.Row) (Journal, error) {
	var j Journal
	err := row.Scan(&j.ID, &j.Number, &j.Date, &j.DueDate, &j.Memo, &j.Ref, &j.Kind, &j.Source, &j.SourceID, &j.ContactID, &j.ContactName, &j.Voided, &j.Currency, &j.Rate)
	return j, err
}

func (r *Repository) GetJournal(ctx context.Context, orgID, id string) (Journal, error) {
	if !isUUID(id) {
		return Journal{}, ErrNotFound
	}
	j, err := scanJournal(r.pool.QueryRow(ctx, `SELECT `+journalCols+` FROM adrient.acct_journals j WHERE j.org_id=$1 AND j.id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Journal{}, ErrNotFound
	}
	if err != nil {
		return Journal{}, err
	}
	js := []Journal{j}
	if err := r.loadLines(ctx, orgID, js); err != nil {
		return Journal{}, err
	}
	js[0].Attachments, _ = r.ListJournalFiles(ctx, orgID, id)
	return js[0], nil
}

// JournalFilter narrows ListJournals.
type JournalFilter struct {
	From, To, Query, Source, ContactID, AccountID string
	Limit                                         int
	// Keyset cursor: only journals after (AfterDate, AfterNumber, AfterID) in
	// the newest-first order. All three set together (see journal_page.go).
	AfterDate   string
	AfterNumber int64
	AfterID     string
}

func (r *Repository) ListJournals(ctx context.Context, orgID string, f JournalFilter) ([]Journal, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	where := []string{"j.org_id=$1"}
	args := []any{orgID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.Replace(cond, "?", fmt.Sprintf("$%d", len(args)), -1))
	}
	if f.From != "" {
		add("j.entry_date >= ?::date", f.From)
	}
	if f.To != "" {
		add("j.entry_date <= ?::date", f.To)
	}
	if f.Source != "" {
		add("j.source = ?", f.Source)
	}
	if f.ContactID != "" {
		add("j.contact_id = ?", f.ContactID)
	}
	if isUUID(f.AccountID) {
		add("EXISTS (SELECT 1 FROM adrient.acct_lines l WHERE l.journal_id=j.id AND l.account_id=?::uuid)", f.AccountID)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add(`(j.memo ILIKE ? ESCAPE '\' OR j.ref ILIKE ? ESCAPE '\' OR j.contact_name ILIKE ? ESCAPE '\' OR j.number::text = ?)`, "%"+escapeLike(q)+"%")
		// The ? replacement above only bound the first placeholder; bind the rest explicitly.
		n := len(args)
		where[len(where)-1] = fmt.Sprintf(`(j.memo ILIKE $%d ESCAPE '\' OR j.ref ILIKE $%d ESCAPE '\' OR j.contact_name ILIKE $%d ESCAPE '\' OR j.number::text = $%d)`, n, n, n, n+1)
		args = append(args, strings.TrimPrefix(q, "#"))
	}
	if f.AfterDate != "" && isUUID(f.AfterID) {
		args = append(args, f.AfterDate, f.AfterNumber, f.AfterID)
		where = append(where, fmt.Sprintf("(j.entry_date, j.number, j.id) < ($%d::date, $%d, $%d::uuid)", len(args)-2, len(args)-1, len(args)))
	}
	args = append(args, f.Limit)
	rows, err := r.pool.Query(ctx, `SELECT `+journalCols+` FROM adrient.acct_journals j WHERE `+strings.Join(where, " AND ")+
		fmt.Sprintf(` ORDER BY j.entry_date DESC, j.number DESC, j.id DESC LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	out := []Journal{}
	for rows.Next() {
		j, err := scanJournal(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, r.loadLines(ctx, orgID, out)
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}

// UpdateManual replaces a manual journal.
func (r *Repository) UpdateManual(ctx context.Context, orgID, userID, id string, in JournalInput) (Journal, error) {
	cur, err := r.GetJournal(ctx, orgID, id)
	if err != nil {
		return Journal{}, err
	}
	if cur.Source != "manual" {
		return Journal{}, invalid("only manual journals can be edited; this one is managed by %s", cur.Source)
	}
	in.Source, in.SourceID, in.SourceHash = "manual", "", ""
	if _, err := checkBalanced(in.Lines); err != nil {
		return Journal{}, err
	}
	date, err := validDate(in.Date)
	if err != nil {
		return Journal{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Journal{}, err
	}
	defer tx.Rollback(ctx)
	closing := r.closingDate(ctx, tx, orgID)
	if closing != "" && (date <= closing || cur.Date <= closing) {
		return Journal{}, lockedErr(date, closing)
	}
	if err := guardReconciled(ctx, tx, id); err != nil {
		return Journal{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.acct_journals SET entry_date=$3::date, due_date=NULLIF($4,'')::date, memo=$5, ref=$6, kind=$7,
		contact_id=$8, contact_name=$9, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`,
		orgID, id, date, in.DueDate, in.Memo, in.Ref, orDefault(in.Kind, "journal"), in.ContactID, in.ContactName); err != nil {
		return Journal{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.acct_lines WHERE journal_id=$1::uuid`, id); err != nil {
		return Journal{}, err
	}
	pos := 0
	for _, l := range in.Lines {
		if paise(l.Debit) == 0 && paise(l.Credit) == 0 {
			continue
		}
		if err := insertLine(ctx, tx, id, orgID, l, pos); err != nil {
			return Journal{}, invalid("a line uses an account that does not exist")
		}
		pos++
	}
	if err := tx.Commit(ctx); err != nil {
		return Journal{}, err
	}
	after, aerr := r.GetJournal(ctx, orgID, id)
	r.auditChange(ctx, orgID, userID, "update", "journal", id, in.Memo, cur, after)
	return after, aerr
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// DeleteJournal deletes a manual journal (auto-posted ones are managed by their
// source).
func (r *Repository) DeleteJournal(ctx context.Context, orgID, userID, id string) error {
	cur, err := r.GetJournal(ctx, orgID, id)
	if err != nil {
		return err
	}
	if cur.Source != "manual" {
		return invalid("only manual journals can be deleted; this one is managed by %s", cur.Source)
	}
	return r.deleteJournal(ctx, orgID, userID, cur)
}

func (r *Repository) deleteJournal(ctx context.Context, orgID, userID string, cur Journal) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	closing := r.closingDate(ctx, tx, orgID)
	if closing != "" && cur.Date <= closing {
		return lockedErr(cur.Date, closing)
	}
	if err := guardReconciled(ctx, tx, cur.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.acct_journals WHERE org_id=$1 AND id=$2::uuid`, orgID, cur.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	r.auditChange(ctx, orgID, userID, "delete", "journal", cur.ID, cur.Source+" "+cur.Memo, cur, nil)
	return nil
}

// deleteBySource removes the journal for a source row, if any.
func (r *Repository) deleteBySource(ctx context.Context, orgID, userID, source, sourceID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.acct_journals WHERE org_id=$1 AND source=$2 AND source_id=$3`, orgID, source, sourceID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	j, err := r.GetJournal(ctx, orgID, id)
	if err != nil {
		return err
	}
	return r.deleteJournal(ctx, orgID, userID, j)
}

// voidBySource marks a source's journal voided (kept for audit, excluded from reports).
func (r *Repository) voidBySource(ctx context.Context, orgID, userID, source, sourceID string) (bool, error) {
	var id, date string
	var voided bool
	err := r.pool.QueryRow(ctx, `SELECT id::text, to_char(entry_date,'YYYY-MM-DD'), voided FROM adrient.acct_journals WHERE org_id=$1 AND source=$2 AND source_id=$3`,
		orgID, source, sourceID).Scan(&id, &date, &voided)
	if errors.Is(err, pgx.ErrNoRows) || voided {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var closing string
	_ = r.pool.QueryRow(ctx, `SELECT coalesce(to_char(closing_date,'YYYY-MM-DD'),'') FROM adrient.acct_settings WHERE org_id=$1`, orgID).Scan(&closing)
	if closing != "" && date <= closing {
		return false, lockedErr(date, closing)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_journals SET voided=true, updated_at=now() WHERE id=$1::uuid`, id); err != nil {
		return false, err
	}
	if before, e := r.GetJournal(ctx, orgID, id); e == nil {
		after := before
		after.Voided = true
		before.Voided = false
		r.auditChange(ctx, orgID, userID, "void", "journal", id, source+" "+sourceID, before, after)
	} else {
		r.audit(ctx, orgID, userID, "void", "journal", id, source+" "+sourceID)
	}
	return true, nil
}

func hashOf(parts ...any) string {
	h := sha1.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%v|", p)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// guardReconciled refuses to change a journal that has a reconciled line.
func guardReconciled(ctx context.Context, tx pgx.Tx, journalID string) error {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.acct_lines WHERE journal_id=$1::uuid AND reconciliation_id IS NOT NULL`, journalID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return invalid("this entry has been reconciled; undo the reconciliation before changing it")
	}
	return nil
}
