package chat

import (
	"context"
	"errors"
	"testing"
)

func TestChannelTabs(t *testing.T) {
	pool, orgID, a := setupDB(t)
	ctx := context.Background()
	st := NewStore(NewRepository(pool))
	ch, err := st.CreateTChannel(ctx, orgID, a, "public", "tabs", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddTab(ctx, a, ch.ID, "sheet", "Budget", "/sheets/abc"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := st.AddTab(ctx, a, ch.ID, "link", "x", "javascript:alert(1)"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid url, got %v", err)
	}
	if _, err := st.AddTab(ctx, a, ch.ID, "bogus", "x", "/docs/1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid kind, got %v", err)
	}
	if _, err := st.AddTab(ctx, a, ch.ID, "link", "x", "//evil.com"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid // url, got %v", err)
	}
	tabs, err := st.ListTabs(ctx, ch.ID)
	if err != nil || len(tabs) != 1 || tabs[0].Title != "Budget" {
		t.Fatalf("list: %+v %v", tabs, err)
	}
	if err := st.RenameTab(ctx, ch.ID, tabs[0].ID, "Budget 2026"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteTab(ctx, ch.ID, tabs[0].ID); err != nil {
		t.Fatal(err)
	}
	if tabs, _ = st.ListTabs(ctx, ch.ID); len(tabs) != 0 {
		t.Fatalf("not deleted: %+v", tabs)
	}
	if fs, err := st.ListChannelFiles(ctx, orgID, ch.ID); err != nil || len(fs) != 0 {
		t.Fatalf("files: %+v %v", fs, err)
	}
}
