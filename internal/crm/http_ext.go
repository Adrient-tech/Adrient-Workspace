package crm

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// crud wires the standard list/create/update/delete shape for a simple resource.
type crud[T any] struct {
	list func() ([]T, error)
	save func(id string, v T) (T, error)
	del  func(id string) error
}

func serveCRUD[T any](h *HTTPHandler, w http.ResponseWriter, r *http.Request, seg []string, key string, c crud[T], adminOnlyWrite bool) {
	m, n := r.Method, len(seg)
	if m != http.MethodGet && adminOnlyWrite && !h.requireAdmin(w, r) {
		return
	}
	switch {
	case n == 1 && m == http.MethodGet:
		v, err := c.list()
		reply(w, map[string]any{key: v}, err)
	case n == 1 && m == http.MethodPost, n == 2 && m == http.MethodPut:
		var v T
		if !decode(w, r, &v) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		out, err := c.save(id, v)
		reply(w, out, err)
	case n == 2 && m == http.MethodDelete && c.del != nil:
		replyOK(w, c.del(seg[1]))
	default:
		http.NotFound(w, r)
	}
}

func (h *HTTPHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.auth.IsAdmin != nil && h.auth.IsAdmin(r) {
		return true
	}
	http.Error(w, "this needs a CRM administrator", http.StatusForbidden)
	return false
}

func qParam(r *http.Request, k string) string { return strings.TrimSpace(r.URL.Query().Get(k)) }

// serveExt handles every resource added after the original CRM. It reports
// whether it recognised the resource.
func (h *HTTPHandler) serveExt(w http.ResponseWriter, r *http.Request, org, user string, seg []string) bool {
	ctx := r.Context()
	repo := h.repo
	m, n := r.Method, len(seg)
	switch seg[0] {
	case "me":
		switch m {
		case http.MethodGet:
			scope, _ := repo.ResolveScope(ctx, org, user)
			admin := h.auth.IsAdmin != nil && h.auth.IsAdmin(r)
			if admin {
				scope = ScopeAll
			}
			reply(w, map[string]any{"user_id": user, "is_admin": admin, "scope": scope, "auto_log_mail": repo.AutoLogMailEnabled(ctx, org, user)}, nil)
		case http.MethodPut:
			var b struct {
				AutoLogMail bool `json:"auto_log_mail"`
			}
			if decode(w, r, &b) {
				replyOK(w, repo.SetAutoLogMail(ctx, org, user, b.AutoLogMail))
			}
		default:
			http.NotFound(w, r)
		}
	case "users":
		if m != http.MethodGet {
			http.NotFound(w, r)
			return true
		}
		// Only the people CRM actually refers to (record owners, team managers
		// and members, the caller): enough to show names and the existing
		// custom-field user values. New people are picked with the async
		// picker (/api/v1/directory/search), never from a full org list.
		rows, err := repo.pool.Query(ctx, `SELECT u.id::text, u.display_name, u.email FROM adrient.users u
			WHERE u.org_id=$1 AND (u.id=$2::uuid
			   OR u.id IN (SELECT DISTINCT owner_id FROM adrient.crm_people WHERE org_id=$1 AND owner_id IS NOT NULL)
			   OR u.id IN (SELECT DISTINCT owner_id FROM adrient.crm_companies WHERE org_id=$1 AND owner_id IS NOT NULL)
			   OR u.id IN (SELECT DISTINCT owner_id FROM adrient.crm_deals WHERE org_id=$1 AND owner_id IS NOT NULL)
			   OR u.id IN (SELECT manager_id FROM adrient.crm_teams WHERE org_id=$1 AND manager_id IS NOT NULL)
			   OR u.id IN (SELECT m.user_id FROM adrient.crm_team_members m JOIN adrient.crm_teams t ON t.id=m.team_id WHERE t.org_id=$1))
			ORDER BY lower(u.display_name) LIMIT 300`, org, user)
		if err != nil {
			httpErr(w, err)
			return true
		}
		defer rows.Close()
		type u struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		out := []u{}
		for rows.Next() {
			var x u
			_ = rows.Scan(&x.ID, &x.Name, &x.Email)
			out = append(out, x)
		}
		writeJSON(w, map[string]any{"users": out})
	case "fields":
		serveCRUD(h, w, r, seg, "fields", crud[FieldDef]{
			list: func() ([]FieldDef, error) { return repo.ListFieldDefs(ctx, org) },
			save: func(id string, v FieldDef) (FieldDef, error) { return repo.SaveFieldDef(ctx, org, id, v) },
			del:  func(id string) error { return repo.DeleteFieldDef(ctx, org, id) },
		}, true)
	case "views":
		serveCRUD(h, w, r, seg, "views", crud[View]{
			list: func() ([]View, error) { return repo.ListViews(ctx, org, user) },
			save: func(id string, v View) (View, error) { return repo.SaveView(ctx, org, user, id, v) },
			del:  func(id string) error { return repo.DeleteView(ctx, org, user, id) },
		}, false)
	case "search":
		hits, err := repo.Search(ctx, org, qParam(r, "q"), 8)
		reply(w, map[string]any{"hits": hits}, err)
	case "rules":
		serveCRUD(h, w, r, seg, "rules", crud[Rule]{
			list: func() ([]Rule, error) { return repo.ListRules(ctx, org) },
			save: func(id string, v Rule) (Rule, error) { return repo.SaveRule(ctx, org, id, v) },
			del:  func(id string) error { return repo.DeleteRule(ctx, org, id) },
		}, true)
	case "rule-runs":
		runs, err := repo.ListRuleRuns(ctx, org, 100)
		reply(w, map[string]any{"runs": runs}, err)
	case "automation":
		if n == 2 && seg[1] == "sweep" && m == http.MethodPost && h.requireAdmin(w, r) {
			c, err := repo.SweepIdle(ctx)
			reply(w, map[string]any{"fired": c}, err)
			return true
		}
		http.NotFound(w, r)
	case "playbooks":
		serveCRUD(h, w, r, seg, "playbooks", crud[Playbook]{
			list: func() ([]Playbook, error) { return repo.ListPlaybooks(ctx, org) },
			save: func(_ string, v Playbook) (Playbook, error) { return repo.SavePlaybook(ctx, org, v) },
			del:  func(id string) error { return repo.DeletePlaybook(ctx, org, id) },
		}, true)
	case "score-rules":
		serveCRUD(h, w, r, seg, "rules", crud[ScoreRule]{
			list: func() ([]ScoreRule, error) { return repo.ListScoreRules(ctx, org) },
			save: func(id string, v ScoreRule) (ScoreRule, error) { return repo.SaveScoreRule(ctx, org, id, v) },
			del:  func(id string) error { return repo.DeleteScoreRule(ctx, org, id) },
		}, true)
	case "rescore":
		if m == http.MethodPost && h.requireAdmin(w, r) {
			c, err := repo.RescoreAll(ctx, org)
			reply(w, map[string]any{"rescored": c}, err)
			return true
		}
		http.NotFound(w, r)
	case "duplicates":
		g, err := repo.FindDuplicates(ctx, org)
		reply(w, map[string]any{"groups": g}, err)
	case "merge":
		var b struct{ Kind, Winner, Loser string }
		if m != http.MethodPost || !decode(w, r, &b) {
			return true
		}
		switch b.Kind {
		case "company":
			replyOK(w, repo.MergeCompanies(ctx, org, b.Winner, b.Loser))
		case "person":
			replyOK(w, repo.MergePeople(ctx, org, b.Winner, b.Loser))
		default:
			http.Error(w, "kind must be person or company", http.StatusBadRequest)
		}
	case "lead-forms":
		serveCRUD(h, w, r, seg, "forms", crud[LeadForm]{
			list: func() ([]LeadForm, error) { return repo.ListLeadForms(ctx, org) },
			save: func(id string, v LeadForm) (LeadForm, error) { return repo.SaveLeadForm(ctx, org, user, id, v) },
			del:  func(id string) error { return repo.DeleteLeadForm(ctx, org, id) },
		}, true)
	case "form-links":
		serveCRUD(h, w, r, seg, "links", crud[FormLink]{
			list: func() ([]FormLink, error) { return repo.ListFormLinks(ctx, org) },
			save: func(_ string, v FormLink) (FormLink, error) { return repo.SaveFormLink(ctx, org, user, v) },
			del:  func(id string) error { return repo.DeleteFormLink(ctx, org, id) },
		}, true)
	case "templates":
		serveCRUD(h, w, r, seg, "templates", crud[EmailTemplate]{
			list: func() ([]EmailTemplate, error) { return repo.ListTemplates(ctx, org) },
			save: func(id string, v EmailTemplate) (EmailTemplate, error) { return repo.SaveTemplate(ctx, org, id, v) },
			del:  func(id string) error { return repo.DeleteTemplate(ctx, org, id) },
		}, true)
	case "mail":
		switch {
		case n == 2 && seg[1] == "sync" && m == http.MethodPost:
			c, err := repo.SyncMail(ctx, org, user)
			reply(w, map[string]any{"logged": c}, err)
		case n == 2 && seg[1] == "send" && m == http.MethodPost:
			var in SendInput
			if decode(w, r, &in) {
				t, err := repo.SendFromCRM(ctx, org, in)
				reply(w, map[string]any{"thread_id": t}, err)
			}
		default:
			http.NotFound(w, r)
		}
	case "teams":
		serveCRUD(h, w, r, seg, "teams", crud[Team]{
			list: func() ([]Team, error) { return repo.ListTeams(ctx, org) },
			save: func(id string, v Team) (Team, error) { return repo.SaveTeam(ctx, org, id, v) },
			del:  func(id string) error { return repo.DeleteTeam(ctx, org, id) },
		}, true)
	case "territories":
		if n == 2 && seg[1] == "apply" && m == http.MethodPost {
			if h.requireAdmin(w, r) {
				c, err := repo.ApplyTerritories(ctx, org)
				reply(w, map[string]any{"assigned": c}, err)
			}
			return true
		}
		serveCRUD(h, w, r, seg, "territories", crud[Territory]{
			list: func() ([]Territory, error) { return repo.ListTerritories(ctx, org) },
			save: func(id string, v Territory) (Territory, error) { return repo.SaveTerritory(ctx, org, id, v) },
			del:  func(id string) error { return repo.DeleteTerritory(ctx, org, id) },
		}, true)
	case "user-scopes":
		switch m {
		case http.MethodGet:
			s, err := repo.ListUserScopes(ctx, org)
			reply(w, map[string]any{"scopes": s}, err)
		case http.MethodPut:
			var b UserScope
			if h.requireAdmin(w, r) && decode(w, r, &b) {
				replyOK(w, repo.SetUserScope(ctx, org, b.UserID, b.Scope))
			}
		default:
			http.NotFound(w, r)
		}
	case "quotas":
		var q Quota
		if m == http.MethodPut && h.requireAdmin(w, r) && decode(w, r, &q) {
			replyOK(w, repo.SetQuota(ctx, org, q))
			return true
		}
		if m == http.MethodGet {
			rows, err := repo.pool.Query(ctx, `SELECT user_id::text, to_char(period,'YYYY-MM'), target::float8 FROM adrient.crm_quotas WHERE org_id=$1 ORDER BY period DESC`, org)
			if err != nil {
				httpErr(w, err)
				return true
			}
			defer rows.Close()
			out := []Quota{}
			for rows.Next() {
				var x Quota
				_ = rows.Scan(&x.UserID, &x.Period, &x.Target)
				out = append(out, x)
			}
			writeJSON(w, map[string]any{"quotas": out})
		}
	case "leaderboard":
		p := qParam(r, "period")
		if p == "" {
			p = time.Now().Format("2006-01")
		}
		rows, err := repo.Leaderboard(ctx, org, p)
		reply(w, map[string]any{"period": p, "rows": rows}, err)
	case "pricebook":
		if n == 2 && seg[1] == "sync" && m == http.MethodPost {
			if h.requireAdmin(w, r) {
				a, u, err := repo.SyncInventoryProducts(ctx, org)
				reply(w, map[string]any{"added": a, "updated": u}, err)
			}
			return true
		}
		serveCRUD(h, w, r, seg, "items", crud[PriceItem]{
			list: func() ([]PriceItem, error) { return repo.ListPriceBook(ctx, org) },
			save: func(id string, v PriceItem) (PriceItem, error) { return repo.SavePriceItem(ctx, org, id, v) },
			del:  func(id string) error { return repo.DeletePriceItem(ctx, org, id) },
		}, true)
	case "360":
		if n == 3 && m == http.MethodGet {
			c, err := repo.BuildCustomer360(ctx, org, seg[1], seg[2])
			reply(w, c, err)
			return true
		}
		http.NotFound(w, r)
	case "import":
		h.serveImport(w, r, org, user, seg)
	case "reports":
		if n != 2 || m != http.MethodGet {
			http.NotFound(w, r)
			return true
		}
		params := map[string]string{"days": qParam(r, "days")}
		rp, err := repo.RunReport(ctx, org, seg[1], params)
		if err != nil {
			httpErr(w, err)
			return true
		}
		if qParam(r, "format") == "csv" {
			writeCSV(w, seg[1]+".csv", rp.CSV())
			return true
		}
		writeJSON(w, rp)
	case "export":
		if n != 2 || m != http.MethodGet {
			http.NotFound(w, r)
			return true
		}
		name := strings.TrimSuffix(seg[1], ".csv")
		if seg[1] == "all.zip" {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			for _, e := range ExportEntities {
				rp, err := repo.Export(ctx, org, e)
				if err != nil {
					httpErr(w, err)
					return true
				}
				f, _ := zw.Create(e + ".csv")
				_, _ = f.Write(rp.CSV())
			}
			_ = zw.Close()
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", `attachment; filename="crm-export.zip"`)
			_, _ = w.Write(buf.Bytes())
			return true
		}
		rp, err := repo.Export(ctx, org, name)
		if err != nil {
			httpErr(w, err)
			return true
		}
		writeCSV(w, name+".csv", rp.CSV())
	default:
		return false
	}
	return true
}

func writeCSV(w http.ResponseWriter, name string, b []byte) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(b)
}

func (h *HTTPHandler) serveImport(w http.ResponseWriter, r *http.Request, org, user string, seg []string) {
	ctx := r.Context()
	if !h.requireAdmin(w, r) {
		return
	}
	switch {
	case len(seg) == 1 && r.Method == http.MethodPost:
		var req ImportRequest
		if !decodeBig(w, r, &req) {
			return
		}
		rep, err := h.repo.RunImport(ctx, org, user, req)
		reply(w, rep, err)
	case len(seg) == 2 && seg[1] == "preview" && r.Method == http.MethodPost:
		var req ImportRequest
		if !decodeBig(w, r, &req) {
			return
		}
		p, err := h.repo.PreviewImport(ctx, org, req.Source, req.Entity, req.CSV)
		reply(w, p, err)
	case len(seg) == 2 && seg[1] == "jobs" && r.Method == http.MethodGet:
		j, err := h.repo.ListImportJobs(ctx, org)
		reply(w, map[string]any{"jobs": j}, err)
	default:
		http.NotFound(w, r)
	}
}

func decodeBig(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(v); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

var _ = strconv.Itoa
