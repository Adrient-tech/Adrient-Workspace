package authz

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Policy subject types and effects (adrient.app_access_policies).
const (
	SubjectEveryone   = "everyone"
	SubjectUser       = "user"
	SubjectGroup      = "group"
	SubjectDepartment = "department"
	SubjectRole       = "role"
	// SubjectUserType is declared in controls.go ("user_type").

	EffectAllow = "allow"
	EffectDeny  = "deny"
)

// CoreApps are always allowed: no policy applies to them.
var CoreApps = map[string]bool{
	"admin": true, "settings": true, "profile": true, "search": true, "notifications": true,
}

// GatedApps lists every launcher app an access policy can target (the app ids
// of web/app/src/catalog/apps.ts, minus the core apps).
var GatedApps = []string{
	"drive", "calendar", "contacts", "whiteboard", "mail", "chat", "meet", "docs", "sheets", "slides",
	"pdf", "forms", "photos", "keep", "tasks", "inventory", "pos", "invoicing", "accounting", "sites",
	"groups", "access", "assemble", "spacelight", "homeassistant", "projects", "crm", "books", "music",
	"video", "live", "git", "cloudimport", "telephony", "vpn", "games", "tickets", "translate", "3d",
	"podcasts", "archaeology", "maps", "learn", "orgsync", "talent", "hr",
	"shifts", "timetracking", "fieldservice", "fleet",
	"records", "wiki", "contracts", "signatures", "legal", "captable",
}

// BusinessApps hold company records (people, payroll, books, stock, customers).
// External users (contractors) never reach them unless an admin names them or
// one of their groups, departments or roles in an allow rule; "everyone" does
// not count. Guests only ever see the apps on their invitation.
var BusinessApps = map[string]bool{
	"hr": true, "talent": true, "shifts": true, "timetracking": true, "fieldservice": true, "fleet": true, "contracts": true, "legal": true, "captable": true, "accounting": true, "inventory": true, "pos": true, "invoicing": true, "crm": true,
}

// externalBlocked reports whether an external user is shut out of appID.
func externalBlocked(policies []Policy, appID string, s Subject) bool {
	if !BusinessApps[appID] || s.userType() != TypeExternal {
		return false
	}
	for _, p := range policies {
		if p.AppID == appID && p.Effect == EffectAllow && p.SubjectType != SubjectEveryone && p.matches(s) {
			return false
		}
	}
	return true
}

// IsKnownApp reports whether id may carry a policy.
func IsKnownApp(id string) bool {
	for _, a := range GatedApps {
		if a == id {
			return true
		}
	}
	return false
}

var httpApps = map[string]string{
	"docs": "docs", "sheets": "sheets", "slides": "slides", "whiteboards": "whiteboard",
	"meet": "meet", "games": "games", "gamerooms": "games", "tickets": "tickets", "books": "books",
	"photos": "photos", "live": "live", "mail": "mail", "chat": "chat", "drive": "drive", "crm": "crm",
	"access": "access", "calendar": "calendar", "forgejo": "git", "pos": "pos", "podcasts": "podcasts", "stories": "podcasts",
	"vpn": "vpn", "orgsync": "orgsync", "invoicing": "invoicing", "accounting": "accounting",
	"inventory": "inventory", "stock": "inventory", "purchasing": "inventory", "forms": "forms", "telephony": "telephony", "tasks": "tasks",
	"projects": "projects", "videos": "video", "video": "video", "music": "music", "keep": "keep",
	"contacts": "contacts", "sites": "sites", "groups": "groups", "talent": "talent", "hr": "hr",
	"shifts": "shifts", "timetracking": "timetracking", "fieldservice": "fieldservice", "fleet": "fleet",
	"records": "records", "wiki": "wiki", "contracts": "contracts", "signatures": "signatures", "legal": "legal", "captable": "captable",
}

// AppForHTTP maps an /api/v1/<prefix>/... path to the app that owns it ("" for
// shared or core routes such as /me, /admin, /auth, /people, /directory).
func AppForHTTP(path string) string {
	rest, ok := strings.CutPrefix(path, "/api/v1/")
	if !ok {
		return ""
	}
	seg, _, _ := strings.Cut(rest, "/")
	return httpApps[seg]
}

var rpcApps = map[string]string{
	"Drive": "drive", "Docs": "docs", "Sheets": "sheets", "Slides": "slides", "Contacts": "contacts",
	"Whiteboards": "whiteboard", "Calendar": "calendar", "Mail": "mail", "Chat": "chat", "Meet": "meet",
	"Telephony": "telephony", "Forms": "forms", "Photos": "photos", "Books": "books", "Video": "video",
	"Live": "live", "Music": "music", "Projects": "projects", "Keep": "keep", "Tasks": "tasks",
	"Inventory": "inventory", "Pos": "pos", "Sites": "sites", "Groups": "groups",
}

// AppForRPC maps "/adrient.v1.PosService/Method" to its app id ("" for core
// services: Admin, Auth, Health, Notifications, Preferences, Search).
func AppForRPC(fullMethod string) string {
	rest, ok := strings.CutPrefix(fullMethod, "/adrient.v1.")
	if !ok {
		return ""
	}
	svc, _, _ := strings.Cut(rest, "/")
	return rpcApps[strings.TrimSuffix(svc, "Service")]
}

// Policy is one row of adrient.app_access_policies.
type Policy struct {
	ID          string `json:"id,omitempty"`
	AppID       string `json:"service_id"`
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
	Effect      string `json:"effect"`
	// Condition optionally limits when the rule applies (IP range, user type).
	Condition *Condition `json:"condition,omitempty"`
}

// Subject describes the caller for policy matching. DeptIDs holds the user's
// department and all its ancestors, so a policy on a parent department covers
// the whole subtree.
type Subject struct {
	UserID   string
	GroupIDs []string
	DeptIDs  []string
	RoleKeys []string
	UserType string          // member | external | guest | service
	IP       string          // caller IP, for conditions
	Admin    bool            // Owner/Admin (org:admin)
	AppAdmin map[string]bool // apps the caller administers
}

func (p Policy) matches(s Subject) bool {
	if ok, _ := p.Condition.Met(s); !ok {
		return false
	}
	switch p.SubjectType {
	case SubjectUserType:
		return p.SubjectID == s.userType()
	case SubjectEveryone:
		return true
	case SubjectUser:
		return p.SubjectID == s.UserID
	case SubjectGroup:
		return contains(s.GroupIDs, p.SubjectID)
	case SubjectDepartment:
		return contains(s.DeptIDs, p.SubjectID)
	case SubjectRole:
		return contains(s.RoleKeys, p.SubjectID)
	}
	return false
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// Decision is the resolved access of one app for one subject.
type Decision struct {
	AppID   string `json:"id"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

// Resolve applies the policy rules for appID: core apps, Admin and the app's
// own App admin always pass; otherwise a matching deny blocks; with no allow
// rows everyone passes, else the caller must match one allow row.
func Resolve(policies []Policy, appID string, s Subject) Decision {
	d := func(ok bool, why string) Decision { return Decision{AppID: appID, Allowed: ok, Reason: why} }
	if CoreApps[appID] {
		return d(true, "core app")
	}
	if s.Admin {
		return d(true, "org admin")
	}
	if s.AppAdmin[appID] {
		return d(true, "app admin")
	}
	if externalBlocked(policies, appID, s) {
		return d(false, "external users cannot open business apps unless named in an allow rule")
	}
	var anyAllow, allowed bool
	for _, p := range policies {
		if p.AppID != appID {
			continue
		}
		switch p.Effect {
		case EffectDeny:
			if p.matches(s) {
				return d(false, "denied by "+p.SubjectType+" rule")
			}
		case EffectAllow:
			anyAllow = true
			if p.matches(s) {
				allowed = true
			}
		}
	}
	switch {
	case !anyAllow:
		return d(true, "no restriction")
	case allowed:
		return d(true, "allowed by policy")
	}
	return d(false, "not in any allowed group, department, role or user")
}

// AppStore loads policies and the caller's directory facts.
type AppStore interface {
	Policies(ctx context.Context, orgID string) ([]Policy, error)
	// Facts returns the user's group ids and department ids (own + ancestors).
	Facts(ctx context.Context, orgID, userID string) (groups, depts []string, err error)
}

// PGAppStore is the Postgres AppStore.
type PGAppStore struct{ Pool *pgxpool.Pool }

// Policies implements AppStore.
func (s PGAppStore) Policies(ctx context.Context, orgID string) ([]Policy, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, service_id, subject_type, subject_id, effect, condition
FROM adrient.app_access_policies WHERE org_id = $1 ORDER BY service_id, subject_type, subject_id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("authz.Policies: %w", err)
	}
	defer rows.Close()
	var out []Policy
	for rows.Next() {
		var p Policy
		var cond []byte
		if err := rows.Scan(&p.ID, &p.AppID, &p.SubjectType, &p.SubjectID, &p.Effect, &cond); err != nil {
			return nil, err
		}
		p.Condition = ParseCondition(cond)
		out = append(out, p)
	}
	return out, rows.Err()
}

// Facts implements AppStore.
func (s PGAppStore) Facts(ctx context.Context, orgID, userID string) ([]string, []string, error) {
	var groups, depts []string
	gr, err := s.Pool.Query(ctx, `SELECT group_id::text FROM adrient.group_members WHERE org_id=$1 AND user_id=$2::uuid`, orgID, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("authz.Facts groups: %w", err)
	}
	for gr.Next() {
		var g string
		if err := gr.Scan(&g); err != nil {
			gr.Close()
			return nil, nil, err
		}
		groups = append(groups, g)
	}
	gr.Close()
	if err := gr.Err(); err != nil {
		return nil, nil, err
	}
	dr, err := s.Pool.Query(ctx, `
WITH RECURSIVE d(id) AS (
    SELECT department_id FROM adrient.users WHERE id=$2::uuid AND org_id=$1 AND department_id IS NOT NULL
    UNION
    SELECT p.parent_id FROM adrient.departments p JOIN d ON p.id = d.id WHERE p.parent_id IS NOT NULL
) SELECT id::text FROM d`, orgID, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("authz.Facts departments: %w", err)
	}
	for dr.Next() {
		var x string
		if err := dr.Scan(&x); err != nil {
			dr.Close()
			return nil, nil, err
		}
		depts = append(depts, x)
	}
	dr.Close()
	return groups, depts, dr.Err()
}

// AppService resolves app access for callers. Results are cached briefly.
type AppService struct {
	store AppStore
	roles *Service
	ttl   time.Duration
	now   func() time.Time

	mu       sync.Mutex
	policies map[string]cachedPolicies
}

type cachedPolicies struct {
	list []Policy
	exp  time.Time
}

// NewAppService builds the service; roles resolves the caller's role set.
func NewAppService(store AppStore, roles *Service) *AppService {
	return &AppService{store: store, roles: roles, ttl: 5 * time.Second, now: time.Now, policies: map[string]cachedPolicies{}}
}

// Invalidate drops cached policies (call after editing them).
func (a *AppService) Invalidate() {
	a.mu.Lock()
	a.policies = map[string]cachedPolicies{}
	a.mu.Unlock()
}

func (a *AppService) orgPolicies(ctx context.Context, orgID string) ([]Policy, error) {
	a.mu.Lock()
	if c, ok := a.policies[orgID]; ok && a.now().Before(c.exp) {
		a.mu.Unlock()
		return c.list, nil
	}
	a.mu.Unlock()
	l, err := a.store.Policies(ctx, orgID)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.policies[orgID] = cachedPolicies{list: l, exp: a.now().Add(a.ttl)}
	a.mu.Unlock()
	return l, nil
}

func subjectFrom(userID string, set *Set, groups, depts []string) Subject {
	s := Subject{UserID: userID, UserType: TypeMember, GroupIDs: groups, DeptIDs: depts, Admin: set.Can(PermOrgAdmin), AppAdmin: map[string]bool{}}
	for _, r := range set.Roles {
		if app, ok := strings.CutPrefix(r, RoleAppAdmin+":"); ok {
			s.AppAdmin[app] = true
			s.RoleKeys = append(s.RoleKeys, RoleAppAdmin)
			continue
		}
		s.RoleKeys = append(s.RoleKeys, r)
	}
	return s
}

// ResolveAll returns the decision for every gated app for userID. set is the
// user's role set.
func (a *AppService) ResolveAll(ctx context.Context, orgID, userID string, set *Set) ([]Decision, error) {
	pol, err := a.orgPolicies(ctx, orgID)
	if err != nil {
		return nil, err
	}
	groups, depts, err := a.store.Facts(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	sub := subjectFrom(userID, set, groups, depts)
	sub.IP = ClientIP(ctx)
	if utype, ok := userTypeOf(ctx, a, orgID, userID); ok {
		sub.UserType = utype
	}
	out := make([]Decision, 0, len(GatedApps))
	// A guest caller sees only the apps on their invitation.
	guest, guestApps := false, []string(nil)
	if cu, ok := auth.UserFromContext(ctx); ok && cu.ID == userID && cu.EffectiveType() == TypeGuest {
		guest, guestApps = true, cu.GuestApps
	}
	for _, id := range GatedApps {
		if guest {
			ok := contains(guestApps, id)
			why := "guest: not on invitation"
			if ok {
				why = "guest: on invitation"
			}
			if ok && deniedBy(pol, id, sub) {
				ok, why = false, "guest: denied by user type rule"
			}
			out = append(out, Decision{AppID: id, Allowed: ok, Reason: why})
			continue
		}
		out = append(out, Resolve(pol, id, sub))
	}
	return out, nil
}

// CanOpen reports whether the caller in ctx may open appID. Fails closed on a
// lookup error (core apps never fail).
func (a *AppService) CanOpen(ctx context.Context, appID string) bool {
	if CoreApps[appID] {
		return true
	}
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return false
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return false
	}
	if u.EffectiveType() == TypeGuest {
		// Guests open only the apps listed on their invitation (users.guest_apps),
		// and a deny rule (for example on user type guest) still closes the app.
		if !contains(u.GuestApps, appID) {
			return false
		}
		pol, err := a.orgPolicies(ctx, o.ID)
		if err != nil {
			return false
		}
		return !deniedBy(pol, appID, Subject{UserID: u.ID, UserType: TypeGuest, IP: ClientIP(ctx)})
	}
	set, err := a.roles.Resolve(ctx)
	if err != nil {
		return false
	}
	pol, err := a.orgPolicies(ctx, o.ID)
	if err != nil {
		return false
	}
	hasRule := false
	for _, p := range pol {
		if p.AppID == appID {
			hasRule = true
			break
		}
	}
	if !hasRule && !(BusinessApps[appID] && u.EffectiveType() == TypeExternal) { // common case: skip the directory lookups
		return true
	}
	groups, depts, err := a.store.Facts(ctx, o.ID, u.ID)
	if err != nil {
		return false
	}
	sub := subjectFrom(u.ID, set, groups, depts)
	sub.UserType, sub.IP = u.EffectiveType(), ClientIP(ctx)
	return Resolve(pol, appID, sub).Allowed
}

// deniedBy reports whether a deny rule for appID matches s.
func deniedBy(policies []Policy, appID string, s Subject) bool {
	for _, p := range policies {
		if p.AppID == appID && p.Effect == EffectDeny && p.matches(s) {
			return true
		}
	}
	return false
}

// userTypeOf returns the type of userID: from the context when it is the
// caller, otherwise from the store when it can tell.
func userTypeOf(ctx context.Context, a *AppService, orgID, userID string) (string, bool) {
	if cu, ok := auth.UserFromContext(ctx); ok && cu.ID == userID {
		return cu.EffectiveType(), true
	}
	if t, ok := a.store.(interface {
		UserType(ctx context.Context, orgID, userID string) (string, error)
	}); ok {
		if v, err := t.UserType(ctx, orgID, userID); err == nil {
			return v, true
		}
	}
	return "", false
}

// UserType implements the optional store lookup used for "preview as user".
func (s PGAppStore) UserType(ctx context.Context, orgID, userID string) (string, error) {
	var t string
	err := s.Pool.QueryRow(ctx, `SELECT user_type FROM adrient.users WHERE id=$1::uuid AND org_id=$2`, userID, orgID).Scan(&t)
	return t, err
}

// ResolveUser resolves the role set of another user (admin "preview as user");
// the break-glass override is not applied.
func (s *Service) ResolveUser(ctx context.Context, orgID, userID string) (*Set, error) {
	as, err := s.loader.Load(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return NewSet(as), nil
}

var stdApps atomic.Pointer[AppService]

// SetDefaultApps installs the process-wide AppService used by CanOpenApp.
func SetDefaultApps(a *AppService) { stdApps.Store(a) }

// DefaultApps returns the installed AppService (nil before SetDefaultApps).
func DefaultApps() *AppService { return stdApps.Load() }

// CanOpenApp reports whether the caller in ctx may open appID. With no
// AppService installed (no database) no policies exist, so every app is open.
func CanOpenApp(ctx context.Context, appID string) bool {
	a := stdApps.Load()
	if a == nil {
		return true
	}
	return a.CanOpen(ctx, appID)
}

func appPerm(app string) string { return app + ":app:open" }

// AppGuard enforces CanOpenApp on every API route that belongs to an app. It
// must run inside the auth middleware.
func AppGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Anonymous callers (share links, public tokens, unauthenticated probes) have no
		// app access to check: the route's own handler decides between 401 and a token grant.
		if _, authed := auth.UserFromContext(r.Context()); !authed {
			next.ServeHTTP(w, r)
			return
		}
		if app := AppForHTTP(r.URL.Path); app != "" && !CanOpenApp(r.Context(), app) {
			reportDenied(r.Context(), Denial{Permission: appPerm(app), Method: r.Method, Target: r.URL.Path, IP: httpClientIP(r), UserAgent: r.UserAgent()})
			WriteForbidden(w, appPerm(app))
			return
		}
		next.ServeHTTP(w, r)
	})
}
