package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

// stateCookieName is the name of the short-lived cookie that binds the OIDC
// state value across the redirect.
const stateCookieName = "adrient_oidc_state"

// browserIDCookieName is the stable, long-lived cookie that identifies a browser
// for multi-account tracking. Kept here so the Callback can set it in the same
// response as the session cookie, and so the HTTP middleware can read it. The
// actual account-list logic lives in internal/multiaccounts.
const browserIDCookieName = "adrient_bid"

// FirstAdminBootstrapper grants the org-admin role to a user when their org has
// no admins yet (the auto-bootstrap of the first member). server.go injects an
// implementation backed by internal/orgadmin so this package stays free of that
// dependency in tests; nil disables bootstrap.
type FirstAdminBootstrapper interface {
	EnsureFirstAdmin(ctx context.Context, orgID, userID string) (bool, error)
}

// BrowserAccountAdder registers a newly-created session token under the browser
// identified by the browser_id cookie, enabling multi-account switching. server.go
// injects an implementation backed by internal/multiaccounts; nil disables it.
type BrowserAccountAdder interface {
	AddAccount(ctx context.Context, browserID, sessionToken string) error
}

// SignInRoute is the identity policy's decision for a first-ever sign-in: which
// org the new user joins and with what type/expiry (invitation or domain
// auto-join). A nil route means "no special routing" (legacy behavior).
type SignInRoute struct {
	OrgID        string
	UserType     string // member | external | guest
	SponsorID    string
	InvitationID string
	HomeDomainID string
	ExpiresAt    *time.Time
	GuestApps    []string
	Via          string // invitation | domain_auto_join
}

// SignInPolicy is the identity layer hooked into the OIDC callback (verified
// domains, invitations, blocked/expired accounts, allowed sign-in domains).
// internal/lifecycle implements it; nil keeps the legacy behavior.
type SignInPolicy interface {
	// Route decides the org of a first-ever sign-in (nil = no special routing).
	Route(ctx context.Context, email string, emailVerified bool) (*SignInRoute, error)
	// SignupOpen reports whether orgID accepts unrouted first-ever sign-ins
	// (signup_mode other than invite_only).
	SignupOpen(ctx context.Context, orgID string) bool
	// Gate refuses blocked/expired users and e-mail domains outside the org's
	// allowed sign-in list. existing is nil for a first-ever sign-in.
	Gate(ctx context.Context, orgID, email, ip string, existing *users.User, route *SignInRoute) error
	// Finish records a successful sign-in: applies the route to the new user,
	// assigns the UPN, accepts the invitation and stamps last_sign_in_at.
	Finish(ctx context.Context, u users.User, route *SignInRoute, ip string) error
}

// Service implements adrientv1.AuthServiceServer.
type Service struct {
	adrientv1.UnimplementedAuthServiceServer

	cfg            Config
	oidc           *OIDC
	sessions       *SessionStore
	users          *users.Repository
	orgs           *orgs.Repository
	firstAdmin     FirstAdminBootstrapper
	browserAccount BrowserAccountAdder
	policy         SignInPolicy
}

// WithSignInPolicy injects the identity sign-in policy and returns the Service
// for chaining. nil disables it.
func (s *Service) WithSignInPolicy(p SignInPolicy) *Service {
	s.policy = p
	return s
}

// NewService constructs an AuthService.
func NewService(cfg Config, oidcClient *OIDC, sessions *SessionStore, ur *users.Repository, or *orgs.Repository) *Service {
	return &Service{
		cfg:      cfg,
		oidc:     oidcClient,
		sessions: sessions,
		users:    ur,
		orgs:     or,
	}
}

// WithFirstAdminBootstrapper injects the first-admin bootstrapper and returns
// the Service for chaining. When set, the OIDC callback auto-grants org-admin to
// a user who is the first member of an org with no admins yet.
func (s *Service) WithFirstAdminBootstrapper(b FirstAdminBootstrapper) *Service {
	s.firstAdmin = b
	return s
}

// WithBrowserAccountAdder injects the multi-account tracker and returns the
// Service for chaining. When set, each OIDC callback appends the new session to
// the browser's account list so the user can switch between them without a new
// OIDC redirect.
func (s *Service) WithBrowserAccountAdder(b BrowserAccountAdder) *Service {
	s.browserAccount = b
	return s
}

// Login generates a CSRF state, sets it as a short-lived cookie via the
// gRPC-gateway response header, and returns the IdP authorization URL.
// The HTTP gateway maps the URL to a 302 response (see internal/server's
// redirectOnAuthURL forward-response option).
func (s *Service) Login(ctx context.Context, req *adrientv1.LoginRequest) (*adrientv1.LoginResponse, error) {
	state, err := NewState()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate state: %v", err)
	}
	cookie := (&http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/api/v1/auth",
		Domain:   s.cfg.CookieDomain,
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}).String()
	if err := setHTTPHeader(ctx, "set-cookie", cookie); err != nil {
		return nil, err
	}
	// Whitelist the prompt values we forward to the IdP.
	prompt := req.GetPrompt()
	if prompt != "select_account" && prompt != "login" {
		prompt = ""
	}
	authURL := s.oidc.AuthCodeURL(state, prompt)
	// return_to is held client-side for V1; default "/" if absent.
	_ = req.GetReturnTo()
	return &adrientv1.LoginResponse{AuthorizationUrl: authURL}, nil
}

// StartSSO redirects the browser to one Zitadel identity provider (domain based
// single sign-on). It sets the same state cookie as Login so Callback accepts it.
func (s *Service) StartSSO(w http.ResponseWriter, r *http.Request, idpID, loginHint string) error {
	state, err := NewState()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: state, Path: "/api/v1/auth", Domain: s.cfg.CookieDomain,
		MaxAge: 600, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, s.oidc.AuthCodeURLForIdP(state, idpID, loginHint), http.StatusFound)
	return nil
}

// Callback handles the IdP redirect: validate state cookie, exchange the code
// for tokens, upsert the user, create a session, set the session cookie, then
// instruct the gateway to redirect to "/".
func (s *Service) Callback(ctx context.Context, req *adrientv1.CallbackRequest) (*adrientv1.CallbackResponse, error) {
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing code")
	}
	if req.GetState() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing state")
	}
	stateCookie, err := readHTTPCookie(ctx, stateCookieName)
	if err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "missing state cookie: %v", err)
	}
	if stateCookie != req.GetState() {
		return nil, status.Error(codes.PermissionDenied, "state mismatch")
	}
	claims, err := s.oidc.Exchange(ctx, req.GetCode())
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "exchange: %v", err)
	}
	// Decide which org this sign-in belongs to. A returning user keeps whatever
	// org they were first provisioned into (the shared default org for existing
	// members, or their personal org). Only a first-EVER sign-in (no adrient user
	// row for this issuer+subject in any org) may be routed to a fresh personal
	// org — and only when personal orgs are enabled. This guarantees we never
	// move an existing default-org member into a personal org.
	ip, userAgent := requestContext(ctx)
	orgID, isNewPersonal, route, existing, err := s.resolveSignInOrg(ctx, claims.Issuer, claims.Subject, claims.DisplayName(), claims.Email, claims.EmailVerified)
	if err != nil {
		return nil, err
	}
	if gerr := checkSignInGate(ctx, ip, ""); gerr != nil {
		observeLogin(ctx, LoginEvent{OrgID: orgID, Email: claims.Email, IP: ip, UserAgent: userAgent, Method: "sso", Reason: gerr.Error()})
		return nil, status.Errorf(codes.PermissionDenied, "sign-in denied: this address is not allowed")
	}
	if s.policy != nil {
		if gerr := s.policy.Gate(ctx, orgID, claims.Email, ip, existing, route); gerr != nil {
			observeLogin(ctx, LoginEvent{OrgID: orgID, Email: claims.Email, IP: ip, UserAgent: userAgent, Method: "sso", Reason: gerr.Error()})
			return nil, status.Errorf(codes.PermissionDenied, "sign-in denied: %v", gerr)
		}
	}
	u, err := s.users.UpsertByOIDC(ctx, users.UpsertInput{
		OrgID:       orgID,
		OIDCIssuer:  claims.Issuer,
		OIDCSubject: claims.Subject,
		Email:       claims.Email,
		DisplayName: claims.DisplayName(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "upsert user: %v", err)
	}
	if s.policy != nil {
		// Best-effort: a failure to stamp lifecycle fields must not lock the user out.
		_ = s.policy.Finish(ctx, u, route, ip)
	}
	_ = isNewPersonal // the firstAdmin bootstrap below covers personal-org admin setup
	// Auto-bootstrap: if this user is the first member of an org that has no
	// admins yet, make them the org's first admin. Best-effort — a failure here
	// must not block sign-in (a super-admin in ADRIENT_ADMIN_EMAILS can still grant
	// admins manually). See docs/rbac-design.md.
	if s.firstAdmin != nil {
		_, _ = s.firstAdmin.EnsureFirstAdmin(ctx, u.OrgID, u.ID)
	}
	// Capture the client IP + user agent at sign-in for the admin Sessions view.
	// These arrive as gRPC metadata the gateway forwards from the HTTP request:
	// the User-Agent as "grpcgateway-user-agent" and the original client IP as
	// "x-forwarded-for" (first hop). See requestContext below.
	tok, err := s.sessions.CreateWithContext(ctx, u.ID, s.cfg.SessionLifetime, ip, userAgent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create session: %v", err)
	}
	observeLogin(ctx, LoginEvent{OrgID: u.OrgID, UserID: u.ID, Email: u.Email, IP: ip, UserAgent: userAgent, Method: "sso", Success: true})
	sessCookie := (&http.Cookie{
		Name:     s.cfg.CookieName,
		Value:    tok,
		Path:     "/",
		Domain:   s.cfg.CookieDomain,
		MaxAge:   int(s.cfg.SessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}).String()
	clearState := (&http.Cookie{
		Name: stateCookieName, Value: "", Path: "/api/v1/auth", Domain: s.cfg.CookieDomain, MaxAge: -1,
	}).String()
	if err := setHTTPHeader(ctx, "set-cookie", sessCookie); err != nil {
		return nil, err
	}
	if err := setHTTPHeader(ctx, "set-cookie", clearState); err != nil {
		return nil, err
	}
	// Multi-account: resolve or mint the browser_id cookie, then register this
	// new session under that browser. Best-effort — failure must not block sign-in
	// (multi-account is a UX feature, not a security gate).
	if s.browserAccount != nil {
		bid, _ := readHTTPCookie(ctx, browserIDCookieName)
		if bid == "" {
			bid = newBrowserID()
			bidCookie := (&http.Cookie{
				Name:     browserIDCookieName,
				Value:    bid,
				Path:     "/",
				Domain:   s.cfg.CookieDomain,
				MaxAge:   365 * 24 * 60 * 60, // 1 year
				HttpOnly: true,
				Secure:   s.cfg.CookieSecure,
				SameSite: http.SameSiteLaxMode,
			}).String()
			_ = setHTTPHeader(ctx, "set-cookie", bidCookie)
		}
		_ = s.browserAccount.AddAccount(ctx, bid, tok)
	}
	return &adrientv1.CallbackResponse{RedirectTo: "/"}, nil
}

// newBrowserID generates a random 16-byte hex browser identifier.
func newBrowserID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// resolveSignInOrg returns the org id this sign-in should belong to, and whether
// a new personal org was created for it.
//
//   - If a adrient user already exists for (issuer, subject) in any org, reuse that
//     org — returning users never change orgs.
//   - Else, if personal orgs are enabled, create a personal org and route the new
//     user there.
//   - Else, fall back to the shared default org (legacy single-org behavior).
func (s *Service) resolveSignInOrg(ctx context.Context, issuer, subject, displayName, email string, emailVerified bool) (orgID string, newPersonal bool, route *SignInRoute, existing *users.User, err error) {
	if ex, gerr := s.users.GetByOIDCAnyOrg(ctx, issuer, subject); gerr == nil {
		return ex.OrgID, false, nil, &ex, nil
	} else if !errors.Is(gerr, users.ErrNotFound) {
		return "", false, nil, nil, status.Errorf(codes.Internal, "lookup user: %v", gerr)
	}

	// An account created ahead of time (SCIM) is adopted by the first verified sign-in.
	if emailVerified {
		if ex, ok := s.users.ClaimPreprovisioned(ctx, issuer, subject, email); ok {
			return ex.OrgID, false, nil, &ex, nil
		}
	}

	// Identity routing: a pending invitation for this e-mail, or a verified
	// domain with auto-join, decides the org before any personal-org fallback.
	if s.policy != nil {
		r, rerr := s.policy.Route(ctx, email, emailVerified)
		if rerr != nil {
			return "", false, nil, nil, status.Errorf(codes.Internal, "route sign-in: %v", rerr)
		}
		if r != nil {
			return r.OrgID, false, r, nil, nil
		}
	}

	if s.cfg.PersonalOrgs {
		name := displayName
		if name == "" {
			name = email
		}
		if name == "" {
			name = "Personal workspace"
		}
		po, cerr := s.orgs.CreatePersonal(ctx, name)
		if cerr != nil {
			return "", false, nil, nil, status.Errorf(codes.Internal, "create personal org: %v", cerr)
		}
		return po.ID, true, nil, nil, nil
	}

	def, derr := s.orgs.GetBySlug(ctx, s.cfg.DefaultOrgSlug)
	if derr != nil {
		return "", false, nil, nil, status.Errorf(codes.Internal, "lookup default org: %v", derr)
	}
	if s.policy != nil && !s.policy.SignupOpen(ctx, def.ID) {
		return "", false, nil, nil, status.Error(codes.PermissionDenied, "sign-up is invite-only; ask an admin for an invitation")
	}
	return def.ID, false, nil, nil, nil
}

// Whoami returns the authenticated user and their org. The auth middleware
// (see internal/auth/middleware.go) must have validated the session and
// attached both via WithUser/WithOrg.
func (s *Service) Whoami(ctx context.Context, _ *adrientv1.WhoamiRequest) (*adrientv1.WhoamiResponse, error) {
	u, ok := UserFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no session")
	}
	o, ok := OrgFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Internal, "missing org context")
	}
	return &adrientv1.WhoamiResponse{
		User: &adrientv1.User{
			Id:          u.ID,
			OrgId:       u.OrgID,
			OidcIssuer:  u.OIDCIssuer,
			OidcSubject: u.OIDCSubject,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			CreatedAt:   u.CreatedAt.Unix(),
		},
		Org: &adrientv1.Org{
			Id:          o.ID,
			Slug:        o.Slug,
			DisplayName: o.DisplayName,
			IsPersonal:  o.IsPersonal,
		},
	}, nil
}

// Logout revokes the current session and clears the cookie.
func (s *Service) Logout(ctx context.Context, _ *adrientv1.LogoutRequest) (*adrientv1.LogoutResponse, error) {
	tok, err := readHTTPCookie(ctx, s.cfg.CookieName)
	if err == nil && tok != "" {
		if err := s.sessions.Revoke(ctx, tok); err != nil {
			return nil, status.Errorf(codes.Internal, "revoke: %v", err)
		}
	}
	clear := (&http.Cookie{Name: s.cfg.CookieName, Value: "", Path: "/", Domain: s.cfg.CookieDomain, MaxAge: -1}).String()
	if err := setHTTPHeader(ctx, "set-cookie", clear); err != nil {
		return nil, err
	}
	return &adrientv1.LogoutResponse{}, nil
}

// setHTTPHeader appends a header value to the gRPC-gateway's response.
// Keys must be lowercase; the server's OutgoingHeaderMatcher maps "set-cookie"
// to the HTTP "Set-Cookie" header.
func setHTTPHeader(ctx context.Context, name, value string) error {
	md := metadata.Pairs(name, value)
	return grpc.SendHeader(ctx, md)
}

// readHTTPCookie returns the value of the cookie named `name`, sourced from
// gRPC-gateway's incoming metadata key `grpcgateway-cookie`.
func readHTTPCookie(ctx context.Context, name string) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata")
	}
	for _, c := range md.Get("grpcgateway-cookie") {
		header := http.Header{"Cookie": []string{c}}
		req := http.Request{Header: header}
		ck, err := req.Cookie(name)
		if err != nil {
			continue
		}
		v, _ := url.QueryUnescape(ck.Value)
		return v, nil
	}
	return "", fmt.Errorf("cookie %q not present", name)
}

// requestContext extracts the best-guess client IP and User-Agent from the
// gRPC-gateway incoming metadata for the current request. The gateway forwards
// the HTTP User-Agent header as the metadata key "grpcgateway-user-agent", and
// (when adrient sits behind a proxy/tunnel) the original client IP as the first
// hop of "x-forwarded-for". Both are best-effort and may be empty. The IP falls
// back to the gRPC peer address when no forwarded header is present.
func requestContext(ctx context.Context) (ip, userAgent string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if vs := md.Get("grpcgateway-user-agent"); len(vs) > 0 {
			userAgent = vs[0]
		}
		if vs := md.Get("x-forwarded-for"); len(vs) > 0 && vs[0] != "" {
			xff := vs[0]
			if i := strings.IndexByte(xff, ','); i >= 0 {
				ip = strings.TrimSpace(xff[:i])
			} else {
				ip = strings.TrimSpace(xff)
			}
		}
	}
	if ip == "" {
		if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
			host := p.Addr.String()
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			ip = host
		}
	}
	return ip, userAgent
}

// Context-attached values. Auth middleware sets these; service methods read them.

type ctxKey int

const (
	userCtxKey ctxKey = iota
	orgCtxKey
	sessionTokenCtxKey
	tokenAuthCtxKey // set true when the request was authenticated by an API token
	scopesCtxKey    // []string of the API token's scopes
)

// WithSessionToken attaches the caller's live session token to the context (set
// by the auth middleware). It is read only to flag the caller's OWN session in
// the Sessions view — the token itself is never returned to the client.
func WithSessionToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, sessionTokenCtxKey, token)
}

// SessionTokenFromContext returns the session token attached by WithSessionToken.
func SessionTokenFromContext(ctx context.Context) (string, bool) {
	t, ok := ctx.Value(sessionTokenCtxKey).(string)
	return t, ok && t != ""
}

// WithUser attaches a user to the context (used by the auth middleware).
func WithUser(ctx context.Context, u users.User) context.Context {
	return context.WithValue(ctx, userCtxKey, u)
}

// UserFromContext returns the user attached by WithUser, if any.
func UserFromContext(ctx context.Context) (users.User, bool) {
	u, ok := ctx.Value(userCtxKey).(users.User)
	return u, ok
}

// WithOrg attaches an org to the context (used by the tenancy middleware).
func WithOrg(ctx context.Context, o orgs.Org) context.Context {
	return context.WithValue(ctx, orgCtxKey, o)
}

// OrgFromContext returns the org attached by WithOrg, if any.
func OrgFromContext(ctx context.Context) (orgs.Org, bool) {
	o, ok := ctx.Value(orgCtxKey).(orgs.Org)
	return o, ok
}

// WithTokenAuth marks the request as authenticated by an API token (carrying its
// scopes), as opposed to an interactive session.
func WithTokenAuth(ctx context.Context, scopes []string) context.Context {
	ctx = context.WithValue(ctx, tokenAuthCtxKey, true)
	return context.WithValue(ctx, scopesCtxKey, scopes)
}

// IsTokenAuth reports whether the request was authenticated by an API token.
func IsTokenAuth(ctx context.Context) bool {
	v, _ := ctx.Value(tokenAuthCtxKey).(bool)
	return v
}

// ScopesFromContext returns the API token scopes attached by WithTokenAuth.
func ScopesFromContext(ctx context.Context) ([]string, bool) {
	s, ok := ctx.Value(scopesCtxKey).([]string)
	return s, ok
}
