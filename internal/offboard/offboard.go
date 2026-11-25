// Package offboard covers the end of a person's time in the organization:
// suspend, archive, delete with data transfer and a restore window, legal
// holds that block deletion, retention settings and the admin e-mail alerts.
package offboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors with messages that are safe to show to an admin.
var (
	ErrNotFound   = errors.New("user not found")
	ErrLegalHold  = errors.New("blocked by a legal hold")
	ErrBadRequest = errors.New("invalid request")
	ErrNoSeats    = errors.New("no seats available")
)

// HoldError says which hold blocked an action.
type HoldError struct{ Hold string }

func (e *HoldError) Error() string { return "This action is blocked by the legal hold \"" + e.Hold + "\"." }
func (e *HoldError) Unwrap() error { return ErrLegalHold }

// IdP mirrors user state into the identity provider (optional).
type IdP interface {
	SetActive(ctx context.Context, issuer, subject string, active bool) error
	Delete(ctx context.Context, issuer, subject string) error
}

// Service holds the dependencies.
type Service struct {
	Pool *pgxpool.Pool
	IdP  IdP
	// SeatCheck is consulted before a user becomes active again (restore, unsuspend).
	SeatCheck func(ctx context.Context, orgID string) error
	// Audit records an admin audit event.
	Audit func(ctx context.Context, orgID, actorID, action string, detail map[string]any)
	// Mail sends an e-mail (admin alerts). nil disables alerts.
	Mail func(ctx context.Context, to []string, subject, body string) error
	Now  func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// transferable lists the content a person owns that can be handed to someone
// else. Group is the checkbox shown in the admin.
var transferable = []struct{ Group, Table, Col, Label string }{
	{"drive", "drive_files", "owner_id", "Drive files and folders"},
	{"drive", "drive_shares", "created_by", "Drive share links"},
	{"calendar", "calendar_events", "owner_id", "Calendar events"},
	{"docs", "docs_documents", "owner_id", "Docs"},
	{"docs", "sheets_documents", "owner_id", "Sheets"},
	{"docs", "slides_documents", "owner_id", "Slides"},
	{"docs", "docs_shares", "created_by", "Docs share links"},
	{"contacts", "contacts", "owner_id", "Contacts"},
	{"contacts", "contact_groups", "owner_user_id", "Contact groups"},
	{"tasks", "tasks", "owner_user_id", "Tasks"},
	{"tasks", "task_lists", "owner_user_id", "Task lists"},
	{"forms", "forms", "owner_id", "Forms"},
	{"other", "whiteboards", "owner_id", "Whiteboards"},
	{"other", "keep_notes", "owner_id", "Keep notes"},
	{"other", "meet_rooms", "owner_id", "Meet rooms"},
	{"other", "groups", "owner_id", "Groups owned"},
}

// Groups are the options the admin can pick (all on by default).
var Groups = []string{"drive", "calendar", "docs", "contacts", "tasks", "forms", "other"}

// DeleteInput describes a delete with transfer.
type DeleteInput struct {
	OrgID, UserID, Actor string
	TransferTo           string   // user id receiving the data ("" = nothing is transferred)
	Transfer             []string // groups to transfer; empty with TransferTo set = all
	ForwardMail          bool     // forward the user's incoming mail to TransferTo
	LeaveChats           bool     // remove the user from group channels (direct messages stay)
	Note                 string
}

// Result reports what a lifecycle action did.
type Result struct {
	Action      string         `json:"action"`
	Transferred map[string]int `json:"transferred,omitempty"`
	MailForward string         `json:"mail_forward,omitempty"`
	ChatsLeft   int            `json:"chats_left,omitempty"`
	PurgeAfter  *time.Time     `json:"purge_after,omitempty"`
}

type target struct {
	ID, Email, Name, Status, Type, Issuer, Subject string
	Deleted, Archived                              *time.Time
}

func (s *Service) load(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, orgID, id string) (target, error) {
	var t target
	err := q.QueryRow(ctx, `SELECT id::text, email, display_name, status, user_type, oidc_issuer, oidc_subject, deleted_at, archived_at
FROM adrient.users WHERE org_id::text=$1 AND id::text=$2`, orgID, id).
		Scan(&t.ID, &t.Email, &t.Name, &t.Status, &t.Type, &t.Issuer, &t.Subject, &t.Deleted, &t.Archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// HoldFor returns the name of an active legal hold covering userID.
func (s *Service) HoldFor(ctx context.Context, orgID, userID string) (string, bool) {
	var name string
	err := s.Pool.QueryRow(ctx, `SELECT name FROM adrient.legal_holds
WHERE org_id::text=$1 AND released_at IS NULL AND (org_wide OR $2::uuid = ANY(user_ids)) ORDER BY created_at LIMIT 1`, orgID, userID).Scan(&name)
	return name, err == nil
}

func (s *Service) audit(ctx context.Context, orgID, actor, action string, d map[string]any) {
	if s.Audit != nil {
		s.Audit(ctx, orgID, actor, action, d)
	}
}

func (s *Service) job(ctx context.Context, orgID, userID, kind, actor, transferTo, status, note string, opts, result any) {
	ob, _ := json.Marshal(opts)
	rb, _ := json.Marshal(result)
	_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.offboard_jobs (org_id, user_id, kind, transfer_to, options, result, status, note, started_by)
VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,NULLIF($9,'')::uuid)`, orgID, userID, kind, transferTo, ob, rb, status, note, actor)
}

func (s *Service) revokeSessions(ctx context.Context, userID string) {
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.sessions SET revoked_at=now() WHERE user_id::text=$1 AND revoked_at IS NULL`, userID)
}

func (s *Service) idpActive(ctx context.Context, t target, active bool) {
	if s.IdP != nil && t.Issuer != "adrient:preprovisioned" {
		_ = s.IdP.SetActive(ctx, t.Issuer, t.Subject, active)
	}
}

// Suspend blocks sign-in and ends every session. The user keeps their data.
func (s *Service) Suspend(ctx context.Context, orgID, userID, actor, reason string) error {
	t, err := s.load(ctx, s.Pool, orgID, userID)
	if err != nil {
		return err
	}
	if t.Type == "service" {
		return fmt.Errorf("%w: service accounts cannot be suspended here", ErrBadRequest)
	}
	if userID == actor {
		return fmt.Errorf("%w: you cannot suspend yourself", ErrBadRequest)
	}
	if reason == "" {
		reason = "Suspended by an administrator"
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.users SET status='blocked', blocked_at=now(), blocked_reason=$3, updated_at=now() WHERE org_id::text=$1 AND id::text=$2`, orgID, userID, reason); err != nil {
		return err
	}
	s.revokeSessions(ctx, userID)
	s.idpActive(ctx, t, false)
	s.job(ctx, orgID, userID, "suspend", actor, "", "done", reason, nil, nil)
	s.audit(ctx, orgID, actor, "user.suspend", map[string]any{"user_id": userID, "email": t.Email})
	s.alert(ctx, orgID, "user.suspend", "User suspended: "+t.Email, t.Email+" was suspended.")
	return nil
}

// Unsuspend lets a suspended (not deleted) user sign in again.
func (s *Service) Unsuspend(ctx context.Context, orgID, userID, actor string) error {
	t, err := s.load(ctx, s.Pool, orgID, userID)
	if err != nil {
		return err
	}
	if t.Deleted != nil {
		return fmt.Errorf("%w: the user is deleted; restore them instead", ErrBadRequest)
	}
	if s.SeatCheck != nil && t.Type == "member" && t.Status != "active" {
		if err := s.SeatCheck(ctx, orgID); err != nil {
			return err
		}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.users SET status='active', blocked_at=NULL, blocked_reason='', archived_at=NULL, updated_at=now() WHERE org_id::text=$1 AND id::text=$2`, orgID, userID); err != nil {
		return err
	}
	s.idpActive(ctx, t, true)
	s.job(ctx, orgID, userID, "unsuspend", actor, "", "done", "", nil, nil)
	s.audit(ctx, orgID, actor, "user.unsuspend", map[string]any{"user_id": userID, "email": t.Email})
	return nil
}

// Archive suspends the user and frees their seat while keeping every item.
func (s *Service) Archive(ctx context.Context, orgID, userID, actor, note string) error {
	if err := s.Suspend(ctx, orgID, userID, actor, "Archived"); err != nil {
		return err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.users SET archived_at=now(), offboard_note=$3 WHERE org_id::text=$1 AND id::text=$2`, orgID, userID, note); err != nil {
		return err
	}
	s.job(ctx, orgID, userID, "archive", actor, "", "done", note, nil, nil)
	s.audit(ctx, orgID, actor, "user.archive", map[string]any{"user_id": userID})
	return nil
}

// RestoreWindow returns the org's restore window.
func (s *Service) RestoreWindow(ctx context.Context, orgID string) int {
	d := 30
	_ = s.Pool.QueryRow(ctx, `SELECT restore_window_days FROM adrient.retention_settings WHERE org_id::text=$1`, orgID).Scan(&d)
	return d
}

// SoftDelete deletes without transferring anything (used by SCIM).
func (s *Service) SoftDelete(ctx context.Context, orgID, userID, actor string) error {
	_, err := s.Delete(ctx, DeleteInput{OrgID: orgID, UserID: userID, Actor: actor, LeaveChats: true})
	return err
}

// Delete removes a person: it refuses while a legal hold covers them, hands
// their content and mail to TransferTo, then blocks the account. The account
// can be restored until the restore window ends, after which Purge anonymizes it.
func (s *Service) Delete(ctx context.Context, in DeleteInput) (Result, error) {
	res := Result{Action: "delete"}
	t, err := s.load(ctx, s.Pool, in.OrgID, in.UserID)
	if err != nil {
		return res, err
	}
	if t.Deleted != nil {
		return res, fmt.Errorf("%w: the user is already deleted", ErrBadRequest)
	}
	if t.Type == "service" {
		return res, fmt.Errorf("%w: delete service accounts through their own page", ErrBadRequest)
	}
	if in.UserID == in.Actor {
		return res, fmt.Errorf("%w: you cannot delete yourself", ErrBadRequest)
	}
	if hold, held := s.HoldFor(ctx, in.OrgID, in.UserID); held {
		s.job(ctx, in.OrgID, in.UserID, "delete", in.Actor, in.TransferTo, "refused", "legal hold: "+hold, in, nil)
		s.audit(ctx, in.OrgID, in.Actor, "user.delete.refused", map[string]any{"user_id": in.UserID, "hold": hold})
		return res, &HoldError{Hold: hold}
	}
	var dest target
	if in.TransferTo != "" {
		if in.TransferTo == in.UserID {
			return res, fmt.Errorf("%w: choose someone else to receive the data", ErrBadRequest)
		}
		if dest, err = s.load(ctx, s.Pool, in.OrgID, in.TransferTo); err != nil {
			return res, fmt.Errorf("%w: the person receiving the data was not found", ErrBadRequest)
		}
		if dest.Deleted != nil || dest.Status != "active" || dest.Type == "guest" {
			return res, fmt.Errorf("%w: the person receiving the data must be an active member", ErrBadRequest)
		}
	}
	groups := map[string]bool{}
	for _, g := range in.Transfer {
		groups[g] = true
	}
	if in.TransferTo != "" && len(groups) == 0 {
		for _, g := range Groups {
			groups[g] = true
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)
	res.Transferred = map[string]int{}
	if in.TransferTo != "" {
		for _, tr := range transferable {
			if !groups[tr.Group] {
				continue
			}
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT to_regclass('adrient.'||$1) IS NOT NULL`, tr.Table).Scan(&exists); err != nil || !exists {
				continue
			}
			tag, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE adrient.%s SET %s=$1::uuid WHERE %s=$2::uuid`, tr.Table, tr.Col, tr.Col), in.TransferTo, in.UserID)
			if err != nil {
				return res, fmt.Errorf("transfer %s: %w", tr.Label, err)
			}
			if n := int(tag.RowsAffected()); n > 0 {
				res.Transferred[tr.Label] = n
			}
		}
		if in.ForwardMail {
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.mail_rules (org_id, owner_id, name, act_forward) VALUES ($1,$2,$3,$4)`,
				in.OrgID, in.UserID, "Forward to "+dest.Email+" (account deleted)", dest.Email); err != nil {
				return res, fmt.Errorf("mail forwarding: %w", err)
			}
			res.MailForward = dest.Email
		}
	}
	if in.LeaveChats {
		tag, err := tx.Exec(ctx, `UPDATE adrient.chat_channels SET member_ids = member_ids - $2::text
WHERE org_id::text=$1 AND kind='group' AND member_ids ? $2::text`, in.OrgID, in.UserID)
		if err != nil {
			return res, fmt.Errorf("leave chats: %w", err)
		}
		res.ChatsLeft = int(tag.RowsAffected())
	}
	purge := s.now().AddDate(0, 0, s.RestoreWindow(ctx, in.OrgID))
	res.PurgeAfter = &purge
	if _, err := tx.Exec(ctx, `UPDATE adrient.users SET status='blocked', blocked_at=now(), blocked_reason='Deleted', deleted_at=now(), purge_after=$3, offboard_note=$4, updated_at=now()
WHERE org_id::text=$1 AND id::text=$2`, in.OrgID, in.UserID, purge, in.Note); err != nil {
		return res, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sessions SET revoked_at=now() WHERE user_id::text=$1 AND revoked_at IS NULL`, in.UserID); err != nil {
		return res, err
	}
	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	s.idpActive(ctx, t, false)
	s.job(ctx, in.OrgID, in.UserID, "delete", in.Actor, in.TransferTo, "done", in.Note, in, res)
	s.audit(ctx, in.OrgID, in.Actor, "user.delete", map[string]any{"user_id": in.UserID, "email": t.Email, "transfer_to": in.TransferTo, "transferred": res.Transferred})
	s.alert(ctx, in.OrgID, "user.delete", "User deleted: "+t.Email, fmt.Sprintf("%s was deleted. Restorable until %s.", t.Email, purge.Format("2006-01-02")))
	return res, nil
}

// Restore brings a deleted user back inside the restore window. Content that
// was transferred stays with its new owner.
func (s *Service) Restore(ctx context.Context, orgID, userID, actor string) error {
	t, err := s.load(ctx, s.Pool, orgID, userID)
	if err != nil {
		return err
	}
	if t.Deleted == nil {
		return fmt.Errorf("%w: the user is not deleted", ErrBadRequest)
	}
	var purged bool
	_ = s.Pool.QueryRow(ctx, `SELECT purge_after IS NULL OR purge_after < now() FROM adrient.users WHERE id::text=$1`, userID).Scan(&purged)
	if purged || strings.HasPrefix(t.Email, "deleted-") {
		return fmt.Errorf("%w: the restore window has ended", ErrBadRequest)
	}
	if s.SeatCheck != nil && t.Type == "member" {
		if err := s.SeatCheck(ctx, orgID); err != nil {
			return err
		}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.users SET status='active', blocked_at=NULL, blocked_reason='', deleted_at=NULL, purge_after=NULL, archived_at=NULL, updated_at=now()
WHERE org_id::text=$1 AND id::text=$2`, orgID, userID); err != nil {
		return err
	}
	s.idpActive(ctx, t, true)
	s.job(ctx, orgID, userID, "restore", actor, "", "done", "", nil, nil)
	s.audit(ctx, orgID, actor, "user.restore", map[string]any{"user_id": userID, "email": t.Email})
	return nil
}

// Purge anonymizes users whose restore window has ended and who are not under a
// legal hold. It returns how many accounts were purged.
func (s *Service) Purge(ctx context.Context) (int, error) {
	rows, err := s.Pool.Query(ctx, `SELECT org_id::text, id::text, oidc_issuer, oidc_subject FROM adrient.users
WHERE deleted_at IS NOT NULL AND purge_after < now() AND email NOT LIKE 'deleted-%'`)
	if err != nil {
		return 0, err
	}
	type row struct{ org, id, iss, sub string }
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.org, &r.id, &r.iss, &r.sub); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, r)
	}
	rows.Close()
	n := 0
	for _, r := range todo {
		if _, held := s.HoldFor(ctx, r.org, r.id); held {
			continue
		}
		if s.IdP != nil && r.iss != "adrient:preprovisioned" {
			_ = s.IdP.Delete(ctx, r.iss, r.sub)
		}
		if _, err := s.Pool.Exec(ctx, `UPDATE adrient.users SET email='deleted-'||id::text||'@deleted.invalid', display_name='Deleted user',
oidc_issuer='adrient:deleted', oidc_subject='deleted-'||id::text, upn=NULL, phone='', employee_id='', job_title='', scim_external_id=NULL
WHERE id::text=$1`, r.id); err != nil {
			continue
		}
		_, _ = s.Pool.Exec(ctx, `DELETE FROM adrient.group_members WHERE user_id::text=$1`, r.id)
		s.job(ctx, r.org, r.id, "purge", "", "", "done", "", nil, nil)
		s.audit(ctx, r.org, "", "user.purge", map[string]any{"user_id": r.id})
		n++
	}
	return n, nil
}

// ---- legal holds and retention ---------------------------------------------

// Hold is a legal hold.
type Hold struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Reason     string     `json:"reason"`
	OrgWide    bool       `json:"org_wide"`
	UserIDs    []string   `json:"user_ids"`
	CreatedAt  time.Time  `json:"created_at"`
	ReleasedAt *time.Time `json:"released_at"`
}

// CreateHold places a hold on an org or a set of users.
func (s *Service) CreateHold(ctx context.Context, orgID, actor string, h Hold) (Hold, error) {
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" || len(h.Name) > 120 {
		return h, fmt.Errorf("%w: a hold needs a name", ErrBadRequest)
	}
	if !h.OrgWide && len(h.UserIDs) == 0 {
		return h, fmt.Errorf("%w: choose people, or hold the whole organization", ErrBadRequest)
	}
	ids := []string{}
	if len(h.UserIDs) > 0 {
		rows, err := s.Pool.Query(ctx, `SELECT id::text FROM adrient.users WHERE org_id::text=$1 AND id::text = ANY($2)`, orgID, h.UserIDs)
		if err != nil {
			return h, err
		}
		ids = ids[:0]
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
		if len(ids) != len(h.UserIDs) {
			return h, fmt.Errorf("%w: some people are not in this organization", ErrBadRequest)
		}
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.legal_holds (org_id, name, reason, org_wide, user_ids, created_by)
VALUES ($1,$2,$3,$4,$5::uuid[],NULLIF($6,'')::uuid) RETURNING id::text, created_at`, orgID, h.Name, h.Reason, h.OrgWide, ids, actor).Scan(&h.ID, &h.CreatedAt)
	if err != nil {
		return h, err
	}
	h.UserIDs = ids
	s.audit(ctx, orgID, actor, "hold.create", map[string]any{"hold": h.Name, "org_wide": h.OrgWide, "users": len(ids)})
	return h, nil
}

// ReleaseHold releases a hold.
func (s *Service) ReleaseHold(ctx context.Context, orgID, actor, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.legal_holds SET released_at=now(), released_by=NULLIF($3,'')::uuid WHERE org_id::text=$1 AND id::text=$2 AND released_at IS NULL`, orgID, id, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	s.audit(ctx, orgID, actor, "hold.release", map[string]any{"hold_id": id})
	return nil
}

// ListHolds lists holds (active first).
func (s *Service) ListHolds(ctx context.Context, orgID string) ([]Hold, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, name, reason, org_wide, user_ids::text[], created_at, released_at FROM adrient.legal_holds
WHERE org_id::text=$1 ORDER BY (released_at IS NOT NULL), created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hold{}
	for rows.Next() {
		var h Hold
		if err := rows.Scan(&h.ID, &h.Name, &h.Reason, &h.OrgWide, &h.UserIDs, &h.CreatedAt, &h.ReleasedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Retention is the org's retention settings.
type Retention struct {
	RestoreWindowDays  int  `json:"restore_window_days"`
	AuditRetentionDays *int `json:"audit_retention_days"`
}

// GetRetention reads the settings (defaults when unset).
func (s *Service) GetRetention(ctx context.Context, orgID string) Retention {
	r := Retention{RestoreWindowDays: 30}
	_ = s.Pool.QueryRow(ctx, `SELECT restore_window_days, audit_retention_days FROM adrient.retention_settings WHERE org_id::text=$1`, orgID).Scan(&r.RestoreWindowDays, &r.AuditRetentionDays)
	return r
}

// PutRetention saves the settings.
func (s *Service) PutRetention(ctx context.Context, orgID, actor string, r Retention) error {
	if r.RestoreWindowDays < 0 || r.RestoreWindowDays > 3650 || (r.AuditRetentionDays != nil && *r.AuditRetentionDays < 30) {
		return fmt.Errorf("%w: restore window 0-3650 days, audit retention at least 30 days", ErrBadRequest)
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.retention_settings (org_id, restore_window_days, audit_retention_days) VALUES ($1,$2,$3)
ON CONFLICT (org_id) DO UPDATE SET restore_window_days=$2, audit_retention_days=$3, updated_at=now()`, orgID, r.RestoreWindowDays, r.AuditRetentionDays)
	if err == nil {
		s.audit(ctx, orgID, actor, "retention.update", map[string]any{"restore_days": r.RestoreWindowDays})
	}
	return err
}

// ---- admin e-mail alerts -----------------------------------------------------

// AlertSettings says who gets e-mail for which admin events.
type AlertSettings struct {
	Recipients []string `json:"recipients"`
	Events     []string `json:"events"`
}

// AlertEvents lists the events an admin can subscribe to.
var AlertEvents = []string{"user.delete", "user.suspend", "login.suspicious", "hold.create", "review.completed", "admin.role"}

// GetAlerts reads the alert settings.
func (s *Service) GetAlerts(ctx context.Context, orgID string) AlertSettings {
	a := AlertSettings{Recipients: []string{}, Events: []string{}}
	_ = s.Pool.QueryRow(ctx, `SELECT recipients, events FROM adrient.admin_alert_settings WHERE org_id::text=$1`, orgID).Scan(&a.Recipients, &a.Events)
	if a.Recipients == nil {
		a.Recipients = []string{}
	}
	if a.Events == nil {
		a.Events = []string{}
	}
	return a
}

// PutAlerts saves the alert settings after validating them.
func (s *Service) PutAlerts(ctx context.Context, orgID string, a AlertSettings) error {
	if len(a.Recipients) > 20 {
		return fmt.Errorf("%w: at most 20 recipients", ErrBadRequest)
	}
	for _, r := range a.Recipients {
		at := strings.LastIndexByte(r, '@')
		if at < 1 || !strings.Contains(r[at:], ".") || strings.ContainsAny(r, " <>\",;\r\n") {
			return fmt.Errorf("%w: %q is not an e-mail address", ErrBadRequest, r)
		}
	}
	for _, e := range a.Events {
		ok := false
		for _, k := range AlertEvents {
			ok = ok || k == e
		}
		if !ok {
			return fmt.Errorf("%w: unknown event %q", ErrBadRequest, e)
		}
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.admin_alert_settings (org_id, recipients, events) VALUES ($1,$2,$3)
ON CONFLICT (org_id) DO UPDATE SET recipients=$2, events=$3, updated_at=now()`, orgID, a.Recipients, a.Events)
	return err
}

// Alert sends an e-mail to the admins subscribed to event (best effort).
func (s *Service) Alert(ctx context.Context, orgID, event, subject, body string) { s.alert(ctx, orgID, event, subject, body) }

func (s *Service) alert(ctx context.Context, orgID, event, subject, body string) {
	if s.Mail == nil {
		return
	}
	a := s.GetAlerts(ctx, orgID)
	on := false
	for _, e := range a.Events {
		on = on || e == event
	}
	if !on || len(a.Recipients) == 0 {
		return
	}
	_ = s.Mail(ctx, a.Recipients, "[Adrient admin] "+subject, body)
}

// ---- history -----------------------------------------------------------------

// Job is one lifecycle action in the history.
type Job struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Email     string          `json:"email"`
	Kind      string          `json:"kind"`
	Status    string          `json:"status"`
	Note      string          `json:"note"`
	Result    json.RawMessage `json:"result"`
	CreatedAt time.Time       `json:"created_at"`
}

// History lists recent lifecycle actions of the org.
func (s *Service) History(ctx context.Context, orgID string, limit int) ([]Job, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `SELECT j.id::text, j.user_id::text, u.email, j.kind, j.status, j.note, j.result, j.created_at
FROM adrient.offboard_jobs j JOIN adrient.users u ON u.id=j.user_id WHERE j.org_id::text=$1 ORDER BY j.created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.UserID, &j.Email, &j.Kind, &j.Status, &j.Note, &j.Result, &j.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
