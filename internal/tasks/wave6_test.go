package tasks

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWave6TagsRepeatAndICS(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	l, err := repo.CreateList(ctx, orgID, userID, ListFields{Name: "Home"})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	task, err := repo.CreateTask(ctx, orgID, l.ID, userID, TaskFields{Title: "Pay rent", DueAt: &due})
	if err != nil {
		t.Fatal(err)
	}
	min := 30
	e := Extras{Tags: []string{"#Home", "bills", "home"}, Recurrence: "every:2:weekly:1,3", RemindMinutes: &min, BoardStatus: "todo"}
	if _, err := repo.SaveExtras(ctx, orgID, userID, task.ID, e); err != nil {
		t.Fatalf("SaveExtras: %v", err)
	}
	ex, err := repo.ListExtras(ctx, orgID, l.ID)
	if err != nil || len(ex[task.ID].Tags) != 2 || ex[task.ID].Recurrence != "every:2:weekly:1,3" {
		t.Fatalf("extras %+v err=%v", ex[task.ID], err)
	}
	if _, err := repo.SaveExtras(ctx, orgID, userID, task.ID, Extras{Tags: []string{"a b"}}); err == nil {
		t.Fatal("bad tag accepted")
	}
	tags, err := repo.Tags(ctx, orgID, userID)
	if err != nil || len(tags) != 2 {
		t.Fatalf("tags %+v err=%v", tags, err)
	}
	feed, err := repo.OpenDated(ctx, orgID, userID)
	if err != nil || len(feed) != 1 {
		t.Fatalf("feed %+v err=%v", feed, err)
	}
	ics := BuildICS(feed, "x.test")
	for _, w := range []string{"SUMMARY:Pay rent", "RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE", "TRIGGER:-PT30M"} {
		if !strings.Contains(ics, w) {
			t.Errorf("missing %s", w)
		}
	}
	// Completing spawns the next occurrence in the future, tags and rule kept.
	done, err := repo.ToggleTask(ctx, orgID, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := repo.SpawnNext(ctx, done)
	if err != nil || next == nil || next.DueAt == nil || next.DueAt.Before(time.Now()) {
		t.Fatalf("spawn %+v err=%v", next, err)
	}
	nx, _ := repo.ListExtras(ctx, orgID, l.ID)
	if len(nx[next.ID].Tags) != 2 || nx[next.ID].Recurrence == "" {
		t.Fatalf("next extras %+v", nx[next.ID])
	}
}

func TestWave6PublicLinkAndFeed(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	l, _ := repo.CreateList(ctx, orgID, userID, ListFields{Name: "Shared <b>list</b>"})
	due := time.Now().Add(24 * time.Hour)
	if _, err := repo.CreateTask(ctx, orgID, l.ID, userID, TaskFields{Title: "Buy <script>x</script>", DueAt: &due}); err != nil {
		t.Fatal(err)
	}
	if tok, err := repo.PublicLink(ctx, orgID, userID, l.ID); err != nil || tok != "" {
		t.Fatalf("expected no link, got %q %v", tok, err)
	}
	tok, err := repo.CreatePublicLink(ctx, orgID, userID, l.ID, false)
	if err != nil || len(tok) < 32 {
		t.Fatalf("create %q %v", tok, err)
	}
	if again, _ := repo.CreatePublicLink(ctx, orgID, userID, l.ID, false); again != tok {
		t.Fatal("create is not idempotent")
	}
	pl, err := repo.PublicByToken(ctx, tok)
	if err != nil || len(pl.Tasks) != 1 {
		t.Fatalf("public %+v %v", pl, err)
	}
	// HTML page escapes user text.
	rec := httptest.NewRecorder()
	NewPublicHandler(repo).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/public/tasks/list/"+tok, nil))
	body := rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "<script>x") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("page %d %s", rec.Code, body)
	}
	// Non-owner cannot manage; revoked token 404s.
	if _, err := repo.CreatePublicLink(ctx, orgID, "00000000-0000-0000-0000-000000000001", l.ID, false); err == nil {
		t.Fatal("non-owner created a link")
	}
	if err := repo.RevokePublicLink(ctx, orgID, userID, l.ID); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	NewPublicHandler(repo).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/public/tasks/list/"+tok, nil))
	if rec.Code != 404 {
		t.Fatalf("revoked link status %d", rec.Code)
	}
	// Feed token round trip and rotation.
	ft, err := repo.FeedToken(ctx, orgID, userID, false)
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	NewPublicHandler(repo).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/public/tasks/feed/"+ft+".ics", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "SUMMARY:Buy <script>x<\\/script>") && !strings.Contains(rec.Body.String(), "BEGIN:VEVENT") {
		t.Fatalf("feed %d %s", rec.Code, rec.Body.String())
	}
	nt, _ := repo.FeedToken(ctx, orgID, userID, true)
	rec = httptest.NewRecorder()
	NewPublicHandler(repo).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/public/tasks/feed/"+ft+".ics", nil))
	if nt == ft || rec.Code != 404 {
		t.Fatalf("old token still works: %d", rec.Code)
	}
}
