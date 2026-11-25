package iamops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrBad is a request the caller must fix (shown to the person).
var ErrBad = errors.New("invalid request")

// ErrForbidden is a permission failure.
var ErrForbidden = errors.New("not allowed")

// ErrStepUp means a fresh re-authentication is needed first.
var ErrStepUp = errors.New("step-up required")

// ErrNotFound is a missing record.
var ErrNotFound = errors.New("not found")

func bad(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrBad, fmt.Sprintf(format, a...))
}

// Settings is the org's identity-operations configuration.
type Settings struct {
	Thresholds      Thresholds `json:"thresholds"`
	AnonNetworks    []string   `json:"anon_networks"`
	SecurityMailbox string     `json:"security_mailbox"`
	WebhookURL      string     `json:"webhook_url"`
	NotifyManager   bool       `json:"notify_manager"`
	MFAPolicy       MFAPolicy  `json:"mfa_policy"`
	RequireApproval []string   `json:"require_approval"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// MFAPolicy is the org's second-step policy. Mode "all" is enforced by the
// identity provider's login policy; "admins" and "risky" are monitored here and
// raise alerts and report lines, they do not block a sign-in by themselves.
type MFAPolicy struct {
	Mode string `json:"mode"` // off | admins | all | risky
}

// GetSettings loads the org settings with defaults applied.
func (s *Service) GetSettings(ctx context.Context, orgID string) Settings {
	st := Settings{Thresholds: DefaultThresholds(), AnonNetworks: []string{}, RequireApproval: []string{}, MFAPolicy: MFAPolicy{Mode: "off"}}
	var th, mfa []byte
	var upd time.Time
	err := s.Pool.QueryRow(ctx, `SELECT thresholds, anon_networks, security_mailbox, webhook_url, notify_manager, mfa_policy, require_approval, updated_at
FROM adrient.iam_settings WHERE org_id::text=$1`, orgID).Scan(&th, &st.AnonNetworks, &st.SecurityMailbox, &st.WebhookURL, &st.NotifyManager, &mfa, &st.RequireApproval, &upd)
	if err != nil {
		return st
	}
	t := Thresholds{}
	_ = json.Unmarshal(th, &t)
	if len(th) > 2 {
		st.Thresholds = t.Normalize()
		// booleans default to true only when the org never saved settings
	}
	_ = json.Unmarshal(mfa, &st.MFAPolicy)
	if st.MFAPolicy.Mode == "" {
		st.MFAPolicy.Mode = "off"
	}
	if st.AnonNetworks == nil {
		st.AnonNetworks = []string{}
	}
	if st.RequireApproval == nil {
		st.RequireApproval = []string{}
	}
	st.UpdatedAt = &upd
	return st
}

var approvable = map[string]bool{"mfa.bypass": true, "password.temp": true, "mfa.reset_all": true, "contain": true, "account.suspend": true}

// PutSettings validates and stores the org settings.
func (s *Service) PutSettings(ctx context.Context, orgID, actor string, st Settings) (Settings, error) {
	st.Thresholds = st.Thresholds.Normalize()
	clean := []string{}
	for _, n := range st.AnonNetworks {
		if strings.TrimSpace(n) == "" {
			continue
		}
		v, err := ValidateNetworkEntry(n)
		if err != nil {
			return st, bad("%v", err)
		}
		clean = append(clean, v)
	}
	st.AnonNetworks = clean
	if st.SecurityMailbox != "" && !strings.Contains(st.SecurityMailbox, "@") {
		return st, bad("the security mailbox must be an e-mail address")
	}
	if st.WebhookURL != "" && !strings.HasPrefix(st.WebhookURL, "https://") && !strings.HasPrefix(st.WebhookURL, "http://") {
		return st, bad("the webhook address must start with http:// or https://")
	}
	switch st.MFAPolicy.Mode {
	case "off", "admins", "all", "risky":
	case "":
		st.MFAPolicy.Mode = "off"
	default:
		return st, bad("unknown second-step policy")
	}
	ra := []string{}
	for _, a := range st.RequireApproval {
		if !approvable[a] {
			return st, bad("%q cannot require a second approver", a)
		}
		ra = append(ra, a)
	}
	st.RequireApproval = ra
	th, _ := json.Marshal(st.Thresholds)
	mfa, _ := json.Marshal(st.MFAPolicy)
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.iam_settings (org_id, thresholds, anon_networks, security_mailbox, webhook_url, notify_manager, mfa_policy, require_approval)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (org_id) DO UPDATE SET thresholds=$2, anon_networks=$3, security_mailbox=$4, webhook_url=$5, notify_manager=$6, mfa_policy=$7, require_approval=$8, updated_at=now()`,
		orgID, th, st.AnonNetworks, st.SecurityMailbox, st.WebhookURL, st.NotifyManager, mfa, st.RequireApproval)
	if err != nil {
		return st, err
	}
	s.audit(ctx, orgID, actor, "iam.settings", map[string]any{"mfa_mode": st.MFAPolicy.Mode, "thresholds": st.Thresholds})
	return s.GetSettings(ctx, orgID), nil
}

// ---- events -----------------------------------------------------------------

// DeviceFingerprint derives a stable short id from a user-agent string.
func DeviceFingerprint(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return ""
	}
	h := sha256.Sum256([]byte(browserFamily(ua) + "|" + osFamily(ua)))
	return hex.EncodeToString(h[:6])
}

func browserFamily(ua string) string {
	l := strings.ToLower(ua)
	switch {
	case strings.Contains(l, "edg/"):
		return "Edge"
	case strings.Contains(l, "firefox"):
		return "Firefox"
	case strings.Contains(l, "chrome") || strings.Contains(l, "chromium"):
		return "Chrome"
	case strings.Contains(l, "safari"):
		return "Safari"
	case strings.Contains(l, "curl"):
		return "curl"
	}
	return "Other"
}

func osFamily(ua string) string {
	l := strings.ToLower(ua)
	switch {
	case strings.Contains(l, "android"):
		return "Android"
	case strings.Contains(l, "iphone") || strings.Contains(l, "ipad"):
		return "iOS"
	case strings.Contains(l, "windows"):
		return "Windows"
	case strings.Contains(l, "mac os") || strings.Contains(l, "macintosh"):
		return "macOS"
	case strings.Contains(l, "linux"):
		return "Linux"
	}
	return "Other"
}

// DeviceLabel is the human text used in notices ("Chrome on Windows").
func DeviceLabel(ua string) string {
	if strings.TrimSpace(ua) == "" {
		return "an unknown device"
	}
	return browserFamily(ua) + " on " + osFamily(ua)
}

// InsertEvent stores an event (used by the ingest API and tests) and evaluates it.
func (s *Service) InsertEvent(ctx context.Context, orgID string, ev Event) (int64, []Finding, error) {
	ev.Country = strings.ToUpper(strings.TrimSpace(ev.Country))
	if ev.DeviceFP == "" {
		ev.DeviceFP = DeviceFingerprint(ev.UserAgent)
	}
	if ev.At.IsZero() {
		ev.At = s.now()
	}
	if ev.UserID == "" && ev.Email != "" {
		_ = s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.users WHERE org_id::text=$1 AND lower(email)=lower($2) AND deleted_at IS NULL LIMIT 1`, orgID, ev.Email).Scan(&ev.UserID)
	}
	if len(ev.UserAgent) > 300 {
		ev.UserAgent = ev.UserAgent[:300]
	}
	var id int64
	err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.login_events (org_id, user_id, email, ip, user_agent, method, success, reason, country, city, device_fp, mfa_result, source, created_at)
VALUES ($1::uuid, NULLIF($2,'')::uuid, $3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'ingest',$13) RETURNING id`,
		orgID, ev.UserID, ev.Email, ev.IP, ev.UserAgent, ev.Method, ev.Success, ev.Reason, ev.Country, ev.City, ev.DeviceFP, ev.MFAResult, ev.At).Scan(&id)
	if err != nil {
		return 0, nil, err
	}
	ev.ID, ev.OrgID = id, orgID
	f := s.evaluate(ctx, orgID, ev)
	return id, f, nil
}

// facts reads the history around an event.
func (s *Service) facts(ctx context.Context, orgID string, ev Event, st Settings) Facts {
	th := st.Thresholds
	f := Facts{PriorCountries: map[string]bool{}, PriorDevices: map[string]bool{}, Anonymizers: st.AnonNetworks}
	if ev.UserID != "" {
		var status string
		if err := s.Pool.QueryRow(ctx, `SELECT status FROM adrient.users WHERE org_id::text=$1 AND id::text=$2`, orgID, ev.UserID).Scan(&status); err == nil {
			f.UserKnown = true
			f.AccountDisabled = status != "active"
		}
		if s.IsAdminUser != nil {
			f.IsAdmin = s.IsAdminUser(ctx, orgID, ev.UserID)
		}
		rows, err := s.Pool.Query(ctx, `SELECT country, city, device_fp, created_at FROM adrient.login_events
WHERE org_id::text=$1 AND user_id::text=$2 AND success AND id<$3 AND created_at > now() - interval '90 days' ORDER BY id DESC LIMIT 500`, orgID, ev.UserID, ev.ID)
		if err == nil {
			for rows.Next() {
				var c, city, fp string
				var at time.Time
				if rows.Scan(&c, &city, &fp, &at) != nil {
					continue
				}
				f.PriorSuccesses++
				if c != "" {
					f.PriorCountries[strings.ToUpper(c)] = true
				}
				if fp != "" {
					f.PriorDevices[fp] = true
				}
				if f.LastSuccessAt.IsZero() {
					f.LastSuccessAt, f.LastSuccessCtry, f.LastSuccessCity = at, strings.ToUpper(c), city
				}
			}
			rows.Close()
		}
	}
	win := fmt.Sprintf("%d minutes", th.WindowMinutes)
	if ev.Email != "" {
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.login_events WHERE org_id::text=$1 AND lower(email)=lower($2) AND NOT success AND id<=$3 AND created_at > $4::timestamptz - $5::interval`,
			orgID, ev.Email, ev.ID, ev.At, win).Scan(&f.FailsAccount)
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.login_events WHERE org_id::text=$1 AND lower(email)=lower($2) AND mfa_result IN ('denied','failed') AND id<=$3 AND created_at > $4::timestamptz - $5::interval`,
			orgID, ev.Email, ev.ID, ev.At, win).Scan(&f.MFADenied)
	}
	if ev.IP != "" {
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE NOT success), count(DISTINCT lower(email)) FROM adrient.login_events
WHERE org_id::text=$1 AND ip=$2 AND id<=$3 AND created_at > $4::timestamptz - $5::interval`, orgID, ev.IP, ev.ID, ev.At, win).Scan(&f.FailsIP, &f.AccountsFromIP)
		_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.honeypot_alerts WHERE ip=$1 AND created_at > now() - interval '24 hours'`, ev.IP).Scan(&f.HoneypotHits)
	}
	if strings.HasPrefix(ev.Reason, "blocked_by_list") {
		f.BlockedByRule, f.BlockedRuleDetail = true, ev.Reason
	}
	return f
}

// evaluate runs the rules for a stored event, stores alerts and notifies.
func (s *Service) evaluate(ctx context.Context, orgID string, ev Event) []Finding {
	st := s.GetSettings(ctx, orgID)
	f := s.facts(ctx, orgID, ev, st)
	findings := Evaluate(ev, f, st.Thresholds)
	if len(findings) == 0 {
		return nil
	}
	for _, fd := range findings {
		s.raise(ctx, orgID, ev, fd, st)
	}
	if sevRank(findings[0].Severity) >= 2 {
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.login_events SET suspicious=true, reason=CASE WHEN reason='' THEN $2 ELSE reason END WHERE id=$1`, ev.ID, findings[0].Title)
	}
	// Tell the person about a sign-in that looks new, with a "was this you?" link.
	if ev.Success && ev.UserID != "" && s.Notify != nil {
		for _, fd := range findings {
			if fd.Rule == "new_country" || fd.Rule == "new_device" || fd.Rule == "admin_new_place" || fd.Rule == "impossible_travel" {
				place := strings.TrimSpace(strings.Join([]string{ev.City, ev.Country}, ", "))
				place = strings.Trim(place, ", ")
				if place == "" {
					place = "a new place"
				}
				body := fmt.Sprintf("New sign-in from %s on %s. Was this you?", place, DeviceLabel(ev.UserAgent))
				s.Notify(ctx, orgID, ev.UserID, "New sign-in to your account", body, fmt.Sprintf("/security?event=%d", ev.ID))
				if s.Mail != nil && ev.Email != "" {
					_ = s.Mail(ctx, []string{ev.Email}, "New sign-in to your account",
						body+"\n\nIf it was you, nothing to do. If it was not, open Security in your account and choose \"This wasn't me\".")
				}
				break
			}
		}
	}
	return findings
}

// raise stores one finding as an alert (merging into an open one) and notifies for new ones.
func (s *Service) raise(ctx context.Context, orgID string, ev Event, fd Finding, st Settings) {
	evid, _ := json.Marshal(fd.Evidence)
	due := s.now().Add(st.Thresholds.SLA(fd.Severity))
	var id string
	var inserted bool
	err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.iam_alerts (org_id, user_id, email, ip, rule, severity, title, evidence, recommended, dedupe_key, event_id, sla_due_at)
VALUES ($1::uuid, NULLIF($2,'')::uuid, $3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (org_id, dedupe_key) WHERE dedupe_key <> '' AND status IN ('open','investigating')
DO UPDATE SET evidence=EXCLUDED.evidence, title=EXCLUDED.title, event_id=EXCLUDED.event_id, updated_at=now(),
  severity=CASE WHEN array_position(ARRAY['low','medium','high','critical'], EXCLUDED.severity) > array_position(ARRAY['low','medium','high','critical'], adrient.iam_alerts.severity) THEN EXCLUDED.severity ELSE adrient.iam_alerts.severity END
RETURNING id::text, (xmax = 0)`, orgID, ev.UserID, ev.Email, ev.IP, fd.Rule, fd.Severity, fd.Title, evid, fd.Recommended, fd.DedupeKey, ev.ID, due).Scan(&id, &inserted)
	if err != nil || !inserted {
		return
	}
	if sevRank(fd.Severity) >= 2 {
		s.fanOut(ctx, orgID, id, fd, ev, st)
	}
}

// recipients returns the admins and analysts who should see alerts.
func (s *Service) recipients(ctx context.Context, orgID string) []string {
	rows, err := s.Pool.Query(ctx, `SELECT user_id::text FROM adrient.org_admins WHERE org_id::text=$1
UNION SELECT user_id::text FROM adrient.iam_role_assignments WHERE org_id::text=$1 AND role IN ('identity_admin','security_analyst')`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

func (s *Service) fanOut(ctx context.Context, orgID, alertID string, fd Finding, ev Event, st Settings) {
	if s.Notify != nil {
		for _, uid := range s.recipients(ctx, orgID) {
			s.Notify(ctx, orgID, uid, "Security alert: "+fd.Title, fd.Recommended, "/admin/secops?alert="+alertID)
		}
	}
	if s.Mail != nil && st.SecurityMailbox != "" {
		_ = s.Mail(ctx, []string{st.SecurityMailbox}, "[Security alert, "+fd.Severity+"] "+fd.Title, fd.Title+"\n\n"+fd.Recommended+"\n\nRule: "+fd.Rule)
	}
	if st.WebhookURL != "" && s.Webhook != nil {
		go s.Webhook(context.WithoutCancel(ctx), st.WebhookURL, map[string]any{"type": "iam.alert", "alert_id": alertID, "org_id": orgID, "rule": fd.Rule,
			"severity": fd.Severity, "title": fd.Title, "email": ev.Email, "ip": ev.IP, "recommended": fd.Recommended})
	}
}

// ---- helpers -----------------------------------------------------------------

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func (s *Service) audit(ctx context.Context, org, actor, action string, d map[string]any) {
	if s.Audit != nil {
		s.Audit(ctx, org, actor, "iam."+strings.TrimPrefix(action, "iam."), d)
	}
}

func scanOne(row pgx.Row, dst ...any) error {
	if err := row.Scan(dst...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

var _ = pgxpool.New
