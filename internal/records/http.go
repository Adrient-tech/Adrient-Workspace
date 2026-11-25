package records

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// NewHTTPHandler serves /api/v1/records inside the auth wall.
func NewHTTPHandler(repo *Repository, auth knowkit.Auth) *knowkit.Handler {
	return &knowkit.Handler{Mount: "/api/v1/records", Auth: auth, Route: func(q knowkit.Req) bool { return route(repo, q) }}
}

func route(repo *Repository, q knowkit.Req) bool {
	w, r, a, ctx := q.W, q.R, q.A, q.R.Context()
	id := q.ID()
	switch q.Seg[0] {
	case "templates":
		if q.Is(1, http.MethodGet) {
			knowkit.JSON(w, map[string]any{"templates": Templates, "types": AllTypes, "rollup_fns": RollupFns, "operators": FilterOps})
			return true
		}
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
			knowkit.JSON(w, map[string]any{"people": out})
			return true
		}
	case "bases":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.ListBases(ctx, a, r.URL.Query().Get("trashed") == "1")
			knowkit.Reply(w, map[string]any{"bases": l}, err)
		case q.Is(1, http.MethodPost):
			var in BaseInput
			if knowkit.Decode(w, r, &in) {
				b, err := repo.CreateBase(ctx, a, in)
				knowkit.Reply(w, b, err)
			}
		case q.Is(2, http.MethodGet):
			b, err := repo.GetBase(ctx, a, id)
			knowkit.Reply(w, b, err)
		case q.Is(2, http.MethodPut):
			var in BaseInput
			if knowkit.Decode(w, r, &in) {
				b, err := repo.UpdateBase(ctx, a, id, in)
				knowkit.Reply(w, b, err)
			}
		case q.Is(2, http.MethodDelete):
			if r.URL.Query().Get("permanent") == "1" {
				knowkit.OK(w, repo.DeleteBase(ctx, a, id))
			} else {
				knowkit.OK(w, repo.TrashBase(ctx, a, id, true))
			}
		case q.Sub("restore", http.MethodPost):
			knowkit.OK(w, repo.TrashBase(ctx, a, id, false))
		case q.Sub("members", http.MethodPut):
			var in struct {
				Members []Member `json:"members"`
			}
			if knowkit.Decode(w, r, &in) {
				knowkit.OK(w, repo.SetMembers(ctx, a, id, in.Members))
			}
		case q.Sub("tables", http.MethodPost):
			var in struct {
				Name string `json:"name"`
			}
			if knowkit.Decode(w, r, &in) {
				t, err := repo.CreateTable(ctx, a, id, in.Name)
				knowkit.Reply(w, t, err)
			}
		case q.Sub("backup", http.MethodGet):
			bk, err := repo.ExportBackup(ctx, a, id)
			if err != nil {
				knowkit.Fail(w, err)
				return true
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", `attachment; filename="records-backup.json"`)
			_ = json.NewEncoder(w).Encode(bk)
		case q.Sub("import-preview", http.MethodPost):
			var in struct {
				Data      string `json:"data"`
				HasHeader bool   `json:"has_header"`
			}
			if knowkit.Decode(w, r, &in) {
				if _, err := repo.getBase(ctx, a, id, RoleEdit); err != nil {
					knowkit.Fail(w, err)
					return true
				}
				p, err := PreviewCSV([]byte(in.Data), in.HasHeader)
				knowkit.Reply(w, p, err)
			}
		case q.Sub("import", http.MethodPost):
			var in ImportRequest
			if knowkit.Decode(w, r, &in) {
				res, err := repo.ImportCSV(ctx, a, id, in)
				knowkit.Reply(w, res, err)
			}
		default:
			if q.N() == 2 && q.Seg[1] == "restore-backup" && q.Method == http.MethodPost {
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 200<<20))
				if err != nil {
					knowkit.Fail(w, knowkit.Invalid("cannot read the upload"))
					return true
				}
				b, err := repo.RestoreBackup(ctx, a, body, r.URL.Query().Get("name"))
				knowkit.Reply(w, b, err)
				return true
			}
			return false
		}
		return true
	case "tables":
		switch {
		case q.Is(2, http.MethodPut):
			var in struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Position    *int   `json:"position"`
			}
			if knowkit.Decode(w, r, &in) {
				knowkit.OK(w, repo.UpdateTable(ctx, a, id, in.Name, in.Description, in.Position))
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteTable(ctx, a, id))
		case q.Sub("fields", http.MethodPost):
			var in FieldInput
			if knowkit.Decode(w, r, &in) {
				f, err := repo.CreateField(ctx, a, id, in)
				knowkit.Reply(w, f, err)
			}
		case q.Sub("views", http.MethodPost):
			var in View
			if knowkit.Decode(w, r, &in) {
				v, err := repo.SaveView(ctx, a, id, "", in)
				knowkit.Reply(w, v, err)
			}
		case q.N() == 4 && q.Seg[2] == "views" && q.Method == http.MethodPut:
			var in View
			if knowkit.Decode(w, r, &in) {
				v, err := repo.SaveView(ctx, a, id, q.Seg[3], in)
				knowkit.Reply(w, v, err)
			}
		case q.N() == 4 && q.Seg[2] == "views" && q.Method == http.MethodDelete:
			knowkit.OK(w, repo.DeleteView(ctx, a, id, q.Seg[3]))
		case q.Sub("query", http.MethodPost):
			var in Query
			if knowkit.Decode(w, r, &in) {
				res, err := repo.ListRecords(ctx, a, id, in)
				knowkit.Reply(w, res, err)
			}
		case q.Sub("records", http.MethodGet):
			lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			res, err := repo.ListRecords(ctx, a, id, Query{Limit: lim, Offset: off, Search: r.URL.Query().Get("q")})
			knowkit.Reply(w, res, err)
		case q.Sub("records", http.MethodPost):
			var in struct {
				Records []RecordInput `json:"records"`
			}
			if knowkit.Decode(w, r, &in) {
				rs, err := repo.CreateRecords(ctx, a, id, in.Records)
				knowkit.Reply(w, map[string]any{"records": rs}, err)
			}
		case q.Sub("records", http.MethodPatch):
			var in struct {
				Records []RecordInput `json:"records"`
			}
			if knowkit.Decode(w, r, &in) {
				rs, err := repo.UpdateRecords(ctx, a, id, in.Records)
				knowkit.Reply(w, map[string]any{"records": rs}, err)
			}
		case q.Sub("records", http.MethodDelete):
			var in struct {
				IDs []string `json:"ids"`
			}
			if knowkit.Decode(w, r, &in) {
				n, err := repo.DeleteRecords(ctx, a, id, in.IDs)
				knowkit.Reply(w, map[string]any{"deleted": n}, err)
			}
		case q.Sub("export", http.MethodPost):
			var in struct {
				Query   Query    `json:"query"`
				Fields  []string `json:"fields"`
				ToDrive bool     `json:"to_drive"`
			}
			if !knowkit.Decode(w, r, &in) {
				return true
			}
			data, name, err := repo.ExportCSV(ctx, a, id, in.Query, in.Fields)
			if err != nil {
				knowkit.Fail(w, err)
				return true
			}
			if in.ToDrive {
				if repo.Deps.SaveFile == nil {
					knowkit.Fail(w, knowkit.ErrUnavailable)
					return true
				}
				fid, err := repo.Deps.SaveFile(ctx, a.Org, a.User, name+".csv", "text/csv", data)
				knowkit.Reply(w, map[string]string{"file_id": fid, "name": name + ".csv"}, err)
				return true
			}
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(name)+`.csv"`)
			_, _ = w.Write(data)
		default:
			return false
		}
		return true
	case "fields":
		switch {
		case q.Is(2, http.MethodPut):
			var in FieldInput
			if knowkit.Decode(w, r, &in) {
				f, cleared, err := repo.UpdateField(ctx, a, id, in)
				knowkit.Reply(w, map[string]any{"field": f, "cleared": cleared}, err)
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteField(ctx, a, id))
		default:
			return false
		}
		return true
	case "records":
		switch {
		case q.Is(2, http.MethodGet):
			rc, tid, err := repo.GetRecord(ctx, a, id)
			knowkit.Reply(w, map[string]any{"record": rc, "table_id": tid}, err)
		case q.Sub("comments", http.MethodGet):
			l, err := repo.ListComments(ctx, a, id)
			knowkit.Reply(w, map[string]any{"comments": l}, err)
		case q.Sub("comments", http.MethodPost):
			var in struct {
				Body string `json:"body"`
			}
			if knowkit.Decode(w, r, &in) {
				c, err := repo.AddComment(ctx, a, id, in.Body)
				knowkit.Reply(w, c, err)
			}
		case q.Sub("history", http.MethodGet):
			l, err := repo.History(ctx, a, id)
			knowkit.Reply(w, map[string]any{"history": l}, err)
		default:
			return false
		}
		return true
	}
	return false
}

func safeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if c == '"' || c == '\\' || c < 32 || c == '/' {
			c = '_'
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return "export"
	}
	return string(out)
}
