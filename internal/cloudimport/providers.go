package cloudimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/oauth2/jwt"
)

// ProviderInfo is what the wizard shows for a provider.
type ProviderInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Short       string   `json:"short"`  // used in "Imported from <Short>" folder names
	Auth        string   `json:"auth"`   // oauth | password
	Scopes      []string `json:"scopes"` // me | org
	Kinds       []Kind   `json:"kinds"`
	NeedsApp    bool     `json:"needs_app"`  // admin must enter OAuth app credentials first
	Configured  bool     `json:"configured"` // filled per org at request time
	Notes       string   `json:"notes"`
	SetupDocURL string   `json:"setup_doc"`
	Fields      []Field  `json:"fields"` // connection form fields (password-type providers)
}

// Field is one input of a connection form.
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // text | password | url | select
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
	Required    bool     `json:"required"`
	Secret      bool     `json:"secret"`
}

var imapFields = []Field{
	{Key: "imap_host", Label: "IMAP server", Type: "text", Placeholder: "imap.example.com"},
	{Key: "imap_port", Label: "IMAP port", Type: "text", Placeholder: "993"},
	{Key: "imap_security", Label: "Encryption", Type: "select", Options: []string{"tls", "starttls"}},
}

// Providers is the static registry, in wizard order.
var Providers = []ProviderInfo{
	{ID: "google", Short: "Google", Name: "Google Workspace / Gmail", Auth: "oauth", Scopes: []string{"me", "org"}, NeedsApp: true,
		Kinds:       []Kind{MigUsers, MigGroups, MigFiles, MigMail, MigCalendar, MigContacts, MigChat},
		Notes:       "Drive (My Drive and shared drives, with sharing), Gmail over IMAP with labels, Calendar, Contacts, Google Chat (your own spaces). Organisation-wide migration of every user's data needs a service account with domain-wide delegation; without it an admin can map users and groups and each person migrates their own account.",
		SetupDocURL: "docs/migrating-from-google.md"},
	{ID: "microsoft", Short: "Microsoft 365", Name: "Microsoft 365 / Outlook", Auth: "oauth", Scopes: []string{"me", "org"}, NeedsApp: true,
		Kinds:       []Kind{MigUsers, MigGroups, MigFiles, MigMail, MigCalendar, MigContacts},
		Notes:       "OneDrive and SharePoint libraries with sharing, Outlook mail with folders and categories, Calendar, Contacts via Microsoft Graph. Organisation scope uses application permissions granted by a Microsoft 365 admin. Teams chat export is a Microsoft protected API and is not available.",
		SetupDocURL: "docs/migrating-from-microsoft.md"},
	{ID: "zoho", Short: "Zoho", Name: "Zoho Workplace", Auth: "password", Scopes: []string{"me"}, NeedsApp: false,
		Kinds:       []Kind{MigMail, MigCalendar, MigContacts},
		Notes:       "Mail over IMAP, Calendar over CalDAV and Contacts over CardDAV, signed in with a Zoho app-specific password. WorkDrive files: export a ZIP from WorkDrive and upload it under 'Upload an export'. Cliq chat export is not available.",
		SetupDocURL: "docs/migrating-from-zoho.md",
		Fields: []Field{
			{Key: "username", Label: "Zoho e-mail address", Type: "text", Required: true},
			{Key: "password", Label: "App-specific password", Type: "password", Required: true, Secret: true},
			{Key: "region", Label: "Data centre", Type: "select", Options: []string{"com", "eu", "in", "com.au", "jp", "ca"}},
		}},
	{ID: "nextcloud", Short: "Nextcloud", Name: "Nextcloud", Auth: "password", Scopes: []string{"me"},
		Kinds: []Kind{MigFiles, MigMail, MigCalendar, MigContacts},
		Notes: "Files over WebDAV, Calendar over CalDAV, Contacts over CardDAV. Use an app password (Settings > Security > Devices & sessions). Mail is optional: enter your mail server's IMAP details. Nextcloud Talk history is not exported by Nextcloud.",
		Fields: append([]Field{
			{Key: "server", Label: "Nextcloud address", Type: "url", Placeholder: "https://cloud.example.com", Required: true},
			{Key: "username", Label: "Username", Type: "text", Required: true},
			{Key: "password", Label: "App password", Type: "password", Required: true, Secret: true},
		}, imapFields...)},
	{ID: "owncloud", Short: "ownCloud", Name: "ownCloud 10", Auth: "password", Scopes: []string{"me"},
		Kinds: []Kind{MigFiles, MigCalendar, MigContacts},
		Notes: "Files over WebDAV, Calendar and Contacts over CalDAV/CardDAV (needs the ownCloud Calendar/Contacts apps). ownCloud Infinite Scale (oCIS) is not covered.",
		Fields: []Field{
			{Key: "server", Label: "ownCloud address", Type: "url", Placeholder: "https://owncloud.example.com", Required: true},
			{Key: "username", Label: "Username", Type: "text", Required: true},
			{Key: "password", Label: "App password", Type: "password", Required: true, Secret: true},
		}},
	{ID: "imap", Short: "IMAP", Name: "Any mail server (IMAP)", Auth: "password", Scopes: []string{"me"},
		Kinds: []Kind{MigMail},
		Notes: "Copies every folder from any IMAP server (Dovecot, Exchange, Yahoo, Fastmail, cPanel, ...). Your source mailbox is opened read-only.",
		Fields: append([]Field{
			{Key: "username", Label: "Username / e-mail", Type: "text", Required: true},
			{Key: "password", Label: "Password or app password", Type: "password", Required: true, Secret: true},
		}, imapFields...)},
	{ID: "dav", Short: "WebDAV", Name: "Any CalDAV / CardDAV / WebDAV server", Auth: "password", Scopes: []string{"me"},
		Kinds: []Kind{MigFiles, MigCalendar, MigContacts},
		Notes: "Radicale, Baïkal, SOGo, Fastmail, iCloud (app password) and other standards-based servers. Leave a URL empty to skip it.",
		Fields: []Field{
			{Key: "caldav_url", Label: "CalDAV URL", Type: "url"},
			{Key: "carddav_url", Label: "CardDAV URL", Type: "url"},
			{Key: "webdav_url", Label: "WebDAV folder URL", Type: "url"},
			{Key: "username", Label: "Username", Type: "text", Required: true},
			{Key: "password", Label: "Password or app password", Type: "password", Required: true, Secret: true},
		}},
}

// ProviderByID looks up a provider.
func ProviderByID(id string) (ProviderInfo, bool) {
	for _, p := range Providers {
		if p.ID == id {
			return p, true
		}
	}
	return ProviderInfo{}, false
}

// OAuthEndpoints lets tests replace vendor endpoints.
type OAuthEndpoints struct {
	GoogleAuth, GoogleToken string
	MSLogin                 string // https://login.microsoftonline.com
	Google                  *GoogleEndpoints
	Graph                   string
}

// DefaultOAuthEndpoints are the real vendors.
var DefaultOAuthEndpoints = OAuthEndpoints{
	GoogleAuth: "https://accounts.google.com/o/oauth2/auth", GoogleToken: "https://oauth2.googleapis.com/token",
	MSLogin: "https://login.microsoftonline.com", Graph: "https://graph.microsoft.com",
}

var msDelegatedScopes = []string{"offline_access", "openid", "email", "User.Read", "User.ReadBasic.All", "Files.Read.All", "Sites.Read.All", "Mail.Read", "Calendars.Read", "Contacts.Read"}

func msTenant(ps ProviderSettings) string {
	if t := strings.TrimSpace(ps.Extra["tenant"]); t != "" {
		return t
	}
	return "organizations"
}

// oauthConfig builds the authorization-code config for a provider.
func oauthConfig(ep OAuthEndpoints, provider string, ps ProviderSettings, redirect string) (*oauth2.Config, error) {
	if ps.ClientID == "" || ps.clientSecret.ClientSecret == "" {
		return nil, ErrNotConfigured
	}
	c := &oauth2.Config{ClientID: ps.ClientID, ClientSecret: ps.clientSecret.ClientSecret, RedirectURL: redirect}
	switch provider {
	case "google":
		c.Endpoint = oauth2.Endpoint{AuthURL: ep.GoogleAuth, TokenURL: ep.GoogleToken, AuthStyle: oauth2.AuthStyleInParams}
		c.Scopes = GoogleScopes
	case "microsoft":
		base := strings.TrimRight(ep.MSLogin, "/") + "/" + url.PathEscape(msTenant(ps)) + "/oauth2/v2.0"
		c.Endpoint = oauth2.Endpoint{AuthURL: base + "/authorize", TokenURL: base + "/token", AuthStyle: oauth2.AuthStyleInParams}
		c.Scopes = msDelegatedScopes
	default:
		return nil, fmt.Errorf("%s does not use OAuth", provider)
	}
	return c, nil
}

// ErrNotConfigured means the admin has not entered the provider's OAuth app.
var ErrNotConfigured = errors.New("not configured: an administrator must enter this provider's OAuth client ID and secret under Migration > Provider settings")

// persistingTS wraps a token source and saves refreshed tokens.
type persistingTS struct {
	src  oauth2.TokenSource
	last string
	save func(*oauth2.Token)
}

func (p *persistingTS) Token() (*oauth2.Token, error) {
	t, err := p.src.Token()
	if err == nil && t.AccessToken != p.last {
		p.last = t.AccessToken
		if p.save != nil {
			p.save(t)
		}
	}
	return t, err
}

type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// BuildSource turns a stored connection into a Source. save persists refreshed
// OAuth tokens. hc is the HTTP client (tests inject fakes).
func BuildSource(ctx context.Context, ep OAuthEndpoints, c Connection, ps ProviderSettings, redirect string, hc *http.Client, save func(*oauth2.Token)) (Source, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Minute}
	}
	octx := context.WithValue(ctx, oauth2.HTTPClient, hc)
	user := c.Config["username"]
	pass := c.secret.Password
	dav := &DAVClient{HTTP: hc, User: user, Password: pass}
	imapFrom := func(defHost, defPort string) *IMAPConfig {
		host := c.Config["imap_host"]
		if host == "" {
			host = defHost
		}
		if host == "" {
			return nil
		}
		port := c.Config["imap_port"]
		if port == "" {
			port = defPort
		}
		if port == "" {
			port = "993"
		}
		pw := c.secret.IMAPPassword
		if pw == "" {
			pw = pass
		}
		iu := c.Config["imap_username"]
		if iu == "" {
			iu = user
		}
		return &IMAPConfig{Addr: host + ":" + port, Security: c.Config["imap_security"], Username: iu, Password: pw}
	}
	switch c.Provider {
	case "nextcloud", "owncloud":
		f, cal, card := NextcloudURLs(c.Config["server"], user)
		src := &StandardSource{FilesURL: f, CalDAVURL: cal, CardDAVURL: card, DAV: dav}
		if c.Provider == "nextcloud" {
			src.IMAP = imapFrom("", "")
		}
		return src, nil
	case "imap":
		return &StandardSource{IMAP: imapFrom("", "")}, nil
	case "dav":
		return &StandardSource{FilesURL: c.Config["webdav_url"], CalDAVURL: c.Config["caldav_url"], CardDAVURL: c.Config["carddav_url"], DAV: dav}, nil
	case "zoho":
		r := c.Config["region"]
		if r == "" {
			r = "com"
		}
		src := &StandardSource{DAV: dav,
			CalDAVURL:  firstNonEmpty(c.Config["caldav_url"], "https://calendar.zoho."+r+"/caldav/"),
			CardDAVURL: firstNonEmpty(c.Config["carddav_url"], "https://contacts.zoho."+r+"/carddav/"),
			IMAP:       imapFrom("imap.zoho."+r, "993")}
		return src, nil
	case "google", "microsoft":
	default:
		return nil, fmt.Errorf("unknown provider %q", c.Provider)
	}

	// OAuth providers.
	var ts oauth2.TokenSource
	if c.Provider == "microsoft" && c.Scope == "org" {
		if ps.ClientID == "" || ps.clientSecret.ClientSecret == "" {
			return nil, ErrNotConfigured
		}
		if msTenant(ps) == "organizations" || msTenant(ps) == "common" {
			return nil, errors.New("organisation-wide Microsoft 365 migration needs the tenant ID in provider settings")
		}
		cc := clientcredentials.Config{ClientID: ps.ClientID, ClientSecret: ps.clientSecret.ClientSecret,
			TokenURL: strings.TrimRight(ep.MSLogin, "/") + "/" + url.PathEscape(msTenant(ps)) + "/oauth2/v2.0/token",
			Scopes:   []string{"https://graph.microsoft.com/.default"}, AuthStyle: oauth2.AuthStyleInParams}
		ts = cc.TokenSource(octx)
	} else {
		cfg, err := oauthConfig(ep, c.Provider, ps, redirect)
		if err != nil {
			return nil, err
		}
		if c.secret.RefreshToken == "" && c.secret.AccessToken == "" {
			return nil, errors.New("this connection is not signed in yet; use Connect to sign in with the provider")
		}
		tok := &oauth2.Token{AccessToken: c.secret.AccessToken, RefreshToken: c.secret.RefreshToken, Expiry: c.secret.Expiry, TokenType: "Bearer"}
		ts = &persistingTS{src: cfg.TokenSource(octx, tok), last: tok.AccessToken, save: save}
	}
	self := c.Config["account_email"]
	switch c.Provider {
	case "google":
		gep := DefaultGoogleEndpoints
		if ep.Google != nil {
			gep = *ep.Google
		}
		g := &GoogleSource{EP: gep, HTTP: hc, Token: ts, Self: self}
		if sa := ps.clientSecret.ServiceAccountJSON; sa != "" && c.Scope == "org" {
			var key serviceAccount
			if err := json.Unmarshal([]byte(sa), &key); err != nil {
				return nil, fmt.Errorf("service account JSON is invalid: %w", err)
			}
			tokenURL := firstNonEmpty(key.TokenURI, ep.GoogleToken)
			g.UserToken = func(email string) oauth2.TokenSource {
				jc := &jwt.Config{Email: key.ClientEmail, PrivateKey: []byte(key.PrivateKey), TokenURL: tokenURL, Subject: email,
					Scopes: GoogleScopes[2:]}
				return jc.TokenSource(octx)
			}
		}
		return g, nil
	default:
		return &MicrosoftSource{Graph: ep.Graph, HTTP: hc, Token: ts, Self: self}, nil
	}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
