package talent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// AbuseGuard is a sliding-window counter per key, used for the public apply form.
type AbuseGuard struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
	now    func() time.Time
}

// NewAbuseGuard allows `limit` events per key per window.
func NewAbuseGuard(limit int, window time.Duration) *AbuseGuard {
	return &AbuseGuard{hits: map[string][]time.Time{}, limit: limit, window: window, now: time.Now}
}

// Allow records an event and reports whether it is within the limit.
func (g *AbuseGuard) Allow(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
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
	if len(g.hits) > 20000 { // bound memory under attack
		for k, v := range g.hits {
			if len(v) == 0 || !v[len(v)-1].After(cut) {
				delete(g.hits, k)
			}
		}
	}
	return true
}

// MaxResumeBytes caps a resume upload.
const MaxResumeBytes = 5 << 20

var resumeTypes = map[string]string{
	".pdf": "application/pdf", ".doc": "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".odt":  "application/vnd.oasis.opendocument.text", ".rtf": "application/rtf", ".txt": "text/plain",
}

// CheckResume validates the name, size and content signature of a resume and
// returns the mime type to store (never the client-declared one).
func CheckResume(name string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", invalid("the resume file is empty")
	}
	if len(data) > MaxResumeBytes {
		return "", invalid("the resume is larger than 5 MB")
	}
	ext := strings.ToLower(filepath.Ext(name))
	mime, ok := resumeTypes[ext]
	if !ok {
		return "", invalid("resume must be a PDF, Word, ODT, RTF or text file")
	}
	okSig := false
	switch ext {
	case ".pdf":
		okSig = bytes.HasPrefix(data, []byte("%PDF-"))
	case ".docx", ".odt":
		okSig = bytes.HasPrefix(data, []byte("PK\x03\x04"))
	case ".doc":
		okSig = bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0})
	case ".rtf":
		okSig = bytes.HasPrefix(data, []byte("{\\rtf"))
	case ".txt":
		okSig = utf8.Valid(data) && !bytes.Contains(data, []byte{0})
	}
	if !okSig {
		return "", invalid("the file content does not match its type")
	}
	return mime, nil
}

// PublicHandler serves the unauthenticated career page API.
//
//	GET  /api/v1/public/talent/{org}/jobs
//	GET  /api/v1/public/talent/{org}/jobs/{id}
//	POST /api/v1/public/talent/{org}/jobs/{id}/apply   (multipart)
//	GET  /api/v1/public/talent/offers/{token}[/pdf]
//	POST /api/v1/public/talent/offers/{token}/respond
type PublicHandler struct {
	repo       *Repository
	ipGuard    *AbuseGuard
	emailGuard *AbuseGuard
	// MinFillMillis is the shortest believable time to fill the form.
	MinFillMillis int64
}

// NewPublicHandler constructs the public handler: 5 applications per IP per
// hour and 3 per email per day.
func NewPublicHandler(repo *Repository) *PublicHandler {
	return &PublicHandler{repo: repo, ipGuard: NewAbuseGuard(5, time.Hour), emailGuard: NewAbuseGuard(3, 24*time.Hour), MinFillMillis: 2500}
}

const publicMount = "/api/v1/public/talent/"

// Match reports whether the path is a public talent route.
func (h *PublicHandler) Match(path string) bool { return strings.HasPrefix(path, publicMount) }

func clientKey(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if fwd := r.Header.Get("X-Real-IP"); fwd != "" && (strings.HasPrefix(ip, "127.") || ip == "::1" || strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") || strings.HasPrefix(ip, "192.168.")) {
		ip = strings.TrimSpace(fwd)
	}
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:8])
}

func (h *PublicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, publicMount), "/"), "/")
	w.Header().Set("Cache-Control", "no-store")
	if seg[0] == "offers" {
		h.offers(w, r, seg)
		return
	}
	if len(seg) < 2 || seg[1] != "jobs" {
		http.NotFound(w, r)
		return
	}
	org, err := h.repo.OrgIDBySlug(ctx, seg[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	st, err := h.repo.GetSettings(ctx, org)
	if err != nil || !st.CareerEnabled {
		http.NotFound(w, r)
		return
	}
	switch {
	case len(seg) == 2 && r.Method == http.MethodGet:
		raw, err := h.repo.PublicJobs(ctx, org, "")
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"title": st.CareerTitle, "blurb": st.CareerBlurb, "privacy_text": st.PrivacyText, "jobs": raw})
	case len(seg) == 3 && r.Method == http.MethodGet:
		raw, err := h.repo.PublicJobs(ctx, org, seg[2])
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"title": st.CareerTitle, "privacy_text": st.PrivacyText, "jobs": raw})
	case len(seg) == 4 && seg[3] == "apply" && r.Method == http.MethodPost:
		h.apply(w, r, org, seg[2])
	default:
		http.NotFound(w, r)
	}
}

func (h *PublicHandler) apply(w http.ResponseWriter, r *http.Request, org, jobID string) {
	if !h.ipGuard.Allow(clientKey(r)) {
		w.Header().Set("Retry-After", "3600")
		http.Error(w, "too many applications from your network; try again later", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxResumeBytes+256<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "invalid form or file too large (resume limit is 5 MB)", http.StatusBadRequest)
		return
	}
	// Honeypot: a hidden field real people never fill. Pretend success.
	if strings.TrimSpace(r.FormValue("website")) != "" {
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	started, _ := strconv.ParseInt(r.FormValue("started_at"), 10, 64)
	if started <= 0 {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if time.Now().UnixMilli()-started < h.MinFillMillis {
		writeJSON(w, map[string]any{"ok": true}) // too fast to be a person; drop quietly
		return
	}
	if r.FormValue("consent") != "true" && r.FormValue("consent") != "on" {
		http.Error(w, "please accept the privacy statement to apply", http.StatusBadRequest)
		return
	}
	in := ApplyInput{JobID: jobID, Name: r.FormValue("name"), Email: r.FormValue("email"), Phone: r.FormValue("phone"),
		Location: r.FormValue("location"), CoverNote: r.FormValue("cover_note"), Source: "career_page",
		SourceDetail: r.FormValue("source"), Consent: true}
	if err := in.Validate(); err != nil {
		httpErr(w, err)
		return
	}
	if !h.emailGuard.Allow(strings.ToLower(in.Email)) {
		http.Error(w, "too many applications with this email today", http.StatusTooManyRequests)
		return
	}
	if f, fh, err := r.FormFile("resume"); err == nil {
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, MaxResumeBytes+1))
		if err != nil {
			http.Error(w, "could not read the resume", http.StatusBadRequest)
			return
		}
		mime, err := CheckResume(fh.Filename, data)
		if err != nil {
			httpErr(w, err)
			return
		}
		in.Resume = &ResumeFile{Name: sanitizeName(fh.Filename), Mime: mime, Data: data}
	}
	// Source: only known values from the public form.
	if !oneOf(in.SourceDetail, "") && len(in.SourceDetail) > 100 {
		in.SourceDetail = in.SourceDetail[:100]
	}
	if _, err := h.repo.CreateApplication(r.Context(), org, "", in); err != nil {
		if errors.Is(err, ErrDuplicate) {
			// Do not reveal whether an email already applied.
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		httpErr(w, err)
		return
	}
	h.repo.notifyNewApplicant(r.Context(), org, jobID, in.Name)
	writeJSON(w, map[string]any{"ok": true})
}

func (h *PublicHandler) offers(w http.ResponseWriter, r *http.Request, seg []string) {
	ctx := r.Context()
	if len(seg) < 2 || len(seg[1]) < 20 {
		http.NotFound(w, r)
		return
	}
	tok := seg[1]
	switch {
	case len(seg) == 2 && r.Method == http.MethodGet:
		p, _, err := h.repo.GetPublicOffer(ctx, tok)
		reply(w, p, err)
	case len(seg) == 3 && seg[2] == "pdf" && r.Method == http.MethodGet:
		data, err := h.repo.PublicOfferPDF(ctx, tok)
		if err != nil {
			httpErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="offer.pdf"`)
		_, _ = w.Write(data)
	case len(seg) == 3 && seg[2] == "respond" && r.Method == http.MethodPost:
		if !h.ipGuard.Allow("offer:" + clientKey(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		var b struct {
			Accept bool   `json:"accept"`
			Name   string `json:"name"`
		}
		if decode(w, r, &b) {
			replyOK(w, h.repo.RespondOffer(ctx, tok, b.Accept, b.Name))
		}
	default:
		http.NotFound(w, r)
	}
}

// OrgIDBySlug resolves an org slug.
func (r *Repository) OrgIDBySlug(ctx context.Context, slug string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug=$1`, slug).Scan(&id)
	return id, mapNoRows(err)
}

// PublicJobs lists open, approved jobs for the career page (one job when id is set).
func (r *Repository) PublicJobs(ctx context.Context, org, id string) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT j.id::text AS id, j.title, j.department, j.location, j.employment_type, j.skills, j.description,
		j.salary_min::float8 AS salary_min, j.salary_max::float8 AS salary_max, j.currency, j.published_at
		FROM adrient.talent_jobs j WHERE j.org_id=$1 AND j.status='open' AND ($2='' OR j.id::text=$2) ORDER BY j.published_at DESC`, org, id)
}

// notifyNewApplicant mails the recruiter and hiring manager.
func (r *Repository) notifyNewApplicant(ctx context.Context, org, jobID, name string) {
	if r.Deps.Mail == nil {
		return
	}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT u.email, j.title FROM adrient.talent_jobs j JOIN adrient.users u ON u.id IN (j.recruiter_id, j.hiring_manager_id)
		WHERE j.id=$1 AND j.org_id=$2`, jobID, org)
	if err != nil {
		return
	}
	var to []string
	var title string
	for rows.Next() {
		var e string
		if rows.Scan(&e, &title) == nil {
			to = append(to, e)
		}
	}
	rows.Close()
	if len(to) > 0 {
		msg := "New application from " + name + " for " + title + "."
		_ = r.Deps.Mail(ctx, to, "New applicant: "+title, "<p>"+htmlEscape(msg)+"</p>", msg)
	}
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
