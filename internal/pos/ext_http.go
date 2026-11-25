package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
)

// Mailer sends email (satisfied by *email.Sender).
type Mailer interface {
	Configured() bool
	Send(ctx context.Context, m email.Message) error
}

// ExtAuth supplies the caller from the request context.
type ExtAuth struct {
	UserID func(r *http.Request) (string, bool)
	OrgID  func(r *http.Request) (string, bool)
}

// ExtHandler serves the wave-3 POS routes under /api/v1/pos/x/ (the generated
// gateway cannot grow without regenerating protobuf, so these are plain JSON).
type ExtHandler struct {
	repo *Repository
	auth ExtAuth
	mail Mailer
}

// NewExtHandler constructs the handler. mail may be nil.
func NewExtHandler(repo *Repository, auth ExtAuth, mail Mailer) *ExtHandler {
	return &ExtHandler{repo: repo, auth: auth, mail: mail}
}

const extMount = "/api/v1/pos/x/"

// Match reports whether path belongs to this handler.
func (h *ExtHandler) Match(path string) bool { return strings.HasPrefix(path, extMount) }

var pj = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}

func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func protoOut(w http.ResponseWriter, status int, wrap string, sale *adrientv1.PosSale, extra map[string]any) {
	b, _ := pj.Marshal(sale)
	out := map[string]any{wrap: json.RawMessage(b)}
	for k, v := range extra {
		out[k] = v
	}
	jsonOut(w, status, out)
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		jsonOut(w, http.StatusNotFound, map[string]string{"message": "not found"})
	case errors.Is(err, ErrInvalid):
		jsonOut(w, http.StatusBadRequest, map[string]string{"message": cleanErr(err)})
	case errors.Is(err, ErrConflict), errors.Is(err, ErrSyncBusy):
		jsonOut(w, http.StatusConflict, map[string]string{"message": cleanErr(err)})
	case errors.Is(err, ErrBadPin):
		jsonOut(w, http.StatusForbidden, map[string]string{"message": err.Error(), "code": "bad_pin"})
	case errors.Is(err, ErrPinLocked):
		jsonOut(w, http.StatusTooManyRequests, map[string]string{"message": err.Error(), "code": "pin_locked"})
	default:
		slog.Error("pos ext", "err", err)
		jsonOut(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(v); err != nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"message": "invalid json: " + err.Error()})
		return false
	}
	return true
}

func (h *ExtHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.auth.OrgID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, _ := h.auth.UserID(r)
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, extMount), "/"), "/")
	ctx := r.Context()
	m := r.Method
	q := r.URL.Query()
	n := len(seg)
	out := func(v any, err error) {
		if err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, v)
	}
	key := func(k string, v any, err error) {
		if err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{k: v})
	}

	switch {
	case n == 2 && seg[0] == "sales" && seg[1] == "page" && m == http.MethodGet:
		h.salesPage(w, r, orgID)
	// settings
	case n == 1 && seg[0] == "settings" && m == http.MethodGet:
		out(h.repo.GetExt(ctx, orgID))
	case n == 1 && seg[0] == "settings" && m == http.MethodPut:
		var s ExtSettings
		if readJSON(w, r, &s) {
			out(h.repo.SaveExt(ctx, orgID, s))
		}

	// manager PIN
	case n == 1 && seg[0] == "pin" && m == http.MethodGet:
		out(h.repo.GetPinInfo(ctx, orgID, userID))
	case n == 1 && seg[0] == "pin" && m == http.MethodPut:
		var b struct {
			PIN string `json:"pin"`
			PinInfo
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := h.repo.SetPin(ctx, orgID, userID, b.PIN, b.PinInfo); err != nil {
			fail(w, err)
			return
		}
		out(h.repo.GetPinInfo(ctx, orgID, userID))
	case n == 1 && seg[0] == "pin" && m == http.MethodDelete:
		if err := h.repo.RemovePin(ctx, orgID, userID); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
	case n == 2 && seg[0] == "pin" && seg[1] == "verify" && m == http.MethodPost:
		var b struct {
			PIN   string `json:"pin"`
			Scope string `json:"scope"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		a, err := h.repo.VerifyPin(ctx, orgID, userID, b.PIN, b.Scope)
		if err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "approver": a})

	// sales: idempotent completion (used online and by the offline outbox)
	case n == 2 && seg[0] == "sales" && seg[1] == "complete" && m == http.MethodPost:
		var b struct {
			Key         string          `json:"idempotency_key"`
			Sale        json.RawMessage `json:"sale"`
			ApprovalPIN string          `json:"approval_pin"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		in := &adrientv1.PosSaleInput{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(b.Sale, in); err != nil {
			jsonOut(w, http.StatusBadRequest, map[string]string{"message": "invalid sale: " + err.Error()})
			return
		}
		res, err := h.repo.CompleteIdempotent(ctx, orgID, userID, b.Key, in, b.ApprovalPIN)
		if err != nil {
			fail(w, err)
			return
		}
		status := http.StatusOK
		if res.Status == "failed" {
			status = http.StatusUnprocessableEntity // permanent: do not retry, show the reason
		}
		extra := map[string]any{"status": res.Status, "replay": res.Replay, "sale_id": res.SaleID, "error": res.Error}
		if res.Sale != nil {
			protoOut(w, status, "sale", res.Sale, extra)
			return
		}
		jsonOut(w, status, extra)
	case n == 3 && seg[0] == "sales" && seg[2] == "void" && m == http.MethodPost:
		var b struct {
			Reason     string `json:"reason"`
			PIN        string `json:"pin"`
			RegisterID string `json:"register_id"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		s, err := h.repo.VoidSale(ctx, orgID, userID, seg[1], b.RegisterID, b.Reason, b.PIN)
		if err != nil {
			fail(w, err)
			return
		}
		protoOut(w, http.StatusOK, "sale", s, nil)
	case n == 3 && seg[0] == "sales" && seg[2] == "refund" && m == http.MethodPost:
		raw := json.RawMessage{}
		var env struct {
			PIN string `json:"pin"`
		}
		body := make([]byte, 0)
		if !readJSON(w, r, &raw) {
			return
		}
		body = raw
		_ = json.Unmarshal(body, &env)
		req := &adrientv1.RefundPosSaleRequest{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, req); err != nil {
			jsonOut(w, http.StatusBadRequest, map[string]string{"message": "invalid refund: " + err.Error()})
			return
		}
		req.Id = seg[1]
		ext, err := h.repo.GetExt(ctx, orgID)
		if err != nil {
			fail(w, err)
			return
		}
		if ext.RefundNeedsPin {
			if _, err := h.repo.VerifyPin(ctx, orgID, userID, env.PIN, "refund"); err != nil {
				fail(w, err)
				return
			}
		}
		s, err := h.repo.RefundSale(ctx, orgID, userID, req)
		if err != nil {
			fail(w, err)
			return
		}
		protoOut(w, http.StatusOK, "sale", s, nil)
	case n == 3 && seg[0] == "sales" && seg[2] == "email" && m == http.MethodPost:
		h.emailReceipt(w, r, orgID, seg[1])

	// reports
	case n == 1 && seg[0] == "profit" && m == http.MethodGet:
		out(h.repo.ProfitReport(ctx, orgID, q.Get("since"), q.Get("until")))
	case n == 1 && seg[0] == "zreport" && m == http.MethodGet:
		out(h.repo.ZReport(ctx, orgID, q.Get("shift_id")))

	// modifiers
	case n == 1 && seg[0] == "modifiers" && m == http.MethodGet:
		g, err := h.repo.ListModGroups(ctx, orgID)
		if err != nil {
			fail(w, err)
			return
		}
		mp, err := h.repo.ProductModMap(ctx, orgID)
		if err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"groups": g, "products": mp})
	case n == 1 && seg[0] == "modifiers" && m == http.MethodPost:
		var g ModGroup
		if readJSON(w, r, &g) {
			out(h.repo.SaveModGroup(ctx, orgID, "", g))
		}
	case n == 2 && seg[0] == "modifiers" && seg[1] == "assign" && m == http.MethodPut:
		var b struct {
			ProductID string   `json:"product_id"`
			GroupIDs  []string `json:"group_ids"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := h.repo.AssignModGroups(ctx, orgID, b.ProductID, b.GroupIDs); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
	case n == 2 && seg[0] == "modifiers" && m == http.MethodPut:
		var g ModGroup
		if readJSON(w, r, &g) {
			out(h.repo.SaveModGroup(ctx, orgID, seg[1], g))
		}
	case n == 2 && seg[0] == "modifiers" && m == http.MethodDelete:
		if err := h.repo.DeleteModGroup(ctx, orgID, seg[1]); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})

	// favourites
	case n == 1 && seg[0] == "favourites" && m == http.MethodGet:
		f, err := h.repo.ListFavourites(ctx, orgID, q.Get("register_id"))
		key("favourites", f, err)
	case n == 1 && seg[0] == "favourites" && m == http.MethodPut:
		var b struct {
			Favourites []Favourite `json:"favourites"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := h.repo.SaveFavourites(ctx, orgID, b.Favourites); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})

	// gift cards
	case n == 1 && seg[0] == "gift-cards" && m == http.MethodGet:
		l, err := h.repo.ListGiftCards(ctx, orgID)
		key("cards", l, err)
	case n == 1 && seg[0] == "gift-cards" && m == http.MethodPost:
		var b struct {
			Amount      float64 `json:"amount"`
			HolderName  string  `json:"holder_name"`
			HolderEmail string  `json:"holder_email"`
			ExpiresOn   string  `json:"expires_on"`
		}
		if readJSON(w, r, &b) {
			out(h.repo.IssueGiftCard(ctx, orgID, userID, b.Amount, b.HolderName, b.HolderEmail, b.ExpiresOn))
		}
	case n == 2 && seg[0] == "gift-cards" && m == http.MethodGet:
		out(h.repo.GetGiftCard(ctx, orgID, seg[1]))
	case n == 3 && seg[0] == "gift-cards" && seg[2] == "topup" && m == http.MethodPost:
		var b struct {
			Amount float64 `json:"amount"`
		}
		if readJSON(w, r, &b) {
			out(h.repo.TopUpGiftCard(ctx, orgID, seg[1], b.Amount))
		}
	case n == 3 && seg[0] == "gift-cards" && seg[2] == "status" && m == http.MethodPost:
		var b struct {
			Status string `json:"status"`
		}
		if readJSON(w, r, &b) {
			out(h.repo.SetGiftCardStatus(ctx, orgID, seg[1], b.Status))
		}

	// loyalty
	case n == 2 && seg[0] == "loyalty" && m == http.MethodGet:
		out(h.repo.Loyalty(ctx, orgID, seg[1]))

	// stores
	case n == 1 && seg[0] == "stores" && m == http.MethodGet:
		s, err := h.repo.ListStores(ctx, orgID)
		if err != nil {
			fail(w, err)
			return
		}
		mp, err := h.repo.RegisterStoreMap(ctx, orgID)
		if err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"stores": s, "registers": mp})
	case n == 1 && seg[0] == "stores" && m == http.MethodPost:
		var s Store
		if readJSON(w, r, &s) {
			out(h.repo.SaveStore(ctx, orgID, "", s))
		}
	case n == 2 && seg[0] == "stores" && seg[1] == "assign" && m == http.MethodPut:
		var b struct {
			RegisterID string `json:"register_id"`
			StoreID    string `json:"store_id"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := h.repo.AssignRegisterStore(ctx, orgID, b.RegisterID, b.StoreID); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
	case n == 2 && seg[0] == "stores" && m == http.MethodPut:
		var s Store
		if readJSON(w, r, &s) {
			out(h.repo.SaveStore(ctx, orgID, seg[1], s))
		}
	case n == 1 && seg[0] == "capabilities" && m == http.MethodGet:
		jsonOut(w, http.StatusOK, map[string]any{"email": h.mail != nil && h.mail.Configured()})
	default:
		http.NotFound(w, r)
	}
}

// emailReceipt sends the receipt of a sale to the customer from the server.
func (h *ExtHandler) emailReceipt(w http.ResponseWriter, r *http.Request, orgID, saleID string) {
	var b struct {
		To string `json:"to"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	b.To = strings.TrimSpace(b.To)
	if !strings.Contains(b.To, "@") || len(b.To) > 200 || strings.ContainsAny(b.To, "\r\n,; ") {
		jsonOut(w, http.StatusBadRequest, map[string]string{"message": "enter one valid email address"})
		return
	}
	if h.mail == nil || !h.mail.Configured() {
		jsonOut(w, http.StatusConflict, map[string]string{"message": "email_not_configured"})
		return
	}
	ctx := r.Context()
	s, err := h.repo.GetSale(ctx, orgID, saleID)
	if err != nil {
		fail(w, err)
		return
	}
	if s.GetNumber() == 0 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"message": "only completed sales have a receipt"})
		return
	}
	set, _ := h.repo.GetSettings(ctx, orgID)
	ext, _ := h.repo.GetExt(ctx, orgID)
	st := h.repo.StoreForRegister(ctx, orgID, s.GetRegisterId())
	subject, text, htmlBody := RenderReceipt(s, set.GetStoreName(), set.GetReceiptHeader(), set.GetReceiptFooter(), set.GetCurrency(), st, ext.ReceiptFooter)
	if err := h.mail.Send(ctx, email.Message{To: []string{b.To}, Subject: subject, Text: text, HTML: htmlBody}); err != nil {
		slog.Warn("pos: receipt email failed", "err", err)
		jsonOut(w, http.StatusBadGateway, map[string]string{"message": "the email could not be sent"})
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

// RenderReceipt builds the plain-text and HTML receipt for an email.
func RenderReceipt(s *adrientv1.PosSale, storeName, header, footer, currency string, st Store, extFooter string) (subject, text, htmlBody string) {
	name := storeName
	if st.Name != "" {
		name = st.Name
	}
	if name == "" {
		name = "Receipt"
	}
	hdr := header
	if st.ReceiptHeader != "" {
		hdr = st.ReceiptHeader
	}
	ft := footer
	if st.ReceiptFooter != "" {
		ft = st.ReceiptFooter
	} else if extFooter != "" && footer == "" {
		ft = extFooter
	}
	cur := currency
	if cur == "" {
		cur = "INR"
	}
	subject = fmt.Sprintf("Your receipt #%d from %s", s.GetNumber(), name)
	var tb, hb strings.Builder
	fmt.Fprintf(&tb, "%s\n", name)
	if st.Address != "" {
		fmt.Fprintf(&tb, "%s\n", st.Address)
	}
	if hdr != "" {
		fmt.Fprintf(&tb, "%s\n", hdr)
	}
	fmt.Fprintf(&tb, "Receipt #%d\n\n", s.GetNumber())
	e := html.EscapeString
	fmt.Fprintf(&hb, `<div style="font-family:Arial,sans-serif;max-width:480px"><h2 style="margin:0">%s</h2>`, e(name))
	if st.Address != "" {
		fmt.Fprintf(&hb, `<div>%s</div>`, e(st.Address))
	}
	if hdr != "" {
		fmt.Fprintf(&hb, `<div>%s</div>`, e(hdr))
	}
	fmt.Fprintf(&hb, `<p>Receipt #%d</p><table style="width:100%%;border-collapse:collapse">`, s.GetNumber())
	for _, l := range s.GetLines() {
		fmt.Fprintf(&tb, "%g x %s  %s %.2f\n", l.GetQty(), l.GetName(), cur, l.GetLineTotal())
		fmt.Fprintf(&hb, `<tr><td>%g x %s</td><td style="text-align:right">%.2f</td></tr>`, l.GetQty(), e(l.GetName()), l.GetLineTotal())
	}
	fmt.Fprintf(&tb, "\nTotal: %s %.2f\n", cur, s.GetTotal())
	fmt.Fprintf(&hb, `<tr><td style="border-top:1px solid #333"><b>Total</b></td><td style="border-top:1px solid #333;text-align:right"><b>%s %.2f</b></td></tr></table>`, e(cur), s.GetTotal())
	for _, p := range s.GetPayments() {
		fmt.Fprintf(&tb, "Paid by %s: %.2f\n", p.GetMethod(), p.GetAmount())
		fmt.Fprintf(&hb, `<div>Paid by %s: %.2f</div>`, e(p.GetMethod()), p.GetAmount())
	}
	if s.GetChangeDue() > 0 {
		fmt.Fprintf(&tb, "Change: %.2f\n", s.GetChangeDue())
		fmt.Fprintf(&hb, `<div>Change: %.2f</div>`, s.GetChangeDue())
	}
	if ft != "" {
		fmt.Fprintf(&tb, "\n%s\n", ft)
		fmt.Fprintf(&hb, `<p>%s</p>`, e(ft))
	}
	hb.WriteString(`</div>`)
	return subject, tb.String(), hb.String()
}
