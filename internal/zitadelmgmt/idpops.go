package zitadelmgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Factor is one enrolled second factor or passkey.
type Factor struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // totp | webauthn | passkey | otp_sms | otp_email
	Name  string `json:"name"`
	State string `json:"state"`
}

// ListFactors returns every second factor and passkey the user has enrolled.
func (c *Client) ListFactors(ctx context.Context, id string) ([]Factor, error) {
	var res struct {
		Result []struct {
			State string         `json:"state"`
			OTP   map[string]any `json:"otp"`
			U2F   *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"u2f"`
			OTPSMS   map[string]any `json:"otpSms"`
			OTPEmail map[string]any `json:"otpEmail"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/management/v1/users/"+url.PathEscape(id)+"/auth_factors/_search", map[string]any{}, &res); err != nil {
		return nil, err
	}
	out := []Factor{}
	for _, r := range res.Result {
		st := strings.TrimPrefix(r.State, "AUTH_FACTOR_STATE_")
		switch {
		case r.OTP != nil:
			out = append(out, Factor{ID: "totp", Kind: "totp", Name: "Authenticator app", State: st})
		case r.U2F != nil:
			out = append(out, Factor{ID: r.U2F.ID, Kind: "webauthn", Name: firstNonEmpty(r.U2F.Name, "Security key"), State: st})
		case r.OTPSMS != nil:
			out = append(out, Factor{ID: "otp_sms", Kind: "otp_sms", Name: "Text message code", State: st})
		case r.OTPEmail != nil:
			out = append(out, Factor{ID: "otp_email", Kind: "otp_email", Name: "Email code", State: st})
		}
	}
	var pl struct {
		Result []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/management/v1/users/"+url.PathEscape(id)+"/passwordless/_search", map[string]any{}, &pl); err == nil {
		for _, r := range pl.Result {
			out = append(out, Factor{ID: r.ID, Kind: "passkey", Name: firstNonEmpty(r.Name, "Passkey"), State: strings.TrimPrefix(r.State, "AUTH_FACTOR_STATE_")})
		}
	}
	return out, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// RemoveFactor removes one factor by the id and kind ListFactors returned.
func (c *Client) RemoveFactor(ctx context.Context, userID string, f Factor) error {
	u := url.PathEscape(userID)
	switch f.Kind {
	case "totp":
		return c.do(ctx, http.MethodDelete, "/v2/users/"+u+"/totp", nil, nil)
	case "otp_sms":
		return c.do(ctx, http.MethodDelete, "/v2/users/"+u+"/otp_sms", nil, nil)
	case "otp_email":
		return c.do(ctx, http.MethodDelete, "/v2/users/"+u+"/otp_email", nil, nil)
	case "webauthn":
		return c.do(ctx, http.MethodDelete, "/v2/users/"+u+"/u2f/"+url.PathEscape(f.ID), nil, nil)
	case "passkey":
		return c.do(ctx, http.MethodDelete, "/management/v1/users/"+u+"/passwordless/"+url.PathEscape(f.ID), nil, nil)
	}
	return fmt.Errorf("unknown factor kind %q", f.Kind)
}

// RemoveAllFactors removes every factor and returns how many were removed.
// A factor that is already gone counts as removed, so the call is idempotent.
func (c *Client) RemoveAllFactors(ctx context.Context, userID string) (int, error) {
	fs, err := c.ListFactors(ctx, userID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range fs {
		if err := c.RemoveFactor(ctx, userID, f); err != nil {
			var ze *Error
			if asErr(err, &ze) && (ze.Status == 404 || strings.Contains(ze.Message, "doesn't exist")) {
				continue
			}
			return n, err
		}
		n++
	}
	return n, nil
}

func asErr(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}

// SendPasswordResetLink asks Zitadel to e-mail the user a reset link.
func (c *Client) SendPasswordResetLink(ctx context.Context, id, urlTemplate string) error {
	body := map[string]any{"sendLink": map[string]any{"notificationType": "NOTIFICATION_TYPE_Email"}}
	if urlTemplate != "" {
		body["sendLink"].(map[string]any)["urlTemplate"] = urlTemplate
	}
	return c.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(id)+"/password_reset", body, nil)
}

// AuthMethods lists the authentication method types of a user (PASSWORD, TOTP, ...).
func (c *Client) AuthMethods(ctx context.Context, id string) ([]string, error) {
	var res struct {
		Types []string `json:"authMethodTypes"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/users/"+url.PathEscape(id)+"/authentication_methods", nil, &res); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(res.Types))
	for _, t := range res.Types {
		out = append(out, strings.TrimPrefix(t, "AUTHENTICATION_METHOD_TYPE_"))
	}
	return out, nil
}

// VerifyCredentials checks a password (and an optional one-time code) without
// creating a login session for the browser. It returns nil when they are right.
func (c *Client) VerifyCredentials(ctx context.Context, loginName, password, totp string) error {
	checks := map[string]any{
		"user":     map[string]any{"loginName": loginName},
		"password": map[string]any{"password": password},
	}
	if totp != "" {
		checks["totp"] = map[string]any{"code": totp}
	}
	var out struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/sessions", map[string]any{"checks": checks, "lifetime": "30s"}, &out); err != nil {
		return err
	}
	if out.SessionID != "" {
		_ = c.do(ctx, http.MethodDelete, "/v2/sessions/"+url.PathEscape(out.SessionID), map[string]any{}, nil)
	}
	return nil
}

// SetContact changes a user's e-mail (and marks it unverified so a code is sent) or phone.
func (c *Client) SetEmail(ctx context.Context, id, email string, verified bool) error {
	body := map[string]any{"email": email}
	if verified {
		body["isVerified"] = true
	} else {
		body["sendCode"] = map[string]any{}
	}
	return c.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(id)+"/email", body, nil)
}

// SetPhone changes a user's phone number and sends a verification code.
func (c *Client) SetPhone(ctx context.Context, id, phone string) error {
	return c.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(id)+"/phone", map[string]any{"phone": phone, "sendCode": map[string]any{}}, nil)
}

// IdPLink is one external identity provider link of a user.
type IdPLink struct {
	IdPID    string `json:"idp_id"`
	IdPName  string `json:"idp_name"`
	UserName string `json:"user_name"`
}

// ListIdPLinks lists the external identity providers linked to a user.
func (c *Client) ListIdPLinks(ctx context.Context, id string) ([]IdPLink, error) {
	var res struct {
		Result []struct {
			IdpID        string `json:"idpId"`
			IdpName      string `json:"idpName"`
			ProvidedUser string `json:"providedUserName"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/management/v1/users/"+url.PathEscape(id)+"/idps/_search", map[string]any{}, &res); err != nil {
		return nil, err
	}
	out := []IdPLink{}
	for _, r := range res.Result {
		out = append(out, IdPLink{IdPID: r.IdpID, IdPName: r.IdpName, UserName: r.ProvidedUser})
	}
	return out, nil
}

// RemoveIdPLink unlinks an external identity provider from a user.
func (c *Client) RemoveIdPLink(ctx context.Context, id, idpID, linkedUserID string) error {
	return c.do(ctx, http.MethodDelete, "/management/v1/users/"+url.PathEscape(id)+"/idps/"+url.PathEscape(idpID)+"/"+url.PathEscape(linkedUserID), nil, nil)
}

// TerminateSessions ends the user's sessions at the identity provider.
func (c *Client) TerminateSessions(ctx context.Context, id string) (int, error) {
	var res struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	body := map[string]any{"queries": []any{map[string]any{"userIdQuery": map[string]any{"id": id}}}}
	if err := c.do(ctx, http.MethodPost, "/v2/sessions/search", body, &res); err != nil {
		return 0, err
	}
	n := 0
	for _, s := range res.Sessions {
		if c.do(ctx, http.MethodDelete, "/v2/sessions/"+url.PathEscape(s.ID), map[string]any{}, nil) == nil {
			n++
		}
	}
	return n, nil
}

// IsLocked reports whether the user is in the locked state at the IdP.
func (c *Client) UserState(ctx context.Context, id string) (string, error) {
	var res struct {
		User struct {
			State string `json:"state"`
		} `json:"user"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/users/"+url.PathEscape(id), nil, &res); err != nil {
		return "", err
	}
	return strings.TrimPrefix(res.User.State, "USER_STATE_"), nil
}

// StartTOTP begins enrolling an authenticator app and returns the secret and otpauth URI.
func (c *Client) StartTOTP(ctx context.Context, id string) (secret, uri string, err error) {
	var res struct {
		URI    string `json:"uri"`
		Secret string `json:"secret"`
	}
	if err = c.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(id)+"/totp", map[string]any{}, &res); err != nil {
		return "", "", err
	}
	return res.Secret, res.URI, nil
}

// VerifyTOTP finishes enrolling an authenticator app with a code from it.
func (c *Client) VerifyTOTP(ctx context.Context, id, code string) error {
	return c.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(id)+"/totp/verify", map[string]any{"code": code}, nil)
}

// ChangePassword sets a new password after checking the current one.
func (c *Client) ChangePassword(ctx context.Context, id, current, next string) error {
	return c.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(id)+"/password",
		map[string]any{"currentPassword": current, "newPassword": map[string]any{"password": next, "changeRequired": false}}, nil)
}

// SetForceMFA turns the organisation's "second step required" login policy on or off.
// orgID is the identity provider's organisation (resource owner) of the user.
func (c *Client) SetForceMFA(ctx context.Context, orgID string, force bool) error {
	if c == nil {
		return ErrNotConfigured
	}
	// Read the current org login policy, then write it back with the flag changed.
	get := func(method, path string, body any, out any) error {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.URL+path, rd)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("x-zitadel-orgid", orgID)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode/100 != 2 {
			return &Error{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
		}
		if out != nil && len(raw) > 0 {
			return json.Unmarshal(raw, out)
		}
		return nil
	}
	var cur struct {
		Policy map[string]any `json:"policy"`
	}
	if err := get(http.MethodGet, "/management/v1/policies/login", nil, &cur); err != nil {
		return err
	}
	p := cur.Policy
	if p == nil {
		p = map[string]any{}
	}
	body := map[string]any{}
	for _, k := range []string{"allowUsernamePassword", "allowRegister", "allowExternalIdp", "forceMfaLocalOnly", "passwordlessType",
		"hidePasswordReset", "ignoreUnknownUsernames", "defaultRedirectUri", "passwordCheckLifetime", "externalLoginCheckLifetime",
		"mfaInitSkipLifetime", "secondFactorCheckLifetime", "multiFactorCheckLifetime"} {
		if v, ok := p[k]; ok {
			body[k] = v
		}
	}
	body["forceMfa"] = force
	isDefault, _ := p["isDefault"].(bool)
	method := http.MethodPut
	if isDefault {
		method = http.MethodPost
	}
	return get(method, "/management/v1/policies/login", body, nil)
}

// ResourceOwner returns the identity provider organisation that owns the user.
func (c *Client) ResourceOwner(ctx context.Context, id string) (string, error) {
	var res struct {
		Details struct {
			ResourceOwner string `json:"resourceOwner"`
		} `json:"details"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/users/"+url.PathEscape(id), nil, &res); err != nil {
		return "", err
	}
	return res.Details.ResourceOwner, nil
}

// AddOTPEmail enrols the e-mail one-time-code factor for a user (used by tests and recovery flows).
func (c *Client) AddOTPEmail(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(id)+"/otp_email", map[string]any{}, nil)
}
