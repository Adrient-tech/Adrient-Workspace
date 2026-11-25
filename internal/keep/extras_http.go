package keep

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ExtrasAuth extracts the caller from the request.
type ExtrasAuth struct {
	UserID func(r *http.Request) (string, bool)
	OrgID  func(r *http.Request) (string, bool)
}

// ExtrasHandler serves Keep extras under /api/v1/kx/ (kept off the
// grpc-gateway /api/v1/keep routes):
//
//	GET    /kx/index                       notebooks, filing and attachments
//	POST   /kx/notebooks {name}            PUT /kx/notebooks/{id}   DELETE /kx/notebooks/{id}
//	PUT    /kx/notes/{id}/notebook         {notebook_id} ("" to unfile)
//	POST   /kx/notes/{id}/attachments?kind=image|drawing   raw image bytes
//	GET    /kx/attachments/{id}            image bytes
//	DELETE /kx/attachments/{id}
type ExtrasHandler struct {
	repo *Repository
	svc  *Service
	a    ExtrasAuth
}

// NewExtrasHandler builds the handler. svc supplies the note access rules
// (owner, org member, or per-note grant).
func NewExtrasHandler(repo *Repository, svc *Service, a ExtrasAuth) *ExtrasHandler {
	return &ExtrasHandler{repo: repo, svc: svc, a: a}
}

const kxMount = "/api/v1/kx/"

// Match reports whether path belongs to this handler.
func (h *ExtrasHandler) Match(path string) bool { return strings.HasPrefix(path, kxMount) }

func out(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	if s, ok := status.FromError(err); ok && s.Code() != codes.OK {
		switch s.Code() {
		case codes.NotFound:
			http.Error(w, "not found", http.StatusNotFound)
		case codes.PermissionDenied:
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid input: "), http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// canEdit reports whether a role may change a note.
func canEdit(role string) bool { return role == "owner" || role == "editor" }

func (h *ExtrasHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok1 := h.a.OrgID(r)
	user, ok2 := h.a.UserID(r)
	if !ok1 || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, kxMount), "/"), "/")
	switch {
	case len(seg) == 1 && seg[0] == "index" && r.Method == http.MethodGet:
		w.Header().Set("Cache-Control", "no-store")
		idx, err := h.repo.BuildIndex(ctx, org, user)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, idx)
	case len(seg) == 1 && seg[0] == "notebooks" && r.Method == http.MethodPost:
		var in struct{ Name string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		nb, err := h.repo.CreateNotebook(ctx, org, user, in.Name)
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		out(w, nb)
	case len(seg) == 2 && seg[0] == "notebooks" && r.Method == http.MethodPut:
		var in struct{ Name string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		nb, err := h.repo.RenameNotebook(ctx, org, user, seg[1], in.Name)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, nb)
	case len(seg) == 2 && seg[0] == "notebooks" && r.Method == http.MethodDelete:
		if err := h.repo.DeleteNotebook(ctx, org, user, seg[1]); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 3 && seg[0] == "notes" && seg[2] == "notebook" && r.Method == http.MethodPut:
		var in struct {
			NotebookID string `json:"notebook_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if err := h.repo.MoveNote(ctx, org, user, seg[1], in.NotebookID); err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]bool{"ok": true})
	case len(seg) == 3 && seg[0] == "notes" && seg[2] == "attachments" && r.Method == http.MethodPost:
		_, role, err := h.svc.accessNote(ctx, org, user, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		if !canEdit(role) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxAttachmentBytes+1))
		if err != nil {
			http.Error(w, "file too large (limit 2 MB)", http.StatusRequestEntityTooLarge)
			return
		}
		ref, err := h.repo.AddAttachment(ctx, org, seg[1], r.URL.Query().Get("kind"), b)
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		out(w, ref)
	case len(seg) == 2 && seg[0] == "attachments" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		noteID, mime, data, err := h.repo.GetAttachment(ctx, org, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		if _, _, err := h.svc.accessNote(ctx, org, user, noteID); err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		_, _ = w.Write(data)
	case len(seg) == 2 && seg[0] == "attachments" && r.Method == http.MethodDelete:
		noteID, _, _, err := h.repo.GetAttachment(ctx, org, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		_, role, err := h.svc.accessNote(ctx, org, user, noteID)
		if err != nil {
			fail(w, err)
			return
		}
		if !canEdit(role) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := h.repo.DeleteAttachment(ctx, org, seg[1]); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}
