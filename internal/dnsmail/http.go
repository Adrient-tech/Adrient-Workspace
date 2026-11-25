package dnsmail

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgdomains"
)

// AuditFn records an event (audit.Recorder.Emit).
type AuditFn func(ctx context.Context, service, action string, detail map[string]any)

// Handler serves the admin DNS/DKIM APIs and the public client-config endpoints.
type Handler struct {
	Store      *Store
	Domains    *orgdomains.Store
	Resolver   Resolver
	Fetch      Fetcher
	Audit      AuditFn
	Secret     string // server secret for DKIM private keys
	WebHost    string // host serving autoconfig/MTA-STS/DAV; defaults to the request host
	SPFInclude string
	Now        func() time.Time
}

const adminPrefix = "/api/v1/admin/domains/"

// MatchAdmin reports whether path is an admin DNS route.
func MatchAdmin(path string) bool {
	if !strings.HasPrefix(path, adminPrefix) {
		return false
	}
	_, a, _ := strings.Cut(strings.TrimPrefix(strings.TrimRight(path, "/"), adminPrefix), "/")
	return strings.HasPrefix(a, "dns/") || strings.HasPrefix(a, "dkim/") || a == "mail-settings"
}

// MatchPublic reports whether path is a public client-config endpoint.
func MatchPublic(path string) bool {
	switch strings.TrimRight(path, "/") {
	case "/.well-known/mta-sts.txt", "/mail/config-v1.1.xml", "/.well-known/autoconfig/mail/config-v1.1.xml",
		"/autodiscover/autodiscover.xml", "/Autodiscover/Autodiscover.xml", "/mobileconfig":
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jerr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Handler) audit(r *http.Request, action string, d map[string]any) {
	if h.Audit != nil {
		if d == nil {
			d = map[string]any{}
		}
		h.Audit(r.Context(), "dns", action, d)
	}
}

func hostOnly(r *http.Request) string {
	h := r.Host
	if x, _, err := net.SplitHostPort(h); err == nil {
		h = x
	}
	return strings.ToLower(h)
}

func (h *Handler) webHost(r *http.Request) string {
	if h.WebHost != "" {
		return h.WebHost
	}
	return hostOnly(r)
}

// ServeHTTP routes both admin and public requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if MatchPublic(r.URL.Path) {
		h.servePublic(w, r)
		return
	}
	h.serveAdmin(w, r)
}

type viewKey struct {
	ID          string     `json:"id"`
	Selector    string     `json:"selector"`
	Algo        string     `json:"algo"`
	State       string     `json:"state"`
	PublicKey   string     `json:"public_key"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
}

func viewKeys(ks []Key) []viewKey {
	out := []viewKey{}
	for _, k := range ks {
		out = append(out, viewKey{k.ID, k.Selector, k.Algo, k.State, k.PublicKey, k.CreatedAt, k.ActivatedAt})
	}
	return out
}

func (h *Handler) planInput(ctx context.Context, r *http.Request, d orgdomains.Domain) (PlanInput, Settings, []Key, error) {
	st, err := h.Store.GetSettings(ctx, d.ID)
	if err != nil {
		return PlanInput{}, st, nil, err
	}
	ks, err := h.Store.Keys(ctx, d.ID)
	if err != nil {
		return PlanInput{}, st, nil, err
	}
	return PlanFor(d.Domain, st, ks, h.webHost(r), "mail."+d.Domain, h.SPFInclude), st, ks, nil
}

func (h *Handler) serveAdmin(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	if _, hasUser := auth.UserFromContext(r.Context()); !ok || !hasUser {
		jerr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !authz.Can(r.Context(), orgdomains.PermDomainWrite) {
		authz.WriteForbidden(w, orgdomains.PermDomainWrite)
		return
	}
	rest := strings.TrimPrefix(strings.TrimRight(r.URL.Path, "/"), adminPrefix)
	id, action, _ := strings.Cut(rest, "/")
	d, err := h.Domains.Get(r.Context(), o.ID, id)
	if err != nil {
		jerr(w, http.StatusNotFound, "domain not found")
		return
	}
	if d.Status != orgdomains.StatusVerified {
		jerr(w, http.StatusConflict, "verify the domain before setting up mail")
		return
	}
	ctx := r.Context()
	switch {
	case action == "dns/plan" && r.Method == http.MethodGet:
		in, st, ks, err := h.planInput(ctx, r, d)
		if err != nil {
			jerr(w, 500, "plan failed")
			return
		}
		runs, _ := h.Store.Runs(ctx, d.ID)
		days := CleanDays(runs, h.now())
		writeJSON(w, 200, map[string]any{"domain": d.Domain, "records": Plan(in), "settings": st, "keys": viewKeys(ks),
			"clean_days": days, "ramp_hint": RampHint(st.DMARCPolicy, days), "mail_host": in.MailHost, "web_host": in.WebHost,
			"mail_backend": mailBackendName()})
	case action == "dns/report" && r.Method == http.MethodGet:
		rs, err := h.Store.Report(ctx, d.ID)
		if err != nil {
			jerr(w, 500, "report failed")
			return
		}
		writeJSON(w, 200, map[string]any{"results": rs})
	case action == "dns/check" && r.Method == http.MethodPost:
		in, st, _, err := h.planInput(ctx, r, d)
		if err != nil {
			jerr(w, 500, "check failed")
			return
		}
		if c, ok := h.Resolver.(*CachedResolver); ok {
			c.Flush()
		}
		cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		rs := Check(cctx, h.Resolver, h.Fetch, in)
		if err := h.Store.SaveResults(ctx, d.ID, rs); err != nil {
			jerr(w, 500, "could not store results")
			return
		}
		runs, _ := h.Store.Runs(ctx, d.ID)
		days := CleanDays(runs, h.now())
		p, wn, f := Summary(rs)
		h.audit(r, "check", map[string]any{"resource_type": "domain", "resource_id": d.ID, "domain": d.Domain, "pass": p, "warn": wn, "fail": f})
		writeJSON(w, 200, map[string]any{"results": rs, "pass": p, "warn": wn, "fail": f, "clean": Clean(rs),
			"clean_days": days, "ramp_hint": RampHint(st.DMARCPolicy, days)})
	case action == "dkim/generate" && r.Method == http.MethodPost, action == "dkim/rotate" && r.Method == http.MethodPost:
		var in struct {
			Algo string `json:"algo"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in)
		if in.Algo != "" && in.Algo != AlgoRSA && in.Algo != AlgoEd25519 {
			jerr(w, 400, "algo must be rsa-2048 or ed25519")
			return
		}
		k, err := h.Store.CreateNext(ctx, h.Secret, d.ID, in.Algo)
		if errors.Is(err, ErrNoServerKey) {
			jerr(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err != nil {
			jerr(w, 500, "could not create key")
			return
		}
		h.audit(r, "dkim."+strings.TrimPrefix(action, "dkim/"), map[string]any{"resource_type": "domain", "resource_id": d.ID, "domain": d.Domain, "selector": k.Selector, "algo": k.Algo})
		writeJSON(w, 201, viewKeys([]Key{k})[0])
	case action == "dkim/activate" && r.Method == http.MethodPost:
		ks, err := h.Store.Keys(ctx, d.ID)
		if err != nil {
			jerr(w, 500, "activate failed")
			return
		}
		var next *Key
		for i := range ks {
			if ks[i].State == StateNext {
				next = &ks[i]
			}
		}
		if next == nil {
			jerr(w, 409, "no next key to activate; generate one first")
			return
		}
		txt, _ := h.Resolver.LookupTXT(ctx, next.Selector+"._domainkey."+d.Domain)
		published := false
		for _, t := range txt {
			if strings.ReplaceAll(tagMap(t)["p"], " ", "") == next.PublicKey {
				published = true
			}
		}
		if _, err := (Rotation{Keys: ks, Now: h.now()}).CanActivate(published); err != nil {
			jerr(w, 409, err.Error())
			return
		}
		k, err := h.Store.Activate(ctx, d.ID)
		if err != nil {
			jerr(w, 500, "activate failed")
			return
		}
		h.audit(r, "dkim.activate", map[string]any{"resource_type": "domain", "resource_id": d.ID, "domain": d.Domain, "selector": k.Selector})
		writeJSON(w, 200, viewKeys([]Key{k})[0])
	case action == "dkim/export" && r.Method == http.MethodGet:
		h.export(w, r, d)
	case action == "mail-settings" && r.Method == http.MethodPut:
		var in Settings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil {
			jerr(w, 400, "invalid body")
			return
		}
		before, _ := h.Store.GetSettings(ctx, d.ID)
		st, err := h.Store.PutSettings(ctx, d.ID, in)
		if err != nil {
			jerr(w, 400, err.Error())
			return
		}
		h.audit(r, "mail_settings.change", map[string]any{"resource_type": "domain", "resource_id": d.ID, "domain": d.Domain, "before": before, "after": st})
		writeJSON(w, 200, st)
	default:
		jerr(w, 404, "not found")
	}
}

// export returns the key material and the steps to import it into mailcow.
// The private key leaves the server only here, to an admin, and is audited.
func (h *Handler) export(w http.ResponseWriter, r *http.Request, d orgdomains.Domain) {
	ks, err := h.Store.Keys(r.Context(), d.ID)
	if err != nil {
		jerr(w, 500, "export failed")
		return
	}
	want := r.URL.Query().Get("selector")
	for _, k := range ks {
		if (want != "" && k.Selector != want) || (want == "" && k.State == StateRetired) {
			continue
		}
		raw, err := Decrypt(h.Secret, k.Private, []byte(d.ID+"|"+k.Selector))
		if err != nil {
			jerr(w, 500, "key cannot be decrypted with the current server secret")
			return
		}
		h.audit(r, "dkim.export", map[string]any{"resource_type": "domain", "resource_id": d.ID, "domain": d.Domain, "selector": k.Selector})
		writeJSON(w, 200, map[string]any{"domain": d.Domain, "selector": k.Selector, "algo": k.Algo, "state": k.State,
			"public_key": k.PublicKey, "private_key_pem": string(raw), "mailcow_steps": MailcowSteps(d.Domain, k.Selector)})
		return
	}
	jerr(w, 404, "no such key")
}

// MailcowSteps documents how to give mailcow the Adrient-generated key. Adrient
// does not provision mailcow (mailboxes, domains) in this release.
func MailcowSteps(domain, selector string) []string {
	return []string{
		"In mailcow open Configuration > Configuration & Details > ARC/DKIM keys.",
		"Under 'Import' enter domain " + domain + " and selector " + selector + ", paste the private key PEM and save.",
		"Make sure the domain " + domain + " exists in mailcow (Mail setup > Domains) so it signs outgoing mail.",
		"Publish the TXT record " + selector + "._domainkey." + domain + " shown in the wizard, wait until the check is green, then click 'Activate signing' here.",
		"Delete the exported private key from your clipboard and shell history.",
	}
}

func mailBackendName() string {
	return mailBackendEnv()
}

// ---------------------------------------------------------------- public

var emailRe = regexp.MustCompile(`^[^@\s]+@([A-Za-z0-9.-]+)$`)

func domainFromHost(host string) string {
	for _, p := range []string{"autoconfig.", "autodiscover.", "mta-sts."} {
		if strings.HasPrefix(host, p) {
			return strings.TrimPrefix(host, p)
		}
	}
	return ""
}

func (h *Handler) publicDomain(r *http.Request, email string) (DomainRef, Settings, bool) {
	cand := domainFromHost(hostOnly(r))
	if m := emailRe.FindStringSubmatch(strings.TrimSpace(email)); m != nil {
		cand = strings.ToLower(m[1])
	}
	if cand == "" {
		return DomainRef{}, Settings{}, false
	}
	d, ok := h.Store.Verified(r.Context(), cand)
	if !ok {
		return DomainRef{}, Settings{}, false
	}
	st, err := h.Store.GetSettings(r.Context(), d.ID)
	if err != nil {
		return DomainRef{}, Settings{}, false
	}
	if st.MailHost == "" {
		st.MailHost = "mail." + d.Domain
	}
	return d, st, true
}

func (h *Handler) servePublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := strings.TrimRight(r.URL.Path, "/")
	switch p {
	case "/.well-known/mta-sts.txt":
		host := hostOnly(r)
		if !strings.HasPrefix(host, "mta-sts.") {
			http.NotFound(w, r)
			return
		}
		d, st, ok := h.publicDomain(r, "")
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = d
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = io.WriteString(w, MTASTSPolicy(st.MTASTSMode, st.MailHost))
	case "/mail/config-v1.1.xml", "/.well-known/autoconfig/mail/config-v1.1.xml":
		email := r.URL.Query().Get("emailaddress")
		d, st, ok := h.publicDomain(r, email)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = io.WriteString(w, AutoconfigXML(d.Domain, st.MailHost))
	case "/autodiscover/autodiscover.xml", "/Autodiscover/Autodiscover.xml":
		email := r.URL.Query().Get("Email")
		if r.Method == http.MethodPost {
			var body struct {
				Email string `xml:"Request>EMailAddress"`
			}
			b, _ := io.ReadAll(io.LimitReader(r.Body, 16<<10))
			_ = xml.Unmarshal(b, &body)
			email = body.Email
		}
		d, st, ok := h.publicDomain(r, email)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = io.WriteString(w, AutodiscoverXML(strings.TrimSpace(email), d.Domain, st.MailHost))
	case "/mobileconfig":
		email := r.URL.Query().Get("email")
		d, st, ok := h.publicDomain(r, email)
		if !ok || emailRe.FindStringSubmatch(email) == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-apple-aspen-config")
		w.Header().Set("Content-Disposition", `attachment; filename="`+d.Domain+`.mobileconfig"`)
		_, _ = io.WriteString(w, MobileConfig(email, d.Domain, st.MailHost))
	}
}

// AutoconfigXML is the Thunderbird autoconfig document.
func AutoconfigXML(domain, mailHost string) string {
	d, m := html.EscapeString(domain), html.EscapeString(mailHost)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<clientConfig version="1.1">
  <emailProvider id="%[1]s">
    <domain>%[1]s</domain>
    <displayName>%[1]s mail</displayName>
    <incomingServer type="imap">
      <hostname>%[2]s</hostname><port>993</port><socketType>SSL</socketType>
      <authentication>password-cleartext</authentication><username>%%EMAILADDRESS%%</username>
    </incomingServer>
    <outgoingServer type="smtp">
      <hostname>%[2]s</hostname><port>587</port><socketType>STARTTLS</socketType>
      <authentication>password-cleartext</authentication><username>%%EMAILADDRESS%%</username>
    </outgoingServer>
  </emailProvider>
</clientConfig>
`, d, m)
}

// AutodiscoverXML is the Outlook autodiscover response.
func AutodiscoverXML(email, domain, mailHost string) string {
	m, e := html.EscapeString(mailHost), html.EscapeString(email)
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006">
  <Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">
    <Account>
      <AccountType>email</AccountType>
      <Action>settings</Action>
      <Protocol><Type>IMAP</Type><Server>%[1]s</Server><Port>993</Port><DomainRequired>off</DomainRequired>
        <LoginName>%[2]s</LoginName><SPA>off</SPA><SSL>on</SSL><AuthRequired>on</AuthRequired></Protocol>
      <Protocol><Type>SMTP</Type><Server>%[1]s</Server><Port>587</Port><DomainRequired>off</DomainRequired>
        <LoginName>%[2]s</LoginName><SPA>off</SPA><Encryption>TLS</Encryption><AuthRequired>on</AuthRequired></Protocol>
    </Account>
  </Response>
</Autodiscover>
`, m, e)
}

// MobileConfig is an unsigned Apple configuration profile for the mail account.
func MobileConfig(email, domain, mailHost string) string {
	f := fnv.New64a()
	_, _ = f.Write([]byte(strings.ToLower(email)))
	h1 := f.Sum64() % 1000000000000
	_, _ = f.Write([]byte("|profile"))
	h2 := f.Sum64() % 1000000000000
	e, m, d := html.EscapeString(email), html.EscapeString(mailHost), html.EscapeString(domain)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>PayloadContent</key><array><dict>
    <key>EmailAccountType</key><string>EmailTypeIMAP</string>
    <key>EmailAddress</key><string>%[1]s</string>
    <key>IncomingMailServerHostName</key><string>%[2]s</string>
    <key>IncomingMailServerPortNumber</key><integer>993</integer>
    <key>IncomingMailServerUseSSL</key><true/>
    <key>IncomingMailServerUsername</key><string>%[1]s</string>
    <key>IncomingMailServerAuthentication</key><string>EmailAuthPassword</string>
    <key>OutgoingMailServerHostName</key><string>%[2]s</string>
    <key>OutgoingMailServerPortNumber</key><integer>587</integer>
    <key>OutgoingMailServerUseSSL</key><true/>
    <key>OutgoingMailServerUsername</key><string>%[1]s</string>
    <key>OutgoingMailServerAuthentication</key><string>EmailAuthPassword</string>
    <key>PayloadType</key><string>com.apple.mail.managed</string>
    <key>PayloadVersion</key><integer>1</integer>
    <key>PayloadIdentifier</key><string>com.adrient.mail.%[3]s.account</string>
    <key>PayloadUUID</key><string>6F1D0B7A-5C1E-4B1E-9D55-%[4]012d</string>
  </dict></array>
  <key>PayloadDisplayName</key><string>%[3]s mail</string>
  <key>PayloadIdentifier</key><string>com.adrient.mail.%[3]s</string>
  <key>PayloadType</key><string>Configuration</string>
  <key>PayloadUUID</key><string>3B0E5E2A-2A6C-4D3F-8E1B-%[5]012d</string>
  <key>PayloadVersion</key><integer>1</integer>
</dict></plist>
`, e, m, d, h1, h2)
}
