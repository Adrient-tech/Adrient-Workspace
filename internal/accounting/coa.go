package accounting

import "context"

type seedAccount struct{ code, name, typ, subtype, key string }

// DefaultChart is the India-first starter chart of accounts (small business,
// GST registered). Accounts with a key are used by auto-posting.
var DefaultChart = []seedAccount{
	{"1000", "Cash in Hand", "asset", "cash", "cash"},
	{"1010", "Bank Account", "asset", "bank", "bank"},
	{"1100", "Accounts Receivable (Debtors)", "asset", "receivable", "ar"},
	{"1200", "Inventory / Stock in Hand", "asset", "other", "inventory"},
	{"1300", "GST Input Credit - CGST", "asset", "gst_input", "gst_in_cgst"},
	{"1310", "GST Input Credit - SGST", "asset", "gst_input", "gst_in_sgst"},
	{"1320", "GST Input Credit - IGST", "asset", "gst_input", "gst_in_igst"},
	{"1400", "TDS Receivable", "asset", "other", "tds_receivable"},
	{"1500", "Fixed Assets", "asset", "fixed_asset", "fixed_assets"},
	{"1900", "Prepaid Expenses & Advances", "asset", "other", ""},
	{"2000", "Accounts Payable (Creditors)", "liability", "payable", "ap"},
	{"2100", "GST Output - CGST", "liability", "gst_output", "gst_out_cgst"},
	{"2110", "GST Output - SGST", "liability", "gst_output", "gst_out_sgst"},
	{"2120", "GST Output - IGST", "liability", "gst_output", "gst_out_igst"},
	{"2200", "Employee Reimbursements Payable", "liability", "other", "emp_payable"},
	{"2300", "Store Credit & Loyalty Liability", "liability", "other", "store_credit"},
	{"2400", "TDS Payable", "liability", "other", ""},
	{"2500", "Loans", "liability", "loan", ""},
	{"3000", "Owner's Capital", "equity", "capital", "capital"},
	{"3100", "Owner's Drawings", "equity", "drawings", ""},
	{"3900", "Opening Balance Equity", "equity", "opening", "opening"},
	{"4000", "Sales", "income", "sales", "sales"},
	{"4010", "Sales Returns", "income", "sales_returns", "sales_returns"},
	{"4100", "Service Income", "income", "sales", ""},
	{"4900", "Other Income", "income", "other", ""},
	{"5000", "Cost of Goods Sold", "expense", "cogs", "cogs"},
	{"6000", "Rent", "expense", "opex", ""},
	{"6010", "Salaries & Wages", "expense", "opex", ""},
	{"6020", "Electricity & Utilities", "expense", "opex", ""},
	{"6030", "Telephone & Internet", "expense", "opex", ""},
	{"6040", "Office Supplies", "expense", "opex", ""},
	{"6050", "Travel & Conveyance", "expense", "opex", ""},
	{"6060", "Professional Fees", "expense", "opex", ""},
	{"6070", "Advertising & Marketing", "expense", "opex", ""},
	{"6080", "Repairs & Maintenance", "expense", "opex", ""},
	{"6090", "Bank Charges", "expense", "opex", ""},
	{"6100", "Insurance", "expense", "opex", ""},
	{"6110", "Bad Debts", "expense", "opex", "bad_debt"},
	{"6120", "Round Off", "expense", "opex", "round_off"},
	{"6900", "Miscellaneous Expenses", "expense", "opex", "misc"},
}

// EnsureSeeded creates the settings row and the default chart the first time
// an org opens Accounting. Safe to call repeatedly and concurrently.
func (r *Repository) EnsureSeeded(ctx context.Context, orgID string) error {
	var have bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.acct_settings WHERE org_id=$1)`, orgID).Scan(&have); err != nil {
		return err
	}
	if have {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "acct-seed:"+orgID); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.acct_settings WHERE org_id=$1)`, orgID).Scan(&have); err != nil {
		return err
	}
	if have {
		return nil
	}
	for _, a := range DefaultChart {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.acct_accounts (org_id, code, name, type, subtype, system_key)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, orgID, a.code, a.name, a.typ, a.subtype, a.key); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.acct_settings (org_id) VALUES ($1) ON CONFLICT DO NOTHING`, orgID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
