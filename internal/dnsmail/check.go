package dnsmail

import (
	"context"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Check statuses.
const (
	Pass = "pass"
	Warn = "warn"
	Fail = "fail"
)

// MX is one MX answer.
type MX struct {
	Host string
	Pref uint16
}

// SRV is one SRV answer.
type SRV struct {
	Target                 string
	Port, Priority, Weight uint16
}

// Resolver is the DNS surface the checker needs. Tests inject a fake.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupMX(ctx context.Context, name string) ([]MX, error)
	LookupSRV(ctx context.Context, name string) ([]SRV, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
}

// Fetcher fetches an HTTPS URL (MTA-STS policy, autoconfig).
type Fetcher func(ctx context.Context, url string) (body string, status int, err error)

// Result is the outcome for one record.
type Result struct {
	Record
	Status   string   `json:"status"`
	Expected string   `json:"expected"`
	Found    []string `json:"found"`
	Message  string   `json:"message"`
}

// NetResolver queries public resolvers (1.1.1.1, 8.8.8.8), never the host's own
// resolver, with a per-query timeout.
type NetResolver struct{ Timeout time.Duration }

func (n NetResolver) each(ctx context.Context, fn func(r *net.Resolver) error) error {
	to := n.Timeout
	if to == 0 {
		to = 4 * time.Second
	}
	var last error
	for _, srv := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
		srv := srv
		r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: to}
			return d.DialContext(ctx, "udp", srv)
		}}
		err := fn(r)
		if err == nil {
			return nil
		}
		last = err
	}
	return last
}

func (n NetResolver) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	to := n.Timeout
	if to == 0 {
		to = 4 * time.Second
	}
	return context.WithTimeout(ctx, to)
}

// LookupTXT implements Resolver.
func (n NetResolver) LookupTXT(ctx context.Context, name string) (out []string, err error) {
	err = n.each(ctx, func(r *net.Resolver) error {
		c, cancel := n.withTimeout(ctx)
		defer cancel()
		v, e := r.LookupTXT(c, name)
		if e == nil && len(v) == 0 {
			e = &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
		}
		out = v
		return e
	})
	return
}

// LookupMX implements Resolver.
func (n NetResolver) LookupMX(ctx context.Context, name string) (out []MX, err error) {
	err = n.each(ctx, func(r *net.Resolver) error {
		c, cancel := n.withTimeout(ctx)
		defer cancel()
		v, e := r.LookupMX(c, name)
		out = out[:0]
		for _, m := range v {
			out = append(out, MX{Host: m.Host, Pref: m.Pref})
		}
		if e == nil && len(out) == 0 {
			e = &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
		}
		return e
	})
	return
}

// LookupSRV implements Resolver.
func (n NetResolver) LookupSRV(ctx context.Context, name string) (out []SRV, err error) {
	err = n.each(ctx, func(r *net.Resolver) error {
		c, cancel := n.withTimeout(ctx)
		defer cancel()
		_, v, e := r.LookupSRV(c, "", "", name)
		out = out[:0]
		for _, s := range v {
			out = append(out, SRV{Target: s.Target, Port: s.Port, Priority: s.Priority, Weight: s.Weight})
		}
		if e == nil && len(out) == 0 {
			e = &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
		}
		return e
	})
	return
}

// LookupCNAME implements Resolver.
func (n NetResolver) LookupCNAME(ctx context.Context, name string) (out string, err error) {
	err = n.each(ctx, func(r *net.Resolver) error {
		c, cancel := n.withTimeout(ctx)
		defer cancel()
		v, e := r.LookupCNAME(c, name)
		out = v
		return e
	})
	return
}

// HTTPFetcher is the production Fetcher (5 s timeout, 64 KiB cap, no redirects
// off-host are followed beyond 3).
func HTTPFetcher(ctx context.Context, url string) (string, int, error) {
	c := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return string(b), resp.StatusCode, nil
}

// CachedResolver caches answers (including failures) for TTL (default 60 s).
type CachedResolver struct {
	Next Resolver
	TTL  time.Duration
	Now  func() time.Time
	mu   sync.Mutex
	m    map[string]cached
}
type cached struct {
	at  time.Time
	val any
	err error
}

func (c *CachedResolver) get(key string, fn func() (any, error)) (any, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	ttl := c.TTL
	if ttl == 0 {
		ttl = 60 * time.Second
	}
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]cached{}
	}
	if e, ok := c.m[key]; ok && now().Sub(e.at) < ttl {
		c.mu.Unlock()
		return e.val, e.err
	}
	c.mu.Unlock()
	v, err := fn()
	c.mu.Lock()
	c.m[key] = cached{at: now(), val: v, err: err}
	c.mu.Unlock()
	return v, err
}

// Flush drops all cached answers (used by the explicit re-check button).
func (c *CachedResolver) Flush() { c.mu.Lock(); c.m = nil; c.mu.Unlock() }

// LookupTXT implements Resolver.
func (c *CachedResolver) LookupTXT(ctx context.Context, n string) ([]string, error) {
	v, err := c.get("txt|"+n, func() (any, error) { return c.Next.LookupTXT(ctx, n) })
	s, _ := v.([]string)
	return s, err
}

// LookupMX implements Resolver.
func (c *CachedResolver) LookupMX(ctx context.Context, n string) ([]MX, error) {
	v, err := c.get("mx|"+n, func() (any, error) { return c.Next.LookupMX(ctx, n) })
	s, _ := v.([]MX)
	return s, err
}

// LookupSRV implements Resolver.
func (c *CachedResolver) LookupSRV(ctx context.Context, n string) ([]SRV, error) {
	v, err := c.get("srv|"+n, func() (any, error) { return c.Next.LookupSRV(ctx, n) })
	s, _ := v.([]SRV)
	return s, err
}

// LookupCNAME implements Resolver.
func (c *CachedResolver) LookupCNAME(ctx context.Context, n string) (string, error) {
	v, err := c.get("cname|"+n, func() (any, error) { return c.Next.LookupCNAME(ctx, n) })
	s, _ := v.(string)
	return s, err
}

func norm(s string) string { return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), ".")) }

func joinTXT(parts []string) string { return strings.Join(parts, "") }

// tagMap parses "k=v; k2=v2" into a map (keys lower-cased, values trimmed).
func tagMap(s string) map[string]string {
	m := map[string]string{}
	for _, p := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if ok {
			m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	return m
}

// SPFLookupCount counts the DNS-querying mechanisms of one SPF record
// (include, a, mx, ptr, exists, redirect). Nested includes are not expanded.
func SPFLookupCount(spf string) int {
	n := 0
	for _, t := range strings.Fields(spf)[1:] {
		t = strings.TrimLeft(t, "+-~?")
		l := strings.ToLower(t)
		switch {
		case strings.HasPrefix(l, "include:"), strings.HasPrefix(l, "exists:"), strings.HasPrefix(l, "redirect="),
			l == "a", l == "mx", l == "ptr", strings.HasPrefix(l, "a:"), strings.HasPrefix(l, "a/"),
			strings.HasPrefix(l, "mx:"), strings.HasPrefix(l, "mx/"), strings.HasPrefix(l, "ptr:"):
			n++
		}
	}
	return n
}

func res(r Record, status, msg string, found []string) Result {
	return Result{Record: r, Status: status, Expected: r.Value, Found: found, Message: msg}
}

// miss is the status for an absent record: fail when required, warn otherwise.
func miss(r Record, found []string, msg string) Result {
	if r.Required {
		return res(r, Fail, msg, found)
	}
	return res(r, Warn, msg, found)
}

// Check runs the live checks for every planned record. fetch may be nil, in
// which case MTA-STS/autoconfig HTTPS fetch checks are skipped.
func Check(ctx context.Context, rs Resolver, fetch Fetcher, in PlanInput) []Result {
	in.defaults()
	plan := Plan(in)
	out := make([]Result, 0, len(plan)+2)
	for _, r := range plan {
		switch r.Kind {
		case "mx":
			out = append(out, checkMX(ctx, rs, r))
		case "spf":
			out = append(out, checkSPF(ctx, rs, r))
		case "dkim":
			out = append(out, checkExactTXT(ctx, rs, r, "DKIM key"))
		case "dmarc":
			out = append(out, checkDMARC(ctx, rs, r, in))
		case "mta-sts-txt":
			out = append(out, checkPrefixTXT(ctx, rs, r, "v=STSv1"))
		case "tls-rpt":
			out = append(out, checkPrefixTXT(ctx, rs, r, "v=TLSRPTv1"))
		case "mta-sts-host", "autoconfig", "autodiscover":
			out = append(out, checkCNAME(ctx, rs, r))
		case "srv":
			out = append(out, checkSRV(ctx, rs, r))
		default:
			out = append(out, res(r, Warn, "no check for this record", nil))
		}
	}
	if fetch != nil {
		out = append(out, checkMTASTSPolicy(ctx, fetch, in))
	}
	return out
}

func checkMX(ctx context.Context, rs Resolver, r Record) Result {
	mx, err := rs.LookupMX(ctx, r.Name)
	if err != nil || len(mx) == 0 {
		return miss(r, nil, "No MX record found. Mail to this domain cannot be delivered.")
	}
	want := norm(strings.Fields(r.Value)[1])
	var found []string
	for _, m := range mx {
		found = append(found, strings.TrimSpace(strings.Join([]string{itoa(int(m.Pref)), norm(m.Host) + "."}, " ")))
	}
	for _, m := range mx {
		if norm(m.Host) == want {
			if len(mx) > 1 {
				return res(r, Warn, "Expected host is present but other MX records exist; they may receive mail first.", found)
			}
			return res(r, Pass, "MX points at the mail host.", found)
		}
	}
	return res(r, Fail, "MX points somewhere else than "+want+".", found)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

func spfRecords(txt []string) []string {
	var out []string
	for _, t := range txt {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=spf1") {
			out = append(out, strings.TrimSpace(t))
		}
	}
	return out
}

func checkSPF(ctx context.Context, rs Resolver, r Record) Result {
	txt, err := rs.LookupTXT(ctx, r.Name)
	spf := spfRecords(txt)
	if err != nil || len(spf) == 0 {
		return miss(r, nil, "No SPF record found.")
	}
	if len(spf) > 1 {
		return res(r, Fail, "More than one SPF record: receivers treat this as a permanent error. Merge them.", spf)
	}
	have := strings.Fields(strings.ToLower(spf[0]))
	var missing []string
	for _, t := range strings.Fields(strings.ToLower(r.Value)) {
		if t == "v=spf1" || t == "-all" {
			continue
		}
		ok := false
		for _, h := range have {
			if h == t {
				ok = true
			}
		}
		if !ok {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return res(r, Fail, "SPF is missing: "+strings.Join(missing, ", ")+".", spf)
	}
	if n := SPFLookupCount(spf[0]); n > 10 {
		return res(r, Warn, "SPF needs more than 10 DNS lookups ("+itoa(n)+"); receivers will reject it.", spf)
	}
	last := have[len(have)-1]
	if last != "-all" && last != "~all" {
		return res(r, Warn, "SPF does not end in -all or ~all.", spf)
	}
	return res(r, Pass, "SPF includes the required mechanisms.", spf)
}

func checkExactTXT(ctx context.Context, rs Resolver, r Record, what string) Result {
	txt, err := rs.LookupTXT(ctx, r.Name)
	if err != nil || len(txt) == 0 {
		return miss(r, nil, what+" is not published yet.")
	}
	want := tagMap(r.Value)
	for _, t := range txt {
		got := tagMap(t)
		if got["p"] != "" && strings.ReplaceAll(got["p"], " ", "") == want["p"] {
			return res(r, Pass, what+" matches.", txt)
		}
	}
	return res(r, Fail, what+" is published but differs from the key Adrient holds.", txt)
}

func checkDMARC(ctx context.Context, rs Resolver, r Record, in PlanInput) Result {
	txt, err := rs.LookupTXT(ctx, r.Name)
	var dm []string
	for _, t := range txt {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
			dm = append(dm, t)
		}
	}
	if err != nil || len(dm) == 0 {
		return miss(r, nil, "No DMARC record found.")
	}
	if len(dm) > 1 {
		return res(r, Fail, "More than one DMARC record.", dm)
	}
	got := tagMap(dm[0])
	if got["p"] == "" {
		return res(r, Fail, "DMARC record has no p= policy.", dm)
	}
	if got["p"] != in.DMARCPolicy {
		return res(r, Warn, "DMARC policy is p="+got["p"]+", the plan says p="+in.DMARCPolicy+".", dm)
	}
	if got["rua"] == "" {
		return res(r, Warn, "DMARC has no rua= address, so you will not receive aggregate reports.", dm)
	}
	return res(r, Pass, "DMARC is published (p="+got["p"]+").", dm)
}

func checkPrefixTXT(ctx context.Context, rs Resolver, r Record, prefix string) Result {
	txt, err := rs.LookupTXT(ctx, r.Name)
	if err != nil || len(txt) == 0 {
		return miss(r, nil, "Not published.")
	}
	for _, t := range txt {
		if strings.HasPrefix(strings.TrimSpace(t), prefix) {
			if prefix == "v=TLSRPTv1" && tagMap(t)["rua"] != tagMap(r.Value)["rua"] {
				return res(r, Warn, "TLS-RPT report address differs from the plan.", txt)
			}
			if prefix == "v=STSv1" && tagMap(t)["id"] != tagMap(r.Value)["id"] {
				return res(r, Warn, "Published with a different policy id than the plan.", txt)
			}
			return res(r, Pass, "Published.", txt)
		}
	}
	return res(r, Fail, "Published value is not a "+prefix+" record.", txt)
}

func checkCNAME(ctx context.Context, rs Resolver, r Record) Result {
	cn, err := rs.LookupCNAME(ctx, r.Name)
	if err != nil || cn == "" {
		return miss(r, nil, "No CNAME found for "+r.Name+".")
	}
	if norm(cn) == norm(r.Value) {
		return res(r, Pass, "CNAME points at the Adrient host.", []string{norm(cn) + "."})
	}
	if norm(cn) == norm(r.Name) {
		return miss(r, nil, "No CNAME found for "+r.Name+".")
	}
	return res(r, Warn, "CNAME points at "+norm(cn)+", expected "+norm(r.Value)+".", []string{norm(cn) + "."})
}

func checkSRV(ctx context.Context, rs Resolver, r Record) Result {
	srv, err := rs.LookupSRV(ctx, r.Name)
	if err != nil || len(srv) == 0 {
		return miss(r, nil, "No SRV record found.")
	}
	f := strings.Fields(r.Value)
	wantPort, wantTarget := f[2], norm(f[3])
	var found []string
	for _, s := range srv {
		found = append(found, itoa(int(s.Priority))+" "+itoa(int(s.Weight))+" "+itoa(int(s.Port))+" "+norm(s.Target)+".")
	}
	sort.Strings(found)
	for _, s := range srv {
		if itoa(int(s.Port)) == wantPort && norm(s.Target) == wantTarget {
			return res(r, Pass, "SRV matches.", found)
		}
	}
	return res(r, Warn, "SRV exists but differs from the plan.", found)
}

func checkMTASTSPolicy(ctx context.Context, fetch Fetcher, in PlanInput) Result {
	url := "https://mta-sts." + in.Domain + "/.well-known/mta-sts.txt"
	r := Record{Kind: "mta-sts-policy", Name: "mta-sts." + in.Domain, Type: "HTTPS", Value: url, Required: false}
	body, status, err := fetch(ctx, url)
	if err != nil {
		return res(r, Warn, "Policy file not reachable over HTTPS: "+err.Error(), nil)
	}
	if status != 200 {
		return res(r, Warn, "Policy file returned HTTP "+itoa(status)+".", []string{"HTTP " + itoa(status)})
	}
	found := []string{strings.TrimSpace(strings.ReplaceAll(body, "\r\n", " | "))}
	hasMX, hasMode := false, false
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		k, v, _ := strings.Cut(l, ":")
		switch strings.TrimSpace(k) {
		case "mx":
			if norm(v) == norm(in.MailHost) {
				hasMX = true
			}
		case "mode":
			hasMode = norm(v) == in.MTASTSMode
		}
	}
	if !hasMX || !hasMode {
		return res(r, Warn, "Policy file is reachable but mode or mx differs from the plan.", found)
	}
	return res(r, Pass, "Policy file is served and matches.", found)
}

// Summary counts results by status.
func Summary(rs []Result) (pass, warn, fail int) {
	for _, r := range rs {
		switch r.Status {
		case Pass:
			pass++
		case Warn:
			warn++
		default:
			fail++
		}
	}
	return
}

// Clean reports whether the mail-critical records (mx, spf, dkim, dmarc) all pass.
func Clean(rs []Result) bool {
	seen := 0
	for _, r := range rs {
		switch r.Kind {
		case "mx", "spf", "dmarc":
			seen++
			if r.Status != Pass {
				return false
			}
		case "dkim":
			if r.Status == Fail {
				return false
			}
		}
	}
	return seen == 3
}

// CleanDays returns how many whole days the latest unbroken streak of clean
// runs spans. runs are newest first.
func CleanDays(runs []Run, now time.Time) int {
	var since time.Time
	for _, r := range runs {
		if !r.Clean {
			break
		}
		since = r.At
	}
	if since.IsZero() {
		return 0
	}
	return int(now.Sub(since) / (24 * time.Hour))
}

// Run is one stored check run.
type Run struct {
	At    time.Time
	Clean bool
}

// RampHint returns advice for the DMARC policy ramp, or "" when none applies.
func RampHint(policy string, cleanDays int) string {
	switch {
	case policy == "none" && cleanDays >= 14:
		return "SPF, DKIM and DMARC have passed for " + itoa(cleanDays) + " days. You can move DMARC to p=quarantine."
	case policy == "none":
		return "Keep DMARC at p=none until SPF, DKIM and DMARC have passed for 14 clean days (" + itoa(cleanDays) + " so far)."
	case policy == "quarantine" && cleanDays >= 14:
		return "Clean for " + itoa(cleanDays) + " days at p=quarantine. You can move DMARC to p=reject."
	}
	return ""
}
