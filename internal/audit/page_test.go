package audit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage/storagetest"
)

// Pagination edge cases against a real, throwaway database: many events with
// the SAME timestamp must still page exactly once each, filters combine with
// the cursor, and the capped count stops early.
func TestListKeysetStableWithEqualTimestamps(t *testing.T) {
	pool := storagetest.ScratchDB(t)
	ctx := context.Background()
	var org string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	same := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for i := 0; i < 47; i++ {
		ts := same
		if i >= 40 {
			ts = same.Add(time.Duration(i) * time.Minute)
		}
		svc := "drive"
		if i%2 == 0 {
			svc = "hr"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.audit_events (org_id, actor_email, service, action, status, created_at)
			VALUES ($1,'a@x',$2,'update','ok',$3)`, org, svc, ts); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewRepository(pool)

	page := func(f Filter) []string {
		var ids []string
		f.Limit = 10
		for guard := 0; guard < 20; guard++ {
			evs, err := repo.List(ctx, org, f)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range evs {
				ids = append(ids, e.ID)
			}
			if len(evs) < 10 {
				return ids
			}
			last := evs[len(evs)-1]
			f.CursorTime, f.CursorID = last.CreatedAt, last.ID
		}
		t.Fatal("paging did not end")
		return nil
	}
	seen := map[string]bool{}
	all := page(Filter{})
	if len(all) != 47 {
		t.Fatalf("paged %d events, want 47", len(all))
	}
	for _, id := range all {
		if seen[id] {
			t.Fatalf("event %s returned twice", id)
		}
		seen[id] = true
	}
	// Filter + cursor: only the 24 "hr" rows, again each once.
	hr := page(Filter{Service: "hr"})
	if len(hr) != 24 {
		t.Fatalf("service filter paged %d, want 24", len(hr))
	}
	// Date jump: events at or before the shared timestamp are the 40 older ones.
	if got := page(Filter{To: same}); len(got) != 40 {
		t.Fatalf("To filter paged %d, want 40", len(got))
	}
	n, capped, err := repo.CountCapped(ctx, org, Filter{}, 10)
	if err != nil || n != 10 || !capped {
		t.Fatalf("CountCapped = %d %v %v, want 10 true", n, capped, err)
	}
	n, capped, _ = repo.CountCapped(ctx, org, Filter{Service: "hr"}, 100)
	if n != 24 || capped {
		t.Fatalf("exact count = %d capped %v", n, capped)
	}
	rows := 0
	if err := repo.Each(ctx, org, Filter{}, 0, func(Event) error { rows++; return nil }); err != nil || rows != 47 {
		t.Fatalf("Each streamed %d rows (%v)", rows, err)
	}
	// Tail mode returns only newer rows, oldest first.
	newest, _ := repo.List(ctx, org, Filter{Limit: 5})
	older := newest[4]
	tail, _ := repo.List(ctx, org, Filter{Limit: 100, CursorTime: older.CreatedAt, CursorID: older.ID, Asc: true})
	if len(tail) != 4 || !tail[0].CreatedAt.Before(tail[3].CreatedAt) && !tail[0].CreatedAt.Equal(tail[3].CreatedAt) {
		t.Fatalf("tail returned %d rows", len(tail))
	}
	_ = fmt.Sprint()
}
