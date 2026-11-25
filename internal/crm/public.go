package crm

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const publicMount = "/api/v1/public/crm/lead/"

// PublicHandler serves the embeddable web-to-lead form without a session.
// The only credential is the form's unguessable token.
type PublicHandler struct {
	repo *Repository
	mu   sync.Mutex
	hits map[string][]time.Time
}

// NewPublicHandler constructs the handler.
func NewPublicHandler(repo *Repository) *PublicHandler {
	return &PublicHandler{repo: repo, hits: map[string][]time.Time{}}
}

// Match reports whether path is a public lead form request.
func (p *PublicHandler) Match(path string) bool { return strings.HasPrefix(path, publicMount) }

// allow permits 10 submissions per IP per minute.
func (p *PublicHandler) allow(ip string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	keep := p.hits[ip][:0]
	for _, t := range p.hits[ip] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 10 {
		p.hits[ip] = keep
		return false
	}
	p.hits[ip] = append(keep, now)
	if len(p.hits) > 10000 {
		p.hits = map[string][]time.Time{}
	}
	return true
}

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

var formPage = template.Must(template.New("f").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Name}}</title>
<style>body{font:15px/1.4 system-ui,sans-serif;margin:0;padding:16px;color:#1b2a27}form{max-width:460px}
label{display:block;font-weight:600;margin:12px 0 4px}input,textarea{width:100%;box-sizing:border-box;padding:9px 10px;border:1px solid #b8c4c1;border-radius:8px;font:inherit}
button{margin-top:16px;background:#126C5A;color:#fff;border:0;border-radius:8px;padding:10px 18px;font:inherit;font-weight:600;cursor:pointer}
.hp{position:absolute;left:-9999px;height:0;overflow:hidden}.msg{padding:12px;border-radius:8px;background:#e3f3ef;display:none;margin-top:14px}.err{background:#fde8e8}</style></head>
<body><form id="f" method="post" action="">
{{range .Fields}}<label for="{{.Key}}">{{.Label}}{{if .Required}} *{{end}}</label>
{{if eq .Key "message"}}<textarea id="{{.Key}}" name="{{.Key}}" rows="4" {{if .Required}}required{{end}}></textarea>
{{else}}<input id="{{.Key}}" name="{{.Key}}" type="{{if eq .Key "email"}}email{{else if eq .Key "phone"}}tel{{else}}text{{end}}" {{if .Required}}required{{end}}>{{end}}{{end}}
<div class="hp" aria-hidden="true"><label>Leave blank<input name="website_url" tabindex="-1" autocomplete="off"></label></div>
<button type="submit">Send</button><div id="m" class="msg" role="status"></div></form>
<script>
document.getElementById('f').addEventListener('submit',function(e){e.preventDefault();var d={};new FormData(e.target).forEach(function(v,k){d[k]=v});
fetch('',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(d)}).then(function(r){return r.json().catch(function(){return{}}).then(function(j){return[r.ok,j]})})
.then(function(x){var m=document.getElementById('m');m.style.display='block';m.className='msg'+(x[0]?'':' err');m.textContent=x[0]?(x[1].message||'Thanks!'):(x[1].error||'Could not send, please try again.');if(x[0])e.target.reset()})
.catch(function(){var m=document.getElementById('m');m.style.display='block';m.className='msg err';m.textContent='Could not send, please try again.'})});
</script></body></html>`))

func (p *PublicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := strings.Trim(strings.TrimPrefix(r.URL.Path, publicMount), "/")
	if token == "" || strings.Contains(token, "/") || len(token) > 64 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Del("X-Frame-Options")
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		f, _, _, err := p.repo.LeadFormByToken(r.Context(), token)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; frame-ancestors *")
		_ = formPage.Execute(w, f)
	case http.MethodPost:
		if !p.allow(clientIP(r)) {
			jsonErr(w, http.StatusTooManyRequests, "Too many submissions, please try again in a minute.")
			return
		}
		values := map[string]string{}
		ct := r.Header.Get("Content-Type")
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if strings.HasPrefix(ct, "application/json") {
			var raw map[string]any
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid request")
				return
			}
			for k, v := range raw {
				if s, ok := v.(string); ok {
					values[k] = s
				}
			}
		} else {
			if err := r.ParseForm(); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid request")
				return
			}
			for k, v := range r.PostForm {
				if len(v) > 0 {
					values[k] = v[0]
				}
			}
		}
		if values["website_url"] != "" { // honeypot: pretend success, store nothing
			writeJSON(w, map[string]any{"ok": true, "message": "Thanks, we will be in touch."})
			return
		}
		sub := values["_submission"]
		if sub == "" {
			b := make([]byte, 12)
			_, _ = rand.Read(b)
			sub = hex.EncodeToString(b)
		}
		_, thanks, err := p.repo.SubmitLeadForm(r.Context(), token, values, sub)
		switch {
		case errors.Is(err, ErrNotFound):
			http.NotFound(w, r)
		case errors.Is(err, ErrInvalid):
			jsonErr(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "invalid: "))
		case err != nil:
			jsonErr(w, http.StatusInternalServerError, "Could not save your details, please try again.")
		default:
			writeJSON(w, map[string]any{"ok": true, "message": thanks})
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
