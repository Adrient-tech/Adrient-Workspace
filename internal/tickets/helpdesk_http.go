package tickets

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ExtrasHandler serves help-desk extras under /api/v1/ts/ (signed in):
//
//	GET/PUT  /ts/projects/{id}/sla            SLA policies
//	GET      /ts/projects/{id}/sla-state      SLA state per ticket
//	GET/POST /ts/projects/{id}/rules          assignment rules   PUT/DELETE /ts/rules/{id}
//	GET/POST /ts/canned?project=              canned replies     DELETE /ts/canned/{id}
//	GET      /ts/customers?project=&q=        customers grouped by email
//	GET      /ts/tickets/{id}/csat            survey link and answer
//	GET/POST /ts/projects/{id}/kb             articles           PUT/DELETE /ts/kb/{id}
//	GET      /ts/projects/{id}/report?days=   report
type ExtrasHandler struct {
	repo *Repository
	auth AuthFuncs
}

// NewExtrasHandler builds the handler.
func NewExtrasHandler(repo *Repository, a AuthFuncs) *ExtrasHandler {
	return &ExtrasHandler{repo: repo, auth: a}
}

const tsMount = "/api/v1/ts/"

// Match reports whether path belongs to this handler.
func (h *ExtrasHandler) Match(path string) bool { return strings.HasPrefix(path, tsMount) }

func outJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func failHTTP(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid input: "), http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func (h *ExtrasHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok := h.auth.OrgID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, _ := h.auth.UserID(r)
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, tsMount), "/"), "/")
	get, post, put, del := r.Method == http.MethodGet, r.Method == http.MethodPost, r.Method == http.MethodPut, r.Method == http.MethodDelete
	switch {
	// SLA
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "sla" && get:
		v, err := h.repo.SLAPolicies(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"policies": v})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "sla" && put:
		var in struct {
			Policies []SLAPolicy `json:"policies"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := h.repo.SaveSLAPolicies(ctx, org, seg[1], in.Policies); err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"policies": in.Policies})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "sla-state" && get:
		v, err := h.repo.SLAStates(ctx, org, seg[1], time.Now())
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"states": v})
	// rules
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "rules" && get:
		v, err := h.repo.ListRules(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"rules": v})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "rules" && post:
		var x Rule
		if !decode(w, r, &x) {
			return
		}
		x.ID = ""
		v, err := h.repo.SaveRule(ctx, org, seg[1], x)
		if err != nil {
			failHTTP(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		outJSON(w, v)
	case len(seg) == 2 && seg[0] == "rules" && put:
		var in struct {
			Rule
			ProjectID string `json:"project_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		in.ID = seg[1]
		v, err := h.repo.SaveRule(ctx, org, in.ProjectID, in.Rule)
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, v)
	case len(seg) == 2 && seg[0] == "rules" && del:
		if err := h.repo.DeleteRule(ctx, org, seg[1]); err != nil {
			failHTTP(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	// canned
	case len(seg) == 1 && seg[0] == "canned" && get:
		v, err := h.repo.ListCanned(ctx, org, r.URL.Query().Get("project"))
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"canned": v})
	case len(seg) == 1 && seg[0] == "canned" && post:
		var in struct{ ProjectID, Title, Body string }
		if !decode(w, r, &in) {
			return
		}
		v, err := h.repo.CreateCanned(ctx, org, user, in.ProjectID, in.Title, in.Body)
		if err != nil {
			failHTTP(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		outJSON(w, v)
	case len(seg) == 2 && seg[0] == "canned" && del:
		if err := h.repo.DeleteCanned(ctx, org, seg[1]); err != nil {
			failHTTP(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	// customers
	case len(seg) == 1 && seg[0] == "customers" && get:
		v, err := h.repo.Customers(ctx, org, r.URL.Query().Get("project"), r.URL.Query().Get("q"))
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"customers": v})
	// csat
	case len(seg) == 3 && seg[0] == "tickets" && seg[2] == "csat" && get:
		v, err := h.repo.EnsureCSAT(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		info, err := h.repo.CSATFor(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		info.Token = v
		outJSON(w, info)
	// knowledge base
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "kb" && get:
		v, err := h.repo.ListArticles(ctx, org, seg[1], false)
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"articles": v})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "kb" && (post || put):
		var a Article
		if !decode(w, r, &a) {
			return
		}
		if post {
			a.ID = ""
		}
		v, err := h.repo.SaveArticle(ctx, org, user, seg[1], a)
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, v)
	case len(seg) == 2 && seg[0] == "kb" && del:
		if err := h.repo.DeleteArticle(ctx, org, seg[1]); err != nil {
			failHTTP(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	// report
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "report" && get:
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		v, err := h.repo.ReportFor(ctx, org, seg[1], days, time.Now())
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, v)
	default:
		if !h.serveSvc(w, r, org, user, seg) && !h.serveDesk(w, r, org, user, seg) {
			http.NotFound(w, r)
		}
	}
}

// HelpPublicHandler serves the public help centre and satisfaction survey:
//
//	GET  /api/v1/public/help/{projectToken}            published articles
//	GET  /api/v1/public/csat/{token}                   survey
//	POST /api/v1/public/csat/{token}                   {rating, comment}
type HelpPublicHandler struct {
	repo  *Repository
	guard *abuseGuard
}

// NewHelpPublicHandler builds the public handler.
func NewHelpPublicHandler(repo *Repository) *HelpPublicHandler {
	return &HelpPublicHandler{repo: repo, guard: newAbuseGuard(120, time.Minute)}
}

// Match reports whether path is a public help route.
func (h *HelpPublicHandler) Match(path string) bool {
	return strings.HasPrefix(path, "/api/v1/public/help/") || strings.HasPrefix(path, "/api/v1/public/csat/") || strings.HasPrefix(path, "/api/v1/public/track/")
}

func (h *HelpPublicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ip := r.RemoteAddr
	if !h.guard.Allow(ip) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/public/"), "/"), "/")
	switch {
	case len(seg) == 2 && seg[0] == "help" && r.Method == http.MethodGet:
		name, list, err := h.repo.PublicHelp(ctx, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"name": name, "articles": list})
	case len(seg) == 3 && seg[0] == "help" && seg[2] == "forms" && r.Method == http.MethodGet:
		p, err := h.repo.projectByToken(ctx, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		v, err := h.repo.ListForms(ctx, p.OrgID, p.ID, true)
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"forms": v})
	case len(seg) == 5 && seg[0] == "help" && seg[2] == "forms" && seg[4] == "submit" && r.Method == http.MethodPost:
		var in struct {
			Title  string            `json:"title"`
			Name   string            `json:"name"`
			Email  string            `json:"email"`
			Values map[string]string `json:"values"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !h.guard.Allow("submit:" + ip) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		t, ref, err := h.repo.SubmitForm(ctx, seg[1], seg[3], strings.TrimSpace(in.Name), strings.TrimSpace(in.Email), in.Title, in.Values)
		if err != nil {
			failHTTP(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		outJSON(w, map[string]any{"ref": ref, "track_token": h.repo.PortalToken(ctx, t.ID)})
	case len(seg) == 3 && seg[0] == "help" && seg[2] == "suggest" && r.Method == http.MethodGet:
		p, err := h.repo.projectByToken(ctx, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		v, err := h.repo.SuggestArticles(ctx, p.OrgID, p.ID, r.URL.Query().Get("q"), true)
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]any{"articles": v})
	case len(seg) == 4 && seg[0] == "help" && seg[2] == "article" && r.Method == http.MethodPost:
		var in struct {
			Helpful   bool `json:"helpful"`
			Deflected bool `json:"deflected"`
			View      bool `json:"view"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.View {
			h.repo.ArticleView(ctx, seg[1], seg[3])
		} else if err := h.repo.ArticleFeedback(ctx, seg[1], seg[3], in.Helpful, in.Deflected); err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]bool{"ok": true})
	case len(seg) == 2 && seg[0] == "track" && r.Method == http.MethodGet:
		v, err := h.repo.TrackByToken(ctx, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, v)
	case len(seg) == 3 && seg[0] == "track" && seg[2] == "reply" && r.Method == http.MethodPost:
		var in struct {
			Body string `json:"body"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := h.repo.TrackReply(ctx, seg[1], in.Body); err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]bool{"ok": true})
	case len(seg) == 3 && seg[0] == "track" && seg[2] == "close" && r.Method == http.MethodPost:
		if err := h.repo.TrackClose(ctx, seg[1]); err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]bool{"ok": true})
	case len(seg) == 2 && seg[0] == "csat" && r.Method == http.MethodGet:
		v, err := h.repo.CSATByToken(ctx, seg[1])
		if err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, v)
	case len(seg) == 2 && seg[0] == "csat" && r.Method == http.MethodPost:
		var in struct {
			Rating  int    `json:"rating"`
			Comment string `json:"comment"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if _, err := h.repo.CSATByToken(ctx, seg[1]); err != nil {
			failHTTP(w, err)
			return
		}
		if err := h.repo.AnswerCSAT(ctx, seg[1], in.Rating, in.Comment); err != nil {
			failHTTP(w, err)
			return
		}
		outJSON(w, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}
