package iamops

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Paths served by this package.
const (
	AdminBase = "/api/v1/admin/iam"
	MyBase    = "/api/v1/iam/my"
)

// Match reports whether a path belongs to the handler.
func Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == AdminBase || strings.HasPrefix(p, AdminBase+"/") || p == MyBase || strings.HasPrefix(p, MyBase+"/")
}

// Handler serves the console API.
type Handler struct {
	Svc *Service
	// IsOrgAdmin reports whether the caller holds the organisation admin permission.
	IsOrgAdmin func(ctx context.Context) bool
	// IsSuper reports whether the caller is a super admin (owner or bootstrap admin).
	IsSuper func(ctx context.Context) bool
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrStepUp):
		writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "Confirm your password to continue.", "step_up": true})
	case errors.Is(err, ErrBad):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": strings.TrimPrefix(err.Error(), ErrBad.Error()+": ")})
	case errors.Is(err, ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]any{"error": strings.TrimPrefix(err.Error(), ErrForbidden.Error()+": ")})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	default:
		msg := err.Error()
		if strings.Contains(msg, "identity provider") {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": msg})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "request failed"})
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		fail(w, bad("the request body is not valid"))
		return false
	}
	return true
}

func intQ(r *http.Request, k string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(k)); err == nil {
		return v
	}
	return def
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	ctx := r.Context()
	tok, _ := auth.SessionTokenFromContext(ctx)
	a := Actor{ID: u.ID, Email: u.Email, SessionKey: ""}
	if tok != "" {
		a.SessionKey = SessionKey(tok)
	}
	p := strings.TrimRight(r.URL.Path, "/")
	if strings.HasPrefix(p, MyBase) {
		h.serveMy(w, r, o.ID, a, tok, strings.TrimPrefix(p, MyBase))
		return
	}
	a.Roles = h.Svc.Roles(ctx, o.ID, u.ID, h.IsOrgAdmin != nil && h.IsOrgAdmin(ctx))
	a.Super = h.IsSuper != nil && h.IsSuper(ctx)
	if len(a.Roles) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "You do not have an identity operations role."})
		return
	}
	rest := strings.TrimPrefix(p, AdminBase)
	seg := strings.Split(strings.Trim(rest, "/"), "/")
	s := h.Svc
	need := func(c string) bool {
		if Has(a.Roles, c) {
			return true
		}
		fail(w, ErrForbidden)
		return false
	}
	q := r.URL.Query()
	switch {
	case rest == "/me" && r.Method == http.MethodGet:
		until := s.StepUpUntil(ctx, a.SessionKey)
		var su *time.Time
		if !until.IsZero() {
			su = &until
		}
		writeJSON(w, 200, map[string]any{"roles": a.Roles, "super": a.Super, "capabilities": Caps(a.Roles), "step_up_until": su, "email": u.Email, "user_id": u.ID,
			"idp_configured": s.IdP != nil, "summary": s.Summary(ctx, o.ID), "role_catalogue": Roles})
	case rest == "/step-up" && r.Method == http.MethodPost:
		var b struct{ Password, Code string }
		if !decode(w, r, &b) {
			return
		}
		if err := s.VerifyStepUp(ctx, a, b.Password, strings.TrimSpace(b.Code)); err != nil {
			fail(w, err)
			return
		}
		until := s.StepUpUntil(ctx, a.SessionKey)
		writeJSON(w, 200, map[string]any{"ok": true, "step_up_until": until})
	case rest == "/users" && r.Method == http.MethodGet:
		if !need(CapViewUsers) {
			return
		}
		list, next, total, err := s.ListPeople(ctx, o.ID, q.Get("q"), q.Get("cursor"), intQ(r, "limit", 50))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": list, "next_cursor": next, "total": total})
	case len(seg) == 3 && seg[0] == "users" && seg[2] == "security" && r.Method == http.MethodGet:
		if !need(CapViewUsers) {
			return
		}
		v, err := s.UserSecurity(ctx, o.ID, a, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, v)
	case len(seg) == 3 && seg[0] == "users" && seg[2] == "actions" && r.Method == http.MethodPost:
		var b ActionReq
		if !decode(w, r, &b) {
			return
		}
		delete(b.Params, "_approved")
		res, err := s.Do(ctx, o.ID, a, seg[1], b)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, res)
	case rest == "/events" && r.Method == http.MethodGet:
		if !need(CapViewEvents) {
			return
		}
		list, next, err := s.ListEvents(ctx, o.ID, EventFilter{UserID: q.Get("user_id"), IP: q.Get("ip"), Q: q.Get("q"), Result: q.Get("result"), Cursor: q.Get("cursor"), Limit: intQ(r, "limit", 50)})
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": list, "next_cursor": next})
	case rest == "/events/ingest" && r.Method == http.MethodPost:
		if !need(CapPolicyEdit) {
			return
		}
		var b struct {
			Events []Event `json:"events"`
		}
		if !decode(w, r, &b) {
			return
		}
		if len(b.Events) == 0 || len(b.Events) > 500 {
			fail(w, bad("send between 1 and 500 events"))
			return
		}
		type out struct {
			ID       int64     `json:"id"`
			Findings []Finding `json:"findings"`
		}
		res := []out{}
		for _, e := range b.Events {
			id, f, err := s.InsertEvent(ctx, o.ID, e)
			if err != nil {
				fail(w, err)
				return
			}
			res = append(res, out{id, f})
		}
		writeJSON(w, 200, map[string]any{"results": res})
	case rest == "/alerts" && r.Method == http.MethodGet:
		if !need(CapViewAlerts) {
			return
		}
		f := AlertFilter{Status: q.Get("status"), Severity: q.Get("severity"), Assignee: q.Get("assignee"), Q: q.Get("q"), UserID: q.Get("user_id"), Cursor: q.Get("cursor"), Limit: intQ(r, "limit", 50)}
		if f.Assignee == "me" {
			f.Assignee = u.ID
		}
		list, next, total, err := s.ListAlerts(ctx, o.ID, f)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": list, "next_cursor": next, "total": total, "summary": s.Summary(ctx, o.ID)})
	case rest == "/alerts/bulk" && r.Method == http.MethodPost:
		var b struct {
			IDs        []string `json:"ids"`
			Status     string   `json:"status"`
			AssigneeID *string  `json:"assignee_id"`
		}
		if !decode(w, r, &b) {
			return
		}
		as := ""
		if b.AssigneeID != nil {
			as = *b.AssigneeID
		}
		n, err := s.BulkAlerts(ctx, o.ID, a, b.IDs, b.Status, as, b.AssigneeID != nil)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"changed": n})
	case len(seg) >= 2 && seg[0] == "alerts":
		if !need(CapViewAlerts) {
			return
		}
		id := seg[1]
		switch {
		case len(seg) == 2 && r.Method == http.MethodGet:
			d, err := s.GetAlert(ctx, o.ID, id)
			if err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, 200, d)
		case len(seg) == 2 && r.Method == http.MethodPatch:
			var b struct {
				Status     string  `json:"status"`
				AssigneeID *string `json:"assignee_id"`
				Note       string  `json:"note"`
			}
			if !decode(w, r, &b) {
				return
			}
			as := ""
			if b.AssigneeID != nil {
				as = *b.AssigneeID
			}
			if err := s.UpdateAlert(ctx, o.ID, a, id, b.Status, as, b.AssigneeID != nil, b.Note); err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true})
		case len(seg) == 3 && seg[2] == "comments" && r.Method == http.MethodPost:
			var b struct{ Body string }
			if !decode(w, r, &b) {
				return
			}
			if err := s.Comment(ctx, o.ID, a, id, b.Body); err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true})
		case len(seg) == 3 && seg[2] == "contain" && r.Method == http.MethodPost:
			var b ActionReq
			if !decode(w, r, &b) {
				return
			}
			d, err := s.GetAlert(ctx, o.ID, id)
			if err != nil {
				fail(w, err)
				return
			}
			if d.Alert.UserID == "" {
				fail(w, bad("this alert is about an address, not an account. Block the address instead"))
				return
			}
			b.Action = "contain"
			if b.Params == nil {
				b.Params = map[string]any{}
			}
			b.Params["alert_id"] = id
			res, err := s.Do(ctx, o.ID, a, d.Alert.UserID, b)
			if err != nil {
				fail(w, err)
				return
			}
			if !b.DryRun && res.Status == "done" {
				_ = s.UpdateAlert(ctx, o.ID, Actor{ID: a.ID, Roles: []string{RoleIdentityAdmin}}, id, "compromised", "", false, "Contained by "+a.Email)
			}
			writeJSON(w, 200, res)
		default:
			writeJSON(w, 404, map[string]any{"error": "not found"})
		}
	case rest == "/settings" && r.Method == http.MethodGet:
		if !need(CapViewAlerts) {
			return
		}
		writeJSON(w, 200, map[string]any{"settings": s.GetSettings(ctx, o.ID), "rules": RuleNames})
	case rest == "/settings" && r.Method == http.MethodPut:
		if !need(CapPolicyEdit) {
			return
		}
		if !NeedsStepUpOK(s, ctx, a, "policy.edit") {
			fail(w, ErrStepUp)
			return
		}
		var b Settings
		if !decode(w, r, &b) {
			return
		}
		st, err := s.PutSettings(ctx, o.ID, a.ID, b)
		if err != nil {
			fail(w, err)
			return
		}
		note := ""
		if st.MFAPolicy.Mode == "all" || st.MFAPolicy.Mode == "off" {
			if err := s.applyMFAPolicy(ctx, u.ID, o.ID, st.MFAPolicy.Mode == "all"); err != nil {
				note = "Saved, but the identity provider's login policy could not be updated: " + err.Error()
			}
		}
		writeJSON(w, 200, map[string]any{"settings": st, "note": note})
	case rest == "/lists" && r.Method == http.MethodGet:
		if !need(CapViewAlerts) {
			return
		}
		l, err := s.ListRules(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": l})
	case rest == "/lists" && r.Method == http.MethodPost:
		if !need(CapListEdit) {
			return
		}
		if !NeedsStepUpOK(s, ctx, a, "list.edit") {
			fail(w, ErrStepUp)
			return
		}
		var b struct {
			IPRule
			Hours int `json:"hours"`
		}
		if !decode(w, r, &b) {
			return
		}
		ctx2 := context.WithValue(ctx, ipKey{}, clientIP(r))
		rule, err := s.AddRule(ctx2, o.ID, a, b.IPRule, b.Hours)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 201, rule)
	case len(seg) == 2 && seg[0] == "lists" && r.Method == http.MethodDelete:
		if err := s.DeleteRule(ctx, o.ID, a, seg[1]); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case rest == "/roles" && r.Method == http.MethodGet:
		l, err := s.ListRoleAssignments(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": l, "catalogue": Roles})
	case rest == "/roles" && (r.Method == http.MethodPost || r.Method == http.MethodDelete):
		var b struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		}
		if !decode(w, r, &b) {
			return
		}
		if err := s.AssignRole(ctx, o.ID, a, b.UserID, b.Role, r.Method == http.MethodDelete); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case rest == "/approvals" && r.Method == http.MethodGet:
		if !need(CapViewActionLog) && !Has(a.Roles, CapApprove) {
			return
		}
		l, err := s.ListApprovals(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": l})
	case len(seg) == 3 && seg[0] == "approvals" && r.Method == http.MethodPost && (seg[2] == "approve" || seg[2] == "reject"):
		res, err := s.Decide(ctx, o.ID, a, seg[1], seg[2] == "approve")
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, res)
	case rest == "/actions" && r.Method == http.MethodGet:
		if !need(CapViewActionLog) {
			return
		}
		l, next, err := s.ListActions(ctx, o.ID, q.Get("user_id"), q.Get("cursor"), intQ(r, "limit", 50))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": l, "next_cursor": next})
	case len(seg) == 2 && seg[0] == "reports" && r.Method == http.MethodGet:
		if !need(CapReports) {
			return
		}
		rep, err := s.RunReport(ctx, o.ID, seg[1], intQ(r, "days", 90))
		if err != nil {
			fail(w, err)
			return
		}
		if q.Get("format") == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="`+rep.Key+`.csv"`)
			cw := csv.NewWriter(w)
			_ = cw.Write(rep.Columns)
			for _, row := range rep.Rows {
				for i := range row {
					row[i] = csvSafe(row[i])
				}
				_ = cw.Write(row)
			}
			cw.Flush()
			s.audit(ctx, o.ID, a.ID, "report.export", map[string]any{"report": rep.Key, "rows": len(rep.Rows)})
			return
		}
		writeJSON(w, 200, rep)
	default:
		writeJSON(w, 404, map[string]any{"error": "not found"})
	}
}

// csvSafe stops a cell from being read as a formula by a spreadsheet.
func csvSafe(v string) string {
	if v != "" && strings.ContainsAny(v[:1], "=+-@\t\r") {
		return "'" + v
	}
	return v
}

type ipKey struct{}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	h := r.RemoteAddr
	if i := strings.LastIndex(h, ":"); i > 0 {
		h = h[:i]
	}
	return strings.Trim(h, "[]")
}

// applyMFAPolicy turns the IdP's "second step required" policy on or off for the person's IdP org.
func (s *Service) applyMFAPolicy(ctx context.Context, userID, orgID string, force bool) error {
	if s.IdP == nil {
		return errors.New("identity provider not configured")
	}
	t, err := s.loadTarget(ctx, orgID, userID)
	if err != nil {
		return err
	}
	owner, err := s.IdP.ResourceOwner(ctx, t.Subject)
	if err != nil {
		return err
	}
	return s.IdP.SetForceMFA(ctx, owner, force)
}

// serveMy handles the signed-in person's own security page.
func (h *Handler) serveMy(w http.ResponseWriter, r *http.Request, orgID string, a Actor, tok, rest string) {
	ctx := r.Context()
	s := h.Svc
	seg := strings.Split(strings.Trim(rest, "/"), "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		v, err := s.My(ctx, orgID, a)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, v)
	case rest == "/sessions/revoke-others" && r.Method == http.MethodPost:
		n := s.SignOutOthers(ctx, a.ID, tok)
		s.audit(ctx, orgID, a.ID, "self.sign_out_others", map[string]any{"count": n})
		writeJSON(w, 200, map[string]any{"revoked": n})
	case len(seg) == 3 && seg[0] == "sessions" && seg[2] == "revoke" && r.Method == http.MethodPost:
		if !s.RevokeOwnSession(ctx, a.ID, seg[1]) {
			fail(w, ErrNotFound)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case rest == "/report" && r.Method == http.MethodPost:
		var b struct {
			EventID int64  `json:"event_id"`
			Verdict string `json:"verdict"`
			Note    string `json:"note"`
		}
		if !decode(w, r, &b) {
			return
		}
		id, err := s.ReportActivity(ctx, orgID, a, tok, b.EventID, b.Verdict, b.Note)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "alert_id": id})
	case rest == "/password" && r.Method == http.MethodPost:
		var b struct{ Current, New string }
		if !decode(w, r, &b) {
			return
		}
		if err := s.ChangeOwnPassword(ctx, orgID, a, b.Current, b.New); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case rest == "/factors/totp/start" && r.Method == http.MethodPost:
		var b struct{ Password string }
		if !decode(w, r, &b) {
			return
		}
		secret, uri, err := s.StartOwnTOTP(ctx, orgID, a, b.Password)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"secret": secret, "uri": uri})
	case rest == "/factors/totp/verify" && r.Method == http.MethodPost:
		var b struct{ Code string }
		if !decode(w, r, &b) {
			return
		}
		if err := s.VerifyOwnTOTP(ctx, orgID, a, b.Code); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case rest == "/factors/remove" && r.Method == http.MethodPost:
		var b struct{ Kind, ID, Password string }
		if !decode(w, r, &b) {
			return
		}
		if err := s.RemoveOwnFactor(ctx, orgID, a, b.Kind, b.ID, b.Password); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	default:
		writeJSON(w, 404, map[string]any{"error": "not found"})
	}
}

// PostWebhook delivers an alert to the org's webhook address (best effort, short timeout).
func PostWebhook(ctx context.Context, url string, payload map[string]any) {
	b, _ := json.Marshal(payload)
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(c, http.MethodPost, url, strings.NewReader(string(b)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}
