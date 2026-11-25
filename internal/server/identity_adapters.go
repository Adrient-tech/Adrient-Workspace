package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelmgmt"
)

// baseURLOf returns the public origin of the request (honoring proxy headers).
func baseURLOf(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if r.TLS != nil {
			proto = "https"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host
}

// zitadelIdP mirrors offboarding state into Zitadel.
type zitadelIdP struct{ zm *zitadelmgmt.Client }

func (z zitadelIdP) SetActive(ctx context.Context, issuer, subject string, active bool) error {
	action := "deactivate"
	if active {
		action = "reactivate"
	}
	return z.zm.SetUserState(ctx, subject, action)
}

func (z zitadelIdP) Delete(ctx context.Context, issuer, subject string) error {
	return z.zm.DeleteUser(ctx, subject)
}

// scimIdP creates provisioned people in Zitadel so they can sign in right away
// (by SSO, or with an invitation e-mail from Zitadel for a password).
type scimIdP struct {
	zm     *zitadelmgmt.Client
	issuer string
}

func (s scimIdP) CreateUser(ctx context.Context, email, displayName string) (string, string, error) {
	given, family, _ := strings.Cut(strings.TrimSpace(displayName), " ")
	id, err := s.zm.CreateHumanUser(ctx, zitadelmgmt.NewUser{Email: email, GivenName: given, FamilyName: family, EmailVerified: true})
	if err != nil {
		return "", "", err
	}
	return s.issuer, id, nil
}

func (s scimIdP) SetActive(ctx context.Context, issuer, subject string, active bool) error {
	action := "deactivate"
	if active {
		action = "reactivate"
	}
	return s.zm.SetUserState(ctx, subject, action)
}
