package accounting

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/jackc/pgx/v5"
)

// ---- tax rates and codes (GST rates, TDS sections) ----

// TaxRate is an editable GST rate or TDS section.
type TaxRate struct {
	ID      string  `json:"id"`
	Kind    string  `json:"kind"` // gst | tds
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Rate    float64 `json:"rate"`
	Section string  `json:"section"`
	Active  bool    `json:"active"`
}

var defaultTaxRates = []TaxRate{
	{Kind: "gst", Code: "GST0", Name: "GST 0% / exempt", Rate: 0}, {Kind: "gst", Code: "GST5", Name: "GST 5%", Rate: 5},
	{Kind: "gst", Code: "GST12", Name: "GST 12%", Rate: 12}, {Kind: "gst", Code: "GST18", Name: "GST 18%", Rate: 18},
	{Kind: "gst", Code: "GST28", Name: "GST 28%", Rate: 28},
	{Kind: "tds", Code: "194C", Name: "Contractors (company/firm)", Rate: 2, Section: "194C"},
	{Kind: "tds", Code: "194H", Name: "Commission or brokerage", Rate: 5, Section: "194H"},
	{Kind: "tds", Code: "194I", Name: "Rent (land, building)", Rate: 10, Section: "194I"},
	{Kind: "tds", Code: "194J", Name: "Professional / technical fees", Rate: 10, Section: "194J"},
	{Kind: "tds", Code: "194Q", Name: "Purchase of goods", Rate: 0.1, Section: "194Q"},
	{Kind: "tds", Code: "194A", Name: "Interest other than securities", Rate: 10, Section: "194A"},
}

func (r *Repository) ListTaxRates(ctx context.Context, orgID string) ([]TaxRate, error) {
	var n int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.acct_tax_rates WHERE org_id=$1`, orgID).Scan(&n)
	if n == 0 {
		for _, t := range defaultTaxRates {
			_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.acct_tax_rates (org_id, kind, code, name, rate, section) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
				orgID, t.Kind, t.Code, t.Name, t.Rate, t.Section)
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, kind, code, name, rate::float8, section, active FROM adrient.acct_tax_rates WHERE org_id=$1 ORDER BY kind, rate, code`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaxRate{}
	for rows.Next() {
		var t TaxRate
		if err := rows.Scan(&t.ID, &t.Kind, &t.Code, &t.Name, &t.Rate, &t.Section, &t.Active); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) SaveTaxRate(ctx context.Context, orgID, userID, id string, t TaxRate) (TaxRate, error) {
	t.Code, t.Name = strings.ToUpper(strings.TrimSpace(t.Code)), strings.TrimSpace(t.Name)
	if t.Kind != "gst" && t.Kind != "tds" {
		return TaxRate{}, invalid("kind must be gst or tds")
	}
	if t.Code == "" {
		return TaxRate{}, invalid("code is required")
	}
	if t.Rate < 0 || t.Rate > 100 {
		return TaxRate{}, invalid("rate must be between 0 and 100")
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_tax_rates (org_id, kind, code, name, rate, section, active) VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (org_id, kind, code) DO UPDATE SET name=EXCLUDED.name, rate=EXCLUDED.rate, section=EXCLUDED.section, active=EXCLUDED.active RETURNING id::text`,
			orgID, t.Kind, t.Code, t.Name, t.Rate, t.Section, t.Active).Scan(&t.ID)
	} else {
		if !isUUID(id) {
			return TaxRate{}, ErrNotFound
		}
		tag, e := r.pool.Exec(ctx, `UPDATE adrient.acct_tax_rates SET kind=$3, code=$4, name=$5, rate=$6, section=$7, active=$8 WHERE org_id=$1 AND id=$2::uuid`,
			orgID, id, t.Kind, t.Code, t.Name, t.Rate, t.Section, t.Active)
		err, t.ID = e, id
		if err == nil && tag.RowsAffected() == 0 {
			return TaxRate{}, ErrNotFound
		}
	}
	if err != nil {
		return TaxRate{}, err
	}
	r.audit(ctx, orgID, userID, "save", "tax_rate", t.ID, t.Code)
	return t, nil
}

func (r *Repository) DeleteTaxRate(ctx context.Context, orgID, userID, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_tax_rates WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.audit(ctx, orgID, userID, "delete", "tax_rate", id, "")
	return nil
}

// ---- GST parties and HSN map ----

// GSTParty holds the GST details of a customer or vendor contact.
type GSTParty struct {
	ContactID string `json:"contact_id"`
	Name      string `json:"name"`
	GSTIN     string `json:"gstin"`
	StateCode string `json:"state_code"`
	PAN       string `json:"pan"`
}

// ValidGSTIN checks the 15-character GSTIN shape and its check character.
func ValidGSTIN(g string) bool {
	g = strings.ToUpper(strings.TrimSpace(g))
	if len(g) != 15 {
		return false
	}
	const cs = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	sum := 0
	for i := 0; i < 14; i++ {
		v := strings.IndexByte(cs, g[i])
		if v < 0 {
			return false
		}
		f := 1
		if i%2 == 1 {
			f = 2
		}
		p := v * f
		sum += p/36 + p%36
	}
	return cs[(36-sum%36)%36] == g[14]
}

func (r *Repository) ListGSTParties(ctx context.Context, orgID string) ([]GSTParty, error) {
	rows, err := r.pool.Query(ctx, `SELECT contact_id, name, gstin, state_code, pan FROM adrient.acct_gst_parties WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GSTParty{}
	for rows.Next() {
		var p GSTParty
		if err := rows.Scan(&p.ContactID, &p.Name, &p.GSTIN, &p.StateCode, &p.PAN); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) SaveGSTParty(ctx context.Context, orgID, userID string, p GSTParty) (GSTParty, error) {
	p.GSTIN, p.PAN = strings.ToUpper(strings.TrimSpace(p.GSTIN)), strings.ToUpper(strings.TrimSpace(p.PAN))
	p.Name = strings.TrimSpace(p.Name)
	if p.ContactID == "" {
		p.ContactID = "name:" + strings.ToLower(p.Name)
	}
	if p.Name == "" && p.ContactID == "" {
		return p, invalid("name is required")
	}
	if p.GSTIN != "" && !ValidGSTIN(p.GSTIN) {
		return p, invalid("%s is not a valid GSTIN (15 characters with a correct check digit)", p.GSTIN)
	}
	if p.StateCode == "" && len(p.GSTIN) >= 2 {
		p.StateCode = p.GSTIN[:2]
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.acct_gst_parties (org_id, contact_id, name, gstin, state_code, pan) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (org_id, contact_id) DO UPDATE SET name=EXCLUDED.name, gstin=EXCLUDED.gstin, state_code=EXCLUDED.state_code, pan=EXCLUDED.pan`,
		orgID, p.ContactID, p.Name, p.GSTIN, p.StateCode, p.PAN)
	if err == nil {
		r.audit(ctx, orgID, userID, "save", "gst_party", p.ContactID, p.Name+" "+p.GSTIN)
	}
	return p, err
}

func (r *Repository) partyIndex(ctx context.Context, orgID string) map[string]GSTParty {
	m := map[string]GSTParty{}
	ps, _ := r.ListGSTParties(ctx, orgID)
	for _, p := range ps {
		m[p.ContactID] = p
		m["name:"+strings.ToLower(p.Name)] = p
	}
	return m
}

func (r *Repository) SaveHSN(ctx context.Context, orgID, match, hsn string) error {
	match = strings.ToLower(strings.TrimSpace(match))
	hsn = strings.TrimSpace(hsn)
	if match == "" || len(hsn) < 4 || len(hsn) > 8 {
		return invalid("item text and a 4 to 8 digit HSN/SAC code are required")
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.acct_hsn_map (org_id, match, hsn) VALUES ($1,$2,$3) ON CONFLICT (org_id, match) DO UPDATE SET hsn=EXCLUDED.hsn`, orgID, match, hsn)
	return err
}

func (r *Repository) hsnIndex(ctx context.Context, orgID string) map[string]string {
	m := map[string]string{}
	rows, err := r.pool.Query(ctx, `SELECT match, hsn FROM adrient.acct_hsn_map WHERE org_id=$1`, orgID)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var a, b string
		if rows.Scan(&a, &b) == nil {
			m[a] = b
		}
	}
	return m
}

// ---- GSTR-1 / GSTR-3B data ----

// GSTRInvoice is one outward supply row of a GSTR-1 style return.
type GSTRInvoice struct {
	Section  string  `json:"section"` // b2b | b2cl | b2cs | cdnr | cdnur | exp
	GSTIN    string  `json:"gstin"`
	Party    string  `json:"party"`
	Number   string  `json:"number"`
	Date     string  `json:"date"`
	POS      string  `json:"place_of_supply"`
	Rate     float64 `json:"rate"`
	Taxable  float64 `json:"taxable"`
	IGST     float64 `json:"igst"`
	CGST     float64 `json:"cgst"`
	SGST     float64 `json:"sgst"`
	Total    float64 `json:"total"`
	Currency string  `json:"currency,omitempty"`
}

// TaxReturn is GSTR-1 (rows) or GSTR-3B (sections) data for a period.
type TaxReturn struct {
	Return   string                `json:"return"` // gstr1 | gstr3b
	From     string                `json:"from"`
	To       string                `json:"to"`
	GSTIN    string                `json:"gstin"`
	Rows     []GSTRInvoice         `json:"rows"`
	Summary  map[string]GSTRTotals `json:"summary"`
	Sections []ReportRow           `json:"sections"` // GSTR-3B lines (Debit = taxable/ITC, Credit = tax)
	Notes    []string              `json:"notes"`
}

// GSTRTotals sums one section.
type GSTRTotals struct {
	Count   int     `json:"count"`
	Taxable float64 `json:"taxable"`
	IGST    float64 `json:"igst"`
	CGST    float64 `json:"cgst"`
	SGST    float64 `json:"sgst"`
}

func nearestSlab(pct float64) float64 {
	best, diff := 0.0, 1e9
	for _, s := range []float64{0, 0.25, 3, 5, 12, 18, 28, 40} {
		if d := math.Abs(pct - s); d < diff {
			best, diff = s, d
		}
	}
	return best
}

func stateOf(gstin, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if len(gstin) >= 2 {
		return gstin[:2]
	}
	return ""
}

// splitTax divides tax into heads: IGST for inter-state, CGST+SGST for intra-state.
func splitTax(tax float64, inter bool) (igst, cgst, sgst float64) {
	p := paise(tax)
	if inter {
		return rupees(p), 0, 0
	}
	return 0, rupees(p / 2), rupees(p - p/2)
}

func (s *TaxReturn) finish() {
	s.Summary = map[string]GSTRTotals{}
	sort.SliceStable(s.Rows, func(i, j int) bool {
		if s.Rows[i].Section != s.Rows[j].Section {
			return s.Rows[i].Section < s.Rows[j].Section
		}
		return s.Rows[i].Date < s.Rows[j].Date
	})
	for _, r := range s.Rows {
		t := s.Summary[r.Section]
		t.Count++
		t.Taxable = rupees(paise(t.Taxable) + paise(r.Taxable))
		t.IGST = rupees(paise(t.IGST) + paise(r.IGST))
		t.CGST = rupees(paise(t.CGST) + paise(r.CGST))
		t.SGST = rupees(paise(t.SGST) + paise(r.SGST))
		s.Summary[r.Section] = t
	}
}

// docRows turns an invoice into GSTR-1 rows, one per tax rate.
func docRows(d invoicing.Document, st Settings, parties map[string]GSTParty, rate float64) []GSTRInvoice {
	party := parties[d.ContactID]
	if party.Name == "" {
		party = parties["name:"+strings.ToLower(d.ClientName)]
	}
	home := stateOf(st.GSTIN, "")
	pos := stateOf(party.GSTIN, party.StateCode)
	inter := !st.Intrastate
	if pos != "" && home != "" {
		inter = pos != home
	}
	byRate := map[float64]float64{}
	var sub float64
	nets := make([]float64, len(d.Lines))
	for i, l := range d.Lines {
		nets[i] = l.Qty * l.Price * (1 - math.Min(math.Max(l.DiscountPct, 0), 100)/100)
		sub += nets[i]
	}
	for i, l := range d.Lines {
		share := nets[i]
		if sub > 0 {
			share = nets[i] - d.DiscountTotal*nets[i]/sub
		}
		byRate[l.TaxRate] += share
	}
	if len(d.Lines) == 0 {
		byRate[0] = d.Subtotal - d.DiscountTotal
	}
	var rates []float64
	for r := range byRate {
		rates = append(rates, r)
	}
	sort.Float64s(rates)
	foreign := d.Currency != "" && d.Currency != st.Currency
	var out []GSTRInvoice
	for _, rt := range rates {
		taxable := byRate[rt] * rate
		tax := taxable * rt / 100
		g := GSTRInvoice{GSTIN: party.GSTIN, Party: d.ClientName, Number: d.Number, Date: d.IssueDate, POS: pos, Rate: rt, Taxable: rupees(paise(taxable))}
		g.IGST, g.CGST, g.SGST = splitTax(tax, inter || foreign)
		g.Total = rupees(paise(g.Taxable) + paise(g.IGST) + paise(g.CGST) + paise(g.SGST))
		switch {
		case foreign:
			g.Section, g.Currency = "exp", d.Currency
		case party.GSTIN != "":
			g.Section = "b2b"
		case inter && g.Total > 250000:
			g.Section = "b2cl"
		default:
			g.Section = "b2cs"
		}
		out = append(out, g)
	}
	return out
}

// BuildGSTR1 assembles outward supplies for the period from Invoicing, POS and credit memos.
func (r *Repository) BuildGSTR1(ctx context.Context, orgID, from, to string, inv InvoiceSource, pos PosSource) (TaxReturn, error) {
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return TaxReturn{}, err
	}
	parties := r.partyIndex(ctx, orgID)
	out := TaxReturn{Return: "gstr1", From: from, To: to, GSTIN: st.GSTIN, Rows: []GSTRInvoice{}, Notes: []string{}}
	if st.GSTIN == "" {
		out.Notes = append(out.Notes, "Your GSTIN is not set (Settings). Place of supply is assumed to be your own state for parties without a GSTIN.")
	}
	if inv != nil {
		docs, err := inv.ListDocuments(ctx, orgID, invoicing.DocFilter{Kind: "invoice"})
		if err != nil {
			return out, err
		}
		for _, d := range docs {
			if d.Status == "draft" || d.Deleted || d.IssueDate < from || d.IssueDate > to {
				continue
			}
			rate := 1.0
			if d.Currency != "" && d.Currency != st.Currency {
				if rate, err = r.RateOn(ctx, orgID, d.Currency, d.IssueDate); err != nil {
					out.Notes = append(out.Notes, "Invoice "+d.Number+" skipped: "+err.Error())
					continue
				}
			}
			out.Rows = append(out.Rows, docRows(d, st, parties, rate)...)
		}
	}
	if pos != nil {
		until := ""
		for page := 0; page < 100; page++ {
			sales, err := pos.ListSales(ctx, orgID, &adrientv1.ListPosSalesRequest{Until: until, Limit: 500, Detail: false})
			if err != nil {
				return out, err
			}
			stop := false
			for _, s := range sales {
				d := dateOf(orDefault(s.GetCompletedAt(), s.GetCreatedAt()))
				if d < from {
					stop = true
				}
				if d < from || d > to || (s.GetStatus() != "completed" && s.GetStatus() != "refunded" && s.GetStatus() != "partially_refunded") {
					continue
				}
				total, tax := s.GetTotal(), s.GetTaxTotal()
				taxable := total - tax
				rt := 0.0
				if taxable > 0 {
					rt = nearestSlab(tax / taxable * 100)
				}
				g := GSTRInvoice{Section: "b2cs", Party: "POS " + orDefault(s.GetCustomerName(), "walk-in"), Number: fmt.Sprintf("POS-%d", s.GetNumber()), Date: d, Rate: rt, Taxable: rupees(paise(taxable))}
				g.IGST, g.CGST, g.SGST = splitTax(tax, !st.Intrastate)
				g.Total = rupees(paise(total))
				out.Rows = append(out.Rows, g)
			}
			if len(sales) < 500 || stop {
				break
			}
			last := sales[len(sales)-1]
			until = orDefault(last.GetCompletedAt(), last.GetCreatedAt())
		}
	}
	// credit memos: registered customers are CDNR, others CDNUR (rows carry negative-sign semantics by section)
	memos, err := r.ListCreditMemos(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, m := range memos {
		if m.Status != "open" || m.Date < from || m.Date > to {
			continue
		}
		party := parties[m.CustomerID]
		if party.Name == "" {
			party = parties["name:"+strings.ToLower(m.CustomerName)]
		}
		home, posS := stateOf(st.GSTIN, ""), stateOf(party.GSTIN, party.StateCode)
		inter := !st.Intrastate
		if home != "" && posS != "" {
			inter = home != posS
		}
		rate := m.Rate
		if rate <= 0 {
			rate = 1
		}
		g := GSTRInvoice{Section: "cdnur", GSTIN: party.GSTIN, Party: m.CustomerName, Number: m.Number, Date: m.Date, POS: posS, Rate: m.GSTRate, Taxable: rupees(paise(m.Amount * rate))}
		if party.GSTIN != "" {
			g.Section = "cdnr"
		}
		g.IGST, g.CGST, g.SGST = splitTax(m.TaxTotal*rate, inter)
		g.Total = rupees(paise(g.Taxable) + paise(g.IGST) + paise(g.CGST) + paise(g.SGST))
		out.Rows = append(out.Rows, g)
	}
	out.finish()
	return out, nil
}

// BuildGSTR3B summarises the period from the ledger: outward taxable value and tax by head,
// input tax credit by head, and the net payable.
func BuildGSTR3B(accts []Account, ps []Posting, st Settings, from, to string) TaxReturn {
	out := TaxReturn{Return: "gstr3b", From: from, To: to, GSTIN: st.GSTIN, Rows: []GSTRInvoice{}, Summary: map[string]GSTRTotals{}, Sections: []ReportRow{}, Notes: []string{
		"Built from the ledger. Taxable value is net sales income (returns deducted). ITC set-off between heads is not computed; the payable is shown per head.",
	}}
	am := accountMap(accts)
	var taxable int64
	heads := map[string][2]int64{} // key -> {output, input}
	for _, p := range ps {
		if (from != "" && p.Date < from) || (to != "" && p.Date > to) {
			continue
		}
		a := am[p.AccountID]
		switch {
		case a.Type == "income" && a.Subtype != "other":
			taxable += p.Credit - p.Debit
		case a.Subtype == "gst_output":
			h := heads[headOf(a.SystemKey)]
			h[0] += p.Credit - p.Debit
			heads[headOf(a.SystemKey)] = h
		case a.Subtype == "gst_input":
			h := heads[headOf(a.SystemKey)]
			h[1] += p.Debit - p.Credit
			heads[headOf(a.SystemKey)] = h
		}
	}
	add := func(name string, taxableAmt, tax int64) {
		out.Sections = append(out.Sections, ReportRow{Name: name, Debit: rupees(taxableAmt), Credit: rupees(tax), Amount: rupees(tax)})
	}
	var outTax, itcTax int64
	for _, h := range []string{"igst", "cgst", "sgst"} {
		outTax += heads[h][0]
		itcTax += heads[h][1]
	}
	add("3.1(a) Outward taxable supplies (other than zero rated, nil rated, exempted): taxable value | tax", taxable, outTax)
	for _, h := range []string{"igst", "cgst", "sgst"} {
		add("   Output tax - "+strings.ToUpper(h), 0, heads[h][0])
	}
	add("4(A)(5) All other ITC: total ITC available", 0, itcTax)
	for _, h := range []string{"igst", "cgst", "sgst"} {
		add("   ITC - "+strings.ToUpper(h), 0, heads[h][1])
	}
	for _, h := range []string{"igst", "cgst", "sgst"} {
		add("6.1 Net tax payable - "+strings.ToUpper(h), 0, heads[h][0]-heads[h][1])
	}
	add("6.1 Net tax payable - total (negative = ITC carried forward)", 0, outTax-itcTax)
	return out
}

func headOf(key string) string {
	switch {
	case strings.HasSuffix(key, "igst"):
		return "igst"
	case strings.HasSuffix(key, "cgst"):
		return "cgst"
	case strings.HasSuffix(key, "sgst"):
		return "sgst"
	}
	return "other"
}

// Table renders a return for CSV/XLSX/PDF export.
func (t TaxReturn) Table() Table {
	if t.Return == "gstr3b" {
		tb := Table{Title: "GSTR-3B summary", Subtitle: t.GSTIN + " | " + t.From + " to " + t.To, Headers: []string{"Item", "Taxable value", "Tax"}}
		for _, s := range t.Sections {
			tb.Rows = append(tb.Rows, []any{s.Name, s.Debit, s.Credit})
		}
		return tb
	}
	tb := Table{Title: "GSTR-1 outward supplies", Subtitle: t.GSTIN + " | " + t.From + " to " + t.To,
		Headers: []string{"Section", "GSTIN", "Party", "Invoice", "Date", "POS", "Rate %", "Taxable", "IGST", "CGST", "SGST", "Total"}}
	for _, r := range t.Rows {
		tb.Rows = append(tb.Rows, []any{r.Section, r.GSTIN, r.Party, r.Number, r.Date, r.POS, r.Rate, r.Taxable, r.IGST, r.CGST, r.SGST, r.Total})
	}
	return tb
}

// ---- HSN summary ----

// HSNRow is one HSN/SAC line of the summary.
type HSNRow struct {
	HSN     string  `json:"hsn"`
	Desc    string  `json:"description"`
	UQC     string  `json:"uqc"`
	Qty     float64 `json:"qty"`
	Rate    float64 `json:"rate"`
	Taxable float64 `json:"taxable"`
	IGST    float64 `json:"igst"`
	CGST    float64 `json:"cgst"`
	SGST    float64 `json:"sgst"`
}

// HSNSummary groups outward supplies by HSN/SAC and rate.
type HSNSummary struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Rows    []HSNRow `json:"rows"`
	Missing int      `json:"lines_without_hsn"`
}

// BuildHSNSummary builds the HSN summary from invoice lines.
func (r *Repository) BuildHSNSummary(ctx context.Context, orgID, from, to string, inv InvoiceSource) (HSNSummary, error) {
	out := HSNSummary{From: from, To: to, Rows: []HSNRow{}}
	if inv == nil {
		return out, nil
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return out, err
	}
	parties, hsnMap := r.partyIndex(ctx, orgID), r.hsnIndex(ctx, orgID)
	docs, err := inv.ListDocuments(ctx, orgID, invoicing.DocFilter{Kind: "invoice"})
	if err != nil {
		return out, err
	}
	type key struct {
		hsn  string
		rate float64
	}
	acc := map[key]*HSNRow{}
	for _, d := range docs {
		if d.Status == "draft" || d.Deleted || d.IssueDate < from || d.IssueDate > to {
			continue
		}
		party := parties[d.ContactID]
		inter := !st.Intrastate
		if h, p := stateOf(st.GSTIN, ""), stateOf(party.GSTIN, party.StateCode); h != "" && p != "" {
			inter = h != p
		}
		var sub float64
		nets := make([]float64, len(d.Lines))
		for i, l := range d.Lines {
			nets[i] = l.Qty * l.Price * (1 - math.Min(math.Max(l.DiscountPct, 0), 100)/100)
			sub += nets[i]
		}
		for i, l := range d.Lines {
			share := nets[i]
			if sub > 0 {
				share -= d.DiscountTotal * nets[i] / sub
			}
			h := strings.TrimSpace(l.HSN)
			if h == "" {
				h = strings.TrimSpace(l.HSNSAC) // codes entered on sales documents
			}
			if h == "" {
				h = hsnMap[strings.ToLower(strings.TrimSpace(l.Description))]
			}
			if h == "" {
				out.Missing++
				h = "NA"
			}
			k := key{h, l.TaxRate}
			row := acc[k]
			if row == nil {
				row = &HSNRow{HSN: h, Desc: l.Description, UQC: orDefault(strings.ToUpper(l.Unit), "NOS"), Rate: l.TaxRate}
				acc[k] = row
			}
			row.Qty += l.Qty
			row.Taxable += share
			ig, cg, sg := splitTax(share*l.TaxRate/100, inter || (d.Currency != "" && d.Currency != st.Currency))
			row.IGST += ig
			row.CGST += cg
			row.SGST += sg
		}
	}
	for _, row := range acc {
		row.Taxable, row.IGST, row.CGST, row.SGST = rupees(paise(row.Taxable)), rupees(paise(row.IGST)), rupees(paise(row.CGST)), rupees(paise(row.SGST))
		out.Rows = append(out.Rows, *row)
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		if out.Rows[i].HSN != out.Rows[j].HSN {
			return out.Rows[i].HSN < out.Rows[j].HSN
		}
		return out.Rows[i].Rate < out.Rows[j].Rate
	})
	return out, nil
}

func (h HSNSummary) Table() Table {
	t := Table{Title: "HSN summary of outward supplies", Subtitle: h.From + " to " + h.To, Headers: []string{"HSN/SAC", "Description", "UQC", "Qty", "Rate %", "Taxable", "IGST", "CGST", "SGST"}}
	for _, r := range h.Rows {
		t.Rows = append(t.Rows, []any{r.HSN, r.Desc, r.UQC, r.Qty, r.Rate, r.Taxable, r.IGST, r.CGST, r.SGST})
	}
	return t
}

// ---- TDS ----

// TDSEntry tracks tax deducted at source: payable (we deducted from a vendor) or receivable (a customer deducted from us).
type TDSEntry struct {
	ID        string  `json:"id"`
	Direction string  `json:"direction"` // payable | receivable
	Section   string  `json:"section"`
	PartyID   string  `json:"party_id"`
	PartyName string  `json:"party_name"`
	PAN       string  `json:"pan"`
	Date      string  `json:"date"`
	Base      float64 `json:"base_amount"`
	Rate      float64 `json:"rate"`
	TDS       float64 `json:"tds_amount"`
	Status    string  `json:"status"` // pending | deposited | claimed
	Challan   string  `json:"challan"`
	SettledOn string  `json:"settled_on"`
	Note      string  `json:"note"`
}

// TDSReport lists entries with totals by section.
type TDSReport struct {
	From      string             `json:"from"`
	To        string             `json:"to"`
	Direction string             `json:"direction"`
	Rows      []TDSEntry         `json:"rows"`
	BySection map[string]float64 `json:"by_section"`
	Pending   float64            `json:"pending"`
	Total     float64            `json:"total"`
}

// RecordTDS saves an entry and posts it: payable = Dr Accounts Payable, Cr TDS Payable
// (the vendor is paid net of TDS); receivable = Dr TDS Receivable, Cr Accounts Receivable.
func (r *Repository) RecordTDS(ctx context.Context, orgID, userID string, e TDSEntry) (TDSEntry, error) {
	if e.Direction != "payable" && e.Direction != "receivable" {
		return e, invalid("direction must be payable or receivable")
	}
	d, err := validDate(e.Date)
	if err != nil {
		return e, err
	}
	e.Section = strings.ToUpper(strings.TrimSpace(e.Section))
	if e.Section == "" {
		return e, invalid("TDS section is required")
	}
	if paise(e.Base) <= 0 {
		return e, invalid("base amount must be greater than zero")
	}
	if e.TDS <= 0 {
		if e.Rate <= 0 {
			return e, invalid("give a rate or the TDS amount")
		}
		e.TDS = rupees(int64(float64(paise(e.Base))*e.Rate/100 + 0.5))
	} else if e.Rate <= 0 {
		e.Rate = math.Round(e.TDS/e.Base*10000) / 100
	}
	if e.PAN != "" && len(strings.TrimSpace(e.PAN)) != 10 {
		return e, invalid("PAN must be 10 characters")
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return e, err
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_tds_entries (org_id, direction, section, party_id, party_name, pan, entry_date, base_amount, rate, tds_amount, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7::date,$8,$9,$10,$11) RETURNING id::text`, orgID, e.Direction, e.Section, e.PartyID, strings.TrimSpace(e.PartyName), strings.ToUpper(strings.TrimSpace(e.PAN)),
		d, rupees(paise(e.Base)), e.Rate, rupees(paise(e.TDS)), e.Note).Scan(&e.ID); err != nil {
		return e, err
	}
	memo := fmt.Sprintf("TDS %s %s", e.Section, e.PartyName)
	lines := []Line{{AccountID: keys["ap"], Debit: e.TDS, Memo: memo}, {AccountID: keys["tds_payable"], Credit: e.TDS, Memo: memo}}
	if e.Direction == "receivable" {
		lines = []Line{{AccountID: keys["tds_receivable"], Debit: e.TDS, Memo: memo}, {AccountID: keys["ar"], Credit: e.TDS, Memo: memo}}
	}
	if _, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: d, Memo: memo, Kind: "journal", Source: SourceTDS, SourceID: e.ID, ContactID: e.PartyID, ContactName: e.PartyName,
		Lines: lines, SourceHash: hashOf(e.ID, e.TDS)}); err != nil {
		_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.acct_tds_entries WHERE id=$1::uuid`, e.ID)
		return e, err
	}
	e.Status, e.Date = "pending", d
	r.audit(ctx, orgID, userID, "create", "tds", e.ID, fmt.Sprintf("%s %s %.2f", e.Direction, e.Section, e.TDS))
	return e, nil
}

// SettleTDS marks a payable entry deposited with a challan (Dr TDS Payable, Cr bank) or a
// receivable entry claimed in the return (no ledger movement, tracking only).
func (r *Repository) SettleTDS(ctx context.Context, orgID, userID, id, date, accountID, challan string) (TDSEntry, error) {
	e, err := r.getTDS(ctx, orgID, id)
	if err != nil {
		return e, err
	}
	if e.Status != "pending" {
		return e, invalid("this entry is already %s", e.Status)
	}
	d, err := validDate(date)
	if err != nil {
		return e, err
	}
	if e.Direction == "payable" {
		if !isUUID(accountID) {
			return e, invalid("choose the bank account the TDS was deposited from")
		}
		keys, _ := r.accountKeys(ctx, r.pool, orgID)
		memo := fmt.Sprintf("TDS deposit %s challan %s", e.Section, challan)
		if _, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: d, Memo: memo, Kind: "payment", Source: SourceTDS, SourceID: id + ":deposit", SourceHash: hashOf(id, d, accountID),
			Lines: []Line{{AccountID: keys["tds_payable"], Debit: e.TDS}, {AccountID: accountID, Credit: e.TDS}}}); err != nil {
			return e, err
		}
	}
	status := map[string]string{"payable": "deposited", "receivable": "claimed"}[e.Direction]
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_tds_entries SET status=$3, challan=$4, settled_on=$5::date WHERE org_id=$1 AND id=$2::uuid`, orgID, id, status, challan, d); err != nil {
		return e, err
	}
	r.audit(ctx, orgID, userID, status, "tds", id, challan)
	return r.getTDS(ctx, orgID, id)
}

func (r *Repository) getTDS(ctx context.Context, orgID, id string) (TDSEntry, error) {
	if !isUUID(id) {
		return TDSEntry{}, ErrNotFound
	}
	rows, err := r.queryTDS(ctx, orgID, "", "", "", id)
	if err != nil {
		return TDSEntry{}, err
	}
	if len(rows) == 0 {
		return TDSEntry{}, ErrNotFound
	}
	return rows[0], nil
}

func (r *Repository) queryTDS(ctx context.Context, orgID, direction, from, to, id string) ([]TDSEntry, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, direction, section, party_id, party_name, pan, to_char(entry_date,'YYYY-MM-DD'), base_amount::float8, rate::float8, tds_amount::float8,
		status, challan, coalesce(to_char(settled_on,'YYYY-MM-DD'),''), note FROM adrient.acct_tds_entries
		WHERE org_id=$1 AND ($2='' OR direction=$2) AND ($3='' OR entry_date >= $3::date) AND ($4='' OR entry_date <= $4::date) AND ($5='' OR id=$5::uuid)
		ORDER BY entry_date DESC, created_at DESC`, orgID, direction, from, to, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TDSEntry{}
	for rows.Next() {
		var e TDSEntry
		if err := rows.Scan(&e.ID, &e.Direction, &e.Section, &e.PartyID, &e.PartyName, &e.PAN, &e.Date, &e.Base, &e.Rate, &e.TDS, &e.Status, &e.Challan, &e.SettledOn, &e.Note); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) BuildTDSReport(ctx context.Context, orgID, direction, from, to string) (TDSReport, error) {
	rows, err := r.queryTDS(ctx, orgID, direction, from, to, "")
	if err != nil {
		return TDSReport{}, err
	}
	rep := TDSReport{From: from, To: to, Direction: direction, Rows: rows, BySection: map[string]float64{}}
	var tot, pend int64
	for _, e := range rows {
		rep.BySection[e.Section] = rupees(paise(rep.BySection[e.Section]) + paise(e.TDS))
		tot += paise(e.TDS)
		if e.Status == "pending" {
			pend += paise(e.TDS)
		}
	}
	rep.Total, rep.Pending = rupees(tot), rupees(pend)
	return rep, nil
}

func (t TDSReport) Table() Table {
	tb := Table{Title: "TDS " + orDefault(t.Direction, "all"), Subtitle: t.From + " to " + t.To, Headers: []string{"Date", "Direction", "Section", "Party", "PAN", "Base", "Rate %", "TDS", "Status", "Challan"}}
	for _, e := range t.Rows {
		tb.Rows = append(tb.Rows, []any{e.Date, e.Direction, e.Section, e.PartyName, e.PAN, e.Base, e.Rate, e.TDS, e.Status, e.Challan})
	}
	tb.Rows = append(tb.Rows, []any{"Total", "", "", "", "", "", "", t.Total, "pending " + cellStr(t.Pending), ""})
	return tb
}

// ---- ITC reconciliation against GSTR-2B ----

// ITCRow is one supplier invoice with its books and GSTR-2B figures.
type ITCRow struct {
	GSTIN      string  `json:"gstin"`
	Supplier   string  `json:"supplier"`
	InvoiceNo  string  `json:"invoice_no"`
	Date       string  `json:"date"`
	BooksTax   float64 `json:"books_tax"`
	PortalTax  float64 `json:"portal_tax"`
	Difference float64 `json:"difference"`
	Status     string  `json:"status"` // matched | mismatch | missing_in_2b | missing_in_books
}

// ITCReconciliation compares ITC claimed in books (bills) with GSTR-2B.
type ITCReconciliation struct {
	Period    string   `json:"period"`
	Rows      []ITCRow `json:"rows"`
	Matched   int      `json:"matched"`
	Issues    int      `json:"issues"`
	BooksITC  float64  `json:"books_itc"`
	PortalITC float64  `json:"portal_itc"`
}

func normInv(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return strings.TrimLeft(b.String(), "0")
}

// ImportGSTR2B replaces the stored GSTR-2B rows of a period from CSV:
// header names gstin, supplier, invoice_no, invoice_date, taxable, igst, cgst, sgst.
func (r *Repository) ImportGSTR2B(ctx context.Context, orgID, userID, period, csvText string) (int, error) {
	if len(period) != 7 {
		return 0, invalid("period must be YYYY-MM")
	}
	recs, err := csv.NewReader(strings.NewReader(csvText)).ReadAll()
	if err != nil || len(recs) < 2 {
		return 0, invalid("could not read the CSV, or it has no rows")
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[strings.ToLower(strings.ReplaceAll(strings.TrimSpace(h), " ", "_"))] = i
	}
	for _, need := range []string{"gstin", "invoice_no"} {
		if _, ok := col[need]; !ok {
			return 0, invalid("missing column %q", need)
		}
	}
	get := func(rec []string, k string) string {
		if i, ok := col[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	num := func(s string) float64 { f, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64); return f }
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.acct_gstr2b WHERE org_id=$1 AND period=$2`, orgID, period); err != nil {
		return 0, err
	}
	n := 0
	for _, rec := range recs[1:] {
		if get(rec, "invoice_no") == "" {
			continue
		}
		date := get(rec, "invoice_date")
		if date != "" {
			if date, err = validDate(date); err != nil {
				return 0, invalid("row %d: invoice_date must be YYYY-MM-DD", n+2)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.acct_gstr2b (org_id, period, supplier_gstin, supplier_name, invoice_no, invoice_date, taxable, igst, cgst, sgst)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::date,$7,$8,$9,$10)`, orgID, period, strings.ToUpper(get(rec, "gstin")), get(rec, "supplier"), get(rec, "invoice_no"), date,
			num(get(rec, "taxable")), num(get(rec, "igst")), num(get(rec, "cgst")), num(get(rec, "sgst"))); err != nil {
			return 0, err
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	r.audit(ctx, orgID, userID, "import", "gstr2b", period, fmt.Sprintf("%d rows", n))
	return n, nil
}

// ReconcileITC lines up bills of the month with the uploaded GSTR-2B rows.
func (r *Repository) ReconcileITC(ctx context.Context, orgID, period string) (ITCReconciliation, error) {
	out := ITCReconciliation{Period: period, Rows: []ITCRow{}}
	if len(period) != 7 {
		return out, invalid("period must be YYYY-MM")
	}
	parties := r.partyIndex(ctx, orgID)
	type rec struct {
		row  ITCRow
		seen bool
	}
	books := map[string]*rec{}
	bills, err := r.ListBills(ctx, orgID, "bill", "", "")
	if err != nil {
		return out, err
	}
	for _, b := range bills {
		if b.Status != "open" && b.Status != "paid" || b.BillDate[:7] != period || b.TaxTotal == 0 {
			continue
		}
		p := parties[b.VendorID]
		if p.Name == "" {
			p = parties["name:"+strings.ToLower(b.VendorName)]
		}
		k := p.GSTIN + "|" + normInv(b.Number)
		tax := b.TaxTotal * orDefaultF(b.Rate, 1)
		if ex := books[k]; ex != nil {
			ex.row.BooksTax = rupees(paise(ex.row.BooksTax) + paise(tax))
			continue
		}
		books[k] = &rec{row: ITCRow{GSTIN: p.GSTIN, Supplier: b.VendorName, InvoiceNo: b.Number, Date: b.BillDate, BooksTax: rupees(paise(tax))}}
	}
	rows, err := r.pool.Query(ctx, `SELECT supplier_gstin, supplier_name, invoice_no, coalesce(to_char(invoice_date,'YYYY-MM-DD'),''), (igst+cgst+sgst)::float8
		FROM adrient.acct_gstr2b WHERE org_id=$1 AND period=$2`, orgID, period)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var g, name, inv, date string
		var tax float64
		if err := rows.Scan(&g, &name, &inv, &date, &tax); err != nil {
			return out, err
		}
		k := g + "|" + normInv(inv)
		if b := books[k]; b != nil {
			b.seen = true
			b.row.PortalTax = tax
			continue
		}
		books["portal:"+k] = &rec{seen: true, row: ITCRow{GSTIN: g, Supplier: name, InvoiceNo: inv, Date: date, PortalTax: tax, Status: "missing_in_books"}}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	var bi, pi int64
	for _, b := range books {
		x := b.row
		x.Difference = rupees(paise(x.BooksTax) - paise(x.PortalTax))
		switch {
		case x.Status == "missing_in_books":
		case !b.seen:
			x.Status = "missing_in_2b"
		case math.Abs(x.Difference) <= 1:
			x.Status = "matched"
		default:
			x.Status = "mismatch"
		}
		if x.Status == "matched" {
			out.Matched++
		} else {
			out.Issues++
		}
		bi += paise(x.BooksTax)
		pi += paise(x.PortalTax)
		out.Rows = append(out.Rows, x)
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		if out.Rows[i].Status != out.Rows[j].Status {
			return out.Rows[i].Status < out.Rows[j].Status
		}
		return out.Rows[i].InvoiceNo < out.Rows[j].InvoiceNo
	})
	out.BooksITC, out.PortalITC = rupees(bi), rupees(pi)
	return out, nil
}

func orDefaultF(v, d float64) float64 {
	if v == 0 {
		return d
	}
	return v
}

func (t ITCReconciliation) Table() Table {
	tb := Table{Title: "ITC reconciliation (books vs GSTR-2B)", Subtitle: t.Period, Headers: []string{"Status", "GSTIN", "Supplier", "Invoice", "Date", "Books tax", "GSTR-2B tax", "Difference"}}
	for _, r := range t.Rows {
		tb.Rows = append(tb.Rows, []any{r.Status, r.GSTIN, r.Supplier, r.InvoiceNo, r.Date, r.BooksTax, r.PortalTax, r.Difference})
	}
	tb.Rows = append(tb.Rows, []any{"Total", "", "", "", "", t.BooksITC, t.PortalITC, rupees(paise(t.BooksITC) - paise(t.PortalITC))})
	return tb
}

var _ = errors.Is
var _ pgx.Row
