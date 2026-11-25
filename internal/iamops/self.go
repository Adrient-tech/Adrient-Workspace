package iamops

import (
	"context"
	"strings"
)

func (s *Service) self(ctx context.Context, orgID string, a Actor) (target, error) {
	if s.IdP == nil {
		return target{}, bad("the identity provider is not configured on this server")
	}
	t, err := s.loadTarget(ctx, orgID, a.ID)
	if err != nil {
		return t, err
	}
	if t.Subject == "" || strings.HasPrefix(t.Subject, "adrient:") {
		return t, bad("this account is not linked to the identity provider")
	}
	return t, nil
}

// ChangeOwnPassword changes the signed-in person's password after checking the current one,
// then signs out their other sessions.
func (s *Service) ChangeOwnPassword(ctx context.Context, orgID string, a Actor, current, next string) error {
	t, err := s.self(ctx, orgID, a)
	if err != nil {
		return err
	}
	if len(next) < 8 || len(next) > 200 {
		return bad("the new password must be at least 8 characters")
	}
	if err := s.IdP.ChangePassword(ctx, t.Subject, current, next); err != nil {
		return bad("the current password was not accepted, or the new password does not meet the password rules")
	}
	s.audit(ctx, orgID, a.ID, "self.password_change", map[string]any{})
	return nil
}

// StartOwnTOTP begins enrolling an authenticator app, after the person confirms their password.
func (s *Service) StartOwnTOTP(ctx context.Context, orgID string, a Actor, password string) (string, string, error) {
	t, err := s.self(ctx, orgID, a)
	if err != nil {
		return "", "", err
	}
	if err := s.IdP.VerifyCredentials(ctx, t.Email, password, ""); err != nil {
		return "", "", bad("the password was not accepted")
	}
	secret, uri, err := s.IdP.StartTOTP(ctx, t.Subject)
	return secret, uri, idpErr(err)
}

// VerifyOwnTOTP finishes enrolment with a code from the authenticator app.
func (s *Service) VerifyOwnTOTP(ctx context.Context, orgID string, a Actor, code string) error {
	t, err := s.self(ctx, orgID, a)
	if err != nil {
		return err
	}
	if err := s.IdP.VerifyTOTP(ctx, t.Subject, strings.TrimSpace(code)); err != nil {
		return bad("that code was not accepted")
	}
	s.audit(ctx, orgID, a.ID, "self.mfa_add", map[string]any{"kind": "totp"})
	return nil
}

// RemoveOwnFactor removes one of the person's own second steps after a fresh password check.
func (s *Service) RemoveOwnFactor(ctx context.Context, orgID string, a Actor, kind, id, password string) error {
	t, err := s.self(ctx, orgID, a)
	if err != nil {
		return err
	}
	if err := s.IdP.VerifyCredentials(ctx, t.Email, password, ""); err != nil {
		return bad("the password was not accepted")
	}
	fs, err := s.IdP.ListFactors(ctx, t.Subject)
	if err != nil {
		return idpErr(err)
	}
	for _, f := range fs {
		if f.Kind == kind && (f.ID == id || kind != "webauthn" && kind != "passkey") {
			if err := s.IdP.RemoveFactor(ctx, t.Subject, f); err != nil {
				return idpErr(err)
			}
			s.audit(ctx, orgID, a.ID, "self.mfa_remove", map[string]any{"kind": kind})
			return nil
		}
	}
	return ErrNotFound
}
