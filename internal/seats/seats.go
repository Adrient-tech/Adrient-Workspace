// Package seats covers plans and seats, per-app licences, bulk invitations,
// the session policy, and the login history with suspicious sign-in alerts.
package seats

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/lifecycle"
)

// Service holds the dependencies.
type Service struct {
	Pool  *pgxpool.Pool
	Apps  *authz.AppService
	Roles *authz.Service
	// Invites creates member invitations for bulk invite.
	Invites *lifecycle.Store
	// SendInvite delivers an invitation link (nil = links are only returned).
	SendInvite func(ctx context.Context, to, link, orgName string) error
	// InviteCheck is the external collaboration policy for an address.
	InviteCheck func(ctx context.Context, orgID, email string) error
	// Alert e-mails admins about an event (best effort).
	Alert func(ctx context.Context, orgID, event, subject, body string)
	// Audit records an admin audit event.
	Audit func(ctx context.Context, orgID, actorID, action string, detail map[string]any)
}

// ErrNoSeats is returned when the plan has no free seat.
var ErrNoSeats = errors.New("no seats available")

// ErrBad marks a request the admin can fix.
var ErrBad = errors.New("invalid request")

func (s *Service) audit(ctx context.Context, org, actor, action string, d map[string]any) {
	if s.Audit != nil {
		s.Audit(ctx, org, actor, action, d)
	}
}

// ---- seats -----------------------------------------------------------------

// Plan is the org's plan and seat count (Seats nil = unlimited).
type Plan struct {
	Plan  string `json:"plan"`
	Seats *int   `json:"seats"`
}

// Usage counts what uses a seat: active member accounts (not guests, external
// members or service accounts; archived and deleted people free their seat) and
// open member invitations are reported separately.
type Usage struct {
	Plan
	Used    int  `json:"used"`
	Pending int  `json:"pending_invites"`
	Free    *int `json:"free"`
	Guests  int  `json:"guests"`
}

const usedSQL = `SELECT count(*) FROM adrient.users WHERE org_id::text=$1 AND user_type='member' AND status='active' AND deleted_at IS NULL AND archived_at IS NULL`

// GetPlan reads the plan (defaults: standard, unlimited).
func (s *Service) GetPlan(ctx context.Context, orgID string) Plan {
	p := Plan{Plan: "standard"}
	_ = s.Pool.QueryRow(ctx, `SELECT plan, seats FROM adrient.org_plans WHERE org_id::text=$1`, orgID).Scan(&p.Plan, &p.Seats)
	return p
}

// PutPlan saves the plan. Lowering the seat count under current use is allowed
// (nobody is removed) but blocks new members until usage drops.
func (s *Service) PutPlan(ctx context.Context, orgID, actor string, p Plan) error {
	p.Plan = strings.TrimSpace(p.Plan)
	if p.Plan == "" || len(p.Plan) > 40 || (p.Seats != nil && (*p.Seats < 0 || *p.Seats > 1000000)) {
		return fmt.Errorf("%w: plan name and a seat count of 0 or more", ErrBad)
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.org_plans (org_id, plan, seats) VALUES ($1,$2,$3)
ON CONFLICT (org_id) DO UPDATE SET plan=$2, seats=$3, updated_at=now()`, orgID, p.Plan, p.Seats)
	if err == nil {
		s.audit(ctx, orgID, actor, "seats.plan", map[string]any{"plan": p.Plan, "seats": p.Seats})
	}
	return err
}

// GetUsage reports seat use.
func (s *Service) GetUsage(ctx context.Context, orgID string) (Usage, error) {
	u := Usage{Plan: s.GetPlan(ctx, orgID)}
	if err := s.Pool.QueryRow(ctx, usedSQL, orgID).Scan(&u.Used); err != nil {
		return u, err
	}
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.invitations WHERE org_id::text=$1 AND user_type='member' AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()`, orgID).Scan(&u.Pending)
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users WHERE org_id::text=$1 AND user_type IN ('guest','external') AND status='active' AND deleted_at IS NULL`, orgID).Scan(&u.Guests)
	if u.Seats != nil {
		f := *u.Seats - u.Used
		if f < 0 {
			f = 0
		}
		u.Free = &f
	}
	return u, nil
}

// Check is the seat gate used by SCIM, sign-in, restore and bulk invite.
func (s *Service) Check(ctx context.Context, orgID string) error {
	p := s.GetPlan(ctx, orgID)
	if p.Seats == nil {
		return nil
	}
	var used int
	if err := s.Pool.QueryRow(ctx, usedSQL, orgID).Scan(&used); err != nil {
		return err
	}
	if used >= *p.Seats {
		return fmt.Errorf("%w: all %d seats are in use. Ask a billing admin to add seats", ErrNoSeats, *p.Seats)
	}
	return nil
}

// ---- per-app licences ----------------------------------------------------------

// AppUse is one row of the licence report.
type AppUse struct {
	AppID    string `json:"app_id"`
	Cap      *int   `json:"cap"`
	Assigned int    `json:"assigned"`
	Over     bool   `json:"over"`
}

// Report is the licence usage report.
type Report struct {
	Users int      `json:"users"`
	Apps  []AppUse `json:"apps"`
	Plan  Usage    `json:"plan"`
}

func (s *Service) caps(ctx context.Context, orgID string) map[string]int {
	out := map[string]int{}
	rows, err := s.Pool.Query(ctx, `SELECT app_id, seats FROM adrient.app_seat_caps WHERE org_id::text=$1`, orgID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var a string
		var n int
		if rows.Scan(&a, &n) == nil {
			out[a] = n
		}
	}
	return out
}

// countByApp resolves every active member's app access with the same resolver
// the server enforces with.
func (s *Service) countByApp(ctx context.Context, orgID string) (map[string]int, int, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text FROM adrient.users WHERE org_id::text=$1 AND user_type='member' AND status='active' AND deleted_at IS NULL`, orgID)
	if err != nil {
		return nil, 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	counts := map[string]int{}
	for _, id := range ids {
		set, err := s.Roles.ResolveUser(ctx, orgID, id)
		if err != nil {
			continue
		}
		ds, err := s.Apps.ResolveAll(ctx, orgID, id, set)
		if err != nil {
			continue
		}
		for _, d := range ds {
			if d.Allowed {
				counts[d.AppID]++
			}
		}
	}
	return counts, len(ids), nil
}

// LicenseReport lists, per app, how many active members can open it.
func (s *Service) LicenseReport(ctx context.Context, orgID string) (Report, error) {
	counts, n, err := s.countByApp(ctx, orgID)
	if err != nil {
		return Report{}, err
	}
	caps := s.caps(ctx, orgID)
	r := Report{Users: n}
	r.Plan, _ = s.GetUsage(ctx, orgID)
	for _, id := range authz.GatedApps {
		a := AppUse{AppID: id, Assigned: counts[id]}
		if c, ok := caps[id]; ok {
			c := c
			a.Cap, a.Over = &c, counts[id] > c
		}
		r.Apps = append(r.Apps, a)
	}
	return r, nil
}

// SetCap sets (or clears with nil) the seat cap of one app.
func (s *Service) SetCap(ctx context.Context, orgID, actor, appID string, seats *int) error {
	if !authz.IsKnownApp(appID) {
		return fmt.Errorf("%w: unknown app", ErrBad)
	}
	if seats == nil {
		_, err := s.Pool.Exec(ctx, `DELETE FROM adrient.app_seat_caps WHERE org_id::text=$1 AND app_id=$2`, orgID, appID)
		return err
	}
	if *seats < 0 || *seats > 1000000 {
		return fmt.Errorf("%w: seats must be 0 or more", ErrBad)
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.app_seat_caps (org_id, app_id, seats) VALUES ($1,$2,$3)
ON CONFLICT (org_id, app_id) DO UPDATE SET seats=$3`, orgID, appID, *seats)
	if err == nil {
		s.audit(ctx, orgID, actor, "seats.app_cap", map[string]any{"app": appID, "seats": *seats})
	}
	return err
}

// Assign gives (or takes away) an app for a user or group through the same
// access policies the Admin > App access page edits. A per-app cap refuses an
// assignment that would put the app over its seats.
func (s *Service) Assign(ctx context.Context, orgID, actor, appID, subjectType, subjectID string, assign bool) error {
	if !authz.IsKnownApp(appID) {
		return fmt.Errorf("%w: unknown app", ErrBad)
	}
	if subjectType != "user" && subjectType != "group" {
		return fmt.Errorf("%w: assign to a user or a group", ErrBad)
	}
	var exists bool
	q := `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE org_id::text=$1 AND id::text=$2)`
	if subjectType == "group" {
		q = `SELECT EXISTS(SELECT 1 FROM adrient.groups WHERE org_id::text=$1 AND id::text=$2)`
	}
	if err := s.Pool.QueryRow(ctx, q, orgID, subjectID).Scan(&exists); err != nil || !exists {
		return fmt.Errorf("%w: that %s is not in this organization", ErrBad, subjectType)
	}
	if !assign {
		_, err := s.Pool.Exec(ctx, `DELETE FROM adrient.app_access_policies WHERE org_id::text=$1 AND service_id=$2 AND subject_type=$3 AND subject_id=$4 AND effect='allow'`,
			orgID, appID, subjectType, subjectID)
		s.Apps.Invalidate()
		if err == nil {
			s.audit(ctx, orgID, actor, "seats.unassign", map[string]any{"app": appID, "subject_type": subjectType, "subject_id": subjectID})
		}
		return err
	}
	tag, err := s.Pool.Exec(ctx, `INSERT INTO adrient.app_access_policies (org_id, service_id, subject_type, subject_id, effect, created_by)
VALUES ($1,$2,$3,$4,'allow',NULLIF($5,'')::uuid) ON CONFLICT DO NOTHING`, orgID, appID, subjectType, subjectID, actor)
	if err != nil {
		return err
	}
	s.Apps.Invalidate()
	if caps := s.caps(ctx, orgID); caps[appID] > 0 || hasKey(caps, appID) {
		counts, _, cerr := s.countByApp(ctx, orgID)
		if cerr == nil && counts[appID] > caps[appID] {
			if tag.RowsAffected() > 0 {
				_, _ = s.Pool.Exec(ctx, `DELETE FROM adrient.app_access_policies WHERE org_id::text=$1 AND service_id=$2 AND subject_type=$3 AND subject_id=$4 AND effect='allow'`,
					orgID, appID, subjectType, subjectID)
				s.Apps.Invalidate()
			}
			return fmt.Errorf("%w: %s has %d seats and %d people would use it", ErrNoSeats, appID, caps[appID], counts[appID])
		}
	}
	s.audit(ctx, orgID, actor, "seats.assign", map[string]any{"app": appID, "subject_type": subjectType, "subject_id": subjectID})
	return nil
}

func hasKey(m map[string]int, k string) bool { _, ok := m[k]; return ok }

// ---- bulk invitations (CSV) -------------------------------------------------------

// InviteRow is the outcome for one CSV line.
type InviteRow struct {
	Line   int    `json:"line"`
	Email  string `json:"email"`
	Status string `json:"status"` // invited | skipped | error
	Reason string `json:"reason,omitempty"`
	Link   string `json:"link,omitempty"`
	Sent   bool   `json:"email_sent,omitempty"`
}

// MaxBulk is the largest CSV accepted in one go.
const MaxBulk = 500

// BulkInvite reads "email[,name[,apps]]" lines (header optional) and creates a
// member invitation per line. Seats are honored: lines beyond the free seats
// (counting open invitations) are reported and not invited. dryRun validates
// without creating anything.
func (s *Service) BulkInvite(ctx context.Context, orgID, actor, orgName, baseURL string, csvData io.Reader, dryRun bool) ([]InviteRow, error) {
	rd := csv.NewReader(io.LimitReader(csvData, 1<<20))
	rd.FieldsPerRecord = -1
	rd.TrimLeadingSpace = true
	recs, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: the file is not valid CSV (%v)", ErrBad, err)
	}
	if len(recs) > 0 && len(recs[0]) > 0 && strings.EqualFold(strings.TrimSpace(recs[0][0]), "email") {
		recs = recs[1:]
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("%w: the file has no rows", ErrBad)
	}
	if len(recs) > MaxBulk {
		return nil, fmt.Errorf("%w: at most %d rows at a time", ErrBad, MaxBulk)
	}
	usage, err := s.GetUsage(ctx, orgID)
	if err != nil {
		return nil, err
	}
	free := -1
	if usage.Seats != nil {
		free = *usage.Seats - usage.Used - usage.Pending
		if free < 0 {
			free = 0
		}
	}
	seen := map[string]bool{}
	out := make([]InviteRow, 0, len(recs))
	for i, rec := range recs {
		row := InviteRow{Line: i + 1}
		if len(rec) == 0 {
			continue
		}
		raw := strings.TrimSpace(rec[0])
		addr, perr := mail.ParseAddress(raw)
		if perr != nil || !strings.Contains(addr.Address, "@") {
			row.Email, row.Status, row.Reason = raw, "error", "not an e-mail address"
			out = append(out, row)
			continue
		}
		email := strings.ToLower(addr.Address)
		row.Email = email
		switch {
		case seen[email]:
			row.Status, row.Reason = "skipped", "listed twice"
		case free == 0:
			row.Status, row.Reason = "error", "no free seat left"
		default:
			seen[email] = true
			if s.InviteCheck != nil {
				if cerr := s.InviteCheck(ctx, orgID, email); cerr != nil {
					row.Status, row.Reason = "error", cerr.Error()
					out = append(out, row)
					continue
				}
			}
			if dryRun {
				row.Status = "invited"
				if free > 0 {
					free--
				}
				out = append(out, row)
				continue
			}
			inv, secret, ierr := s.Invites.CreateInvitation(ctx, orgID, actor, lifecycle.InviteInput{Email: email, UserType: "member"})
			switch {
			case errors.Is(ierr, lifecycle.ErrExists):
				row.Status, row.Reason = "skipped", "already has an account"
			case ierr != nil:
				row.Status, row.Reason = "error", "could not create the invitation"
			default:
				_ = inv
				row.Status = "invited"
				row.Link = strings.TrimRight(baseURL, "/") + "/accept-invite?code=" + secret
				if s.SendInvite != nil {
					row.Sent = s.SendInvite(ctx, email, row.Link, orgName) == nil
				}
				if free > 0 {
					free--
				}
			}
		}
		out = append(out, row)
	}
	if !dryRun {
		n := 0
		for _, r := range out {
			if r.Status == "invited" {
				n++
			}
		}
		s.audit(ctx, orgID, actor, "users.bulk_invite", map[string]any{"invited": n, "rows": len(out)})
	}
	return out, nil
}

// ---- session policy ------------------------------------------------------------------

// SessionSettings are enforced by the session store and sign-in.
type SessionSettings struct {
	MaxHours      *int `json:"session_max_hours"`
	IdleMinutes   *int `json:"session_idle_minutes"`
	MaxConcurrent *int `json:"max_concurrent_sessions"`
	AlertNewDev   bool `json:"alert_new_device"`
	FailThreshold int  `json:"alert_failed_threshold"`
}

// GetSessionSettings reads the policy (defaults when none).
func (s *Service) GetSessionSettings(ctx context.Context, orgID string) SessionSettings {
	v := SessionSettings{AlertNewDev: true, FailThreshold: 5}
	_ = s.Pool.QueryRow(ctx, `SELECT session_max_hours, session_idle_minutes, max_concurrent_sessions, alert_new_device, alert_failed_threshold
FROM adrient.org_security_settings WHERE org_id::text=$1`, orgID).Scan(&v.MaxHours, &v.IdleMinutes, &v.MaxConcurrent, &v.AlertNewDev, &v.FailThreshold)
	return v
}

// PutSessionSettings validates and saves the policy.
func (s *Service) PutSessionSettings(ctx context.Context, orgID, actor string, v SessionSettings) error {
	in := func(p *int, lo, hi int) bool { return p == nil || (*p >= lo && *p <= hi) }
	if !in(v.MaxHours, 1, 8760) || !in(v.IdleMinutes, 5, 525600) || !in(v.MaxConcurrent, 1, 100) || v.FailThreshold < 2 || v.FailThreshold > 100 {
		return fmt.Errorf("%w: session length 1-8760 hours, idle timeout 5 minutes or more, 1-100 sessions, alert after 2-100 failures", ErrBad)
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.org_security_settings (org_id, session_max_hours, session_idle_minutes, max_concurrent_sessions, alert_new_device, alert_failed_threshold)
VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (org_id) DO UPDATE SET session_max_hours=$2, session_idle_minutes=$3, max_concurrent_sessions=$4,
alert_new_device=$5, alert_failed_threshold=$6, updated_at=now()`, orgID, v.MaxHours, v.IdleMinutes, v.MaxConcurrent, v.AlertNewDev, v.FailThreshold)
	if err == nil {
		s.audit(ctx, orgID, actor, "security.sessions", map[string]any{"max_hours": v.MaxHours, "idle": v.IdleMinutes, "concurrent": v.MaxConcurrent})
	}
	return err
}

// LimitsFor resolves the limits that apply to a user (used by the session store).
func (s *Service) LimitsFor(ctx context.Context, userID string) auth.SessionLimits {
	var mh, im, mc *int
	err := s.Pool.QueryRow(ctx, `SELECT o.session_max_hours, o.session_idle_minutes, o.max_concurrent_sessions
FROM adrient.users u JOIN adrient.org_security_settings o ON o.org_id=u.org_id WHERE u.id::text=$1`, userID).Scan(&mh, &im, &mc)
	if err != nil {
		return auth.SessionLimits{}
	}
	var l auth.SessionLimits
	if mh != nil {
		l.MaxAge = time.Duration(*mh) * time.Hour
	}
	if im != nil {
		l.Idle = time.Duration(*im) * time.Minute
	}
	if mc != nil {
		l.MaxConcurrent = *mc
	}
	return l
}

// RevokeUserSessions signs a user out everywhere.
func (s *Service) RevokeUserSessions(ctx context.Context, orgID, actor, userID string) (int, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.sessions SET revoked_at=now() WHERE revoked_at IS NULL AND user_id IN
(SELECT id FROM adrient.users WHERE org_id::text=$1 AND id::text=$2)`, orgID, userID)
	if err != nil {
		return 0, err
	}
	s.audit(ctx, orgID, actor, "sessions.revoke_all", map[string]any{"user_id": userID, "count": tag.RowsAffected()})
	return int(tag.RowsAffected()), nil
}

// ---- login history -----------------------------------------------------------------------

// Login is one row of the history.
type Login struct {
	ID         int64     `json:"id"`
	UserID     string    `json:"user_id"`
	Email      string    `json:"email"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	Method     string    `json:"method"`
	Success    bool      `json:"success"`
	Reason     string    `json:"reason"`
	Suspicious bool      `json:"suspicious"`
	CreatedAt  time.Time `json:"created_at"`
}

// Record stores a sign-in attempt, flags suspicious ones and alerts admins.
// It is the function registered with auth.SetLoginObserver.
func (s *Service) Record(ctx context.Context, ev auth.LoginEvent) {
	orgID, userID := ev.OrgID, ev.UserID
	if orgID == "" && ev.Email != "" {
		_ = s.Pool.QueryRow(ctx, `SELECT org_id::text, id::text FROM adrient.users WHERE lower(email)=lower($1) AND deleted_at IS NULL ORDER BY created_at LIMIT 1`, ev.Email).Scan(&orgID, &userID)
	}
	if len(ev.UserAgent) > 300 {
		ev.UserAgent = ev.UserAgent[:300]
	}
	settings := SessionSettings{AlertNewDev: true, FailThreshold: 5}
	if orgID != "" {
		settings = s.GetSessionSettings(ctx, orgID)
	}
	suspicious, why := false, ""
	if ev.Success && userID != "" {
		var prior, same int
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE true), count(*) FILTER (WHERE ip=$2 AND user_agent=$3)
FROM adrient.login_events WHERE user_id::text=$1 AND success AND created_at > now() - interval '90 days'`, userID, ev.IP, ev.UserAgent).Scan(&prior, &same)
		if prior > 0 && same == 0 && settings.AlertNewDev {
			suspicious, why = true, "sign-in from a new device and address"
		}
	}
	if !ev.Success && ev.Email != "" {
		var fails int
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.login_events WHERE lower(email)=lower($1) AND NOT success AND created_at > now() - interval '10 minutes'`, ev.Email).Scan(&fails)
		if fails+1 == settings.FailThreshold {
			suspicious, why = true, fmt.Sprintf("%d failed sign-ins in 10 minutes", fails+1)
		}
	}
	reason := ev.Reason
	if why != "" {
		reason = why
	}
	_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.login_events (org_id, user_id, email, ip, user_agent, method, success, reason, suspicious)
VALUES (NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, $3, $4, $5, $6, $7, $8, $9)`, orgID, userID, ev.Email, ev.IP, ev.UserAgent, ev.Method, ev.Success, reason, suspicious)
	if suspicious && orgID != "" && s.Alert != nil {
		s.Alert(ctx, orgID, "login.suspicious", "Suspicious sign-in: "+ev.Email,
			fmt.Sprintf("%s: %s (address %s, %s).", ev.Email, why, ev.IP, ev.Method))
	}
}

// Logins lists the org's history, optionally for one user or only suspicious/failed rows.
func (s *Service) Logins(ctx context.Context, orgID, userID, filter string, limit int) ([]Login, error) {
	return s.LoginsPage(ctx, orgID, userID, filter, "", 0, limit)
}

// LoginsPage is Logins with a keyset cursor: only rows with id below beforeID
// (0 = from the newest), an optional email substring, and a capped page size.
// The org and user comparisons are on the uuid columns so the (org_id, id) and
// (user_id, created_at) indexes are used.
func (s *Service) LoginsPage(ctx context.Context, orgID, userID, filter, email string, beforeID int64, limit int) ([]Login, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, COALESCE(user_id::text,''), email, ip, user_agent, method, success, reason, suspicious, created_at
FROM adrient.login_events WHERE org_id=$1::uuid`
	args := []any{orgID}
	if userID != "" {
		args = append(args, userID)
		q += fmt.Sprintf(" AND user_id=$%d::uuid", len(args))
	}
	if email != "" {
		args = append(args, strings.ToLower(email)+"%")
		q += fmt.Sprintf(" AND lower(email) LIKE $%d", len(args))
	}
	if beforeID > 0 {
		args = append(args, beforeID)
		q += fmt.Sprintf(" AND id < $%d", len(args))
	}
	switch filter {
	case "suspicious":
		q += " AND suspicious"
	case "failed":
		q += " AND NOT success"
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d", len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Login{}
	for rows.Next() {
		var l Login
		if err := rows.Scan(&l.ID, &l.UserID, &l.Email, &l.IP, &l.UserAgent, &l.Method, &l.Success, &l.Reason, &l.Suspicious, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

var _ = pgx.ErrNoRows
