package crm

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// AuthFuncs supplies the caller's identity from the request context.
type AuthFuncs struct {
	UserID func(r *http.Request) (string, bool)
	OrgID  func(r *http.Request) (string, bool)
	// IsAdmin reports whether the caller administers the org (sees every record, edits configuration).
	IsAdmin func(r *http.Request) bool
}

// HTTPHandler serves /api/v1/crm/* inside the auth wall.
type HTTPHandler struct {
	repo *Repository
	auth AuthFuncs
}

// NewHTTPHandler constructs the authenticated handler.
func NewHTTPHandler(repo *Repository, auth AuthFuncs) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth}
}

const mount = "/api/v1/crm"

// Match reports whether path belongs to the CRM API.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok := h.auth.OrgID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/"), "/")
	user, _ := h.auth.UserID(r)
	scope := ScopeAll
	if user != "" && !(h.auth.IsAdmin != nil && h.auth.IsAdmin(r)) {
		sc, err := h.repo.ResolveScope(r.Context(), org, user)
		if err != nil {
			httpErr(w, err)
			return
		}
		scope = sc
	}
	ctx := WithViewer(r.Context(), Viewer{UserID: user, Scope: scope})
	r = r.WithContext(ctx)
	m := r.Method
	n := len(seg)
	res := seg[0]
	id := ""
	if n >= 2 {
		id = seg[1]
	}
	list := n == 1 && m == http.MethodGet
	create := n == 1 && m == http.MethodPost
	update := n == 2 && m == http.MethodPut
	remove := n == 2 && m == http.MethodDelete

	switch res {
	case "settings":
		if list {
			s, err := h.repo.GetSettings(ctx, org)
			reply(w, s, err)
		} else if n == 1 && m == http.MethodPut {
			var s Settings
			if decode(w, r, &s) {
				v, err := h.repo.SaveSettings(ctx, org, s)
				reply(w, v, err)
			}
		} else {
			http.NotFound(w, r)
		}
	case "companies":
		switch {
		case list:
			raw, err := h.repo.ListCompaniesPage(ctx, org, listOptsFrom(r))
			replyRaw(w, "companies", raw, err)
		case create || update:
			var c Company
			if decode(w, r, &c) {
				raw, err := h.repo.SaveCompany(ctx, org, id, c)
				replyRawBody(w, raw, err)
			}
		case remove:
			replyOK(w, h.repo.DeleteCompany(ctx, org, id))
		default:
			http.NotFound(w, r)
		}
	case "people":
		switch {
		case list:
			raw, err := h.repo.ListPeoplePage(ctx, org, listOptsFrom(r))
			replyRaw(w, "people", raw, err)
		case create || update:
			var p Person
			if decode(w, r, &p) {
				raw, err := h.repo.SavePerson(ctx, org, id, p)
				replyRawBody(w, raw, err)
			}
		case remove:
			replyOK(w, h.repo.DeletePerson(ctx, org, id))
		default:
			http.NotFound(w, r)
		}
	case "deals":
		switch {
		case list:
			raw, err := h.repo.ListDealsPage(ctx, org, listOptsFrom(r))
			replyRaw(w, "deals", raw, err)
		case create || update:
			var d Deal
			if decode(w, r, &d) {
				raw, err := h.repo.SaveDeal(ctx, org, id, d)
				replyRawBody(w, raw, err)
			}
		case n == 2 && m == http.MethodGet:
			raw, err := h.repo.GetDeal(ctx, org, id)
			replyRawBody(w, raw, err)
		case remove:
			replyOK(w, h.repo.DeleteDeal(ctx, org, id))
		case n == 3 && seg[2] == "checklist" && m == http.MethodGet:
			items, err := h.repo.ListChecklist(ctx, org, id)
			reply(w, map[string]any{"items": items}, err)
		case n == 3 && seg[2] == "checklist" && m == http.MethodPost:
			var b struct {
				Stage    string `json:"stage"`
				Position int    `json:"position"`
				Done     bool   `json:"done"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.SetChecklistDone(ctx, org, id, b.Stage, b.Position, b.Done))
			}
		case n == 3 && seg[2] == "stage" && m == http.MethodPost:
			var b struct {
				Stage  string `json:"stage"`
				Reason string `json:"reason"`
			}
			if decode(w, r, &b) {
				raw, err := h.repo.MoveDeal(ctx, org, id, b.Stage, b.Reason)
				replyRawBody(w, raw, err)
			}
		default:
			http.NotFound(w, r)
		}
	case "summary":
		if list {
			s, err := h.repo.GetSummary(ctx, org)
			replyRawBody(w, mustJSON(s), err)
		} else {
			http.NotFound(w, r)
		}
	case "history":
		if list {
			raw, err := h.repo.ListHistory(ctx, org)
			replyRaw(w, "history", raw, err)
		} else {
			http.NotFound(w, r)
		}
	case "followups":
		switch {
		case list:
			raw, err := h.repo.ListFollowups(ctx, org)
			replyRaw(w, "followups", raw, err)
		case create || update:
			var f Followup
			if decode(w, r, &f) {
				raw, err := h.repo.SaveFollowup(ctx, org, id, f)
				replyRawBody(w, raw, err)
			}
		case remove:
			replyOK(w, h.repo.DeleteFollowup(ctx, org, id))
		default:
			http.NotFound(w, r)
		}
	case "activities":
		switch {
		case list:
			raw, err := h.repo.ListActivities(ctx, org)
			replyRaw(w, "activities", raw, err)
		case create:
			var a Activity
			if decode(w, r, &a) {
				raw, err := h.repo.SaveActivity(ctx, org, a)
				replyRawBody(w, raw, err)
			}
		case remove:
			replyOK(w, h.repo.DeleteActivity(ctx, org, id))
		default:
			http.NotFound(w, r)
		}
	default:
		if !h.serveExt(w, r, org, user, seg) {
			http.NotFound(w, r)
		}
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(v); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func httpErr(w http.ResponseWriter, err error) {
	var pe *pgconn.PgError
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid: "), http.StatusBadRequest)
	case errors.As(err, &pe) && (pe.Code == "22P02" || pe.Code == "23503" || pe.Code == "22007" || pe.Code == "22008"):
		http.Error(w, "invalid reference or value", http.StatusBadRequest)
	default:
		slog.Error("crm", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, v)
}

func replyOK(w http.ResponseWriter, err error) {
	reply(w, map[string]any{"ok": true}, err)
}

// replyRaw wraps a JSON array as {key: [...]}.
func replyRaw(w http.ResponseWriter, key string, raw json.RawMessage, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]json.RawMessage{key: raw})
}

func replyRawBody(w http.ResponseWriter, raw json.RawMessage, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, raw)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
