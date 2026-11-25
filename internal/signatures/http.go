package signatures

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// NewHTTPHandler serves /api/v1/signatures inside the auth wall.
func NewHTTPHandler(repo *Repository, auth knowkit.Auth) *knowkit.Handler {
	return &knowkit.Handler{Mount: "/api/v1/signatures", Auth: auth, Route: func(q knowkit.Req) bool { return route(repo, q) }}
}

func route(repo *Repository, q knowkit.Req) bool {
	w, r, a, ctx := q.W, q.R, q.A, q.R.Context()
	id := q.ID()
	switch q.Seg[0] {
	case "people":
		if q.Is(1, http.MethodGet) {
			rows, err := repo.pool.Query(ctx, `SELECT id::text, COALESCE(NULLIF(display_name,''),email), email FROM adrient.users WHERE org_id=$1 AND COALESCE(status,'active')='active' ORDER BY 2 LIMIT 2000`, a.Org)
			if err != nil {
				knowkit.Fail(w, err)
				return true
			}
			defer rows.Close()
			out := []map[string]string{}
			for rows.Next() {
				var i, n, e string
				_ = rows.Scan(&i, &n, &e)
				out = append(out, map[string]string{"id": i, "name": n, "email": e})
			}
			knowkit.JSON(w, map[string]any{"people": out, "me": a.User})
			return true
		}
	case "contacts":
		if q.Is(1, http.MethodGet) {
			rows, err := repo.pool.Query(ctx, `SELECT id::text, COALESCE(NULLIF(display_name,''), NULLIF(trim(first_name||' '||last_name),''), company, ''), COALESCE(emails->>0,'') FROM adrient.contacts
				WHERE org_id=$1 AND trashed_at IS NULL AND ($2='' OR display_name ILIKE '%'||$2||'%' OR company ILIKE '%'||$2||'%' OR emails::text ILIKE '%'||$2||'%') ORDER BY 2 LIMIT 30`, a.Org, r.URL.Query().Get("q"))
			if err != nil {
				knowkit.Fail(w, err)
				return true
			}
			defer rows.Close()
			out := []map[string]string{}
			for rows.Next() {
				var i, n, e string
				_ = rows.Scan(&i, &n, &e)
				out = append(out, map[string]string{"id": i, "name": n, "email": e})
			}
			knowkit.JSON(w, map[string]any{"contacts": out})
			return true
		}
	case "requests":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.List(ctx, a, r.URL.Query().Get("status"))
			knowkit.Reply(w, map[string]any{"requests": l}, err)
		case q.Is(1, http.MethodPost):
			var in Input
			if knowkit.Decode(w, r, &in) {
				x, err := repo.Create(ctx, a, in)
				knowkit.Reply(w, x, err)
			}
		case q.Is(2, http.MethodGet):
			x, err := repo.Get(ctx, a, id)
			knowkit.Reply(w, x, err)
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.Delete(ctx, a, id))
		case q.Sub("send", http.MethodPost):
			x, err := repo.Send(ctx, a, id, q.Base)
			knowkit.Reply(w, x, err)
		case q.Sub("void", http.MethodPost):
			var in struct {
				Reason string `json:"reason"`
			}
			_ = knowkit.DecodeOptional(r, &in)
			knowkit.OK(w, repo.Void(ctx, a, id, in.Reason))
		case q.Sub("remind", http.MethodPost):
			n, err := repo.Remind(ctx, a, id, q.Base)
			knowkit.Reply(w, map[string]int{"sent": n}, err)
		case q.Sub("trail", http.MethodGet):
			ev, ok, err := repo.Trail(ctx, a, id)
			knowkit.Reply(w, map[string]any{"events": ev, "intact": ok}, err)
		case q.Sub("pdf", http.MethodGet):
			data, name, err := repo.Download(ctx, a, id, r.URL.Query().Get("original") == "1")
			if err != nil {
				knowkit.Fail(w, err)
				return true
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `inline; filename="`+strings.NewReplacer(`"`, "", "\n", "", "\r", "").Replace(name)+`"`)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = w.Write(data)
		default:
			return false
		}
		return true
	}
	return false
}

// ---- public (signer links, no session) ---------------------------------------------

const publicMount = "/api/v1/public/signatures/"

// PublicHandler serves the signer's page API.
type PublicHandler struct {
	repo *Repository
	mu   sync.Mutex
	hits map[string][]time.Time
}

// NewPublicHandler constructs the signer-side handler.
func NewPublicHandler(repo *Repository) *PublicHandler {
	return &PublicHandler{repo: repo, hits: map[string][]time.Time{}}
}

// Match reports whether path belongs to the signer API.
func (h *PublicHandler) Match(path string) bool { return strings.HasPrefix(path, publicMount) }

func (h *PublicHandler) allow(key string, limit int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	l := h.hits[key][:0]
	for _, t := range h.hits[key] {
		if now.Sub(t) < time.Minute {
			l = append(l, t)
		}
	}
	if len(l) >= limit {
		h.hits[key] = l
		return false
	}
	h.hits[key] = append(l, now)
	if len(h.hits) > 20000 {
		for k, v := range h.hits {
			if len(v) == 0 || now.Sub(v[len(v)-1]) > time.Minute {
				delete(h.hits, k)
			}
		}
	}
	return true
}

func clientOf(r *http.Request) Client {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if fwd := r.Header.Get("X-Real-IP"); fwd != "" && (strings.HasPrefix(ip, "127.") || ip == "::1" || strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") || strings.HasPrefix(ip, "192.168.")) {
		ip = strings.TrimSpace(fwd)
	}
	proto := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	return Client{IP: ip, UA: r.UserAgent(), Base: proto + "://" + r.Host}
}

func (h *PublicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	c := clientOf(r)
	if !h.allow(c.IP, 90) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, publicMount), "/"), "/")
	tok := seg[0]
	ctx := r.Context()
	switch {
	case len(seg) == 1 && r.Method == http.MethodGet:
		d, err := h.repo.PublicView(ctx, tok, c)
		knowkit.Reply(w, d, err)
	case len(seg) == 2 && seg[1] == "pdf" && r.Method == http.MethodGet:
		data, name, err := h.repo.PublicPDF(ctx, tok)
		if err != nil {
			knowkit.Fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="`+strings.NewReplacer(`"`, "", "\n", "", "\r", "").Replace(name)+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(data)
	case len(seg) == 2 && seg[1] == "sign" && r.Method == http.MethodPost:
		if !h.allow("sign:"+c.IP, 20) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		var in SignInput
		if knowkit.Decode(w, r, &in) {
			d, err := h.repo.PublicSign(ctx, tok, in, c)
			knowkit.Reply(w, d, err)
		}
	case len(seg) == 2 && seg[1] == "decline" && r.Method == http.MethodPost:
		var in struct {
			Reason string `json:"reason"`
		}
		if knowkit.Decode(w, r, &in) {
			knowkit.OK(w, h.repo.PublicDecline(ctx, tok, in.Reason, c))
		}
	default:
		http.NotFound(w, r)
	}
}
