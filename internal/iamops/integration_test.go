package iamops

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelmgmt"
)

// Integration test against the dev identity provider and a SCRATCH database.
// Run with:
//
//	IAM_TEST_DSN=postgres://adrient@127.0.0.1:5533/w9iam_test?sslmode=disable \
//	IAM_TEST_IDP=http://localhost:8081 IAM_TEST_PAT=$(cat .../bootstrap-pat.txt) \
//	go test ./internal/iamops -run Integration
//
// It creates one throwaway identity provider user and deletes it at the end. It
// refuses to run against a database named adrient or the identity provider database.
func TestIntegrationActions(t *testing.T) {
	dsn, zurl, pat := os.Getenv("IAM_TEST_DSN"), os.Getenv("IAM_TEST_IDP"), os.Getenv("IAM_TEST_PAT")
	if dsn == "" || zurl == "" || pat == "" {
		t.Skip("set IAM_TEST_DSN, IAM_TEST_IDP and IAM_TEST_PAT to run")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if db := strings.Trim(u.Path, "/"); db == "adrient" || db == "zitadel" || db == "" {
		t.Fatalf("refusing to run against database %q", db)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	idp := zitadelmgmt.New(zurl, pat)

	var orgID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs ORDER BY created_at LIMIT 1`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	stamp := fmt.Sprint(time.Now().UnixNano())
	email := "w9iam.it" + stamp + "@adrient.localtest.me"
	zid, err := idp.CreateHumanUser(ctx, zitadelmgmt.NewUser{Username: email, Email: email, GivenName: "It", FamilyName: "Test"})
	if err != nil {
		t.Fatalf("create idp user: %v", err)
	}
	defer idp.DeleteUser(ctx, zid)
	if err := idp.SetPassword(ctx, zid, "Start-Pass#2026a", false); err != nil {
		t.Fatal(err)
	}
	var uid string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1::uuid,'http://localhost:8081',$2,$3,'Integration Test') RETURNING id::text`, orgID, zid, email).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM adrient.users WHERE id::text=$1`, uid)
	var auditN, notified int
	svc := &Service{Pool: pool, IdP: idp,
		Audit:       func(context.Context, string, string, string, map[string]any) { auditN++ },
		Notify:      func(context.Context, string, string, string, string, string) { notified++ },
		IsAdminUser: func(context.Context, string, string) bool { return false }}
	admin := Actor{ID: "00000000-0000-0000-0000-00000000aaaa", Email: "ops@x.test", Roles: []string{RoleIdentityAdmin}, SessionKey: "it-" + stamp}
	help := Actor{ID: admin.ID, Roles: []string{RoleHelpDesk}, SessionKey: "it2-" + stamp}
	defer pool.Exec(ctx, `DELETE FROM adrient.iam_stepups WHERE session_key LIKE 'it%-'||$1`, stamp)
	defer pool.Exec(ctx, `DELETE FROM adrient.iam_actions WHERE target_id::text=$1`, uid)

	do := func(a Actor, act string, params map[string]any, dry bool) (Result, error) {
		return svc.Do(ctx, orgID, a, uid, ActionReq{Action: act, Reason: "integration test", Ticket: "IT-1", Params: params, DryRun: dry})
	}

	// permission: help desk cannot contain or suspend
	if _, err := do(help, "contain", nil, false); err == nil {
		t.Fatal("help desk must not contain")
	}
	// reason is required
	if _, err := svc.Do(ctx, orgID, admin, uid, ActionReq{Action: "password.reset_link"}); err == nil {
		t.Fatal("empty reason accepted")
	}
	// reset link works without step-up
	if r, err := do(admin, "password.reset_link", nil, false); err != nil || r.Status != "done" {
		t.Fatalf("reset link: %v %+v", err, r)
	}
	// temp password needs step-up
	if _, err := do(admin, "password.temp", nil, false); err != ErrStepUp {
		t.Fatalf("want step-up, got %v", err)
	}
	if err := svc.RecordStepUp(ctx, admin.ID, admin.SessionKey, "test"); err != nil {
		// iam_stepups.user_id is a uuid; the fixed id above is valid
		t.Fatal(err)
	}
	r, err := do(admin, "password.temp", nil, false)
	if err != nil || r.Secret == "" {
		t.Fatalf("temp password: %v %+v", err, r)
	}
	// the temporary password really works at the identity provider
	if err := idp.VerifyCredentials(ctx, email, r.Secret, ""); err != nil {
		t.Fatalf("temporary password rejected by identity provider: %v", err)
	}
	// MFA: enrol an e-mail code factor, list it, revoke it
	if err := idp.SetEmail(ctx, zid, "v"+email, true); err != nil {
		t.Fatal(err)
	}
	if err := idp.AddOTPEmail(ctx, zid); err != nil {
		t.Fatal(err)
	}
	fs, err := idp.ListFactors(ctx, zid)
	if err != nil || len(fs) != 1 {
		t.Fatalf("factors: %v %v", err, fs)
	}
	if _, err := do(admin, "mfa.revoke", map[string]any{"kind": fs[0].Kind, "factor_id": fs[0].ID}, false); err != nil {
		t.Fatalf("mfa.revoke: %v", err)
	}
	if fs, _ := idp.ListFactors(ctx, zid); len(fs) != 0 {
		t.Fatalf("factor still there: %v", fs)
	}
	// contain: dry run changes nothing, real run twice is idempotent
	_ = idp.AddOTPEmail(ctx, zid)
	dry, err := do(admin, "contain", nil, true)
	if err != nil || dry.Status != "dry_run" || len(dry.Steps) != 4 {
		t.Fatalf("dry run: %v %+v", err, dry)
	}
	if fs, _ := idp.ListFactors(ctx, zid); len(fs) != 1 {
		t.Fatal("dry run removed a factor")
	}
	first, err := do(admin, "contain", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if st := stepStatus(first, "Remove second steps"); st != "done" {
		t.Fatalf("first contain second steps = %s", st)
	}
	second, err := do(admin, "contain", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if stepStatus(second, "Reset password") != "skipped" || stepStatus(second, "Remove second steps") != "skipped" {
		t.Fatalf("second contain not idempotent: %+v", second.Steps)
	}
	// the old password no longer works after containment
	if err := idp.VerifyCredentials(ctx, email, "Start-Pass#2026a", ""); err == nil {
		t.Fatal("old password still accepted after contain")
	}
	// audit, notification and action log
	var logged int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.iam_actions WHERE target_id::text=$1 AND status='done' AND ticket='IT-1'`, uid).Scan(&logged)
	if logged < 5 || auditN < 5 || notified < 4 {
		t.Fatalf("logged=%d audit=%d notified=%d", logged, auditN, notified)
	}
	// suspend / unsuspend through the hooks
	svc.Suspend = func(ctx context.Context, o, u, actor, reason string) error {
		_, err := pool.Exec(ctx, `UPDATE adrient.users SET status='blocked' WHERE id::text=$1`, u)
		return err
	}
	if _, err := do(admin, "account.suspend", nil, false); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	var st string
	_ = pool.QueryRow(ctx, `SELECT status FROM adrient.users WHERE id::text=$1`, uid).Scan(&st)
	if st != "blocked" {
		t.Fatalf("status %s", st)
	}

	// administrator targets: not reachable without super admin; the last admin is never locked out
	svc.IsAdminUser = func(_ context.Context, _, id string) bool { return id == uid }
	if _, err := do(admin, "password.reset_link", nil, false); err == nil || !strings.Contains(err.Error(), "super admin") {
		t.Fatalf("non-super admin reached an administrator: %v", err)
	}
	super := admin
	super.Super = true
	if _, err := do(super, "account.suspend", nil, false); err == nil || !strings.Contains(err.Error(), "last active administrator") {
		t.Fatalf("last admin suspend not refused: %v", err)
	}
	// two-person approval: the requester cannot approve their own request
	svc.IsAdminUser = func(context.Context, string, string) bool { return false }
	if _, err := pool.Exec(ctx, `UPDATE adrient.users SET status='active' WHERE id::text=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutSettings(ctx, orgID, admin.ID, Settings{Thresholds: DefaultThresholds(), RequireApproval: []string{"mfa.bypass"}, MFAPolicy: MFAPolicy{Mode: "off"}}); err != nil {
		t.Fatal(err)
	}
	defer svc.PutSettings(ctx, orgID, admin.ID, Settings{Thresholds: DefaultThresholds(), MFAPolicy: MFAPolicy{Mode: "off"}})
	res, err := do(admin, "mfa.bypass", map[string]any{"hours": float64(2)}, false)
	if err != nil || res.Status != "pending_approval" || res.Approval == "" {
		t.Fatalf("approval not requested: %v %+v", err, res)
	}
	if _, err := svc.Decide(ctx, orgID, admin, res.Approval, true); err == nil {
		t.Fatal("requester approved own request")
	}
	other := Actor{ID: "00000000-0000-0000-0000-00000000bbbb", Email: "second@x.test", Roles: []string{RoleIdentityAdmin}, SessionKey: "it3-" + stamp}
	if err := svc.RecordStepUp(ctx, other.ID, other.SessionKey, "test"); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Decide(ctx, orgID, other, res.Approval, true)
	if err != nil || out.Secret == "" {
		t.Fatalf("approved bypass: %v %+v", err, out)
	}
	defer pool.Exec(ctx, `DELETE FROM adrient.iam_approvals WHERE id::text=$1`, res.Approval)
}

func stepStatus(r Result, name string) string {
	for _, s := range r.Steps {
		if s.Name == name {
			return s.Status
		}
	}
	return ""
}
