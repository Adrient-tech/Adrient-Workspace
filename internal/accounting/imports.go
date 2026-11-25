package accounting

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ContactSink lets the import wizard create contacts in the Contacts module. The server
// sets it; when nil, contacts are only saved as GST parties. created=false means a contact
// with that name already existed.
var ContactSink func(ctx context.Context, orgID, userID string, c ImportContact) (created bool, err error)

// ImportContact is one row of a contacts import.
type ImportContact struct {
	Name, Company, Email, Phone, GSTIN, StateCode, PAN, Kind string
}

// ImportRequest drives the migration wizard.
type ImportRequest struct {
	Type   string `json:"type"`   // coa | opening_balances | contacts | invoices | bills
	Source string `json:"source"` // zoho | quickbooks | tally | generic
	CSV    string `json:"csv"`
	Mode   string `json:"mode"`  // invoices/bills: open_items (default) | history
	AsOf   string `json:"as_of"` // opening balances date
	DryRun bool   `json:"dry_run"`
}

// ImportRowResult is the outcome for one CSV row.
type ImportRowResult struct {
	Line    int    `json:"line"`
	Status  string `json:"status"` // ok | existing | error
	Message string `json:"message"`
	Summary string `json:"summary"`
}

// ImportReport is the dry-run preview or the result of a run.
type ImportReport struct {
	Type     string            `json:"type"`
	Source   string            `json:"source"`
	DryRun   bool              `json:"dry_run"`
	Total    int               `json:"total"`
	OK       int               `json:"ok"`
	Existing int               `json:"existing"`
	Errors   int               `json:"errors"`
	Applied  int               `json:"applied"`
	Rows     []ImportRowResult `json:"rows"`
	Mapping  map[string]string `json:"mapping"` // field -> CSV column used
	Notes    []string          `json:"notes"`
}

// ImportColumnMaps documents the CSV columns recognised per field (also shown in the wizard).
var ImportColumnMaps = map[string]map[string][]string{
	"coa": {
		"name":    {"account name", "account", "name", "particulars", "ledger name"},
		"code":    {"account code", "code", "number", "account number", "no."},
		"type":    {"account type", "type", "group", "under", "parent"},
		"subtype": {"detail type", "sub type", "subtype"},
	},
	"opening_balances": {
		"account": {"account", "account name", "name", "particulars", "ledger name"},
		"code":    {"account code", "code", "number"},
		"debit":   {"debit", "debit amount", "dr", "debit (inr)"},
		"credit":  {"credit", "credit amount", "cr", "credit (inr)"},
		"balance": {"balance", "closing balance", "amount", "opening balance"},
	},
	"contacts": {
		"name":    {"contact name", "display name", "customer name", "vendor name", "customer", "vendor", "supplier", "name", "particulars", "party name"},
		"company": {"company name", "company"},
		"email":   {"email", "emailid", "email id", "e-mail"},
		"phone":   {"phone", "mobilephone", "mobile phone", "mobile", "phone no", "contact number"},
		"gstin":   {"gst identification number (gstin)", "gstin", "gstin/uin", "gst no", "gst number", "tax id"},
		"state":   {"place of contact", "place of supply", "state", "state code"},
		"pan":     {"pan", "pan no", "pan number", "it pan"},
		"kind":    {"contact type", "type", "group"},
	},
	"invoices": {
		"number":   {"invoice number", "invoice no", "invoice#", "num", "number", "voucher no.", "vch no.", "voucher no", "invoice"},
		"date":     {"invoice date", "date", "txn date", "transaction date"},
		"due":      {"due date", "due"},
		"customer": {"customer name", "customer", "name", "particulars", "party name"},
		"total":    {"total", "amount", "total amount", "invoice total", "gross total", "debit amount", "debit", "value", "open balance (total)"},
		"balance":  {"balance", "open balance", "balance due", "outstanding", "pending"},
		"tax":      {"tax total", "total tax", "item tax", "tax amount", "gst", "tax"},
		"currency": {"currency code", "currency"},
		"status":   {"invoice status", "status"},
	},
	"bills": {
		"number":   {"bill number", "bill no", "bill#", "num", "number", "ref no.", "voucher no.", "vch no.", "bill"},
		"date":     {"bill date", "date", "txn date", "transaction date"},
		"due":      {"due date", "due"},
		"customer": {"vendor name", "vendor", "name", "particulars", "party name", "supplier"},
		"total":    {"total", "amount", "total amount", "bill total", "gross total", "credit amount", "credit", "value"},
		"balance":  {"balance", "open balance", "balance due", "outstanding", "pending"},
		"tax":      {"tax total", "total tax", "item tax", "tax amount", "gst", "tax"},
		"currency": {"currency code", "currency"},
		"status":   {"bill status", "status"},
		"account":  {"account", "expense account", "category", "account name"},
	},
}

func normHeader(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(s, "\ufeff"), "_", " "), "  ", " ")))
}

type csvTable struct {
	header  []string
	rows    [][]string
	mapping map[string]int
}

func readCSVTable(text string, fields map[string][]string) (*csvTable, error) {
	rd := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	var recs [][]string
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalid("could not read the CSV: %v", err)
		}
		blank := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				blank = false
			}
		}
		if !blank {
			recs = append(recs, rec)
		}
		if len(recs) > 20001 {
			return nil, invalid("too many rows: import at most 20,000 rows at a time")
		}
	}
	if len(recs) < 2 {
		return nil, invalid("the CSV needs a header row and at least one data row")
	}
	t := &csvTable{header: recs[0], rows: recs[1:], mapping: map[string]int{}}
	// The first matching synonym (in priority order) among the headers wins.
	for field, syns := range fields {
		for _, syn := range syns {
			found := -1
			for i, h := range t.header {
				if normHeader(h) == syn {
					found = i
					break
				}
			}
			if found >= 0 {
				t.mapping[field] = found
				break
			}
		}
	}
	return t, nil
}

func (t *csvTable) get(row []string, field string) string {
	if i, ok := t.mapping[field]; ok && i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

func (t *csvTable) mappingNames() map[string]string {
	out := map[string]string{}
	for f, i := range t.mapping {
		out[f] = t.header[i]
	}
	return out
}

// classifyAccountType maps Zoho, QuickBooks and Tally type/group names to Adrient types.
func classifyAccountType(s string) (typ, sub string, ok bool) {
	l := strings.ToLower(strings.TrimSpace(s))
	has := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(l, p) {
				return true
			}
		}
		return false
	}
	switch {
	case l == "":
		return "", "", false
	case has("bank account", "bank", "credit card"):
		if has("credit card") {
			return "liability", "other", true
		}
		return "asset", "bank", true
	case has("cash"):
		return "asset", "cash", true
	case has("receivable", "debtor"):
		return "asset", "receivable", true
	case has("fixed asset"):
		return "asset", "fixed_asset", true
	case has("stock", "inventory"):
		return "asset", "other", true
	case has("asset", "deposit", "advance", "prepaid"):
		return "asset", "other", true
	case has("payable", "creditor"):
		return "liability", "payable", true
	case has("loan", "long term", "borrowing", "secured", "unsecured"):
		return "liability", "loan", true
	case has("liabilit", "duties", "tax", "provision"):
		return "liability", "other", true
	case has("equity", "capital", "reserve", "drawing", "retained", "surplus"):
		return "equity", "capital", true
	case has("cost of goods", "cogs", "purchase accounts", "direct expense"):
		return "expense", "cogs", true
	case has("income", "revenue", "sales"):
		return "income", "sales", true
	case has("expense"):
		return "expense", "opex", true
	}
	return "", "", false
}

// findAccountByRef finds an account by name first (the name is what exports agree on), then by code.
func (r *Repository) findAccountByRef(accts []Account, code, name string) (Account, bool) {
	code, name = strings.TrimSpace(code), strings.ToLower(strings.TrimSpace(name))
	if name != "" {
		for _, a := range accts {
			if strings.ToLower(a.Name) == name {
				return a, true
			}
		}
	}
	if code != "" {
		for _, a := range accts {
			if a.Code == code && name == "" {
				return a, true
			}
		}
	}
	return Account{}, false
}

// RunImport parses the CSV for the chosen type and source, validates every row and, unless
// DryRun, applies the valid ones. It is safe to run again: rows that already exist are
// reported as "existing" and nothing is duplicated.
func (r *Repository) RunImport(ctx context.Context, orgID, userID string, req ImportRequest) (ImportReport, error) {
	rep := ImportReport{Type: req.Type, Source: orDefault(req.Source, "generic"), DryRun: req.DryRun, Rows: []ImportRowResult{}, Notes: []string{}}
	switch req.Source {
	case "", "generic", "zoho", "quickbooks", "tally":
	default:
		return rep, invalid("source must be zoho, quickbooks, tally or generic")
	}
	maps, ok := ImportColumnMaps[req.Type]
	if !ok {
		return rep, invalid("type must be coa, opening_balances, contacts, invoices or bills")
	}
	t, err := readCSVTable(req.CSV, maps)
	if err != nil {
		return rep, err
	}
	rep.Mapping = t.mappingNames()
	rep.Total = len(t.rows)
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return rep, err
	}
	switch req.Type {
	case "coa":
		err = r.importCOA(ctx, orgID, userID, t, req, &rep)
	case "opening_balances":
		err = r.importOpening(ctx, orgID, userID, t, req, &rep)
	case "contacts":
		err = r.importContacts(ctx, orgID, userID, t, req, &rep)
	case "invoices":
		err = r.importInvoices(ctx, orgID, userID, t, req, &rep)
	case "bills":
		err = r.importBills(ctx, orgID, userID, t, req, &rep)
	}
	if err != nil {
		return rep, err
	}
	if !req.DryRun {
		r.audit(ctx, orgID, userID, "import", req.Type, req.Source, fmt.Sprintf("%d applied, %d existing, %d errors", rep.Applied, rep.Existing, rep.Errors))
	}
	return rep, nil
}

func (rep *ImportReport) add(line int, status, msg, summary string) {
	switch status {
	case "ok":
		rep.OK++
	case "existing":
		rep.Existing++
	case "error":
		rep.Errors++
	}
	if len(rep.Rows) < 500 || status == "error" {
		rep.Rows = append(rep.Rows, ImportRowResult{Line: line, Status: status, Message: msg, Summary: summary})
	}
}

func (r *Repository) importCOA(ctx context.Context, orgID, userID string, t *csvTable, req ImportRequest, rep *ImportReport) error {
	if _, ok := t.mapping["name"]; !ok {
		return invalid("no account name column found (expected one of: %s)", strings.Join(ImportColumnMaps["coa"]["name"], ", "))
	}
	accts, err := r.ListAccounts(ctx, orgID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, row := range t.rows {
		line := i + 2
		name, code := t.get(row, "name"), t.get(row, "code")
		if name == "" {
			rep.add(line, "error", "account name is empty", "")
			continue
		}
		key := strings.ToLower(code + "|" + name)
		if seen[key] {
			rep.add(line, "existing", "duplicate row in the file", name)
			continue
		}
		seen[key] = true
		typ, sub, ok := classifyAccountType(orDefault(t.get(row, "type"), t.get(row, "subtype")))
		if !ok {
			if _, has := r.findAccountByRef(accts, code, name); has {
				rep.add(line, "existing", "account already exists", name)
				continue
			}
			rep.add(line, "error", fmt.Sprintf("cannot tell the account type from %q", t.get(row, "type")), name)
			continue
		}
		if a, has := r.findAccountByRef(accts, code, name); has {
			rep.add(line, "existing", "account already exists as "+a.Code+" "+a.Name, name)
			continue
		}
		if code != "" {
			clash := false
			for _, a := range accts {
				if a.Code == code {
					rep.add(line, "error", fmt.Sprintf("code %s is already used by %q; change the code or the name", code, a.Name), name)
					clash = true
					break
				}
			}
			if clash {
				continue
			}
		}
		if req.DryRun {
			rep.add(line, "ok", "will create as "+typ, strings.TrimSpace(code+" "+name))
			continue
		}
		a, err := r.SaveAccount(ctx, orgID, userID, "", Account{Code: code, Name: name, Type: typ, Subtype: sub, Active: true})
		if err != nil {
			rep.add(line, "error", err.Error(), name)
			continue
		}
		accts = append(accts, a)
		rep.Applied++
		rep.add(line, "ok", "created as "+typ, strings.TrimSpace(code+" "+name))
	}
	return nil
}

func (r *Repository) importOpening(ctx context.Context, orgID, userID string, t *csvTable, req ImportRequest, rep *ImportReport) error {
	asOf, err := validDate(req.AsOf)
	if err != nil {
		return invalid("choose the opening balance date (as_of)")
	}
	if _, ok := t.mapping["account"]; !ok {
		return invalid("no account column found")
	}
	_, hasDebit := t.mapping["debit"]
	_, hasCredit := t.mapping["credit"]
	_, hasBalance := t.mapping["balance"]
	if !hasDebit && !hasCredit && !hasBalance {
		return invalid("no debit/credit or balance column found")
	}
	accts, err := r.ListAccounts(ctx, orgID)
	if err != nil {
		return err
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	var lines []Line
	var dr, cr int64
	for i, row := range t.rows {
		line := i + 2
		name := t.get(row, "account")
		if name == "" || strings.EqualFold(name, "total") || strings.EqualFold(name, "grand total") {
			continue
		}
		a, ok := r.findAccountByRef(accts, t.get(row, "code"), name)
		if !ok {
			rep.add(line, "error", "account not found; import the chart of accounts first", name)
			continue
		}
		var d, c int64
		if hasDebit || hasCredit {
			d, _ = ParseAmount(t.get(row, "debit"))
			c, _ = ParseAmount(t.get(row, "credit"))
			d, c = abs64(d), abs64(c)
		} else {
			// A signed balance, or one with Dr/Cr; positive = the account's natural side.
			v, okb := ParseAmount(t.get(row, "balance"))
			if !okb {
				rep.add(line, "error", "balance is not a number", name)
				continue
			}
			// ParseAmount returns negative for Dr and positive for Cr (bank convention: + money in).
			// Opening balances use the natural side for plain numbers.
			s := strings.ToLower(t.get(row, "balance"))
			switch {
			case strings.Contains(s, "dr"):
				d = abs64(v)
			case strings.Contains(s, "cr"):
				c = abs64(v)
			case debitNormal(a.Type) && v >= 0, !debitNormal(a.Type) && v < 0:
				d = abs64(v)
			default:
				c = abs64(v)
			}
		}
		if d == 0 && c == 0 {
			rep.add(line, "existing", "zero balance, nothing to post", name)
			continue
		}
		if d > 0 && c > 0 {
			if d >= c {
				d, c = d-c, 0
			} else {
				d, c = 0, c-d
			}
		}
		lines = append(lines, Line{AccountID: a.ID, Debit: rupees(d), Credit: rupees(c), Memo: "Opening balance"})
		dr += d
		cr += c
		rep.add(line, "ok", "", fmt.Sprintf("%s  Dr %.2f  Cr %.2f", name, rupees(d), rupees(c)))
	}
	if len(lines) == 0 {
		return nil
	}
	if diff := dr - cr; diff != 0 {
		l := Line{AccountID: keys["opening"], Memo: "Opening balance difference"}
		if diff > 0 {
			l.Credit = rupees(diff)
		} else {
			l.Debit = rupees(-diff)
		}
		lines = append(lines, l)
		rep.Notes = append(rep.Notes, fmt.Sprintf("Debits %.2f and credits %.2f differ by %.2f; the difference goes to Opening Balance Equity.", rupees(dr), rupees(cr), rupees(abs64(diff))))
	}
	if req.DryRun || rep.Errors > 0 {
		if rep.Errors > 0 && !req.DryRun {
			rep.Notes = append(rep.Notes, "Nothing was posted because some rows have errors. Fix them and run again; opening balances post as one journal.")
		}
		return nil
	}
	_, created, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: asOf, Memo: "Opening balances (" + rep.Source + " import)", Kind: "journal", Source: "manual_import",
		SourceID: "opening:" + asOf, SourceHash: hashOf(lines2hash(lines)...), Lines: lines})
	if err != nil {
		return err
	}
	if created {
		rep.Applied = len(lines)
	} else {
		rep.Applied = 0
		rep.Notes = append(rep.Notes, "Opening balances for this date were already posted; the journal was refreshed in place if anything changed.")
	}
	return nil
}

func lines2hash(ls []Line) []any {
	out := []any{}
	for _, l := range ls {
		out = append(out, l.AccountID, paise(l.Debit), paise(l.Credit))
	}
	return out
}

func (r *Repository) importContacts(ctx context.Context, orgID, userID string, t *csvTable, req ImportRequest, rep *ImportReport) error {
	if _, ok := t.mapping["name"]; !ok {
		return invalid("no contact name column found")
	}
	seen := map[string]bool{}
	for i, row := range t.rows {
		line := i + 2
		c := ImportContact{Name: t.get(row, "name"), Company: t.get(row, "company"), Email: t.get(row, "email"), Phone: t.get(row, "phone"),
			GSTIN: strings.ToUpper(t.get(row, "gstin")), StateCode: t.get(row, "state"), PAN: strings.ToUpper(t.get(row, "pan")), Kind: t.get(row, "kind")}
		if c.Name == "" {
			rep.add(line, "error", "name is empty", "")
			continue
		}
		if seen[strings.ToLower(c.Name)] {
			rep.add(line, "existing", "duplicate name in the file", c.Name)
			continue
		}
		seen[strings.ToLower(c.Name)] = true
		if c.GSTIN != "" && !ValidGSTIN(c.GSTIN) {
			rep.add(line, "error", c.GSTIN+" is not a valid GSTIN", c.Name)
			continue
		}
		if c.Email != "" && !strings.Contains(c.Email, "@") {
			rep.add(line, "error", "email looks wrong: "+c.Email, c.Name)
			continue
		}
		if len(c.StateCode) > 2 {
			c.StateCode = "" // a state name, not a GST code; derive from the GSTIN instead
		}
		if req.DryRun {
			rep.add(line, "ok", "will import", c.Name)
			continue
		}
		created := true
		if ContactSink != nil {
			var err error
			if created, err = ContactSink(ctx, orgID, userID, c); err != nil {
				rep.add(line, "error", err.Error(), c.Name)
				continue
			}
		}
		if c.GSTIN != "" || c.PAN != "" {
			if _, err := r.SaveGSTParty(ctx, orgID, userID, GSTParty{Name: c.Name, GSTIN: c.GSTIN, StateCode: c.StateCode, PAN: c.PAN}); err != nil {
				rep.add(line, "error", err.Error(), c.Name)
				continue
			}
		}
		if created {
			rep.Applied++
			rep.add(line, "ok", "imported", c.Name)
		} else {
			rep.add(line, "existing", "contact already exists", c.Name)
		}
	}
	if ContactSink == nil {
		rep.Notes = append(rep.Notes, "The Contacts app is not connected here; only GST details were saved.")
	}
	return nil
}

type docRow struct {
	number, party, currency, status string
	date, due                       string
	total, balance, tax             int64
}

func (r *Repository) parseDocRows(t *csvTable, rep *ImportReport, what string) []struct {
	line int
	docRow
} {
	var out []struct {
		line int
		docRow
	}
	for i, row := range t.rows {
		line := i + 2
		d := docRow{number: t.get(row, "number"), party: t.get(row, "customer"), currency: strings.ToUpper(t.get(row, "currency")), status: strings.ToLower(t.get(row, "status"))}
		if d.number == "" {
			rep.add(line, "error", what+" number is empty", d.party)
			continue
		}
		if d.party == "" {
			rep.add(line, "error", "party name is empty", d.number)
			continue
		}
		var ok bool
		if d.date, ok = ParseDate(t.get(row, "date")); !ok {
			rep.add(line, "error", "date is not recognised: "+t.get(row, "date"), d.number)
			continue
		}
		if due := t.get(row, "due"); due != "" {
			if d.due, ok = ParseDate(due); !ok {
				rep.add(line, "error", "due date is not recognised: "+due, d.number)
				continue
			}
		}
		if d.total, ok = ParseAmount(t.get(row, "total")); !ok || d.total <= 0 {
			if ok && d.total < 0 {
				d.total = -d.total // Tally exports debits as negative in some views
			} else {
				rep.add(line, "error", "total is not a positive number: "+t.get(row, "total"), d.number)
				continue
			}
		}
		d.balance = d.total
		if b := t.get(row, "balance"); b != "" {
			if d.balance, ok = ParseAmount(b); !ok {
				rep.add(line, "error", "balance is not a number: "+b, d.number)
				continue
			}
			d.balance = abs64(d.balance)
			if d.balance > d.total {
				rep.add(line, "error", "balance is larger than the total", d.number)
				continue
			}
		}
		if tx := t.get(row, "tax"); tx != "" {
			v, _ := ParseAmount(tx)
			d.tax = abs64(v)
			if d.tax > d.total {
				rep.add(line, "error", "tax is larger than the total", d.number)
				continue
			}
		}
		if strings.Contains(d.status, "void") || strings.Contains(d.status, "draft") || strings.Contains(d.status, "cancel") {
			rep.add(line, "existing", "skipped: "+d.status, d.number)
			continue
		}
		out = append(out, struct {
			line int
			docRow
		}{line, d})
	}
	return out
}

func (r *Repository) importInvoices(ctx context.Context, orgID, userID string, t *csvTable, req ImportRequest, rep *ImportReport) error {
	for _, need := range []string{"number", "date", "customer", "total"} {
		if _, ok := t.mapping[need]; !ok {
			return invalid("no %s column found (expected one of: %s)", need, strings.Join(ImportColumnMaps["invoices"][need], ", "))
		}
	}
	mode := orDefault(req.Mode, "open_items")
	if mode != "open_items" && mode != "history" {
		return invalid("mode must be open_items or history")
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return err
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	if mode == "open_items" {
		rep.Notes = append(rep.Notes, "Open items mode: only unpaid balances are brought in, offset against Opening Balance Equity, so earlier history is not double counted. Use it together with an opening balance import.")
	} else {
		rep.Notes = append(rep.Notes, "History mode: each invoice posts sales and GST; paid amounts post as a receipt into the bank account on the invoice date.")
	}
	for _, d := range r.parseDocRows(t, rep, "invoice") {
		sid := fmt.Sprintf("invoice:%s:%s", rep.Source, d.number)
		var exists bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.acct_journals WHERE org_id=$1 AND source='manual_import' AND source_id=$2 AND NOT voided)`, orgID, sid).Scan(&exists)
		if exists {
			rep.add(d.line, "existing", "already imported", d.number)
			continue
		}
		if mode == "open_items" && d.balance == 0 {
			rep.add(d.line, "existing", "fully paid, nothing open", d.number)
			continue
		}
		in := JournalInput{Date: d.date, DueDate: d.due, Memo: "Invoice " + d.number, Ref: d.number, Kind: "invoice", Source: "manual_import", SourceID: sid,
			ContactName: d.party, SourceHash: hashOf(d.number, d.total, d.balance, d.date)}
		memo := in.Memo
		if mode == "open_items" {
			in.Lines = []Line{{AccountID: keys["ar"], Debit: rupees(d.balance), Memo: memo}, {AccountID: keys["opening"], Credit: rupees(d.balance), Memo: memo}}
		} else {
			in.Lines = []Line{{AccountID: keys["ar"], Debit: rupees(d.total), Memo: memo}, {AccountID: keys["sales"], Credit: rupees(d.total - d.tax), Memo: memo}}
			in.Lines = append(in.Lines, taxLines(keys, d.tax, st.Intrastate, true, memo)...)
			if paid := d.total - d.balance; paid > 0 {
				in.Lines = append(in.Lines, Line{AccountID: keys["bank"], Debit: rupees(paid), Memo: "Received " + d.number}, Line{AccountID: keys["ar"], Credit: rupees(paid), Memo: "Received " + d.number})
			}
		}
		in.Currency = d.currency
		if err := r.applyCurrency(ctx, orgID, &in); err != nil {
			rep.add(d.line, "error", err.Error(), d.number)
			continue
		}
		if in.Currency != "" {
			in.SourceHash = hashOf(in.SourceHash, in.Currency, in.Rate)
		}
		if req.DryRun {
			if _, err := checkBalanced(in.Lines); err != nil {
				rep.add(d.line, "error", err.Error(), d.number)
				continue
			}
			rep.add(d.line, "ok", "will post", fmt.Sprintf("%s %s %.2f", d.number, d.party, rupees(d.balance)))
			continue
		}
		if _, _, err := r.PostJournal(ctx, orgID, userID, in); err != nil {
			rep.add(d.line, "error", err.Error(), d.number)
			continue
		}
		rep.Applied++
		rep.add(d.line, "ok", "posted", fmt.Sprintf("%s %s %.2f", d.number, d.party, rupees(d.balance)))
	}
	return nil
}

func (r *Repository) importBills(ctx context.Context, orgID, userID string, t *csvTable, req ImportRequest, rep *ImportReport) error {
	for _, need := range []string{"number", "date", "customer", "total"} {
		if _, ok := t.mapping[need]; !ok {
			return invalid("no %s column found (expected one of: %s)", need, strings.Join(ImportColumnMaps["bills"][need], ", "))
		}
	}
	mode := orDefault(req.Mode, "open_items")
	if mode != "open_items" && mode != "history" {
		return invalid("mode must be open_items or history")
	}
	accts, err := r.ListAccounts(ctx, orgID)
	if err != nil {
		return err
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	if mode == "open_items" {
		rep.Notes = append(rep.Notes, "Open items mode: only unpaid balances are brought in, offset against Opening Balance Equity. The bills can then be paid in Adrient.")
	}
	byLine := map[int]string{}
	for i, row := range t.rows {
		byLine[i+2] = t.get(row, "account")
	}
	for _, d := range r.parseDocRows(t, rep, "bill") {
		var exists bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.acct_bills WHERE org_id=$1 AND kind='bill' AND number=$2 AND lower(vendor_name)=lower($3) AND status<>'void')`,
			orgID, d.number, d.party).Scan(&exists)
		if exists {
			rep.add(d.line, "existing", "already imported", d.number)
			continue
		}
		if mode == "open_items" && d.balance == 0 {
			rep.add(d.line, "existing", "fully paid, nothing open", d.number)
			continue
		}
		acct := keys["misc"]
		if mode == "open_items" {
			acct = keys["opening"]
		} else if a, ok := r.findAccountByRef(accts, "", byLine[d.line]); ok {
			acct = a.ID
		}
		amount := d.total - d.tax
		if mode == "open_items" {
			amount, d.tax = d.balance, 0
		}
		rate := 0.0
		if d.tax > 0 && amount > 0 {
			rate = float64(d.tax) / float64(amount) * 100
		}
		in := BillInput{Kind: "bill", Number: d.number, VendorName: d.party, BillDate: d.date, DueDate: d.due, Currency: d.currency, SkipApproval: true,
			Lines: []BillLineIn{{AccountID: acct, Description: "Imported " + d.number, Amount: rupees(amount), TaxRate: float64(int64(rate*1000+0.5)) / 1000}}}
		if mode == "open_items" {
			in.Lines[0].TaxRate = 0
		}
		if req.DryRun {
			if _, _, _, err := computeBill(in.Lines); err != nil {
				rep.add(d.line, "error", err.Error(), d.number)
				continue
			}
			rep.add(d.line, "ok", "will create bill", fmt.Sprintf("%s %s %.2f", d.number, d.party, rupees(amount+d.tax)))
			continue
		}
		b, err := r.SaveBill(ctx, orgID, userID, "", in)
		if err != nil {
			rep.add(d.line, "error", err.Error(), d.number)
			continue
		}
		if mode == "history" && d.total > d.balance {
			// record the paid part from the bank so the open balance matches the export
			if _, err := r.PayVendorBill(ctx, orgID, userID, b.ID, BillPayment{Date: d.date, Amount: rupees(d.total - d.balance), AccountID: keys["bank"], Reference: "imported payment"}); err != nil {
				rep.add(d.line, "error", "bill created but its payment failed: "+err.Error(), d.number)
				continue
			}
		}
		rep.Applied++
		rep.add(d.line, "ok", "created", fmt.Sprintf("%s %s %.2f", d.number, d.party, b.Total))
	}
	return nil
}

// ImportColumnHelp returns the documented column names for the wizard, sorted.
func ImportColumnHelp() map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for typ, m := range ImportColumnMaps {
		var fields []string
		for f := range m {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, f := range fields {
			out[typ] = append(out[typ], map[string]any{"field": f, "columns": m[f]})
		}
	}
	return out
}
