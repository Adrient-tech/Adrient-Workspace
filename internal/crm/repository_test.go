package crm

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateStages(t *testing.T) {
	if err := ValidateStages(nil); err == nil {
		t.Error("empty must fail")
	}
	if err := ValidateStages([]Stage{{ID: "won", Name: "W"}}); err == nil {
		t.Error("reserved id must fail")
	}
	if err := ValidateStages([]Stage{{ID: "a", Name: "A"}, {ID: "a", Name: "B"}}); err == nil {
		t.Error("duplicate must fail")
	}
	if err := ValidateStages([]Stage{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}); err != nil {
		t.Error(err)
	}
}

func TestPipelineLifecycle(t *testing.T) {
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var org, user, contact string
	_ = pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org)
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test','subject-crm','crm@adrient.localtest.me','Crm') RETURNING id::text`, org).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.contacts (org_id, owner_id, display_name) VALUES ($1,$2,'Asha') RETURNING id::text`, org, user).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	r := NewRepository(pool)

	set, err := r.GetSettings(ctx, org)
	if err != nil || set.Currency != "INR" || len(set.Stages) != 4 {
		t.Fatalf("settings %+v %v", set, err)
	}
	raw, err := r.SaveCompany(ctx, org, "", Company{Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	var co struct{ ID string }
	_ = json.Unmarshal(raw, &co)
	if _, err := r.SavePerson(ctx, org, "", Person{ContactID: contact, CompanyID: co.ID}); err != nil {
		t.Fatal(err)
	}
	raw, err = r.SaveDeal(ctx, org, "", Deal{Title: "Big", CompanyID: co.ID, Value: 1000, Billing: "monthly", Committee: []Member{{ContactID: contact, Role: "champion"}}})
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		ID        string
		Stage     string
		Committee []Member
	}
	_ = json.Unmarshal(raw, &d)
	if d.Stage != "new" || len(d.Committee) != 1 {
		t.Fatalf("deal %s", raw)
	}
	if _, err := r.MoveDeal(ctx, org, d.ID, "nope", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad stage err %v", err)
	}
	if _, err := r.MoveDeal(ctx, org, d.ID, "won", "price"); err != nil {
		t.Fatal(err)
	}
	h, _ := r.ListHistory(ctx, org)
	var hist []map[string]any
	_ = json.Unmarshal(h, &hist)
	if len(hist) != 2 {
		t.Fatalf("history %s", h)
	}
	// Removing a stage moves its deals to the first remaining stage.
	raw, _ = r.SaveDeal(ctx, org, "", Deal{Title: "Q", Stage: "negotiation"})
	var q struct{ ID string }
	_ = json.Unmarshal(raw, &q)
	set.Stages = set.Stages[:2]
	if _, err := r.SaveSettings(ctx, org, set); err != nil {
		t.Fatal(err)
	}
	raw, _ = r.GetDeal(ctx, org, q.ID)
	var qd struct{ Stage string }
	_ = json.Unmarshal(raw, &qd)
	if qd.Stage != "new" {
		t.Fatalf("stage after removal %q", qd.Stage)
	}
	// Org isolation.
	if _, err := r.GetDeal(ctx, "00000000-0000-0000-0000-000000000000", d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org read: %v", err)
	}
	if _, err := r.SaveFollowup(ctx, org, "", Followup{Title: "Call", DueDate: "2026-12-01", DealID: d.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveActivity(ctx, org, Activity{Kind: "call", Summary: "Intro", DealID: d.ID, ContactID: contact}); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteDeal(ctx, org, d.ID); err != nil {
		t.Fatal(err)
	}
}
