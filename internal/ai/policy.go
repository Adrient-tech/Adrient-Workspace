package ai

import (
	"context"
	"regexp"
	"strings"
)

// Business-app tool policy.
//
// The assistant's tools run in the user's browser and call the module REST
// APIs with the user's own session, so the module APIs are always the final
// permission check (an employee's token cannot read other people's pay no
// matter what the model asks for). This file is the second line: a tool that
// needs a capability is not even offered to the model when the caller lacks it,
// so a prompt-injected "read everyone's salary" has no tool to call.
//
// Capabilities are either platform permissions (authz strings such as
// "accounting:ledger:read") or one of the derived names below.
const (
	CapOrgAdmin  = "org.admin"        // organisation admin
	CapHRAdmin   = "hr.admin"         // org admin or HR admin grant
	CapRecruiter = "talent.recruiter" // org admin or HR user in Talent settings

	CapDealRead     = "crm:deal:read"
	CapDealWrite    = "crm:deal:write"
	CapLedgerRead   = "accounting:ledger:read"
	CapLedgerWrite  = "accounting:ledger:write"
	CapStockRead    = "inventory:stock:read"
	CapProductWrite = "inventory:product:write"
	CapDocRead      = "invoicing:document:read"
	CapDocWrite     = "invoicing:document:write"
	CapPOSRead      = "pos:read"
	CapPOSReport    = "pos:report:read"
)

// ToolCaps maps a tool name to the capability it needs. Tools not listed need
// none (the module API still checks). Keep in step with web/app/src/pages/*/ai/tools.ts
// (TestToolCapsCoverBusinessTools fails when a listed prefix has no entry).
var ToolCaps = map[string]string{
	// CRM
	"crm_find_deals": CapDealRead, "crm_get_deal": CapDealRead, "crm_company_summary": CapDealRead,
	"crm_pipeline_forecast": CapDealRead, "crm_stalled_deals": CapDealRead, "crm_followup_context": CapDealRead,
	"crm_log_activity": CapDealWrite, "crm_create_followup": CapDealWrite, "crm_move_deal": CapDealWrite,
	"crm_bulk_update_deals": CapDealWrite,
	// HR: employee tools need no capability (the API scopes them to the signed-in employee).
	"hr_headcount": CapHRAdmin, "hr_attrition": CapHRAdmin, "hr_payroll_summary": CapHRAdmin,
	// Talent: candidate data is personal data, recruiters only.
	"talent_summarise_candidate": CapRecruiter, "talent_find_candidates": CapRecruiter, "talent_draft_job": CapRecruiter,
	"talent_job_context": CapRecruiter, "talent_interview_questions": CapRecruiter, "talent_screen_resumes": CapRecruiter,
	"talent_offer_context": CapRecruiter, "talent_create_job_draft": CapRecruiter, "talent_create_offer_draft": CapRecruiter,
	"talent_note_screening": CapRecruiter,
	// Accounting
	"acct_report": CapLedgerRead, "acct_find_anomalies": CapLedgerRead, "acct_spend": CapLedgerRead,
	"acct_bank_lines": CapLedgerRead, "acct_accounts": CapLedgerRead,
	"acct_categorise_bank_lines": CapLedgerWrite, "acct_draft_journal": CapLedgerWrite, "acct_post_journal": CapLedgerWrite,
	// Inventory
	"inv_low_stock": CapStockRead, "inv_reorder_suggestions": CapStockRead, "inv_find_product": CapStockRead,
	"inv_draft_po": CapProductWrite,
	// Invoicing / Sales
	"sales_overdue": CapDocRead, "sales_aging": CapDocRead, "sales_find_documents": CapDocRead, "sales_customers": CapDocRead,
	"sales_draft_document": CapDocWrite, "sales_untrusted_note": CapDocRead,
	// POS
	"pos_day_summary": CapPOSReport, "pos_find_product": CapPOSRead,
	// Admin
	"admin_effective_access": CapOrgAdmin, "admin_audit_summary": CapOrgAdmin, "admin_policy_suggestions": CapOrgAdmin,
	"admin_list_users": CapOrgAdmin,
}

// bizApps are the apps whose assistant is opened inside a business module.
var bizApps = []string{"crm", "hr", "talent", "shifts", "timetracking", "fieldservice", "fleet", "accounting", "inventory", "invoicing", "pos", "projects", "tickets", "records", "wiki", "contracts", "signatures", "legal", "captable", "podcasts", "admin"}

// allowedByPolicy reports whether the caller may be offered tool.
func (c Caller) allowedByPolicy(tool string) bool {
	need, ok := ToolCaps[tool]
	if !ok {
		return true
	}
	if need == CapOrgAdmin && c.IsAdmin {
		return true
	}
	return c.Can != nil && c.Can(need)
}

// ToolAudit is the browser's report of a tool it ran (POST /api/v1/ai/audit).
// It carries the tool name and the one-line summary shown in the panel, never
// the arguments or results (they can hold personal or financial data).
type ToolAudit struct {
	App     string `json:"app"`
	Tool    string `json:"tool"`
	Risk    string `json:"risk"`
	Status  string `json:"status"` // ok | declined | error
	Summary string `json:"summary,omitempty"`
	Target  string `json:"target,omitempty"` // record id the tool changed, if any
}

var targetRe = regexp.MustCompile(`^[A-Za-z0-9._:/-]{0,80}$`)

// RecordTool writes an audit entry for a tool run in the browser. Only tools
// the caller may be offered are accepted, so the log cannot be filled with
// entries for actions the policy would have hidden.
func (g *Gateway) RecordTool(ctx context.Context, c Caller, a ToolAudit) *APIError {
	if c.OrgID == "" || c.UserID == "" {
		return apiErr(401, "unauthorized", "sign in first")
	}
	if !appNameRe.MatchString(a.App) || !toolNameRe.MatchString(a.Tool) {
		return apiErr(400, "bad_request", "invalid app or tool name")
	}
	switch a.Risk {
	case "read", "write", "destructive":
	default:
		return apiErr(400, "bad_request", "invalid risk")
	}
	switch a.Status {
	case "ok", "declined", "error":
	default:
		return apiErr(400, "bad_request", "invalid status")
	}
	if !targetRe.MatchString(a.Target) {
		a.Target = ""
	}
	s, err := g.Store.GetSettings(ctx, c.OrgID)
	if err != nil {
		return apiErr(500, "internal", "could not load AI settings")
	}
	if s.KillSwitch || !s.AppEnabled(a.App) || !c.Allowed || !c.appOK(a.App) {
		return apiErr(403, "forbidden", "the assistant is not available here")
	}
	if !c.allowedByPolicy(a.Tool) {
		return apiErr(403, "forbidden", "that action is not available to you")
	}
	g.audit(ctx, "tool."+a.Status, map[string]any{
		"resource_type": "ai_" + a.App, "resource_id": a.Target, "app": a.App, "tool": a.Tool,
		"risk": a.Risk, "summary": truncate(strings.TrimSpace(a.Summary), 300),
	})
	return nil
}
