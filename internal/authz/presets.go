package authz

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Preset is a named bundle of apps for a job role. Applying it to a group adds
// an allow rule for that group on each app. Important: once an app has any
// allow rule, only the people those rules name can open it.
type Preset struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Apps        []string `json:"apps"`
	// AppAdmin lists apps where the group also gets the App admin role.
	AppAdmin []string `json:"app_admin,omitempty"`
}

// Presets are the built-in job roles for the business apps.
var Presets = []Preset{
	{Key: "hr_manager", Name: "HR manager", Description: "Runs people operations: employees, leave, attendance, payroll, hiring and onboarding.",
		Apps: []string{"hr", "talent", "contacts", "calendar", "tasks", "projects"}, AppAdmin: []string{"hr", "talent"}},
	{Key: "recruiter", Name: "Recruiter", Description: "Hiring pipeline, interviews and offers. No payroll or employee records.",
		Apps: []string{"talent", "contacts", "calendar", "meet", "mail"}},
	{Key: "accountant", Name: "Accountant", Description: "Books, expenses, bank reconciliation and invoices.",
		Apps: []string{"accounting", "invoicing", "contacts", "drive", "sheets"}, AppAdmin: []string{"accounting"}},
	{Key: "sales_rep", Name: "Sales rep", Description: "CRM pipeline, quotes and invoices, calendar and mail.",
		Apps: []string{"crm", "invoicing", "contacts", "calendar", "mail", "tickets"}},
	{Key: "store_manager", Name: "Store manager", Description: "Stock, purchasing, the till and sales documents.",
		Apps: []string{"inventory", "pos", "invoicing", "contacts"}, AppAdmin: []string{"inventory", "pos"}},
	{Key: "cashier", Name: "Cashier", Description: "Rings up sales at the till and nothing else in the business apps.",
		Apps: []string{"pos"}},
	{Key: "legal_counsel", Name: "Legal counsel", Description: "Matters, deadlines, billing and conflict checks, with contracts and e-signature.",
		Apps: []string{"legal", "contracts", "signatures", "contacts", "calendar", "tasks", "invoicing"}, AppAdmin: []string{"legal"}},
	{Key: "equity_manager", Name: "Equity manager (cap table)", Description: "Opens the cap table: shareholders, options, rounds and exit models. Also add each person under Cap table > Access; the cap table shows nothing to anyone not named there.",
		Apps: []string{"captable", "accounting", "contracts", "signatures"}, AppAdmin: []string{"captable"}},
	{Key: "purchasing_officer", Name: "Purchasing officer", Description: "Purchase orders, receiving and supplier bills.",
		Apps: []string{"inventory", "accounting", "contacts"}},
}

func presetByKey(k string) (Preset, bool) {
	for _, p := range Presets {
		if p.Key == k {
			return p, true
		}
	}
	return Preset{}, false
}

func (h *AdminHTTP) listPresets(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"presets": Presets,
		"note": "Applying a preset adds an allow rule for the chosen group on each app. An app with allow rules can only be opened by the people they name, so apply the matching preset to every group that needs the app."})
}

// applyPreset adds allow rules for a group (or user) on each app of the preset,
// keeping every existing rule. It returns which apps became restricted.
func (h *AdminHTTP) applyPreset(w http.ResponseWriter, r *http.Request, orgID, actorID, key string) {
	p, ok := presetByKey(key)
	if !ok {
		jerr(w, http.StatusNotFound, "unknown preset")
		return
	}
	var in struct {
		SubjectType string `json:"subject_type"`
		SubjectID   string `json:"subject_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&in); err != nil || in.SubjectID == "" {
		jerr(w, http.StatusBadRequest, "subject_type and subject_id are required")
		return
	}
	if in.SubjectType != SubjectGroup && in.SubjectType != SubjectUser {
		jerr(w, http.StatusBadRequest, "subject_type must be group or user")
		return
	}
	q := `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id::text=$1 AND org_id=$2)`
	if in.SubjectType == SubjectGroup {
		q = `SELECT EXISTS(SELECT 1 FROM adrient.groups WHERE id::text=$1 AND org_id=$2)`
	}
	var exists bool
	if err := h.Pool.QueryRow(r.Context(), q, in.SubjectID, orgID).Scan(&exists); err != nil || !exists {
		jerr(w, http.StatusBadRequest, "subject not found in this organization")
		return
	}
	restricted := []string{}
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		jerr(w, http.StatusInternalServerError, "tx failed")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck
	for _, app := range p.Apps {
		var had bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM adrient.app_access_policies WHERE org_id=$1 AND service_id=$2 AND effect='allow')`, orgID, app).Scan(&had); err != nil {
			jerr(w, http.StatusInternalServerError, "update failed")
			return
		}
		// Only the business apps are restricted by a preset; shared apps (mail,
		// drive, calendar ...) stay open to everyone, so no rule is added there.
		if !BusinessApps[app] {
			continue
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO adrient.app_access_policies (org_id, service_id, subject_type, subject_id, effect, created_by)
			VALUES ($1,$2,$3,$4,'allow',$5::uuid) ON CONFLICT DO NOTHING`, orgID, app, in.SubjectType, in.SubjectID, actorID); err != nil {
			jerr(w, http.StatusInternalServerError, "update failed")
			return
		}
		if !had {
			restricted = append(restricted, app)
		}
	}
	for _, app := range p.AppAdmin {
		if _, err := tx.Exec(r.Context(), `INSERT INTO adrient.role_assignments (org_id, subject_type, subject_id, role_id, scope_app, created_by)
			SELECT $1, $2, $3::uuid, r.id, $5, $6::uuid FROM adrient.roles r WHERE r.key=$4 AND r.org_id IS NULL
			ON CONFLICT (org_id, subject_type, subject_id, role_id, scope_app) DO NOTHING`, orgID, in.SubjectType, in.SubjectID, RoleAppAdmin, app, actorID); err != nil {
			jerr(w, http.StatusInternalServerError, "update failed")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		jerr(w, http.StatusInternalServerError, "commit failed")
		return
	}
	h.invalidate()
	h.audit(r, "app-access", "apply_preset", "preset", key, map[string]any{"subject_type": in.SubjectType, "subject_id": in.SubjectID, "apps": p.Apps})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied": p.Apps, "newly_restricted": restricted})
}

func presetKeyFromPath(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, pAppAccess+"/presets/")
	if !ok {
		return "", false
	}
	k, tail, _ := strings.Cut(rest, "/")
	return k, tail == "apply"
}
