package demoseed

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

func TestDemoEmailsAreReserved(t *testing.T) {
	e := demoEmail("Aarav 1", "Kulkarni")
	if !strings.HasSuffix(e, ".example") || strings.Contains(e, " ") {
		t.Fatalf("bad demo email %q", e)
	}
}

// TestSeedRemoveRoundTrip needs ADRIENT_TEST_DSN (a scratch database).
func TestSeedRemoveRoundTrip(t *testing.T) {
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		SELECT id,'t','s1','admin@x.test','Admin' FROM adrient.orgs WHERE slug='default'`); err != nil {
		t.Fatal(err)
	}
	rep, err := Seed(ctx, pool, "default")
	if err != nil || rep.Action != "seeded" || rep.Counts["hr_employees"] != 25 || rep.Counts["inventory_products"] != 150 || rep.Counts["crm_deals"] != 30 {
		t.Fatalf("seed: %+v %v", rep, err)
	}
	if rep.Counts["stories"] != 6 || rep.Counts["story_episodes"] != 12 {
		t.Fatalf("stories not seeded: %+v", rep.Counts)
	}
	if again, _ := Seed(ctx, pool, "default"); again.Action != "already_seeded" {
		t.Fatalf("second seed should be a no-op: %+v", again)
	}
	rm, err := Remove(ctx, pool, "default")
	if err != nil || rm.Action != "removed" || len(rm.Left) != 0 {
		t.Fatalf("remove: %+v %v", rm, err)
	}
	for _, tbl := range []string{"hr_employees", "inv_products", "acct_journals", "contacts", "crm_deals", "invc_documents", "story_shows", "story_episodes"} {
		var n int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM adrient."+tbl).Scan(&n)
		if n != 0 {
			t.Errorf("%s still has %d rows after remove", tbl, n)
		}
	}
}
