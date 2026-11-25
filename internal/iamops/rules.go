// Package iamops is the identity and access operations console: sign-in risk
// detection and triage, per-user security actions (password, MFA, sessions,
// account state), delegation, step-up and reports.
//
// This file holds the risk rules. They are pure functions of an event, a set of
// facts read from the history and the org's thresholds, so they can be tested
// with fixtures and without a database.
package iamops

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// Event is one sign-in attempt.
type Event struct {
	ID        int64     `json:"id"`
	OrgID     string    `json:"org_id,omitempty"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Method    string    `json:"method"`
	Success   bool      `json:"success"`
	Reason    string    `json:"reason"`
	Country   string    `json:"country"`
	City      string    `json:"city"`
	DeviceFP  string    `json:"device_fp"`
	MFAResult string    `json:"mfa_result"` // "", passed, failed, denied, skipped
	At        time.Time `json:"at"`
}

// Thresholds are the tunable limits of the rules engine. Zero values fall back
// to the defaults in DefaultThresholds.
type Thresholds struct {
	WindowMinutes        int  `json:"window_minutes"`           // sliding window for the burst rules
	FailsPerAccount      int  `json:"fails_per_account"`        // failures on one account in the window
	FailsPerIP           int  `json:"fails_per_ip"`             // failures from one address in the window
	StuffingAccounts     int  `json:"stuffing_accounts"`        // distinct accounts tried from one address
	MFADenied            int  `json:"mfa_denied"`               // denied or failed MFA prompts in the window
	TravelMinutes        int  `json:"travel_minutes"`           // two countries within this time is impossible travel
	OffHoursStart        int  `json:"off_hours_start"`          // hour of day (0-23) when the quiet period starts
	OffHoursEnd          int  `json:"off_hours_end"`            // hour of day when it ends
	OffHoursUTCOffsetMin int  `json:"off_hours_utc_offset_min"` // org clock offset from UTC, minutes
	OffHoursEnabled      bool `json:"off_hours_enabled"`
	NewDeviceEnabled     bool `json:"new_device_enabled"`
	SLAHoursCritical     int  `json:"sla_hours_critical"`
	SLAHoursHigh         int  `json:"sla_hours_high"`
	SLAHoursMedium       int  `json:"sla_hours_medium"`
	SLAHoursLow          int  `json:"sla_hours_low"`
}

// DefaultThresholds returns the shipped defaults.
func DefaultThresholds() Thresholds {
	return Thresholds{WindowMinutes: 10, FailsPerAccount: 5, FailsPerIP: 10, StuffingAccounts: 5, MFADenied: 3,
		TravelMinutes: 180, OffHoursStart: 22, OffHoursEnd: 5, OffHoursEnabled: true, NewDeviceEnabled: true,
		SLAHoursCritical: 1, SLAHoursHigh: 4, SLAHoursMedium: 24, SLAHoursLow: 72}
}

// Normalize fills zero fields from the defaults and clamps silly values.
func (t Thresholds) Normalize() Thresholds {
	d := DefaultThresholds()
	pick := func(v, def, lo, hi int) int {
		if v == 0 {
			return def
		}
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	t.WindowMinutes = pick(t.WindowMinutes, d.WindowMinutes, 1, 1440)
	t.FailsPerAccount = pick(t.FailsPerAccount, d.FailsPerAccount, 2, 1000)
	t.FailsPerIP = pick(t.FailsPerIP, d.FailsPerIP, 2, 10000)
	t.StuffingAccounts = pick(t.StuffingAccounts, d.StuffingAccounts, 2, 10000)
	t.MFADenied = pick(t.MFADenied, d.MFADenied, 2, 100)
	t.TravelMinutes = pick(t.TravelMinutes, d.TravelMinutes, 5, 1440)
	t.SLAHoursCritical = pick(t.SLAHoursCritical, d.SLAHoursCritical, 1, 720)
	t.SLAHoursHigh = pick(t.SLAHoursHigh, d.SLAHoursHigh, 1, 720)
	t.SLAHoursMedium = pick(t.SLAHoursMedium, d.SLAHoursMedium, 1, 720)
	t.SLAHoursLow = pick(t.SLAHoursLow, d.SLAHoursLow, 1, 720)
	if t.OffHoursStart < 0 || t.OffHoursStart > 23 {
		t.OffHoursStart = d.OffHoursStart
	}
	if t.OffHoursEnd < 0 || t.OffHoursEnd > 23 {
		t.OffHoursEnd = d.OffHoursEnd
	}
	if t.OffHoursUTCOffsetMin < -840 || t.OffHoursUTCOffsetMin > 840 {
		t.OffHoursUTCOffsetMin = 0
	}
	return t
}

// SLA returns the response time for a severity.
func (t Thresholds) SLA(sev string) time.Duration {
	switch sev {
	case "critical":
		return time.Duration(t.SLAHoursCritical) * time.Hour
	case "high":
		return time.Duration(t.SLAHoursHigh) * time.Hour
	case "medium":
		return time.Duration(t.SLAHoursMedium) * time.Hour
	}
	return time.Duration(t.SLAHoursLow) * time.Hour
}

// Facts is what the rules need to know about the history around an event.
type Facts struct {
	UserKnown         bool
	IsAdmin           bool
	AccountDisabled   bool
	PriorSuccesses    int             // successful sign-ins in the lookback period, before this event
	PriorCountries    map[string]bool // upper-case country codes
	PriorDevices      map[string]bool // device fingerprints
	LastSuccessAt     time.Time
	LastSuccessCtry   string
	LastSuccessCity   string
	FailsAccount      int // failures on this account in the window, including this event
	FailsIP           int // failures from this address in the window, including this event
	AccountsFromIP    int // distinct accounts attempted from this address in the window
	MFADenied         int // denied or failed MFA results on this account in the window, including this event
	HoneypotHits      int // decoy hits from this address in the last 24 hours
	BlockedByRule     bool
	BlockedRuleDetail string
	Anonymizers       []string // IPs or CIDRs of anonymising networks
}

// Finding is one risk the engine raised for an event.
type Finding struct {
	Rule        string         `json:"rule"`
	Severity    string         `json:"severity"`
	Title       string         `json:"title"`
	Recommended string         `json:"recommended"`
	Evidence    map[string]any `json:"evidence"`
	DedupeKey   string         `json:"dedupe_key"`
	Subject     string         `json:"subject"` // user | ip
}

// RuleNames lists every rule key with a short description, for the settings screen.
var RuleNames = map[string]string{
	"impossible_travel":    "Sign-in from a different country than the last one, too soon to have travelled",
	"new_country":          "First sign-in from a country this person has not used before",
	"new_device":           "First sign-in from a device this person has not used before",
	"failed_burst_account": "Many failed sign-ins on one account",
	"failed_burst_ip":      "Many failed sign-ins from one address",
	"credential_stuffing":  "One address trying many different accounts",
	"off_hours":            "Sign-in outside the organisation's normal hours",
	"anonymizer":           "Sign-in from a VPN, proxy or anonymising network on the org's list",
	"mfa_fatigue":          "Many denied or failed second-step prompts in a short time",
	"admin_new_place":      "Administrator signing in from a new country or device",
	"disabled_account":     "Sign-in attempt on a suspended or disabled account",
	"honeypot_hit":         "Sign-in from an address that also hit the decoy pages",
	"blocked_by_rule":      "Sign-in attempt from an address or country on the block list",
}

func sevRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	}
	return 1
}

func bump(s string) string {
	switch s {
	case "low":
		return "medium"
	case "medium":
		return "high"
	}
	return "critical"
}

// Evaluate runs every rule against one event and returns the findings, most
// severe first. It never touches storage.
func Evaluate(ev Event, f Facts, th Thresholds) []Finding {
	th = th.Normalize()
	var out []Finding
	subj := ev.UserID
	if subj == "" {
		subj = strings.ToLower(ev.Email)
	}
	ev.Country = strings.ToUpper(strings.TrimSpace(ev.Country))
	add := func(rule, sev, title, rec string, evid map[string]any, subject string) {
		key := rule + ":" + subj
		if subject == "ip" {
			key = rule + ":" + ev.IP
		}
		evid["event_id"] = ev.ID
		evid["ip"] = ev.IP
		evid["country"] = ev.Country
		evid["method"] = ev.Method
		out = append(out, Finding{Rule: rule, Severity: sev, Title: title, Recommended: rec, Evidence: evid, DedupeKey: key, Subject: subject})
	}
	who := ev.Email
	if who == "" {
		who = "unknown account"
	}

	if f.BlockedByRule {
		add("blocked_by_rule", "medium", fmt.Sprintf("Blocked sign-in attempt for %s from %s", who, ev.IP),
			"Check whether the address should stay on the block list.", map[string]any{"detail": f.BlockedRuleDetail}, "ip")
	}
	if f.AccountDisabled && ev.UserID != "" {
		add("disabled_account", "medium", fmt.Sprintf("Sign-in attempt on the disabled account %s", who),
			"Confirm the account should stay disabled; a leaver's credentials may be in use.", map[string]any{"reason": ev.Reason}, "user")
	}
	if !ev.Success && f.FailsAccount >= th.FailsPerAccount {
		sev := "medium"
		if f.IsAdmin {
			sev = "high"
		}
		add("failed_burst_account", sev, fmt.Sprintf("%d failed sign-ins on %s in %d minutes", f.FailsAccount, who, th.WindowMinutes),
			"Unlock the account only after confirming with the owner; consider a password reset.",
			map[string]any{"failures": f.FailsAccount, "window_minutes": th.WindowMinutes}, "user")
	}
	if !ev.Success && f.FailsIP >= th.FailsPerIP {
		add("failed_burst_ip", "medium", fmt.Sprintf("%d failed sign-ins from %s in %d minutes", f.FailsIP, ev.IP, th.WindowMinutes),
			"Block the address for a few hours.", map[string]any{"failures": f.FailsIP, "window_minutes": th.WindowMinutes}, "ip")
	}
	if f.AccountsFromIP >= th.StuffingAccounts {
		add("credential_stuffing", "high", fmt.Sprintf("%s tried %d different accounts in %d minutes", ev.IP, f.AccountsFromIP, th.WindowMinutes),
			"Block the address and reset passwords of any account that signed in from it.",
			map[string]any{"accounts": f.AccountsFromIP, "window_minutes": th.WindowMinutes}, "ip")
	}
	if f.MFADenied >= th.MFADenied {
		add("mfa_fatigue", "high", fmt.Sprintf("%d denied or failed second-step prompts for %s", f.MFADenied, who),
			"Contact the person; if they did not trigger the prompts their password is known to someone else. Contain the account.",
			map[string]any{"denied": f.MFADenied, "window_minutes": th.WindowMinutes}, "user")
	}
	if f.HoneypotHits > 0 {
		add("honeypot_hit", "medium", fmt.Sprintf("%s hit the decoy pages %d time(s) and also tried to sign in as %s", ev.IP, f.HoneypotHits, who),
			"The address is probing the site. Block it.", map[string]any{"decoy_hits": f.HoneypotHits}, "ip")
	}
	if inList(ev.IP, f.Anonymizers) {
		sev := "medium"
		if f.IsAdmin {
			sev = "high"
		}
		add("anonymizer", sev, fmt.Sprintf("Sign-in for %s from an anonymising network (%s)", who, ev.IP),
			"Ask the person whether they use a VPN; otherwise treat as suspicious.", map[string]any{}, "user")
	}

	if ev.Success && ev.UserID != "" && f.UserKnown {
		newCountry := ev.Country != "" && f.PriorSuccesses > 0 && len(f.PriorCountries) > 0 && !f.PriorCountries[ev.Country]
		newDevice := th.NewDeviceEnabled && ev.DeviceFP != "" && f.PriorSuccesses > 0 && len(f.PriorDevices) > 0 && !f.PriorDevices[ev.DeviceFP]
		if newCountry && !f.LastSuccessAt.IsZero() && f.LastSuccessCtry != "" && f.LastSuccessCtry != ev.Country &&
			ev.At.Sub(f.LastSuccessAt) >= 0 && ev.At.Sub(f.LastSuccessAt) < time.Duration(th.TravelMinutes)*time.Minute {
			add("impossible_travel", "high", fmt.Sprintf("%s signed in from %s %d minutes after %s", who, ev.Country, int(ev.At.Sub(f.LastSuccessAt).Minutes()), f.LastSuccessCtry),
				"Contain the account unless the person confirms it was them.",
				map[string]any{"previous_country": f.LastSuccessCtry, "previous_city": f.LastSuccessCity, "minutes_apart": int(ev.At.Sub(f.LastSuccessAt).Minutes())}, "user")
		}
		switch {
		case f.IsAdmin && (newCountry || newDevice):
			add("admin_new_place", "high", fmt.Sprintf("Administrator %s signed in from a new %s", who, newWhat(newCountry, newDevice)),
				"Confirm with the administrator right away.", map[string]any{"new_country": newCountry, "new_device": newDevice, "device": ev.DeviceFP}, "user")
		case newCountry:
			add("new_country", "medium", fmt.Sprintf("%s signed in from a new country (%s)", who, ev.Country),
				"Ask the person; use 'Was this you?' on the sign-in notice.", map[string]any{"known_countries": keys(f.PriorCountries)}, "user")
		case newDevice:
			add("new_device", "low", fmt.Sprintf("%s signed in from a new device", who),
				"No action unless the person says it was not them.", map[string]any{"device": ev.DeviceFP, "user_agent": ev.UserAgent}, "user")
		}
		if th.OffHoursEnabled && offHours(ev.At, th) {
			sev := "low"
			if f.IsAdmin {
				sev = "medium"
			}
			add("off_hours", sev, fmt.Sprintf("%s signed in outside normal hours", who), "Check it against the person's shift or travel.",
				map[string]any{"local_hour": localHour(ev.At, th)}, "user")
		}
	}
	// stable order: most severe first
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && sevRank(out[j].Severity) > sevRank(out[j-1].Severity); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func newWhat(country, device bool) string {
	switch {
	case country && device:
		return "country and device"
	case country:
		return "country"
	}
	return "device"
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func localHour(t time.Time, th Thresholds) int {
	return t.UTC().Add(time.Duration(th.OffHoursUTCOffsetMin) * time.Minute).Hour()
}

func offHours(t time.Time, th Thresholds) bool {
	h := localHour(t, th)
	s, e := th.OffHoursStart, th.OffHoursEnd
	if s == e {
		return false
	}
	if s < e {
		return h >= s && h < e
	}
	return h >= s || h < e
}

// inList reports whether ip equals or falls inside one of the entries (IPs or CIDRs).
func inList(ip string, list []string) bool {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return false
	}
	for _, e := range list {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(e); err == nil {
			if n.Contains(p) {
				return true
			}
			continue
		}
		if q := net.ParseIP(e); q != nil && q.Equal(p) {
			return true
		}
	}
	return false
}

// ValidateNetworkEntry checks an IP or CIDR entered by an admin.
func ValidateNetworkEntry(v string) (string, error) {
	v = strings.TrimSpace(v)
	if _, n, err := net.ParseCIDR(v); err == nil {
		return n.String(), nil
	}
	if p := net.ParseIP(v); p != nil {
		return p.String(), nil
	}
	return "", fmt.Errorf("%q is not an IP address or CIDR range", v)
}
