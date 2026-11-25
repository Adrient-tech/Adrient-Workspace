// Package app wires the PDF signing backend's services into adrient's own
// server process. The auth bridge in this file translates adrient's session
// identity (resolved by adrient's auth middleware into the request context) into
// the context keys the PDF handlers expect, so the PDF code authenticates off
// adrient's session instead of its own standalone OIDC.
package app

import (
	"context"
	"net/http"

	"google.golang.org/grpc"

	adrientauth "github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	pdfauth "github.com/Adrient-tech/Adrient-Workspace/internal/pdf/auth"
)

// stampPDFIdentity reads adrient's session user off ctx (set by adrient's auth
// middleware via auth.WithUser) and re-stamps the PDF auth context keys that
// the PDF handlers read: UserEmailFromContext (email) and UserIDFromCtx (sub).
// When no adrient user is present the context is returned unchanged — the PDF
// handlers then see an empty caller and apply their own per-endpoint rules
// (e.g. the SigningService and guest /api/sign/* paths are intentionally
// unauthenticated for token-bearing signers).
func stampPDFIdentity(ctx context.Context) context.Context {
	u, ok := adrientauth.UserFromContext(ctx)
	if !ok {
		return ctx
	}
	if u.Email != "" {
		ctx = pdfauth.WithUserEmail(ctx, u.Email)
	}
	if u.ID != "" {
		// PDF's UserIDFromCtx reads what standalone set from claims.Sub. Adrient's
		// stable user id plays that role here.
		ctx = pdfauth.WithUserID(ctx, u.ID)
	}
	return ctx
}

// UnaryServerInterceptor bridges adrient's session identity into the PDF auth
// context for unary RPCs. Chain it AFTER adrient's own interceptors so adrient's
// auth context is already populated when this runs.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(stampPDFIdentity(ctx), req)
	}
}

// StreamServerInterceptor bridges adrient's session identity into the PDF auth
// context for streaming RPCs.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &bridgedStream{ServerStream: ss, ctx: stampPDFIdentity(ss.Context())})
	}
}

// bridgedStream overrides Context() so the PDF handler reads the stamped ctx.
type bridgedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *bridgedStream) Context() context.Context { return s.ctx }

// HTTPMiddleware bridges adrient's session identity into the PDF auth context for
// the raw-HTTP PDF handlers. Mount it OUTSIDE adrient's auth middleware so the
// adrient user is already on the request context when this wrapper runs.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(stampPDFIdentity(r.Context())))
	})
}
