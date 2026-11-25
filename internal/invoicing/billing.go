package invoicing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- reminders ----

// RuleKey identifies a reminder rule in the log.
func RuleKey(days int) string { return fmt.Sprintf("d%+d", days) }

// PickRule chooses which reminder is due for an invoice that is offset days
// past its due date (negative = before). It returns the latest rule that has
// been reached (the one to send) and the earlier ones that are skipped, so an
// invoice first seen when it is already 20 days late gets one reminder, not four.
func PickRule(rules []ReminderRule, offset int) (send *ReminderRule, earlier []ReminderRule) {
	sorted := append([]ReminderRule(nil), rules...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Days < sorted[j].Days })
	for i := range sorted {
		if sorted[i].Days <= offset {
			send = &sorted[i]
		}
	}
	if send != nil {
		for _, r := range sorted {
			if r.Days < send.Days {
				earlier = append(earlier, r)
			}
		}
	}
	return
}

var tplVar = regexp.MustCompile(`\{([a-z_]+)\}`)

// RenderTemplate replaces {name} placeholders; unknown ones are left as typed.
func RenderTemplate(tpl string, vars map[string]string) string {
	return tplVar.ReplaceAllStringFunc(tpl, func(m string) string {
		if v, ok := vars[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

// ReminderVars are the placeholders a reminder template can use.
func ReminderVars(d Document, s Settings, origin string, today time.Time) map[string]string {
	status := "is awaiting payment"
	if due := parseDate(orDue(d)); due != nil {
		days := int(today.Sub(*due).Hours() / 24)
		switch {
		case days > 0:
			status = fmt.Sprintf("was due on %s and is %d day(s) overdue", orDue(d), days)
		case days == 0:
			status = "is due today"
		default:
			status = "is due on " + orDue(d)
		}
	}
	return map[string]string{
		"client": d.ClientName, "number": d.Number, "amount": fmt.Sprintf("%s %.2f", d.Currency, d.Balance),
		"total": fmt.Sprintf("%s %.2f", d.Currency, d.Total), "due_date": d.DueDate, "status": status,
		"link": strings.TrimRight(origin, "/") + "/invoice/" + d.ShareToken, "business": s.BusinessName,
	}
}

// WhatsAppLink builds a click-to-chat link (wa.me) with the reminder text. No
// WhatsApp API is called: the user's own WhatsApp opens with the message ready.
func WhatsAppLink(phone, text string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	base := "https://wa.me/"
	if digits != "" {
		base += digits
	}
	return base + "?text=" + url.QueryEscape(text)
}

// ReminderLogRow is a row of the reminder history.
type ReminderLogRow struct {
	DocumentID string `json:"document_id"`
	Number     string `json:"number"`
	Client     string `json:"client_name"`
	Rule       string `json:"rule"`
	Channel    string `json:"channel"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	At         string `json:"at"`
}

// ListReminderLog returns the latest scheduled reminders.
func (r *Repository) ListReminderLog(ctx context.Context, orgID string) ([]ReminderLogRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT l.document_id::text, d.number, d.client_name, l.rule_key, l.channel, l.status, l.detail,
		to_char(l.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM adrient.invc_reminder_log l JOIN adrient.invc_documents d ON d.id=l.document_id
		WHERE l.org_id=$1 ORDER BY l.created_at DESC LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReminderLogRow{}
	for rows.Next() {
		var x ReminderLogRow
		if err := rows.Scan(&x.DocumentID, &x.Number, &x.Client, &x.Rule, &x.Channel, &x.Status, &x.Detail, &x.At); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ReminderSender delivers a reminder email.
type ReminderSender func(ctx context.Context, to, subject, text string) error

// RunReminders sends the scheduled reminders of one org. It is safe to call as
// often as you like and from several processes: each (invoice, rule) pair is
// claimed in the log before sending, so it is sent at most once. A failed send
// releases the claim so the next run retries.
func (r *Repository) RunReminders(ctx context.Context, orgID, origin string, send ReminderSender, now time.Time) (sent int, err error) {
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return 0, err
	}
	if !s.Sales.RemindersAuto || len(s.Sales.ReminderRules) == 0 {
		return 0, nil
	}
	docs, err := r.ListDocuments(ctx, orgID, DocFilter{Kind: "invoice"})
	if err != nil {
		return 0, err
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, d := range docs {
		due := parseDate(orDue(d))
		if d.Status == "draft" || d.Balance <= 0.004 || due == nil || d.ClientEmail == "" {
			continue
		}
		offset := int(day.Sub(*due).Hours() / 24)
		rule, earlier := PickRule(s.Sales.ReminderRules, offset)
		if rule == nil {
			continue
		}
		for _, e := range earlier {
			_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.invc_reminder_log (org_id, document_id, rule_key, status, detail) VALUES ($1,$2::uuid,$3,'skipped','a later rule was already due')
				ON CONFLICT DO NOTHING`, orgID, d.ID, RuleKey(e.Days))
		}
		tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.invc_reminder_log (org_id, document_id, rule_key, status) VALUES ($1,$2::uuid,$3,'sent')
			ON CONFLICT DO NOTHING`, orgID, d.ID, RuleKey(rule.Days))
		if err != nil {
			return sent, err
		}
		if tag.RowsAffected() == 0 {
			continue // already handled
		}
		vars := ReminderVars(d, s, origin, day)
		if serr := send(ctx, d.ClientEmail, RenderTemplate(s.Sales.ReminderSubject, vars), RenderTemplate(s.Sales.ReminderBody, vars)); serr != nil {
			_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.invc_reminder_log WHERE document_id=$1::uuid AND rule_key=$2`, d.ID, RuleKey(rule.Days))
			continue
		}
		_ = r.NoteReminder(ctx, orgID, d.ID)
		sent++
	}
	return sent, nil
}

// OrgIDsWhere returns org ids for a scheduler query that selects one org_id column.
func (r *Repository) OrgIDsWhere(ctx context.Context, query string) ([]string, error) {
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- customers: statement, aging, portal ----

func sameCustomer(d Document, key string) bool {
	if strings.HasPrefix(key, "name:") {
		return d.ContactID == "" && strings.EqualFold(d.ClientName, key[5:])
	}
	return d.ContactID == key
}

// CustomerKey is the stable key of a document's customer.
func CustomerKey(d Document) string {
	if d.ContactID != "" {
		return d.ContactID
	}
	return "name:" + strings.ToLower(d.ClientName)
}

// StatementLine is one row of a statement of account.
type StatementLine struct {
	Date        string  `json:"date"`
	Type        string  `json:"type"` // invoice | credit_note | payment | refund | writeoff
	Number      string  `json:"number"`
	Description string  `json:"description"`
	Debit       float64 `json:"debit"`  // customer owes more
	Credit      float64 `json:"credit"` // customer owes less
	Balance     float64 `json:"balance"`
}

// Statement is a customer's account between two dates.
type Statement struct {
	Customer string          `json:"customer"`
	Email    string          `json:"email"`
	Address  string          `json:"address"`
	Currency string          `json:"currency"`
	From     string          `json:"from"`
	To       string          `json:"to"`
	Opening  float64         `json:"opening"`
	Lines    []StatementLine `json:"lines"`
	Closing  float64         `json:"closing"`
	Overdue  float64         `json:"overdue"`
	Skipped  int             `json:"skipped_other_currency"`
}

// BuildStatement folds invoices, credit notes and money movements of one
// customer into a running balance. Credit applied from a note to an invoice is
// not listed twice: the note itself is the credit line.
func BuildStatement(docs []Document, key, currency, from, to string, now time.Time) Statement {
	st := Statement{Currency: currency, From: from, To: to, Lines: []StatementLine{}}
	type ev struct {
		StatementLine
		order int
	}
	var evs []ev
	for _, d := range docs {
		if d.Deleted || d.Status == "draft" || !sameCustomer(d, key) || (d.Kind != "invoice" && d.Kind != "credit_note") {
			continue
		}
		if st.Customer == "" {
			st.Customer, st.Email, st.Address = d.ClientName, d.ClientEmail, d.ClientAddress
		}
		if d.Currency != currency {
			st.Skipped++
			continue
		}
		if d.Kind == "invoice" {
			evs = append(evs, ev{StatementLine{Date: d.IssueDate, Type: "invoice", Number: d.Number, Description: "Invoice, due " + d.DueDate, Debit: d.Total}, 0})
			st.Overdue += OverdueAmount(d, now)
		} else {
			evs = append(evs, ev{StatementLine{Date: d.IssueDate, Type: "credit_note", Number: d.Number, Description: "Credit note " + trunc(d.Notes, 60), Credit: d.Total}, 0})
		}
		for _, p := range d.Payments {
			switch p.Kind {
			case "payment":
				evs = append(evs, ev{StatementLine{Date: p.PaidAt, Type: "payment", Number: d.Number, Description: strings.TrimSpace("Payment " + p.Method + " " + p.Reference), Credit: p.Amount}, 1})
			case "refund":
				evs = append(evs, ev{StatementLine{Date: p.PaidAt, Type: "refund", Number: d.Number, Description: strings.TrimSpace("Refund " + p.Method + " " + p.Reference), Debit: p.Amount}, 1})
			case "writeoff":
				evs = append(evs, ev{StatementLine{Date: p.PaidAt, Type: "writeoff", Number: d.Number, Description: "Written off", Credit: p.Amount}, 1})
			}
		}
	}
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].Date != evs[j].Date {
			return evs[i].Date < evs[j].Date
		}
		return evs[i].order < evs[j].order
	})
	bal := 0.0
	for _, e := range evs {
		bal = Round2(bal + e.Debit - e.Credit)
		e.Balance = bal
		if from != "" && e.Date < from {
			st.Opening = bal
			continue
		}
		if to != "" && e.Date > to {
			continue
		}
		st.Lines = append(st.Lines, e.StatementLine)
	}
	st.Closing = st.Opening
	if len(st.Lines) > 0 {
		st.Closing = st.Lines[len(st.Lines)-1].Balance
	}
	st.Overdue = Round2(st.Overdue)
	return st
}

// Statement builds the statement for one customer key.
func (r *Repository) Statement(ctx context.Context, orgID, key, currency, from, to string) (Statement, error) {
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Statement{}, err
	}
	if currency == "" {
		currency = s.DefaultCurrency
	}
	docs := []Document{}
	for _, k := range []string{"invoice", "credit_note"} {
		ds, err := r.ListDocuments(ctx, orgID, DocFilter{Kind: k})
		if err != nil {
			return Statement{}, err
		}
		docs = append(docs, ds...)
	}
	st := BuildStatement(docs, key, currency, from, to, today())
	if st.Customer == "" {
		return st, ErrNotFound
	}
	return st, nil
}

// AgingRow is one customer's open balance by age.
type AgingRow struct {
	CustomerKey string  `json:"customer_key"`
	Customer    string  `json:"customer"`
	Email       string  `json:"email"`
	Current     float64 `json:"current"`
	D1to30      float64 `json:"d1_30"`
	D31to60     float64 `json:"d31_60"`
	D61to90     float64 `json:"d61_90"`
	D90plus     float64 `json:"d90_plus"`
	Credit      float64 `json:"unapplied_credit"`
	Total       float64 `json:"total"`
	Invoices    int     `json:"invoices"`
}

// BuildAging buckets open invoice balances per customer by days past due, in
// the base currency. Unapplied credit-note balances are shown separately and
// netted in the total.
func BuildAging(docs []Document, s Settings, now time.Time) []AgingRow {
	idx := map[string]*AgingRow{}
	var order []string
	get := func(d Document) *AgingRow {
		k := CustomerKey(d)
		if idx[k] == nil {
			idx[k] = &AgingRow{CustomerKey: k, Customer: d.ClientName, Email: d.ClientEmail}
			order = append(order, k)
		}
		return idx[k]
	}
	for _, d := range docs {
		if d.Deleted || d.Status == "draft" {
			continue
		}
		conv := func(a float64) float64 { return ToBase(a, d.Currency, s.DefaultCurrency, s.Rates) }
		switch {
		case d.Kind == "invoice" && d.Balance > 0.004:
			row := get(d)
			for _, part := range AgingParts(d, now) {
				b := conv(part.Amount)
				switch ageBucket(part.Days) {
				case 4:
					row.D90plus += b
				case 3:
					row.D61to90 += b
				case 2:
					row.D31to60 += b
				case 1:
					row.D1to30 += b
				default:
					row.Current += b
				}
			}
			row.Invoices++
		case d.Kind == "credit_note" && d.CreditRemain > 0.004:
			get(d).Credit += conv(d.CreditRemain)
		}
	}
	out := make([]AgingRow, 0, len(order))
	for _, k := range order {
		a := idx[k]
		a.Current, a.D1to30, a.D31to60, a.D61to90, a.D90plus, a.Credit = Round2(a.Current), Round2(a.D1to30), Round2(a.D31to60), Round2(a.D61to90), Round2(a.D90plus), Round2(a.Credit)
		a.Total = Round2(a.Current + a.D1to30 + a.D31to60 + a.D61to90 + a.D90plus - a.Credit)
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out
}

// CustomerAging returns the aging table for the org.
func (r *Repository) CustomerAging(ctx context.Context, orgID string) ([]AgingRow, error) {
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	docs := []Document{}
	for _, k := range []string{"invoice", "credit_note"} {
		ds, err := r.ListDocuments(ctx, orgID, DocFilter{Kind: k})
		if err != nil {
			return nil, err
		}
		docs = append(docs, ds...)
	}
	return BuildAging(docs, s, today()), nil
}

// PortalToken returns the customer's portal token, creating it on first use.
func (r *Repository) PortalToken(ctx context.Context, orgID, customerKey string) (string, error) {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	var tok string
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.invc_portal (org_id, customer_key, token) VALUES ($1,$2,$3)
		ON CONFLICT (org_id, customer_key) DO UPDATE SET customer_key=EXCLUDED.customer_key RETURNING token`, orgID, customerKey, hex.EncodeToString(b)).Scan(&tok)
	return tok, err
}

// PortalInvoice is what a customer sees for each invoice.
type PortalInvoice struct {
	Number     string  `json:"number"`
	IssueDate  string  `json:"issue_date"`
	DueDate    string  `json:"due_date"`
	Currency   string  `json:"currency"`
	Total      float64 `json:"total"`
	Balance    float64 `json:"balance"`
	Status     string  `json:"status"`
	ShareToken string  `json:"share_token"`
}

// Portal is the public customer portal payload.
type Portal struct {
	Business  map[string]any  `json:"business"`
	Customer  string          `json:"customer"`
	Invoices  []PortalInvoice `json:"invoices"`
	Estimates []PortalInvoice `json:"estimates"`
	Due       float64         `json:"total_due"`
	Currency  string          `json:"currency"`
}

// PortalByToken returns a customer's invoices and open estimates.
func (r *Repository) PortalByToken(ctx context.Context, token string) (Portal, error) {
	var orgID, key string
	err := r.pool.QueryRow(ctx, `SELECT org_id::text, customer_key FROM adrient.invc_portal WHERE token=$1`, token).Scan(&orgID, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Portal{}, ErrNotFound
	}
	if err != nil {
		return Portal{}, err
	}
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Portal{}, err
	}
	p := Portal{Currency: s.DefaultCurrency, Invoices: []PortalInvoice{}, Estimates: []PortalInvoice{},
		Business: map[string]any{"business_name": s.BusinessName, "email": s.Email, "phone": s.Phone, "logo_url": s.LogoURL, "address": s.Address}}
	for _, kind := range []string{"invoice", "estimate"} {
		ds, err := r.ListDocuments(ctx, orgID, DocFilter{Kind: kind})
		if err != nil {
			return p, err
		}
		for _, d := range ds {
			if d.Status == "draft" || !sameCustomer(d, key) {
				continue
			}
			if p.Customer == "" {
				p.Customer = d.ClientName
			}
			pi := PortalInvoice{Number: d.Number, IssueDate: d.IssueDate, DueDate: d.DueDate, Currency: d.Currency, Total: d.Total,
				Balance: d.Balance, Status: d.DisplayStatus, ShareToken: d.ShareToken}
			if kind == "invoice" {
				p.Invoices = append(p.Invoices, pi)
				p.Due += ToBase(d.Balance, d.Currency, s.DefaultCurrency, s.Rates)
			} else if d.Status == "sent" || d.Status == "accepted" {
				p.Estimates = append(p.Estimates, pi)
			}
		}
	}
	p.Due = Round2(p.Due)
	return p, nil
}

// ---- e-invoice ----

// EInvoice builds the GST e-invoice (IRP schema 1.1) JSON for an invoice or
// credit note and lists what is missing for a valid submission. The JSON is
// ready to upload to the IRP; Adrient does not call the IRP itself.
func EInvoice(d Document, s Settings) (map[string]any, []string) {
	var issues []string
	need := func(ok bool, msg string) {
		if !ok {
			issues = append(issues, msg)
		}
	}
	supplierState := StateCode(s.Sales.StateCode)
	need(ValidGSTIN(s.TaxID), "your business GSTIN (Settings > Tax ID) is missing or invalid")
	need(supplierState != "", "your business GST state code (Settings > GST) is not set")
	need(d.Kind == "invoice" || d.Kind == "credit_note", "only invoices and credit notes can be e-invoiced")
	need(ValidGSTIN(d.ClientGSTIN), "the customer's GSTIN is missing or invalid (e-invoice is for B2B supplies)")
	need(d.Status != "draft", "the document is still a draft")
	need(d.Currency == "INR", "values must be in INR")
	b := GST(d.Lines, d.DiscountKind, d.DiscountValue, supplierState, d.PlaceOfSupply)
	items := make([]map[string]any, 0, len(d.Lines))
	var assTotal float64
	for i, l := range d.Lines {
		lt := b.Lines[i]
		need(strings.TrimSpace(l.HSNSAC) != "", fmt.Sprintf("line %d (%s) needs an HSN/SAC code", i+1, trunc(l.Description, 30)))
		gross := Round2(l.Qty * l.Price)
		lineDisc := Round2(gross - l.Qty*l.Price*(1-clampPct(l.DiscountPct)/100))
		share := Round2(lt.Taxable)
		items = append(items, map[string]any{
			"SlNo": fmt.Sprint(i + 1), "PrdDesc": l.Description, "IsServc": yn(isService(l.HSNSAC)), "HsnCd": l.HSNSAC,
			"Qty": l.Qty, "Unit": unitCode(l.Unit), "UnitPrice": Round2(l.Price), "TotAmt": gross, "Discount": lineDisc,
			"AssAmt": share, "GstRt": lt.Rate, "IgstAmt": lt.IGST, "CgstAmt": lt.CGST, "SgstAmt": lt.SGST,
			"TotItemVal": Round2(share + lt.Tax),
		})
		assTotal += share
	}
	typ := "INV"
	if d.Kind == "credit_note" {
		typ = "CRN"
	}
	doc := map[string]any{
		"Version": "1.1",
		"TranDtls": map[string]any{"TaxSch": "GST", "SupTyp": supTyp(d.PlaceOfSupply), "RegRev": "N", "IgstOnIntra": "N"},
		"DocDtls":  map[string]any{"Typ": typ, "No": d.Number, "Dt": dmy(d.IssueDate)},
		"SellerDtls": map[string]any{"Gstin": s.TaxID, "LglNm": s.BusinessName, "Addr1": trunc(s.Address, 100),
			"Loc": StateName(supplierState), "Pin": pin(s.Address), "Stcd": supplierState},
		"BuyerDtls": map[string]any{"Gstin": d.ClientGSTIN, "LglNm": d.ClientName, "Pos": StateCode(d.PlaceOfSupply),
			"Addr1": trunc(d.ClientAddress, 100), "Loc": StateName(StateCode(d.PlaceOfSupply)), "Pin": pin(d.ClientAddress), "Stcd": StateCode(d.PlaceOfSupply)},
		"ItemList": items,
		"ValDtls": map[string]any{"AssVal": Round2(assTotal), "CgstVal": b.CGST, "SgstVal": b.SGST, "IgstVal": b.IGST,
			"Discount": 0, "TotInvVal": d.Total},
	}
	if d.Kind == "invoice" && d.DueDate != "" {
		doc["PayDtls"] = map[string]any{"Nm": s.BusinessName, "PayTerm": "Due " + d.DueDate}
	}
	need(len(pin(s.Address)) == 6, "your address needs a 6-digit PIN code")
	need(len(pin(d.ClientAddress)) == 6, "the customer's address needs a 6-digit PIN code")
	return doc, issues
}

func yn(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}

// isService reports whether an HSN/SAC code is a service (SAC codes start with 99).
func isService(code string) bool { return strings.HasPrefix(strings.TrimSpace(code), "99") }

func supTyp(pos string) string {
	if StateCode(pos) == "96" {
		return "EXPWOP"
	}
	return "B2B"
}

func dmy(iso string) string {
	if t := parseDate(iso); t != nil {
		return t.Format("02/01/2006")
	}
	return iso
}

var pinRe = regexp.MustCompile(`\b[1-9][0-9]{5}\b`)

func pin(addr string) string { return pinRe.FindString(addr) }

// unitCode maps common unit words to the UQC codes the IRP accepts.
func unitCode(u string) string {
	switch strings.ToLower(strings.TrimSpace(u)) {
	case "pcs", "pc", "piece", "pieces":
		return "PCS"
	case "kg", "kgs":
		return "KGS"
	case "hrs", "hr", "hours":
		return "OTH"
	case "days", "day", "month", "months":
		return "OTH"
	case "nos", "no":
		return "NOS"
	case "box":
		return "BOX"
	case "ltr", "l", "litre", "liter":
		return "LTR"
	case "mtr", "m", "meter", "metre":
		return "MTR"
	}
	return "OTH"
}
