package authz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// AuditFunc records an administrative change (service e.g. "roles" or "app-access").
type AuditFunc func(ctx context.Context, service, action, resourceType, resourceID string, detail map[string]any, r *http.Request)

// AdminHTTP serves the role assignment and app access admin APIs plus
// GET /api/v1/me/apps.
//
//	GET    /api/v1/me/apps                              caller's resolved app list (any member)
//	GET    /api/v1/admin/roles                          roles + assignments (admin)
//	POST   /api/v1/admin/roles/assignments             {subject_type,subject_id,role_key,scope_app}
//	DELETE /api/v1/admin/roles/assignments/{id}
//	GET    /api/v1/admin/app-access                     policies per app + known apps
//	PUT    /api/v1/admin/app-access/{app}               {policies:[{subject_type,subject_id,effect}]} replaces the rows
//	GET    /api/v1/admin/app-access/preview?user_id=    that user's resolved app list
//	GET    /api/v1/admin/subjects?type=&q=              user|group|department|role picker
type AdminHTTP struct {
	Pool  *pgxpool.Pool
	Roles *Service
	Apps  *AppService
	Audit AuditFunc
}

const (
	pMeApps     = "/api/v1/me/apps"
	pRoles      = "/api/v1/admin/roles"
	pAppAccess  = "/api/v1/admin/app-access"
	pSubjects   = "/api/v1/admin/subjects"
	assignSeg   = "/assignments"
	previewPath = pAppAccess + "/preview"
)

// Match reports whether path belongs to this handler.
func (h *AdminHTTP) Match(path string) bool {
	path = strings.TrimRight(path, "/")
	return path == pMeApps || path == pRoles || strings.HasPrefix(path, pRoles+"/") ||
		path == pAppAccess || strings.HasPrefix(path, pAppAccess+"/") || path == pSubjects
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jerr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func (h *AdminHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	o, ok2 := auth.OrgFromContext(r.Context())
	if !ok || !ok2 {
		jerr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.TrimRight(r.URL.Path, "/")
	if path == pMeApps {
		h.meApps(w, r, o.ID, u.ID)
		return
	}
	if !IsAdmin(r.Context()) {
		WriteForbidden(w, PermOrgAdmin)
		return
	}
	switch {
	case path == pRoles && r.Method == http.MethodGet:
		h.listRoles(w, r, o.ID)
	case path == pRoles+assignSeg && r.Method == http.MethodPost:
		h.assign(w, r, o.ID, u.ID)
	case strings.HasPrefix(path, pRoles+assignSeg+"/") && r.Method == http.MethodDelete:
		h.unassign(w, r, o.ID, strings.TrimPrefix(path, pRoles+assignSeg+"/"))
	case path == pAppAccess && r.Method == http.MethodGet:
		h.listPolicies(w, r, o.ID)
	case path == pAppAccess+"/presets" && r.Method == http.MethodGet:
		h.listPresets(w)
	case strings.HasPrefix(path, pAppAccess+"/presets/") && r.Method == http.MethodPost:
		if k, ok := presetKeyFromPath(path); ok {
			h.applyPreset(w, r, o.ID, u.ID, k)
		} else {
			jerr(w, http.StatusNotFound, "not found")
		}
	case path == previewPath && r.Method == http.MethodGet:
		h.preview(w, r, o.ID)
	case strings.HasPrefix(path, pAppAccess+"/") && r.Method == http.MethodPut:
		h.putPolicies(w, r, o.ID, u.ID, strings.TrimPrefix(path, pAppAccess+"/"))
	case path == pSubjects && r.Method == http.MethodGet:
		h.subjects(w, r, o.ID)
	default:
		jerr(w, http.StatusNotFound, "not found")
	}
}

func (h *AdminHTTP) audit(r *http.Request, service, action, rtype, rid string, detail map[string]any) {
	if h.Audit != nil {
		h.Audit(r.Context(), service, action, rtype, rid, detail, r)
	}
}

func (h *AdminHTTP) invalidate() {
	if h.Roles != nil {
		h.Roles.Invalidate()
	}
	if h.Apps != nil {
		h.Apps.Invalidate()
	}
}

// ----- /me/apps ------------------------------------------------------------

func (h *AdminHTTP) meApps(w http.ResponseWriter, r *http.Request, orgID, userID string) {
	if r.Method != http.MethodGet {
		jerr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	set, err := h.Roles.Resolve(r.Context())
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	ds, err := h.Apps.ResolveAll(r.Context(), orgID, userID, set)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	if IsGuest(r.Context()) {
		// Guests never get the Admin tile (its APIs are permission-gated anyway).
		ds = append(ds, Decision{AppID: "admin", Allowed: false, Reason: "guest"})
	}
	body := decisionsBody(ds)
	body["user_type"] = UserType(r.Context())
	writeJSON(w, http.StatusOK, body)
}

func decisionsBody(ds []Decision) map[string]any {
	allowed, denied := []string{}, []string{}
	for _, d := range ds {
		if d.Allowed {
			allowed = append(allowed, d.AppID)
		} else {
			denied = append(denied, d.AppID)
		}
	}
	return map[string]any{"apps": ds, "allowed": allowed, "denied": denied}
}

// ----- roles ---------------------------------------------------------------

var assignableRoles = []struct{ Key, Name string }{
	{RoleOwner, "Owner"}, {RoleAdmin, "Admin"}, {RoleAppAdmin, "App admin"},
	{RoleManager, "Manager"}, {RoleGuest, "Guest"},
}

type assignmentRow struct {
	ID          string `json:"id"`
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
	SubjectName string `json:"subject_name"`
	SubjectSub  string `json:"subject_detail"`
	RoleKey     string `json:"role_key"`
	RoleName    string `json:"role_name"`
	ScopeApp    string `json:"scope_app"`
}

func (h *AdminHTTP) listRoles(w http.ResponseWriter, r *http.Request, orgID string) {
	rows, err := h.Pool.Query(r.Context(), `
SELECT ra.id::text, ra.subject_type, ra.subject_id::text,
       COALESCE(CASE ra.subject_type WHEN 'user' THEN NULLIF(u.display_name,'') ELSE g.name END, ''),
       COALESCE(CASE ra.subject_type WHEN 'user' THEN u.email ELSE '' END, ''),
       r.key, r.name, ra.scope_app
FROM adrient.role_assignments ra
JOIN adrient.roles r ON r.id = ra.role_id
LEFT JOIN adrient.users u ON ra.subject_type='user' AND u.id = ra.subject_id
LEFT JOIN adrient.groups g ON ra.subject_type='group' AND g.id = ra.subject_id
WHERE ra.org_id = $1
ORDER BY r.key, ra.scope_app, 4, ra.created_at`, orgID)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	out := []assignmentRow{}
	for rows.Next() {
		var a assignmentRow
		if err := rows.Scan(&a.ID, &a.SubjectType, &a.SubjectID, &a.SubjectName, &a.SubjectSub, &a.RoleKey, &a.RoleName, &a.ScopeApp); err != nil {
			jerr(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if a.SubjectName == "" {
			a.SubjectName = a.SubjectSub
		}
		out = append(out, a)
	}
	roles := []map[string]string{}
	for _, ar := range assignableRoles {
		roles = append(roles, map[string]string{"key": ar.Key, "name": ar.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"roles": roles, "assignments": out, "apps": GatedApps,
		"note": "Every member is implicitly a Member unless assigned Guest.",
	})
}

func (h *AdminHTTP) assign(w http.ResponseWriter, r *http.Request, orgID, actorID string) {
	var in struct {
		SubjectType string `json:"subject_type"`
		SubjectID   string `json:"subject_id"`
		RoleKey     string `json:"role_key"`
		ScopeApp    string `json:"scope_app"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		jerr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.SubjectType != SubjectUser && in.SubjectType != SubjectGroup {
		jerr(w, http.StatusBadRequest, "subject_type must be user or group")
		return
	}
	okRole := false
	for _, ar := range assignableRoles {
		okRole = okRole || ar.Key == in.RoleKey
	}
	if !okRole {
		jerr(w, http.StatusBadRequest, "unknown role")
		return
	}
	if in.RoleKey == RoleAppAdmin && !IsKnownApp(in.ScopeApp) {
		jerr(w, http.StatusBadRequest, "app_admin needs a valid scope_app")
		return
	}
	if in.RoleKey != RoleAppAdmin {
		in.ScopeApp = ""
	}
	if in.RoleKey == RoleOwner && !Can(r.Context(), PermOrgOwner) {
		WriteForbidden(w, PermOrgOwner)
		return
	}
	var exists bool
	q := `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id=$1::uuid AND org_id=$2)`
	if in.SubjectType == SubjectGroup {
		q = `SELECT EXISTS(SELECT 1 FROM adrient.groups WHERE id=$1::uuid AND org_id=$2)`
	}
	if err := h.Pool.QueryRow(r.Context(), q, in.SubjectID, orgID).Scan(&exists); err != nil || !exists {
		jerr(w, http.StatusBadRequest, "subject not found in this organization")
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `
INSERT INTO adrient.role_assignments (org_id, subject_type, subject_id, role_id, scope_app, created_by)
SELECT $1, $2, $3::uuid, r.id, $5, $6::uuid FROM adrient.roles r WHERE r.key=$4 AND r.org_id IS NULL
ON CONFLICT (org_id, subject_type, subject_id, role_id, scope_app) DO UPDATE SET scope_app = EXCLUDED.scope_app
RETURNING id::text`, orgID, in.SubjectType, in.SubjectID, in.RoleKey, in.ScopeApp, actorID).Scan(&id)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "assign failed")
		return
	}
	h.invalidate()
	h.audit(r, "roles", "assign", "role_assignment", id, map[string]any{
		"subject_type": in.SubjectType, "subject_id": in.SubjectID, "role": in.RoleKey, "scope_app": in.ScopeApp,
	})
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *AdminHTTP) unassign(w http.ResponseWriter, r *http.Request, orgID, id string) {
	var key, subjType, subjID, scope string
	err := h.Pool.QueryRow(r.Context(), `
SELECT r.key, ra.subject_type, ra.subject_id::text, ra.scope_app
FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id
WHERE ra.id=$1::uuid AND ra.org_id=$2`, id, orgID).Scan(&key, &subjType, &subjID, &scope)
	if errors.Is(err, pgx.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "invalid input syntax")) {
		jerr(w, http.StatusNotFound, "assignment not found")
		return
	}
	if err != nil {
		jerr(w, http.StatusInternalServerError, "query failed")
		return
	}
	if key == RoleOwner {
		if !Can(r.Context(), PermOrgOwner) {
			WriteForbidden(w, PermOrgOwner)
			return
		}
		var n int
		_ = h.Pool.QueryRow(r.Context(), `SELECT count(*) FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id
WHERE ra.org_id=$1 AND r.key='owner'`, orgID).Scan(&n)
		if n <= 1 {
			jerr(w, http.StatusConflict, "an organization needs at least one Owner")
			return
		}
	}
	if _, err := h.Pool.Exec(r.Context(), `DELETE FROM adrient.role_assignments WHERE id=$1::uuid AND org_id=$2`, id, orgID); err != nil {
		jerr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	h.invalidate()
	h.audit(r, "roles", "revoke", "role_assignment", id, map[string]any{
		"subject_type": subjType, "subject_id": subjID, "role": key, "scope_app": scope,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----- app access ----------------------------------------------------------

func (h *AdminHTTP) listPolicies(w http.ResponseWriter, r *http.Request, orgID string) {
	pol, err := h.Apps.store.Policies(r.Context(), orgID)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "query failed")
		return
	}
	// Resolve display names for the chips.
	names := h.subjectNames(r.Context(), orgID, pol)
	type item struct {
		Policy
		Name string `json:"subject_name"`
	}
	items := []item{}
	for _, p := range pol {
		items = append(items, item{Policy: p, Name: names[p.SubjectType+"/"+p.SubjectID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"policies": items, "apps": GatedApps})
}

func (h *AdminHTTP) subjectNames(ctx context.Context, orgID string, pol []Policy) map[string]string {
	out := map[string]string{}
	var users, groups, depts []string
	for _, p := range pol {
		switch p.SubjectType {
		case SubjectUser:
			users = append(users, p.SubjectID)
		case SubjectGroup:
			groups = append(groups, p.SubjectID)
		case SubjectDepartment:
			depts = append(depts, p.SubjectID)
		case SubjectRole:
			out["role/"+p.SubjectID] = roleName(p.SubjectID)
		case SubjectEveryone:
			out["everyone/"] = "Everyone"
		}
	}
	fill := func(kind, q string, ids []string) {
		if len(ids) == 0 {
			return
		}
		rows, err := h.Pool.Query(ctx, q, orgID, ids)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id, n string
			if rows.Scan(&id, &n) == nil {
				out[kind+"/"+id] = n
			}
		}
	}
	fill("user", `SELECT id::text, COALESCE(NULLIF(display_name,''), email) FROM adrient.users WHERE org_id=$1 AND id::text = ANY($2)`, users)
	fill("group", `SELECT id::text, name FROM adrient.groups WHERE org_id=$1 AND id::text = ANY($2)`, groups)
	fill("department", `SELECT id::text, name FROM adrient.departments WHERE org_id=$1 AND id::text = ANY($2)`, depts)
	return out
}

func roleName(key string) string {
	for _, ar := range assignableRoles {
		if ar.Key == key {
			return ar.Name
		}
	}
	if key == RoleMember {
		return "Member"
	}
	return key
}

func (h *AdminHTTP) putPolicies(w http.ResponseWriter, r *http.Request, orgID, actorID, app string) {
	if !IsKnownApp(app) {
		jerr(w, http.StatusNotFound, "unknown app")
		return
	}
	var in struct {
		Policies []struct {
			SubjectType string     `json:"subject_type"`
			SubjectID   string     `json:"subject_id"`
			Effect      string     `json:"effect"`
			Condition   *Condition `json:"condition"`
		} `json:"policies"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&in); err != nil {
		jerr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(in.Policies) > 500 {
		jerr(w, http.StatusBadRequest, "too many policies")
		return
	}
	for _, p := range in.Policies {
		if p.Effect != EffectAllow && p.Effect != EffectDeny {
			jerr(w, http.StatusBadRequest, "effect must be allow or deny")
			return
		}
		switch p.SubjectType {
		case SubjectEveryone:
		case SubjectUser, SubjectGroup, SubjectDepartment, SubjectRole, SubjectUserType:
			if p.SubjectID == "" {
				jerr(w, http.StatusBadRequest, "subject_id required")
				return
			}
			if p.SubjectType == SubjectUserType && !validUserType(p.SubjectID) {
				jerr(w, http.StatusBadRequest, "invalid user type")
				return
			}
		default:
			jerr(w, http.StatusBadRequest, "invalid subject_type")
			return
		}
		if err := p.Condition.Validate(); err != nil {
			jerr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		jerr(w, http.StatusInternalServerError, "tx failed")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `DELETE FROM adrient.app_access_policies WHERE org_id=$1 AND service_id=$2`, orgID, app); err != nil {
		jerr(w, http.StatusInternalServerError, "update failed")
		return
	}
	for _, p := range in.Policies {
		sid := p.SubjectID
		if p.SubjectType == SubjectEveryone {
			sid = ""
		}
		var cond any
		if !p.Condition.Empty() {
			b, _ := json.Marshal(p.Condition)
			cond = string(b)
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO adrient.app_access_policies (org_id, service_id, subject_type, subject_id, effect, created_by, condition)
VALUES ($1,$2,$3,$4,$5,$6::uuid,$7::jsonb) ON CONFLICT DO NOTHING`, orgID, app, p.SubjectType, sid, p.Effect, actorID, cond); err != nil {
			jerr(w, http.StatusBadRequest, "invalid policy")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		jerr(w, http.StatusInternalServerError, "commit failed")
		return
	}
	h.invalidate()
	h.audit(r, "app-access", "update", "app", app, map[string]any{"policies": in.Policies, "count": len(in.Policies)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *AdminHTTP) preview(w http.ResponseWriter, r *http.Request, orgID string) {
	uid := r.URL.Query().Get("user_id")
	var exists bool
	if err := h.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id::text=$1 AND org_id=$2)`, uid, orgID).Scan(&exists); err != nil || !exists {
		jerr(w, http.StatusNotFound, "user not found")
		return
	}
	set, err := h.Roles.ResolveUser(r.Context(), orgID, uid)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	ds, err := h.Apps.ResolveAll(r.Context(), orgID, uid, set)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "authz error")
		return
	}
	b := decisionsBody(ds)
	b["roles"] = nonNil(set.Roles)
	writeJSON(w, http.StatusOK, b)
}

// subjects powers the chip pickers.
func (h *AdminHTTP) subjects(w http.ResponseWriter, r *http.Request, orgID string) {
	typ := r.URL.Query().Get("type")
	q := "%" + strings.ReplaceAll(strings.ReplaceAll(r.URL.Query().Get("q"), "%", ""), "_", "") + "%"
	type subj struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Detail string `json:"detail,omitempty"`
	}
	out := []subj{}
	var sql string
	switch typ {
	case SubjectUser:
		sql = `SELECT id::text, COALESCE(NULLIF(display_name,''), email), email FROM adrient.users
WHERE org_id=$1 AND (display_name ILIKE $2 OR email ILIKE $2) ORDER BY 2 LIMIT 30`
	case SubjectGroup:
		sql = `SELECT id::text, name, '' FROM adrient.groups WHERE org_id=$1 AND name ILIKE $2 ORDER BY 2 LIMIT 30`
	case SubjectDepartment:
		sql = `SELECT id::text, name, '' FROM adrient.departments WHERE org_id=$1 AND name ILIKE $2 ORDER BY 2 LIMIT 50`
	case SubjectUserType:
		for _, t := range []string{TypeMember, TypeExternal, TypeGuest, TypeService} {
			out = append(out, subj{ID: t, Name: strings.ToUpper(t[:1]) + t[1:]})
		}
		writeJSON(w, http.StatusOK, map[string]any{"subjects": out})
		return
	case SubjectRole:
		for _, ar := range assignableRoles {
			out = append(out, subj{ID: ar.Key, Name: ar.Name})
		}
		out = append(out, subj{ID: RoleMember, Name: "Member"})
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		writeJSON(w, http.StatusOK, map[string]any{"subjects": out})
		return
	default:
		jerr(w, http.StatusBadRequest, "type must be user, group, department, role or user_type")
		return
	}
	rows, err := h.Pool.Query(r.Context(), sql, orgID, q)
	if err != nil {
		jerr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var s subj
		if rows.Scan(&s.ID, &s.Name, &s.Detail) == nil {
			out = append(out, s)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"subjects": out})
}

func validUserType(t string) bool {
	switch t {
	case TypeMember, TypeExternal, TypeGuest, TypeService:
		return true
	}
	return false
}
