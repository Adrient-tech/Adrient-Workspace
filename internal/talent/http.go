package talent

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// AuthFuncs supplies the caller's identity from the request context.
type AuthFuncs struct {
	UserID  func(r *http.Request) (string, bool)
	OrgID   func(r *http.Request) (string, bool)
	IsAdmin func(r *http.Request) bool
}

// HTTPHandler serves /api/v1/talent/* inside the auth wall.
type HTTPHandler struct {
	repo *Repository
	auth AuthFuncs
}

// NewHTTPHandler constructs the authenticated handler.
func NewHTTPHandler(repo *Repository, auth AuthFuncs) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth}
}

const mount = "/api/v1/talent"

// Match reports whether path belongs to the Talent API.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

// route is one parsed request.
type route struct {
	w      http.ResponseWriter
	r      *http.Request
	a      Actor
	seg    []string
	method string
	base   string
}

func (rt route) n() int { return len(rt.seg) }
func (rt route) id() string {
	if len(rt.seg) >= 2 {
		return rt.seg[1]
	}
	return ""
}
func (rt route) is(n int, method string) bool { return len(rt.seg) == n && rt.method == method }
func (rt route) sub(name, method string) bool {
	return len(rt.seg) == 3 && rt.seg[2] == name && rt.method == method
}

// BaseURL derives the public origin of the request (for links in mails).
func BaseURL(r *http.Request) string {
	proto := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	return proto + "://" + r.Host
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok := h.auth.OrgID(r)
	user, ok2 := h.auth.UserID(r)
	if !ok || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	hr := h.auth.IsAdmin != nil && h.auth.IsAdmin(r)
	if !hr {
		hr = h.repo.IsHRUser(ctx, org, user)
	}
	a := Actor{OrgID: org, UserID: user, IsHR: hr}
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/"), "/")
	rt := route{w: w, r: r, a: a, seg: seg, method: r.Method, base: BaseURL(r)}
	if h.recruitment(rt) || h.onboarding(rt) || h.performance(rt) || h.learning(rt) {
		return
	}
	switch {
	case seg[0] == "me" && rt.n() == 1:
		writeJSON(w, map[string]any{"user_id": user, "is_hr": hr})
	case seg[0] == "directory" && rt.n() == 1:
		raw, err := h.repo.Directory(ctx, org)
		replyRaw(w, "people", raw, err)
	default:
		http.NotFound(w, r)
	}
}

func (h *HTTPHandler) recruitment(rt route) bool {
	w, r, a, ctx := rt.w, rt.r, rt.a, rt.r.Context()
	id := rt.id()
	switch rt.seg[0] {
	case "settings":
		if rt.is(1, http.MethodGet) {
			s, err := h.repo.GetSettings(ctx, a.OrgID)
			reply(w, s, err)
		} else if rt.is(1, http.MethodPut) {
			var s Settings
			if decode(w, r, &s) {
				v, err := h.repo.SaveSettings(ctx, a, s)
				reply(w, v, err)
			}
		} else {
			return false
		}
	case "jobs":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListJobs(ctx, a)
			replyRaw(w, "jobs", raw, err)
		case rt.is(1, http.MethodPost), rt.is(2, http.MethodPut):
			var in JobInput
			if decode(w, r, &in) {
				raw, err := h.repo.SaveJob(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		case rt.is(2, http.MethodGet):
			raw, err := h.repo.GetJob(ctx, a, id)
			replyRawBody(w, raw, err)
		case rt.is(2, http.MethodDelete):
			replyOK(w, h.repo.DeleteJob(ctx, a, id))
		case rt.sub("request-approval", http.MethodPost):
			raw, err := h.repo.RequestApproval(ctx, a, id)
			replyRawBody(w, raw, err)
		case rt.sub("approve", http.MethodPost), rt.sub("reject", http.MethodPost):
			raw, err := h.repo.DecideApproval(ctx, a, id, rt.seg[2] == "approve")
			replyRawBody(w, raw, err)
		case rt.sub("status", http.MethodPost):
			var b struct {
				Status string `json:"status"`
			}
			if decode(w, r, &b) {
				raw, err := h.repo.SetJobStatus(ctx, a, id, b.Status)
				replyRawBody(w, raw, err)
			}
		default:
			return false
		}
	case "guides":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListGuides(ctx, a)
			replyRaw(w, "guides", raw, err)
		case rt.is(1, http.MethodPost), rt.is(2, http.MethodPut):
			var in GuideInput
			if decode(w, r, &in) {
				raw, err := h.repo.SaveGuide(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		case rt.is(2, http.MethodDelete):
			replyOK(w, h.repo.DeleteGuide(ctx, a, id))
		default:
			return false
		}
	case "applications":
		h.applications(rt)
	case "interviews":
		if rt.is(2, http.MethodDelete) {
			replyOK(w, h.repo.CancelInterview(ctx, a, id))
		} else {
			return false
		}
	case "offers":
		switch {
		case rt.sub("pdf", http.MethodGet):
			data, name, err := h.repo.OfferPDF(ctx, a, id)
			if err != nil {
				httpErr(w, err)
				return true
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
			_, _ = w.Write(data)
		case rt.sub("send", http.MethodPost):
			raw, err := h.repo.SendOffer(ctx, a, id, rt.base)
			replyRawBody(w, raw, err)
		case rt.is(2, http.MethodDelete):
			replyOK(w, h.repo.WithdrawOffer(ctx, a, id))
		default:
			return false
		}
	case "candidates":
		switch {
		case rt.is(1, http.MethodGet) && r.URL.Query().Get("email") != "":
			d, err := h.repo.CheckDuplicate(ctx, a, r.URL.Query().Get("email"))
			reply(w, map[string]any{"duplicate": d}, err)
		case rt.sub("resume", http.MethodGet):
			name, body, mime, err := h.repo.OpenResume(ctx, a, id)
			if err != nil {
				httpErr(w, err)
				return true
			}
			defer body.Close()
			w.Header().Set("Content-Type", mime)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeName(name)+`"`)
			buf := make([]byte, 32*1024)
			for {
				n, rerr := body.Read(buf)
				if n > 0 {
					_, _ = w.Write(buf[:n])
				}
				if rerr != nil {
					break
				}
			}
		case rt.sub("pool", http.MethodPost):
			var b struct {
				In   bool     `json:"in"`
				Tags []string `json:"tags"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.SetTalentPool(ctx, a, id, b.In, b.Tags))
			}
		case rt.is(2, http.MethodDelete):
			replyOK(w, h.repo.DeleteCandidate(ctx, a, id))
		default:
			return false
		}
	case "pool":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListPool(ctx, a)
			replyRaw(w, "pool", raw, err)
		case rt.sub("apply", http.MethodPost):
			var b struct {
				JobID string `json:"job_id"`
			}
			if decode(w, r, &b) {
				raw, err := h.repo.PoolToJob(ctx, a, id, b.JobID)
				replyRawBody(w, raw, err)
			}
		default:
			return false
		}
	case "reports":
		if rt.is(1, http.MethodGet) {
			rep, err := h.repo.Reports(ctx, a, r.URL.Query().Get("job_id"))
			reply(w, rep, err)
		} else {
			return false
		}
	default:
		return false
	}
	return true
}

func (h *HTTPHandler) applications(rt route) {
	w, r, a, ctx := rt.w, rt.r, rt.a, rt.r.Context()
	id := rt.id()
	switch {
	case rt.is(1, http.MethodGet):
		raw, err := h.repo.ListApplications(ctx, a, r.URL.Query().Get("job_id"))
		replyRaw(w, "applications", raw, err)
	case rt.is(1, http.MethodPost):
		var b struct {
			JobID        string `json:"job_id"`
			Name         string `json:"name"`
			Email        string `json:"email"`
			Phone        string `json:"phone"`
			Source       string `json:"source"`
			SourceDetail string `json:"source_detail"`
			CoverNote    string `json:"cover_note"`
		}
		if decode(w, r, &b) {
			raw, err := h.repo.AddCandidate(ctx, a, ApplyInput{JobID: b.JobID, Name: b.Name, Email: b.Email, Phone: b.Phone,
				Source: b.Source, SourceDetail: b.SourceDetail, CoverNote: b.CoverNote})
			replyRawBody(w, raw, err)
		}
	case rt.is(2, http.MethodGet):
		raw, err := h.repo.GetApplication(ctx, a, id)
		replyRawBody(w, raw, err)
	case rt.sub("move", http.MethodPost):
		var b struct {
			Stage string `json:"stage"`
			Note  string `json:"note"`
		}
		if decode(w, r, &b) {
			raw, err := h.repo.MoveApplication(ctx, a, id, b.Stage, b.Note)
			replyRawBody(w, raw, err)
		}
	case rt.sub("reject", http.MethodPost):
		var b struct {
			Reason string `json:"reason"`
		}
		if decode(w, r, &b) {
			raw, err := h.repo.RejectApplication(ctx, a, id, b.Reason)
			replyRawBody(w, raw, err)
		}
	case rt.sub("hire", http.MethodPost):
		raw, err := h.repo.HireApplication(ctx, a, id)
		replyRawBody(w, raw, err)
	case rt.sub("restore", http.MethodPost):
		raw, err := h.repo.RestoreApplication(ctx, a, id)
		replyRawBody(w, raw, err)
	case rt.sub("scorecard", http.MethodPost):
		var in ScorecardInput
		if decode(w, r, &in) {
			raw, err := h.repo.SaveScorecard(ctx, a, id, in)
			replyRawBody(w, raw, err)
		}
	case rt.sub("interviews", http.MethodPost):
		var in InterviewInput
		if decode(w, r, &in) {
			raw, err := h.repo.ScheduleInterview(ctx, a, id, rt.base, in)
			replyRawBody(w, raw, err)
		}
	case rt.sub("offers", http.MethodPost):
		var in OfferInput
		if decode(w, r, &in) {
			raw, err := h.repo.CreateOffer(ctx, a, id, in)
			replyRawBody(w, raw, err)
		}
	case rt.sub("new-hire", http.MethodGet):
		nh, err := h.repo.ConvertToNewHire(ctx, a, id)
		reply(w, nh, err)
	default:
		http.NotFound(w, r)
	}
}

func sanitizeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == '"' || r == '\\' || r == '/' || r > 126 {
			return '_'
		}
		return r
	}, s)
	if s == "" {
		return "file"
	}
	return s
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(v); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func httpErr(w http.ResponseWriter, err error) {
	var pe *pgconn.PgError
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, ErrDuplicate):
		http.Error(w, "this candidate has already applied to this opening", http.StatusConflict)
	case errors.Is(err, ErrUnavailable):
		http.Error(w, err.Error(), http.StatusNotImplemented)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid: "), http.StatusBadRequest)
	case errors.As(err, &pe) && (pe.Code == "22P02" || pe.Code == "23503" || pe.Code == "22007" || pe.Code == "22008" || pe.Code == "23514"):
		http.Error(w, "invalid reference or value", http.StatusBadRequest)
	default:
		slog.Error("talent", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, v)
}

func replyOK(w http.ResponseWriter, err error) { reply(w, map[string]any{"ok": true}, err) }

func replyRaw(w http.ResponseWriter, key string, raw json.RawMessage, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]json.RawMessage{key: raw})
}

func replyRawBody(w http.ResponseWriter, raw json.RawMessage, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, raw)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
