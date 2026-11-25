package invoicing

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ReminderRule sends one automatic reminder when an invoice is Days days past
// its due date (negative = before the due date).
type ReminderRule struct {
	Days int `json:"days"`
}

// SalesSettings are the sales-side org settings kept in one JSON column.
type SalesSettings struct {
	StateCode         string         `json:"state_code"` // GST state code of the business, e.g. "29"; empty switches GST splitting off
	UPIID             string         `json:"upi_id"`
	UPIName           string         `json:"upi_name"`
	BankDetails       string         `json:"bank_details"`
	PDFAccent         string         `json:"pdf_accent"`
	PDFFooter         string         `json:"pdf_footer"`
	ShowUPIQR         bool           `json:"show_upi_qr"`
	ShowBank          bool           `json:"show_bank"`
	EstimateValidDays int            `json:"estimate_valid_days"`
	OrderPrefix       string         `json:"order_prefix"`
	DeliveryPrefix    string         `json:"delivery_prefix"`
	CreditPrefix      string         `json:"credit_prefix"`
	RemindersAuto     bool           `json:"reminders_auto"`
	ReminderRules     []ReminderRule `json:"reminder_rules"`
	ReminderSubject   string         `json:"reminder_subject"`
	ReminderBody      string         `json:"reminder_body"`
	WhatsAppBody      string         `json:"whatsapp_body"`
	PublicURL         string         `json:"public_url"` // base URL used in links sent by the scheduler (set from the browser)
}

const (
	defReminderSubject = "Payment reminder: invoice {number} from {business}"
	defReminderBody    = "Hello {client},\n\nThis is a friendly reminder that invoice {number} for {amount} {status}.\nYou can view and pay it here: {link}\n\nThank you,\n{business}"
	defWhatsAppBody    = "Hello {client}, invoice {number} for {amount} {status}. View and pay: {link}"
)

func defaultSales() SalesSettings {
	return SalesSettings{
		PDFAccent: "#126C5A", ShowUPIQR: true, ShowBank: true, EstimateValidDays: 30,
		OrderPrefix: "SO-", DeliveryPrefix: "DN-", CreditPrefix: "CN-",
		ReminderRules:   []ReminderRule{{Days: -3}, {Days: 1}, {Days: 7}, {Days: 14}},
		ReminderSubject: defReminderSubject, ReminderBody: defReminderBody, WhatsAppBody: defWhatsAppBody,
	}
}

func parseSalesSettings(b []byte) SalesSettings {
	s := defaultSales()
	if len(b) > 2 {
		_ = json.Unmarshal(b, &s)
	}
	return normalizeSales(s)
}

func normalizeSales(s SalesSettings) SalesSettings {
	d := defaultSales()
	if s.PDFAccent == "" {
		s.PDFAccent = d.PDFAccent
	}
	if s.EstimateValidDays <= 0 {
		s.EstimateValidDays = d.EstimateValidDays
	}
	for _, p := range [][2]*string{{&s.OrderPrefix, &d.OrderPrefix}, {&s.DeliveryPrefix, &d.DeliveryPrefix}, {&s.CreditPrefix, &d.CreditPrefix},
		{&s.ReminderSubject, &d.ReminderSubject}, {&s.ReminderBody, &d.ReminderBody}, {&s.WhatsAppBody, &d.WhatsAppBody}} {
		if strings.TrimSpace(*p[0]) == "" {
			*p[0] = *p[1]
		}
	}
	if s.ReminderRules == nil {
		s.ReminderRules = d.ReminderRules
	}
	s.StateCode = StateCode(s.StateCode)
	return s
}

func marshalSalesSettings(s SalesSettings) ([]byte, error) {
	s = normalizeSales(s)
	if len(s.ReminderRules) > 12 {
		return nil, invalid("at most 12 reminder rules")
	}
	seen := map[int]bool{}
	for _, r := range s.ReminderRules {
		if r.Days < -60 || r.Days > 365 {
			return nil, invalid("reminder days must be between -60 and 365")
		}
		if seen[r.Days] {
			return nil, invalid("duplicate reminder rule for %d days", r.Days)
		}
		seen[r.Days] = true
	}
	return json.Marshal(s)
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var gstinRe = regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)

// ValidGSTIN reports whether v has the shape of a GSTIN (the checksum is not verified).
func ValidGSTIN(v string) bool { return gstinRe.MatchString(strings.ToUpper(strings.TrimSpace(v))) }

// nextSeq returns the next number of a named per-org counter.
func nextSeq(ctx context.Context, tx pgx.Tx, orgID, name string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `INSERT INTO adrient.invc_sequences (org_id, name, next_val) VALUES ($1,$2,2)
		ON CONFLICT (org_id, name) DO UPDATE SET next_val = adrient.invc_sequences.next_val + 1
		RETURNING next_val - 1`, orgID, name).Scan(&n)
	return n, err
}

// applyGST fills the GST-related input fields and, when the business has a GST
// state configured, replaces the tax total by the per-line rounded GST total so
// the CGST/SGST/IGST split always adds up to the paisa. It returns the
// breakup, or nil when GST splitting is off.
func applyGST(in *DocInput, t *Totals, s Settings) *Breakup {
	in.ClientGSTIN = strings.ToUpper(strings.TrimSpace(in.ClientGSTIN))
	if in.ValidUntil == "" && in.Kind == "estimate" && in.IssueDate != "" {
		if d := parseDate(in.IssueDate); d != nil {
			in.ValidUntil = d.AddDate(0, 0, s.Sales.EstimateValidDays).Format("2006-01-02")
		}
	}
	supplier := StateCode(s.Sales.StateCode)
	pos := StateCode(in.PlaceOfSupply)
	if pos == "" && len(in.ClientGSTIN) >= 2 {
		pos = StateCode(in.ClientGSTIN[:2])
	}
	if pos == "" {
		pos = supplier
	}
	in.PlaceOfSupply = pos
	if supplier == "" {
		return nil
	}
	b := GST(in.Lines, in.DiscountKind, in.DiscountValue, supplier, pos)
	t.TaxTotal = b.TaxTotal
	t.Total = Round2(t.Subtotal - t.DiscountTotal + t.TaxTotal)
	return &b
}

func validateExtInput(in *DocInput) error {
	if in.PlaceOfSupply != "" && StateCode(in.PlaceOfSupply) == "" {
		return invalid("place of supply must be a GST state code or state name")
	}
	if g := strings.ToUpper(strings.TrimSpace(in.ClientGSTIN)); g != "" && !ValidGSTIN(g) {
		return invalid("client GSTIN %q is not a valid GSTIN", g)
	}
	if in.ValidUntil != "" && parseDate(in.ValidUntil) == nil {
		return invalid("valid until must be YYYY-MM-DD")
	}
	for _, id := range []string{in.PriceListID, in.SalesOrderID, in.CreditFor, in.SubscriptionID} {
		if id != "" && len(id) != 36 {
			return invalid("invalid id")
		}
	}
	return nil
}

// writeExt stores the sales columns of a document next to its core columns.
func (r *Repository) writeExt(ctx context.Context, x execer, orgID, id string, in DocInput, b *Breakup, insert bool) error {
	var cg, sg, ig float64
	supplier := ""
	if b != nil {
		supplier = b.Supplier
		cg, sg, ig = b.CGST, b.SGST, b.IGST
	}
	if _, err := x.Exec(ctx, `UPDATE adrient.invc_documents SET valid_until=NULLIF($3,'')::date, place_of_supply=$4, client_gstin=$5,
		cgst_total=$6, sgst_total=$7, igst_total=$8, supplier_state=$9, price_list_id=NULLIF($10,'')::uuid,
		source_ref=CASE WHEN $11 <> '' THEN $11 ELSE source_ref END
		WHERE org_id=$1 AND id=$2::uuid`, orgID, id, in.ValidUntil, in.PlaceOfSupply, in.ClientGSTIN, cg, sg, ig,
		supplier, in.PriceListID, in.SourceRef); err != nil {
		return err
	}
	if !insert {
		return nil
	}
	ver := in.Version
	if ver < 1 {
		ver = 1
	}
	_, err := x.Exec(ctx, `UPDATE adrient.invc_documents SET version=$3, version_group=COALESCE(NULLIF($4,'')::uuid, id),
		sales_order_id=NULLIF($5,'')::uuid, credit_for=NULLIF($6,'')::uuid, subscription_id=NULLIF($7,'')::uuid,
		recurring_id=NULLIF($8,'')::uuid, recurring_period=NULLIF($9,'')::date, stock_deducted=$10
		WHERE org_id=$1 AND id=$2::uuid`, orgID, id, ver, in.VersionGroup, in.SalesOrderID, in.CreditFor,
		in.SubscriptionID, in.RecurringID, in.RecurringDate, in.NoStock)
	return err
}

const extCols = `id::text, COALESCE(to_char(valid_until,'YYYY-MM-DD'),''), version, COALESCE(version_group::text,''),
	COALESCE(to_char(accepted_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''), accepted_name, accepted_hash, declined_reason,
	COALESCE(sales_order_id::text,''), COALESCE(credit_for::text,''), place_of_supply, supplier_state, client_gstin,
	cgst_total::float8, sgst_total::float8, igst_total::float8, COALESCE(price_list_id::text,''), source_ref,
	COALESCE(subscription_id::text,''), payment_link_url`

// loadExt fills the sales columns of the given documents (same order, by id).
func (r *Repository) loadExt(ctx context.Context, orgID string, docs []Document) error {
	ids := make([]string, len(docs))
	idx := map[string]int{}
	for i, d := range docs {
		ids[i] = d.ID
		idx[d.ID] = i
	}
	rows, err := r.pool.Query(ctx, `SELECT `+extCols+` FROM adrient.invc_documents WHERE org_id=$1 AND id = ANY($2::uuid[])`, orgID, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var d Document
		if err := rows.Scan(&id, &d.ValidUntil, &d.Version, &d.VersionGroup, &d.AcceptedAt, &d.AcceptedName, &d.AcceptedHash,
			&d.DeclinedReason, &d.SalesOrderID, &d.CreditFor, &d.PlaceOfSupply, &d.SupplierState, &d.ClientGSTIN,
			&d.CGST, &d.SGST, &d.IGST, &d.PriceListID, &d.SourceRef, &d.SubscriptionID, &d.PaymentLinkURL); err != nil {
			return err
		}
		t := &docs[idx[id]]
		t.ValidUntil, t.Version, t.VersionGroup, t.AcceptedAt, t.AcceptedName, t.AcceptedHash = d.ValidUntil, d.Version, d.VersionGroup, d.AcceptedAt, d.AcceptedName, d.AcceptedHash
		t.DeclinedReason, t.SalesOrderID, t.CreditFor, t.PlaceOfSupply, t.SupplierState, t.ClientGSTIN = d.DeclinedReason, d.SalesOrderID, d.CreditFor, d.PlaceOfSupply, d.SupplierState, d.ClientGSTIN
		t.CGST, t.SGST, t.IGST, t.PriceListID, t.SourceRef, t.SubscriptionID, t.PaymentLinkURL = d.CGST, d.SGST, d.IGST, d.PriceListID, d.SourceRef, d.SubscriptionID, d.PaymentLinkURL
	}
	return rows.Err()
}
