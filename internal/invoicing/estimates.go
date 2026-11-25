package invoicing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var versionSuffix = regexp.MustCompile(`-v\d+$`)

// NewVersion creates the next version of an estimate as a draft and marks the
// current one superseded. Every version keeps its own share link; the old link
// shows that it was replaced and points at the latest version.
func (r *Repository) NewVersion(ctx context.Context, orgID, userID, id string) (Document, error) {
	d, err := r.GetDocument(ctx, orgID, id)
	if err != nil {
		return d, err
	}
	if d.Kind != "estimate" || d.Deleted {
		return d, invalid("only a live estimate can have a new version")
	}
	if d.Status == "converted" || d.Status == "superseded" {
		return d, invalid("a %s estimate cannot be revised", d.Status)
	}
	in := inputOf(d)
	in.Number = versionSuffix.ReplaceAllString(d.Number, "") + "-v" + strconv.Itoa(d.Version+1)
	in.IssueDate, in.ValidUntil = "", ""
	in.PlaceOfSupply, in.ClientGSTIN, in.PriceListID, in.SourceRef = d.PlaceOfSupply, d.ClientGSTIN, d.PriceListID, d.SourceRef
	in.VersionGroup, in.Version = d.VersionGroup, d.Version+1
	if in.VersionGroup == "" {
		in.VersionGroup = d.ID
	}
	nd, err := r.SaveDocument(ctx, orgID, userID, "", in)
	if err != nil {
		return nd, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='superseded', version_group=$3::uuid, updated_at=now()
		WHERE org_id=$1 AND id=$2::uuid`, orgID, id, in.VersionGroup); err != nil {
		return nd, err
	}
	return r.GetDocument(ctx, orgID, nd.ID)
}

// Versions lists every version of the estimate's family, oldest first.
func (r *Repository) Versions(ctx context.Context, orgID, id string) ([]Document, error) {
	d, err := r.GetDocument(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	g := d.VersionGroup
	if g == "" {
		g = d.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM adrient.invc_documents WHERE org_id=$1 AND version_group=$2::uuid
		AND deleted_at IS NULL ORDER BY version`, orgID, g)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var x string
		if err := rows.Scan(&x); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, x)
	}
	rows.Close()
	out := make([]Document, 0, len(ids))
	for _, x := range ids {
		v, err := r.GetDocument(ctx, orgID, x)
		if err != nil {
			return nil, err
		}
		v.ShareToken, v.Payments = "", nil
		out = append(out, v)
	}
	return out, nil
}

// Acceptance is the evidence recorded when a customer accepts an estimate.
type Acceptance struct {
	Name  string `json:"name"`
	Agree bool   `json:"agree"`
	IP    string `json:"-"`
	UA    string `json:"-"`
}

// acceptanceHash fingerprints what was accepted: the document number, version,
// total and lines together with who accepted it and when. It lets anyone check
// later that the estimate was not changed after acceptance.
func acceptanceHash(d Document, name string, at time.Time) string {
	lines, _ := json.Marshal(d.Lines)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%.2f|%s|%s|%s", d.Number, d.Version, d.Total, lines, name, at.UTC().Format(time.RFC3339))))
	return hex.EncodeToString(sum[:])
}

// AcceptByToken records a customer's acceptance on the public link. The
// estimate must be sent, not expired and not superseded.
func (r *Repository) AcceptByToken(ctx context.Context, token string, a Acceptance) (Document, error) {
	a.Name = strings.TrimSpace(a.Name)
	if len(a.Name) < 2 || len(a.Name) > 120 {
		return Document{}, invalid("type your full name to accept")
	}
	if !a.Agree {
		return Document{}, invalid("tick the box to confirm you accept this estimate")
	}
	d, orgID, err := r.byToken(ctx, token)
	if err != nil {
		return d, err
	}
	if d.Kind != "estimate" {
		return d, invalid("only an estimate can be accepted")
	}
	if d.Status == "accepted" {
		return d, nil // idempotent: a double click or a retry changes nothing
	}
	if d.Status != "sent" {
		return d, invalid("this estimate is %s and can no longer be accepted", d.DisplayStatus)
	}
	if d.DisplayStatus == "expired" {
		return d, invalid("this estimate expired on %s", d.ValidUntil)
	}
	now := time.Now()
	h := acceptanceHash(d, a.Name, now)
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='accepted', accepted_at=$3, accepted_name=$4, accepted_ip=$5,
		accepted_ua=$6, accepted_hash=$7, updated_at=now() WHERE org_id=$1 AND id=$2::uuid AND status='sent'`,
		orgID, d.ID, now, a.Name, a.IP, trunc(a.UA, 300), h)
	if err != nil {
		return d, err
	}
	if tag.RowsAffected() == 0 {
		return d, invalid("this estimate changed, reload the page")
	}
	return r.GetDocument(ctx, orgID, d.ID)
}

// DeclineByToken records a customer's decline with an optional reason.
func (r *Repository) DeclineByToken(ctx context.Context, token, reason string) (Document, error) {
	d, orgID, err := r.byToken(ctx, token)
	if err != nil {
		return d, err
	}
	if d.Kind != "estimate" {
		return d, invalid("only an estimate can be declined")
	}
	if d.Status == "declined" {
		return d, nil
	}
	if d.Status != "sent" {
		return d, invalid("this estimate is %s and can no longer be declined", d.DisplayStatus)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='declined', declined_reason=$3, updated_at=now()
		WHERE org_id=$1 AND id=$2::uuid AND status='sent'`, orgID, d.ID, trunc(strings.TrimSpace(reason), 500)); err != nil {
		return d, err
	}
	return r.GetDocument(ctx, orgID, d.ID)
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// byToken loads a live, non-draft document by its public share token.
func (r *Repository) byToken(ctx context.Context, token string) (Document, string, error) {
	var orgID string
	err := r.pool.QueryRow(ctx, `SELECT org_id::text FROM adrient.invc_documents WHERE share_token=$1 AND deleted_at IS NULL AND status <> 'draft'`, token).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, "", ErrNotFound
	}
	if err != nil {
		return Document{}, "", err
	}
	d, err := scanDoc(r.pool.QueryRow(ctx, `SELECT `+docCols+` FROM adrient.invc_documents WHERE share_token=$1`, token))
	if err != nil {
		return d, orgID, err
	}
	d, err = r.finish(ctx, orgID, d)
	return d, orgID, err
}

// ConvertEstimateTo creates the invoice from an estimate (same as the older
// ConvertEstimate but it carries GST fields and refuses declined/superseded
// estimates). Converting twice returns the invoice that already exists.
func (r *Repository) ConvertEstimateTo(ctx context.Context, orgID, userID, id string) (Document, error) {
	d, err := r.GetDocument(ctx, orgID, id)
	if err != nil {
		return d, err
	}
	if d.Kind != "estimate" {
		return d, invalid("only an estimate can be converted")
	}
	if d.Status == "declined" || d.Status == "superseded" {
		return d, invalid("a %s estimate cannot be converted", d.Status)
	}
	if d.Status == "converted" {
		var inv string
		if err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.invc_documents WHERE org_id=$1 AND converted_from=$2::uuid AND kind='invoice'
			ORDER BY created_at LIMIT 1`, orgID, id).Scan(&inv); err == nil {
			return r.GetDocument(ctx, orgID, inv)
		}
		return d, invalid("estimate was already converted")
	}
	in := inputOf(d)
	in.Kind, in.IssueDate, in.DueDate, in.ValidUntil = "invoice", "", "", ""
	inv, err := r.SaveDocument(ctx, orgID, userID, "", in)
	if err != nil {
		return inv, err
	}
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET converted_from=$3::uuid WHERE org_id=$1 AND id=$2::uuid`, orgID, inv.ID, id)
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='converted' WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	return r.GetDocument(ctx, orgID, inv.ID)
}

// DealEstimate is what the CRM passes when it creates an estimate from a deal.
type DealEstimate struct {
	DealID      string  `json:"deal_id"`
	Title       string  `json:"title"`
	ContactID   string  `json:"contact_id"`
	ClientName  string  `json:"client_name"`
	ClientEmail string  `json:"client_email"`
	ClientAddr  string  `json:"client_address"`
	ClientGSTIN string  `json:"client_gstin"`
	Currency    string  `json:"currency"`
	Value       float64 `json:"value"`
	TaxRate     float64 `json:"tax_rate"`
	Notes       string  `json:"notes"`
	Lines       []Line  `json:"lines"` // optional; when empty one line "Title" for Value is created
}

// CreateEstimateFromDeal creates a draft estimate for a CRM deal. It is
// idempotent per deal: if a live estimate for the deal exists it is returned
// (created=false), so a double click in the CRM never makes two quotes.
// Source: Go callers use this directly, REST callers POST
// /api/v1/invoicing/estimates/from-deal.
func (r *Repository) CreateEstimateFromDeal(ctx context.Context, orgID, userID string, in DealEstimate) (doc Document, created bool, err error) {
	in.DealID = strings.TrimSpace(in.DealID)
	if in.DealID == "" {
		return doc, false, invalid("deal_id is required")
	}
	ref := "crm:deal:" + in.DealID
	var existing string
	err = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.invc_documents WHERE org_id=$1 AND kind='estimate' AND source_ref=$2
		AND deleted_at IS NULL AND status NOT IN ('superseded','declined') ORDER BY created_at DESC LIMIT 1`, orgID, ref).Scan(&existing)
	if err == nil {
		doc, err = r.GetDocument(ctx, orgID, existing)
		return doc, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return doc, false, err
	}
	lines := in.Lines
	if len(lines) == 0 {
		if in.Value < 0 {
			return doc, false, invalid("deal value must not be negative")
		}
		title := strings.TrimSpace(in.Title)
		if title == "" {
			title = "Services"
		}
		lines = []Line{{Description: title, Qty: 1, Unit: "pcs", Price: in.Value, TaxRate: in.TaxRate}}
	}
	doc, err = r.SaveDocument(ctx, orgID, userID, "", DocInput{
		Kind: "estimate", ContactID: in.ContactID, ClientName: in.ClientName, ClientEmail: in.ClientEmail,
		ClientAddress: in.ClientAddr, ClientGSTIN: in.ClientGSTIN, Currency: in.Currency, Lines: lines,
		Notes: in.Notes, SourceRef: ref,
	})
	return doc, err == nil, err
}
