package authz

import (
	"context"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// User types (users.user_type). The zero value of an unset type is TypeMember.
const (
	TypeMember   = "member"
	TypeExternal = "external"
	TypeGuest    = "guest"
	TypeService  = "service"
)

// UserType returns the caller's user type ("" when unauthenticated).
func UserType(ctx context.Context) string {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return ""
	}
	return u.EffectiveType()
}

// IsExternal reports whether the caller is a guest or an external (contractor)
// user, i.e. someone outside the org's own member population. Modules use it
// for "must wait in the lobby", "hide from directory" style decisions.
// Unauthenticated callers are NOT external (they are anonymous).
func IsExternal(ctx context.Context) bool {
	t := UserType(ctx)
	return t == TypeGuest || t == TypeExternal
}

// IsGuest reports whether the caller is a B2B guest (limited apps, guest role).
func IsGuest(ctx context.Context) bool { return UserType(ctx) == TypeGuest }
