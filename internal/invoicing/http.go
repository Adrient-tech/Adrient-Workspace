package invoicing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
)

// AuthFuncs supplies the caller's identity from the request context.
type AuthFuncs struct {
	UserID func(r *http.Request) (string, bool)
	OrgID  func(r *http.Request) (string, bool)
}

// Mailer sends email (satisfied by *email.Sender).
type Mailer interface {
	Configured() bool
	Send(ctx context.Context, m email.Message) error
}

// StockAdjuster removes sold quantity from Inventory (reason "sale").
type StockAdjuster func(ctx context.Context, orgID, userID, productID string, qty float64, ref string) error

// HTTPHandler serves /api/v1/invoicing/* inside the auth wall.
type HTTPHandler struct {
	repo   *Repository
	auth   AuthFuncs
	mail   Mailer
	adjust StockAdjuster
}

// NewHTTPHandler constructs the authenticated handler. mail and adjust may be nil.
func NewHTTPHandler(repo *Repository, auth AuthFuncs, mail Mailer, adjust StockAdjuster) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth, mail: mail, adjust: adjust}
}

const mount = "/api/v1/invoicing"

// Match reports whether path belongs to the authenticated surface.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.auth.OrgID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, _ := h.auth.UserID(r)
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/"), "/")
	ctx := r.Context()
	m := r.Method
	q := r.URL.Query()
	n := len(seg)

	switch {
	case n == 1 && seg[0] == "settings" && m == http.MethodGet:
		reply(w)(h.repo.GetSettings(ctx, orgID))
	case n == 1 && seg[0] == "settings" && m == http.MethodPut:
		var s Settings
		if !decode(w, r, &s) {
			return
		}
		reply(w)(h.repo.SaveSettings(ctx, orgID, s))

	case n == 1 && seg[0] == "items" && m == http.MethodGet:
		items, err := h.repo.ListItems(ctx, orgID)
		replyKey(w, "items", items, err)
	case n == 1 && seg[0] == "items" && m == http.MethodPost:
		var it Item
		if !decode(w, r, &it) {
			return
		}
		reply(w)(h.repo.SaveItem(ctx, orgID, "", it))
	case n == 2 && seg[0] == "items" && m == http.MethodPut:
		var it Item
		if !decode(w, r, &it) {
			return
		}
		reply(w)(h.repo.SaveItem(ctx, orgID, seg[1], it))
	case n == 2 && seg[0] == "items" && m == http.MethodDelete:
		replyOK(w, h.repo.DeleteItem(ctx, orgID, seg[1]))

	case n == 1 && seg[0] == "documents" && m == http.MethodGet:
		h.documentsPage(w, r, orgID)
	case n == 1 && seg[0] == "documents" && m == http.MethodPost:
		var in DocInput
		if !decode(w, r, &in) {
			return
		}
		reply(w)(h.repo.SaveDocument(ctx, orgID, userID, "", in))
	case n == 2 && seg[0] == "documents" && m == http.MethodGet:
		reply(w)(h.repo.GetDocument(ctx, orgID, seg[1]))
	case n == 2 && seg[0] == "documents" && m == http.MethodPut:
		var in DocInput
		if !decode(w, r, &in) {
			return
		}
		reply(w)(h.repo.SaveDocument(ctx, orgID, userID, seg[1], in))
	case n == 2 && seg[0] == "documents" && m == http.MethodDelete:
		if q.Get("purge") == "1" {
			replyOK(w, h.repo.Purge(ctx, orgID, seg[1]))
		} else {
			replyOK(w, h.repo.Trash(ctx, orgID, seg[1]))
		}
	case n == 3 && seg[0] == "documents" && seg[2] == "restore" && m == http.MethodPost:
		replyOK(w, h.repo.Restore(ctx, orgID, seg[1]))
	case n == 3 && seg[0] == "documents" && seg[2] == "duplicate" && m == http.MethodPost:
		reply(w)(h.repo.Duplicate(ctx, orgID, userID, seg[1]))
	case n == 3 && seg[0] == "documents" && seg[2] == "convert" && m == http.MethodPost:
		reply(w)(h.repo.ConvertEstimateTo(ctx, orgID, userID, seg[1]))
	case n == 3 && seg[0] == "documents" && seg[2] == "status" && m == http.MethodPost:
		var b struct {
			Status string `json:"status"`
		}
		if !decode(w, r, &b) {
			return
		}
		reply(w)(h.repo.SetEstimateStatus(ctx, orgID, seg[1], b.Status))
	case n == 3 && seg[0] == "documents" && seg[2] == "send" && m == http.MethodPost:
		h.send(w, r, orgID, userID, seg[1], false)
	case n == 3 && seg[0] == "documents" && seg[2] == "remind" && m == http.MethodPost:
		h.send(w, r, orgID, userID, seg[1], true)
	case n == 3 && seg[0] == "documents" && seg[2] == "payments" && m == http.MethodPost:
		var p Payment
		if !decode(w, r, &p) {
			return
		}
		reply(w)(h.repo.AddPayment(ctx, orgID, userID, seg[1], p))
	case n == 4 && seg[0] == "documents" && seg[2] == "payments" && m == http.MethodDelete:
		reply(w)(h.repo.DeletePayment(ctx, orgID, seg[1], seg[3]))

	case n == 1 && seg[0] == "payments" && m == http.MethodGet:
		ps, err := h.repo.ListPayments(ctx, orgID, q.Get("from"), q.Get("to"))
		replyKey(w, "payments", ps, err)

	case n == 1 && seg[0] == "recurring" && m == http.MethodGet:
		l, err := h.repo.ListRecurring(ctx, orgID)
		replyKey(w, "recurring", l, err)
	case n == 1 && seg[0] == "recurring" && m == http.MethodPost:
		var rc Recurring
		if !decode(w, r, &rc) {
			return
		}
		reply(w)(h.repo.SaveRecurring(ctx, orgID, "", rc))
	case n == 2 && seg[0] == "recurring" && seg[1] == "run" && m == http.MethodPost:
		made, auto, err := h.repo.RunRecurring(ctx, orgID, userID)
		if err == nil && len(auto) > 0 {
			for _, d := range made {
				if d.Status == "sent" && d.ClientEmail != "" {
					_ = h.emailDoc(ctx, orgID, d, r.URL.Query().Get("origin"), "", false)
				}
			}
		}
		replyKey(w, "generated", made, err)
	case n == 2 && seg[0] == "recurring" && m == http.MethodPut:
		var rc Recurring
		if !decode(w, r, &rc) {
			return
		}
		reply(w)(h.repo.SaveRecurring(ctx, orgID, seg[1], rc))
	case n == 2 && seg[0] == "recurring" && m == http.MethodDelete:
		replyOK(w, h.repo.DeleteRecurring(ctx, orgID, seg[1]))

	case n == 1 && seg[0] == "clients" && m == http.MethodGet:
		c, err := h.repo.Clients(ctx, orgID)
		replyKey(w, "clients", c, err)
	case n == 1 && seg[0] == "report" && m == http.MethodGet:
		reply(w)(h.repo.BuildReport(ctx, orgID, q.Get("from"), q.Get("to")))
	case n == 1 && seg[0] == "capabilities" && m == http.MethodGet:
		gw, _ := h.repo.GatewayStatus(ctx, orgID)
		writeJSON(w, http.StatusOK, map[string]any{"email": h.mail != nil && h.mail.Configured(), "stock": h.adjust != nil,
			"payment_gateway": gw.Configured, "payment_gateway_mode": gw.Mode, "whatsapp": "link"})
	default:
		if !h.serveExt(w, r, orgID, userID, seg) {
			http.NotFound(w, r)
		}
	}
}

func (h *HTTPHandler) send(w http.ResponseWriter, r *http.Request, orgID, userID, id string, reminder bool) {
	var b struct {
		Origin   string `json:"origin"`
		Message  string `json:"message"`
		MarkOnly bool   `json:"mark_only"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	ctx := r.Context()
	d, err := h.repo.GetDocument(ctx, orgID, id)
	if err != nil {
		httpErr(w, err)
		return
	}
	if d.Deleted {
		httpErr(w, invalid("document is in the trash"))
		return
	}
	if reminder && (d.Kind != "invoice" || d.Balance <= 0.004 || d.Status == "draft") {
		httpErr(w, invalid("a reminder needs a sent invoice with a balance"))
		return
	}
	emailed := false
	if !b.MarkOnly {
		if h.mail == nil || !h.mail.Configured() {
			http.Error(w, "email_not_configured", http.StatusConflict)
			return
		}
		if d.ClientEmail == "" {
			httpErr(w, invalid("the client has no email address"))
			return
		}
		if err := h.emailDoc(ctx, orgID, d, b.Origin, b.Message, reminder); err != nil {
			slog.Warn("invoicing: send email failed", "err", err)
			http.Error(w, "email failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		emailed = true
	}
	var first bool
	if reminder {
		_ = h.repo.NoteReminder(ctx, orgID, id)
		d, err = h.repo.GetDocument(ctx, orgID, id)
	} else {
		d, first, err = h.repo.MarkSent(ctx, orgID, id)
	}
	if err != nil {
		httpErr(w, err)
		return
	}
	warnings := []string{}
	if first && d.Kind == "invoice" {
		warnings = h.deductStock(ctx, orgID, userID, d)
		d, _ = h.repo.GetDocument(ctx, orgID, id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": d, "emailed": emailed, "stock_warnings": warnings})
}

func (h *HTTPHandler) deductStock(ctx context.Context, orgID, userID string, d Document) []string {
	out := []string{}
	if h.adjust == nil || d.StockDeducted {
		return out
	}
	s, err := h.repo.GetSettings(ctx, orgID)
	if err != nil || !s.DeductStock {
		return out
	}
	for _, l := range d.Lines {
		if l.ProductID == "" || l.Qty <= 0 {
			continue
		}
		if err := h.adjust(ctx, orgID, userID, l.ProductID, -l.Qty, d.Number); err != nil {
			out = append(out, fmt.Sprintf("%s: %v", l.Description, err))
		}
	}
	_ = h.repo.SetStockDeducted(ctx, orgID, d.ID)
	return out
}

func (h *HTTPHandler) emailDoc(ctx context.Context, orgID string, d Document, origin, message string, reminder bool) error {
	if h.mail == nil || !h.mail.Configured() {
		return errors.New("email not configured")
	}
	s, err := h.repo.GetSettings(ctx, orgID)
	if err != nil {
		return err
	}
	origin = strings.TrimRight(origin, "/")
	link := origin + "/invoice/" + d.ShareToken
	noun := "Invoice"
	if d.Kind == "estimate" {
		noun = "Estimate"
	}
	subject := fmt.Sprintf("%s %s from %s", noun, d.Number, orDefault(s.BusinessName, "us"))
	if reminder {
		subject = fmt.Sprintf("Reminder: invoice %s", d.Number)
	}
	amount := d.Total
	if d.Kind == "invoice" {
		amount = d.Balance
	}
	e := html.EscapeString
	body := fmt.Sprintf(`<p>Hello %s,</p><p>%s</p><p>%s %s: <strong>%s %.2f</strong>%s</p><p><a href="%s">View %s online</a></p><p>%s</p>`,
		e(d.ClientName), e(orDefault(message, defaultMessage(noun, reminder))), noun, e(d.Number), e(d.Currency), amount,
		dueLine(d), e(link), strings.ToLower(noun), e(s.BusinessName))
	text := fmt.Sprintf("%s\n\n%s %s: %s %.2f\n%s\n", orDefault(message, defaultMessage(noun, reminder)), noun, d.Number, d.Currency, amount, link)
	return h.mail.Send(ctx, email.Message{To: []string{d.ClientEmail}, Subject: subject, HTML: body, Text: text})
}

func defaultMessage(noun string, reminder bool) string {
	if reminder {
		return "This is a friendly reminder that the invoice below is awaiting payment."
	}
	return "Please find your " + strings.ToLower(noun) + " below."
}

func dueLine(d Document) string {
	if d.DueDate == "" || d.Kind != "invoice" {
		return ""
	}
	return " (due " + html.EscapeString(d.DueDate) + ")"
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// PublicHandler serves the signed-out share page data.
type PublicHandler struct{ repo *Repository }

// NewPublicHandler constructs the public handler.
func NewPublicHandler(repo *Repository) *PublicHandler { return &PublicHandler{repo: repo} }

const publicMount = "/api/v1/public/invoices/"

// Match reports whether path is a public invoice request.
func (p *PublicHandler) Match(path string) bool {
	return strings.HasPrefix(path, publicMount) || strings.HasPrefix(path, portalMount)
}

func (p *PublicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, portalMount) {
		p.servePortal(w, r)
		return
	}
	rest := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, publicMount), "/"), "/")
	if len(rest) > 1 {
		if !p.servePublicAction(w, r, rest) {
			http.NotFound(w, r)
		}
		return
	}
	token := rest[0]
	if token == "" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	d, s, err := p.repo.PublicByToken(r.Context(), token)
	if err != nil {
		httpErr(w, err)
		return
	}
	orgGW, _ := p.repo.orgOfToken(r.Context(), token)
	gw, _ := p.repo.GatewayStatus(r.Context(), orgGW)
	d.ContactID, d.ShareToken, d.RecurringID, d.ConvertedFrom, d.SalesOrderID, d.SourceRef = "", "", "", "", "", ""
	// Only branding and payment instructions are public, not numbering or rates.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	writeJSON(w, http.StatusOK, map[string]any{"document": d, "business": publicBusiness(s), "can_pay_online": gw.Configured && d.Kind == "invoice"})
}

// ---- helpers ----

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(v); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func httpErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid: "), http.StatusBadRequest)
	default:
		slog.Error("invoicing", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func reply(w http.ResponseWriter) func(v any, err error) {
	return func(v any, err error) {
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func replyKey(w http.ResponseWriter, key string, v any, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{key: v})
}

func replyOK(w http.ResponseWriter, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (r *Repository) orgOfToken(ctx context.Context, token string) (string, error) {
	var org string
	err := r.pool.QueryRow(ctx, `SELECT org_id::text FROM adrient.invc_documents WHERE share_token=$1`, token).Scan(&org)
	return org, err
}
