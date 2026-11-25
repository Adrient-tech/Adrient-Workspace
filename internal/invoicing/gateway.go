package invoicing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrGatewayNotConfigured is returned when no Razorpay keys exist.
var ErrGatewayNotConfigured = errors.New("payment gateway not configured")

// Gateway holds Razorpay credentials. Env vars RAZORPAY_KEY_ID and
// RAZORPAY_KEY_SECRET (and RAZORPAY_WEBHOOK_SECRET) win over the keys saved in
// the admin settings; nothing is ever hardcoded.
type Gateway struct {
	KeyID         string
	KeySecret     string
	WebhookSecret string
	Source        string // env | settings | none
}

// Configured reports whether payment links can be created.
func (g Gateway) Configured() bool { return g.KeyID != "" && g.KeySecret != "" }

// GatewayStatus is what the API returns: never the secrets.
type GatewayStatus struct {
	Configured    bool   `json:"configured"`
	Source        string `json:"source"`
	KeyID         string `json:"key_id"`          // masked
	WebhookSecret bool   `json:"webhook_secret"` // whether one is set
	Mode          string `json:"mode"`            // test | live | ""
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return strings.Repeat("*", len(k))
	}
	return k[:8] + strings.Repeat("*", 4)
}

// GetGateway resolves the credentials for an org.
func (r *Repository) GetGateway(ctx context.Context, orgID string) (Gateway, error) {
	g := Gateway{Source: "none"}
	var id, sec, wh string
	err := r.pool.QueryRow(ctx, `SELECT razorpay_key_id, razorpay_key_secret, razorpay_webhook_secret FROM adrient.invc_gateway WHERE org_id=$1`, orgID).Scan(&id, &sec, &wh)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return g, err
	}
	if id != "" && sec != "" {
		g = Gateway{KeyID: id, KeySecret: sec, WebhookSecret: wh, Source: "settings"}
	}
	if eid, esec := os.Getenv("RAZORPAY_KEY_ID"), os.Getenv("RAZORPAY_KEY_SECRET"); eid != "" && esec != "" {
		g = Gateway{KeyID: eid, KeySecret: esec, WebhookSecret: os.Getenv("RAZORPAY_WEBHOOK_SECRET"), Source: "env"}
		if g.WebhookSecret == "" {
			g.WebhookSecret = wh
		}
	} else if g.WebhookSecret == "" {
		g.WebhookSecret = os.Getenv("RAZORPAY_WEBHOOK_SECRET")
	}
	return g, nil
}

// GatewayStatus describes the gateway without leaking secrets.
func (r *Repository) GatewayStatus(ctx context.Context, orgID string) (GatewayStatus, error) {
	g, err := r.GetGateway(ctx, orgID)
	if err != nil {
		return GatewayStatus{}, err
	}
	st := GatewayStatus{Configured: g.Configured(), Source: g.Source, KeyID: maskKey(g.KeyID), WebhookSecret: g.WebhookSecret != ""}
	switch {
	case strings.HasPrefix(g.KeyID, "rzp_test_"):
		st.Mode = "test"
	case strings.HasPrefix(g.KeyID, "rzp_live_"):
		st.Mode = "live"
	}
	return st, nil
}

// SaveGateway stores keys entered by an admin. An empty secret keeps the saved one.
func (r *Repository) SaveGateway(ctx context.Context, orgID, keyID, secret, webhook string) error {
	keyID, secret, webhook = strings.TrimSpace(keyID), strings.TrimSpace(secret), strings.TrimSpace(webhook)
	if keyID != "" && !strings.HasPrefix(keyID, "rzp_") {
		return invalid("a Razorpay key id starts with rzp_test_ or rzp_live_")
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.invc_gateway (org_id, razorpay_key_id, razorpay_key_secret, razorpay_webhook_secret)
		VALUES ($1,$2,$3,$4) ON CONFLICT (org_id) DO UPDATE SET razorpay_key_id=EXCLUDED.razorpay_key_id,
		razorpay_key_secret=CASE WHEN EXCLUDED.razorpay_key_secret <> '' THEN EXCLUDED.razorpay_key_secret ELSE adrient.invc_gateway.razorpay_key_secret END,
		razorpay_webhook_secret=CASE WHEN EXCLUDED.razorpay_webhook_secret <> '' THEN EXCLUDED.razorpay_webhook_secret ELSE adrient.invc_gateway.razorpay_webhook_secret END,
		updated_at=now()`, orgID, keyID, secret, webhook)
	return err
}

func razorpayBase() string {
	if b := os.Getenv("RAZORPAY_BASE_URL"); b != "" {
		return strings.TrimRight(b, "/")
	}
	return "https://api.razorpay.com"
}

// RazorpayPaymentLink is the part of the Razorpay response we use.
type RazorpayPaymentLink struct {
	ID       string `json:"id"`
	ShortURL string `json:"short_url"`
}

// CreatePaymentLink creates (or reuses) a Razorpay payment link for the balance
// of an invoice. The link accepts partial payments. The amount is sent in paise.
func (r *Repository) CreatePaymentLink(ctx context.Context, orgID string, d Document, callbackURL string) (string, error) {
	g, err := r.GetGateway(ctx, orgID)
	if err != nil {
		return "", err
	}
	if !g.Configured() {
		return "", ErrGatewayNotConfigured
	}
	if d.Kind != "invoice" || d.Status == "draft" || d.Balance <= 0.004 {
		return "", invalid("a payment link needs a sent invoice with a balance")
	}
	if d.Currency != "INR" {
		return "", invalid("Razorpay payment links are created in INR (this invoice is %s)", d.Currency)
	}
	if d.PaymentLinkURL != "" {
		return d.PaymentLinkURL, nil
	}
	body := map[string]any{
		"amount": int64(Round2(d.Balance)*100 + 0.5), "currency": "INR", "accept_partial": true, "first_min_partial_amount": 100,
		"reference_id": d.Number, "description": "Invoice " + d.Number,
		"customer": map[string]any{"name": d.ClientName, "email": d.ClientEmail},
		"notify":   map[string]any{"email": false, "sms": false}, "reminder_enable": false,
		"notes": map[string]any{"invoice_id": d.ID, "org_id": orgID},
	}
	if callbackURL != "" {
		body["callback_url"], body["callback_method"] = callbackURL, "get"
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, razorpayBase()+"/v1/payment_links", bytes.NewReader(b))
	req.SetBasicAuth(g.KeyID, g.KeySecret)
	req.Header.Set("Content-Type", "application/json")
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("razorpay unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("razorpay said %d: %s", resp.StatusCode, trunc(string(raw), 300))
	}
	var pl RazorpayPaymentLink
	if err := json.Unmarshal(raw, &pl); err != nil || pl.ShortURL == "" {
		return "", fmt.Errorf("razorpay returned an unexpected answer")
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET payment_link_id=$3, payment_link_url=$4 WHERE org_id=$1 AND id=$2::uuid`, orgID, d.ID, pl.ID, pl.ShortURL); err != nil {
		return "", err
	}
	return pl.ShortURL, nil
}

// VerifyRazorpaySignature checks X-Razorpay-Signature: hex HMAC-SHA256 of the raw body.
func VerifyRazorpaySignature(body []byte, signature, secret string) bool {
	if secret == "" || signature == "" {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	want := hex.EncodeToString(m.Sum(nil))
	return hmac.Equal([]byte(want), []byte(strings.ToLower(strings.TrimSpace(signature))))
}

type razorpayEvent struct {
	Event   string `json:"event"`
	Payload struct {
		PaymentLink struct {
			Entity struct {
				ID          string `json:"id"`
				ReferenceID string `json:"reference_id"`
			} `json:"entity"`
		} `json:"payment_link"`
		Payment struct {
			Entity struct {
				ID       string `json:"id"`
				Amount   int64  `json:"amount"`
				Method   string `json:"method"`
				Currency string `json:"currency"`
			} `json:"entity"`
		} `json:"payment"`
	} `json:"payload"`
}

// HandleRazorpayWebhook verifies and applies a payment_link.paid / partially_paid
// event. It records the payment once (the unique index on method+reference makes
// a repeated delivery harmless) and reports whether a row was added.
func (r *Repository) HandleRazorpayWebhook(ctx context.Context, body []byte, signature string) (recorded bool, err error) {
	var ev razorpayEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return false, invalid("bad payload")
	}
	linkID := ev.Payload.PaymentLink.Entity.ID
	if linkID == "" {
		return false, nil // not a payment-link event
	}
	var orgID, docID string
	err = r.pool.QueryRow(ctx, `SELECT org_id::text, id::text FROM adrient.invc_documents WHERE payment_link_id=$1`, linkID).Scan(&orgID, &docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	g, err := r.GetGateway(ctx, orgID)
	if err != nil {
		return false, err
	}
	if !VerifyRazorpaySignature(body, signature, g.WebhookSecret) {
		return false, errors.New("bad signature")
	}
	if ev.Event != "payment_link.paid" && ev.Event != "payment_link.partially_paid" {
		return false, nil
	}
	pay := ev.Payload.Payment.Entity
	if pay.ID == "" || pay.Amount <= 0 {
		return false, invalid("no payment in event")
	}
	amount := Round2(float64(pay.Amount) / 100)
	tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.invc_payments (org_id, document_id, kind, amount, method, reference, note, paid_at)
		VALUES ($1,$2::uuid,'payment',$3,'razorpay',$4,$5,CURRENT_DATE) ON CONFLICT DO NOTHING`, orgID, docID, amount, pay.ID, "Razorpay "+pay.Method)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
