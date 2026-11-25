package people

import (
	"context"
	"fmt"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage/storagetest"
)

// Pages sorted by a column with many equal values must not repeat or skip
// people, in either direction, and a search must combine with the department.
func TestPageSortTiesAndSearch(t *testing.T) {
	pool := storagetest.ScratchDB(t)
	ctx := context.Background()
	var org, dept string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.departments (org_id, name) VALUES ($1,'Finance Ops') RETURNING id::text`, org).Scan(&dept); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 95; i++ {
		var d any
		if i%5 == 0 {
			d = dept
		}
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, job_title, department_id)
			VALUES ($1,'i',$2,$3,$4,'Analyst',$5)`, org, fmt.Sprint("s", i), fmt.Sprintf("u%d@x.test", i), fmt.Sprintf("User %d", i), d); err != nil {
			t.Fatal(err)
		}
	}
	s := NewStore(pool)
	for _, desc := range []bool{false, true} {
		seen := map[string]bool{}
		for page := 0; page < 5; page++ {
			list, total, capped, more, err := s.Page(ctx, org, PageQuery{Sort: "title", Desc: desc, Limit: 20, Offset: page * 20})
			if err != nil {
				t.Fatal(err)
			}
			if total != 95 || capped {
				t.Fatalf("total %d capped %v", total, capped)
			}
			for _, p := range list {
				if seen[p.ID] {
					t.Fatalf("desc=%v: %s repeated across pages", desc, p.ID)
				}
				seen[p.ID] = true
			}
			if more != (page < 4) {
				t.Fatalf("page %d more=%v", page, more)
			}
		}
		if len(seen) != 95 {
			t.Fatalf("desc=%v saw %d people, want 95", desc, len(seen))
		}
	}
	list, total, _, _, err := s.Page(ctx, org, PageQuery{Q: "finance", Limit: 50})
	if err != nil || total != 19 || len(list) != 19 {
		t.Fatalf("department search: %d rows, total %d, err %v", len(list), total, err)
	}
	if list, _, _, _, _ = s.Page(ctx, org, PageQuery{Q: "100%", Limit: 50}); len(list) != 0 {
		t.Fatalf("a literal %% matched %d people", len(list))
	}
}
