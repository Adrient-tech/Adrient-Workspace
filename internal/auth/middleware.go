package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/apitokens"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

// HTTPMiddleware returns a function that authenticates incoming HTTP requests
// based on the session cookie. If the cookie is absent or invalid, the request
// is passed through with no user attached — downstream services that require
// authentication should check `UserFromContext` and return Unauthenticated.
//
// `defaultOrg` is attached for anonymous requests (so they still know the
// default tenant). For an AUTHENTICATED user, the middleware resolves that
// user's actual org fresh from the DB each request (orepo) and attaches it —
// so org renames/branding reflect immediately and personal/multi-org users get
// their own org, not the startup default.
func HTTPMiddleware(cfg Config, sessions *SessionStore, urepo *users.Repository, orepo *orgs.Repository, defaultOrg orgs.Org, tokens *apitokens.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithOrg(r.Context(), defaultOrg)
			if sessions == nil || urepo == nil {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			authed := false
			cookie, err := r.Cookie(cfg.CookieName)
			if err == nil && cookie.Value != "" {
				sess, lookupErr := sessions.Lookup(r.Context(), cookie.Value)
				if lookupErr == nil {
					// A blocked / expired account is treated as unauthenticated on
					// the very next request (the session row stays but is inert).
					if u, uerr := urepo.GetByID(r.Context(), sess.UserID); uerr == nil && !u.Blocked(time.Now()) {
						ctx = WithUser(ctx, u)
						if orepo != nil {
							if o, oerr := orepo.GetByID(r.Context(), u.OrgID); oerr == nil {
								ctx = WithOrg(ctx, o)
							}
						}
						if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions &&
							!strings.HasPrefix(r.URL.Path, "/api/v1/auth/") && !strings.HasPrefix(r.URL.Path, "/api/v1/iam/my") && isReadOnly(r.Context(), u.ID) {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusForbidden)
							_, _ = w.Write([]byte(`{"error":"Your account is read-only while a security check runs. Ask your IT team."}`))
							return
						}
						ctx = WithSessionToken(ctx, cookie.Value)
						_ = sessions.TouchLastSeen(r.Context(), cookie.Value)
						authed = true
					}
				}
			}
			// API token auth: when there's no session, accept a
			// "Authorization: Bearer grw_..." personal access token. It
			// authenticates as its owning user, gated to its scopes.
			if !authed && tokens != nil {
				if bearer, ok := bearerToken(r); ok {
					if uid, _, scopes, terr := tokens.Resolve(r.Context(), bearer); terr == nil {
						if u, uerr := urepo.GetByID(r.Context(), uid); uerr == nil && !u.Blocked(time.Now()) {
							ctx = WithUser(ctx, u)
							if orepo != nil {
								if o, oerr := orepo.GetByID(r.Context(), u.OrgID); oerr == nil {
									ctx = WithOrg(ctx, o)
								}
							}
							ctx = WithTokenAuth(ctx, scopes)
							// Enforce scopes on the JSON API surface.
							if !apitokens.ScopesAllow(scopes, r.URL.Path, r.Method) {
								http.Error(w, "api token scope does not permit this request", http.StatusForbidden)
								return
							}
						}
					}
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):]), true
	}
	return "", false
}
