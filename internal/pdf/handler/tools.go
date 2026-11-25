package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	adrientauth "github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pdf/database"
	pdfgen "github.com/Adrient-tech/Adrient-Workspace/internal/pdf/pdf"
)

// maxToolPDF bounds the request body for the stateless PDF tools.
const maxToolPDF = 100 << 20

// ToolsHandler serves stateless PDF operations that run on this server (no
// document row needed): true content removal for redaction and in-place text
// editing, and compression. The PDF travels in the request body and the
// result in the response body.
type ToolsHandler struct{}

func NewToolsHandler() *ToolsHandler { return &ToolsHandler{} }

func readPDFBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxToolPDF))
	if err != nil {
		http.Error(w, "pdf too large or unreadable", http.StatusRequestEntityTooLarge)
		return nil, false
	}
	if !bytes.HasPrefix(bytes.TrimLeft(body, "\x00\t\r\n "), []byte("%PDF-")) {
		http.Error(w, "body is not a PDF", http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

// RemoveContent handles POST /api/tools/remove-content?regions=<json array>.
// The response carries the rewritten PDF and an X-Removed-Content header with
// per-page counts as JSON.
func (h *ToolsHandler) RemoveContent(w http.ResponseWriter, r *http.Request) {
	var regions []pdfgen.RemoveRegion
	if err := json.Unmarshal([]byte(r.URL.Query().Get("regions")), &regions); err != nil || len(regions) == 0 || len(regions) > 2000 {
		http.Error(w, "regions: want a JSON array of 1..2000 {page,x,y,w,h}", http.StatusBadRequest)
		return
	}
	body, ok := readPDFBody(w, r)
	if !ok {
		return
	}
	out, stats, err := pdfgen.RemoveContent(body, regions)
	if err != nil {
		slog.Warn("pdf tools: remove content failed", "error", err)
		http.Error(w, "could not rewrite this PDF: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	sj, _ := json.Marshal(stats)
	w.Header().Set("X-Removed-Content", string(sj))
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(out)
}

// PageFonts handles POST /api/tools/page-fonts?page=N: lists the fonts of a
// page with their embedded TrueType/OpenType programs (base64 in "file") so
// the editor can redraw edited text in the document's own font.
func (h *ToolsHandler) PageFonts(w http.ResponseWriter, r *http.Request) {
	var pageNr int
	if err := json.Unmarshal([]byte(r.URL.Query().Get("page")), &pageNr); err != nil || pageNr < 1 {
		http.Error(w, "page: want a 1-based page number", http.StatusBadRequest)
		return
	}
	body, ok := readPDFBody(w, r)
	if !ok {
		return
	}
	fonts, err := pdfgen.PageFonts(body, pageNr)
	if err != nil {
		http.Error(w, "could not read fonts: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"fonts": fonts})
}

// Compress handles POST /api/tools/compress: removes duplicate fonts, images
// and unused objects and recompresses streams.
func (h *ToolsHandler) Compress(w http.ResponseWriter, r *http.Request) {
	body, ok := readPDFBody(w, r)
	if !ok {
		return
	}
	conf := model.NewDefaultConfiguration()
	conf.WriteObjectStream = true
	conf.WriteXRefStream = true
	var out bytes.Buffer
	if err := api.Optimize(bytes.NewReader(body), &out, conf); err != nil {
		http.Error(w, "could not compress this PDF: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	res := out.Bytes()
	if len(res) >= len(body) {
		res = body // never hand back a bigger file
	}
	w.Header().Set("X-Original-Size", itoa(len(body)))
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(res)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// SignaturesHandler keeps each user's saved signatures and initials.
type SignaturesHandler struct {
	db *database.DB
}

func NewSignaturesHandler(db *database.DB) *SignaturesHandler { return &SignaturesHandler{db: db} }

type savedSignature struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Label     string    `json:"label"`
	ImagePNG  string    `json:"imagePng"`
	CreatedAt time.Time `json:"createdAt"`
}

const (
	maxSignaturePNG     = 512 << 10
	maxSignaturesPerKey = 20
)

func callerEmail(r *http.Request) string {
	if u, ok := adrientauth.UserFromContext(r.Context()); ok {
		return strings.ToLower(u.Email)
	}
	return ""
}

// List handles GET /api/user/signatures.
func (h *SignaturesHandler) List(w http.ResponseWriter, r *http.Request) {
	email := callerEmail(r)
	if email == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rows, err := h.db.Pool().Query(r.Context(),
		`SELECT id::text, kind, label, image_png, created_at FROM user_signatures
		 WHERE lower(user_email) = $1 ORDER BY created_at DESC LIMIT 100`, email)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []savedSignature{}
	for rows.Next() {
		var s savedSignature
		if err := rows.Scan(&s.ID, &s.Kind, &s.Label, &s.ImagePNG, &s.CreatedAt); err != nil {
			http.Error(w, "list failed", http.StatusInternalServerError)
			return
		}
		out = append(out, s)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"signatures": out})
}

// Create handles POST /api/user/signatures {kind, label, imagePng}.
func (h *SignaturesHandler) Create(w http.ResponseWriter, r *http.Request) {
	email := callerEmail(r)
	if email == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in savedSignature
	if err := json.NewDecoder(io.LimitReader(r.Body, maxSignaturePNG+4096)).Decode(&in); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if in.Kind != "signature" && in.Kind != "initials" {
		http.Error(w, "kind must be signature or initials", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(in.ImagePNG, "data:image/png;base64,") || len(in.ImagePNG) > maxSignaturePNG {
		http.Error(w, "imagePng must be a PNG data URL under 512 KB", http.StatusBadRequest)
		return
	}
	if len(in.Label) > 80 {
		in.Label = in.Label[:80]
	}
	var n int
	if err := h.db.Pool().QueryRow(r.Context(),
		`SELECT count(*) FROM user_signatures WHERE lower(user_email) = $1 AND kind = $2`, email, in.Kind).Scan(&n); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	if n >= maxSignaturesPerKey {
		http.Error(w, "too many saved items; delete one first", http.StatusConflict)
		return
	}
	err := h.db.Pool().QueryRow(r.Context(),
		`INSERT INTO user_signatures (user_email, kind, label, image_png) VALUES ($1, $2, $3, $4)
		 RETURNING id::text, created_at`, email, in.Kind, in.Label, in.ImagePNG).Scan(&in.ID, &in.CreatedAt)
	if err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(in)
}

// Delete handles DELETE /api/user/signatures/{id}; only the owner's rows match.
func (h *SignaturesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	email := callerEmail(r)
	if email == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var id string
	err := h.db.Pool().QueryRow(r.Context(),
		`DELETE FROM user_signatures WHERE id::text = $1 AND lower(user_email) = $2 RETURNING id::text`,
		r.PathValue("id"), email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
