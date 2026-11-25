package auth

import (
	"context"
	"sync"
	"time"
)

// LoginEvent describes one sign-in attempt for the login history.
type LoginEvent struct {
	OrgID, UserID, Email, IP, UserAgent string
	Method                              string // sso | password | demo
	Success                             bool
	Reason                              string
	Country, City, MFAResult            string // optional, filled where the request carries them
}

// SessionLimits are per-organization session rules, enforced on every request.
type SessionLimits struct {
	MaxAge        time.Duration // 0 = no limit
	Idle          time.Duration // 0 = no limit
	MaxConcurrent int           // 0 = no limit
}

var (
	hookMu       sync.RWMutex
	loginObs     func(context.Context, LoginEvent)
	passwordGate func(ctx context.Context, email string) error
	limitsFn     func(ctx context.Context, userID string) SessionLimits
	signInGate   func(ctx context.Context, ip, country string) error
	readOnlyFn   func(ctx context.Context, userID string) bool
)

// SetSignInGate registers the address and country allow/block check run before a
// password sign-in is attempted.
func SetSignInGate(f func(ctx context.Context, ip, country string) error) {
	hookMu.Lock()
	signInGate = f
	hookMu.Unlock()
}

// SetReadOnlyCheck registers the quarantine check: a read-only account may read
// but every write request is refused.
func SetReadOnlyCheck(f func(ctx context.Context, userID string) bool) {
	hookMu.Lock()
	readOnlyFn = f
	hookMu.Unlock()
}

func checkSignInGate(ctx context.Context, ip, country string) error {
	hookMu.RLock()
	f := signInGate
	hookMu.RUnlock()
	if f == nil {
		return nil
	}
	return f(ctx, ip, country)
}

func isReadOnly(ctx context.Context, userID string) bool {
	hookMu.RLock()
	f := readOnlyFn
	hookMu.RUnlock()
	return f != nil && f(ctx, userID)
}

// SetLoginObserver registers the login history recorder.
func SetLoginObserver(f func(context.Context, LoginEvent)) {
	hookMu.Lock()
	loginObs = f
	hookMu.Unlock()
}

// SetPasswordGate registers a check run before a password sign-in is attempted
// (used to force single sign-in for domains that require it).
func SetPasswordGate(f func(ctx context.Context, email string) error) {
	hookMu.Lock()
	passwordGate = f
	hookMu.Unlock()
}

// SetSessionLimits registers the per-user session limits resolver.
func SetSessionLimits(f func(ctx context.Context, userID string) SessionLimits) {
	hookMu.Lock()
	limitsFn = f
	hookMu.Unlock()
}

func observeLogin(ctx context.Context, ev LoginEvent) {
	hookMu.RLock()
	f := loginObs
	hookMu.RUnlock()
	if f != nil {
		f(context.WithoutCancel(ctx), ev)
	}
}

func checkPasswordGate(ctx context.Context, email string) error {
	hookMu.RLock()
	f := passwordGate
	hookMu.RUnlock()
	if f == nil {
		return nil
	}
	return f(ctx, email)
}

func sessionLimits(ctx context.Context, userID string) SessionLimits {
	hookMu.RLock()
	f := limitsFn
	hookMu.RUnlock()
	if f == nil {
		return SessionLimits{}
	}
	return f(ctx, userID)
}
