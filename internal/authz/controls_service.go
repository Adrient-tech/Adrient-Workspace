package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// ControlStore loads the org's control rules.
type ControlStore interface {
	Rules(ctx context.Context, orgID string) ([]ControlRule, error)
}

// PGControlStore is the Postgres ControlStore.
type PGControlStore struct{ Pool *pgxpool.Pool }

// Rules implements ControlStore.
func (s PGControlStore) Rules(ctx context.Context, orgID string) ([]ControlRule, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, app_id, control, value, subject_type, subject_id,
COALESCE(condition,'null'::jsonb), priority, effect_mode, note
FROM adrient.app_control_rules WHERE org_id=$1 ORDER BY created_at, id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("authz.Rules: %w", err)
	}
	defer rows.Close()
	var out []ControlRule
	for rows.Next() {
		var r ControlRule
		var val, cond []byte
		if err := rows.Scan(&r.ID, &r.AppID, &r.Control, &val, &r.SubjectType, &r.SubjectID, &cond, &r.Priority, &r.EffectMode, &r.Note); err != nil {
			return nil, err
		}
		if json.Unmarshal(val, &r.Value) != nil {
			continue
		}
		r.Condition = ParseCondition(cond)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ControlService resolves controls for callers and caches rules briefly.
type ControlService struct {
	store ControlStore
	facts AppStore
	roles *Service
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	rules map[string]cachedRules
}

type cachedRules struct {
	list []ControlRule
	exp  time.Time
}

// NewControlService builds the service. facts supplies group/department
// membership; roles resolves role keys.
func NewControlService(store ControlStore, facts AppStore, roles *Service) *ControlService {
	return &ControlService{store: store, facts: facts, roles: roles, ttl: 5 * time.Second, now: time.Now, rules: map[string]cachedRules{}}
}

// Invalidate drops cached rules (call after editing them).
func (c *ControlService) Invalidate() {
	c.mu.Lock()
	c.rules = map[string]cachedRules{}
	c.mu.Unlock()
}

func (c *ControlService) orgRules(ctx context.Context, orgID string) ([]ControlRule, error) {
	c.mu.Lock()
	if e, ok := c.rules[orgID]; ok && c.now().Before(e.exp) {
		c.mu.Unlock()
		return e.list, nil
	}
	c.mu.Unlock()
	l, err := c.store.Rules(ctx, orgID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.rules[orgID] = cachedRules{list: l, exp: c.now().Add(c.ttl)}
	c.mu.Unlock()
	return l, nil
}

// SubjectFor builds the subject for userID. roleSet may be nil.
func (c *ControlService) SubjectFor(ctx context.Context, orgID, userID, userType, ip string, set *Set) (Subject, error) {
	groups, depts, err := c.facts.Facts(ctx, orgID, userID)
	if err != nil {
		return Subject{}, err
	}
	if set == nil {
		set = NewSet(nil)
	}
	s := subjectFrom(userID, set, groups, depts)
	s.UserType, s.IP = userType, ip
	if s.UserType == "" {
		s.UserType = TypeMember
	}
	return s, nil
}

// restrictiveValue is the fail-closed value used when rules cannot be read.
func restrictiveValue(d ControlDef) any {
	switch d.Type {
	case CtlBool:
		return false
	case CtlEnum:
		return d.Enum[0]
	}
	return d.Min
}

// ResolveFor resolves key for an explicit subject.
func (c *ControlService) ResolveFor(ctx context.Context, orgID string, sub Subject, key string) (ControlResult, error) {
	def, ok := ControlByKey(key)
	if !ok {
		return ControlResult{}, fmt.Errorf("unknown control %q", key)
	}
	rules, err := c.orgRules(ctx, orgID)
	if err != nil {
		return ControlResult{Key: key, App: def.App, Value: restrictiveValue(def), Source: "error"}, err
	}
	return ResolveControl(def, rules, sub), nil
}

// ResolveAllFor resolves every control for sub.
func (c *ControlService) ResolveAllFor(ctx context.Context, orgID string, sub Subject) ([]ControlResult, error) {
	rules, err := c.orgRules(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]ControlResult, 0, len(Controls))
	for _, d := range Controls {
		out = append(out, ResolveControl(d, rules, sub))
	}
	return out, nil
}

// callerSubject builds the subject of the authenticated caller in ctx.
func (c *ControlService) callerSubject(ctx context.Context) (string, Subject, error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return "", Subject{}, errNoCaller
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return "", Subject{}, errNoCaller
	}
	var set *Set
	if c.roles != nil {
		set, _ = c.roles.Resolve(ctx)
	}
	sub, err := c.SubjectFor(ctx, o.ID, u.ID, u.EffectiveType(), ClientIP(ctx), set)
	return o.ID, sub, err
}

// ForCaller resolves key for the authenticated caller; on error the result
// carries the fail-closed value.
func (c *ControlService) ForCaller(ctx context.Context, key string) (ControlResult, error) {
	orgID, sub, err := c.callerSubject(ctx)
	if err != nil {
		def, ok := ControlByKey(key)
		if !ok {
			return ControlResult{}, fmt.Errorf("unknown control %q", key)
		}
		return ControlResult{Key: key, App: def.App, Value: restrictiveValue(def), Source: "error"}, err
	}
	return c.ResolveFor(ctx, orgID, sub, key)
}

var stdControls atomic.Pointer[ControlService]

// SetDefaultControls installs the process-wide ControlService used by Control.
func SetDefaultControls(c *ControlService) { stdControls.Store(c) }

// DefaultControls returns the installed ControlService (nil without a database).
func DefaultControls() *ControlService { return stdControls.Load() }

// Control resolves a control for the caller in ctx. Without a ControlService
// (no database) the catalog default for the caller's user type applies. Unknown
// keys panic in tests and resolve to false in production via ok=false.
func Control(ctx context.Context, key string) ControlResult {
	def, ok := ControlByKey(key)
	if !ok {
		return ControlResult{Key: key, Value: false, Source: "error"}
	}
	if c := stdControls.Load(); c != nil {
		r, _ := c.ForCaller(ctx, key)
		return r
	}
	return ControlResult{Key: key, App: def.App, Value: def.DefaultFor(UserType(ctx)), Source: "default"}
}

// ControlBool is shorthand for a bool control.
func ControlBool(ctx context.Context, key string) bool { return Control(ctx, key).Bool() }

// ControlEnum is shorthand for an enum control.
func ControlEnum(ctx context.Context, key string) string { return Control(ctx, key).String() }

// ControlNumber is shorthand for a number control.
func ControlNumber(ctx context.Context, key string) float64 { return Control(ctx, key).Number() }
