package contracts

import (
	"net/http"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// NewHTTPHandler serves /api/v1/contracts inside the auth wall.
func NewHTTPHandler(repo *Repository, auth knowkit.Auth) *knowkit.Handler {
	return &knowkit.Handler{Mount: "/api/v1/contracts", Auth: auth, Route: func(q knowkit.Req) bool { return route(repo, q) }}
}

func route(repo *Repository, q knowkit.Req) bool {
	w, r, a, ctx := q.W, q.R, q.A, q.R.Context()
	id := q.ID()
	switch q.Seg[0] {
	case "meta":
		knowkit.JSON(w, map[string]any{"types": Types, "billings": []string{"one-time", "monthly", "quarterly", "annual"}, "today": repo.today()})
	case "overview":
		st, obs, err := repo.Overview(ctx, a)
		knowkit.Reply(w, map[string]any{"stats": st, "obligations": obs}, err)
	case "people":
		rows, err := repo.pool.Query(ctx, `SELECT id::text, COALESCE(NULLIF(display_name,''),email) FROM adrient.users WHERE org_id=$1 AND COALESCE(status,'active')='active' ORDER BY 2 LIMIT 2000`, a.Org)
		if err != nil {
			knowkit.Fail(w, err)
			return true
		}
		defer rows.Close()
		out := []map[string]string{}
		for rows.Next() {
			var i, n string
			_ = rows.Scan(&i, &n)
			out = append(out, map[string]string{"id": i, "name": n})
		}
		knowkit.JSON(w, map[string]any{"people": out, "me": a.User})
	case "parties":
		rows, err := repo.pool.Query(ctx, `SELECT id::text, COALESCE(NULLIF(display_name,''), NULLIF(trim(first_name||' '||last_name),''), company, ''), company FROM adrient.contacts WHERE org_id=$1 AND trashed_at IS NULL AND ($2='' OR display_name ILIKE '%'||$2||'%' OR company ILIKE '%'||$2||'%') ORDER BY lower(display_name) LIMIT 30`, a.Org, r.URL.Query().Get("q"))
		if err != nil {
			knowkit.Fail(w, err)
			return true
		}
		defer rows.Close()
		out := []map[string]string{}
		for rows.Next() {
			var i, n, c string
			_ = rows.Scan(&i, &n, &c)
			out = append(out, map[string]string{"id": i, "name": n, "company": c})
		}
		knowkit.JSON(w, map[string]any{"parties": out})
	case "calendar.ics":
		list, err := repo.List(ctx, a, "", "active")
		if err != nil {
			knowkit.Fail(w, err)
			return true
		}
		obs := map[string][]Obligation{}
		for _, f := range list {
			if o, err := repo.obligations(ctx, a.Org, f.ID); err == nil {
				obs[f.ID] = o
			}
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="contract-deadlines.ics"`)
		_, _ = w.Write([]byte(ICS(list, obs, repo.now(), repo.today(), q.Base)))
	case "contracts":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.List(ctx, a, r.URL.Query().Get("q"), r.URL.Query().Get("status"))
			knowkit.Reply(w, map[string]any{"contracts": l}, err)
		case q.Is(1, http.MethodPost):
			var in Input
			if knowkit.Decode(w, r, &in) {
				f, err := repo.Create(ctx, a, in)
				knowkit.Reply(w, f, err)
			}
		case q.Is(2, http.MethodGet):
			f, err := repo.Get(ctx, a, id)
			knowkit.Reply(w, f, err)
		case q.Is(2, http.MethodPut):
			var in Input
			if knowkit.Decode(w, r, &in) {
				f, err := repo.Update(ctx, a, id, in)
				knowkit.Reply(w, f, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.Delete(ctx, a, id))
		case q.Sub("decision", http.MethodPost):
			var d Decision
			if knowkit.Decode(w, r, &d) {
				f, err := repo.Decide(ctx, a, id, d)
				knowkit.Reply(w, f, err)
			}
		case q.Sub("sync", http.MethodPost):
			if _, err := repo.load(ctx, a, id, true); err != nil {
				knowkit.Fail(w, err)
				return true
			}
			knowkit.OK(w, repo.SyncDeadlines(ctx, a, id))
		case q.Sub("obligations", http.MethodPost):
			var in ObligationInput
			if knowkit.Decode(w, r, &in) {
				o, err := repo.AddObligation(ctx, a, id, in)
				knowkit.Reply(w, o, err)
			}
		case q.Sub("documents", http.MethodPost):
			var in struct {
				FileID string `json:"file_id"`
				Kind   string `json:"kind"`
			}
			if knowkit.Decode(w, r, &in) {
				d, err := repo.AddDocument(ctx, a, id, in.FileID, in.Kind)
				knowkit.Reply(w, d, err)
			}
		case q.N() == 4 && q.Seg[2] == "documents" && q.Method == http.MethodDelete:
			knowkit.OK(w, repo.RemoveDocument(ctx, a, id, q.Seg[3]))
		default:
			return false
		}
	case "obligations":
		switch {
		case q.Is(2, http.MethodPut):
			var in ObligationInput
			if knowkit.Decode(w, r, &in) {
				o, err := repo.UpdateObligation(ctx, a, id, in)
				knowkit.Reply(w, o, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteObligation(ctx, a, id))
		case q.Sub("complete", http.MethodPost):
			o, err := repo.CompleteObligation(ctx, a, id, false)
			knowkit.Reply(w, o, err)
		case q.Sub("reopen", http.MethodPost):
			o, err := repo.CompleteObligation(ctx, a, id, true)
			knowkit.Reply(w, o, err)
		default:
			return false
		}
	default:
		return false
	}
	return true
}
