package authz_test

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

func seedUser(t *testing.T, pool *pgxpool.Pool, orgID, sub string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		 VALUES ($1,'https://issuer.test',$2,$2,$2) RETURNING id::text`, orgID, sub).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPGLoaderAndBackfill(t *testing.T) {
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set")
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
	// Migrate everything, then replay the backfill with legacy admins present.
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var orgID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	first := seedUser(t, pool, orgID, "first")
	second := seedUser(t, pool, orgID, "second")
	member := seedUser(t, pool, orgID, "member")
	grpUser := seedUser(t, pool, orgID, "grp")
	acct := seedUser(t, pool, orgID, "acct")
	for _, q := range []string{
		`INSERT INTO adrient.org_admins (org_id,user_id,granted_at) VALUES ('` + orgID + `','` + first + `', now() - interval '1 day')`,
		`INSERT INTO adrient.org_admins (org_id,user_id) VALUES ('` + orgID + `','` + second + `')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	svc := authz.NewService(authz.PGLoader{Pool: pool})
	authz.SetDefault(svc)
	cctx := func(id string) context.Context {
		c := auth.WithUser(ctx, users.User{ID: id})
		return auth.WithOrg(c, orgs.Org{ID: orgID})
	}

	// Legacy org_admins rows count as Admin at runtime without a backfill row.
	if !authz.IsAdmin(cctx(first)) || !authz.IsAdmin(cctx(second)) {
		t.Fatal("legacy admins must be admin")
	}
	if authz.IsAdmin(cctx(member)) || authz.Can(cctx(member), authz.AccountingRead) || authz.Can(cctx(member), authz.PosRefund) {
		t.Fatal("plain member must not be admin / read ledger / refund")
	}
	if !authz.Can(cctx(member), authz.CRMWrite) || !authz.Can(cctx(member), authz.PosSell) || !authz.Can(cctx(member), authz.InventoryAdjust) {
		t.Fatal("member keeps crm/pos sell/inventory")
	}

	// Group assignment of Manager, direct app_admin(accounting).
	var gid string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.groups (org_id, owner_id, name, member_ids) VALUES ($1,$2,'Mgrs',jsonb_build_array($3::text)) RETURNING id::text`,
		orgID, first, grpUser).Scan(&gid); err != nil {
		t.Fatal(err)
	}
	assign := func(typ, subj, role, scope string) {
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.role_assignments (org_id,subject_type,subject_id,role_id,scope_app)
			SELECT $1,$2,$3::uuid,id,$5 FROM adrient.roles WHERE key=$4 AND org_id IS NULL`, orgID, typ, subj, role, scope); err != nil {
			t.Fatal(err)
		}
	}
	assign("group", gid, "manager", "")
	assign("user", acct, "app_admin", "accounting")
	svc.Invalidate()
	if !authz.Can(cctx(grpUser), authz.PosRefund) || authz.Can(cctx(grpUser), authz.AccountingRead) {
		t.Fatal("group manager: refund yes, ledger no")
	}
	if !authz.Can(cctx(acct), authz.AccountingRead) || authz.Can(cctx(acct), authz.PosRefund) {
		t.Fatal("app admin(accounting): ledger yes, refund no")
	}

	// Guest loses the implicit Member role.
	assign("user", member, "guest", "")
	svc.Invalidate()
	if authz.Can(cctx(member), authz.CRMRead) {
		t.Fatal("guest must not keep member permissions")
	}

	// Backfill statements of 0116 are idempotent and produce Owner for the first admin.
	var owners int
	if _, err := pool.Exec(ctx, `DELETE FROM adrient.role_assignments WHERE role_id IN (SELECT id FROM adrient.roles WHERE key IN ('owner','admin'))`); err != nil {
		t.Fatal(err)
	}
	sqlb, _ := os.ReadFile("../storage/migrations/0116_rbac.sql")
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, string(sqlb)); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id
		WHERE r.key='owner' AND ra.subject_id=$1::uuid`, first).Scan(&owners); err != nil || owners != 1 {
		t.Fatalf("owner backfill: %d %v", owners, err)
	}
	var admins int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id WHERE r.key='admin'`).Scan(&admins)
	if admins != 2 {
		t.Fatalf("admin backfill: %d", admins)
	}
}
