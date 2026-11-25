package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/crm"
	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/projects"
	"github.com/Adrient-tech/Adrient-Workspace/internal/talent"
)

// bridgeMount is the path prefix of the cross-app hand-off API. Each endpoint
// calls the owning modules' repositories; permissions are re-checked there.
const bridgeMount = "/api/v1/bridge/"

// bridgeHandler wires hand-offs that touch more than one app (hire a
// candidate into HR, CRM deal won into Projects, and the generic link list).
type bridgeHandler struct {
	pool      *pgxpool.Pool
	hr        *hr.Repository
	talent    *talent.Repository
	contacts  *contacts.Repository
	projects  *projects.Repository
	invoicing *invoicing.Repository
	inventory *inventory.Repository
	crm       *crm.Repository
}

func (b *bridgeHandler) Match(p string) bool { return strings.HasPrefix(p, bridgeMount) }

func bridgeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func bridgeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, hr.ErrForbidden), errors.Is(err, talent.ErrForbidden):
		code = http.StatusForbidden
	case errors.Is(err, hr.ErrNotFound), errors.Is(err, talent.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, hr.ErrInvalid), errors.Is(err, talent.ErrInvalid), errors.Is(err, errBridgeInvalid):
		code = http.StatusBadRequest
	case errors.Is(err, hr.ErrConflict):
		code = http.StatusConflict
	}
	msg := err.Error()
	if code == http.StatusInternalServerError {
		msg = "internal error"
	}
	bridgeJSON(w, code, map[string]string{"error": msg})
}

var errBridgeInvalid = errors.New("invalid request")

func (b *bridgeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	o, ok2 := auth.OrgFromContext(r.Context())
	if !ok || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, bridgeMount), "/")
	need := map[string][]string{"talent/hire": {"hr", "talent"}, "crm/deal-won": {"crm", "invoicing"}, "stock/reconciliation": {"inventory", "accounting"}}
	for _, app := range need[path] {
		if !authz.CanOpenApp(r.Context(), app) {
			authz.WriteForbidden(w, app+":app:open")
			return
		}
	}
	switch {
	case path == "talent/hire" && r.Method == http.MethodPost:
		b.hireToHR(w, r, o.ID, u.ID)
	case path == "crm/deal-won" && r.Method == http.MethodPost:
		b.dealWon(w, r, o.ID, u.ID)
	case path == "stock/reconciliation" && r.Method == http.MethodGet:
		b.stockReconciliation(w, r, o.ID)
	case path == "link" && r.Method == http.MethodPost:
		b.manualLink(w, r, o.ID, u.ID, false)
	case path == "unlink" && r.Method == http.MethodPost:
		b.manualLink(w, r, o.ID, u.ID, true)
	case path == "ticket-crm" && r.Method == http.MethodGet:
		b.ticketCRM(w, r, o.ID)
	case path == "links" && r.Method == http.MethodGet:
		b.links(w, r, o.ID, u.ID)
	default:
		http.NotFound(w, r)
	}
}

// ---- Talent -> HR ---------------------------------------------------------

type hireReq struct {
	ApplicationID string `json:"application_id"`
	TemplateID    string `json:"template_id"`
	ManagerEmpID  string `json:"manager_employee_id"`
	CreateProject bool   `json:"create_project"`
}

type hireResp struct {
	Employee      hr.Employee     `json:"employee"`
	Created       bool            `json:"created"`
	ChecklistID   string          `json:"checklist_id"`
	ProjectID     string          `json:"project_id,omitempty"`
	Links         []bridgeLink    `json:"links"`
	SalaryOffer   float64         `json:"salary_offer"`
	Currency      string          `json:"currency"`
	Notes         []string        `json:"notes"`
	ChecklistTask json.RawMessage `json:"-"`
}

type bridgeLink struct {
	App   string `json:"app"`
	Label string `json:"label"`
	Href  string `json:"href"`
}

func (b *bridgeHandler) hireToHR(w http.ResponseWriter, r *http.Request, orgID, userID string) {
	var req hireReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || req.ApplicationID == "" {
		bridgeErr(w, errBridgeInvalid)
		return
	}
	ctx := r.Context()
	ha, err := b.hr.ActorFor(ctx, orgID, userID, authz.IsAdmin(ctx))
	if err != nil {
		bridgeErr(w, err)
		return
	}
	if !ha.IsHR {
		bridgeErr(w, hr.ErrForbidden)
		return
	}
	ta := talent.Actor{OrgID: orgID, UserID: userID, IsHR: true}
	nh, err := b.talent.ConvertToNewHire(ctx, ta, req.ApplicationID)
	if err != nil {
		bridgeErr(w, err)
		return
	}
	in := hr.NewHireInput{Name: nh.Name, Email: nh.Email, Phone: nh.Phone, JobTitle: nh.JobTitle, Department: nh.Department,
		StartDate: nh.StartDate, ManagerEmpID: req.ManagerEmpID, ManagerUser: nh.ManagerID, SourceRef: "talent:" + nh.ApplicationID}
	emp, created, err := b.hr.CreateFromNewHire(ctx, ha, in, func() (string, error) {
		f := contacts.Fields{DisplayName: nh.Name, JobTitle: nh.JobTitle, Labels: []string{"Employee"}}
		if nh.Email != "" {
			f.Emails = []string{nh.Email}
		}
		if nh.Phone != "" {
			f.Phones = []string{nh.Phone}
		}
		c, err := b.contacts.Create(ctx, orgID, userID, f)
		return c.ID, err
	})
	if err != nil {
		bridgeErr(w, err)
		return
	}
	resp := hireResp{Employee: emp, Created: created, SalaryOffer: nh.SalaryOffer, Currency: nh.Currency, Notes: []string{}}
	b.link(ctx, orgID, userID, "talent", "application", nh.ApplicationID, "hr", "employee", emp.ID, "Hired as employee")
	if nh.SalaryOffer > 0 {
		resp.Notes = append(resp.Notes, "The offered salary is not copied: set the salary structure for this employee in HR before the next payroll run.")
	}
	// Onboarding checklist, once per employee.
	cid := b.existingChecklist(ctx, orgID, emp.ID)
	if cid == "" {
		cid, err = b.startOnboarding(ctx, ta, req.TemplateID, nh, emp)
		if err != nil {
			resp.Notes = append(resp.Notes, "Employee created but the onboarding checklist could not be started: "+err.Error())
		} else if cid != "" {
			b.link(ctx, orgID, userID, "hr", "employee", emp.ID, "talent", "checklist", cid, "Onboarding checklist")
		}
	}
	resp.ChecklistID = cid
	if req.CreateProject && b.projects != nil {
		if pid, perr := b.onboardingProject(ctx, orgID, userID, nh, emp, cid); perr == nil {
			resp.ProjectID = pid
			b.link(ctx, orgID, userID, "hr", "employee", emp.ID, "projects", "project", pid, "Onboarding project")
		} else {
			resp.Notes = append(resp.Notes, "Project not created: "+perr.Error())
		}
	}
	resp.Links = b.employeeLinks(ctx, orgID, emp, cid, resp.ProjectID)
	if !created {
		resp.Notes = append(resp.Notes, "This candidate was already hired into HR; nothing was duplicated.")
	}
	bridgeJSON(w, http.StatusOK, resp)
}

func (b *bridgeHandler) existingChecklist(ctx context.Context, orgID, empID string) string {
	var id string
	_ = b.pool.QueryRow(ctx, `SELECT to_id FROM adrient.record_links WHERE org_id=$1 AND from_app='hr' AND from_type='employee' AND from_id=$2
		AND to_app='talent' AND to_type='checklist'`, orgID, empID).Scan(&id)
	return id
}

var defaultOnboarding = []talent.TemplateTask{
	{Title: "Send welcome email and first-day plan", Role: "hr", DueOffset: -3},
	{Title: "Collect joining documents (PAN, bank details, ID proof)", Role: "hr", DueOffset: 0},
	{Title: "Create accounts and set up laptop", Role: "it", DueOffset: -1},
	{Title: "Assign a buddy and plan the first week", Role: "manager", DueOffset: -2},
	{Title: "Set salary structure and bank details in HR", Role: "hr", DueOffset: 0},
	{Title: "Complete profile and read company policies", Role: "employee", DueOffset: 3},
	{Title: "30-day check-in", Role: "manager", DueOffset: 30},
}

func (b *bridgeHandler) startOnboarding(ctx context.Context, ta talent.Actor, templateID string, nh talent.NewHire, emp hr.Employee) (string, error) {
	if templateID == "" {
		raw, err := b.talent.ListTemplates(ctx, ta)
		if err != nil {
			return "", err
		}
		var list []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(raw, &list)
		for _, t := range list {
			if t.Kind == "onboarding" {
				templateID = t.ID
				break
			}
		}
		if templateID == "" {
			saved, err := b.talent.SaveTemplate(ctx, ta, "", talent.TemplateInput{Kind: "onboarding", Name: "New employee onboarding", Tasks: defaultOnboarding})
			if err != nil {
				return "", err
			}
			var t struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(saved, &t)
			templateID = t.ID
		}
	}
	start := emp.JoinDate
	if start == "" {
		start = time.Now().Format("2006-01-02")
	}
	in := talent.StartInput{TemplateID: templateID, SubjectUserID: emp.UserID, AnchorDate: start}
	if emp.UserID == "" {
		in.SubjectName, in.SubjectEmail = emp.Name, emp.WorkEmail
	}
	if mgr := nh.ManagerID; mgr != "" {
		in.ManagerID = mgr
	}
	raw, err := b.talent.StartChecklist(ctx, ta, in)
	if err != nil {
		return "", err
	}
	var c struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &c)
	return c.ID, nil
}

func (b *bridgeHandler) onboardingProject(ctx context.Context, orgID, userID string, nh talent.NewHire, emp hr.Employee, checklistID string) (string, error) {
	p, err := b.projects.CreateProject(ctx, orgID, projects.ProjectFields{Name: "Onboarding: " + emp.Name, Description: "Onboarding of " + emp.Name + " (" + emp.EmpCode + "), starting " + emp.JoinDate + "."})
	if err != nil {
		return "", err
	}
	teamID, err := b.defaultTeam(ctx, orgID)
	if err != nil {
		return p.ID, err
	}
	for _, t := range defaultOnboarding {
		_, _ = b.projects.CreateIssue(ctx, orgID, teamID, userID, projects.IssueFields{Title: t.Title + " (" + emp.Name + ")", ProjectID: p.ID, Status: "todo"})
	}
	return p.ID, nil
}

func (b *bridgeHandler) defaultTeam(ctx context.Context, orgID string) (string, error) {
	ts, err := b.projects.ListTeams(ctx, orgID)
	if err != nil {
		return "", err
	}
	if len(ts) > 0 {
		return ts[0].ID, nil
	}
	t, err := b.projects.CreateTeam(ctx, orgID, "General", "GEN", "", "")
	return t.ID, err
}

func (b *bridgeHandler) employeeLinks(ctx context.Context, orgID string, emp hr.Employee, checklistID, projectID string) []bridgeLink {
	l := []bridgeLink{{App: "hr", Label: "Open in HR", Href: "/hr?view=people&employee=" + emp.ID}}
	if emp.ContactID != "" {
		l = append(l, bridgeLink{App: "contacts", Label: "Open in Contacts", Href: "/contacts?q=" + emp.ContactID})
	}
	if emp.UserID != "" {
		l = append(l, bridgeLink{App: "contacts", Label: "Open in directory", Href: "/org-chart?user=" + emp.UserID})
	}
	if checklistID != "" {
		l = append(l, bridgeLink{App: "talent", Label: "Open onboarding checklist", Href: "/talent?view=onboarding&checklist=" + checklistID})
	}
	if projectID != "" {
		l = append(l, bridgeLink{App: "projects", Label: "Open onboarding project", Href: "/projects?project=" + projectID})
	}
	return l
}

func (b *bridgeHandler) link(ctx context.Context, orgID, userID, fa, ft, fid, ta, tt, tid, label string) {
	_, _ = b.pool.Exec(ctx, `INSERT INTO adrient.record_links (org_id, from_app, from_type, from_id, to_app, to_type, to_id, label, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::uuid) ON CONFLICT DO NOTHING`, orgID, fa, ft, fid, ta, tt, tid, label, userID)
}

// links lists the stored links of a record (both directions) as "open in" targets.
func (b *bridgeHandler) links(w http.ResponseWriter, r *http.Request, orgID, userID string) {
	q := r.URL.Query()
	app, typ, id := q.Get("app"), q.Get("type"), q.Get("id")
	if app == "" || typ == "" || id == "" {
		bridgeErr(w, errBridgeInvalid)
		return
	}
	rows, err := b.pool.Query(r.Context(), `SELECT to_app, to_type, to_id, label FROM adrient.record_links WHERE org_id=$1 AND from_app=$2 AND from_type=$3 AND from_id=$4
		UNION SELECT from_app, from_type, from_id, label FROM adrient.record_links WHERE org_id=$1 AND to_app=$2 AND to_type=$3 AND to_id=$4`, orgID, app, typ, id)
	if err != nil {
		bridgeErr(w, err)
		return
	}
	defer rows.Close()
	out := []bridgeLink{}
	for rows.Next() {
		var a, t, i, label string
		if rows.Scan(&a, &t, &i, &label) == nil && authz.CanOpenApp(r.Context(), a) {
			out = append(out, bridgeLink{App: a, Label: label, Href: recordHref(a, t, i)})
		}
	}
	bridgeJSON(w, http.StatusOK, map[string]any{"links": out})
}

// recordHref is the in-app route that shows one record.
func recordHref(app, typ, id string) string {
	switch app + "/" + typ {
	case "hr/employee":
		return "/hr?view=people&employee=" + id
	case "talent/application":
		return "/talent?view=jobs&application=" + id
	case "talent/checklist":
		return "/talent?view=onboarding&checklist=" + id
	case "projects/project":
		return "/projects?project=" + id
	case "invoicing/document":
		return "/invoicing?doc=" + id
	case "crm/deal":
		return "/crm?deal=" + id
	case "crm/company":
		return "/crm?company=" + id
	case "tickets/ticket":
		return "/tickets?ticket=" + id
	case "projects/issue":
		return "/projects?issue=" + id
	case "mail/message":
		return "/mail"
	case "chat/message":
		return "/chat"
	case "contacts/contact":
		return "/contacts?q=" + id
	case "accounting/journal":
		return "/accounting?journal=" + id
	}
	return "/" + app
}

// ---- CRM deal won -> estimate (+ project) ----------------------------------

type dealRow struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	CompanyID string          `json:"company_id"`
	ContactID string          `json:"contact_id"`
	Stage     string          `json:"stage"`
	Value     float64         `json:"value"`
	Currency  string          `json:"currency"`
	OwnerID   string          `json:"owner_id"`
	Items     []crmItemRow    `json:"items"`
	Raw       json.RawMessage `json:"-"`
}

type crmItemRow struct {
	Name        string  `json:"name"`
	Qty         float64 `json:"qty"`
	UnitPrice   float64 `json:"unit_price"`
	DiscountPct float64 `json:"discount_pct"`
}

type dealWonResult struct {
	EstimateID string       `json:"estimate_id"`
	Created    bool         `json:"created"`
	ProjectID  string       `json:"project_id,omitempty"`
	Links      []bridgeLink `json:"links"`
	Notes      []string     `json:"notes"`
}

// DealWon creates the draft estimate for a won deal (idempotent: one live
// estimate per deal) and, when asked, a Projects project. It is called by the
// CRM hook when a deal enters Won and by the "Create estimate and project" button.
func (b *bridgeHandler) DealWon(ctx context.Context, orgID, userID, dealID string, withProject bool) (dealWonResult, error) {
	res := dealWonResult{Links: []bridgeLink{}, Notes: []string{}}
	raw, err := b.crm.GetDeal(ctx, orgID, dealID)
	if err != nil {
		return res, err
	}
	var d dealRow
	if err := json.Unmarshal(raw, &d); err != nil {
		return res, err
	}
	if userID == "" {
		userID = d.OwnerID
	}
	in := invoicing.DealEstimate{DealID: d.ID, Title: d.Title, ContactID: d.ContactID, ClientName: d.Title, Currency: d.Currency, Value: d.Value, Notes: "CRM deal: " + d.Title}
	if d.ContactID != "" && b.contacts != nil {
		if c, err := b.contacts.Get(ctx, orgID, d.ContactID); err == nil {
			in.ClientName = c.DisplayName
			if len(c.Emails) > 0 {
				in.ClientEmail = c.Emails[0]
			}
			if c.Company != "" {
				in.ClientName = c.DisplayName + " (" + c.Company + ")"
			}
		}
	}
	for _, i := range d.Items {
		in.Lines = append(in.Lines, invoicing.Line{Description: i.Name, Qty: i.Qty, Price: i.UnitPrice, DiscountPct: i.DiscountPct})
	}
	doc, created, err := b.invoicing.CreateEstimateFromDeal(ctx, orgID, userID, in)
	if err != nil {
		return res, err
	}
	res.EstimateID, res.Created = doc.ID, created
	b.link(ctx, orgID, userID, "crm", "deal", d.ID, "invoicing", "document", doc.ID, "Estimate")
	res.Links = append(res.Links, bridgeLink{App: "invoicing", Label: "Open estimate in Invoicing", Href: recordHref("invoicing", "document", doc.ID)})
	if withProject && b.projects != nil {
		var existing string
		_ = b.pool.QueryRow(ctx, `SELECT to_id FROM adrient.record_links WHERE org_id=$1 AND from_app='crm' AND from_type='deal' AND from_id=$2 AND to_app='projects'`, orgID, d.ID).Scan(&existing)
		if existing == "" {
			p, perr := b.projects.CreateProject(ctx, orgID, projects.ProjectFields{Name: "Delivery: " + d.Title, Description: "Created from won CRM deal " + d.Title})
			if perr != nil {
				res.Notes = append(res.Notes, "Project not created: "+perr.Error())
			} else {
				existing = p.ID
				if tid, terr := b.defaultTeam(ctx, orgID); terr == nil {
					for _, t := range []string{"Kick-off with " + d.Title, "Deliver scope of " + d.Title, "Final handover and feedback"} {
						_, _ = b.projects.CreateIssue(ctx, orgID, tid, userID, projects.IssueFields{Title: t, ProjectID: p.ID, Status: "todo"})
					}
				}
				b.link(ctx, orgID, userID, "crm", "deal", d.ID, "projects", "project", p.ID, "Delivery project")
			}
		}
		if existing != "" {
			res.ProjectID = existing
			res.Links = append(res.Links, bridgeLink{App: "projects", Label: "Open project", Href: recordHref("projects", "project", existing)})
		}
	}
	return res, nil
}

func (b *bridgeHandler) dealWon(w http.ResponseWriter, r *http.Request, orgID, userID string) {
	var req struct {
		DealID  string `json:"deal_id"`
		Project bool   `json:"create_project"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || req.DealID == "" {
		bridgeErr(w, errBridgeInvalid)
		return
	}
	ctx := r.Context()
	scope := crm.ScopeAll
	if !authz.IsAdmin(ctx) {
		sc, err := b.crm.ResolveScope(ctx, orgID, userID)
		if err != nil {
			bridgeErr(w, err)
			return
		}
		scope = sc
	}
	ctx = crm.WithViewer(ctx, crm.Viewer{UserID: userID, Scope: scope})
	res, err := b.DealWon(ctx, orgID, userID, req.DealID, req.Project)
	if err != nil {
		if errors.Is(err, crm.ErrNotFound) {
			err = hr.ErrNotFound
		}
		bridgeErr(w, err)
		return
	}
	bridgeJSON(w, http.StatusOK, res)
}

// ---- Stock value vs the ledger ----------------------------------------------

// StockReconciliation explains the gap between the stock valuation report and
// the Inventory account of the chart. Goods enter the ledger when the supplier
// bill is posted, not at receipt, so received-but-unbilled goods make the
// valuation larger than the ledger until the bill is posted.
type StockReconciliation struct {
	ValuationTotal  float64 `json:"valuation_total"`
	LedgerInventory float64 `json:"ledger_inventory"`
	UnbilledReceipt float64 `json:"unbilled_receipts"`
	OtherDifference float64 `json:"other_difference"`
	PendingJournals int     `json:"pending_journals"`
	FailedJournals  int     `json:"failed_journals"`
	Matches         bool    `json:"matches"`
	Explanation     string  `json:"explanation"`
}

func (b *bridgeHandler) stockReconciliation(w http.ResponseWriter, r *http.Request, orgID string) {
	ctx := r.Context()
	var rec StockReconciliation
	if b.inventory == nil {
		bridgeErr(w, errBridgeInvalid)
		return
	}
	val, err := b.inventory.ValuationAsOf(ctx, orgID, "", "")
	if err != nil {
		bridgeErr(w, err)
		return
	}
	rec.ValuationTotal = round2(val.TotalValue)
	_ = b.pool.QueryRow(ctx, `SELECT coalesce(sum(l.debit-l.credit),0)::float8 FROM adrient.acct_lines l
		JOIN adrient.acct_journals j ON j.id=l.journal_id JOIN adrient.acct_accounts a ON a.id=l.account_id
		WHERE l.org_id=$1 AND NOT j.voided AND a.system_key='inventory'`, orgID).Scan(&rec.LedgerInventory)
	rec.LedgerInventory = round2(rec.LedgerInventory)
	var received, billed float64
	_ = b.pool.QueryRow(ctx, `SELECT coalesce(sum(gl.qty*gl.unit_cost),0)::float8 FROM adrient.inv_grn_lines gl
		JOIN adrient.inv_grns g ON g.id=gl.grn_id WHERE g.org_id=$1 AND g.po_id IS NOT NULL`, orgID).Scan(&received)
	_ = b.pool.QueryRow(ctx, `SELECT coalesce(sum(bl.qty*bl.unit_cost),0)::float8 FROM adrient.inv_supplier_bill_lines bl
		JOIN adrient.inv_supplier_bills sb ON sb.id=bl.bill_id WHERE sb.org_id=$1 AND sb.state='posted' AND sb.po_id IS NOT NULL`, orgID).Scan(&billed)
	if received > billed {
		rec.UnbilledReceipt = round2(received - billed)
	}
	if b.inventory != nil {
		rec.PendingJournals, rec.FailedJournals, _ = b.inventory.PendingJournalCount(ctx, orgID)
	}
	rec.OtherDifference = round2(rec.ValuationTotal - rec.LedgerInventory - rec.UnbilledReceipt)
	rec.Matches = rec.OtherDifference == 0 && rec.UnbilledReceipt == 0
	switch {
	case rec.Matches:
		rec.Explanation = "Stock value equals the Inventory account."
	case rec.UnbilledReceipt > 0 && rec.OtherDifference == 0:
		rec.Explanation = "The gap is goods received but not billed yet. Post the supplier bills and the ledger catches up."
	default:
		rec.Explanation = "Part of the gap is not explained by unbilled receipts: opening stock or manual stock changes without a journal, or entries still waiting to be posted to Accounting."
	}
	bridgeJSON(w, http.StatusOK, rec)
}

func round2(f float64) float64 {
	if f < 0 {
		return -float64(int64(-f*100+0.5)) / 100
	}
	return float64(int64(f*100+0.5)) / 100
}

// ---- Explicit record links (Tickets -> CRM company, deal -> project) -----------

// linkRules lists the record pairs a user may link by hand, and the query that
// proves a record exists in the org. $1 = org id, $2 = record id.
var linkExists = map[string]string{
	"tickets/ticket":   `SELECT true FROM adrient.tickets WHERE org_id=$1 AND id::text=$2`,
	"crm/company":      `SELECT true FROM adrient.crm_companies WHERE org_id=$1 AND id::text=$2`,
	"crm/person":       `SELECT true FROM adrient.crm_people WHERE org_id=$1 AND id::text=$2`,
	"crm/deal":         `SELECT true FROM adrient.crm_deals WHERE org_id=$1 AND id::text=$2`,
	"projects/project": `SELECT true FROM adrient.project_projects WHERE org_id=$1 AND id::text=$2`,
}

var linkAllowed = map[string]bool{
	"tickets/ticket>crm/company": true,
	"tickets/ticket>crm/person":  true,
	"crm/deal>projects/project":  true,
}

type linkReq struct {
	FromApp  string `json:"from_app"`
	FromType string `json:"from_type"`
	FromID   string `json:"from_id"`
	ToApp    string `json:"to_app"`
	ToType   string `json:"to_type"`
	ToID     string `json:"to_id"`
	Label    string `json:"label"`
}

func (b *bridgeHandler) manualLink(w http.ResponseWriter, r *http.Request, orgID, userID string, remove bool) {
	var q linkReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&q); err != nil {
		bridgeErr(w, errBridgeInvalid)
		return
	}
	from, to := q.FromApp+"/"+q.FromType, q.ToApp+"/"+q.ToType
	if !linkAllowed[from+">"+to] || q.FromID == "" || q.ToID == "" || len(q.Label) > 120 {
		bridgeErr(w, errBridgeInvalid)
		return
	}
	ctx := r.Context()
	for _, app := range []string{q.FromApp, q.ToApp} {
		if !authz.CanOpenApp(ctx, app) {
			authz.WriteForbidden(w, app+":app:open")
			return
		}
	}
	if remove {
		_, err := b.pool.Exec(ctx, `DELETE FROM adrient.record_links WHERE org_id=$1 AND from_app=$2 AND from_type=$3 AND from_id=$4 AND to_app=$5 AND to_type=$6 AND to_id=$7`,
			orgID, q.FromApp, q.FromType, q.FromID, q.ToApp, q.ToType, q.ToID)
		if err != nil {
			bridgeErr(w, err)
			return
		}
		bridgeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	for _, side := range [][2]string{{from, q.FromID}, {to, q.ToID}} {
		var ok bool
		if err := b.pool.QueryRow(ctx, linkExists[side[0]], orgID, side[1]).Scan(&ok); err != nil || !ok {
			bridgeErr(w, hr.ErrNotFound)
			return
		}
	}
	label := q.Label
	if label == "" {
		label = "Linked " + q.ToType
	}
	b.link(ctx, orgID, userID, q.FromApp, q.FromType, q.FromID, q.ToApp, q.ToType, q.ToID, label)
	bridgeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ticketCRM returns the CRM company or person a ticket relates to: explicit
// links first, otherwise whoever owns the requester's e-mail address.
func (b *bridgeHandler) ticketCRM(w http.ResponseWriter, r *http.Request, orgID string) {
	ctx := r.Context()
	tid := r.URL.Query().Get("ticket_id")
	type item struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Name   string `json:"name"`
		Href   string `json:"href"`
		Source string `json:"source"`
	}
	out := []item{}
	var email string
	if err := b.pool.QueryRow(ctx, `SELECT lower(requester_email) FROM adrient.tickets WHERE org_id=$1 AND id::text=$2`, orgID, tid).Scan(&email); err != nil {
		bridgeErr(w, hr.ErrNotFound)
		return
	}
	if authz.CanOpenApp(ctx, "crm") {
		rows, err := b.pool.Query(ctx, `SELECT 'company', c.id::text, c.name, 'link' FROM adrient.record_links l JOIN adrient.crm_companies c ON c.id::text=l.to_id
			WHERE l.org_id=$1 AND l.from_app='tickets' AND l.from_id=$2 AND l.to_app='crm' AND l.to_type='company'
			UNION ALL
			SELECT 'person', p.id::text, ct.display_name, 'link' FROM adrient.record_links l JOIN adrient.crm_people p ON p.id::text=l.to_id JOIN adrient.contacts ct ON ct.id=p.contact_id
			WHERE l.org_id=$1 AND l.from_app='tickets' AND l.from_id=$2 AND l.to_app='crm' AND l.to_type='person'
			UNION ALL
			SELECT 'person', p.id::text, ct.display_name, 'email' FROM adrient.crm_people p JOIN adrient.contacts ct ON ct.id=p.contact_id
			WHERE p.org_id=$1 AND $3<>'' AND ct.emails::text ILIKE '%'||$3||'%' AND ct.trashed_at IS NULL LIMIT 10`, orgID, tid, email)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var it item
				if rows.Scan(&it.Kind, &it.ID, &it.Name, &it.Source) == nil {
					it.Href = "/crm?" + it.Kind + "=" + it.ID
					out = append(out, it)
				}
			}
		}
	}
	bridgeJSON(w, http.StatusOK, map[string]any{"crm": out})
}
