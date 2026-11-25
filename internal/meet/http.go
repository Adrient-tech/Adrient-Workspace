package meet

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// MeetingJSON is the JSON shape returned by the codes HTTP surface.
type MeetingJSON struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id"`
	OwnerID   string `json:"owner_id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	CreatedAt string `json:"created_at"`

	// Session state (migration 0345): Ended is true once the last participant
	// left, the host ended it, or the room was closed.
	Ended            bool   `json:"ended"`
	EndedAt          string `json:"ended_at,omitempty"`
	EndedReason      string `json:"ended_reason,omitempty"`
	DurationS        int    `json:"duration_s,omitempty"`
	ParticipantCount int    `json:"participant_count,omitempty"`
}

func roomToJSON(r Room) MeetingJSON {
	j := roomJSON(r)
	if r.Ended() {
		j.Ended = true
		j.EndedAt = r.EndedAt.UTC().Format(time.RFC3339)
		j.EndedReason = r.EndedReason
		j.DurationS = r.DurationS()
		j.ParticipantCount = len(r.ParticipantIDs)
	}
	return j
}

func roomJSON(r Room) MeetingJSON {
	return MeetingJSON{
		ID:        r.ID,
		OrgID:     r.OrgID,
		OwnerID:   r.OwnerID,
		Name:      r.Name,
		Code:      r.Code,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// CodesHandler is a pure-HTTP handler for the short-code surface:
//
//	POST /api/v1/meet/codes           → create a meeting + code
//	GET  /api/v1/meet/codes/{code}    → resolve code → meeting
//
// It is mounted inside adrient's auth middleware so the caller's user/org are
// already on the request context (via internal/auth).
type CodesHandler struct {
	repo        *Repository
	orgFromCtx  func(r *http.Request) (orgID string, ok bool)
	userFromCtx func(r *http.Request) (userID string, ok bool)
	// OnRestart runs after the host reopened an ended meeting (live push).
	OnRestart func(orgID, code string)
}

// codesMount is the path prefix for the codes HTTP surface.
const codesMount = "/api/v1/meet/codes"

// NewCodesHandler constructs a CodesHandler. orgFromCtx and userFromCtx extract
// the caller's org/user IDs from the request context (supplied by server.go
// using closures over auth.OrgFromContext / auth.UserFromContext so this
// package stays free of the auth gen/ dependency).
func NewCodesHandler(
	repo *Repository,
	orgFromCtx func(r *http.Request) (orgID string, ok bool),
	userFromCtx func(r *http.Request) (userID string, ok bool),
) *CodesHandler {
	return &CodesHandler{repo: repo, orgFromCtx: orgFromCtx, userFromCtx: userFromCtx}
}

// Match reports whether path is routed to this handler, and returns the code
// segment (empty for the collection route).
func (h *CodesHandler) Match(path string) (code string, ok bool) {
	if path == codesMount || path == codesMount+"/" {
		return "", true
	}
	if strings.HasPrefix(path, codesMount+"/") {
		rest := strings.TrimPrefix(path, codesMount+"/")
		if rest != "" && !strings.Contains(rest, "/") {
			return rest, true
		}
		if c, ok := strings.CutSuffix(rest, "/restart"); ok && c != "" && !strings.Contains(c, "/") {
			return c, true
		}
	}
	return "", false
}

func (h *CodesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	code, _ := h.Match(r.URL.Path)

	switch {
	case r.Method == http.MethodPost && code == "":
		h.handleCreate(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/restart"):
		h.handleRestart(w, r, code)
	case r.Method == http.MethodGet && code != "":
		h.handleResolve(w, r, code)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// handleCreate creates a new meeting with a generated short code.
//
//	POST /api/v1/meet/codes
//	Body (optional JSON): {"name": "..."}
//	Returns 201 + MeetingJSON.
func (h *CodesHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgFromCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, ok := h.userFromCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	room, err := h.repo.Create(r.Context(), orgID, userID, body.Name)
	if err != nil {
		http.Error(w, "create meeting: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(roomToJSON(room))
}

// handleResolve resolves a short code to a meeting.
//
//	GET /api/v1/meet/codes/{code}
//	Returns 200 + MeetingJSON, or 400 / 404.
func (h *CodesHandler) handleResolve(w http.ResponseWriter, r *http.Request, code string) {
	orgID, ok := h.orgFromCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	room, err := h.repo.GetByCode(r.Context(), orgID, code)
	if errors.Is(err, ErrInvalidCode) {
		http.Error(w, "invalid code format", http.StatusBadRequest)
		return
	}
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "meeting not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "resolve code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(roomToJSON(room))
}

// handleRestart reopens an ended meeting for a new session. Only the host
// (the room owner) may do it.
//
//	POST /api/v1/meet/codes/{code}/restart
func (h *CodesHandler) handleRestart(w http.ResponseWriter, r *http.Request, code string) {
	orgID, ok := h.orgFromCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, ok := h.userFromCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	room, err := h.repo.GetByCode(r.Context(), orgID, code)
	if errors.Is(err, ErrInvalidCode) {
		http.Error(w, "invalid code format", http.StatusBadRequest)
		return
	}
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "meeting not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "resolve code: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if room.OwnerID != userID {
		http.Error(w, "only the host can start a new meeting", http.StatusForbidden)
		return
	}
	if err := h.repo.RestartSession(r.Context(), orgID, room.ID); err != nil {
		http.Error(w, "restart: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if h.OnRestart != nil {
		h.OnRestart(orgID, room.Code)
	}
	room.SessionStartedAt, room.EndedAt, room.EndedReason, room.ParticipantIDs = nil, nil, "", nil
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(roomToJSON(room))
}
