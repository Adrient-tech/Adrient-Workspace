package iamops

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func ruleSet(fs []Finding) map[string]string {
	m := map[string]string{}
	for _, f := range fs {
		m[f.Rule] = f.Severity
	}
	return m
}

func baseFacts() Facts {
	return Facts{UserKnown: true, PriorSuccesses: 20, PriorCountries: map[string]bool{"IN": true}, PriorDevices: map[string]bool{"d1": true},
		LastSuccessAt: t0.Add(-8 * time.Hour), LastSuccessCtry: "IN"}
}

func TestQuietNormalSignIn(t *testing.T) {
	ev := Event{ID: 1, UserID: "u1", Email: "a@x.test", IP: "1.2.3.4", Success: true, Country: "IN", DeviceFP: "d1", At: t0}
	if got := Evaluate(ev, baseFacts(), DefaultThresholds()); len(got) != 0 {
		t.Fatalf("normal sign-in raised %v", got)
	}
}

func TestImpossibleTravel(t *testing.T) {
	f := baseFacts()
	f.LastSuccessAt = t0.Add(-30 * time.Minute)
	ev := Event{ID: 2, UserID: "u1", Email: "a@x.test", IP: "8.8.8.8", Success: true, Country: "US", DeviceFP: "d1", At: t0}
	got := ruleSet(Evaluate(ev, f, DefaultThresholds()))
	if got["impossible_travel"] != "high" || got["new_country"] != "medium" {
		t.Fatalf("got %v", got)
	}
	// Same move after 12 hours is only a new country.
	f.LastSuccessAt = t0.Add(-12 * time.Hour)
	got = ruleSet(Evaluate(ev, f, DefaultThresholds()))
	if _, ok := got["impossible_travel"]; ok || got["new_country"] == "" {
		t.Fatalf("after 12h got %v", got)
	}
}

func TestNewDeviceOnlyAfterHistory(t *testing.T) {
	ev := Event{ID: 3, UserID: "u1", Email: "a@x.test", IP: "1.1.1.1", Success: true, Country: "IN", DeviceFP: "zz", At: t0}
	got := ruleSet(Evaluate(ev, baseFacts(), DefaultThresholds()))
	if got["new_device"] != "low" {
		t.Fatalf("got %v", got)
	}
	f := baseFacts()
	f.PriorSuccesses, f.PriorDevices, f.PriorCountries = 0, map[string]bool{}, map[string]bool{}
	if got := Evaluate(ev, f, DefaultThresholds()); len(got) != 0 {
		t.Fatalf("first-ever sign-in must not alert: %v", got)
	}
}

func TestAdminNewPlaceReplacesNewCountry(t *testing.T) {
	f := baseFacts()
	f.IsAdmin = true
	ev := Event{ID: 4, UserID: "u1", Email: "root@x.test", IP: "9.9.9.9", Success: true, Country: "DE", DeviceFP: "zz", At: t0}
	got := ruleSet(Evaluate(ev, f, DefaultThresholds()))
	if got["admin_new_place"] != "high" {
		t.Fatalf("got %v", got)
	}
	if _, dup := got["new_country"]; dup {
		t.Fatalf("admin should not also raise new_country: %v", got)
	}
}

func TestFailureBurstsAndStuffing(t *testing.T) {
	th := DefaultThresholds()
	ev := Event{ID: 5, UserID: "u1", Email: "a@x.test", IP: "5.5.5.5", Success: false, At: t0}
	f := Facts{FailsAccount: 4, FailsIP: 4, AccountsFromIP: 1}
	if got := Evaluate(ev, f, th); len(got) != 0 {
		t.Fatalf("below threshold raised %v", got)
	}
	f = Facts{FailsAccount: 5, FailsIP: 12, AccountsFromIP: 6}
	got := ruleSet(Evaluate(ev, f, th))
	for _, r := range []string{"failed_burst_account", "failed_burst_ip", "credential_stuffing"} {
		if got[r] == "" {
			t.Errorf("missing %s in %v", r, got)
		}
	}
	// A tuned threshold changes the outcome.
	th.FailsPerAccount = 3
	if got := ruleSet(Evaluate(ev, Facts{FailsAccount: 3}, th)); got["failed_burst_account"] == "" {
		t.Fatalf("tuned threshold ignored: %v", got)
	}
}

func TestMFAFatigue(t *testing.T) {
	ev := Event{ID: 6, UserID: "u1", Email: "a@x.test", IP: "5.5.5.5", Success: false, MFAResult: "denied", At: t0}
	if got := ruleSet(Evaluate(ev, Facts{MFADenied: 3}, DefaultThresholds())); got["mfa_fatigue"] != "high" {
		t.Fatalf("got %v", got)
	}
	if got := Evaluate(ev, Facts{MFADenied: 2}, DefaultThresholds()); len(got) != 0 {
		t.Fatalf("two denials raised %v", got)
	}
}

func TestOffHoursAnonymizerDisabledHoneypot(t *testing.T) {
	th := DefaultThresholds()
	night := time.Date(2026, 11, 20, 2, 30, 0, 0, time.UTC)
	ev := Event{ID: 7, UserID: "u1", Email: "a@x.test", IP: "10.9.8.7", Success: true, Country: "IN", DeviceFP: "d1", At: night}
	f := baseFacts()
	f.Anonymizers = []string{"10.9.8.0/24"}
	f.HoneypotHits = 2
	got := ruleSet(Evaluate(ev, f, th))
	if got["off_hours"] != "low" || got["anonymizer"] != "medium" || got["honeypot_hit"] != "medium" {
		t.Fatalf("got %v", got)
	}
	// Org clock +5:30 turns 02:30 UTC into 08:00 local: no longer off hours.
	th.OffHoursUTCOffsetMin = 330
	if _, ok := ruleSet(Evaluate(ev, f, th))["off_hours"]; ok {
		t.Fatal("off hours ignored the org clock")
	}
	f2 := Facts{UserKnown: true, AccountDisabled: true}
	ev2 := Event{ID: 8, UserID: "u9", Email: "gone@x.test", IP: "1.1.1.1", Success: false, At: t0}
	if ruleSet(Evaluate(ev2, f2, DefaultThresholds()))["disabled_account"] != "medium" {
		t.Fatal("disabled account attempt not raised")
	}
}

func TestOrderingAndDedupeKeys(t *testing.T) {
	f := baseFacts()
	f.IsAdmin = true
	f.FailsAccount = 9
	ev := Event{ID: 9, UserID: "u1", Email: "a@x.test", IP: "7.7.7.7", Success: true, Country: "CN", DeviceFP: "zz", At: t0}
	got := Evaluate(ev, f, DefaultThresholds())
	for i := 1; i < len(got); i++ {
		if sevRank(got[i].Severity) > sevRank(got[i-1].Severity) {
			t.Fatalf("not sorted: %v", got)
		}
	}
	if got[0].DedupeKey == "" {
		t.Fatal("empty dedupe key")
	}
	ip := Evaluate(Event{ID: 10, Email: "x@y.test", IP: "6.6.6.6", At: t0}, Facts{AccountsFromIP: 9}, DefaultThresholds())
	if len(ip) != 1 || ip[0].DedupeKey != "credential_stuffing:6.6.6.6" {
		t.Fatalf("address rule key = %+v", ip)
	}
}

func TestNetworkEntryValidation(t *testing.T) {
	for _, ok := range []string{"1.2.3.4", "10.0.0.0/8", "2001:db8::1"} {
		if _, err := ValidateNetworkEntry(ok); err != nil {
			t.Errorf("%s rejected: %v", ok, err)
		}
	}
	for _, no := range []string{"", "abc", "300.1.1.1", "1.2.3.4/99"} {
		if _, err := ValidateNetworkEntry(no); err == nil {
			t.Errorf("%q accepted", no)
		}
	}
}

func TestPermissionMatrix(t *testing.T) {
	type c struct {
		role, cap string
		want      bool
	}
	cases := []c{
		{RoleHelpDesk, CapResetPassword, true}, {RoleHelpDesk, CapResetMFA, true}, {RoleHelpDesk, CapSessions, true}, {RoleHelpDesk, CapUnlock, true},
		{RoleHelpDesk, CapPolicyEdit, false}, {RoleHelpDesk, CapDelegate, false}, {RoleHelpDesk, CapSuspend, false}, {RoleHelpDesk, CapContain, false},
		{RoleHelpDesk, CapViewAlerts, false}, {RoleHelpDesk, CapBypassMFA, false},
		{RoleSecurityAnalyst, CapViewAlerts, true}, {RoleSecurityAnalyst, CapTriage, true}, {RoleSecurityAnalyst, CapContain, true},
		{RoleSecurityAnalyst, CapPolicyEdit, false}, {RoleSecurityAnalyst, CapListEdit, false}, {RoleSecurityAnalyst, CapResetPassword, false},
		{RoleAuditor, CapViewAlerts, true}, {RoleAuditor, CapReports, true}, {RoleAuditor, CapTriage, false}, {RoleAuditor, CapResetPassword, false},
		{RoleAuditor, CapContain, false},
		{RoleIdentityAdmin, CapPolicyEdit, true}, {RoleIdentityAdmin, CapDelegate, true}, {RoleIdentityAdmin, CapBypassMFA, true}, {RoleIdentityAdmin, CapApprove, true},
	}
	for _, x := range cases {
		if got := Has([]string{x.role}, x.cap); got != x.want {
			t.Errorf("%s/%s = %v, want %v", x.role, x.cap, got, x.want)
		}
	}
	// Every action maps to a capability the identity admin holds.
	for act, cp := range actionCap {
		if !Has([]string{RoleIdentityAdmin}, cp) {
			t.Errorf("identity admin lacks %s for %s", cp, act)
		}
	}
}

func TestTargetRules(t *testing.T) {
	if !CanTarget([]string{RoleHelpDesk}, false, false, false) {
		t.Error("help desk must reach an ordinary user")
	}
	if CanTarget([]string{RoleHelpDesk}, false, true, false) {
		t.Error("help desk must not reach an administrator")
	}
	if CanTarget([]string{RoleIdentityAdmin}, false, true, false) {
		t.Error("an admin who is not a super admin must not reach another admin")
	}
	if !CanTarget([]string{RoleIdentityAdmin}, true, true, false) {
		t.Error("super admin must reach an admin")
	}
}

func TestStepUpSet(t *testing.T) {
	for _, a := range []string{"password.temp", "mfa.reset_all", "contain", "account.suspend", "policy.edit"} {
		if !NeedsStepUp(a) {
			t.Errorf("%s should need step-up", a)
		}
	}
	for _, a := range []string{"password.reset_link", "sessions.revoke_all", "account.unlock"} {
		if NeedsStepUp(a) {
			t.Errorf("%s should not need step-up", a)
		}
	}
}

func TestDeviceFingerprintStable(t *testing.T) {
	a := DeviceFingerprint("Mozilla/5.0 (Windows NT 10.0) Chrome/120.0 Safari/537.36")
	b := DeviceFingerprint("Mozilla/5.0 (Windows NT 10.0) Chrome/121.5 Safari/537.36")
	c := DeviceFingerprint("Mozilla/5.0 (iPhone; CPU iPhone OS 17) Safari/604.1")
	if a == "" || a != b || a == c {
		t.Fatalf("fingerprints: %s %s %s", a, b, c)
	}
	if DeviceLabel("Mozilla/5.0 (Windows NT 10.0) Chrome/120.0") != "Chrome on Windows" {
		t.Fatal("label")
	}
}

func TestRandomPasswordMeetsComplexity(t *testing.T) {
	for i := 0; i < 50; i++ {
		p := randomPassword()
		var lo, up, di, sy bool
		for _, r := range p {
			switch {
			case r >= 'a' && r <= 'z':
				lo = true
			case r >= 'A' && r <= 'Z':
				up = true
			case r >= '0' && r <= '9':
				di = true
			default:
				sy = true
			}
		}
		if len(p) < 16 || !lo || !up || !di || !sy {
			t.Fatalf("weak password %q", p)
		}
	}
}
