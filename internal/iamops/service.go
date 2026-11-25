package iamops

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelmgmt"
)

// StepUpWindow is how long a re-authentication stays fresh.
const StepUpWindow = 10 * time.Minute

// ActionRateLimit is the most sensitive actions one person may run in ten minutes.
const ActionRateLimit = 30

// Service holds the dependencies of the console.
type Service struct {
	Pool *pgxpool.Pool
	IdP  *zitadelmgmt.Client // nil when the identity provider is not configured

	Suspend     func(ctx context.Context, orgID, userID, actor, reason string) error
	Unsuspend   func(ctx context.Context, orgID, userID, actor string) error
	Audit       func(ctx context.Context, orgID, actorID, action string, detail map[string]any)
	Notify      func(ctx context.Context, orgID, userID, title, body, url string)
	Mail        func(ctx context.Context, to []string, subject, body string) error
	Webhook     func(ctx context.Context, url string, payload map[string]any)
	IsAdminUser func(ctx context.Context, orgID, userID string) bool // target holds an admin role
	Clock       func() time.Time
}

// Actor is the signed-in person performing an operation.
type Actor struct {
	ID         string
	Email      string
	Roles      []string
	Super      bool
	SessionKey string
}

// ActionReq is one requested operation.
type ActionReq struct {
	Action string         `json:"action"`
	Reason string         `json:"reason"`
	Ticket string         `json:"ticket"`
	DryRun bool           `json:"dry_run"`
	Params map[string]any `json:"params"`
}

// Result is what an operation returns. Secret is shown once and never stored in clear.
type Result struct {
	Status      string         `json:"status"` // done | dry_run | pending_approval
	Message     string         `json:"message"`
	Steps       []Step         `json:"steps,omitempty"`
	Secret      string         `json:"secret,omitempty"`
	SecretLabel string         `json:"secret_label,omitempty"`
	Approval    string         `json:"approval_id,omitempty"`
	Detail      map[string]any `json:"detail,omitempty"`
}

// Step is one part of a multi-step operation (the contain playbook).
type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"` // planned | done | skipped | failed
	Note   string `json:"note"`
}

type target struct {
	ID, Email, Name, Subject, Status, Type, ManagerID string
}

func (s *Service) loadTarget(ctx context.Context, orgID, userID string) (target, error) {
	var t target
	err := scanOne(s.Pool.QueryRow(ctx, `SELECT id::text, email, display_name, oidc_subject, status, user_type, COALESCE(manager_id::text,'')
FROM adrient.users WHERE org_id::text=$1 AND id::text=$2 AND deleted_at IS NULL`, orgID, userID), &t.ID, &t.Email, &t.Name, &t.Subject, &t.Status, &t.Type, &t.ManagerID)
	return t, err
}

// Roles resolves the operation roles of a person: an explicit assignment, the
// org admin role (identity admin), and the older delegated help desk / read only roles.
func (s *Service) Roles(ctx context.Context, orgID, userID string, isOrgAdmin bool) []string {
	set := map[string]bool{}
	if isOrgAdmin {
		set[RoleIdentityAdmin] = true
	}
	if rows, err := s.Pool.Query(ctx, `SELECT role FROM adrient.iam_role_assignments WHERE org_id::text=$1 AND user_id::text=$2`, orgID, userID); err == nil {
		for rows.Next() {
			var r string
			if rows.Scan(&r) == nil {
				set[r] = true
			}
		}
		rows.Close()
	}
	if rows, err := s.Pool.Query(ctx, `SELECT r.key FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id
WHERE ra.org_id::text=$1 AND ra.subject_type='user' AND ra.subject_id::text=$2 AND r.key IN ('help_desk','readonly_admin')`, orgID, userID); err == nil {
		for rows.Next() {
			var k string
			if rows.Scan(&k) == nil {
				if k == "help_desk" {
					set[RoleHelpDesk] = true
				} else {
					set[RoleAuditor] = true
				}
			}
		}
		rows.Close()
	}
	out := []string{}
	for _, r := range []string{RoleIdentityAdmin, RoleSecurityAnalyst, RoleHelpDesk, RoleAuditor} {
		if set[r] {
			out = append(out, r)
		}
	}
	return out
}

// ---- step-up --------------------------------------------------------------------

// SessionKey hashes a session token so the token itself is never stored.
func SessionKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:16])
}

// RecordStepUp stores a successful re-authentication for the session.
func (s *Service) RecordStepUp(ctx context.Context, userID, sessionKey, method string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.iam_stepups (session_key, user_id, method, verified_at) VALUES ($1,$2::uuid,$3,now())
ON CONFLICT (session_key) DO UPDATE SET method=$3, verified_at=now()`, sessionKey, userID, method)
	return err
}

// StepUpUntil returns when the current step-up expires (zero when none).
func (s *Service) StepUpUntil(ctx context.Context, sessionKey string) time.Time {
	var at time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT verified_at FROM adrient.iam_stepups WHERE session_key=$1`, sessionKey).Scan(&at); err != nil {
		return time.Time{}
	}
	if s.now().Sub(at) > StepUpWindow {
		return time.Time{}
	}
	return at.Add(StepUpWindow)
}

// VerifyStepUp checks the admin's password (and one-time code when they have one)
// at the identity provider and records the step-up.
func (s *Service) VerifyStepUp(ctx context.Context, a Actor, password, code string) error {
	if s.IdP == nil {
		return bad("the identity provider is not configured on this server")
	}
	if password == "" {
		return bad("enter your password")
	}
	if err := s.IdP.VerifyCredentials(ctx, a.Email, password, code); err != nil {
		return bad("the password or code was not accepted")
	}
	method := "password"
	if code != "" {
		method = "password+otp"
	}
	return s.RecordStepUp(ctx, a.ID, a.SessionKey, method)
}

// ---- action pipeline -------------------------------------------------------------

func randomPassword() string {
	const lower, upper, digit, sym = "abcdefghjkmnpqrstuvwxyz", "ABCDEFGHJKLMNPQRSTUVWXYZ", "23456789", "-_#!%"
	all := lower + upper + digit
	pick := func(set string) byte {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		return set[n.Int64()]
	}
	b := []byte{pick(lower), pick(upper), pick(digit), pick(sym)}
	for len(b) < 16 {
		b = append(b, pick(all))
	}
	for i := len(b) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		b[i], b[j.Int64()] = b[j.Int64()], b[i]
	}
	return string(b)
}

func hashCode(c string) string {
	h := sha256.Sum256([]byte(c))
	return hex.EncodeToString(h[:])
}

func (s *Service) isLastAdmin(ctx context.Context, orgID, userID string) bool {
	if s.IsAdminUser == nil || !s.IsAdminUser(ctx, orgID, userID) {
		return false
	}
	rows, err := s.Pool.Query(ctx, `SELECT u.id::text FROM adrient.org_admins oa JOIN adrient.users u ON u.id=oa.user_id
WHERE oa.org_id::text=$1 AND u.status='active' AND u.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM adrient.iam_restrictions r WHERE r.org_id=oa.org_id AND r.user_id=u.id AND r.kind='quarantine')`, orgID)
	if err != nil {
		return true
	}
	defer rows.Close()
	others := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != userID {
			others++
		}
	}
	return others == 0
}

// Do runs one operation through the whole safety pipeline.
func (s *Service) Do(ctx context.Context, orgID string, a Actor, targetID string, req ActionReq) (Result, error) {
	return s.do(ctx, orgID, a, targetID, req, false)
}

func (s *Service) do(ctx context.Context, orgID string, a Actor, targetID string, req ActionReq, approved bool) (Result, error) {
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	cp := CapFor(req.Action)
	if cp == "" {
		return Result{}, bad("unknown action %q", req.Action)
	}
	if !Has(a.Roles, cp) {
		return Result{}, fmt.Errorf("%w: your role cannot do this", ErrForbidden)
	}
	t, err := s.loadTarget(ctx, orgID, targetID)
	if err != nil {
		return Result{}, err
	}
	req.Reason = strings.TrimSpace(req.Reason)
	req.Ticket = strings.TrimSpace(req.Ticket)
	if !req.DryRun && len(req.Reason) < 3 {
		return Result{}, bad("a reason is required")
	}
	if len(req.Reason) > 500 || len(req.Ticket) > 100 {
		return Result{}, bad("reason or ticket is too long")
	}
	self := t.ID == a.ID
	targetAdmin := s.IsAdminUser != nil && s.IsAdminUser(ctx, orgID, t.ID)
	if !CanTarget(a.Roles, a.Super, targetAdmin, self) {
		return Result{}, fmt.Errorf("%w: only a super admin can act on another administrator", ErrForbidden)
	}
	if self && (lockout[req.Action] || strings.HasPrefix(req.Action, "mfa.") || req.Action == "password.temp") {
		return Result{}, bad("you cannot do this to your own account; ask another administrator")
	}
	if lockout[req.Action] && !req.DryRun && s.isLastAdmin(ctx, orgID, t.ID) {
		return Result{}, bad("this is the last active administrator. Add another administrator first, or use the break-glass steps in the runbook")
	}
	if t.Type == "service" && req.Action != "sessions.revoke_all" && req.Action != "tokens.revoke" && req.Action != "tokens.revoke_all" {
		return Result{}, bad("service accounts have no password or second step")
	}
	if !req.DryRun && NeedsStepUp(req.Action) && s.StepUpUntil(ctx, a.SessionKey).IsZero() {
		return Result{}, ErrStepUp
	}
	if !req.DryRun {
		var n int
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.iam_actions WHERE org_id::text=$1 AND actor_id::text=$2 AND status IN ('done','pending_approval') AND created_at > now() - interval '10 minutes'`, orgID, a.ID).Scan(&n)
		if n >= ActionRateLimit {
			return Result{}, fmt.Errorf("%w: too many actions in ten minutes, wait a moment", ErrForbidden)
		}
		st := s.GetSettings(ctx, orgID)
		needApproval := false
		for _, x := range st.RequireApproval {
			needApproval = needApproval || x == req.Action
		}
		if needApproval && !approved {
			return s.requestApproval(ctx, orgID, a, t, req)
		}
	}
	res, err := s.execute(ctx, orgID, a, t, req)
	status := "done"
	if err != nil {
		status = "failed"
	}
	if req.DryRun {
		status = "dry_run"
	}
	detail := map[string]any{"params": scrub(req.Params)}
	if err != nil {
		detail["error"] = err.Error()
	}
	if len(res.Steps) > 0 {
		detail["steps"] = res.Steps
	}
	dj, _ := json.Marshal(detail)
	if !req.DryRun || err != nil {
		_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.iam_actions (org_id, actor_id, actor_role, target_id, target_email, action, reason, ticket, status, detail)
VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7,$8,$9,$10)`, orgID, a.ID, strings.Join(a.Roles, ","), t.ID, t.Email, req.Action, req.Reason, req.Ticket, status, dj)
	}
	if err != nil {
		return res, err
	}
	if !req.DryRun {
		s.audit(ctx, orgID, a.ID, req.Action, map[string]any{"target": t.ID, "target_email": t.Email, "reason": req.Reason, "ticket": req.Ticket})
		s.tell(ctx, orgID, a, t, req, res)
	}
	return res, nil
}

func scrub(p map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range p {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") || strings.Contains(lk, "secret") || strings.Contains(lk, "code") {
			continue
		}
		out[k] = v
	}
	return out
}

var userNotices = map[string]string{
	"password.reset_link":  "An administrator sent a password reset link to your e-mail address.",
	"password.temp":        "An administrator set a temporary password on your account. You must change it when you sign in.",
	"password.expire":      "Your password was expired by an administrator. Use the reset link we e-mailed you to set a new one.",
	"mfa.revoke":           "An administrator removed one of your second sign-in steps.",
	"mfa.reset_all":        "An administrator reset all your second sign-in steps. You will be asked to set them up again.",
	"mfa.require_reenroll": "You must set up your second sign-in step again at your next sign-in.",
	"mfa.bypass":           "An administrator granted you a short, temporary exception for the second sign-in step.",
	"sessions.revoke_all":  "You were signed out of all devices by an administrator.",
	"account.suspend":      "Your account was suspended by an administrator.",
	"account.quarantine":   "Your account was set to read-only by an administrator while a security check runs.",
	"account.email":        "An administrator started changing your e-mail address. Confirm it with the code sent to the new address.",
	"account.phone":        "An administrator started changing your phone number. Confirm it with the code sent by text.",
	"contain":              "Your account was secured after suspicious activity: you were signed out everywhere, your password was reset and your second steps were removed. Check your e-mail for the reset link.",
}

// tell notifies the person (and the manager or the security mailbox when configured).
func (s *Service) tell(ctx context.Context, orgID string, a Actor, t target, req ActionReq, res Result) {
	msg, ok := userNotices[req.Action]
	if !ok {
		return
	}
	if s.Notify != nil {
		s.Notify(ctx, orgID, t.ID, "Security change on your account", msg, "/security")
	}
	st := s.GetSettings(ctx, orgID)
	if s.Mail != nil && t.Email != "" {
		_ = s.Mail(ctx, []string{t.Email}, "Security change on your account", msg+"\n\nIf you did not expect this, contact your IT team.")
	}
	if st.NotifyManager && t.ManagerID != "" && s.Notify != nil {
		s.Notify(ctx, orgID, t.ManagerID, "Security change for "+t.Name, req.Action+" was applied to "+t.Email+".", "")
	}
	if s.Mail != nil && st.SecurityMailbox != "" {
		_ = s.Mail(ctx, []string{st.SecurityMailbox}, "[Identity action] "+req.Action+" on "+t.Email,
			fmt.Sprintf("%s did %s on %s.\nReason: %s\nTicket: %s", a.Email, req.Action, t.Email, req.Reason, req.Ticket))
	}
}

func (s *Service) requestApproval(ctx context.Context, orgID string, a Actor, t target, req ActionReq) (Result, error) {
	p, _ := json.Marshal(scrub(req.Params))
	var id string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.iam_approvals (org_id, action, target_id, requester_id, reason, ticket, payload)
VALUES ($1::uuid,$2,$3::uuid,$4::uuid,$5,$6,$7) RETURNING id::text`, orgID, req.Action, t.ID, a.ID, req.Reason, req.Ticket, p).Scan(&id); err != nil {
		return Result{}, err
	}
	_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.iam_actions (org_id, actor_id, actor_role, target_id, target_email, action, reason, ticket, status, detail)
VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7,$8,'pending_approval',$9)`, orgID, a.ID, strings.Join(a.Roles, ","), t.ID, t.Email, req.Action, req.Reason, req.Ticket, p)
	if s.Notify != nil {
		for _, uid := range s.recipients(ctx, orgID) {
			if uid != a.ID {
				s.Notify(ctx, orgID, uid, "Approval needed: "+req.Action, a.Email+" asks to run "+req.Action+" on "+t.Email+". Reason: "+req.Reason, "/admin/secops?tab=approvals")
			}
		}
	}
	return Result{Status: "pending_approval", Approval: id, Message: "A second administrator must approve this before it runs."}, nil
}

// Decide approves or rejects a pending approval. The approver must be a different person.
func (s *Service) Decide(ctx context.Context, orgID string, a Actor, id string, approve bool) (Result, error) {
	if !Has(a.Roles, CapApprove) {
		return Result{}, fmt.Errorf("%w: your role cannot approve", ErrForbidden)
	}
	var action, target, requester, reason, ticket, status string
	var payload []byte
	var exp time.Time
	if err := scanOne(s.Pool.QueryRow(ctx, `SELECT action, target_id::text, requester_id::text, reason, ticket, payload, status, expires_at FROM adrient.iam_approvals WHERE org_id::text=$1 AND id::text=$2`, orgID, id),
		&action, &target, &requester, &reason, &ticket, &payload, &status, &exp); err != nil {
		return Result{}, err
	}
	if status != "pending" {
		return Result{}, bad("this request was already %s", status)
	}
	if requester == a.ID {
		return Result{}, fmt.Errorf("%w: a different administrator must approve your request", ErrForbidden)
	}
	if s.now().After(exp) {
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.iam_approvals SET status='expired' WHERE id::text=$1`, id)
		return Result{}, bad("this request expired")
	}
	if !approve {
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.iam_approvals SET status='rejected', approver_id=$2::uuid, decided_at=now() WHERE id::text=$1`, id, a.ID)
		s.audit(ctx, orgID, a.ID, "approval.reject", map[string]any{"approval": id, "action": action})
		return Result{Status: "done", Message: "Rejected."}, nil
	}
	params := map[string]any{}
	_ = json.Unmarshal(payload, &params)
	// Run as the approver with the requester recorded in the reason.
	res, err := s.do(ctx, orgID, a, target, ActionReq{Action: action, Reason: reason + " (requested by " + requester + ", approved by " + a.Email + ")", Ticket: ticket, Params: params}, true)
	if err != nil {
		return res, err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.iam_approvals SET status='executed', approver_id=$2::uuid, decided_at=now() WHERE id::text=$1`, id, a.ID)
	s.audit(ctx, orgID, a.ID, "approval.approve", map[string]any{"approval": id, "action": action, "requester": requester})
	return res, nil
}

func (s *Service) revokeAdrientSessions(ctx context.Context, userID string) int {
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.sessions SET revoked_at=now() WHERE user_id::text=$1 AND revoked_at IS NULL AND expires_at > now()`, userID)
	if err != nil {
		return 0
	}
	return int(tag.RowsAffected())
}

func (s *Service) needIdP() error {
	if s.IdP == nil {
		return bad("the identity provider is not configured on this server")
	}
	return nil
}

func (s *Service) execute(ctx context.Context, orgID string, a Actor, t target, req ActionReq) (Result, error) {
	str := func(k string) string { v, _ := req.Params[k].(string); return strings.TrimSpace(v) }
	switch req.Action {
	case "password.reset_link":
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would e-mail " + t.Email + " a password reset link."}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		if err := s.IdP.SendPasswordResetLink(ctx, t.Subject, ""); err != nil {
			return Result{}, idpErr(err)
		}
		return Result{Status: "done", Message: "Reset link sent to " + t.Email + "."}, nil
	case "password.temp":
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would set a one-time temporary password, force a change at the next sign-in and sign " + t.Email + " out everywhere."}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		pw := randomPassword()
		if err := s.IdP.SetPassword(ctx, t.Subject, pw, true); err != nil {
			return Result{}, idpErr(err)
		}
		n := s.revokeAdrientSessions(ctx, t.ID)
		return Result{Status: "done", Message: fmt.Sprintf("Temporary password set; %d session(s) ended. It is shown once.", n), Secret: pw, SecretLabel: "Temporary password"}, nil
	case "password.expire":
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would invalidate the current password, sign the person out and e-mail a reset link."}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		if err := s.IdP.SetPassword(ctx, t.Subject, randomPassword(), true); err != nil {
			return Result{}, idpErr(err)
		}
		n := s.revokeAdrientSessions(ctx, t.ID)
		if err := s.IdP.SendPasswordResetLink(ctx, t.Subject, ""); err != nil {
			return Result{Status: "done", Message: fmt.Sprintf("Password expired and %d session(s) ended, but the reset link could not be sent: %v", n, idpErr(err))}, nil
		}
		return Result{Status: "done", Message: fmt.Sprintf("Password expired, %d session(s) ended and a reset link was e-mailed.", n)}, nil
	case "account.unlock":
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would unlock the account at the identity provider."}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		state, _ := s.IdP.UserState(ctx, t.Subject)
		if state == "LOCKED" {
			if err := s.IdP.SetUserState(ctx, t.Subject, "unlock"); err != nil {
				return Result{}, idpErr(err)
			}
			return Result{Status: "done", Message: "Account unlocked."}, nil
		}
		return Result{Status: "done", Message: "The account was not locked (state " + strings.ToLower(state) + "). Nothing to do."}, nil
	case "mfa.revoke":
		fid, kind := str("factor_id"), str("kind")
		if kind == "" {
			return Result{}, bad("choose which factor to remove")
		}
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would remove the " + kind + " factor."}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		if err := s.IdP.RemoveFactor(ctx, t.Subject, zitadelmgmt.Factor{ID: fid, Kind: kind}); err != nil {
			return Result{}, idpErr(err)
		}
		return Result{Status: "done", Message: "Factor removed."}, nil
	case "mfa.reset_all", "mfa.require_reenroll":
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		fs, err := s.IdP.ListFactors(ctx, t.Subject)
		if err != nil {
			return Result{}, idpErr(err)
		}
		if req.DryRun {
			return Result{Status: "dry_run", Message: fmt.Sprintf("Would remove %d factor(s).", len(fs)), Detail: map[string]any{"factors": fs}}, nil
		}
		n, err := s.IdP.RemoveAllFactors(ctx, t.Subject)
		if err != nil {
			return Result{}, idpErr(err)
		}
		msg := fmt.Sprintf("%d factor(s) removed. The person sets up a new one at the next sign-in.", n)
		if s.GetSettings(ctx, orgID).MFAPolicy.Mode != "all" {
			msg += " The organisation policy does not force a second step for everyone, so enrolment is prompted only where the identity provider's login policy requires it."
		}
		return Result{Status: "done", Message: msg}, nil
	case "mfa.bypass":
		hours := 4
		if h, ok := req.Params["hours"].(float64); ok {
			hours = int(h)
		}
		if hours < 1 || hours > 24 {
			return Result{}, bad("a bypass lasts between 1 and 24 hours")
		}
		if req.DryRun {
			return Result{Status: "dry_run", Message: fmt.Sprintf("Would remove the second steps for %d hour(s) and issue a one-time verification code.", hours)}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		if _, err := s.IdP.RemoveAllFactors(ctx, t.Subject); err != nil {
			return Result{}, idpErr(err)
		}
		code := fmt.Sprintf("%s-%s", randDigits(4), randDigits(4))
		_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.iam_recovery_codes (org_id, user_id, code_hash, kind, created_by, reason, expires_at)
VALUES ($1::uuid,$2::uuid,$3,'bypass',$4::uuid,$5,$6)`, orgID, t.ID, hashCode(code), a.ID, req.Reason, s.now().Add(time.Duration(hours)*time.Hour))
		if err != nil {
			return Result{}, err
		}
		return Result{Status: "done", Message: fmt.Sprintf("Second steps removed for a %d hour window. Read the code to the person to confirm who they are; the person must enrol again within the window.", hours),
			Secret: code, SecretLabel: "One-time verification code"}, nil
	case "sessions.revoke":
		sid := str("session_id")
		if sid == "" {
			return Result{}, bad("choose a session")
		}
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would end this session."}, nil
		}
		tag, err := s.Pool.Exec(ctx, `UPDATE adrient.sessions SET revoked_at=now() WHERE user_id::text=$1 AND public_id=$2 AND revoked_at IS NULL`, t.ID, sid)
		if err != nil {
			return Result{}, err
		}
		return Result{Status: "done", Message: fmt.Sprintf("%d session ended.", tag.RowsAffected())}, nil
	case "sessions.revoke_all":
		var n int
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.sessions WHERE user_id::text=$1 AND revoked_at IS NULL AND expires_at > now()`, t.ID).Scan(&n)
		if req.DryRun {
			return Result{Status: "dry_run", Message: fmt.Sprintf("Would end %d active session(s) and the identity provider's sessions.", n)}, nil
		}
		n = s.revokeAdrientSessions(ctx, t.ID)
		idp := 0
		if s.IdP != nil {
			idp, _ = s.IdP.TerminateSessions(ctx, t.Subject)
		}
		return Result{Status: "done", Message: fmt.Sprintf("Signed out everywhere: %d app session(s) and %d identity provider session(s) ended.", n, idp)}, nil
	case "tokens.revoke", "tokens.revoke_all":
		var tag int64
		if req.DryRun {
			_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.api_tokens WHERE user_id::text=$1 AND revoked_at IS NULL`, t.ID).Scan(&tag)
			return Result{Status: "dry_run", Message: fmt.Sprintf("Would revoke %d token(s).", tag)}, nil
		}
		q, args := `UPDATE adrient.api_tokens SET revoked_at=now() WHERE user_id::text=$1 AND revoked_at IS NULL`, []any{t.ID}
		if req.Action == "tokens.revoke" {
			if str("token_id") == "" {
				return Result{}, bad("choose a token")
			}
			q += ` AND id::text=$2`
			args = append(args, str("token_id"))
		}
		ct, err := s.Pool.Exec(ctx, q, args...)
		if err != nil {
			return Result{}, err
		}
		return Result{Status: "done", Message: fmt.Sprintf("%d API token(s) revoked.", ct.RowsAffected())}, nil
	case "account.suspend":
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would block sign-in and end every session."}, nil
		}
		if s.Suspend == nil {
			return Result{}, bad("suspending is not available on this server")
		}
		if err := s.Suspend(ctx, orgID, t.ID, a.ID, req.Reason); err != nil {
			return Result{}, err
		}
		return Result{Status: "done", Message: "Account suspended."}, nil
	case "account.unsuspend":
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would let the person sign in again."}, nil
		}
		if s.Unsuspend == nil {
			return Result{}, bad("this is not available on this server")
		}
		if err := s.Unsuspend(ctx, orgID, t.ID, a.ID); err != nil {
			return Result{}, err
		}
		return Result{Status: "done", Message: "Account active again."}, nil
	case "account.quarantine":
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would make the account read-only until released."}, nil
		}
		var exp *time.Time
		if h, ok := req.Params["hours"].(float64); ok && h > 0 {
			e := s.now().Add(time.Duration(h) * time.Hour)
			exp = &e
		}
		if _, err := s.Pool.Exec(ctx, `INSERT INTO adrient.iam_restrictions (org_id, user_id, kind, reason, expires_at, created_by) VALUES ($1::uuid,$2::uuid,'quarantine',$3,$4,$5::uuid)
ON CONFLICT (org_id, user_id, kind) DO UPDATE SET reason=$3, expires_at=$4, created_by=$5::uuid, created_at=now()`, orgID, t.ID, req.Reason, exp, a.ID); err != nil {
			return Result{}, err
		}
		return Result{Status: "done", Message: "Account is read-only: the person can sign in and read, but cannot create, change or delete anything."}, nil
	case "account.unquarantine":
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would release the read-only mode."}, nil
		}
		_, err := s.Pool.Exec(ctx, `DELETE FROM adrient.iam_restrictions WHERE org_id::text=$1 AND user_id::text=$2 AND kind='quarantine'`, orgID, t.ID)
		return Result{Status: "done", Message: "Read-only mode released."}, err
	case "account.email":
		e := strings.ToLower(str("email"))
		if !strings.Contains(e, "@") || len(e) > 200 {
			return Result{}, bad("enter a valid e-mail address")
		}
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would send a verification code to " + e + "."}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		if err := s.IdP.SetEmail(ctx, t.Subject, e, false); err != nil {
			return Result{}, idpErr(err)
		}
		return Result{Status: "done", Message: "A verification code was sent to " + e + ". The change takes effect once the person confirms it."}, nil
	case "account.phone":
		p := str("phone")
		if len(p) < 6 || len(p) > 24 || !strings.HasPrefix(p, "+") {
			return Result{}, bad("enter the phone number with country code, like +91...")
		}
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would send a verification code by text."}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		if err := s.IdP.SetPhone(ctx, t.Subject, p); err != nil {
			return Result{}, idpErr(err)
		}
		return Result{Status: "done", Message: "A verification code was sent by text."}, nil
	case "account.expiry":
		days, ok := req.Params["days"].(float64)
		clear := str("clear") == "yes" || (ok && days == 0)
		if !clear && (!ok || days < 1 || days > 3650) {
			return Result{}, bad("enter the number of days, or clear the expiry")
		}
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would set the account expiry."}, nil
		}
		if clear {
			_, err := s.Pool.Exec(ctx, `UPDATE adrient.users SET expires_at=NULL, updated_at=now() WHERE org_id::text=$1 AND id::text=$2`, orgID, t.ID)
			return Result{Status: "done", Message: "Expiry cleared."}, err
		}
		exp := s.now().Add(time.Duration(days) * 24 * time.Hour)
		_, err := s.Pool.Exec(ctx, `UPDATE adrient.users SET expires_at=$3, updated_at=now() WHERE org_id::text=$1 AND id::text=$2`, orgID, t.ID, exp)
		return Result{Status: "done", Message: "Access ends on " + exp.Format("2 Jan 2006") + "."}, err
	case "account.unlink_idp":
		idp, lu := str("idp_id"), str("linked_user_id")
		if idp == "" || lu == "" {
			return Result{}, bad("choose a linked provider")
		}
		if req.DryRun {
			return Result{Status: "dry_run", Message: "Would unlink the external provider."}, nil
		}
		if err := s.needIdP(); err != nil {
			return Result{}, err
		}
		if err := s.IdP.RemoveIdPLink(ctx, t.Subject, idp, lu); err != nil {
			return Result{}, idpErr(err)
		}
		return Result{Status: "done", Message: "External provider unlinked."}, nil
	case "contain":
		return s.contain(ctx, orgID, a, t, req)
	}
	return Result{}, bad("unknown action")
}

func randDigits(n int) string {
	b := make([]byte, n)
	for i := range b {
		v, _ := rand.Int(rand.Reader, big.NewInt(10))
		b[i] = byte('0') + byte(v.Int64())
	}
	return string(b)
}

func idpErr(err error) error {
	if err == nil {
		return nil
	}
	if ze, ok := err.(*zitadelmgmt.Error); ok {
		return fmt.Errorf("the identity provider refused this (%d): %s", ze.Status, ze.Message)
	}
	return fmt.Errorf("could not reach the identity provider: %w", err)
}

// contain is the idempotent playbook: end sessions, reset the password, remove
// second steps, notify. Every step checks its own state, so running it twice is safe.
func (s *Service) contain(ctx context.Context, orgID string, a Actor, t target, req ActionReq) (Result, error) {
	steps := []Step{}
	var activeSessions int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.sessions WHERE user_id::text=$1 AND revoked_at IS NULL AND expires_at > now()`, t.ID).Scan(&activeSessions)
	var fs []zitadelmgmt.Factor
	if s.IdP != nil {
		fs, _ = s.IdP.ListFactors(ctx, t.Subject)
	}
	// The password step is skipped when this account was already contained after its last successful sign-in.
	var already bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM adrient.iam_actions ia WHERE ia.org_id::text=$1 AND ia.target_id::text=$2 AND ia.action='contain' AND ia.status='done'
  AND ia.created_at > COALESCE((SELECT max(created_at) FROM adrient.login_events le WHERE le.user_id::text=$2 AND le.success), '-infinity'))`, orgID, t.ID).Scan(&already)
	pwNote := "Set an unknown password and e-mail a reset link"
	if already {
		pwNote = "Already contained since the last sign-in; password left as it is"
	}
	if req.DryRun {
		steps = append(steps,
			Step{"End sessions", "planned", fmt.Sprintf("%d active session(s) and the identity provider's sessions", activeSessions)},
			Step{"Reset password", "planned", pwNote},
			Step{"Remove second steps", "planned", fmt.Sprintf("%d factor(s) to remove so the person enrols again", len(fs))},
			Step{"Notify the person", "planned", "In-app notice and e-mail to " + t.Email})
		return Result{Status: "dry_run", Message: "Nothing was changed. This is what would happen.", Steps: steps}, nil
	}
	if err := s.needIdP(); err != nil {
		return Result{}, err
	}
	n := s.revokeAdrientSessions(ctx, t.ID)
	idp, _ := s.IdP.TerminateSessions(ctx, t.Subject)
	steps = append(steps, Step{"End sessions", "done", fmt.Sprintf("%d app session(s), %d identity provider session(s)", n, idp)})
	if already {
		steps = append(steps, Step{"Reset password", "skipped", pwNote})
	} else if err := s.IdP.SetPassword(ctx, t.Subject, randomPassword(), true); err != nil {
		steps = append(steps, Step{"Reset password", "failed", err.Error()})
	} else {
		note := "Unknown password set; change required"
		if err := s.IdP.SendPasswordResetLink(ctx, t.Subject, ""); err != nil {
			note += "; reset link could not be sent: " + err.Error()
		} else {
			note += "; reset link e-mailed"
		}
		steps = append(steps, Step{"Reset password", "done", note})
	}
	if c, err := s.IdP.RemoveAllFactors(ctx, t.Subject); err != nil {
		steps = append(steps, Step{"Remove second steps", "failed", err.Error()})
	} else if c == 0 {
		steps = append(steps, Step{"Remove second steps", "skipped", "No factors were enrolled"})
	} else {
		steps = append(steps, Step{"Remove second steps", "done", fmt.Sprintf("%d removed; the person enrols again", c)})
	}
	steps = append(steps, Step{"Notify the person", "done", "Notice and e-mail sent"})
	failed := 0
	for _, st := range steps {
		if st.Status == "failed" {
			failed++
		}
	}
	if aid, ok := req.Params["alert_id"].(string); ok && aid != "" && failed == 0 {
		_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.iam_alert_comments (alert_id, org_id, author_id, kind, body) SELECT id, org_id, $3::uuid, 'action', $4 FROM adrient.iam_alerts WHERE org_id::text=$1 AND id::text=$2`,
			orgID, aid, a.ID, "Contained the account: "+req.Reason)
	}
	if failed > 0 {
		return Result{Status: "done", Message: fmt.Sprintf("Contained with %d step(s) failed. Run it again to retry the failed ones.", failed), Steps: steps}, nil
	}
	return Result{Status: "done", Message: "Account contained.", Steps: steps}, nil
}
