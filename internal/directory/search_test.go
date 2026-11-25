package directory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage/storagetest"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

func TestSearchPagingOrgScopeAndFilters(t *testing.T) {
	pool := storagetest.ScratchDB(t)
	ctx := context.Background()
	var org, other string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ('other','Other') RETURNING id::text`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	// 120 people, 40 of them share the display name "Sam Same" (cursor ties).
	for i := 0; i < 120; i++ {
		name := fmt.Sprintf("Person %03d", i)
		if i%3 == 0 {
			name = "Sam Same"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, employee_id)
			VALUES ($1,'i',$2,$3,$4,$5)`, org, fmt.Sprint("s", i), fmt.Sprintf("p%d@x.test", i), name, fmt.Sprintf("EMP%04d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'i','z','outsider@x.test','Sam Same')`, other); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE adrient.users SET status='blocked' WHERE email='p3@x.test'`); err != nil {
		t.Fatal(err)
	}
	h := NewSearchHandler(pool)
	get := func(query string) pagekit.Envelope {
		r := httptest.NewRequest(http.MethodGet, SearchPath+"?"+query, nil)
		c := auth.WithOrg(auth.WithUser(r.Context(), users.User{ID: "caller", OrgID: org}), orgs.Org{ID: org, Slug: "default"})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r.WithContext(c))
		if w.Code != 200 {
			t.Fatalf("%s -> %d %s", query, w.Code, w.Body.String())
		}
		var raw struct {
			Items      []Entry `json:"items"`
			NextCursor string  `json:"next_cursor"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		return pagekit.Envelope{Items: raw.Items, NextCursor: raw.NextCursor}
	}
	// Walk every page of the whole directory: each person exactly once, never above 50 per page.
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; pages < 20; pages++ {
		e := get("limit=999&cursor=" + cursor)
		items := e.Items.([]Entry)
		if len(items) > MaxSearchLimit {
			t.Fatalf("page of %d exceeds the cap", len(items))
		}
		for _, it := range items {
			if seen[it.ID] {
				t.Fatalf("%s listed twice", it.ID)
			}
			seen[it.ID] = true
		}
		if e.NextCursor == "" {
			break
		}
		cursor = e.NextCursor
	}
	if len(seen) != 119 { // 120 minus the blocked one; the other org's user never appears
		t.Fatalf("listed %d people, want 119", len(seen))
	}
	if got := get("q=EMP0042").Items.([]Entry); len(got) != 1 || got[0].EmployeeID != "EMP0042" {
		t.Fatalf("employee id search = %+v", got)
	}
	if got := get("q=sam%20same&status=any&limit=50").Items.([]Entry); len(got) != 40 {
		t.Fatalf("name search returned %d, want 40 (other org excluded)", len(got))
	}
	if got := get("q=100%25").Items.([]Entry); len(got) != 0 {
		t.Fatalf("a literal %% must not act as a wildcard, got %d", len(got))
	}
}
