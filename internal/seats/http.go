package seats

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Path prefixes served by Handler.
const (
	SeatsBase  = "/api/v1/admin/seats"
	SigninBase = "/api/v1/admin/signin"
)

// Handler serves seats, licences, bulk invite, session policy and login history.
type Handler struct {
	Svc     *Service
	BaseURL func(r *http.Request) string
}

// Match reports whether path belongs to this handler.
func Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == SeatsBase || strings.HasPrefix(p, SeatsBase+"/") || p == SigninBase || strings.HasPrefix(p, SigninBase+"/")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	code, msg := http.StatusInternalServerError, "request failed"
	switch {
	case errors.Is(err, ErrBad):
		code, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrNoSeats):
		code, msg = http.StatusConflict, err.Error()
	}
	writeJSON(w, code, map[string]any{"error": msg})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	billing := strings.HasPrefix(r.URL.Path, SeatsBase)
	area := "billing"
	if !billing {
		area = "sessions"
	}
	perm := "admin:" + area + ":read"
	if r.Method != http.MethodGet {
		perm = "admin:" + area + ":write"
	}
	if !authz.IsAdmin(r.Context()) && !authz.Can(r.Context(), perm) {
		authz.WriteForbidden(w, perm)
		return
	}
	ctx := r.Context()
	base := SeatsBase
	if !billing {
		base = SigninBase
	}
	seg := strings.Split(strings.Trim(strings.TrimPrefix(strings.TrimRight(r.URL.Path, "/"), base), "/"), "/")
	in := func(v any) bool { return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v) == nil }
	switch {
	case billing && seg[0] == "usage" && r.Method == http.MethodGet:
		us, err := h.Svc.GetUsage(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, us)
	case billing && seg[0] == "plan" && r.Method == http.MethodPut:
		var p Plan
		if !in(&p) {
			fail(w, ErrBad)
			return
		}
		if err := h.Svc.PutPlan(ctx, o.ID, u.ID, p); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	case billing && seg[0] == "report" && r.Method == http.MethodGet:
		rep, err := h.Svc.LicenseReport(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rep)
	case billing && seg[0] == "caps" && len(seg) == 2 && r.Method == http.MethodPut:
		var b struct {
			Seats *int `json:"seats"`
		}
		if !in(&b) {
			fail(w, ErrBad)
			return
		}
		if err := h.Svc.SetCap(ctx, o.ID, u.ID, seg[1], b.Seats); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case billing && seg[0] == "assign" && r.Method == http.MethodPost:
		var b struct {
			AppID       string `json:"app_id"`
			SubjectType string `json:"subject_type"`
			SubjectID   string `json:"subject_id"`
			Assign      bool   `json:"assign"`
		}
		if !in(&b) {
			fail(w, ErrBad)
			return
		}
		if err := h.Svc.Assign(ctx, o.ID, u.ID, b.AppID, b.SubjectType, b.SubjectID, b.Assign); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case billing && seg[0] == "invite-csv" && r.Method == http.MethodPost:
		var rd io.Reader = r.Body
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				fail(w, ErrBad)
				return
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				fail(w, ErrBad)
				return
			}
			defer f.Close()
			rd = f
		}
		rows, err := h.Svc.BulkInvite(ctx, o.ID, u.ID, o.DisplayName, h.BaseURL(r), http.MaxBytesReader(w, io.NopCloser(rd), 2<<20), r.URL.Query().Get("dry_run") == "1")
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "dry_run": r.URL.Query().Get("dry_run") == "1"})
	case !billing && seg[0] == "settings" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, h.Svc.GetSessionSettings(ctx, o.ID))
	case !billing && seg[0] == "settings" && r.Method == http.MethodPut:
		var v SessionSettings
		if !in(&v) {
			fail(w, ErrBad)
			return
		}
		if err := h.Svc.PutSessionSettings(ctx, o.ID, u.ID, v); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	case !billing && seg[0] == "history" && r.Method == http.MethodGet:
		qv := r.URL.Query()
		if _, err := uuid.Parse(qv.Get("user_id")); qv.Get("user_id") != "" && err != nil {
			fail(w, ErrBad)
			return
		}
		before, _ := strconv.ParseInt(qv.Get("cursor"), 10, 64)
		if qv.Get("format") == "csv" {
			cw := pagekit.NewCSV(w, "sign-in-history.csv", []string{"time", "email", "ip", "method", "success", "reason", "suspicious"})
			for n := 0; n < 2000; n++ { // up to 1,000,000 rows, 500 at a time
				rows, err := h.Svc.LoginsPage(ctx, o.ID, qv.Get("user_id"), qv.Get("filter"), qv.Get("email"), before, 500)
				if err != nil || len(rows) == 0 {
					break
				}
				for _, l := range rows {
					cw.Row(l.CreatedAt.UTC().Format(time.RFC3339), l.Email, l.IP, l.Method, strconv.FormatBool(l.Success), l.Reason, strconv.FormatBool(l.Suspicious))
				}
				before = rows[len(rows)-1].ID
			}
			cw.Close()
			return
		}
		limit := 100
		if n, err := strconv.Atoi(qv.Get("limit")); err == nil && n > 0 && n <= 200 {
			limit = n
		}
		l, err := h.Svc.LoginsPage(ctx, o.ID, qv.Get("user_id"), qv.Get("filter"), qv.Get("email"), before, limit+1)
		if err != nil {
			fail(w, err)
			return
		}
		next := ""
		if len(l) > limit {
			l = l[:limit]
			next = strconv.FormatInt(l[limit-1].ID, 10)
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": l, "items": l, "next_cursor": next})
	case !billing && seg[0] == "users" && len(seg) == 3 && seg[2] == "sign-out" && r.Method == http.MethodPost:
		n, err := h.Svc.RevokeUserSessions(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "signed_out": n})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}
