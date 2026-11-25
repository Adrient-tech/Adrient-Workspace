package ai

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AuthFuncs supplies the caller from the request context.
type AuthFuncs struct {
	Caller func(r *http.Request) (Caller, bool)
}

// HTTPHandler serves /api/v1/ai/* inside the auth wall.
type HTTPHandler struct {
	g    *Gateway
	auth AuthFuncs
}

// NewHTTPHandler constructs the handler.
func NewHTTPHandler(g *Gateway, auth AuthFuncs) *HTTPHandler { return &HTTPHandler{g: g, auth: auth} }

const mount = "/api/v1/ai"

// Match reports whether path belongs to the AI API.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, ok := h.auth.Caller(r)
	if !ok || c.OrgID == "" {
		writeErr(w, apiErr(401, "unauthorized", "sign in first"))
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/")
	seg := strings.Split(path, "/")
	switch {
	case path == "status" && r.Method == http.MethodGet:
		writeJSON(w, 200, h.g.Status(r.Context(), c, r.URL.Query().Get("app")))
	case path == "chat" && r.Method == http.MethodPost:
		var req ChatRequest
		if !decode(w, r, &req, 1<<20) {
			return
		}
		h.stream(w, r, func(emit func(StreamEvent) error) *APIError { return h.g.Chat(r.Context(), c, req, emit) })
	case path == "audit" && r.Method == http.MethodPost:
		var a ToolAudit
		if decode(w, r, &a, 8<<10) {
			if aerr := h.g.RecordTool(r.Context(), c, a); aerr != nil {
				writeErr(w, aerr)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
		}
	case path == "ask" && r.Method == http.MethodPost:
		var req AskRequest
		if !decode(w, r, &req, 256<<10) {
			return
		}
		h.stream(w, r, func(emit func(StreamEvent) error) *APIError { return h.g.Ask(r.Context(), c, req, emit) })
	case seg[0] == "admin":
		if !c.IsAdmin {
			writeErr(w, apiErr(403, "forbidden", "only organisation admins can change AI settings"))
			return
		}
		h.admin(w, r, c, seg[1:])
	default:
		writeErr(w, apiErr(404, "not_found", "not found"))
	}
}

func (h *HTTPHandler) admin(w http.ResponseWriter, r *http.Request, c Caller, seg []string) {
	ctx := r.Context()
	n := len(seg)
	switch {
	case n == 1 && seg[0] == "settings" && r.Method == http.MethodGet:
		s, err := h.g.Store.GetSettings(ctx, c.OrgID)
		if s.Notice == "" {
			s.Notice = DefaultNotice
		}
		reply(w, map[string]any{"settings": s, "apps": Apps, "server_key": h.g.Secret != ""}, err)
	case n == 1 && seg[0] == "settings" && r.Method == http.MethodPut:
		var s Settings
		if decode(w, r, &s, 64<<10) {
			v, err := h.g.SaveSettings(ctx, c, s)
			reply(w, v, err)
		}
	case n == 1 && seg[0] == "providers" && r.Method == http.MethodGet:
		ps, err := h.g.ListProviders(ctx, c.OrgID)
		if ps == nil {
			ps = []Provider{}
		}
		reply(w, ps, err)
	case n == 1 && seg[0] == "providers" && r.Method == http.MethodPost:
		var in ProviderInput
		if decode(w, r, &in, 64<<10) {
			p, err := h.g.SaveProvider(ctx, c, "", in)
			reply(w, p, err)
		}
	case n == 2 && seg[0] == "providers" && idRe.MatchString(seg[1]) && r.Method == http.MethodPut:
		var in ProviderInput
		if decode(w, r, &in, 64<<10) {
			p, err := h.g.SaveProvider(ctx, c, seg[1], in)
			reply(w, p, err)
		}
	case n == 2 && seg[0] == "providers" && idRe.MatchString(seg[1]) && r.Method == http.MethodDelete:
		reply(w, map[string]bool{"ok": true}, h.g.DeleteProvider(ctx, c, seg[1]))
	case n == 3 && seg[0] == "providers" && idRe.MatchString(seg[1]) && seg[2] == "test" && r.Method == http.MethodPost:
		var body struct {
			Model string `json:"model"`
		}
		if r.ContentLength > 0 && !decode(w, r, &body, 4<<10) {
			return
		}
		res, err := h.g.TestProvider(ctx, c, seg[1], body.Model)
		reply(w, res, err)
	case n == 1 && seg[0] == "usage" && r.Method == http.MethodGet:
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if days <= 0 || days > 365 {
			days = 30
		}
		by := r.URL.Query().Get("by")
		if by != "model" && by != "user" {
			by = "app"
		}
		rows, err := h.g.Store.UsageSummary(ctx, c.OrgID, by, h.g.now().AddDate(0, 0, -days))
		reply(w, map[string]any{"by": by, "days": days, "rows": rows}, err)
	default:
		writeErr(w, apiErr(404, "not_found", "not found"))
	}
}

// stream runs fn as Server-Sent Events. Errors before the first event are
// returned as JSON with a proper status code.
func (h *HTTPHandler) stream(w http.ResponseWriter, r *http.Request, fn func(emit func(StreamEvent) error) *APIError) {
	fl, _ := w.(http.Flusher)
	started := false
	start := func() {
		if started {
			return
		}
		started = true
		hd := w.Header()
		hd.Set("Content-Type", "text/event-stream")
		hd.Set("Cache-Control", "no-store")
		hd.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
	}
	rc := http.NewResponseController(w)
	emit := func(e StreamEvent) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		start()
		b, _ := json.Marshal(e)
		_ = rc.SetWriteDeadline(time.Now().Add(60 * time.Second))
		if _, err := w.Write(append(append([]byte("data: "), b...), '\n', '\n')); err != nil {
			return err
		}
		if fl != nil {
			fl.Flush()
		}
		return nil
	}
	if aerr := fn(emit); aerr != nil {
		if started {
			_ = emit(StreamEvent{Type: "error", Code: aerr.Code, Message: aerr.Message})
			return
		}
		writeErr(w, aerr)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any, max int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, max))
	if err := dec.Decode(v); err != nil {
		writeErr(w, apiErr(400, "bad_request", "invalid JSON body"))
		return false
	}
	return true
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		var ae *APIError
		switch {
		case errors.As(err, &ae):
			writeErr(w, ae)
		case errors.Is(err, ErrNotFound):
			writeErr(w, apiErr(404, "not_found", "not found"))
		default:
			writeErr(w, apiErr(500, "internal", "internal error"))
		}
		return
	}
	writeJSON(w, 200, v)
}

func writeErr(w http.ResponseWriter, e *APIError) {
	st := e.Status
	if st == 0 {
		st = 500
	}
	writeJSON(w, st, e)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
