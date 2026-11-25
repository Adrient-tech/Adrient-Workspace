package iamops

import "sort"

// Operation roles.
const (
	RoleHelpDesk        = "help_desk"
	RoleSecurityAnalyst = "security_analyst"
	RoleIdentityAdmin   = "identity_admin"
	RoleAuditor         = "auditor"
)

// Roles lists the assignable roles with a description for the admin screen.
var Roles = []struct {
	Key, Name, Description string
}{
	{RoleHelpDesk, "Help desk", "Resets passwords and second steps, signs people out and unlocks accounts for non-admin users. Cannot change roles or policies and cannot touch administrators."},
	{RoleSecurityAnalyst, "Security analyst", "Reads sign-in events and alerts, triages them and contains an account. Cannot edit policies."},
	{RoleIdentityAdmin, "Identity admin", "Everything, including policies, lists, delegation and administrator accounts."},
	{RoleAuditor, "Auditor", "Read only: events, alerts, reports and the action log."},
}

// Capabilities, one per kind of operation.
const (
	CapViewUsers     = "view_users"
	CapViewEvents    = "view_events"
	CapViewAlerts    = "view_alerts"
	CapTriage        = "triage"
	CapResetPassword = "reset_password"
	CapTempPassword  = "temp_password"
	CapUnlock        = "unlock"
	CapViewMFA       = "view_mfa"
	CapResetMFA      = "reset_mfa"
	CapBypassMFA     = "bypass_mfa"
	CapSessions      = "sessions"
	CapTokens        = "tokens"
	CapSuspend       = "suspend"
	CapQuarantine    = "quarantine"
	CapAccountEdit   = "account_edit"
	CapContain       = "contain"
	CapPolicyEdit    = "policy_edit"
	CapListEdit      = "list_edit"
	CapDelegate      = "delegate"
	CapReports       = "reports"
	CapApprove       = "approve"
	CapViewActionLog = "view_action_log"
)

var roleCaps = map[string][]string{
	RoleHelpDesk: {CapViewUsers, CapViewMFA, CapResetPassword, CapTempPassword, CapUnlock, CapResetMFA, CapSessions, CapViewEvents},
	RoleSecurityAnalyst: {CapViewUsers, CapViewMFA, CapViewEvents, CapViewAlerts, CapTriage, CapContain, CapSessions, CapQuarantine,
		CapReports, CapViewActionLog},
	RoleAuditor: {CapViewUsers, CapViewMFA, CapViewEvents, CapViewAlerts, CapReports, CapViewActionLog},
	RoleIdentityAdmin: {CapViewUsers, CapViewEvents, CapViewAlerts, CapTriage, CapResetPassword, CapTempPassword, CapUnlock, CapViewMFA,
		CapResetMFA, CapBypassMFA, CapSessions, CapTokens, CapSuspend, CapQuarantine, CapAccountEdit, CapContain, CapPolicyEdit,
		CapListEdit, CapDelegate, CapReports, CapApprove, CapViewActionLog},
}

// Caps returns the union of capabilities of the roles, sorted.
func Caps(roles []string) []string {
	set := map[string]bool{}
	for _, r := range roles {
		for _, c := range roleCaps[r] {
			set[c] = true
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Has reports whether any role grants the capability.
func Has(roles []string, cap string) bool {
	for _, r := range roles {
		for _, c := range roleCaps[r] {
			if c == cap {
				return true
			}
		}
	}
	return false
}

// ValidRole reports whether key is an assignable role.
func ValidRole(key string) bool {
	_, ok := roleCaps[key]
	return ok
}

// Sensitive actions need a fresh step-up (password or one-time code).
var sensitive = map[string]bool{
	"password.temp": true, "mfa.reset_all": true, "mfa.revoke": true, "mfa.bypass": true, "account.suspend": true,
	"account.quarantine": true, "account.email": true, "account.phone": true, "account.expiry": true, "account.unlink_idp": true,
	"contain": true, "policy.edit": true, "list.edit": true, "role.assign": true, "tokens.revoke_all": true,
}

// NeedsStepUp reports whether the action requires a recent re-authentication.
func NeedsStepUp(action string) bool { return sensitive[action] }

// actionCap maps an action to the capability it needs.
var actionCap = map[string]string{
	"password.reset_link": CapResetPassword, "password.temp": CapTempPassword, "password.expire": CapResetPassword,
	"account.unlock": CapUnlock,
	"mfa.revoke":     CapResetMFA, "mfa.reset_all": CapResetMFA, "mfa.require_reenroll": CapResetMFA, "mfa.bypass": CapBypassMFA,
	"sessions.revoke": CapSessions, "sessions.revoke_all": CapSessions, "tokens.revoke": CapTokens, "tokens.revoke_all": CapTokens,
	"account.suspend": CapSuspend, "account.unsuspend": CapSuspend, "account.quarantine": CapQuarantine, "account.unquarantine": CapQuarantine,
	"account.email": CapAccountEdit, "account.phone": CapAccountEdit, "account.expiry": CapAccountEdit, "account.unlink_idp": CapAccountEdit,
	"contain": CapContain,
}

// CapFor returns the capability needed for an action ("" when unknown).
func CapFor(action string) string { return actionCap[action] }

// Actions that may lock a person out, never allowed on the last administrator.
var lockout = map[string]bool{"account.suspend": true, "account.quarantine": true, "contain": true, "account.expiry": true, "password.temp": false}

// CanTarget applies the target rules: only a super admin may act on an administrator,
// and nobody except a super admin may touch another admin; the help desk and analysts
// can never act on administrators.
func CanTarget(callerRoles []string, callerIsSuper, targetIsAdmin, self bool) bool {
	if !targetIsAdmin {
		return true
	}
	if self {
		return true
	}
	return callerIsSuper
}
