package wiki

import (
	"io"
	"net/http"
	"strconv"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// NewHTTPHandler serves /api/v1/wiki inside the auth wall.
func NewHTTPHandler(repo *Repository, auth knowkit.Auth) *knowkit.Handler {
	return &knowkit.Handler{Mount: "/api/v1/wiki", Auth: auth, Route: func(q knowkit.Req) bool { return route(repo, q) }}
}

func route(repo *Repository, q knowkit.Req) bool {
	w, r, a, ctx := q.W, q.R, q.A, q.R.Context()
	id := q.ID()
	switch q.Seg[0] {
	case "templates":
		if q.Is(1, http.MethodGet) {
			knowkit.JSON(w, map[string]any{"templates": TemplateNames})
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
			knowkit.JSON(w, map[string]any{"people": out, "me": a.User})
			return true
		}
	case "spaces":
		switch {
		case q.Is(1, http.MethodGet):
			l, err := repo.ListSpaces(ctx, a)
			knowkit.Reply(w, map[string]any{"spaces": l}, err)
		case q.Is(1, http.MethodPost):
			var in SpaceInput
			if knowkit.Decode(w, r, &in) {
				s, err := repo.CreateSpace(ctx, a, in)
				knowkit.Reply(w, s, err)
			}
		case q.Is(2, http.MethodPut):
			var in SpaceInput
			if knowkit.Decode(w, r, &in) {
				knowkit.OK(w, repo.UpdateSpace(ctx, a, id, in))
			}
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteSpace(ctx, a, id))
		case q.Sub("members", http.MethodGet):
			l, err := repo.SpaceMembers(ctx, a, id)
			knowkit.Reply(w, map[string]any{"members": l}, err)
		case q.Sub("members", http.MethodPut):
			var in struct {
				Members []Member `json:"members"`
			}
			if knowkit.Decode(w, r, &in) {
				knowkit.OK(w, repo.SetMembers(ctx, a, id, in.Members))
			}
		case q.Sub("pages", http.MethodGet):
			l, err := repo.Tree(ctx, a, id, r.URL.Query().Get("trashed") == "1")
			knowkit.Reply(w, map[string]any{"pages": l}, err)
		default:
			return false
		}
		return true
	case "pages":
		switch {
		case q.Is(1, http.MethodPost):
			var in PageInput
			if knowkit.Decode(w, r, &in) {
				p, err := repo.CreatePage(ctx, a, in)
				knowkit.Reply(w, p, err)
			}
		case q.Is(2, http.MethodGet):
			p, err := repo.GetPage(ctx, a, id)
			knowkit.Reply(w, p, err)
		case q.Is(2, http.MethodPut):
			var in PageUpdate
			if knowkit.Decode(w, r, &in) {
				p, err := repo.UpdatePage(ctx, a, id, in)
				knowkit.Reply(w, p, err)
			}
		case q.Is(2, http.MethodDelete):
			if r.URL.Query().Get("permanent") == "1" {
				knowkit.OK(w, repo.DeletePage(ctx, a, id))
			} else {
				knowkit.OK(w, repo.TrashPage(ctx, a, id, true))
			}
		case q.Sub("restore", http.MethodPost):
			knowkit.OK(w, repo.TrashPage(ctx, a, id, false))
		case q.Sub("move", http.MethodPost):
			var in struct {
				ParentID string `json:"parent_id"`
				SpaceID  string `json:"space_id"`
				Index    int    `json:"index"`
			}
			if knowkit.Decode(w, r, &in) {
				knowkit.OK(w, repo.MovePage(ctx, a, id, in.ParentID, in.SpaceID, in.Index))
			}
		case q.Sub("duplicate", http.MethodPost):
			p, err := repo.DuplicatePage(ctx, a, id)
			knowkit.Reply(w, p, err)
		case q.Sub("favorite", http.MethodPut), q.Sub("favorite", http.MethodDelete):
			knowkit.OK(w, repo.SetFavorite(ctx, a, id, q.Method == http.MethodPut))
		case q.Sub("backlinks", http.MethodGet):
			l, err := repo.Backlinks(ctx, a, id)
			knowkit.Reply(w, map[string]any{"pages": l}, err)
		case q.Sub("versions", http.MethodGet):
			l, err := repo.Versions(ctx, a, id)
			knowkit.Reply(w, map[string]any{"versions": l}, err)
		case q.N() == 4 && q.Seg[2] == "versions" && q.Method == http.MethodGet:
			raw, err := repo.VersionContent(ctx, a, id, q.Seg[3])
			knowkit.RawJSON(w, raw, err)
		case q.N() == 5 && q.Seg[2] == "versions" && q.Seg[4] == "restore" && q.Method == http.MethodPost:
			p, err := repo.RestoreVersion(ctx, a, id, q.Seg[3])
			knowkit.Reply(w, p, err)
		case q.Sub("comments", http.MethodGet):
			l, err := repo.Comments(ctx, a, id)
			knowkit.Reply(w, map[string]any{"comments": l}, err)
		case q.Sub("comments", http.MethodPost):
			var in struct {
				Body string `json:"body"`
			}
			if knowkit.Decode(w, r, &in) {
				c, err := repo.AddComment(ctx, a, id, in.Body)
				knowkit.Reply(w, c, err)
			}
		case q.Sub("export", http.MethodGet):
			p, err := repo.GetPage(ctx, a, id)
			if err != nil {
				knowkit.Fail(w, err)
				return true
			}
			if r.URL.Query().Get("format") == "html" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
				_, _ = w.Write([]byte(DocToHTML(p.Title, p.Content)))
				return true
			}
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="page.md"`)
			_, _ = w.Write([]byte(DocToMarkdown(p.Title, p.Content)))
		default:
			return false
		}
		return true
	case "comments":
		switch {
		case q.Sub("resolve", http.MethodPost):
			knowkit.OK(w, repo.ResolveComment(ctx, a, id, true))
		case q.Sub("reopen", http.MethodPost):
			knowkit.OK(w, repo.ResolveComment(ctx, a, id, false))
		case q.Is(2, http.MethodDelete):
			knowkit.OK(w, repo.DeleteComment(ctx, a, id))
		default:
			return false
		}
		return true
	case "favorites":
		if q.Is(1, http.MethodGet) {
			l, err := repo.Favorites(ctx, a)
			knowkit.Reply(w, map[string]any{"pages": l}, err)
			return true
		}
	case "recent":
		if q.Is(1, http.MethodGet) {
			l, err := repo.Recent(ctx, a)
			knowkit.Reply(w, map[string]any{"pages": l}, err)
			return true
		}
	case "search":
		if q.Is(1, http.MethodGet) {
			l, err := repo.Search(ctx, a, r.URL.Query().Get("q"))
			knowkit.Reply(w, map[string]any{"pages": l}, err)
			return true
		}
	case "import":
		if q.Is(1, http.MethodPost) {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 3<<20))
			if err != nil {
				knowkit.Fail(w, knowkit.Invalid("cannot read the upload"))
				return true
			}
			title := r.URL.Query().Get("title")
			t, content := MarkdownToDoc(string(body))
			if title == "" {
				title = t
			}
			if title == "" {
				title = "Imported page"
			}
			p, err := repo.CreatePage(ctx, a, PageInput{SpaceID: r.URL.Query().Get("space_id"), ParentID: r.URL.Query().Get("parent_id"), Title: title})
			if err != nil {
				knowkit.Fail(w, err)
				return true
			}
			p, err = repo.UpdatePage(ctx, a, p.ID, PageUpdate{Content: []byte(content), Version: p.Version})
			knowkit.Reply(w, p, err)
			return true
		}
	}
	_ = strconv.Itoa
	return false
}
