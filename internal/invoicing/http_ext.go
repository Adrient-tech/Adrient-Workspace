package invoicing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
)

// serveExt handles the sales routes added in wave 3. It returns false when the
// path is not one of them.
func (h *HTTPHandler) serveExt(w http.ResponseWriter, r *http.Request, orgID, userID string, seg []string) bool {
	ctx := r.Context()
	m := r.Method
	q := r.URL.Query()
	n := len(seg)
	ok := true
	switch {
	// ---- estimates ----
	case n == 3 && seg[0] == "documents" && seg[2] == "version" && m == http.MethodPost:
		reply(w)(h.repo.NewVersion(ctx, orgID, userID, seg[1]))
	case n == 3 && seg[0] == "documents" && seg[2] == "versions" && m == http.MethodGet:
		v, err := h.repo.Versions(ctx, orgID, seg[1])
		replyKey(w, "versions", v, err)
	case n == 3 && seg[0] == "documents" && seg[2] == "order" && m == http.MethodPost:
		reply(w)(h.repo.OrderFromEstimate(ctx, orgID, userID, seg[1]))
	case n == 2 && seg[0] == "estimates" && seg[1] == "from-deal" && m == http.MethodPost:
		var in DealEstimate
		if !decode(w, r, &in) {
			return true
		}
		d, created, err := h.repo.CreateEstimateFromDeal(ctx, orgID, userID, in)
		if err != nil {
			httpErr(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"document": d, "created": created})

	// ---- credit notes ----
	case n == 3 && seg[0] == "documents" && seg[2] == "credit-note" && m == http.MethodPost:
		var in CreditInput
		if !decode(w, r, &in) {
			return true
		}
		reply(w)(h.repo.CreateCreditNote(ctx, orgID, userID, seg[1], in))
	case n == 3 && seg[0] == "documents" && seg[2] == "apply" && m == http.MethodPost:
		var b struct {
			InvoiceID string  `json:"invoice_id"`
			Amount    float64 `json:"amount"`
		}
		if !decode(w, r, &b) {
			return true
		}
		reply(w)(h.repo.ApplyCreditNote(ctx, orgID, userID, seg[1], b.InvoiceID, b.Amount))
	case n == 3 && seg[0] == "documents" && seg[2] == "refund" && m == http.MethodPost:
		var b struct {
			Amount    float64 `json:"amount"`
			Method    string  `json:"method"`
			Reference string  `json:"reference"`
		}
		if !decode(w, r, &b) {
			return true
		}
		reply(w)(h.repo.RefundCreditNote(ctx, orgID, userID, seg[1], b.Amount, b.Method, b.Reference))

	// ---- per-document tools ----
	case n == 3 && seg[0] == "documents" && seg[2] == "payment-link" && m == http.MethodPost:
		d, err := h.repo.GetDocument(ctx, orgID, seg[1])
		if err != nil {
			httpErr(w, err)
			return true
		}
		url, err := h.repo.CreatePaymentLink(ctx, orgID, d, "")
		switch {
		case errors.Is(err, ErrGatewayNotConfigured):
			http.Error(w, "payment_gateway_not_configured", http.StatusConflict)
		case err != nil && !errors.Is(err, ErrInvalid):
			slog.Warn("invoicing: payment link", "err", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
		case err != nil:
			httpErr(w, err)
		default:
			writeJSON(w, http.StatusOK, map[string]any{"url": url})
		}
	case n == 3 && seg[0] == "documents" && seg[2] == "einvoice" && m == http.MethodGet:
		d, err := h.repo.GetDocument(ctx, orgID, seg[1])
		if err != nil {
			httpErr(w, err)
			return true
		}
		s, err := h.repo.GetSettings(ctx, orgID)
		if err != nil {
			httpErr(w, err)
			return true
		}
		doc, issues := EInvoice(d, s)
		writeJSON(w, http.StatusOK, map[string]any{"ready": len(issues) == 0, "issues": issues, "json": doc})
	case n == 3 && seg[0] == "documents" && seg[2] == "whatsapp" && m == http.MethodGet:
		d, err := h.repo.GetDocument(ctx, orgID, seg[1])
		if err != nil {
			httpErr(w, err)
			return true
		}
		s, _ := h.repo.GetSettings(ctx, orgID)
		vars := ReminderVars(d, s, q.Get("origin"), today())
		text := RenderTemplate(s.Sales.WhatsAppBody, vars)
		writeJSON(w, http.StatusOK, map[string]any{"text": text, "url": WhatsAppLink(q.Get("phone"), text)})
	case n == 2 && seg[0] == "documents" && seg[1] == "bulk" && m == http.MethodPost:
		h.bulk(w, r, orgID, userID)

	// ---- gateway ----
	case n == 1 && seg[0] == "gateway" && m == http.MethodGet:
		reply(w)(h.repo.GatewayStatus(ctx, orgID))
	case n == 1 && seg[0] == "gateway" && m == http.MethodPut:
		var b struct {
			KeyID         string `json:"key_id"`
			KeySecret     string `json:"key_secret"`
			WebhookSecret string `json:"webhook_secret"`
		}
		if !decode(w, r, &b) {
			return true
		}
		if err := h.repo.SaveGateway(ctx, orgID, b.KeyID, b.KeySecret, b.WebhookSecret); err != nil {
			httpErr(w, err)
			return true
		}
		reply(w)(h.repo.GatewayStatus(ctx, orgID))

	// ---- price lists and discount rules ----
	case n == 1 && seg[0] == "price-lists" && m == http.MethodGet:
		l, err := h.repo.ListPriceLists(ctx, orgID)
		replyKey(w, "price_lists", l, err)
	case n == 1 && seg[0] == "price-lists" && m == http.MethodPost:
		var p PriceList
		if !decode(w, r, &p) {
			return true
		}
		reply(w)(h.repo.SavePriceList(ctx, orgID, "", p))
	case n == 2 && seg[0] == "price-lists" && m == http.MethodPut:
		var p PriceList
		if !decode(w, r, &p) {
			return true
		}
		reply(w)(h.repo.SavePriceList(ctx, orgID, seg[1], p))
	case n == 2 && seg[0] == "price-lists" && m == http.MethodDelete:
		replyOK(w, h.repo.DeletePriceList(ctx, orgID, seg[1]))
	case n == 1 && seg[0] == "discount-rules" && m == http.MethodGet:
		l, err := h.repo.ListDiscountRules(ctx, orgID)
		replyKey(w, "rules", l, err)
	case n == 1 && seg[0] == "discount-rules" && m == http.MethodPost:
		var d DiscountRule
		if !decode(w, r, &d) {
			return true
		}
		reply(w)(h.repo.SaveDiscountRule(ctx, orgID, "", d))
	case n == 2 && seg[0] == "discount-rules" && m == http.MethodPut:
		var d DiscountRule
		if !decode(w, r, &d) {
			return true
		}
		reply(w)(h.repo.SaveDiscountRule(ctx, orgID, seg[1], d))
	case n == 2 && seg[0] == "discount-rules" && m == http.MethodDelete:
		replyOK(w, h.repo.DeleteDiscountRule(ctx, orgID, seg[1]))
	case n == 2 && seg[0] == "pricing" && seg[1] == "resolve" && m == http.MethodPost:
		var in PricingRequest
		if !decode(w, r, &in) {
			return true
		}
		reply(w)(h.repo.Price(ctx, orgID, in))

	// ---- orders and deliveries ----
	case n == 1 && seg[0] == "orders" && m == http.MethodGet:
		l, err := h.repo.ListOrders(ctx, orgID, q.Get("status"), q.Get("q"))
		replyKey(w, "orders", l, err)
	case n == 1 && seg[0] == "orders" && m == http.MethodPost:
		var in OrderInput
		if !decode(w, r, &in) {
			return true
		}
		reply(w)(h.repo.SaveOrder(ctx, orgID, userID, "", in))
	case n == 2 && seg[0] == "orders" && m == http.MethodGet:
		reply(w)(h.repo.GetOrder(ctx, orgID, seg[1]))
	case n == 2 && seg[0] == "orders" && m == http.MethodPut:
		var in OrderInput
		if !decode(w, r, &in) {
			return true
		}
		reply(w)(h.repo.SaveOrder(ctx, orgID, userID, seg[1], in))
	case n == 3 && seg[0] == "orders" && seg[2] == "status" && m == http.MethodPost:
		var b struct {
			Status string `json:"status"`
		}
		if !decode(w, r, &b) {
			return true
		}
		reply(w)(h.repo.SetOrderStatus(ctx, orgID, seg[1], b.Status))
	case n == 3 && seg[0] == "orders" && seg[2] == "deliveries" && m == http.MethodPost:
		var in DeliveryInput
		if !decode(w, r, &in) {
			return true
		}
		reply(w)(h.repo.CreateDelivery(ctx, orgID, userID, seg[1], in))
	case n == 3 && seg[0] == "orders" && seg[2] == "invoice" && m == http.MethodPost:
		var in OrderInvoiceInput
		if !decode(w, r, &in) {
			return true
		}
		reply(w)(h.repo.InvoiceOrder(ctx, orgID, userID, seg[1], in))
	case n == 1 && seg[0] == "deliveries" && m == http.MethodGet:
		l, err := h.repo.ListDeliveries(ctx, orgID)
		replyKey(w, "deliveries", l, err)
	case n == 2 && seg[0] == "deliveries" && m == http.MethodGet:
		reply(w)(h.repo.GetDelivery(ctx, orgID, seg[1]))
	case n == 3 && seg[0] == "deliveries" && seg[2] == "dispatch" && m == http.MethodPost:
		h.dispatch(w, r, orgID, userID, seg[1])
	case n == 3 && seg[0] == "deliveries" && seg[2] == "cancel" && m == http.MethodPost:
		reply(w)(h.repo.CancelDelivery(ctx, orgID, seg[1]))

	// ---- customers: statement, aging, portal ----
	case n == 2 && seg[0] == "customers" && seg[1] == "aging" && m == http.MethodGet:
		rows, err := h.repo.CustomerAging(ctx, orgID)
		replyKey(w, "rows", rows, err)
	case n == 1 && seg[0] == "statement" && m == http.MethodGet:
		reply(w)(h.repo.Statement(ctx, orgID, q.Get("customer"), q.Get("currency"), q.Get("from"), q.Get("to")))
	case n == 1 && seg[0] == "portal-link" && m == http.MethodPost:
		var b struct {
			Customer string `json:"customer"`
		}
		if !decode(w, r, &b) {
			return true
		}
		if strings.TrimSpace(b.Customer) == "" {
			httpErr(w, invalid("customer is required"))
			return true
		}
		tok, err := h.repo.PortalToken(ctx, orgID, b.Customer)
		if err != nil {
			httpErr(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": tok, "path": "/portal/" + tok})

	// ---- reminders ----
	case n == 2 && seg[0] == "reminders" && seg[1] == "log" && m == http.MethodGet:
		l, err := h.repo.ListReminderLog(ctx, orgID)
		replyKey(w, "log", l, err)
	case n == 2 && seg[0] == "reminders" && seg[1] == "run" && m == http.MethodPost:
		sent, err := h.runReminders(ctx, orgID, q.Get("origin"))
		if err != nil {
			httpErr(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "email": h.mail != nil && h.mail.Configured()})

	// ---- plans and subscriptions ----
	case n == 1 && seg[0] == "plans" && m == http.MethodGet:
		l, err := h.repo.ListPlans(ctx, orgID)
		replyKey(w, "plans", l, err)
	case n == 1 && seg[0] == "plans" && m == http.MethodPost:
		var p Plan
		if !decode(w, r, &p) {
			return true
		}
		reply(w)(h.repo.SavePlan(ctx, orgID, "", p))
	case n == 2 && seg[0] == "plans" && m == http.MethodPut:
		var p Plan
		if !decode(w, r, &p) {
			return true
		}
		reply(w)(h.repo.SavePlan(ctx, orgID, seg[1], p))
	case n == 1 && seg[0] == "subscriptions" && m == http.MethodGet:
		l, err := h.repo.ListSubscriptions(ctx, orgID)
		replyKey(w, "subscriptions", l, err)
	case n == 1 && seg[0] == "subscriptions" && m == http.MethodPost:
		var s Subscription
		if !decode(w, r, &s) {
			return true
		}
		reply(w)(h.repo.CreateSubscription(ctx, orgID, s))
	case n == 2 && seg[0] == "subscriptions" && seg[1] == "run" && m == http.MethodPost:
		res, err := h.repo.RunSubscriptions(ctx, orgID, userID)
		if err != nil {
			httpErr(w, err)
			return true
		}
		for _, d := range res.Invoiced {
			h.mailInvoice(ctx, orgID, d)
		}
		writeJSON(w, http.StatusOK, res)
	case n == 3 && seg[0] == "subscriptions" && seg[2] == "action" && m == http.MethodPost:
		var b struct {
			Action string `json:"action"`
		}
		if !decode(w, r, &b) {
			return true
		}
		reply(w)(h.repo.SetSubscription(ctx, orgID, seg[1], b.Action))
	default:
		ok = false
	}
	return ok
}

func (h *HTTPHandler) dispatch(w http.ResponseWriter, r *http.Request, orgID, userID, id string) {
	ctx := r.Context()
	var b struct {
		Date string `json:"date"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	dn, err := h.repo.MarkDispatched(ctx, orgID, id, b.Date)
	if err != nil {
		httpErr(w, err)
		return
	}
	warnings := []string{}
	h.repo.SyncReservations(ctx, orgID, userID, dn.OrderID)
	if h.adjust != nil && !dn.StockMoved {
		for _, l := range dn.Lines {
			if l.ProductID == "" || l.Qty <= 0 {
				continue
			}
			if err := h.adjust(ctx, orgID, userID, l.ProductID, -l.Qty, dn.Number); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", l.Description, err))
			}
		}
		_ = h.repo.MarkStockMoved(ctx, orgID, dn.ID)
		dn, _ = h.repo.GetDelivery(ctx, orgID, dn.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"delivery": dn, "stock_warnings": warnings})
}

type bulkResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// bulk applies one action to many documents and reports each outcome, so a
// single bad row never hides the others.
func (h *HTTPHandler) bulk(w http.ResponseWriter, r *http.Request, orgID, userID string) {
	var b struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"` // send | remind | mark_sent | trash | restore
		Origin string   `json:"origin"`
	}
	if !decode(w, r, &b) {
		return
	}
	if len(b.IDs) == 0 || len(b.IDs) > 200 {
		httpErr(w, invalid("choose between 1 and 200 documents"))
		return
	}
	ctx := r.Context()
	out := make([]bulkResult, 0, len(b.IDs))
	for _, id := range b.IDs {
		res := bulkResult{ID: id, OK: true}
		var err error
		switch b.Action {
		case "trash":
			err = h.repo.Trash(ctx, orgID, id)
		case "restore":
			err = h.repo.Restore(ctx, orgID, id)
		case "mark_sent":
			_, _, err = h.repo.MarkSent(ctx, orgID, id)
		case "send", "remind":
			var d Document
			d, err = h.repo.GetDocument(ctx, orgID, id)
			if err == nil {
				switch {
				case d.Deleted:
					err = invalid("document is in the trash")
				case b.Action == "remind" && (d.Kind != "invoice" || d.Balance <= 0.004 || d.Status == "draft"):
					err = invalid("nothing to remind")
				case d.ClientEmail == "":
					err = invalid("no client email")
				case h.mail == nil || !h.mail.Configured():
					err = invalid("email is not configured")
				default:
					if err = h.emailDoc(ctx, orgID, d, b.Origin, "", b.Action == "remind"); err == nil {
						if b.Action == "remind" {
							err = h.repo.NoteReminder(ctx, orgID, id)
						} else {
							_, _, err = h.repo.MarkSent(ctx, orgID, id)
						}
					}
				}
			}
		default:
			httpErr(w, invalid("action must be send, remind, mark_sent, trash or restore"))
			return
		}
		if err != nil {
			res.OK, res.Error = false, strings.TrimPrefix(err.Error(), "invalid: ")
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

func (h *HTTPHandler) originFor(s Settings, hint string) string {
	if hint != "" {
		return hint
	}
	if s.Sales.PublicURL != "" {
		return s.Sales.PublicURL
	}
	return os.Getenv("ADRIENT_PUBLIC_URL")
}

func (h *HTTPHandler) runReminders(ctx context.Context, orgID, originHint string) (int, error) {
	if h.mail == nil || !h.mail.Configured() {
		return 0, nil
	}
	s, err := h.repo.GetSettings(ctx, orgID)
	if err != nil {
		return 0, err
	}
	origin := h.originFor(s, originHint)
	if origin == "" {
		return 0, invalid("set the public URL in Settings so reminder links work")
	}
	return h.repo.RunReminders(ctx, orgID, origin, func(ctx context.Context, to, subject, text string) error {
		return h.mail.Send(ctx, email.Message{To: []string{to}, Subject: subject, Text: text})
	}, time.Now())
}

func (h *HTTPHandler) mailInvoice(ctx context.Context, orgID string, d Document) {
	if h.mail == nil || !h.mail.Configured() || d.ClientEmail == "" || d.Status != "sent" {
		return
	}
	s, err := h.repo.GetSettings(ctx, orgID)
	if err != nil {
		return
	}
	origin := h.originFor(s, "")
	if origin == "" {
		return
	}
	if err := h.emailDoc(ctx, orgID, d, origin, "", false); err != nil {
		slog.Warn("invoicing: scheduled email failed", "err", err)
	}
}

// StartScheduler runs the recurring-invoice, subscription and reminder runners
// for every org on a timer. Every runner is idempotent, so several server
// processes (or a manual "Run now") never double-bill or double-remind.
func (h *HTTPHandler) StartScheduler(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 15 * time.Minute
	}
	go func() {
		t := time.NewTimer(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				h.Tick(ctx)
				t.Reset(every)
			}
		}
	}()
}

// Tick runs all scheduled work once.
func (h *HTTPHandler) Tick(ctx context.Context) {
	rec, _ := h.repo.OrgIDsWhere(ctx, `SELECT DISTINCT org_id::text FROM adrient.invc_recurring WHERE active`)
	for _, org := range rec {
		made, auto, err := h.repo.RunRecurring(ctx, org, "")
		if err != nil {
			slog.Warn("invoicing: scheduled recurring", "org", org, "err", err)
			continue
		}
		if len(auto) > 0 {
			for _, d := range made {
				h.mailInvoice(ctx, org, d)
			}
		}
	}
	subs, _ := h.repo.OrgIDsWhere(ctx, `SELECT DISTINCT org_id::text FROM adrient.sales_subscriptions WHERE status <> 'cancelled'`)
	for _, org := range subs {
		res, err := h.repo.RunSubscriptions(ctx, org, "")
		if err != nil {
			slog.Warn("invoicing: scheduled subscriptions", "org", org, "err", err)
			continue
		}
		for _, d := range res.Invoiced {
			h.mailInvoice(ctx, org, d)
		}
	}
	rem, _ := h.repo.OrgIDsWhere(ctx, `SELECT org_id::text FROM adrient.invc_settings WHERE sales->>'reminders_auto'='true'`)
	for _, org := range rem {
		if _, err := h.runReminders(ctx, org, ""); err != nil {
			slog.Warn("invoicing: scheduled reminders", "org", org, "err", err)
		}
	}
}

// ---- public (signed-out) ----

type ipLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

var publicLimiter = &ipLimiter{hits: map[string][]time.Time{}}

// allow permits at most 30 write attempts per minute per client address.
func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	keep := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 30 {
		l.hits[ip] = keep
		return false
	}
	l.hits[ip] = append(keep, now)
	if len(l.hits) > 5000 {
		l.hits = map[string][]time.Time{}
	}
	return true
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// publicBusiness is the branding and payment information shown to customers.
func publicBusiness(s Settings) map[string]any {
	b := map[string]any{"business_name": s.BusinessName, "address": s.Address, "tax_id": s.TaxID, "email": s.Email,
		"phone": s.Phone, "logo_url": s.LogoURL, "payment_info": s.PaymentInfo, "accent": s.Sales.PDFAccent,
		"footer": s.Sales.PDFFooter, "state_code": s.Sales.StateCode}
	if s.Sales.ShowBank {
		b["bank_details"] = s.Sales.BankDetails
	}
	if s.Sales.ShowUPIQR && s.Sales.UPIID != "" {
		b["upi_id"], b["upi_name"] = s.Sales.UPIID, s.Sales.UPIName
	}
	return b
}

func (p *PublicHandler) servePublicAction(w http.ResponseWriter, r *http.Request, rest []string) bool {
	if rest[0] == "webhook" && len(rest) == 2 && rest[1] == "razorpay" && r.Method == http.MethodPost {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return true
		}
		recorded, err := p.repo.HandleRazorpayWebhook(r.Context(), body, r.Header.Get("X-Razorpay-Signature"))
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recorded": recorded})
		case errors.Is(err, ErrNotFound):
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true}) // not ours: acknowledge so Razorpay stops retrying
		case err.Error() == "bad signature":
			http.Error(w, "bad signature", http.StatusUnauthorized)
		default:
			httpErr(w, err)
		}
		return true
	}
	if len(rest) != 2 || r.Method != http.MethodPost {
		return false
	}
	if !publicLimiter.allow(clientIP(r)) {
		http.Error(w, "too many requests, try again in a minute", http.StatusTooManyRequests)
		return true
	}
	token, action := rest[0], rest[1]
	ctx := r.Context()
	switch action {
	case "accept":
		var b struct {
			Name  string `json:"name"`
			Agree bool   `json:"agree"`
		}
		if !decode(w, r, &b) {
			return true
		}
		d, err := p.repo.AcceptByToken(ctx, token, Acceptance{Name: b.Name, Agree: b.Agree, IP: clientIP(r), UA: r.UserAgent()})
		p.replyDoc(w, d, err)
	case "decline":
		var b struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &b) {
			return true
		}
		d, err := p.repo.DeclineByToken(ctx, token, b.Reason)
		p.replyDoc(w, d, err)
	case "pay":
		d, orgID, err := p.repo.byToken(ctx, token)
		if err != nil {
			httpErr(w, err)
			return true
		}
		url, err := p.repo.CreatePaymentLink(ctx, orgID, d, "")
		switch {
		case errors.Is(err, ErrGatewayNotConfigured):
			http.Error(w, "online_payment_not_available", http.StatusConflict)
		case err != nil && !errors.Is(err, ErrInvalid):
			slog.Warn("invoicing: public pay", "err", err)
			http.Error(w, "payment link could not be created, try again later", http.StatusBadGateway)
		case err != nil:
			httpErr(w, err)
		default:
			writeJSON(w, http.StatusOK, map[string]any{"url": url})
		}
	default:
		return false
	}
	return true
}

func (p *PublicHandler) replyDoc(w http.ResponseWriter, d Document, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	d.ContactID, d.ShareToken, d.RecurringID, d.ConvertedFrom, d.SalesOrderID, d.SourceRef = "", "", "", "", "", ""
	writeJSON(w, http.StatusOK, map[string]any{"document": d})
}

func (p *PublicHandler) servePortal(w http.ResponseWriter, r *http.Request) {
	token := strings.Trim(strings.TrimPrefix(r.URL.Path, portalMount), "/")
	if token == "" || strings.Contains(token, "/") || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	portal, err := p.repo.PortalByToken(r.Context(), token)
	if err != nil {
		httpErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	writeJSON(w, http.StatusOK, portal)
}

const portalMount = "/api/v1/public/portal/"
