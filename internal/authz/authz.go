// Package authz is the role-based access control core. A permission is a
// string "app:resource:action" (accounting:ledger:read, pos:refund,
// crm:deal:write). Roles (owner, admin, app_admin, manager, member, guest) map
// to permission patterns; "*" matches everything except org:owner, and
// "app:*" matches every permission under that app.
//
// Can/Require read the caller from internal/auth's context. They resolve the
// effective permissions through the process-wide default Service (SetDefault)
// and fail closed when none is installed or the lookup fails.
package authz

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Role keys of the seeded system roles.
const (
	RoleOwner    = "owner"
	RoleAdmin    = "admin"
	RoleAppAdmin = "app_admin"
	RoleManager  = "manager"
	RoleMember   = "member"
	RoleGuest    = "guest"
)

// PermOrgAdmin is the permission the legacy "is admin" checks delegate to.
// Owner and Admin hold it through the "*" wildcard.
const PermOrgAdmin = "org:admin"

// PermOrgOwner is held only by Owner (the "*" wildcard does not cover it).
const PermOrgOwner = "org:owner"

// Assignment is one role granted to a caller: the role key, the app scope for
// app_admin (empty otherwise) and the role's permission patterns.
type Assignment struct {
	RoleKey     string
	ScopeApp    string
	Permissions []string
}

// Loader returns the role assignments that apply to userID in orgID, including
// those inherited through group membership, the implicit Member role and the
// legacy org_admins grant.
type Loader interface {
	Load(ctx context.Context, orgID, userID string) ([]Assignment, error)
}

// Set is the resolved effective permissions of one caller.
type Set struct {
	Roles    []string // role keys, sorted, app_admin shown as "app_admin:<app>"
	patterns []string
}

// NewSet builds a Set from assignments. An app_admin assignment contributes
// "<scope_app>:*" in addition to its stored permissions.
func NewSet(as []Assignment) *Set {
	seenP := map[string]bool{}
	seenR := map[string]bool{}
	s := &Set{}
	add := func(p string) {
		if p != "" && !seenP[p] {
			seenP[p] = true
			s.patterns = append(s.patterns, p)
		}
	}
	for _, a := range as {
		role := a.RoleKey
		if a.RoleKey == RoleAppAdmin {
			if a.ScopeApp == "" {
				continue // an unscoped app_admin grants nothing
			}
			role = RoleAppAdmin + ":" + a.ScopeApp
			add(a.ScopeApp + ":*")
		}
		if !seenR[role] {
			seenR[role] = true
			s.Roles = append(s.Roles, role)
		}
		for _, p := range a.Permissions {
			add(p)
		}
	}
	sort.Strings(s.Roles)
	sort.Strings(s.patterns)
	return s
}

// Can reports whether the set grants perm.
func (s *Set) Can(perm string) bool {
	if s == nil {
		return false
	}
	for _, p := range s.patterns {
		if Match(p, perm) {
			return true
		}
	}
	return false
}

// Patterns returns the permission patterns held (sorted copy).
func (s *Set) Patterns() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.patterns...)
}

// Match reports whether pattern grants perm. "*" grants everything except
// org:owner; "app:*" grants any permission starting with "app:"; anything else
// must be equal.
func Match(pattern, perm string) bool {
	switch {
	case pattern == "" || perm == "":
		return false
	case pattern == perm:
		return true
	case pattern == "*":
		return perm != PermOrgOwner
	case strings.HasSuffix(pattern, ":*"):
		return strings.HasPrefix(perm, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

// Service resolves and briefly caches the effective permissions per caller.
type Service struct {
	loader Loader
	ttl    time.Duration
	now    func() time.Time

	breakGlass func(ctx context.Context) bool

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	set *Set
	exp time.Time
}

// NewService builds a Service. A short TTL keeps role changes visible quickly
// without a query per request.
func NewService(l Loader) *Service {
	return &Service{loader: l, ttl: 5 * time.Second, now: time.Now, cache: map[string]cached{}}
}

// WithBreakGlass installs the ADRIENT_ADMIN_EMAILS style override: callers for
// whom fn returns true hold "*" (admin) regardless of assignments.
func (s *Service) WithBreakGlass(fn func(ctx context.Context) bool) *Service {
	s.breakGlass = fn
	return s
}

// Invalidate drops cached sets (call after changing assignments).
func (s *Service) Invalidate() {
	s.mu.Lock()
	s.cache = map[string]cached{}
	s.mu.Unlock()
}

// Resolve returns the caller's permission set from the auth context.
func (s *Service) Resolve(ctx context.Context) (*Set, error) {
	if _, ok := auth.UserFromContext(ctx); !ok {
		return nil, errNoCaller
	}
	if s.breakGlass != nil && !IsGuest(ctx) && s.breakGlass(ctx) {
		return NewSet([]Assignment{{RoleKey: RoleAdmin, Permissions: []string{"*"}}}), nil
	}
	return s.resolveAssigned(ctx)
}

// CanAssigned is Can without the break-glass override: only stored role
// assignments (and the legacy org_admins grant) count.
func (s *Service) CanAssigned(ctx context.Context, perm string) bool {
	if perm == PermOrgAdmin && delegatedFrom(ctx) {
		return true
	}
	set, err := s.resolveAssigned(ctx)
	return err == nil && set.Can(perm)
}

func (s *Service) resolveAssigned(ctx context.Context) (*Set, error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, errNoCaller
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return nil, errNoCaller
	}
	// The user type is part of the key so a member turned guest never keeps a
	// cached admin set for the TTL.
	key := o.ID + "/" + u.ID + "/" + u.EffectiveType()
	s.mu.Lock()
	if c, hit := s.cache[key]; hit && s.now().Before(c.exp) {
		s.mu.Unlock()
		return c.set, nil
	}
	s.mu.Unlock()
	as, err := s.loader.Load(ctx, o.ID, u.ID)
	if err != nil {
		return nil, err
	}
	if u.EffectiveType() == TypeGuest {
		as = guestClamp(as)
	}
	set := NewSet(as)
	s.mu.Lock()
	if len(s.cache) > 4096 {
		s.cache = map[string]cached{}
	}
	s.cache[key] = cached{set: set, exp: s.now().Add(s.ttl)}
	s.mu.Unlock()
	return set, nil
}

// Can reports whether the caller in ctx holds perm; false on any error.
func (s *Service) Can(ctx context.Context, perm string) bool {
	if perm == PermOrgAdmin && delegatedFrom(ctx) {
		return true
	}
	set, err := s.Resolve(ctx)
	return err == nil && set.Can(perm)
}

var std atomic.Pointer[Service]

// SetDefault installs the process-wide Service used by Can and Require.
func SetDefault(s *Service) { std.Store(s) }

// Default returns the installed Service (nil before SetDefault).
func Default() *Service { return std.Load() }

// Can reports whether the caller in ctx holds perm. Fails closed.
func Can(ctx context.Context, perm string) bool {
	s := std.Load()
	return s != nil && s.Can(ctx, perm)
}

// Require returns nil when the caller holds perm, otherwise a *DeniedError.
func Require(ctx context.Context, perm string) error {
	if Can(ctx, perm) {
		return nil
	}
	return &DeniedError{Permission: perm}
}

// IsAdmin reports whether the caller is Owner or Admin (holds org:admin).
func IsAdmin(ctx context.Context) bool { return Can(ctx, PermOrgAdmin) }

// DeniedError names the permission the caller lacks.
type DeniedError struct{ Permission string }

func (e *DeniedError) Error() string { return "forbidden: missing permission " + e.Permission }

type simpleErr string

func (e simpleErr) Error() string { return string(e) }

const errNoCaller = simpleErr("authz: no authenticated caller")

// DefaultMemberPerms is the Member role used by RepoLoader (no database).
var DefaultMemberPerms = []string{"org:member", "crm:*", "invoicing:*", "inventory:*", "pos:read", "pos:sell", "pos:shift:open"}

// AdminFunc reports an admin grant for (org, user).
type AdminFunc func(ctx context.Context, orgID, userID string) (bool, error)

// AdminOnlyLoader derives roles from a single admin predicate (the legacy
// org_admins repository) when no role tables are reachable: admins are Admin,
// everyone else Member.
type AdminOnlyLoader struct{ IsAdmin AdminFunc }

// Load implements Loader.
func (l AdminOnlyLoader) Load(ctx context.Context, orgID, userID string) ([]Assignment, error) {
	if l.IsAdmin != nil {
		ok, err := l.IsAdmin(ctx, orgID, userID)
		if err != nil {
			return nil, err
		}
		if ok {
			return []Assignment{{RoleKey: RoleAdmin, Permissions: []string{"*"}}}, nil
		}
	}
	return []Assignment{{RoleKey: RoleMember, Permissions: DefaultMemberPerms}}, nil
}

// FallbackLoader tries Primary and, if it errors (for example the role tables
// are not migrated yet), answers from Fallback. It never widens access beyond
// what the legacy admin check already allowed.
type FallbackLoader struct{ Primary, Fallback Loader }

// Load implements Loader.
func (l FallbackLoader) Load(ctx context.Context, orgID, userID string) ([]Assignment, error) {
	as, err := l.Primary.Load(ctx, orgID, userID)
	if err != nil {
		return l.Fallback.Load(ctx, orgID, userID)
	}
	return as, nil
}

// guestClamp removes every admin-level assignment from a guest's roles: a guest
// is never an Owner/Admin/App admin/Manager, whatever role rows or legacy
// org_admins grants exist for the account.
func guestClamp(as []Assignment) []Assignment {
	out := make([]Assignment, 0, len(as))
	for _, a := range as {
		switch a.RoleKey {
		case RoleOwner, RoleAdmin, RoleAppAdmin, RoleManager:
			continue
		}
		out = append(out, a)
	}
	return out
}
