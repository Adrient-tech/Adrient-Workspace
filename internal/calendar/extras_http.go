package calendar

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// ExtrasAuth extracts the caller from the request.
type ExtrasAuth struct {
	UserID    func(r *http.Request) (string, bool)
	OrgID     func(r *http.Request) (string, bool)
	UserEmail func(r *http.Request) string
}

// ExtrasHandler serves calendar extras (kept off /api/v1/calendar/ so the
// grpc-gateway routes are untouched):
//
//	GET/PUT /api/v1/calx/settings        working hours and time zone
//	POST    /api/v1/calx/find-time       suggest times everyone is free
//	GET     /api/v1/calx/export.ics      download own events
//	POST    /api/v1/calx/import          import ICS text
//	GET     /api/v1/calx/feed            private subscribe URL token
//	POST    /api/v1/calx/feed/rotate     replace the token
type ExtrasHandler struct {
	repo *Repository
	a    ExtrasAuth
}

// NewExtrasHandler builds the handler.
func NewExtrasHandler(repo *Repository, a ExtrasAuth) *ExtrasHandler {
	return &ExtrasHandler{repo: repo, a: a}
}

const extrasMount = "/api/v1/calx/"

// Match reports whether path belongs to the extras API.
func (h *ExtrasHandler) Match(path string) bool { return strings.HasPrefix(path, extrasMount) }

func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (h *ExtrasHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok1 := h.a.OrgID(r)
	user, ok2 := h.a.UserID(r)
	if !ok1 || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, extrasMount)
	switch {
	case path == "settings" && r.Method == http.MethodGet:
		s, err := h.repo.GetSettings(ctx, org, user)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		jsonOut(w, s)
	case path == "settings" && r.Method == http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		var s Settings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if s.Hours.Days == nil {
			s.Hours.Days = map[string][]HoursWindow{}
		}
		if err := h.repo.SaveSettings(ctx, org, user, s); err != nil {
			if strings.Contains(err.Error(), "SQLSTATE") {
				http.Error(w, "internal error", 500)
				return
			}
			http.Error(w, err.Error(), 400)
			return
		}
		jsonOut(w, s)
	case path == "find-time" && r.Method == http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		var in struct {
			Emails      []string `json:"emails"`
			From        string   `json:"from"`
			To          string   `json:"to"`
			DurationMin int      `json:"duration_min"`
			Limit       int      `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		from, e1 := time.Parse(time.RFC3339, in.From)
		to, e2 := time.Parse(time.RFC3339, in.To)
		if e1 != nil || e2 != nil || !to.After(from) || in.DurationMin < 5 || in.DurationMin > 480 {
			http.Error(w, "from, to (RFC 3339) and duration_min (5 to 480) are required", 400)
			return
		}
		if in.Limit <= 0 || in.Limit > 20 {
			in.Limit = 8
		}
		step := 30 * time.Minute
		if in.DurationMin%60 == 0 {
			step = time.Hour
		}
		sug, unknown, err := h.repo.FindTime(ctx, org, user, h.a.UserEmail(r), in.Emails, from, to, time.Duration(in.DurationMin)*time.Minute, step, in.Limit)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		type item struct {
			Start     time.Time `json:"start"`
			End       time.Time `json:"end"`
			Conflicts []string  `json:"conflicts"`
		}
		out := make([]item, 0, len(sug))
		for _, s := range sug {
			c := s.Conflicts
			if c == nil {
				c = []string{}
			}
			out = append(out, item{s.Start, s.End, c})
		}
		if unknown == nil {
			unknown = []string{}
		}
		jsonOut(w, map[string]any{"suggestions": out, "not_in_org": unknown})
	case path == "export.ics" && r.Method == http.MethodGet:
		evs, err := h.repo.ListOwned(ctx, org, user)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="adrient-calendar.ics"`)
		_, _ = io.WriteString(w, BuildCalendarICS("Adrient Calendar", evs, time.Now()))
	case path == "import" && r.Method == http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "file too large (limit 5 MB)", http.StatusRequestEntityTooLarge)
			return
		}
		txt := string(b)
		if !strings.Contains(strings.ToUpper(txt), "BEGIN:VCALENDAR") {
			http.Error(w, "this does not look like an .ics file", 400)
			return
		}
		evs, skipped := ParseICS(txt)
		res, err := h.repo.ImportICS(ctx, org, user, evs, skipped)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		jsonOut(w, res)
	case path == "feed" && r.Method == http.MethodGet, path == "feed/rotate" && r.Method == http.MethodPost:
		t, err := h.repo.FeedToken(ctx, org, user, path == "feed/rotate")
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		jsonOut(w, map[string]string{"token": t, "path": "/api/v1/public/calendar/feed/" + t + ".ics"})
	default:
		http.NotFound(w, r)
	}
}

// FeedHandler serves the private subscribe feed without a session:
// GET /api/v1/public/calendar/feed/{token}.ics
type FeedHandler struct{ repo *Repository }

// NewFeedHandler builds the feed handler.
func NewFeedHandler(repo *Repository) *FeedHandler { return &FeedHandler{repo: repo} }

const feedMount = "/api/v1/public/calendar/feed/"

// Match reports whether path is a feed URL.
func (h *FeedHandler) Match(path string) bool { return strings.HasPrefix(path, feedMount) }

func (h *FeedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", 405)
		return
	}
	token := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, feedMount), ".ics")
	org, user, name, err := h.repo.FeedByToken(r.Context(), token)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	evs, err := h.repo.ListOwned(r.Context(), org, user)
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = io.WriteString(w, BuildCalendarICS(name+" (Adrient)", evs, time.Now()))
}
