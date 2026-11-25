package captable

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// NewHTTPHandler serves /api/v1/captable inside the auth wall. Every route needs a named level of access:
// org admins, or people an admin added; reads need view, writes need edit.
func NewHTTPHandler(repo *Repository, auth knowkit.Auth) *knowkit.Handler {
	return &knowkit.Handler{Mount: "/api/v1/captable", Auth: auth, Route: func(q knowkit.Req) bool { return route(repo, q) }}
}

var errRestricted = errors.New("the cap table is restricted to named people; ask an organization admin to add you")

func deny(q knowkit.Req, err error) bool {
	if errors.Is(err, errRestricted) {
		q.W.Header().Set("Content-Type", "application/json")
		q.W.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(q.W).Encode(map[string]string{"error": err.Error()})
		return true
	}
	knowkit.Fail(q.W, err)
	return true
}

func route(repo *Repository, q knowkit.Req) bool {
	w, r, a, ctx := q.W, q.R, q.A, q.R.Context()
	id := q.ID()
	qs := r.URL.Query()
	level := repo.Level(ctx, a)
	// "whoami" tells the page whether it may show anything, without leaking any data.
	if q.Seg[0] == "whoami" {
		knowkit.JSON(w, map[string]any{"level": level, "admin": a.Admin})
		return true
	}
	if level == LevelNone {
		return deny(q, errRestricted)
	}
	write := q.Method != http.MethodGet
	// Model calls are POSTs that change nothing; they only need view access.
	readOnlyPost := q.Seg[0] == "model" && !(len(q.Seg) > 1 && q.Seg[1] == "close-round")
	if write && !readOnlyPost && level != LevelEdit {
		return deny(q, knowkit.ErrForbidden)
	}
	switch q.Seg[0] {
	case "meta":
		c, err := repo.GetCompany(ctx, a.Org)
		knowkit.Reply(w, map[string]any{"company": c, "level": level, "admin": a.Admin, "today": repo.today()}, err)
	case "company":
		switch {
		case q.Is(1, http.MethodGet):
			c, err := repo.GetCompany(ctx, a.Org)
			knowkit.Reply(w, c, err)
		case q.Is(1, http.MethodPut):
			var in Company
			if knowkit.Decode(w, r, &in) {
				c, err := repo.SaveCompany(ctx, a, in)
				knowkit.Reply(w, c, err)
			}
		default:
			return false
		}
	case "dashboard":
		d, err := repo.Dashboard(ctx, a.Org)
		knowkit.Reply(w, d, err)
	case "table":
		t, err := repo.TableOf(ctx, a.Org, qs.Get("as_of"))
		knowkit.Reply(w, t, err)
	case "access":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.AccessList(ctx, a)
			knowkit.Reply(w, map[string]any{"access": l}, err)
		case q.Is(1, http.MethodPost):
			var in struct {
				UserID string `json:"user_id"`
				Level  string `json:"level"`
			}
			if knowkit.Decode(w, r, &in) {
				knowkit.OK(w, repo.GrantAccess(ctx, a, in.UserID, in.Level))
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.RevokeAccess(ctx, a, id))
		default:
			return false
		}
	case "people":
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
		knowkit.JSON(w, map[string]any{"people": out})
	case "stakeholders":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Stakeholders(ctx, a.Org)
			knowkit.Reply(w, map[string]any{"stakeholders": l}, err)
		case q.Is(1, http.MethodPost):
			var in StakeholderInput
			if knowkit.Decode(w, r, &in) {
				s, err := repo.CreateStakeholder(ctx, a, in)
				knowkit.Reply(w, s, err)
			}
		case q.Sub("statement", http.MethodGet):
			s, err := repo.HolderStatement(ctx, a.Org, id)
			knowkit.Reply(w, s, err)
		case q.Is(2, http.MethodPut):
			var in StakeholderInput
			if knowkit.Decode(w, r, &in) {
				s, err := repo.UpdateStakeholder(ctx, a, id, in)
				knowkit.Reply(w, s, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteStakeholder(ctx, a, id))
		default:
			return false
		}
	case "classes":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Classes(ctx, a.Org)
			knowkit.Reply(w, map[string]any{"classes": l}, err)
		case q.Is(1, http.MethodPost):
			var in ClassInput
			if knowkit.Decode(w, r, &in) {
				c, err := repo.CreateClass(ctx, a, in)
				knowkit.Reply(w, c, err)
			}
		case q.Is(2, http.MethodPut):
			var in ClassInput
			if knowkit.Decode(w, r, &in) {
				c, err := repo.UpdateClass(ctx, a, id, in)
				knowkit.Reply(w, c, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteClass(ctx, a, id))
		default:
			return false
		}
	case "pools":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Pools(ctx, a.Org)
			knowkit.Reply(w, map[string]any{"pools": l}, err)
		case q.Is(1, http.MethodPost):
			var in PoolInput
			if knowkit.Decode(w, r, &in) {
				p, err := repo.CreatePool(ctx, a, in)
				knowkit.Reply(w, p, err)
			}
		case q.Is(2, http.MethodPut):
			var in PoolInput
			if knowkit.Decode(w, r, &in) {
				p, err := repo.UpdatePool(ctx, a, id, in)
				knowkit.Reply(w, p, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeletePool(ctx, a, id))
		default:
			return false
		}
	case "rounds":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Rounds(ctx, a.Org)
			knowkit.Reply(w, map[string]any{"rounds": l}, err)
		case q.Is(1, http.MethodPost):
			var in RoundInputRec
			if knowkit.Decode(w, r, &in) {
				x, err := repo.CreateRound(ctx, a, in)
				knowkit.Reply(w, x, err)
			}
		case q.Is(2, http.MethodPut):
			var in RoundInputRec
			if knowkit.Decode(w, r, &in) {
				x, err := repo.UpdateRound(ctx, a, id, in)
				knowkit.Reply(w, x, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteRound(ctx, a, id))
		default:
			return false
		}
	case "securities":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Securities(ctx, a.Org, qs.Get("holder"), qs.Get("kind"), qs.Get("as_of"))
			knowkit.Reply(w, map[string]any{"securities": l}, err)
		case q.Is(1, http.MethodPost):
			var in SecurityInput
			if knowkit.Decode(w, r, &in) {
				s, err := repo.Issue(ctx, a, in)
				knowkit.Reply(w, s, err)
			}
		case q.Is(2, http.MethodGet):
			s, err := repo.Security(ctx, a.Org, id)
			knowkit.Reply(w, s, err)
		case q.Is(2, http.MethodPut):
			var in SecurityInput
			if knowkit.Decode(w, r, &in) {
				s, err := repo.UpdateSecurity(ctx, a, id, in)
				knowkit.Reply(w, s, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteSecurity(ctx, a, id))
		case q.N() == 3 && q.Method == http.MethodPost && knowkit.OneOf(q.Seg[2], "exercise", "cancel", "transfer", "convert"):
			var in EventInput
			if knowkit.Decode(w, r, &in) {
				s, err := repo.Record(ctx, a, q.Seg[2], id, in)
				knowkit.Reply(w, s, err)
			}
		default:
			return false
		}
	case "events":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Events(ctx, a.Org)
			knowkit.Reply(w, map[string]any{"events": l}, err)
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.UndoEvent(ctx, a, id))
		default:
			return false
		}
	case "model":
		switch {
		case q.Is(2, http.MethodPost) && q.Seg[1] == "round":
			var in RoundModelInput
			if knowkit.Decode(w, r, &in) {
				m, err := repo.ModelRoundPreview(ctx, a.Org, in)
				knowkit.Reply(w, m, err)
			}
		case q.Is(2, http.MethodPost) && q.Seg[1] == "close-round":
			var in RoundModelInput
			if knowkit.Decode(w, r, &in) {
				x, err := repo.CloseRound(ctx, a, in)
				knowkit.Reply(w, x, err)
			}
		case q.Is(2, http.MethodPost) && q.Seg[1] == "exit":
			var in ExitInput
			if knowkit.Decode(w, r, &in) {
				o, err := repo.ExitModel(ctx, a.Org, in)
				knowkit.Reply(w, map[string]any{"outcomes": o}, err)
			}
		default:
			return false
		}
	case "scenarios":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Scenarios(ctx, a.Org, qs.Get("kind"))
			knowkit.Reply(w, map[string]any{"scenarios": l}, err)
		case q.Is(1, http.MethodPost):
			var in struct {
				Kind   string          `json:"kind"`
				Name   string          `json:"name"`
				Params json.RawMessage `json:"params"`
			}
			if knowkit.Decode(w, r, &in) {
				s, err := repo.SaveScenario(ctx, a, in.Kind, in.Name, in.Params)
				knowkit.Reply(w, s, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteScenario(ctx, a, id))
		default:
			return false
		}
	case "export":
		if !q.Is(2, http.MethodGet) {
			return false
		}
		b, name, err := repo.ExportCSV(ctx, a, id, qs.Get("as_of"))
		if err != nil {
			knowkit.Fail(w, err)
			return true
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	default:
		return false
	}
	return true
}
