package projects

import (
	"context"
	"testing"
)

func TestWipOver(t *testing.T) {
	if WipOver(3, 4) != 0 || WipOver(4, 4) != 0 || WipOver(6, 4) != 2 || WipOver(9, 0) != 0 {
		t.Fatal("WipOver")
	}
	if !ValidStatus("in_progress") || ValidStatus("nope") {
		t.Fatal("ValidStatus")
	}
}

func TestArchiveAndWip(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	team, err := repo.CreateTeam(ctx, orgID, "Eng", "ENG", "", "")
	if err != nil {
		t.Fatal(err)
	}
	open, _ := repo.CreateIssue(ctx, orgID, team.ID, userID, IssueFields{Title: "open", Status: "todo"})
	done, _ := repo.CreateIssue(ctx, orgID, team.ID, userID, IssueFields{Title: "done", Status: "done"})

	// Open work cannot be archived; done work can, and comes back.
	n, err := repo.SetArchived(ctx, orgID, []string{open.ID, done.ID}, true)
	if err != nil || n != 1 {
		t.Fatalf("archive n=%d err=%v", n, err)
	}
	ex, _ := repo.ListExtras(ctx, orgID)
	if !ex[done.ID].Archived || ex[open.ID].Archived {
		t.Fatalf("flags %+v", ex)
	}
	// Saving plan details must not unarchive.
	if err := repo.SaveIssueExtras(ctx, orgID, done.ID, IssueExtras{DueDate: "2026-12-01"}); err != nil {
		t.Fatal(err)
	}
	ex, _ = repo.ListExtras(ctx, orgID)
	if !ex[done.ID].Archived {
		t.Fatal("plan save cleared archived")
	}
	if n, _ := repo.SetArchived(ctx, orgID, []string{done.ID}, false); n != 1 {
		t.Fatalf("restore n=%d", n)
	}
	if _, err := repo.SetArchived(ctx, orgID, []string{"not-a-uuid"}, true); err == nil {
		t.Fatal("bad id accepted")
	}

	// WIP limits: set, replace, clear; unknown team rejected.
	if err := repo.SetWip(ctx, orgID, team.ID, "in_progress", 3); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetWip(ctx, orgID, team.ID, "in_progress", 5); err != nil {
		t.Fatal(err)
	}
	l, _ := repo.ListWip(ctx, orgID, team.ID)
	if len(l) != 1 || l[0].Max != 5 {
		t.Fatalf("limits %+v", l)
	}
	if err := repo.SetWip(ctx, orgID, team.ID, "bogus", 2); err == nil {
		t.Fatal("bad status accepted")
	}
	if err := repo.SetWip(ctx, orgID, "00000000-0000-0000-0000-000000000009", "todo", 2); err == nil {
		t.Fatal("unknown team accepted")
	}
	_ = repo.SetWip(ctx, orgID, team.ID, "in_progress", 0)
	if l, _ = repo.ListWip(ctx, orgID, team.ID); len(l) != 0 {
		t.Fatalf("not cleared %+v", l)
	}
}
