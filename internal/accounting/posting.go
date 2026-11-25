package accounting

import (
	"context"
	"fmt"
	"strings"
)

// Public posting API for other modules (HR payroll, Inventory/Purchasing, Sales/POS).
//
//	j, created, err := repo.PostSourceJournal(ctx, orgID, userID, accounting.SourcePost{
//	    Source: accounting.SourcePayroll, SourceID: runID, Date: "2026-11-30", Memo: "Payroll Nov",
//	    Lines: []accounting.PostLine{
//	        {Account: "salaries_expense", Debit: 100000},
//	        {Account: "payroll_payable", Credit: 100000},
//	    }})
//
// Idempotent: the same (org, Source, SourceID) with the same content is a no-op,
// changed content rewrites that journal in place. VoidSourceJournal voids it.
// Accounts are given as a system key (see SystemAccounts), an account code or an account id.

// Source types accepted by PostSourceJournal.
const (
	SourcePayroll         = "payroll"
	SourcePurchaseBill    = "purchase_bill"
	SourceStockCOGS       = "stock_cogs"
	SourceStockAdjustment = "stock_adjustment"
	SourceBill            = "bill"
	SourceBillPayment     = "bill_payment"
	SourceVendorCredit    = "vendor_credit"
	SourceCreditMemo      = "credit_memo"
	SourceDeposit         = "deposit"
	SourceDepreciation    = "depreciation"
	SourceRecurring       = "recurring"
	SourceRevaluation     = "revaluation"
	SourcePosSale         = "pos_sale"
	SourceInvoice         = "invoice"
	SourceWriteOff        = "write_off"
	SourceTDS             = "tds"
	SourceAssetPurchase   = "asset_purchase"
	SourceAssetDisposal   = "asset_disposal"
)

// postableSources are the sources external modules may post. Built-in sync sources
// (pos_*, invoice*, expense*, bank_line) keep their own paths.
var postableSources = map[string]bool{
	SourcePayroll: true, SourcePurchaseBill: true, SourceStockCOGS: true, SourceStockAdjustment: true,
	SourceBill: true, SourceBillPayment: true, SourceVendorCredit: true, SourceCreditMemo: true, SourceDeposit: true,
	SourceDepreciation: true, SourceRecurring: true, SourceRevaluation: true, SourceWriteOff: true, SourceTDS: true, SourceAssetPurchase: true, SourceAssetDisposal: true, "manual_import": true,
}

// PostLine is one line of a sourced journal. Account is a system key, code or id.
type PostLine struct {
	Account string  `json:"account"`
	Debit   float64 `json:"debit"`
	Credit  float64 `json:"credit"`
	Memo    string  `json:"memo"`
	Class   string  `json:"class"` // optional cost center / class / tag
}

// SourcePost describes a journal owned by a source document in another module.
type SourcePost struct {
	Source      string
	SourceID    string
	Date        string // YYYY-MM-DD
	DueDate     string
	Memo        string
	Ref         string
	ContactID   string
	ContactName string
	Currency    string  // empty = base currency
	Rate        float64 // base units per 1 foreign unit; 0 = look up
	Lines       []PostLine
}

type extraAccount struct{ code, name, typ, subtype, key string }

// SystemAccounts are accounts created on demand (idempotently) for modules that post.
var SystemAccounts = []extraAccount{
	{"1050", "Undeposited Funds", "asset", "cash", "undeposited_funds"},
	{"1510", "Accumulated Depreciation", "asset", "fixed_asset", "accum_depreciation"},
	{"2210", "Payroll Payable", "liability", "other", "payroll_payable"},
	{"2220", "Payroll Deductions Payable (PF/ESI/PT)", "liability", "other", "payroll_deductions"},
	{"2400", "TDS Payable", "liability", "other", "tds_payable"},
	{"4950", "Foreign Exchange Gain/Loss", "income", "other", "fx_gain_loss"},
	{"4960", "Unrealised Exchange Gain/Loss", "income", "other", "fx_unrealised"},
	{"5010", "Inventory Adjustments", "expense", "cogs", "stock_adjustment"},
	{"5020", "Purchases", "expense", "cogs", "purchases"},
	{"4970", "Gain/Loss on Asset Disposal", "income", "other", "asset_disposal"},
	{"6010", "Salaries & Wages", "expense", "opex", "salaries_expense"},
	{"6130", "Depreciation Expense", "expense", "opex", "depreciation_expense"},
	{"6140", "Write-offs", "expense", "opex", "write_off"},
	{"6910", "Employer Contributions (PF/ESI)", "expense", "opex", "employer_contrib"},
}

// EnsureSystemAccounts makes sure every SystemAccounts key exists for the org.
// An existing account with the same code and no key is adopted; code clashes with
// another key get the next free code.
func (r *Repository) EnsureSystemAccounts(ctx context.Context, orgID string) error {
	if err := r.EnsureSeeded(ctx, orgID); err != nil {
		return err
	}
	for _, a := range SystemAccounts {
		var have bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.acct_accounts WHERE org_id=$1 AND system_key=$2)`, orgID, a.key).Scan(&have); err != nil {
			return err
		}
		if have {
			continue
		}
		tag, err := r.pool.Exec(ctx, `UPDATE adrient.acct_accounts SET system_key=$3 WHERE org_id=$1 AND code=$2 AND system_key='' AND type=$4`, orgID, a.code, a.key, a.typ)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			continue
		}
		code := a.code
		for i := 0; i < 50; i++ {
			var taken bool
			_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.acct_accounts WHERE org_id=$1 AND code=$2)`, orgID, code).Scan(&taken)
			if !taken {
				break
			}
			code = fmt.Sprintf("%s-%d", a.code, i+1)
		}
		if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.acct_accounts (org_id, code, name, type, subtype, system_key) VALUES ($1,$2,$3,$4,$5,$6)`,
			orgID, code, a.name, a.typ, a.subtype, a.key); err != nil {
			return err
		}
	}
	return nil
}

// resolveAccount maps a key, code or uuid to an account id.
func resolveAccount(keys, codes map[string]string, ids map[string]bool, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if id, ok := keys[ref]; ok {
		return id, true
	}
	if id, ok := codes[ref]; ok {
		return id, true
	}
	if ids[ref] {
		return ref, true
	}
	return "", false
}

func (r *Repository) accountIndex(ctx context.Context, orgID string) (keys, codes map[string]string, ids map[string]bool, err error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, code, system_key FROM adrient.acct_accounts WHERE org_id=$1`, orgID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	keys, codes, ids = map[string]string{}, map[string]string{}, map[string]bool{}
	for rows.Next() {
		var id, code, key string
		if err := rows.Scan(&id, &code, &key); err != nil {
			return nil, nil, nil, err
		}
		ids[id] = true
		codes[code] = id
		if key != "" {
			keys[key] = id
		}
	}
	return keys, codes, ids, rows.Err()
}

// PostSourceJournal is the public, idempotent posting API. It validates the source,
// resolves account references, balances the lines (in base currency) and posts.
func (r *Repository) PostSourceJournal(ctx context.Context, orgID, userID string, p SourcePost) (Journal, bool, error) {
	if !postableSources[p.Source] {
		return Journal{}, false, invalid("unknown journal source %q", p.Source)
	}
	if strings.TrimSpace(p.SourceID) == "" {
		return Journal{}, false, invalid("source_id is required so posting can be idempotent")
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return Journal{}, false, err
	}
	keys, codes, ids, err := r.accountIndex(ctx, orgID)
	if err != nil {
		return Journal{}, false, err
	}
	in := JournalInput{Date: p.Date, DueDate: p.DueDate, Memo: p.Memo, Ref: p.Ref, Kind: kindFor(p.Source), Source: p.Source, SourceID: p.SourceID,
		ContactID: p.ContactID, ContactName: p.ContactName, Currency: p.Currency, Rate: p.Rate}
	hash := []any{p.Source, p.Date, p.DueDate, p.Memo, p.ContactID, p.Currency, p.Rate}
	for _, l := range p.Lines {
		id, ok := resolveAccount(keys, codes, ids, l.Account)
		if !ok {
			return Journal{}, false, invalid("unknown account %q", l.Account)
		}
		in.Lines = append(in.Lines, Line{AccountID: id, Debit: l.Debit, Credit: l.Credit, Memo: l.Memo, Class: l.Class})
		hash = append(hash, id, paise(l.Debit), paise(l.Credit), l.Class)
	}
	if err := r.applyCurrency(ctx, orgID, &in); err != nil {
		return Journal{}, false, err
	}
	hash = append(hash, in.Rate)
	in.SourceHash = hashOf(hash...)
	return r.PostJournal(ctx, orgID, userID, in)
}

// applyCurrency converts in.Lines from the document currency to base when in.Currency
// is foreign. A zero in.Rate is looked up from the rate table for the journal date.
func (r *Repository) applyCurrency(ctx context.Context, orgID string, in *JournalInput) error {
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return err
	}
	if in.Currency == "" || in.Currency == st.Currency {
		in.Currency, in.Rate = "", 1
		return nil
	}
	if in.Rate <= 0 {
		if in.Rate, err = r.RateOn(ctx, orgID, in.Currency, in.Date); err != nil {
			return err
		}
	}
	in.Lines = toBase(in.Lines, in.Rate)
	return nil
}

// VoidSourceJournal voids the journal owned by (source, sourceID). It reports
// whether something was voided; a missing or already-void journal is not an error.
func (r *Repository) VoidSourceJournal(ctx context.Context, orgID, userID, source, sourceID string) (bool, error) {
	return r.voidBySource(ctx, orgID, userID, source, sourceID)
}

func kindFor(source string) string {
	switch source {
	case SourcePayroll:
		return "payroll"
	case SourceBill, SourcePurchaseBill, SourceVendorCredit:
		return "bill"
	case SourceBillPayment:
		return "payment"
	case SourceCreditMemo:
		return "invoice"
	}
	return "journal"
}
