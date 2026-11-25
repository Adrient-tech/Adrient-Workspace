package iamops

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelmgmt"
)

// AfterRecord is called after the sign-in history row has been written by the
// login recorder: it adds country, device and MFA fields to that row and runs the rules.
func (s *Service) AfterRecord(ctx context.Context, ev auth.LoginEvent, country, city string) {
	orgID := ev.OrgID
	var id int64
	var userID, email string
	err := s.Pool.QueryRow(ctx, `SELECT id, COALESCE(org_id::text,''), COALESCE(user_id::text,''), email FROM adrient.login_events
WHERE lower(email)=lower($1) AND ip=$2 AND created_at > now() - interval '30 seconds' AND source='app' AND device_fp='' ORDER BY id DESC LIMIT 1`, ev.Email, ev.IP).Scan(&id, &orgID, &userID, &email)
	if err != nil {
		return
	}
	if orgID == "" {
		// An attempt on an address nobody owns belongs to the default organisation, so
		// credential-stuffing and per-address rules still see it.
		if s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs ORDER BY created_at LIMIT 1`).Scan(&orgID) != nil || orgID == "" {
			return
		}
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.login_events SET org_id=$2::uuid WHERE id=$1`, id, orgID)
	}
	fp := DeviceFingerprint(ev.UserAgent)
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.login_events SET country=$2, city=$3, device_fp=$4 WHERE id=$1`, id, strings.ToUpper(country), city, fp)
	e := Event{ID: id, OrgID: orgID, UserID: userID, Email: email, IP: ev.IP, UserAgent: ev.UserAgent, Method: ev.Method, Success: ev.Success,
		Reason: ev.Reason, Country: country, City: city, DeviceFP: fp, At: s.now()}
	s.evaluate(ctx, orgID, e)
}

// ---- people list and per-user security ------------------------------------------------

// Person is one row of the people table with security columns.
type Person struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Type        string     `json:"type"`
	IsAdmin     bool       `json:"is_admin"`
	Quarantined bool       `json:"quarantined"`
	LastSignIn  *time.Time `json:"last_sign_in"`
	OpenAlerts  int        `json:"open_alerts"`
	Sessions    int        `json:"sessions"`
}

// ListPeople pages through the org's people by e-mail (keyset cursor = last e-mail).
func (s *Service) ListPeople(ctx context.Context, orgID, q, cursor string, limit int) ([]Person, string, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where := `u.org_id::text=$1 AND u.deleted_at IS NULL`
	args := []any{orgID}
	if q = strings.TrimSpace(q); q != "" {
		args = append(args, "%"+q+"%")
		where += fmt.Sprintf(` AND (u.email ILIKE $%d OR u.display_name ILIKE $%d)`, len(args), len(args))
	}
	var total int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users u WHERE `+where, args...).Scan(&total)
	if cursor != "" {
		args = append(args, cursor)
		where += fmt.Sprintf(` AND lower(u.email) > lower($%d)`, len(args))
	}
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `SELECT u.id::text, u.email, u.display_name, u.status, u.user_type,
 EXISTS (SELECT 1 FROM adrient.iam_restrictions r WHERE r.user_id=u.id AND r.kind='quarantine' AND (r.expires_at IS NULL OR r.expires_at>now())),
 (SELECT max(created_at) FROM adrient.login_events le WHERE le.user_id=u.id AND le.success),
 (SELECT count(*) FROM adrient.iam_alerts a WHERE a.user_id=u.id AND a.status IN ('open','investigating')),
 (SELECT count(*) FROM adrient.sessions se WHERE se.user_id=u.id AND se.revoked_at IS NULL AND se.expires_at>now())
FROM adrient.users u WHERE `+where+fmt.Sprintf(` ORDER BY lower(u.email) LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, "", 0, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Email, &p.Name, &p.Status, &p.Type, &p.Quarantined, &p.LastSignIn, &p.OpenAlerts, &p.Sessions); err != nil {
			return nil, "", 0, err
		}
		out = append(out, p)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].Email
	}
	if s.IsAdminUser != nil {
		for i := range out {
			out[i].IsAdmin = s.IsAdminUser(ctx, orgID, out[i].ID)
		}
	}
	return out, next, total, rows.Err()
}

// SessionOut is one active session.
type SessionOut struct {
	ID         string     `json:"id"`
	IP         string     `json:"ip"`
	Device     string     `json:"device"`
	Country    string     `json:"country"`
	City       string     `json:"city"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	Current    bool       `json:"current"`
}

// Sessions lists a person's active sessions. Geo comes from the matching sign-in event.
func (s *Service) Sessions(ctx context.Context, userID, currentKey string) []SessionOut {
	rows, err := s.Pool.Query(ctx, `SELECT COALESCE(se.public_id,''), COALESCE(se.ip,''), COALESCE(se.user_agent,''), se.created_at, se.last_seen_at, se.token,
 COALESCE((SELECT le.country FROM adrient.login_events le WHERE le.user_id=se.user_id AND le.ip=se.ip AND le.success ORDER BY le.id DESC LIMIT 1),''),
 COALESCE((SELECT le.city FROM adrient.login_events le WHERE le.user_id=se.user_id AND le.ip=se.ip AND le.success ORDER BY le.id DESC LIMIT 1),'')
FROM adrient.sessions se WHERE se.user_id::text=$1 AND se.revoked_at IS NULL AND se.expires_at > now() ORDER BY COALESCE(se.last_seen_at, se.created_at) DESC LIMIT 100`, userID)
	if err != nil {
		return []SessionOut{}
	}
	defer rows.Close()
	out := []SessionOut{}
	for rows.Next() {
		var x SessionOut
		var ua, tok string
		if rows.Scan(&x.ID, &x.IP, &ua, &x.CreatedAt, &x.LastSeenAt, &tok, &x.Country, &x.City) == nil {
			x.Device = DeviceLabel(ua)
			x.Current = currentKey != "" && SessionKey(tok) == currentKey
			out = append(out, x)
		}
	}
	return out
}

// TokenOut is an API token (never the secret).
type TokenOut struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Tokens lists a person's live API tokens.
func (s *Service) Tokens(ctx context.Context, userID string) []TokenOut {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, name, prefix, last_used_at, expires_at, created_at FROM adrient.api_tokens WHERE user_id::text=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		return []TokenOut{}
	}
	defer rows.Close()
	out := []TokenOut{}
	for rows.Next() {
		var x TokenOut
		if rows.Scan(&x.ID, &x.Name, &x.Prefix, &x.LastUsedAt, &x.ExpiresAt, &x.CreatedAt) == nil {
			out = append(out, x)
		}
	}
	return out
}

// SecurityView is everything the per-user Security tab shows.
type SecurityView struct {
	Person       Person                `json:"person"`
	Factors      []zitadelmgmt.Factor  `json:"factors"`
	FactorsErr   string                `json:"factors_error,omitempty"`
	Methods      []string              `json:"methods"`
	IdPLinks     []zitadelmgmt.IdPLink `json:"idp_links"`
	UserState    string                `json:"idp_state"`
	Sessions     []SessionOut          `json:"sessions"`
	Tokens       []TokenOut            `json:"tokens"`
	Events       []Event               `json:"events"`
	Alerts       []Alert               `json:"alerts"`
	Actions      []ActionRow           `json:"actions"`
	Quarantine   string                `json:"quarantine_reason,omitempty"`
	Expires      *time.Time            `json:"expires_at"`
	Reason       string                `json:"blocked_reason"`
	Capabilities []string              `json:"capabilities"`
	Target       map[string]bool       `json:"can"`
}

// UserSecurity assembles the Security tab for one person.
func (s *Service) UserSecurity(ctx context.Context, orgID string, a Actor, userID string) (SecurityView, error) {
	t, err := s.loadTarget(ctx, orgID, userID)
	if err != nil {
		return SecurityView{}, err
	}
	v := SecurityView{Factors: []zitadelmgmt.Factor{}, Methods: []string{}, IdPLinks: []zitadelmgmt.IdPLink{}}
	people, _, _, _ := s.ListPeople(ctx, orgID, t.Email, "", 5)
	for _, p := range people {
		if p.ID == t.ID {
			v.Person = p
		}
	}
	if v.Person.ID == "" {
		v.Person = Person{ID: t.ID, Email: t.Email, Name: t.Name, Status: t.Status, Type: t.Type}
	}
	_ = s.Pool.QueryRow(ctx, `SELECT expires_at, blocked_reason FROM adrient.users WHERE id::text=$1`, t.ID).Scan(&v.Expires, &v.Reason)
	_ = s.Pool.QueryRow(ctx, `SELECT reason FROM adrient.iam_restrictions WHERE user_id::text=$1 AND kind='quarantine'`, t.ID).Scan(&v.Quarantine)
	if s.IdP != nil && t.Subject != "" && !strings.HasPrefix(t.Subject, "adrient:") {
		var wg sync.WaitGroup
		wg.Add(4)
		go func() {
			defer wg.Done()
			if fs, err := s.IdP.ListFactors(ctx, t.Subject); err == nil {
				v.Factors = fs
			} else {
				v.FactorsErr = idpErr(err).Error()
			}
		}()
		go func() { defer wg.Done(); v.Methods, _ = s.IdP.AuthMethods(ctx, t.Subject) }()
		go func() {
			defer wg.Done()
			if l, err := s.IdP.ListIdPLinks(ctx, t.Subject); err == nil {
				v.IdPLinks = l
			}
		}()
		go func() { defer wg.Done(); v.UserState, _ = s.IdP.UserState(ctx, t.Subject) }()
		wg.Wait()
		if v.Methods == nil {
			v.Methods = []string{}
		}
	} else {
		v.FactorsErr = "This account is not linked to the identity provider."
	}
	v.Sessions = s.Sessions(ctx, t.ID, a.SessionKey)
	v.Tokens = s.Tokens(ctx, t.ID)
	v.Events, _, _ = s.ListEvents(ctx, orgID, EventFilter{UserID: t.ID, Limit: 25})
	v.Alerts, _, _, _ = s.ListAlerts(ctx, orgID, AlertFilter{UserID: t.ID, Limit: 10})
	v.Actions, _, _ = s.ListActions(ctx, orgID, t.ID, "", 15)
	v.Capabilities = Caps(a.Roles)
	targetAdmin := v.Person.IsAdmin
	v.Target = map[string]bool{"allowed": CanTarget(a.Roles, a.Super, targetAdmin, t.ID == a.ID), "self": t.ID == a.ID}
	return v, nil
}

// ActionRow is a line of the action log.
type ActionRow struct {
	ID          int64     `json:"id"`
	ActorID     string    `json:"actor_id"`
	Actor       string    `json:"actor"`
	ActorRole   string    `json:"actor_role"`
	TargetID    string    `json:"target_id"`
	TargetEmail string    `json:"target_email"`
	Action      string    `json:"action"`
	Reason      string    `json:"reason"`
	Ticket      string    `json:"ticket"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// ListActions pages through the action log (cursor = last id).
func (s *Service) ListActions(ctx context.Context, orgID, targetID, cursor string, limit int) ([]ActionRow, string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{orgID}
	where := `a.org_id::text=$1`
	if targetID != "" {
		args = append(args, targetID)
		where += fmt.Sprintf(` AND a.target_id::text=$%d`, len(args))
	}
	if cursor != "" {
		args = append(args, cursor)
		where += fmt.Sprintf(` AND a.id < $%d::bigint`, len(args))
	}
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `SELECT a.id, COALESCE(a.actor_id::text,''), COALESCE(u.email,''), a.actor_role, COALESCE(a.target_id::text,''), a.target_email, a.action, a.reason, a.ticket, a.status, a.created_at
FROM adrient.iam_actions a LEFT JOIN adrient.users u ON u.id=a.actor_id WHERE `+where+fmt.Sprintf(` ORDER BY a.id DESC LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []ActionRow{}
	for rows.Next() {
		var r ActionRow
		if err := rows.Scan(&r.ID, &r.ActorID, &r.Actor, &r.ActorRole, &r.TargetID, &r.TargetEmail, &r.Action, &r.Reason, &r.Ticket, &r.Status, &r.CreatedAt); err != nil {
			return nil, "", err
		}
		out = append(out, r)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = fmt.Sprint(out[len(out)-1].ID)
	}
	return out, next, nil
}

// ---- approvals and roles ----------------------------------------------------------------

// Approval is a pending two-person request.
type Approval struct {
	ID          string    `json:"id"`
	Action      string    `json:"action"`
	TargetID    string    `json:"target_id"`
	TargetEmail string    `json:"target_email"`
	Requester   string    `json:"requester"`
	Status      string    `json:"status"`
	Reason      string    `json:"reason"`
	Ticket      string    `json:"ticket"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// ListApprovals returns recent approvals.
func (s *Service) ListApprovals(ctx context.Context, orgID string) ([]Approval, error) {
	rows, err := s.Pool.Query(ctx, `SELECT p.id::text, p.action, p.target_id::text, COALESCE(t.email,''), COALESCE(r.email,''), CASE WHEN p.status='pending' AND p.expires_at < now() THEN 'expired' ELSE p.status END, p.reason, p.ticket, p.expires_at, p.created_at
FROM adrient.iam_approvals p LEFT JOIN adrient.users t ON t.id=p.target_id LEFT JOIN adrient.users r ON r.id=p.requester_id WHERE p.org_id::text=$1 ORDER BY p.created_at DESC LIMIT 100`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Approval{}
	for rows.Next() {
		var x Approval
		if err := rows.Scan(&x.ID, &x.Action, &x.TargetID, &x.TargetEmail, &x.Requester, &x.Status, &x.Reason, &x.Ticket, &x.ExpiresAt, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// RoleRow is one operations-role assignment.
type RoleRow struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// ListRoleAssignments lists who holds which operations role.
func (s *Service) ListRoleAssignments(ctx context.Context, orgID string) ([]RoleRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT a.user_id::text, u.email, u.display_name, a.role FROM adrient.iam_role_assignments a JOIN adrient.users u ON u.id=a.user_id WHERE a.org_id::text=$1 ORDER BY u.email, a.role`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoleRow{}
	for rows.Next() {
		var r RoleRow
		if err := rows.Scan(&r.UserID, &r.Email, &r.Name, &r.Role); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AssignRole gives or removes an operations role.
func (s *Service) AssignRole(ctx context.Context, orgID string, a Actor, userID, role string, remove bool) error {
	if !Has(a.Roles, CapDelegate) {
		return fmt.Errorf("%w: only an identity admin can delegate", ErrForbidden)
	}
	if !ValidRole(role) {
		return bad("unknown role")
	}
	if !a.Super && role == RoleIdentityAdmin {
		return fmt.Errorf("%w: only a super admin can make someone an identity admin", ErrForbidden)
	}
	if !NeedsStepUpOK(s, ctx, a, "role.assign") {
		return ErrStepUp
	}
	if _, err := s.loadTarget(ctx, orgID, userID); err != nil {
		return err
	}
	var err error
	if remove {
		_, err = s.Pool.Exec(ctx, `DELETE FROM adrient.iam_role_assignments WHERE org_id::text=$1 AND user_id::text=$2 AND role=$3`, orgID, userID, role)
	} else {
		_, err = s.Pool.Exec(ctx, `INSERT INTO adrient.iam_role_assignments (org_id, user_id, role, granted_by) VALUES ($1::uuid,$2::uuid,$3,$4::uuid) ON CONFLICT DO NOTHING`, orgID, userID, role, a.ID)
	}
	s.audit(ctx, orgID, a.ID, "role.assign", map[string]any{"user": userID, "role": role, "removed": remove})
	return err
}

// NeedsStepUpOK reports whether the step-up requirement of an action is met.
func NeedsStepUpOK(s *Service, ctx context.Context, a Actor, action string) bool {
	if !NeedsStepUp(action) {
		return true
	}
	return !s.StepUpUntil(ctx, a.SessionKey).IsZero()
}

// ---- reports --------------------------------------------------------------------------

// Report is a table of rows for the reports screen and the CSV export.
type Report struct {
	Key     string     `json:"key"`
	Title   string     `json:"title"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	Note    string     `json:"note,omitempty"`
}

// RunReport builds one report. days is the threshold for dormant accounts and stale sessions.
func (s *Service) RunReport(ctx context.Context, orgID, key string, days int) (Report, error) {
	if days <= 0 || days > 3650 {
		days = 90
	}
	ts := func(t *time.Time) string {
		if t == nil {
			return "never"
		}
		return t.UTC().Format("2006-01-02 15:04")
	}
	switch key {
	case "privileged_no_mfa":
		r := Report{Key: key, Title: "Privileged accounts without a second step", Columns: []string{"Person", "E-mail", "Role", "Last sign-in"}, Rows: [][]string{}}
		rows, err := s.Pool.Query(ctx, `SELECT u.id::text, u.email, u.display_name, (SELECT max(created_at) FROM adrient.login_events le WHERE le.user_id=u.id AND le.success), u.oidc_subject
FROM adrient.users u WHERE u.org_id::text=$1 AND u.deleted_at IS NULL AND u.status='active' ORDER BY u.email LIMIT 2000`, orgID)
		if err != nil {
			return r, err
		}
		type cand struct {
			id, email, name, sub string
			last                 *time.Time
		}
		var cs []cand
		for rows.Next() {
			var c cand
			if rows.Scan(&c.id, &c.email, &c.name, &c.last, &c.sub) == nil {
				cs = append(cs, c)
			}
		}
		rows.Close()
		checked := 0
		for _, c := range cs {
			if s.IsAdminUser == nil || !s.IsAdminUser(ctx, orgID, c.id) {
				continue
			}
			checked++
			if s.IdP == nil || strings.HasPrefix(c.sub, "adrient:") {
				r.Rows = append(r.Rows, []string{c.name, c.email, "Administrator", ts(c.last) + " (second step unknown: no identity provider link)"})
				continue
			}
			fs, err := s.IdP.ListFactors(ctx, c.sub)
			if err != nil {
				r.Rows = append(r.Rows, []string{c.name, c.email, "Administrator", ts(c.last) + " (could not read second steps)"})
				continue
			}
			if len(fs) == 0 {
				r.Rows = append(r.Rows, []string{c.name, c.email, "Administrator", ts(c.last)})
			}
		}
		r.Note = fmt.Sprintf("%d administrator account(s) checked against the identity provider.", checked)
		return r, nil
	case "dormant":
		r := Report{Key: key, Title: fmt.Sprintf("Accounts with no sign-in for %d days", days), Columns: []string{"Person", "E-mail", "Status", "Last sign-in"}, Rows: [][]string{}}
		rows, err := s.Pool.Query(ctx, `SELECT u.display_name, u.email, u.status, (SELECT max(created_at) FROM adrient.login_events le WHERE le.user_id=u.id AND le.success) AS last
FROM adrient.users u WHERE u.org_id::text=$1 AND u.deleted_at IS NULL AND u.user_type='member'
AND COALESCE((SELECT max(created_at) FROM adrient.login_events le WHERE le.user_id=u.id AND le.success), u.created_at) < now() - make_interval(days => $2) ORDER BY last NULLS FIRST, u.email LIMIT 5000`, orgID, days)
		if err != nil {
			return r, err
		}
		defer rows.Close()
		for rows.Next() {
			var n, e, st string
			var l *time.Time
			if rows.Scan(&n, &e, &st, &l) == nil {
				r.Rows = append(r.Rows, []string{n, e, st, ts(l)})
			}
		}
		return r, nil
	case "stale_sessions":
		r := Report{Key: key, Title: fmt.Sprintf("Sessions idle for %d days", days), Columns: []string{"E-mail", "Device", "Address", "Last active"}, Rows: [][]string{}}
		rows, err := s.Pool.Query(ctx, `SELECT u.email, COALESCE(se.user_agent,''), COALESCE(se.ip,''), COALESCE(se.last_seen_at, se.created_at) FROM adrient.sessions se JOIN adrient.users u ON u.id=se.user_id
WHERE u.org_id::text=$1 AND se.revoked_at IS NULL AND se.expires_at > now() AND COALESCE(se.last_seen_at, se.created_at) < now() - make_interval(days => $2) ORDER BY 4 LIMIT 5000`, orgID, days)
		if err != nil {
			return r, err
		}
		defer rows.Close()
		for rows.Next() {
			var e, ua, ip string
			var at time.Time
			if rows.Scan(&e, &ua, &ip, &at) == nil {
				r.Rows = append(r.Rows, []string{e, DeviceLabel(ua), ip, at.UTC().Format("2006-01-02 15:04")})
			}
		}
		return r, nil
	case "coverage":
		st := s.GetSettings(ctx, orgID)
		r := Report{Key: key, Title: "Policy coverage", Columns: []string{"Control", "State"}, Rows: [][]string{}}
		var people, roleHolders, rules, quarantined int
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users WHERE org_id::text=$1 AND deleted_at IS NULL AND status='active'`, orgID).Scan(&people)
		_ = s.Pool.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM adrient.iam_role_assignments WHERE org_id::text=$1`, orgID).Scan(&roleHolders)
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.iam_ip_rules WHERE org_id::text=$1 AND (expires_at IS NULL OR expires_at > now())`, orgID).Scan(&rules)
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.iam_restrictions WHERE org_id::text=$1`, orgID).Scan(&quarantined)
		r.Rows = append(r.Rows,
			[]string{"Second-step policy", st.MFAPolicy.Mode},
			[]string{"Active people", fmt.Sprint(people)},
			[]string{"People holding an operations role", fmt.Sprint(roleHolders)},
			[]string{"Active allow/block entries", fmt.Sprint(rules)},
			[]string{"Accounts in read-only mode", fmt.Sprint(quarantined)},
			[]string{"Actions needing two people", strings.Join(st.RequireApproval, ", ")},
			[]string{"Security mailbox", orDash(st.SecurityMailbox)},
			[]string{"Alert webhook", map[bool]string{true: "set", false: "not set"}[st.WebhookURL != ""]},
		)
		return r, nil
	case "recent_resets":
		r := Report{Key: key, Title: "Recent password and second-step resets", Columns: []string{"When", "Done by", "Person", "Action", "Reason", "Ticket"}, Rows: [][]string{}}
		rows, err := s.Pool.Query(ctx, `SELECT a.created_at, COALESCE(u.email,''), a.target_email, a.action, a.reason, a.ticket FROM adrient.iam_actions a LEFT JOIN adrient.users u ON u.id=a.actor_id
WHERE a.org_id::text=$1 AND a.status='done' AND (a.action LIKE 'password.%' OR a.action LIKE 'mfa.%' OR a.action='contain') AND a.created_at > now() - make_interval(days => $2) ORDER BY a.id DESC LIMIT 5000`, orgID, days)
		if err != nil {
			return r, err
		}
		defer rows.Close()
		for rows.Next() {
			var at time.Time
			var by, tgt, act, why, tk string
			if rows.Scan(&at, &by, &tgt, &act, &why, &tk) == nil {
				r.Rows = append(r.Rows, []string{at.UTC().Format("2006-01-02 15:04"), by, tgt, act, why, tk})
			}
		}
		return r, nil
	}
	return Report{}, bad("unknown report")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---- self service ---------------------------------------------------------------------------

// MySecurity is the data of the person's own security page.
type MySecurity struct {
	Sessions   []SessionOut         `json:"sessions"`
	History    []Event              `json:"history"`
	Factors    []zitadelmgmt.Factor `json:"factors"`
	FactorsErr string               `json:"factors_error,omitempty"`
	Email      string               `json:"email"`
	Quarantine bool                 `json:"quarantined"`
}

// My returns the caller's own security data.
func (s *Service) My(ctx context.Context, orgID string, a Actor) (MySecurity, error) {
	t, err := s.loadTarget(ctx, orgID, a.ID)
	if err != nil {
		return MySecurity{}, err
	}
	v := MySecurity{Email: t.Email, Factors: []zitadelmgmt.Factor{}}
	v.Sessions = s.Sessions(ctx, a.ID, a.SessionKey)
	v.History, _, _ = s.ListEvents(ctx, orgID, EventFilter{UserID: a.ID, Limit: 30})
	v.Quarantine = s.Quarantined(ctx, a.ID)
	if s.IdP != nil && t.Subject != "" && !strings.HasPrefix(t.Subject, "adrient:") {
		if fs, err := s.IdP.ListFactors(ctx, t.Subject); err == nil {
			v.Factors = fs
		} else {
			v.FactorsErr = "Could not read your second steps right now."
		}
	}
	return v, nil
}

// SignOutOthers ends every session of the caller except the current one.
func (s *Service) SignOutOthers(ctx context.Context, userID, currentToken string) int {
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.sessions SET revoked_at=now() WHERE user_id::text=$1 AND revoked_at IS NULL AND token <> $2`, userID, currentToken)
	if err != nil {
		return 0
	}
	return int(tag.RowsAffected())
}

// RevokeOwnSession ends one of the caller's own sessions.
func (s *Service) RevokeOwnSession(ctx context.Context, userID, publicID string) bool {
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.sessions SET revoked_at=now() WHERE user_id::text=$1 AND public_id=$2 AND revoked_at IS NULL`, userID, publicID)
	return err == nil && tag.RowsAffected() > 0
}

// Report a sign-in as not the person ("this wasn't me"). It signs out their other
// sessions, opens a high alert for the security team and tells them.
func (s *Service) ReportActivity(ctx context.Context, orgID string, a Actor, currentToken string, eventID int64, verdict, note string) (string, error) {
	if verdict != "not_me" && verdict != "was_me" && verdict != "suspicious" {
		return "", bad("choose a verdict")
	}
	if len(note) > 500 {
		note = note[:500]
	}
	var ev Event
	if eventID > 0 {
		evs, _ := s.queryEvents(ctx, `SELECT `+eventCols+` FROM adrient.login_events WHERE org_id::text=$1 AND id=$2 AND user_id::text=$3`, orgID, eventID, a.ID)
		if len(evs) == 0 {
			return "", ErrNotFound
		}
		ev = evs[0]
	}
	var alertID string
	if verdict != "was_me" {
		sev := "high"
		title := fmt.Sprintf("%s says a sign-in was not them", a.Email)
		if verdict == "suspicious" {
			sev, title = "medium", fmt.Sprintf("%s reported suspicious activity", a.Email)
		}
		st := s.GetSettings(ctx, orgID)
		evid := fmt.Sprintf(`{"event_id":%d,"ip":%q,"country":%q,"note":%q}`, eventID, ev.IP, ev.Country, note)
		key := "user_report:" + a.ID
		err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.iam_alerts (org_id, user_id, email, ip, rule, severity, title, evidence, recommended, dedupe_key, event_id, sla_due_at)
VALUES ($1::uuid,$2::uuid,$3,$4,'user_report',$5,$6,$7::jsonb,'Contact the person, then contain the account if they confirm it was not them.',$8,NULLIF($9,0),$10)
ON CONFLICT (org_id, dedupe_key) WHERE dedupe_key <> '' AND status IN ('open','investigating') DO UPDATE SET updated_at=now(), evidence=EXCLUDED.evidence RETURNING id::text`,
			orgID, a.ID, a.Email, ev.IP, sev, title, evid, key, eventID, s.now().Add(st.Thresholds.SLA(sev))).Scan(&alertID)
		if err != nil {
			return "", err
		}
		if verdict == "not_me" {
			n := s.SignOutOthers(ctx, a.ID, currentToken)
			s.addComment(ctx, orgID, alertID, a.ID, "action", fmt.Sprintf("The person reported the sign-in. %d other session(s) were ended automatically.", n))
		}
		s.fanOut(ctx, orgID, alertID, Finding{Rule: "user_report", Severity: sev, Title: title, Recommended: "Contact the person and contain the account."}, Event{Email: a.Email, IP: ev.IP}, st)
	}
	_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.iam_user_reports (org_id, user_id, event_id, verdict, note, alert_id) VALUES ($1::uuid,$2::uuid,NULLIF($3,0),$4,$5,NULLIF($6,'')::uuid)`,
		orgID, a.ID, eventID, verdict, note, alertID)
	s.audit(ctx, orgID, a.ID, "user.report", map[string]any{"verdict": verdict, "event": eventID})
	return alertID, nil
}

var _ = http.StatusOK
