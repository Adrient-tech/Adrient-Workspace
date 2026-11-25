// Package knowkit holds the small HTTP and error helpers shared by the
// knowledge modules (records, wiki, contracts, signatures, legal, captable).
// Each module keeps its own repository; this package only removes the
// repeated plumbing (auth lookup, JSON in/out, error to status mapping).
package knowkit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrNotFound means no record matches inside the org (or the caller may not know it exists).
	ErrNotFound = errors.New("not found")
	// ErrInvalid wraps validation failures.
	ErrInvalid = errors.New("invalid")
	// ErrForbidden means the record exists but the caller lacks access.
	ErrForbidden = errors.New("forbidden")
	// ErrConflict means the request clashes with the current state.
	ErrConflict = errors.New("conflict")
	// ErrUnavailable means a connected app is not configured.
	ErrUnavailable = errors.New("not available")
)

// Invalid builds a validation error.
func Invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Auth supplies the caller identity from the request context.
type Auth struct {
	UserID  func(r *http.Request) (string, bool)
	OrgID   func(r *http.Request) (string, bool)
	IsAdmin func(r *http.Request) bool
}

// Actor is the authenticated caller.
type Actor struct {
	Org   string
	User  string
	Admin bool
}

// Req is one parsed request below a module's mount point.
type Req struct {
	W      http.ResponseWriter
	R      *http.Request
	A      Actor
	Seg    []string
	Method string
	Base   string
}

// N is the number of path segments.
func (q Req) N() int { return len(q.Seg) }

// Is reports whether the path has n segments and the method matches.
func (q Req) Is(n int, method string) bool { return len(q.Seg) == n && q.Method == method }

// Sub reports whether the path is <x>/<id>/<name> with the method.
func (q Req) Sub(name, method string) bool {
	return len(q.Seg) == 3 && q.Seg[2] == name && q.Method == method
}

// ID is the second path segment.
func (q Req) ID() string {
	if len(q.Seg) >= 2 {
		return q.Seg[1]
	}
	return ""
}

// Handler serves one module under /api/v1/<name>/.
type Handler struct {
	Mount string
	Auth  Auth
	Route func(q Req) bool
}

// Match reports whether path belongs to the module.
func (h *Handler) Match(path string) bool {
	return path == h.Mount || strings.HasPrefix(path, h.Mount+"/")
}

// ServeHTTP dispatches to the module's route function.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok := h.Auth.OrgID(r)
	user, ok2 := h.Auth.UserID(r)
	if !ok || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	admin := h.Auth.IsAdmin != nil && h.Auth.IsAdmin(r)
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, h.Mount), "/")
	seg := []string{}
	if rest != "" {
		seg = strings.Split(rest, "/")
	}
	proto := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	q := Req{W: w, R: r, A: Actor{Org: org, User: user, Admin: admin}, Seg: seg, Method: r.Method, Base: proto + "://" + r.Host}
	if len(seg) == 0 || !h.Route(q) {
		http.NotFound(w, r)
	}
}

// JSON writes v as JSON.
func JSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// Decode reads a JSON body (max 8 MiB). It writes the error response itself.
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		Fail(w, Invalid("bad JSON body: %v", err))
		return false
	}
	return true
}

// Fail writes err with the matching status.
func Fail(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, ErrInvalid):
		code = http.StatusBadRequest
	case errors.Is(err, ErrForbidden):
		code = http.StatusForbidden
	case errors.Is(err, ErrConflict):
		code = http.StatusConflict
	case errors.Is(err, ErrUnavailable):
		code = http.StatusNotImplemented
	}
	msg := err.Error()
	if code == http.StatusInternalServerError {
		slog.Error("know module", "err", err)
		msg = "internal error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Reply writes v or the error.
func Reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		Fail(w, err)
		return
	}
	JSON(w, v)
}

// OK writes {"ok":true} or the error.
func OK(w http.ResponseWriter, err error) {
	Reply(w, map[string]bool{"ok": true}, err)
}

// RawJSON writes pre-encoded JSON.
func RawJSON(w http.ResponseWriter, raw []byte, err error) {
	if err != nil {
		Fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

// Clean trims a string.
func Clean(s string) string { return strings.TrimSpace(s) }

// OneOf reports whether v is one of allowed.
func OneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}

// Clock returns now from f or the wall clock.
func Clock(f func() time.Time) time.Time {
	if f != nil {
		return f()
	}
	return time.Now()
}

// Audit is the optional audit callback every module carries.
type Audit func(ctx context.Context, action, rtype, rid string)

// IsUUID reports whether s looks like a canonical UUID. Ids from URLs and
// bodies are checked with it before they reach a uuid column.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
		default:
			return false
		}
	}
	return true
}

// NewKey returns a random 32-hex-character token (storage keys, signing links).
func NewKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// DecodeOptional reads a JSON body when there is one; an empty body is not an error.
func DecodeOptional(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	return json.NewDecoder(r.Body).Decode(v)
}

// Public is an unauthenticated handler a module serves (signing links, public pages).
type Public interface {
	Match(path string) bool
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}
