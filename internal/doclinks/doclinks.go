// Package doclinks stores which Docs/Sheets/Slides document a Drive office
// file (.docx/.xlsx/.pptx) was opened as, so every user and browser opening
// the same Drive file edits the SAME document (migration 0134).
//
// Access follows the Drive file: an org member of the file's org (or a user
// holding an object grant on the file) may read and create the link. A
// cross-org grantee also receives a matching grant on the linked document.
//
//	GET /api/v1/doclinks/{driveFileId}?kind=docs|sheets|slides  -> {doc_id}
//	PUT /api/v1/doclinks/{driveFileId} {kind, doc_id}            -> {doc_id}  (first writer wins)
package doclinks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
)

// Prefix is the path prefix Handler serves.
const Prefix = "/api/v1/doclinks/"

// Match reports whether path belongs to Handler.
func Match(path string) bool { return strings.HasPrefix(path, Prefix) }

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// kinds maps a link kind to its document table and sharing object type.
var kinds = map[string]struct{ table, grant string }{
	"docs":   {"adrient.docs_documents", sharing.TypeDocsDoc},
	"sheets": {"adrient.sheets_documents", sharing.TypeSheetsSheet},
	"slides": {"adrient.slides_documents", sharing.TypeSlidesDeck},
}

// Store persists links.
type Store struct {
	pool   *pgxpool.Pool
	grants *sharing.Repository
}

// NewStore builds a Store. grants may be nil (cross-org grantees then get no doc grant).
func NewStore(pool *pgxpool.Pool, grants *sharing.Repository) *Store {
	return &Store{pool: pool, grants: grants}
}

// access returns the file's org and the caller's role on it ("" = none).
func (s *Store) access(ctx context.Context, orgID, userID, fileID string) (fileOrg, role string, err error) {
	var owner string
	err = s.pool.QueryRow(ctx,
		`SELECT org_id::text, owner_id::text FROM adrient.drive_files WHERE id=$1 AND trashed_at IS NULL`, fileID).Scan(&fileOrg, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if fileOrg == orgID {
		return fileOrg, sharing.RoleEditor, nil // org members have full access in V1
	}
	if s.grants != nil {
		if r, ok, gerr := s.grants.RoleFor(ctx, userID, sharing.TypeDriveFile, fileID); gerr != nil {
			return "", "", gerr
		} else if ok {
			return fileOrg, r, nil
		}
	}
	return "", "", nil
}

// alive reports whether the linked document still exists and is not trashed.
func (s *Store) alive(ctx context.Context, kind, docID, orgID string) bool {
	k := kinds[kind]
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT 1 FROM `+k.table+` WHERE id=$1 AND org_id=$2 AND trashed_at IS NULL`, docID, orgID).Scan(&n)
	return err == nil
}

// Resolve returns the live linked doc id for a Drive file, or "" when none.
// ok=false means the caller has no access to the Drive file.
func (s *Store) Resolve(ctx context.Context, orgID, userID, fileID, kind string) (docID string, ok bool, err error) {
	fileOrg, role, err := s.access(ctx, orgID, userID, fileID)
	if err != nil || role == "" {
		return "", false, err
	}
	err = s.pool.QueryRow(ctx,
		`SELECT doc_id::text FROM adrient.drive_doc_links WHERE drive_file_id=$1 AND doc_kind=$2`, fileID, kind).Scan(&docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	if !s.alive(ctx, kind, docID, fileOrg) {
		return "", true, nil
	}
	s.shareWithGrantee(ctx, fileOrg, orgID, userID, kind, docID, role)
	return docID, true, nil
}

// Link records docID for the Drive file unless a live link exists; either way
// it returns the winning doc id.
func (s *Store) Link(ctx context.Context, orgID, userID, fileID, kind, docID string) (winner string, ok bool, err error) {
	fileOrg, role, err := s.access(ctx, orgID, userID, fileID)
	if err != nil || role == "" {
		return "", false, err
	}
	if !s.alive(ctx, kind, docID, fileOrg) {
		return "", false, errors.New("document not found in the file's org")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialise concurrent first opens of the same file.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 134))`, fileID+":"+kind); err != nil {
		return "", false, err
	}
	var cur string
	err = tx.QueryRow(ctx,
		`SELECT doc_id::text FROM adrient.drive_doc_links WHERE drive_file_id=$1 AND doc_kind=$2`, fileID, kind).Scan(&cur)
	switch {
	case err == nil && cur != docID && s.alive(ctx, kind, cur, fileOrg):
		winner = cur
	case err == nil || errors.Is(err, pgx.ErrNoRows):
		if _, err := tx.Exec(ctx,
			`INSERT INTO adrient.drive_doc_links (drive_file_id, doc_kind, doc_id, org_id, created_by)
			 VALUES ($1,$2,$3,$4,$5)
			 ON CONFLICT (drive_file_id, doc_kind) DO UPDATE SET doc_id=EXCLUDED.doc_id, created_by=EXCLUDED.created_by, created_at=now()`,
			fileID, kind, docID, fileOrg, userID); err != nil {
			return "", false, err
		}
		winner = docID
	default:
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	s.shareWithGrantee(ctx, fileOrg, orgID, userID, kind, winner, role)
	return winner, true, nil
}

// shareWithGrantee gives a cross-org Drive grantee the same role on the doc.
func (s *Store) shareWithGrantee(ctx context.Context, fileOrg, callerOrg, userID, kind, docID, role string) {
	if s.grants == nil || fileOrg == callerOrg {
		return
	}
	_ = s.grants.GrantAccess(ctx, kinds[kind].grant, docID, userID, role, "")
}

// Handler serves the doclinks API.
type Handler struct{ Store *Store }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, uok := auth.UserFromContext(r.Context())
	o, ook := auth.OrgFromContext(r.Context())
	if !uok || !ook {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	fileID := strings.Trim(strings.TrimPrefix(r.URL.Path, Prefix), "/")
	if !uuidRe.MatchString(fileID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		kind := r.URL.Query().Get("kind")
		if _, known := kinds[kind]; !known {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind must be docs, sheets or slides"})
			return
		}
		id, ok, err := h.Store.Resolve(ctx, o.ID, u.ID, fileID, kind)
		switch {
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
		case !ok:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		default:
			writeJSON(w, http.StatusOK, map[string]string{"doc_id": id})
		}
	case http.MethodPut:
		var req struct {
			Kind  string `json:"kind"`
			DocID string `json:"doc_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil ||
			!uuidRe.MatchString(req.DocID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind and doc_id required"})
			return
		}
		if _, known := kinds[req.Kind]; !known {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind must be docs, sheets or slides"})
			return
		}
		id, ok, err := h.Store.Link(ctx, o.ID, u.ID, fileID, req.Kind, req.DocID)
		switch {
		case err != nil:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case !ok:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		default:
			writeJSON(w, http.StatusOK, map[string]string{"doc_id": id})
		}
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
