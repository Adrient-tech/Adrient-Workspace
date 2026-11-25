package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// ControlsHTTP serves the controls engine:
//
//	GET    /api/v1/me/controls                  effective controls of the caller (any member)
//	GET    /api/v1/admin/controls               catalog + rules
//	POST   /api/v1/admin/controls/rules         create a rule
//	PUT    /api/v1/admin/controls/rules/{id}    replace a rule
//	DELETE /api/v1/admin/controls/rules/{id}    delete a rule
//	GET    /api/v1/admin/access/effective       effective access viewer (user_id, app, ip, user_type)
type ControlsHTTP struct {
	Pool     *pgxpool.Pool
	Roles    *Service
	Apps     *AppService
	Controls *ControlService
	Audit    AuditFunc
	// OrgPolicy explains the organization-wide external collaboration settings
	// for one person (who can invite, partner domains, trusts). nil = omitted.
	OrgPolicy func(ctx context.Context, orgID, userID, userType string, admin bool) []map[string]any
}

const (
	pMeControls   = "/api/v1/me/controls"
	pControls     = "/api/v1/admin/controls"
	pControlRules = "/api/v1/admin/controls/rules"
	pEffective    = "/api/v1/admin/access/effective"
)

// Match reports whether path belongs to this handler.
func (h *ControlsHTTP) Match(path string) bool {
	path = strings.TrimRight(path, "/")
	return path == pMeControls || path == pControls || strings.HasPrefix(path, pControlRules) || path == pEffective
}

func (h *ControlsHTTP) audit(r *http.Request, action, rid string, detail map[string]any) {
	if h.Audit != nil {
		h.Audit(r.Context(), "app-controls", action, "control-rule", rid, detail, r)
	}
}

func (h *ControlsHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	o, ok2 := auth.OrgFromContext(r.Context())
	if !ok || !ok2 {
		jerr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.TrimRight(r.URL.Path, "/")
	if path == pMeControls {
		h.me(w, r)
		return
	}
	if !IsAdmin(r.Context()) {
		WriteForbidden(w, PermOrgAdmin)
		return
	}
	switch {
	case path == pControls && r.Method == http.MethodGet:
		h.list(w, r, o.ID)
	case path == pControlRules && r.Method == http.MethodPost:
		h.save(w, r, o.ID, u.ID, "")
	case strings.HasPrefix(path, pControlRules+"/") && r.Method == http.MethodPut:
		h.save(w, r, o.ID, u.ID, strings.TrimPrefix(path, pControlRules+"/"))
	case strings.HasPrefix(path, pControlRules+"/") && r.Method == http.MethodDelete:
		h.remove(w, r, o.ID, strings.TrimPrefix(path, pControlRules+"/"))
	case path == pEffective && r.Method == http.MethodGet:
		h.effective(w, r, o.ID)
	default:
		jerr(w, http.StatusNotFound, "not found")
	}
}

func (h *ControlsHTTP) me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jerr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	orgID, sub, err := h.Controls.callerSubject(r.Context())
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	rs, err := h.Controls.ResolveAllFor(r.Context(), orgID, sub)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	vals := map[string]any{}
	for _, c := range rs {
		vals[c.Key] = c.Value
	}
	writeJSON(w, http.StatusOK, map[string]any{"controls": vals, "user_type": sub.UserType})
}

func (h *ControlsHTTP) list(w http.ResponseWriter, r *http.Request, orgID string) {
	rules, err := h.Controls.orgRules(r.Context(), orgID)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "query failed")
		return
	}
	if rules == nil {
		rules = []ControlRule{}
	}
	names := h.subjectNames(r.Context(), orgID, rules)
	type item struct {
		ControlRule
		SubjectName string `json:"subject_name"`
	}
	items := make([]item, 0, len(rules))
	for _, ru := range rules {
		items = append(items, item{ControlRule: ru, SubjectName: names[ru.SubjectType+"/"+ru.SubjectID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"catalog": Controls, "rules": items, "user_types": []string{TypeMember, TypeExternal, TypeGuest, TypeService}})
}

func (h *ControlsHTTP) subjectNames(ctx context.Context, orgID string, rules []ControlRule) map[string]string {
	out := map[string]string{}
	for _, ru := range rules {
		k := ru.SubjectType + "/" + ru.SubjectID
		if _, done := out[k]; done {
			continue
		}
		var n string
		switch ru.SubjectType {
		case SubjectUser:
			_ = h.Pool.QueryRow(ctx, `SELECT COALESCE(NULLIF(display_name,''), email) FROM adrient.users WHERE id::text=$1 AND org_id=$2`, ru.SubjectID, orgID).Scan(&n)
		case SubjectGroup:
			_ = h.Pool.QueryRow(ctx, `SELECT name FROM adrient.groups WHERE id::text=$1 AND org_id=$2`, ru.SubjectID, orgID).Scan(&n)
		case SubjectDepartment:
			_ = h.Pool.QueryRow(ctx, `SELECT name FROM adrient.departments WHERE id::text=$1 AND org_id=$2`, ru.SubjectID, orgID).Scan(&n)
		case SubjectUserType, SubjectRole:
			n = ru.SubjectID
		case SubjectEveryone:
			n = "Everyone"
		}
		out[k] = n
	}
	return out
}

func (h *ControlsHTTP) save(w http.ResponseWriter, r *http.Request, orgID, actorID, id string) {
	var in struct {
		Control     string     `json:"control"`
		Value       any        `json:"value"`
		SubjectType string     `json:"subject_type"`
		SubjectID   string     `json:"subject_id"`
		Condition   *Condition `json:"condition"`
		Priority    *int       `json:"priority"`
		EffectMode  string     `json:"effect_mode"`
		Note        string     `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		jerr(w, http.StatusBadRequest, "invalid json")
		return
	}
	def, ok := ControlByKey(in.Control)
	if !ok {
		jerr(w, http.StatusBadRequest, "unknown control")
		return
	}
	val, err := def.Normalize(in.Value)
	if err != nil {
		jerr(w, http.StatusBadRequest, err.Error())
		return
	}
	switch in.SubjectType {
	case SubjectEveryone:
		in.SubjectID = ""
	case SubjectUser, SubjectGroup, SubjectDepartment, SubjectRole:
		if in.SubjectID == "" {
			jerr(w, http.StatusBadRequest, "subject_id required")
			return
		}
	case SubjectUserType:
		if !validUserType(in.SubjectID) {
			jerr(w, http.StatusBadRequest, "invalid user type")
			return
		}
	default:
		jerr(w, http.StatusBadRequest, "invalid subject_type")
		return
	}
	if err := in.Condition.Validate(); err != nil {
		jerr(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.EffectMode == "" {
		in.EffectMode = ModeSet
	}
	if in.EffectMode != ModeSet && in.EffectMode != ModeRestrictOnly {
		jerr(w, http.StatusBadRequest, "effect_mode must be set or restrict_only")
		return
	}
	prio := 100
	if in.Priority != nil {
		prio = *in.Priority
	}
	vb, _ := json.Marshal(val)
	var cond any
	if !in.Condition.Empty() {
		b, _ := json.Marshal(in.Condition)
		cond = string(b)
	}
	var rid string
	if id == "" {
		err = h.Pool.QueryRow(r.Context(), `INSERT INTO adrient.app_control_rules
(org_id, app_id, control, value, subject_type, subject_id, condition, priority, effect_mode, note, created_by)
VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7::jsonb,$8,$9,$10,$11::uuid) RETURNING id::text`,
			orgID, def.App, def.Key, string(vb), in.SubjectType, in.SubjectID, cond, prio, in.EffectMode, in.Note, actorID).Scan(&rid)
	} else {
		err = h.Pool.QueryRow(r.Context(), `UPDATE adrient.app_control_rules SET app_id=$3, control=$4, value=$5::jsonb,
subject_type=$6, subject_id=$7, condition=$8::jsonb, priority=$9, effect_mode=$10, note=$11
WHERE id::text=$1 AND org_id=$2 RETURNING id::text`,
			id, orgID, def.App, def.Key, string(vb), in.SubjectType, in.SubjectID, cond, prio, in.EffectMode, in.Note).Scan(&rid)
	}
	if err != nil {
		jerr(w, http.StatusBadRequest, "could not save rule")
		return
	}
	h.Controls.Invalidate()
	h.audit(r, map[bool]string{true: "create", false: "update"}[id == ""], rid, map[string]any{
		"control": def.Key, "value": val, "subject_type": in.SubjectType, "subject_id": in.SubjectID, "mode": in.EffectMode, "condition": in.Condition})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": rid})
}

func (h *ControlsHTTP) remove(w http.ResponseWriter, r *http.Request, orgID, id string) {
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM adrient.app_control_rules WHERE id::text=$1 AND org_id=$2`, id, orgID)
	if err != nil || tag.RowsAffected() == 0 {
		jerr(w, http.StatusNotFound, "rule not found")
		return
	}
	h.Controls.Invalidate()
	h.audit(r, "delete", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// AppAccess is the effective access of one app for one user.
type AppAccess struct {
	AppID    string          `json:"id"`
	Allowed  bool            `json:"allowed"`
	Reason   string          `json:"reason"`
	Policies []Policy        `json:"policies,omitempty"`
	Controls []ControlResult `json:"controls"`
}

// effective is the effective access viewer. It calls the same resolvers that
// enforcement uses, so the answer cannot drift from real behavior.
func (h *ControlsHTTP) effective(w http.ResponseWriter, r *http.Request, orgID string) {
	uid := r.URL.Query().Get("user_id")
	var id, email, name, utype, status string
	var guestApps []string
	err := h.Pool.QueryRow(r.Context(), `SELECT id::text, email, COALESCE(display_name,''), user_type, status, guest_apps
FROM adrient.users WHERE id::text=$1 AND org_id=$2`, uid, orgID).Scan(&id, &email, &name, &utype, &status, &guestApps)
	if err != nil {
		jerr(w, http.StatusNotFound, "user not found")
		return
	}
	sim := utype
	if v := r.URL.Query().Get("user_type"); v != "" {
		if !validUserType(v) {
			jerr(w, http.StatusBadRequest, "invalid user_type")
			return
		}
		sim = v
	}
	ip := r.URL.Query().Get("ip")
	set, err := h.Roles.ResolveUser(r.Context(), orgID, id)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	if sim == TypeGuest {
		set = NewSet([]Assignment{{RoleKey: RoleGuest}})
	}
	sub, err := h.Controls.SubjectFor(r.Context(), orgID, id, sim, ip, set)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	pol, err := h.Apps.orgPolicies(r.Context(), orgID)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	results, err := h.Controls.ResolveAllFor(r.Context(), orgID, sub)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	only := r.URL.Query().Get("app")
	byApp := map[string][]ControlResult{}
	var shared []ControlResult
	for _, c := range results {
		if c.App == "*" {
			shared = append(shared, c)
			continue
		}
		byApp[c.App] = append(byApp[c.App], c)
	}
	apps := []AppAccess{}
	for _, appID := range GatedApps {
		if only != "" && only != appID {
			continue
		}
		var d Decision
		if sim == TypeGuest {
			ok := contains(guestApps, appID)
			why := "guest: not on invitation"
			if ok {
				why = "guest: on invitation"
			}
			if ok && deniedBy(pol, appID, sub) {
				ok, why = false, "guest: denied by a rule"
			}
			d = Decision{AppID: appID, Allowed: ok, Reason: why}
		} else {
			d = Resolve(pol, appID, sub)
		}
		var matched []Policy
		for _, p := range pol {
			if p.AppID == appID && p.matches(sub) {
				matched = append(matched, p)
			}
		}
		apps = append(apps, AppAccess{AppID: appID, Allowed: d.Allowed, Reason: d.Reason, Policies: matched, Controls: nonNilCR(byApp[appID])})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":       map[string]any{"id": id, "email": email, "name": name, "user_type": utype, "status": status},
		"simulated":  map[string]any{"user_type": sim, "ip": ip},
		"roles":      nonNil(set.Roles),
		"apps":       apps,
		"shared":     nonNilCR(shared),
		"org_policy": h.orgPolicy(r.Context(), orgID, id, sim, set),
	})
}

func nonNilCR(l []ControlResult) []ControlResult {
	if l == nil {
		return []ControlResult{}
	}
	return l
}

func (h *ControlsHTTP) orgPolicy(ctx context.Context, orgID, uid, utype string, set *Set) []map[string]any {
	if h.OrgPolicy == nil {
		return []map[string]any{}
	}
	return h.OrgPolicy(ctx, orgID, uid, utype, set != nil && set.Can(PermOrgAdmin))
}
