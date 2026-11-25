package drive

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jackc/pgx/v5/pgxpool"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

func driveAuthCtx(orgID, userID string) context.Context {
	ctx := auth.WithUser(context.Background(), users.User{ID: userID, OrgID: orgID, Email: "u@test"})
	return auth.WithOrg(ctx, orgs.Org{ID: orgID, Slug: "s", DisplayName: "S"})
}

func makeOrgUser(t *testing.T, pool *pgxpool.Pool, slug, subject, email string) (string, string) {
	t.Helper()
	var orgID, userID string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO adrient.orgs (slug, display_name) VALUES ($1, $1) RETURNING id::text`, slug,
	).Scan(&orgID); err != nil {
		t.Fatalf("create org %s: %v", slug, err)
	}
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		 VALUES ($1, 'test', $2, $3, $3) RETURNING id::text`, orgID, subject, email,
	).Scan(&userID); err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return orgID, userID
}

// TestDriveCrossOrgGrant proves the per-user ACL read gate for Drive: a user in
// a different org cannot GetFile without a grant (NotFound), can once granted,
// the grant surfaces in their "Shared with me", and revocation re-denies.
func TestDriveCrossOrgGrant(t *testing.T) {
	pool, orgA, alice := setupDB(t)
	grantsRepo := sharing.NewRepository(pool)
	svc := NewService(NewRepository(pool), NewACL(pool), nil).WithSharing(grantsRepo, nil)

	bobOrg, bob := makeOrgUser(t, pool, "personal-bob", "subj-bob", "bob@test")

	aliceCtx := driveAuthCtx(orgA, alice)
	bobCtx := driveAuthCtx(bobOrg, bob)

	// Alice owns a file in org A.
	f, err := NewRepository(pool).CreateFile(context.Background(), orgA, alice, "", "secret.txt", "text/plain", "blobs/secret", 10)
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}

	// 1. Bob cannot see it without a grant.
	if _, err := svc.GetFile(bobCtx, &adrientv1.GetFileRequest{Id: f.ID}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetFile non-grantee: got %v want NotFound", status.Code(err))
	}

	// 2. Alice grants bob viewer.
	if _, err := svc.GrantAccess(aliceCtx, &adrientv1.GrantAccessRequest{
		FileId: f.ID, GranteeUserId: bob, Role: sharing.RoleViewer,
	}); err != nil {
		t.Fatalf("GrantAccess: %v", err)
	}

	// 3. Bob can now read it (cross-org).
	got, err := svc.GetFile(bobCtx, &adrientv1.GetFileRequest{Id: f.ID})
	if err != nil {
		t.Fatalf("GetFile grantee: %v", err)
	}
	if got.GetFile().GetName() != "secret.txt" {
		t.Fatalf("GetFile grantee name = %q", got.GetFile().GetName())
	}

	// 4. "Shared with me" for bob includes the file; alice's is empty.
	swm, err := svc.ListSharedWithMe(bobCtx, &adrientv1.ListSharedWithMeRequest{})
	if err != nil || len(swm.GetFiles()) != 1 || swm.GetFiles()[0].GetId() != f.ID {
		t.Fatalf("bob ListSharedWithMe = %+v, %v; want the one file", swm.GetFiles(), err)
	}
	if aswm, _ := svc.ListSharedWithMe(aliceCtx, &adrientv1.ListSharedWithMeRequest{}); len(aswm.GetFiles()) != 0 {
		t.Fatalf("alice ListSharedWithMe = %+v; want empty", aswm.GetFiles())
	}

	// 5. Bob (grantee, not member) cannot manage grants on the file.
	if _, err := svc.GrantAccess(bobCtx, &adrientv1.GrantAccessRequest{
		FileId: f.ID, GranteeUserId: alice, Role: sharing.RoleViewer,
	}); status.Code(err) != codes.NotFound {
		t.Fatalf("bob GrantAccess: got %v want NotFound", status.Code(err))
	}

	// 6. Revoke → bob denied again.
	if _, err := svc.RevokeAccess(aliceCtx, &adrientv1.RevokeAccessRequest{FileId: f.ID, GranteeUserId: bob}); err != nil {
		t.Fatalf("RevokeAccess: %v", err)
	}
	if _, err := svc.GetFile(bobCtx, &adrientv1.GetFileRequest{Id: f.ID}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetFile after revoke: got %v want NotFound", status.Code(err))
	}
}
