package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit/kittest"
	"github.com/Adrient-tech/Adrient-Workspace/internal/records"
)

func TestMarkdownRoundTrip(t *testing.T) {
	md := "# Title\n\nHello **bold** and *it* with `code` and [link](https://x.example).\n\n## Sub\n\n- one\n- two\n\n1. a\n2. b\n\n- [x] done\n- [ ] todo\n\n> quote\n\n```\ncode here\n```\n\n---\n"
	title, content := MarkdownToDoc(md)
	if title != "Title" {
		t.Fatalf("title = %q", title)
	}
	out := DocToMarkdown(title, []byte(content))
	for _, want := range []string{"# Title", "**bold**", "*it*", "`code`", "[link](https://x.example)", "## Sub", "- one", "1. a", "- [x] done", "- [ ] todo", "> quote", "```\ncode here\n```", "---"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Unsafe links become plain text; raw HTML stays text.
	_, c := MarkdownToDoc("[x](javascript:alert(1)) <script>alert(1)</script>")
	if strings.Contains(c, "javascript:") && strings.Contains(c, `"link"`) {
		t.Error("javascript: link must not become a link mark")
	}
	if got := extractText([]byte(c)); !strings.Contains(got, "<script>") {
		t.Error("html should be kept as text")
	}
	h := DocToHTML("a<b", []byte(c))
	if strings.Contains(h, "<script>") || !strings.Contains(h, "a&lt;b") {
		t.Errorf("html export not escaped: %s", h)
	}
}

func TestExtractLinks(t *testing.T) {
	id := "11111111-2222-3333-4444-555555555555"
	raw := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"/wiki?page=` + id + `"}}]},{"type":"pageLink","attrs":{"id":"` + id + `"}}]}]}`
	if l := extractPageLinks([]byte(raw)); len(l) != 1 || l[0] != id {
		t.Errorf("links = %v", l)
	}
}

func TestWikiEndToEnd(t *testing.T) {
	fx := kittest.Setup(t)
	ctx := context.Background()
	rec := records.NewRepository(fx.Pool)
	repo := NewRepository(fx.Pool)
	repo.Deps.Records = rec
	rec.Deps.ExternalRole = func(ctx context.Context, org, user, source, ref string) int {
		return repo.SpaceRole(ctx, org, user, false, ref)
	}
	a, b := fx.Admin, fx.User

	// Personal space is private, even from org admins.
	sp := func(x knowkit.Actor) []Space { l, err := repo.ListSpaces(ctx, x); noerr(t, err); return l }
	var mine Space
	for _, s := range sp(b) {
		if s.Kind == "personal" {
			mine = s
		}
	}
	if mine.ID == "" || mine.Role != RoleAdmin {
		t.Fatalf("personal space missing: %+v", sp(b))
	}
	for _, s := range sp(a) {
		if s.ID == mine.ID {
			t.Error("org admin must not see another user's personal space")
		}
	}
	if _, err := repo.Tree(ctx, fx.Other, mine.ID, false); err == nil {
		t.Error("other org must not read")
	}

	team, err := repo.CreateSpace(ctx, a, SpaceInput{Name: "Eng"})
	noerr(t, err)
	if _, err := repo.Tree(ctx, b, team.ID, false); err == nil {
		t.Error("unshared team space must be hidden")
	}
	noerr(t, repo.SetMembers(ctx, a, team.ID, []Member{{UserID: b.User, Role: "viewer"}}))
	tree, err := repo.Tree(ctx, b, team.ID, false)
	noerr(t, err)
	if len(tree) != 1 || tree[0].Title != "Welcome" {
		t.Fatalf("tree = %+v", tree)
	}
	if _, err := repo.CreatePage(ctx, b, PageInput{SpaceID: team.ID, Title: "x"}); err == nil {
		t.Error("viewer must not create pages")
	}
	if _, err := repo.AddComment(ctx, b, tree[0].ID, "nice"); err != nil {
		t.Errorf("viewer may comment: %v", err)
	}
	noerr(t, repo.SetMembers(ctx, a, team.ID, []Member{{UserID: b.User, Role: "editor"}}))

	// Pages, nesting, optimistic locking, history, backlinks.
	root, err := repo.CreatePage(ctx, b, PageInput{SpaceID: team.ID, Title: "Runbook", Template: "meeting_notes"})
	noerr(t, err)
	child, err := repo.CreatePage(ctx, b, PageInput{SpaceID: team.ID, ParentID: root.ID, Title: "Deploy"})
	noerr(t, err)
	body := func(text string, link string) json.RawMessage {
		c := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}`
		if link != "" {
			c += `,{"type":"pageLink","attrs":{"id":"` + link + `"}}`
		}
		return json.RawMessage(c + `]}]}`)
	}
	p1, err := repo.UpdatePage(ctx, b, child.ID, PageUpdate{Content: body("first draft needle", root.ID), Version: child.Version})
	noerr(t, err)
	if _, err := repo.UpdatePage(ctx, a, child.ID, PageUpdate{Content: body("stale", ""), Version: child.Version}); !errors.Is(err, ErrConflict) {
		t.Errorf("stale save should conflict, got %v", err)
	}
	p2, err := repo.UpdatePage(ctx, a, child.ID, PageUpdate{Content: body("second", root.ID), Version: p1.Version})
	noerr(t, err)
	if p2.Version != p1.Version+1 {
		t.Errorf("version = %d", p2.Version)
	}
	vs, _ := repo.Versions(ctx, b, child.ID)
	if len(vs) != 1 {
		t.Fatalf("versions = %d", len(vs))
	}
	res, _ := repo.Search(ctx, b, "second")
	if len(res) != 1 || res[0].ID != child.ID {
		t.Errorf("search = %+v", res)
	}
	if res, _ := repo.Search(ctx, fx.Other, "second"); len(res) != 0 {
		t.Error("search leaks across orgs")
	}
	bl, _ := repo.Backlinks(ctx, b, root.ID)
	if len(bl) != 1 || bl[0].ID != child.ID {
		t.Errorf("backlinks = %+v", bl)
	}
	// Snapshots hold the content before a save; restoring the first one returns the empty page.
	restored, err := repo.RestoreVersion(ctx, b, child.ID, vs[0].ID)
	noerr(t, err)
	if !strings.Contains(string(restored.Content), "\"paragraph\"") {
		t.Errorf("restore content = %s", restored.Content)
	}
	if _, err := repo.UpdatePage(ctx, b, child.ID, PageUpdate{Content: json.RawMessage(`{"type":"nope"}`), Version: restored.Version}); err == nil {
		t.Error("non-document content must be refused")
	}

	// Move: no cycles; reorder.
	if err := repo.MovePage(ctx, b, root.ID, child.ID, "", 0); err == nil {
		t.Error("moving a page into its own child must fail")
	}
	noerr(t, repo.MovePage(ctx, b, child.ID, "", "", 0))
	// Trash cascades and restore.
	noerr(t, repo.TrashPage(ctx, b, root.ID, true))
	tr, _ := repo.Tree(ctx, b, team.ID, true)
	if len(tr) != 1 {
		t.Errorf("trashed = %d", len(tr))
	}
	if err := repo.DeletePage(ctx, b, child.ID); err == nil {
		t.Error("delete needs trash first")
	}
	noerr(t, repo.TrashPage(ctx, b, root.ID, false))
	noerr(t, repo.SetFavorite(ctx, b, root.ID, true))
	if f, _ := repo.Favorites(ctx, b); len(f) != 1 {
		t.Errorf("favorites = %d", len(f))
	}

	// Database page: Records base owned by the wiki, access follows the space.
	db, err := repo.CreatePage(ctx, b, PageInput{SpaceID: team.ID, Title: "Tasks", Template: "task_board"})
	noerr(t, err)
	if db.Kind != "database" || db.RecordsTableID == "" {
		t.Fatalf("db page = %+v", db)
	}
	bu := knowkit.Actor{Org: fx.Org, User: b.User}
	if _, err := rec.CreateRecords(ctx, bu, db.RecordsTableID, []records.RecordInput{{Fields: map[string]any{"Name": "Ship it", "Status": "To do"}}}); err != nil {
		t.Fatalf("editor write to wiki db: %v", err)
	}
	third := knowkit.Actor{Org: fx.Org, User: fx.Admin.User}
	_ = third
	noerr(t, repo.SetMembers(ctx, a, team.ID, nil))
	if _, err := rec.ListRecords(ctx, bu, db.RecordsTableID, records.Query{}); err == nil {
		t.Error("after unsharing, the wiki database must be hidden")
	}
	if l, _ := rec.ListBases(ctx, a, false); len(l) != 0 {
		t.Error("wiki databases must not show in the Records base list")
	}
	// Deleting the page removes the database.
	noerr(t, repo.SetMembers(ctx, a, team.ID, []Member{{UserID: b.User, Role: "editor"}}))
	noerr(t, repo.TrashPage(ctx, b, db.ID, true))
	noerr(t, repo.DeletePage(ctx, b, db.ID))
	var n int
	_ = fx.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.records_bases WHERE org_id=$1 AND source='wiki'`, fx.Org).Scan(&n)
	if n != 0 {
		t.Errorf("embedded base left behind: %d", n)
	}
}

func noerr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
