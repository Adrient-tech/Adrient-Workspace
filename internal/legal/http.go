package legal

import (
	"net/http"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// NewHTTPHandler serves /api/v1/legal inside the auth wall.
func NewHTTPHandler(repo *Repository, auth knowkit.Auth) *knowkit.Handler {
	return &knowkit.Handler{Mount: "/api/v1/legal", Auth: auth, Route: func(q knowkit.Req) bool { return route(repo, q) }}
}

func route(repo *Repository, q knowkit.Req) bool {
	w, r, a, ctx := q.W, q.R, q.A, q.R.Context()
	id := q.ID()
	qs := r.URL.Query()
	switch q.Seg[0] {
	case "meta":
		s, err := repo.GetSettings(ctx, a.Org)
		knowkit.Reply(w, map[string]any{"settings": s, "statuses": Statuses, "today": repo.today(), "admin": a.Admin,
			"invoicing": repo.Deps.Invoices != nil, "calendar": repo.Deps.Calendar != nil, "tasks": repo.Deps.Task != nil, "drive": repo.Deps.FileMeta != nil,
			"time_tracking": repo.Deps.TimeEntries != nil, "contacts": repo.Deps.Contacts != nil}, err)
	case "settings":
		switch {
		case q.Is(1, http.MethodGet):
			s, err := repo.GetSettings(ctx, a.Org)
			knowkit.Reply(w, s, err)
		case q.Is(1, http.MethodPut):
			var in Settings
			if knowkit.Decode(w, r, &in) {
				s, err := repo.SaveSettings(ctx, a, in)
				knowkit.Reply(w, s, err)
			}
		default:
			return false
		}
	case "overview":
		o, err := repo.Overview(ctx, a)
		knowkit.Reply(w, o, err)
	case "people":
		rows, err := repo.pool.Query(ctx, `SELECT id::text, COALESCE(NULLIF(display_name,''),email) FROM adrient.users WHERE org_id=$1 AND COALESCE(status,'active')='active' ORDER BY 2 LIMIT 100`, a.Org)
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
	case "contacts":
		if repo.Deps.Contacts == nil {
			knowkit.Fail(w, knowkit.ErrUnavailable)
			return true
		}
		l, err := repo.Deps.Contacts.Search(ctx, a.Org, qs.Get("q"), 20)
		knowkit.Reply(w, map[string]any{"contacts": l}, err)
	case "clients":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Clients(ctx, a, qs.Get("q"), qs.Get("archived") == "1")
			knowkit.Reply(w, map[string]any{"clients": l}, err)
		case q.Is(1, http.MethodPost):
			var in ClientInput
			if knowkit.Decode(w, r, &in) {
				c, err := repo.CreateClient(ctx, a, in)
				knowkit.Reply(w, c, err)
			}
		case q.Sub("from-contact", http.MethodPost):
			c, err := repo.ClientFromContact(ctx, a, id)
			knowkit.Reply(w, c, err)
		case q.Is(2, http.MethodGet):
			c, err := repo.GetClient(ctx, a, id)
			knowkit.Reply(w, c, err)
		case q.Is(2, http.MethodPut):
			var in ClientInput
			if knowkit.Decode(w, r, &in) {
				c, err := repo.UpdateClient(ctx, a, id, in)
				knowkit.Reply(w, c, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteClient(ctx, a, id))
		default:
			return false
		}
	case "matters":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Matters(ctx, a, MatterFilter{Q: qs.Get("q"), Status: qs.Get("status"), Client: qs.Get("client"), Area: qs.Get("area"), Mine: qs.Get("mine")})
			knowkit.Reply(w, map[string]any{"matters": l}, err)
		case q.Is(1, http.MethodPost):
			var in MatterInput
			if knowkit.Decode(w, r, &in) {
				m, err := repo.CreateMatter(ctx, a, in)
				knowkit.Reply(w, m, err)
			}
		case q.Is(2, http.MethodGet):
			m, err := repo.Matter(ctx, a, id)
			knowkit.Reply(w, m, err)
		case q.Is(2, http.MethodPut):
			var in MatterInput
			if knowkit.Decode(w, r, &in) {
				m, err := repo.UpdateMatter(ctx, a, id, in)
				knowkit.Reply(w, m, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteMatter(ctx, a, id))
		case q.Sub("status", http.MethodPost):
			var in MoveInput
			if knowkit.Decode(w, r, &in) {
				m, err := repo.Move(ctx, a, id, in)
				knowkit.Reply(w, m, err)
			}
		case q.Sub("deadlines", http.MethodPost):
			var in DeadlineInput
			if knowkit.Decode(w, r, &in) {
				d, err := repo.AddDeadline(ctx, a, id, in)
				knowkit.Reply(w, d, err)
			}
		case q.Sub("entries", http.MethodGet):
			l, err := repo.Entries(ctx, a, id, qs.Get("billed"), qs.Get("from"), qs.Get("to"), qs.Get("kind"))
			knowkit.Reply(w, map[string]any{"entries": l}, err)
		case q.Sub("entries", http.MethodPost):
			var in EntryInput
			if knowkit.Decode(w, r, &in) {
				e, err := repo.AddEntry(ctx, a, id, in)
				knowkit.Reply(w, e, err)
			}
		case q.Sub("import-time", http.MethodPost):
			var in struct {
				IDs []string `json:"ids"`
			}
			if knowkit.Decode(w, r, &in) {
				l, err := repo.ImportTime(ctx, a, id, in.IDs)
				knowkit.Reply(w, map[string]any{"entries": l}, err)
			}
		case q.Sub("bills", http.MethodGet):
			l, err := repo.Bills(ctx, a, id)
			knowkit.Reply(w, map[string]any{"bills": l}, err)
		case q.Sub("bills", http.MethodPost):
			var in BillInput
			if knowkit.Decode(w, r, &in) {
				b, err := repo.DraftBill(ctx, a, id, in)
				knowkit.Reply(w, b, err)
			}
		case q.Sub("documents", http.MethodPost):
			var in DocInput
			if knowkit.Decode(w, r, &in) {
				d, err := repo.AddDocument(ctx, a, id, in)
				knowkit.Reply(w, d, err)
			}
		case q.N() == 4 && q.Seg[2] == "documents" && q.Method == http.MethodDelete:
			knowkit.OK(w, repo.RemoveDocument(ctx, a, id, q.Seg[3]))
		case q.Sub("notes", http.MethodGet):
			l, err := repo.Notes(ctx, a, id)
			knowkit.Reply(w, map[string]any{"notes": l}, err)
		case q.Sub("notes", http.MethodPost):
			var in struct {
				Kind string `json:"kind"`
				Body string `json:"body"`
			}
			if knowkit.Decode(w, r, &in) {
				n, err := repo.AddNote(ctx, a, id, in.Kind, in.Body)
				knowkit.Reply(w, n, err)
			}
		case q.Sub("conflict", http.MethodPost):
			var in struct {
				Query string `json:"query"`
			}
			if knowkit.Decode(w, r, &in) {
				res, err := repo.RecordConflict(ctx, a, id, in.Query)
				knowkit.Reply(w, res, err)
			}
		default:
			return false
		}
	case "conflicts":
		if q.Is(1, http.MethodGet) {
			res, err := repo.ConflictCheck(ctx, a, qs.Get("q"))
			knowkit.Reply(w, res, err)
		} else {
			return false
		}
	case "deadlines":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Deadlines(ctx, a, qs.Get("done") == "1")
			knowkit.Reply(w, map[string]any{"deadlines": l}, err)
		case q.Is(2, http.MethodPut):
			var in DeadlineInput
			if knowkit.Decode(w, r, &in) {
				d, err := repo.UpdateDeadline(ctx, a, id, in)
				knowkit.Reply(w, d, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteDeadline(ctx, a, id))
		case q.Sub("done", http.MethodPost):
			d, err := repo.SetDeadlineDone(ctx, a, id, true)
			knowkit.Reply(w, d, err)
		case q.Sub("reopen", http.MethodPost):
			d, err := repo.SetDeadlineDone(ctx, a, id, false)
			knowkit.Reply(w, d, err)
		default:
			return false
		}
	case "entries":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Entries(ctx, a, qs.Get("matter"), qs.Get("billed"), qs.Get("from"), qs.Get("to"), qs.Get("kind"))
			knowkit.Reply(w, map[string]any{"entries": l}, err)
		case q.Is(2, http.MethodPut):
			var in EntryInput
			if knowkit.Decode(w, r, &in) {
				e, err := repo.UpdateEntry(ctx, a, id, in)
				knowkit.Reply(w, e, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteEntry(ctx, a, id))
		default:
			return false
		}
	case "time-candidates":
		l, err := repo.TimeCandidates(ctx, a, qs.Get("from"), qs.Get("to"))
		knowkit.Reply(w, map[string]any{"entries": l}, err)
	case "bills":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.Bills(ctx, a, "")
			knowkit.Reply(w, map[string]any{"bills": l}, err)
		case q.Is(2, http.MethodGet):
			b, err := repo.Bill(ctx, a, id)
			knowkit.Reply(w, b, err)
		case q.Sub("issue", http.MethodPost):
			b, err := repo.IssueBill(ctx, a, id)
			knowkit.Reply(w, b, err)
		case q.Sub("void", http.MethodPost):
			b, err := repo.VoidBill(ctx, a, id)
			knowkit.Reply(w, b, err)
		default:
			return false
		}
	case "linkable":
		l, err := repo.Linkable(ctx, a, qs.Get("kind"), qs.Get("q"))
		knowkit.Reply(w, map[string]any{"items": l}, err)
	case "reports":
		if q.Is(2, http.MethodGet) && id == "hours" {
			l, err := repo.HoursByUser(ctx, a, qs.Get("from"), qs.Get("to"))
			knowkit.Reply(w, map[string]any{"rows": l}, err)
		} else {
			return false
		}
	default:
		return false
	}
	return true
}
