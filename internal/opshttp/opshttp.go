// Package opshttp holds the small JSON/HTTP helpers shared by the wave-6 ops
// modules (Shifts, Time tracking, Field service, Fleet): sentinel errors, error
// to status mapping, body decoding and the date helpers they all need.
package opshttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrInvalid   = errors.New("invalid")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
)

// Invalid, Forbidden and Conflict build errors that map to 400, 403 and 409.
func Invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}
func Forbidden(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrForbidden, fmt.Sprintf(format, a...))
}
func Conflict(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, a...))
}

// AuthFuncs supplies the caller identity from the request context.
type AuthFuncs struct {
	UserID  func(r *http.Request) (string, bool)
	OrgID   func(r *http.Request) (string, bool)
	IsAdmin func(r *http.Request) bool
}

// JSON writes v as JSON with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Fail maps an error to a status code; unknown errors are logged and hidden.
func Fail(w http.ResponseWriter, module string, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		http.Error(w, strings.TrimPrefix(err.Error(), "forbidden: "), http.StatusForbidden)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid: "), http.StatusBadRequest)
	case errors.Is(err, ErrConflict):
		http.Error(w, strings.TrimPrefix(err.Error(), "conflict: "), http.StatusConflict)
	default:
		slog.Error(module+": request failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// Reply writes {key: v} (or v itself when key is empty), or the error.
func Reply(w http.ResponseWriter, module, key string, v any, err error) {
	if err != nil {
		Fail(w, module, err)
		return
	}
	if key == "" {
		JSON(w, http.StatusOK, v)
		return
	}
	JSON(w, http.StatusOK, map[string]any{key: v})
}

// Decode reads a JSON body of at most 4 MiB.
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// ValidDate reports whether s is a real YYYY-MM-DD date.
func ValidDate(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}

// ValidTime reports whether s is HH:MM (24h).
func ValidTime(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	t, err := time.Parse("15:04", s)
	return err == nil && t.Format("15:04") == s
}

// Round2 rounds to two decimals.
func Round2(f float64) float64 {
	if f < 0 {
		return -float64(int64(-f*100+0.5)) / 100
	}
	return float64(int64(f*100+0.5)) / 100
}

// AddDays adds n days to an ISO date ("" when s is invalid).
func AddDays(s string, n int) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

// IsUUID reports whether s looks like a UUID.
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
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}
