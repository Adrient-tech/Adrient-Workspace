// Package zitadelmgmt is a small client for the parts of the Zitadel API that
// the identity admin features need: creating, deactivating and deleting users,
// and configuring external identity providers (Microsoft Entra ID, Google,
// generic OIDC, SAML) for the whole instance.
package zitadelmgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to Zitadel with a service-account token.
type Client struct {
	URL   string
	Token string
	HTTP  *http.Client
}

// New returns a client, or nil when url or token is empty (Zitadel not configured).
func New(url, token string) *Client {
	if strings.TrimSpace(url) == "" || strings.TrimSpace(token) == "" {
		return nil
	}
	return &Client{URL: strings.TrimRight(url, "/"), Token: token, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// ErrNotConfigured is returned when no Zitadel client exists.
var ErrNotConfigured = errors.New("zitadel is not configured on this server")

// Error is a non-2xx answer from Zitadel.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("zitadel %d: %s", e.Status, e.Message) }

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if c == nil {
		return ErrNotConfigured
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(raw))
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Message != "" {
			msg = e.Message
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return &Error{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// ---- users -----------------------------------------------------------------

// NewUser describes a human user to create.
type NewUser struct {
	Username, Email, GivenName, FamilyName string
	EmailVerified                          bool
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// CreateHumanUser creates a user and returns its Zitadel id.
func (c *Client) CreateHumanUser(ctx context.Context, u NewUser) (string, error) {
	username := u.Username
	if username == "" {
		username = u.Email
	}
	body := map[string]any{
		"username": username,
		"profile":  map[string]any{"givenName": dash(u.GivenName), "familyName": dash(u.FamilyName)},
		"email":    map[string]any{"email": u.Email, "isVerified": u.EmailVerified},
	}
	var out struct {
		UserID string `json:"userId"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/users/human", body, &out); err != nil {
		return "", err
	}
	return out.UserID, nil
}

// SetUserState deactivates or reactivates a user (action: deactivate|reactivate|lock|unlock).
func (c *Client) SetUserState(ctx context.Context, id, action string) error {
	switch action {
	case "deactivate", "reactivate", "lock", "unlock":
	default:
		return fmt.Errorf("unknown action %q", action)
	}
	return c.do(ctx, http.MethodPost, "/v2/users/"+id+"/"+action, map[string]any{}, nil)
}

// DeleteUser removes the user's IdP account (every organization).
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v2/users/"+id, nil, nil)
}

// SetPassword sets a user's password (changeRequired makes them change it at the next sign-in).
func (c *Client) SetPassword(ctx context.Context, id, password string, changeRequired bool) error {
	return c.do(ctx, http.MethodPost, "/v2/users/"+id+"/password",
		map[string]any{"newPassword": map[string]any{"password": password, "changeRequired": changeRequired}}, nil)
}

// ---- identity providers ----------------------------------------------------

// IdPKind selects the provider template.
type IdPKind string

const (
	KindEntra  IdPKind = "entra"
	KindGoogle IdPKind = "google"
	KindOIDC   IdPKind = "oidc"
	KindSAML   IdPKind = "saml"
)

// IdPInput is what an admin enters for a provider.
type IdPInput struct {
	Kind         IdPKind
	Name         string
	ClientID     string
	ClientSecret string
	Issuer       string // oidc
	TenantID     string // entra: tenant id, or "organizations" / "common"
	MetadataXML  string // saml (base64 not needed, plain XML)
	MetadataURL  string // saml
	Scopes       []string
	AutoCreate   bool // just-in-time user creation at Zitadel
}

func providerOptions(auto bool) map[string]any {
	return map[string]any{
		"isLinkingAllowed": true, "isCreationAllowed": auto, "isAutoCreation": auto, "isAutoUpdate": true,
		"autoLinking": "AUTO_LINKING_OPTION_EMAIL",
	}
}

// Validate checks the fields the kind needs.
func (in IdPInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	switch in.Kind {
	case KindEntra:
		if in.ClientID == "" || in.ClientSecret == "" {
			return errors.New("client id and client secret are required")
		}
	case KindGoogle:
		if in.ClientID == "" || in.ClientSecret == "" {
			return errors.New("client id and client secret are required")
		}
	case KindOIDC:
		if in.ClientID == "" || in.ClientSecret == "" || !strings.HasPrefix(in.Issuer, "http") {
			return errors.New("issuer URL, client id and client secret are required")
		}
	case KindSAML:
		if strings.TrimSpace(in.MetadataXML) == "" && strings.TrimSpace(in.MetadataURL) == "" {
			return errors.New("SAML metadata (XML or URL) is required")
		}
	default:
		return errors.New("unknown provider kind")
	}
	return nil
}

// CreateIdP creates the provider in Zitadel and activates it on the instance
// login policy. It returns the Zitadel provider id.
func (c *Client) CreateIdP(ctx context.Context, in IdPInput) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	scopes := in.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	var path string
	var body map[string]any
	switch in.Kind {
	case KindEntra:
		tenant := map[string]any{"tenantType": "AZURE_AD_TENANT_TYPE_COMMON"}
		if t := strings.TrimSpace(in.TenantID); t != "" && t != "common" {
			if t == "organizations" {
				tenant = map[string]any{"tenantType": "AZURE_AD_TENANT_TYPE_ORGANISATIONS"}
			} else if t == "consumers" {
				tenant = map[string]any{"tenantType": "AZURE_AD_TENANT_TYPE_CONSUMERS"}
			} else {
				tenant = map[string]any{"tenantId": t}
			}
		}
		path = "/admin/v1/idps/azure"
		body = map[string]any{"name": in.Name, "clientId": in.ClientID, "clientSecret": in.ClientSecret,
			"scopes": scopes, "tenant": tenant, "emailVerified": true, "providerOptions": providerOptions(in.AutoCreate)}
	case KindGoogle:
		path = "/admin/v1/idps/google"
		body = map[string]any{"name": in.Name, "clientId": in.ClientID, "clientSecret": in.ClientSecret,
			"scopes": scopes, "providerOptions": providerOptions(in.AutoCreate)}
	case KindOIDC:
		path = "/admin/v1/idps/generic_oidc"
		body = map[string]any{"name": in.Name, "issuer": in.Issuer, "clientId": in.ClientID, "clientSecret": in.ClientSecret,
			"scopes": scopes, "isIdTokenMapping": true, "providerOptions": providerOptions(in.AutoCreate)}
	case KindSAML:
		path = "/admin/v1/idps/saml"
		body = map[string]any{"name": in.Name, "binding": "SAML_BINDING_POST", "withSignedRequest": true,
			"nameIdFormat": "SAML_NAME_ID_FORMAT_EMAIL_ADDRESS", "providerOptions": providerOptions(in.AutoCreate)}
		if strings.TrimSpace(in.MetadataXML) != "" {
			body["metadataXml"] = []byte(in.MetadataXML) // encoded as base64 by encoding/json
		} else {
			body["metadataUrl"] = in.MetadataURL
		}
	}
	if err := c.EnsureExternalIdPAllowed(ctx); err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("zitadel returned no provider id")
	}
	if err := c.do(ctx, http.MethodPost, "/admin/v1/policies/login/idps",
		map[string]any{"idpId": out.ID, "ownerType": "IDP_OWNER_TYPE_SYSTEM"}, nil); err != nil {
		_ = c.do(ctx, http.MethodDelete, "/admin/v1/idps/templates/"+out.ID, nil, nil)
		return "", err
	}
	return out.ID, nil
}

// EnsureExternalIdPAllowed switches on "allow external identity providers" in
// the instance login policy. Zitadel refuses to route a sign-in to a provider
// ("External Login Provider not allowed") until it is on; nothing else in the
// policy changes.
func (c *Client) EnsureExternalIdPAllowed(ctx context.Context) error {
	var cur struct {
		Policy map[string]any `json:"policy"`
	}
	if err := c.do(ctx, http.MethodGet, "/admin/v1/policies/login", nil, &cur); err != nil {
		return err
	}
	if v, _ := cur.Policy["allowExternalIdp"].(bool); v {
		return nil
	}
	body := map[string]any{}
	for _, k := range []string{"allowUsernamePassword", "allowRegister", "forceMfa", "forceMfaLocalOnly", "passwordlessType", "hidePasswordReset",
		"ignoreUnknownUsernames", "defaultRedirectUri", "passwordCheckLifetime", "externalLoginCheckLifetime", "mfaInitSkipLifetime",
		"secondFactorCheckLifetime", "multiFactorCheckLifetime", "allowDomainDiscovery", "disableLoginWithEmail", "disableLoginWithPhone"} {
		if v, ok := cur.Policy[k]; ok {
			body[k] = v
		}
	}
	body["allowExternalIdp"] = true
	return c.do(ctx, http.MethodPut, "/admin/v1/policies/login", body, nil)
}

// DeleteIdP removes the provider from the login policy and deletes it.
func (c *Client) DeleteIdP(ctx context.Context, id string) error {
	_ = c.do(ctx, http.MethodDelete, "/admin/v1/policies/login/idps/"+id, nil, nil)
	return c.do(ctx, http.MethodDelete, "/admin/v1/idps/templates/"+id, nil, nil)
}

// IdPInfo is a provider as Zitadel lists it.
type IdPInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	Type  string `json:"type"`
}

// ListIdPs lists the instance's providers.
func (c *Client) ListIdPs(ctx context.Context) ([]IdPInfo, error) {
	var out struct {
		Result []IdPInfo `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/admin/v1/idps/templates/_search", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

// LoginPolicy holds the instance login policy values the admin UI edits.
type LoginPolicy struct {
	AllowUsernamePassword bool `json:"allowUsernamePassword"`
	ForceMFA              bool `json:"forceMfa"`
}

// GetPasswordComplexity returns the instance password complexity policy.
func (c *Client) GetPasswordComplexity(ctx context.Context) (map[string]any, error) {
	var out struct {
		Policy map[string]any `json:"policy"`
	}
	if err := c.do(ctx, http.MethodGet, "/admin/v1/policies/password/complexity", nil, &out); err != nil {
		return nil, err
	}
	return out.Policy, nil
}
