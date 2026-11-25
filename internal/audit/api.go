package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// ActionDenied is the action recorded when authz refuses a call (403).
const ActionDenied = "permission_denied"

// LogAPI wraps a JSON business API (invoicing, crm, accounting, tickets)
// mounted under mount (e.g. "/api/v1/crm"). Every MUTATING request
// (POST/PUT/PATCH/DELETE) is recorded after it runs, with the resource type and
// id taken from the path ({mount}/{resource}/{id}[/{sub}]) and any detail the
// handler added through Annotate. GETs pass through unrecorded unless the
// optional read predicate names an action for them (exports, reports).
//
// A nil recorder yields the handler unchanged.
func (rec *Recorder) LogAPI(service, mount string, read func(r *http.Request) (action string, ok bool), h http.Handler) http.Handler {
	if rec == nil || rec.repo == nil {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action, ok := "", false
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			action, ok = apiAction(r.Method, mount, r.URL.Path), true
		default:
			if read != nil {
				action, ok = read(r)
			}
		}
		if !ok {
			h.ServeHTTP(w, r)
			return
		}
		ctx, note := WithNote(r.Context())
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(sr, r.WithContext(ctx))

		resType, resID := apiResource(mount, r.URL.Path)
		st := "ok"
		if sr.status >= 400 {
			st = "error"
		}
		e := Event{
			Service:      service,
			Action:       action,
			ResourceType: resType,
			ResourceID:   resID,
			Method:       r.Method + " " + r.URL.Path,
			Status:       st,
			Detail:       map[string]any{"http_status": sr.status},
			IP:           clientIP(r),
			UserAgent:    r.UserAgent(),
		}
		note.apply(&e)
		rec.Record(ctx, e)
	})
}

func apiSegments(mount, path string) []string {
	p := strings.Trim(strings.TrimPrefix(path, mount), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// apiResource returns ({resource}, {id}) from {mount}/{resource}/{id}/....
func apiResource(mount, path string) (string, string) {
	seg := apiSegments(mount, path)
	switch len(seg) {
	case 0:
		return "", ""
	case 1:
		return seg[0], ""
	}
	return seg[0], seg[1]
}

// apiAction derives the verb: a trailing sub-resource names the action
// ("stage", "send", "payments"), otherwise the HTTP method maps to
// create/update/delete.
func apiAction(method, mount, path string) string {
	seg := apiSegments(mount, path)
	if len(seg) >= 3 {
		return strings.ToLower(seg[2])
	}
	switch method {
	case http.MethodPost:
		if len(seg) == 2 {
			return strings.ToLower(seg[1]) // e.g. /accounting/bank/import
		}
		return "create"
	case http.MethodDelete:
		return "delete"
	}
	return "update"
}

// ClientEventHandler serves POST /api/v1/audit/client-event for actions that
// happen entirely in the browser (CSV exports are built client-side). Only the
// listed services are accepted, only the "export" action, and the caller must
// hold the service's read permission (can). Body: {"service","kind","count"}.
func (rec *Recorder) ClientEventHandler(perms map[string]string, can func(ctx context.Context, perm string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		var b struct {
			Service string `json:"service"`
			Kind    string `json:"kind"`
			Count   int    `json:"count"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad request"})
			return
		}
		perm, ok := perms[b.Service]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown service"})
			return
		}
		if can != nil && !can(r.Context(), perm) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden", "permission": perm})
			return
		}
		kind := b.Kind
		if len(kind) > 64 {
			kind = kind[:64]
		}
		rec.Record(r.Context(), Event{
			Service:      b.Service,
			Action:       "export",
			ResourceType: kind,
			Method:       r.Method + " " + r.URL.Path,
			Status:       "ok",
			Detail:       map[string]any{"kind": kind, "rows": b.Count},
			IP:           clientIP(r),
			UserAgent:    r.UserAgent(),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
}

// RecordDenied writes a permission_denied event for a refused call.
func (rec *Recorder) RecordDenied(ctx context.Context, perm, method, target, ip, ua string) {
	svc := perm
	if i := strings.IndexByte(perm, ':'); i > 0 {
		svc = perm[:i]
	}
	rec.Record(ctx, Event{
		Service:      svc,
		Action:       ActionDenied,
		ResourceType: "permission",
		ResourceID:   perm,
		Method:       method + " " + target,
		Status:       "denied",
		Detail:       map[string]any{"permission": perm},
		IP:           ip,
		UserAgent:    ua,
	})
}
