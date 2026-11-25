package booking

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthFuncs extracts the caller from the request context.
type AuthFuncs struct {
	UserID func(r *http.Request) (string, bool)
	OrgID  func(r *http.Request) (string, bool)
}

// HTTPHandler is the signed-in API for managing pages and appointments:
//
//	GET/POST   /api/v1/booking/pages
//	PUT/DELETE /api/v1/booking/pages/{id}
//	GET        /api/v1/booking/appointments
//	POST       /api/v1/booking/appointments/{id}/cancel
type HTTPHandler struct {
	repo *Repository
	a    AuthFuncs
}

// NewHTTPHandler builds the signed-in handler.
func NewHTTPHandler(repo *Repository, a AuthFuncs) *HTTPHandler {
	return &HTTPHandler{repo: repo, a: a}
}

const authMount = "/api/v1/booking/"

// Match reports whether path belongs to the signed-in booking API.
func (h *HTTPHandler) Match(path string) bool { return strings.HasPrefix(path, authMount) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid input: "), http.StatusBadRequest)
	case errors.Is(err, ErrSlotTaken), errors.Is(err, ErrNotBookable):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func decodePage(w http.ResponseWriter, r *http.Request) (Page, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var p Page
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return p, false
	}
	return p, true
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok1 := h.a.OrgID(r)
	user, ok2 := h.a.UserID(r)
	if !ok1 || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, authMount), "/"), "/")
	ctx := r.Context()
	switch {
	case len(seg) == 1 && seg[0] == "pages" && r.Method == http.MethodGet:
		out, err := h.repo.ListPages(ctx, org, user)
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"pages": out, "base_url": h.repo.Deps.BaseURL})
	case len(seg) == 1 && seg[0] == "pages" && r.Method == http.MethodPost:
		p, ok := decodePage(w, r)
		if !ok {
			return
		}
		out, err := h.repo.CreatePage(ctx, org, user, p)
		if err != nil {
			httpErr(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, out)
	case len(seg) == 2 && seg[0] == "pages" && r.Method == http.MethodPut:
		p, ok := decodePage(w, r)
		if !ok {
			return
		}
		out, err := h.repo.UpdatePage(ctx, org, user, seg[1], p)
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, out)
	case len(seg) == 2 && seg[0] == "pages" && r.Method == http.MethodDelete:
		if err := h.repo.DeletePage(ctx, org, user, seg[1]); err != nil {
			httpErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 1 && seg[0] == "appointments" && r.Method == http.MethodGet:
		out, err := h.repo.ListAppointments(ctx, org, user, time.Now().Add(-24*time.Hour))
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"appointments": out})
	case len(seg) == 3 && seg[0] == "appointments" && seg[2] == "cancel" && r.Method == http.MethodPost:
		a, err := h.repo.OwnerAppointment(ctx, org, user, seg[1])
		if err == nil {
			err = h.repo.Cancel(ctx, a, true)
		}
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

// ---- public (no session) ----

// AbuseGuard is a sliding-window counter per key.
type AbuseGuard struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

// NewAbuseGuard allows `limit` events per key per window.
func NewAbuseGuard(limit int, window time.Duration) *AbuseGuard {
	return &AbuseGuard{hits: map[string][]time.Time{}, limit: limit, window: window}
}

// Allow records an event and reports whether it is within the limit.
func (g *AbuseGuard) Allow(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	cut := now.Add(-g.window)
	list := g.hits[key][:0]
	for _, t := range g.hits[key] {
		if t.After(cut) {
			list = append(list, t)
		}
	}
	if len(list) >= g.limit {
		g.hits[key] = list
		return false
	}
	g.hits[key] = append(list, now)
	if len(g.hits) > 20000 {
		for k, v := range g.hits {
			if len(v) == 0 || !v[len(v)-1].After(cut) {
				delete(g.hits, k)
			}
		}
	}
	return true
}

// PublicHandler serves the unauthenticated guest surface:
//
//	GET  /api/v1/public/booking/{slug}                page details
//	GET  /api/v1/public/booking/{slug}/slots?from&to  bookable start times
//	POST /api/v1/public/booking/{slug}/book           book a slot
//	GET  /api/v1/public/booking/cancel/{token}        appointment details
//	POST /api/v1/public/booking/cancel/{token}        cancel it
type PublicHandler struct {
	repo       *Repository
	bookGuard  *AbuseGuard
	readGuard  *AbuseGuard
	emailGuard *AbuseGuard
}

// NewPublicHandler builds the guest handler.
func NewPublicHandler(repo *Repository) *PublicHandler {
	return &PublicHandler{repo: repo, bookGuard: NewAbuseGuard(10, time.Hour),
		readGuard: NewAbuseGuard(300, time.Minute), emailGuard: NewAbuseGuard(5, 24*time.Hour)}
}

const publicMount = "/api/v1/public/booking/"

// Match reports whether path is a public booking route.
func (h *PublicHandler) Match(path string) bool { return strings.HasPrefix(path, publicMount) }

func clientKey(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if fwd := r.Header.Get("X-Real-IP"); fwd != "" && (strings.HasPrefix(ip, "127.") || ip == "::1" || strings.HasPrefix(ip, "10.") ||
		strings.HasPrefix(ip, "172.") || strings.HasPrefix(ip, "192.168.")) {
		ip = strings.TrimSpace(fwd)
	}
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:8])
}

type publicPage struct {
	Slug         string     `json:"slug"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Location     string     `json:"location"`
	DurationMin  int        `json:"duration_min"`
	Timezone     string     `json:"timezone"`
	Questions    []Question `json:"questions"`
	HasMeet      bool       `json:"has_meet"`
	OwnerName    string     `json:"owner_name"`
	MaxDaysAhead int        `json:"max_days_ahead"`
}

func (h *PublicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.readGuard.Allow(clientKey(r)) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, publicMount), "/"), "/")
	if len(seg) == 2 && seg[0] == "cancel" {
		a, err := h.repo.AppointmentByToken(ctx, seg[1])
		if err != nil {
			httpErr(w, err)
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, map[string]any{"title": a.PageTitle, "start_at": a.StartAt, "end_at": a.EndAt, "status": a.Status,
				"guest_name": a.GuestName, "guest_tz": a.GuestTZ})
		case http.MethodPost:
			if a.Status != "confirmed" {
				writeJSON(w, map[string]any{"ok": true})
				return
			}
			if err := h.repo.Cancel(ctx, a, false); err != nil {
				httpErr(w, err)
				return
			}
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
		return
	}
	if len(seg) < 1 || seg[0] == "" || len(seg[0]) > 100 {
		http.NotFound(w, r)
		return
	}
	p, err := h.repo.PageBySlug(ctx, seg[0])
	if err != nil {
		httpErr(w, err)
		return
	}
	switch {
	case len(seg) == 1 && r.Method == http.MethodGet:
		name, _, _ := h.repo.ownerInfo(ctx, p)
		writeJSON(w, publicPage{Slug: p.Slug, Title: p.Title, Description: p.Description, Location: p.Location,
			DurationMin: p.DurationMin, Timezone: p.Timezone, Questions: p.Questions, HasMeet: p.CreateMeet,
			OwnerName: name, MaxDaysAhead: p.MaxDaysAhead})
	case len(seg) == 2 && seg[1] == "slots" && r.Method == http.MethodGet:
		from, err1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		to, err2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err1 != nil || err2 != nil || !to.After(from) {
			http.Error(w, "from and to (RFC 3339) are required", http.StatusBadRequest)
			return
		}
		slots, err := h.repo.Slots(ctx, p, from, to)
		if err != nil {
			httpErr(w, err)
			return
		}
		out := make([]string, 0, len(slots))
		for _, s := range slots {
			out = append(out, s.Format(time.RFC3339))
		}
		writeJSON(w, map[string]any{"slots": out, "duration_min": p.DurationMin})
	case len(seg) == 2 && seg[1] == "book" && r.Method == http.MethodPost:
		if !h.bookGuard.Allow(clientKey(r)) {
			w.Header().Set("Retry-After", "3600")
			http.Error(w, "too many bookings from your network; try again later", http.StatusTooManyRequests)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		var in struct {
			Start   string            `json:"start"`
			Name    string            `json:"name"`
			Email   string            `json:"email"`
			TZ      string            `json:"tz"`
			Answers map[string]string `json:"answers"`
			Website string            `json:"website"` // honeypot
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(in.Website) != "" {
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		start, err := time.Parse(time.RFC3339, in.Start)
		if err != nil {
			http.Error(w, "start must be an RFC 3339 time", http.StatusBadRequest)
			return
		}
		if !h.emailGuard.Allow(strings.ToLower(in.Email)) {
			http.Error(w, "too many bookings with this email today", http.StatusTooManyRequests)
			return
		}
		a, err := h.repo.Book(ctx, p, BookInput{Start: start, Name: in.Name, Email: in.Email, TZ: in.TZ, Answers: in.Answers})
		if err != nil {
			httpErr(w, err)
			return
		}
		out := map[string]any{"ok": true, "start_at": a.StartAt, "end_at": a.EndAt, "cancel_token": a.CancelToken, "email_sent": h.repo.Deps.Mail != nil}
		if a.MeetCode != "" && h.repo.Deps.BaseURL != "" {
			out["meet_url"] = h.repo.Deps.BaseURL + "/meet/" + a.MeetCode
		}
		writeJSON(w, out)
	default:
		http.NotFound(w, r)
	}
}
