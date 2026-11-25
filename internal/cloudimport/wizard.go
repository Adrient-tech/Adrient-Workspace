package cloudimport

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// Wizard serves the migration wizard API under /api/v1/import/wizard/...
//
//	GET    providers                         list providers + whether configured
//	GET    settings/{provider}               admin: OAuth app (secret never returned)
//	PUT    settings/{provider}               admin: save OAuth app
//	GET    connections                       my connections (+ org ones for admins)
//	POST   connections                       create (password providers: test first)
//	DELETE connections/{id}
//	POST   connections/{id}/oauth/start      -> {url} to send the browser to
//	GET    oauth/callback?state&code         vendor redirect target
//	POST   connections/{id}/discover         org scope: list remote users into the map
//	GET    connections/{id}/users            mapping table
//	PUT    connections/{id}/users/{remote}   {local_user_id, skip}
//	POST   connections/{id}/runs             {kinds, dry_run} start (or resume) a run
//	GET    connections/{id}/runs
//	GET    runs/{id}                         progress
//	POST   runs/{id}/cancel
//	GET    runs/{id}/errors[?format=csv]     error report
type Wizard struct {
	Store    *WizardStore
	Sink     Sink
	CallerOf func(ctx context.Context) (orgID, userID, email string, ok bool)
	IsAdmin  func(r *http.Request) bool
	Audit    func(ctx context.Context, service, action string, detail map[string]any)
	// RedirectURL is the absolute OAuth callback URL, e.g.
	// https://workspace.example.com/api/v1/import/wizard/oauth/callback
	RedirectURL string
	Endpoints   OAuthEndpoints
	HTTP        *http.Client // vendor HTTP client (nil = default)
	// AllowPrivateHosts permits connecting to servers on private/loopback
	// addresses (a self-hosted Nextcloud on the LAN). Off by default to avoid
	// server-side request forgery against internal services.
	AllowPrivateHosts bool

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

const wizardPrefix = "/api/v1/import/wizard/"

func (w *Wizard) audit(ctx context.Context, action string, d map[string]any) {
	if w.Audit != nil {
		w.Audit(ctx, "cloudimport", action, d)
	}
}

func (w *Wizard) endpoints() OAuthEndpoints {
	if w.Endpoints.GoogleToken == "" {
		return DefaultOAuthEndpoints
	}
	return w.Endpoints
}

// ResumeAfterRestart marks runs a previous process left running.
func (w *Wizard) ResumeAfterRestart(ctx context.Context) {
	if n, err := w.Store.MarkInterrupted(ctx); err == nil && n > 0 {
		slog.Info("cloudimport: marked interrupted migration runs", "count", n)
	}
}

func (w *Wizard) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, wizardPrefix), "/")
	parts := strings.Split(rest, "/")
	orgID, userID, email, ok := w.CallerOf(r.Context())
	if !ok {
		writeErr(rw, http.StatusUnauthorized, "no session")
		return
	}
	admin := w.IsAdmin != nil && w.IsAdmin(r)
	c := call{w: w, rw: rw, r: r, org: orgID, user: userID, email: email, admin: admin}
	switch {
	case rest == "providers" && r.Method == http.MethodGet:
		c.providers()
	case len(parts) == 2 && parts[0] == "settings":
		c.settings(parts[1])
	case rest == "connections" && r.Method == http.MethodGet:
		c.listConnections()
	case rest == "connections" && r.Method == http.MethodPost:
		c.createConnection()
	case rest == "oauth/callback" && r.Method == http.MethodGet:
		c.oauthCallback()
	case len(parts) >= 2 && parts[0] == "connections":
		conn, err := w.Store.GetConnection(r.Context(), orgID, parts[1])
		if err != nil || !c.canUse(conn) {
			writeErr(rw, http.StatusNotFound, "connection not found")
			return
		}
		sub := strings.Join(parts[2:], "/")
		switch {
		case sub == "" && r.Method == http.MethodDelete:
			c.deleteConnection(conn)
		case sub == "" && r.Method == http.MethodGet:
			writeJSON(rw, 200, c.connJSON(conn))
		case sub == "oauth/start" && r.Method == http.MethodPost:
			c.oauthStart(conn)
		case sub == "discover" && r.Method == http.MethodPost:
			c.discover(conn)
		case sub == "users" && r.Method == http.MethodGet:
			c.listUsers(conn)
		case len(parts) == 4 && parts[2] == "users" && r.Method == http.MethodPut:
			c.setUser(conn, parts[3])
		case sub == "runs" && r.Method == http.MethodPost:
			c.startRun(conn)
		case sub == "runs" && r.Method == http.MethodGet:
			runs, err := w.Store.ListRuns(r.Context(), orgID, conn.ID)
			if err != nil {
				writeErr(rw, 500, err.Error())
				return
			}
			if runs == nil {
				runs = []Run{}
			}
			writeJSON(rw, 200, map[string]any{"runs": runs})
		default:
			writeErr(rw, http.StatusNotFound, "not found")
		}
	case len(parts) >= 2 && parts[0] == "runs":
		run, err := w.Store.GetRun(r.Context(), orgID, parts[1])
		if err != nil {
			writeErr(rw, http.StatusNotFound, "run not found")
			return
		}
		conn, err := w.Store.GetConnection(r.Context(), orgID, run.ConnectionID)
		if err != nil || !c.canUse(conn) {
			writeErr(rw, http.StatusNotFound, "run not found")
			return
		}
		switch {
		case len(parts) == 2 && r.Method == http.MethodGet:
			writeJSON(rw, 200, run)
		case len(parts) == 3 && parts[2] == "cancel" && r.Method == http.MethodPost:
			w.mu.Lock()
			cancel := w.cancels[run.ID]
			w.mu.Unlock()
			if cancel == nil {
				writeErr(rw, http.StatusConflict, "run is not active")
				return
			}
			cancel()
			writeJSON(rw, 202, map[string]any{"status": "cancelling"})
		case len(parts) == 3 && parts[2] == "errors" && r.Method == http.MethodGet:
			c.errors(run)
		default:
			writeErr(rw, http.StatusNotFound, "not found")
		}
	default:
		writeErr(rw, http.StatusNotFound, "not found")
	}
}

type call struct {
	w                *Wizard
	rw               http.ResponseWriter
	r                *http.Request
	org, user, email string
	admin            bool
}

// canUse: your own connections; admins also see org-scope connections.
func (c call) canUse(conn Connection) bool {
	return conn.CreatedBy == c.user || (c.admin && conn.Scope == "org")
}

func (c call) decode(v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(c.rw, c.r.Body, 1<<20)).Decode(v); err != nil {
		writeErr(c.rw, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func (c call) providers() {
	out := make([]ProviderInfo, 0, len(Providers))
	for _, p := range Providers {
		if p.NeedsApp {
			ps, _, _ := c.w.Store.GetSettings(c.r.Context(), c.org, p.ID)
			p.Configured = ps.ClientID != "" && ps.HasSecret
		} else {
			p.Configured = true
		}
		if !c.admin {
			p.Scopes = []string{"me"}
		}
		out = append(out, p)
	}
	writeJSON(c.rw, 200, map[string]any{"providers": out, "is_admin": c.admin, "redirect_url": c.w.RedirectURL})
}

func (c call) settings(provider string) {
	if !c.admin {
		writeErr(c.rw, http.StatusForbidden, "only administrators can change provider settings")
		return
	}
	p, ok := ProviderByID(provider)
	if !ok || !p.NeedsApp {
		writeErr(c.rw, http.StatusNotFound, "provider has no OAuth settings")
		return
	}
	switch c.r.Method {
	case http.MethodGet:
		ps, _, err := c.w.Store.GetSettings(c.r.Context(), c.org, provider)
		if err != nil && !errors.Is(err, ErrNoSecretKey) {
			writeErr(c.rw, 500, err.Error())
			return
		}
		writeJSON(c.rw, 200, map[string]any{"provider": provider, "client_id": ps.ClientID, "has_secret": ps.HasSecret,
			"has_service_account": ps.clientSecret.ServiceAccountJSON != "", "extra": ps.Extra, "redirect_url": c.w.RedirectURL})
	case http.MethodPut:
		var in struct {
			ClientID           string            `json:"client_id"`
			ClientSecret       string            `json:"client_secret"`
			ServiceAccountJSON string            `json:"service_account_json"`
			Extra              map[string]string `json:"extra"`
		}
		if !c.decode(&in) {
			return
		}
		in.ClientID = strings.TrimSpace(in.ClientID)
		if len(in.ClientID) > 512 || len(in.ClientSecret) > 2048 || len(in.ServiceAccountJSON) > 16384 {
			writeErr(c.rw, 400, "value too long")
			return
		}
		if in.ServiceAccountJSON != "" {
			var sa serviceAccount
			if json.Unmarshal([]byte(in.ServiceAccountJSON), &sa) != nil || sa.ClientEmail == "" || sa.PrivateKey == "" {
				writeErr(c.rw, 400, "service account JSON must contain client_email and private_key")
				return
			}
		}
		clean := map[string]string{}
		for k, v := range in.Extra {
			if k == "tenant" {
				clean[k] = strings.TrimSpace(v)
			}
		}
		if err := c.w.Store.SaveSettings(c.r.Context(), c.org, provider, in.ClientID, in.ClientSecret, in.ServiceAccountJSON, clean); err != nil {
			writeErr(c.rw, 500, err.Error())
			return
		}
		c.w.audit(c.r.Context(), "provider_settings_saved", map[string]any{"provider": provider})
		writeJSON(c.rw, 200, map[string]any{"ok": true})
	default:
		writeErr(c.rw, 405, "method not allowed")
	}
}

func (c call) connJSON(conn Connection) map[string]any {
	cfg := map[string]string{}
	for k, v := range conn.Config {
		cfg[k] = v
	}
	return map[string]any{"id": conn.ID, "provider": conn.Provider, "label": conn.Label, "scope": conn.Scope, "config": cfg,
		"status": conn.Status, "created_at": conn.CreatedAt, "mine": conn.CreatedBy == c.user}
}

func (c call) listConnections() {
	list, err := c.w.Store.ListConnections(c.r.Context(), c.org, c.user, c.admin)
	if err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	out := []map[string]any{}
	for _, conn := range list {
		out = append(out, c.connJSON(conn))
	}
	writeJSON(c.rw, 200, map[string]any{"connections": out})
}

// checkHost refuses URLs/hosts on private networks unless allowed.
func (w *Wizard) checkHost(raw string, isURL bool) error {
	host := raw
	if isURL {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("%q is not an http(s) URL", raw)
		}
		if u.Scheme == "http" && !w.AllowPrivateHosts {
			return fmt.Errorf("%q must use https", raw)
		}
		host = u.Hostname()
	} else if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}
	if w.AllowPrivateHosts {
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("cannot resolve %s", host)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("%s is a private address; ask an administrator to allow private hosts (ADRIENT_IMPORT_ALLOW_PRIVATE_HOSTS=1)", host)
		}
	}
	return nil
}

func (c call) createConnection() {
	var in struct {
		Provider string            `json:"provider"`
		Label    string            `json:"label"`
		Scope    string            `json:"scope"`
		Config   map[string]string `json:"config"`
	}
	if !c.decode(&in) {
		return
	}
	p, ok := ProviderByID(in.Provider)
	if !ok {
		writeErr(c.rw, 400, "unknown provider")
		return
	}
	if in.Scope == "" {
		in.Scope = "me"
	}
	scopeOK := false
	for _, s := range p.Scopes {
		scopeOK = scopeOK || s == in.Scope
	}
	if !scopeOK {
		writeErr(c.rw, 400, "scope not supported by this provider")
		return
	}
	if in.Scope == "org" && !c.admin {
		writeErr(c.rw, 403, "only administrators can migrate a whole organisation")
		return
	}
	conn := Connection{OrgID: c.org, CreatedBy: c.user, Provider: p.ID, Label: strings.TrimSpace(in.Label), Scope: in.Scope, Config: map[string]string{}}
	if len(conn.Label) > 200 {
		conn.Label = conn.Label[:200]
	}
	allowed := map[string]Field{}
	for _, f := range p.Fields {
		allowed[f.Key] = f
	}
	allowed["imap_username"] = Field{Key: "imap_username"}
	allowed["imap_password"] = Field{Key: "imap_password", Secret: true}
	for k, v := range in.Config {
		f, ok := allowed[k]
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) > 2048 {
			writeErr(c.rw, 400, k+" is too long")
			return
		}
		switch {
		case k == "password":
			conn.secret.Password = v
		case k == "imap_password":
			conn.secret.IMAPPassword = v
		case !f.Secret:
			conn.Config[k] = v
		}
	}
	for _, f := range p.Fields {
		if f.Required && conn.Config[f.Key] == "" && !(f.Key == "password" && conn.secret.Password != "") {
			writeErr(c.rw, 400, f.Label+" is required")
			return
		}
	}
	for _, k := range []string{"server", "caldav_url", "carddav_url", "webdav_url"} {
		if v := conn.Config[k]; v != "" {
			if err := c.w.checkHost(v, true); err != nil {
				writeErr(c.rw, 400, err.Error())
				return
			}
		}
	}
	if h := conn.Config["imap_host"]; h != "" {
		if err := c.w.checkHost(h, false); err != nil {
			writeErr(c.rw, 400, err.Error())
			return
		}
	}
	if conn.Label == "" {
		conn.Label = p.Name
		if u := conn.Config["username"]; u != "" {
			conn.Label += " (" + u + ")"
		}
	}
	if p.Auth == "oauth" {
		if !(in.Scope == "org" && p.ID == "microsoft") {
			conn.Status = "needs_auth"
		} else {
			conn.Status = "ready"
		}
		ps, _, err := c.w.Store.GetSettings(c.r.Context(), c.org, p.ID)
		if err != nil || ps.ClientID == "" || !ps.HasSecret {
			writeErr(c.rw, 409, ErrNotConfigured.Error())
			return
		}
	} else {
		conn.Status = "ready"
		// Test the credentials before storing them.
		src, err := BuildSource(c.r.Context(), c.w.endpoints(), conn, ProviderSettings{}, "", c.w.HTTP, nil)
		if err == nil {
			err = testSource(c.r.Context(), src)
		}
		if err != nil {
			writeErr(c.rw, 400, "connection test failed: "+err.Error())
			return
		}
	}
	out, err := c.w.Store.CreateConnection(c.r.Context(), conn)
	if err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	c.w.audit(c.r.Context(), "connection_created", map[string]any{"provider": p.ID, "scope": in.Scope, "connection": out.ID})
	writeJSON(c.rw, 201, c.connJSON(out))
}

// testSource makes one cheap read per configured kind.
func testSource(ctx context.Context, src Source) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ss, ok := src.(*StandardSource)
	if !ok {
		return nil
	}
	if ss.IMAP != nil && ss.IMAP.Addr != "" {
		if _, err := TestIMAP(*ss.IMAP); err != nil {
			return err
		}
	}
	if ss.FilesURL != "" {
		if _, err := ss.DAV.multistatus(ctx, "PROPFIND", ss.FilesURL, "0", propfindFiles); err != nil {
			return fmt.Errorf("files: %w", err)
		}
	}
	if ss.CalDAVURL != "" {
		if _, err := ss.DAV.multistatus(ctx, "PROPFIND", ss.CalDAVURL, "0", propfindHome); err != nil {
			return fmt.Errorf("calendar: %w", err)
		}
	}
	if ss.CardDAVURL != "" {
		if _, err := ss.DAV.multistatus(ctx, "PROPFIND", ss.CardDAVURL, "0", propfindHome); err != nil {
			return fmt.Errorf("contacts: %w", err)
		}
	}
	return nil
}

func (c call) deleteConnection(conn Connection) {
	if id, active, _ := c.w.Store.ActiveRun(c.r.Context(), c.org, conn.ID); active {
		c.w.mu.Lock()
		if cancel := c.w.cancels[id]; cancel != nil {
			cancel()
		}
		c.w.mu.Unlock()
	}
	if err := c.w.Store.DeleteConnection(c.r.Context(), c.org, conn.ID); err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	c.w.audit(c.r.Context(), "connection_deleted", map[string]any{"connection": conn.ID, "provider": conn.Provider})
	writeJSON(c.rw, 200, map[string]any{"ok": true})
}

func randomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashState(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (c call) oauthStart(conn Connection) {
	ps, _, err := c.w.Store.GetSettings(c.r.Context(), c.org, conn.Provider)
	if err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	cfg, err := oauthConfig(c.w.endpoints(), conn.Provider, ps, c.w.RedirectURL)
	if err != nil {
		writeErr(c.rw, 409, err.Error())
		return
	}
	if c.w.RedirectURL == "" {
		writeErr(c.rw, 409, "server has no public URL configured for OAuth callbacks")
		return
	}
	st, err := randomState()
	if err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	sec := conn.secret
	sec.OAuthState = hashState(st)
	if err := c.w.Store.SaveSecret(c.r.Context(), c.org, conn.ID, sec); err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	// state = connection id + random; the random half is verified against the hash.
	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent")}
	if conn.Provider == "google" {
		opts = append(opts, oauth2.SetAuthURLParam("include_granted_scopes", "true"))
	}
	writeJSON(c.rw, 200, map[string]any{"url": cfg.AuthCodeURL(conn.ID+"."+st, opts...)})
}

func (c call) oauthCallback() {
	q := c.r.URL.Query()
	back := func(status, msg string) {
		v := url.Values{"oauth": {status}}
		if msg != "" {
			v.Set("message", msg)
		}
		http.Redirect(c.rw, c.r, "/cloudimport?"+v.Encode(), http.StatusFound)
	}
	if e := q.Get("error"); e != "" {
		back("error", e+": "+q.Get("error_description"))
		return
	}
	connID, st, ok := strings.Cut(q.Get("state"), ".")
	if !ok || st == "" {
		back("error", "missing state")
		return
	}
	conn, err := c.w.Store.GetConnection(c.r.Context(), c.org, connID)
	if err != nil || conn.CreatedBy != c.user || conn.secret.OAuthState == "" || conn.secret.OAuthState != hashState(st) {
		back("error", "this sign-in link is not valid for your session")
		return
	}
	ps, _, err := c.w.Store.GetSettings(c.r.Context(), c.org, conn.Provider)
	if err != nil {
		back("error", err.Error())
		return
	}
	cfg, err := oauthConfig(c.w.endpoints(), conn.Provider, ps, c.w.RedirectURL)
	if err != nil {
		back("error", err.Error())
		return
	}
	ctx := c.r.Context()
	if c.w.HTTP != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, c.w.HTTP)
	}
	tok, err := cfg.Exchange(ctx, q.Get("code"))
	if err != nil {
		back("error", "token exchange failed: "+err.Error())
		return
	}
	sec := connSecret{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, Expiry: tok.Expiry}
	if err := c.w.Store.SaveSecret(c.r.Context(), c.org, conn.ID, sec); err != nil {
		back("error", err.Error())
		return
	}
	// Record which account signed in (id_token email claim when present).
	if idt, _ := tok.Extra("id_token").(string); idt != "" {
		if em := jwtEmail(idt); em != "" {
			conn.Config["account_email"] = em
			_ = c.w.Store.SetConfig(c.r.Context(), c.org, conn.ID, conn.Config)
		}
	}
	_ = c.w.Store.SetConnStatus(c.r.Context(), c.org, conn.ID, "ready")
	c.w.audit(c.r.Context(), "connection_authorized", map[string]any{"connection": conn.ID, "provider": conn.Provider})
	back("ok", "")
}

func (c call) discover(conn Connection) {
	if conn.Scope != "org" {
		writeErr(c.rw, 400, "discovery is only for organisation-wide connections")
		return
	}
	src, err := c.w.source(c.r.Context(), conn)
	if err != nil {
		writeErr(c.rw, 409, err.Error())
		return
	}
	n, mapped := 0, 0
	err = src.Users(c.r.Context(), func(u RemoteUser) error {
		n++
		id, skip, known, err := c.w.Store.UserMapping(c.r.Context(), conn.ID, u.ID)
		if err != nil {
			return err
		}
		if !known || (id == "" && !skip) {
			if lid, ok, _ := c.w.Sink.FindUserByEmail(c.r.Context(), c.org, u.Email); ok {
				id = lid
			}
			if err := c.w.Store.SaveRemoteUser(c.r.Context(), conn.ID, u, id); err != nil {
				return err
			}
		}
		if id != "" {
			mapped++
		}
		return nil
	})
	if err != nil {
		writeErr(c.rw, 502, "listing users failed: "+err.Error())
		return
	}
	writeJSON(c.rw, 200, map[string]any{"found": n, "matched": mapped})
}

func (c call) listUsers(conn Connection) {
	rows, err := c.w.Store.ListUserMap(c.r.Context(), c.org, conn.ID)
	if err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	if rows == nil {
		rows = []UserMapRow{}
	}
	writeJSON(c.rw, 200, map[string]any{"users": rows})
}

func (c call) setUser(conn Connection, remote string) {
	remote, _ = url.PathUnescape(remote)
	var in struct {
		LocalUserID string `json:"local_user_id"`
		LocalEmail  string `json:"local_email"`
		Skip        bool   `json:"skip"`
	}
	if !c.decode(&in) {
		return
	}
	if in.LocalUserID == "" && in.LocalEmail != "" {
		id, ok, err := c.w.Sink.FindUserByEmail(c.r.Context(), c.org, in.LocalEmail)
		if err != nil || !ok {
			writeErr(c.rw, 400, "no Adrient user with e-mail "+in.LocalEmail)
			return
		}
		in.LocalUserID = id
	}
	if err := c.w.Store.SetUserMap(c.r.Context(), c.org, conn.ID, remote, in.LocalUserID, in.Skip); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(c.rw, 404, "remote user not found")
			return
		}
		writeErr(c.rw, 400, err.Error())
		return
	}
	writeJSON(c.rw, 200, map[string]any{"ok": true})
}

func (w *Wizard) source(ctx context.Context, conn Connection) (Source, error) {
	ps, _, err := w.Store.GetSettings(ctx, conn.OrgID, conn.Provider)
	if err != nil && !errors.Is(err, ErrNoSecretKey) {
		return nil, err
	}
	save := func(t *oauth2.Token) {
		sec := conn.secret
		sec.AccessToken, sec.Expiry = t.AccessToken, t.Expiry
		if t.RefreshToken != "" {
			sec.RefreshToken = t.RefreshToken
		}
		_ = w.Store.SaveSecret(context.Background(), conn.OrgID, conn.ID, sec)
	}
	return BuildSource(ctx, w.endpoints(), conn, ps, w.RedirectURL, w.HTTP, save)
}

func (c call) startRun(conn Connection) {
	var in struct {
		Kinds  []string `json:"kinds"`
		DryRun bool     `json:"dry_run"`
	}
	if !c.decode(&in) {
		return
	}
	if conn.Status != "ready" {
		writeErr(c.rw, 409, "connect (sign in) first")
		return
	}
	p, _ := ProviderByID(conn.Provider)
	valid := map[Kind]bool{}
	for _, k := range p.Kinds {
		valid[k] = true
	}
	var kinds []Kind
	var ks []string
	for _, k := range in.Kinds {
		if valid[Kind(k)] {
			kinds = append(kinds, Kind(k))
			ks = append(ks, k)
		}
	}
	if len(kinds) == 0 {
		writeErr(c.rw, 400, "choose at least one kind of data")
		return
	}
	if _, active, _ := c.w.Store.ActiveRun(c.r.Context(), c.org, conn.ID); active {
		writeErr(c.rw, 409, "a run is already in progress for this connection")
		return
	}
	src, err := c.w.source(c.r.Context(), conn)
	if err != nil {
		writeErr(c.rw, 409, err.Error())
		return
	}
	run, err := c.w.Store.CreateRun(c.r.Context(), c.org, conn.ID, c.user, ks, in.DryRun)
	if err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	cfg := RunConfig{RunID: run.ID, OrgID: c.org, ConnID: conn.ID, ActingUserID: c.user, Scope: conn.Scope,
		SelfEmail: firstNonEmpty(conn.Config["account_email"], conn.Config["username"], c.email), Kinds: kinds, DryRun: in.DryRun,
		RootFolder: "Imported from " + p.Short}
	c.w.audit(c.r.Context(), "run_started", map[string]any{"connection": conn.ID, "provider": conn.Provider, "kinds": ks, "dry_run": in.DryRun})
	c.w.launch(cfg, src)
	writeJSON(c.rw, 202, run)
}

// launch runs the engine in the background.
func (w *Wizard) launch(cfg RunConfig, src Source) {
	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	if w.cancels == nil {
		w.cancels = map[string]context.CancelFunc{}
	}
	w.cancels[cfg.RunID] = cancel
	w.mu.Unlock()
	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.cancels, cfg.RunID)
			w.mu.Unlock()
			cancel()
		}()
		_ = w.Store.SetRunStatus(context.Background(), cfg.RunID, "running", "")
		e := &Engine{Src: src, Sink: w.Sink, St: w.Store}
		_, err := e.Run(ctx, cfg)
		status, msg := "done", ""
		switch {
		case errors.Is(err, context.Canceled):
			status, msg = "cancelled", "cancelled; start the run again to continue where it stopped"
		case err != nil:
			status, msg = "failed", err.Error()
		case cfg.DryRun:
			msg = "dry run: nothing was written"
		}
		_ = w.Store.SetRunStatus(context.Background(), cfg.RunID, status, msg)
	}()
}

func (c call) errors(run Run) {
	list, err := c.w.Store.ListErrors(c.r.Context(), c.org, run.ID, 10000)
	if err != nil {
		writeErr(c.rw, 500, err.Error())
		return
	}
	if c.r.URL.Query().Get("format") == "csv" {
		c.rw.Header().Set("Content-Type", "text/csv; charset=utf-8")
		c.rw.Header().Set("Content-Disposition", `attachment; filename="import-errors-`+run.ID+`.csv"`)
		cw := csv.NewWriter(c.rw)
		_ = cw.Write([]string{"time", "kind", "item", "remote_id", "error"})
		for _, e := range list {
			_ = cw.Write([]string{e.At.Format(time.RFC3339), e.Kind, csvSafe(e.Label), csvSafe(e.RemoteID), csvSafe(e.Message)})
		}
		cw.Flush()
		return
	}
	if list == nil {
		list = []ImportError{}
	}
	writeJSON(c.rw, 200, map[string]any{"errors": list})
}

// csvSafe neutralises spreadsheet formula injection in exported values.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// jwtEmail reads the email claim of an id_token WITHOUT verifying it; it is
// only used as a display label and a default mailbox name, never for access.
func jwtEmail(tok string) string {
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return ""
	}
	b, err := decodeSegment(parts[1])
	if err != nil {
		return ""
	}
	var cl struct {
		Email string `json:"email"`
		UPN   string `json:"preferred_username"`
	}
	_ = json.Unmarshal(b, &cl)
	return firstNonEmpty(cl.Email, cl.UPN)
}

// CallbackURL derives the OAuth callback from the server's OIDC redirect URL
// (same public origin). Returns "" when it cannot be derived.
func CallbackURL(oidcRedirect string) string {
	u, err := url.Parse(oidcRedirect)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + wizardPrefix + "oauth/callback"
}
