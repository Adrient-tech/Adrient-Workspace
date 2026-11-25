package dnsmail

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeDNS is an in-memory Resolver.
type fakeDNS struct {
	txt   map[string][]string
	mx    map[string][]MX
	srv   map[string][]SRV
	cname map[string]string
}

var errNX = errors.New("nxdomain")

func (f fakeDNS) LookupTXT(_ context.Context, n string) ([]string, error) {
	if v, ok := f.txt[n]; ok {
		return v, nil
	}
	return nil, errNX
}
func (f fakeDNS) LookupMX(_ context.Context, n string) ([]MX, error) {
	if v, ok := f.mx[n]; ok {
		return v, nil
	}
	return nil, errNX
}
func (f fakeDNS) LookupSRV(_ context.Context, n string) ([]SRV, error) {
	if v, ok := f.srv[n]; ok {
		return v, nil
	}
	return nil, errNX
}
func (f fakeDNS) LookupCNAME(_ context.Context, n string) (string, error) {
	if v, ok := f.cname[n]; ok {
		return v, nil
	}
	return "", errNX
}

func testInput() PlanInput {
	return PlanInput{Domain: "Example.com", SPFIPs: []string{"203.0.113.5"},
		DKIM: []DKIMEntry{{Selector: "adr202611", Algo: AlgoRSA, PublicKey: "QUJD", State: StateActive}}}
}

func find(rs []Result, kind string) Result {
	for _, r := range rs {
		if r.Kind == kind {
			return r
		}
	}
	return Result{}
}

func TestRecordStrings(t *testing.T) {
	in := testInput()
	if got := SPFValue(in); got != "v=spf1 mx ip4:203.0.113.5 -all" {
		t.Fatalf("spf %q", got)
	}
	in.SPFInclude = "_spf.adrient.com"
	in.SPFIPs = []string{"2001:db8::1", "203.0.113.5"}
	if got := SPFValue(in); got != "v=spf1 mx ip6:2001:db8::1 ip4:203.0.113.5 include:_spf.adrient.com -all" {
		t.Fatalf("spf2 %q", got)
	}
	if got := DKIMValue(DKIMEntry{Algo: AlgoRSA, PublicKey: "AAA"}); got != "v=DKIM1; k=rsa; p=AAA" {
		t.Fatalf("dkim %q", got)
	}
	if got := DKIMValue(DKIMEntry{Algo: AlgoEd25519, PublicKey: "AAA"}); got != "v=DKIM1; k=ed25519; p=AAA" {
		t.Fatalf("dkim ed %q", got)
	}
	if got := DMARCValue("none", "dmarc@example.com"); got != "v=DMARC1; p=none; rua=mailto:dmarc@example.com" {
		t.Fatalf("dmarc %q", got)
	}
	if got := MTASTSPolicy("testing", "mail.example.com"); got != "version: STSv1\r\nmode: testing\r\nmx: mail.example.com\r\nmax_age: 86400\r\n" {
		t.Fatalf("policy %q", got)
	}
}

func TestPlanRecords(t *testing.T) {
	recs := Plan(testInput())
	names := map[string]string{}
	for _, r := range recs {
		names[r.Kind+"|"+r.Name] = r.Value
	}
	want := map[string]string{
		"mx|example.com":                        "10 mail.example.com.",
		"dkim|adr202611._domainkey.example.com": "v=DKIM1; k=rsa; p=QUJD",
		"dmarc|_dmarc.example.com":              "v=DMARC1; p=none; rua=mailto:dmarc@example.com",
		"mta-sts-txt|_mta-sts.example.com":      "v=STSv1; id=1",
		"tls-rpt|_smtp._tls.example.com":        "v=TLSRPTv1; rua=mailto:tlsrpt@example.com",
		"autoconfig|autoconfig.example.com":     "workspace.example.com.",
		"srv|_imaps._tcp.example.com":           "0 1 993 mail.example.com.",
		"srv|_submission._tcp.example.com":      "0 1 587 mail.example.com.",
		"srv|_submissions._tcp.example.com":     "0 1 465 mail.example.com.",
		"srv|_caldavs._tcp.example.com":         "0 1 443 workspace.example.com.",
		"srv|_carddavs._tcp.example.com":        "0 1 443 workspace.example.com.",
		"autodiscover|autodiscover.example.com": "workspace.example.com.",
		"mta-sts-host|mta-sts.example.com":      "workspace.example.com.",
	}
	for k, v := range want {
		if names[k] != v {
			t.Errorf("%s = %q, want %q", k, names[k], v)
		}
	}
}

func goodDNS() fakeDNS {
	in := testInput()
	d := fakeDNS{txt: map[string][]string{}, mx: map[string][]MX{}, srv: map[string][]SRV{}, cname: map[string]string{}}
	for _, r := range Plan(in) {
		switch r.Type {
		case "TXT":
			d.txt[r.Name] = []string{r.Value}
		case "CNAME":
			d.cname[r.Name] = r.Value
		}
	}
	d.mx["example.com"] = []MX{{Host: "mail.example.com.", Pref: 10}}
	for _, p := range []struct {
		n    string
		port uint16
		tgt  string
	}{{"_imaps._tcp", 993, "mail"}, {"_submission._tcp", 587, "mail"}, {"_submissions._tcp", 465, "mail"},
		{"_caldavs._tcp", 443, "workspace"}, {"_carddavs._tcp", 443, "workspace"}} {
		d.srv[p.n+".example.com"] = []SRV{{Target: p.tgt + ".example.com.", Port: p.port, Weight: 1}}
	}
	d.txt["example.com"] = []string{"v=spf1 mx ip4:203.0.113.5 -all"}
	return d
}

func goodFetch(_ context.Context, url string) (string, int, error) {
	if strings.HasPrefix(url, "https://mta-sts.example.com/") {
		return MTASTSPolicy("testing", "mail.example.com"), 200, nil
	}
	return "", 404, nil
}

func TestCheckAllPass(t *testing.T) {
	rs := Check(context.Background(), goodDNS(), goodFetch, testInput())
	for _, r := range rs {
		if r.Status != Pass {
			t.Errorf("%s %s: %s %s found=%v", r.Kind, r.Name, r.Status, r.Message, r.Found)
		}
	}
	if !Clean(rs) {
		t.Fatal("expected clean")
	}
	p, w, f := Summary(rs)
	if w != 0 || f != 0 || p != len(rs) {
		t.Fatalf("summary %d %d %d", p, w, f)
	}
}

func TestCheckMissingAndMismatch(t *testing.T) {
	rs := Check(context.Background(), fakeDNS{}, nil, testInput())
	for _, k := range []string{"mx", "spf", "dkim", "dmarc"} {
		if r := find(rs, k); r.Status != Fail {
			t.Errorf("%s missing should fail, got %s", k, r.Status)
		}
	}
	for _, k := range []string{"srv", "autoconfig", "tls-rpt", "mta-sts-txt"} {
		if r := find(rs, k); r.Status != Warn {
			t.Errorf("%s missing (optional) should warn, got %s", k, r.Status)
		}
	}
	if Clean(rs) {
		t.Fatal("must not be clean")
	}

	d := goodDNS()
	d.mx["example.com"] = []MX{{Host: "mx.other.net.", Pref: 10}}
	d.txt["example.com"] = []string{"v=spf1 include:a.com -all", "v=spf1 mx -all"}
	d.txt["adr202611._domainkey.example.com"] = []string{"v=DKIM1; k=rsa; p=WRONG"}
	d.txt["_dmarc.example.com"] = []string{"v=DMARC1; p=reject; rua=mailto:a@example.com"}
	rs = Check(context.Background(), d, goodFetch, testInput())
	if r := find(rs, "mx"); r.Status != Fail || len(r.Found) != 1 || r.Found[0] != "10 mx.other.net." {
		t.Errorf("mx mismatch: %+v", r)
	}
	if r := find(rs, "spf"); r.Status != Fail || !strings.Contains(r.Message, "More than one") {
		t.Errorf("double spf: %+v", r)
	}
	if r := find(rs, "dkim"); r.Status != Fail {
		t.Errorf("dkim mismatch: %+v", r)
	}
	if r := find(rs, "dmarc"); r.Status != Warn || !strings.Contains(r.Message, "p=reject") {
		t.Errorf("dmarc policy differs: %+v", r)
	}
}

func TestSPFLookupsAndMissingMechanism(t *testing.T) {
	spf := "v=spf1 mx include:a.com include:b.com include:c.com include:d.com include:e.com include:f.com include:g.com include:h.com include:i.com include:j.com ip4:203.0.113.5 -all"
	if n := SPFLookupCount(spf); n != 11 {
		t.Fatalf("count %d", n)
	}
	d := goodDNS()
	d.txt["example.com"] = []string{spf}
	if r := find(Check(context.Background(), d, nil, testInput()), "spf"); r.Status != Warn || !strings.Contains(r.Message, "10 DNS lookups") {
		t.Errorf("lookup warn: %+v", r)
	}
	d.txt["example.com"] = []string{"v=spf1 mx -all"}
	if r := find(Check(context.Background(), d, nil, testInput()), "spf"); r.Status != Fail || !strings.Contains(r.Message, "ip4:203.0.113.5") {
		t.Errorf("missing ip: %+v", r)
	}
}

func TestMTASTSFetch(t *testing.T) {
	in := testInput()
	bad := func(context.Context, string) (string, int, error) { return "", 404, nil }
	if r := find(Check(context.Background(), goodDNS(), bad, in), "mta-sts-policy"); r.Status != Warn {
		t.Errorf("404 should warn: %+v", r)
	}
	wrong := func(context.Context, string) (string, int, error) {
		return MTASTSPolicy("enforce", "mail.example.com"), 200, nil
	}
	if r := find(Check(context.Background(), goodDNS(), wrong, in), "mta-sts-policy"); r.Status != Warn {
		t.Errorf("mode differs should warn: %+v", r)
	}
}

func TestCachedResolver(t *testing.T) {
	n := 0
	now := time.Unix(1000, 0)
	c := &CachedResolver{Next: countingDNS{&n}, Now: func() time.Time { return now }}
	for i := 0; i < 3; i++ {
		_, _ = c.LookupTXT(context.Background(), "a")
	}
	if n != 1 {
		t.Fatalf("cached lookups %d", n)
	}
	now = now.Add(61 * time.Second)
	_, _ = c.LookupTXT(context.Background(), "a")
	if n != 2 {
		t.Fatalf("expired lookups %d", n)
	}
	c.Flush()
	_, _ = c.LookupTXT(context.Background(), "a")
	if n != 3 {
		t.Fatalf("flushed lookups %d", n)
	}
}

type countingDNS struct{ n *int }

func (c countingDNS) LookupTXT(context.Context, string) ([]string, error) {
	*c.n++
	return []string{"x"}, nil
}
func (countingDNS) LookupMX(context.Context, string) ([]MX, error)      { return nil, errNX }
func (countingDNS) LookupSRV(context.Context, string) ([]SRV, error)    { return nil, errNX }
func (countingDNS) LookupCNAME(context.Context, string) (string, error) { return "", errNX }

func TestDKIMKeyRoundTrip(t *testing.T) {
	pub, privPEM, err := GenerateKey(AlgoRSA)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt("secret", privPEM, []byte("dom|sel"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "PRIVATE") {
		t.Fatal("ciphertext leaks plaintext")
	}
	if _, err := Decrypt("secret", enc, []byte("dom|other")); err == nil {
		t.Fatal("aad must bind the row")
	}
	if _, err := Decrypt("wrong", enc, []byte("dom|sel")); err == nil {
		t.Fatal("wrong secret must fail")
	}
	raw, err := Decrypt("secret", enc, []byte("dom|sel"))
	if err != nil {
		t.Fatal(err)
	}
	blk, _ := pem.Decode(raw)
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	msg := sha256.Sum256([]byte("DKIM-Signature canonical header"))
	sig, err := rsa.SignPKCS1v15(nil, k.(*rsa.PrivateKey), crypto.SHA256, msg[:])
	if err != nil {
		t.Fatal(err)
	}
	der, _ := base64.StdEncoding.DecodeString(pub)
	pk, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPKCS1v15(pk.(*rsa.PublicKey), crypto.SHA256, msg[:], sig); err != nil {
		t.Fatalf("signature does not verify with the published key: %v", err)
	}
	if _, err := Encrypt("", privPEM, nil); !errors.Is(err, ErrNoServerKey) {
		t.Fatal("empty secret must be refused")
	}
	if p, _, err := GenerateKey(AlgoEd25519); err != nil || len(p) != 44 {
		t.Fatalf("ed25519 %q %v", p, err)
	}
}

func TestSelectorAndRotation(t *testing.T) {
	ts := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	if s := Selector(ts, func(string) bool { return false }); s != "adr202611" {
		t.Fatal(s)
	}
	if s := Selector(ts, func(x string) bool { return x == "adr202611" }); s != "adr202611b" {
		t.Fatal(s)
	}

	created := ts
	next := Key{Selector: "adr202611", State: StateNext, CreatedAt: created}
	// First key: nothing signs yet, so no wait; DNS visibility still required.
	if _, err := (Rotation{Keys: []Key{next}, Now: ts}).CanActivate(false); err == nil {
		t.Fatal("must require DNS")
	}
	if _, err := (Rotation{Keys: []Key{next}, Now: ts}).CanActivate(true); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	// Rotation: an active key exists, 7 day wait with a fake clock.
	act := Key{Selector: "old", State: StateActive, CreatedAt: created.AddDate(0, -3, 0)}
	r := Rotation{Keys: []Key{next, act}, Now: ts.Add(3 * 24 * time.Hour)}
	if _, err := r.CanActivate(true); err == nil || !strings.Contains(err.Error(), "4 day(s) left") {
		t.Fatalf("expected wait, got %v", err)
	}
	r.Now = ts.Add(7 * 24 * time.Hour)
	if _, err := r.CanActivate(true); err != nil {
		t.Fatalf("after wait: %v", err)
	}
	rot := ts
	ret := Key{Selector: "old", State: StateRetired, RotatedAt: &rot}
	if n := len((Rotation{Keys: []Key{ret}, Now: ts.Add(6 * 24 * time.Hour)}).Retirable()); n != 0 {
		t.Fatal("retired too early")
	}
	if n := len((Rotation{Keys: []Key{ret}, Now: ts.Add(7 * 24 * time.Hour)}).Retirable()); n != 1 {
		t.Fatal("should be retirable")
	}
}

func TestCleanDaysAndRamp(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	d := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	runs := []Run{{d(0), true}, {d(5), true}, {d(15), true}, {d(16), false}, {d(20), true}}
	if n := CleanDays(runs, now); n != 15 {
		t.Fatalf("clean days %d", n)
	}
	if CleanDays([]Run{{d(0), false}, {d(2), true}}, now) != 0 {
		t.Fatal("latest failure resets")
	}
	if h := RampHint("none", 15); !strings.Contains(h, "p=quarantine") {
		t.Fatal(h)
	}
	if h := RampHint("none", 3); !strings.Contains(h, "14 clean days") || !strings.Contains(h, "3 so far") {
		t.Fatal(h)
	}
	if RampHint("reject", 99) != "" {
		t.Fatal("reject needs no hint")
	}
}

func TestClientConfigXML(t *testing.T) {
	x := AutoconfigXML("example.com", "mail.example.com")
	for _, w := range []string{"<hostname>mail.example.com</hostname>", "<port>993</port>", "<port>587</port>", "%EMAILADDRESS%"} {
		if !strings.Contains(x, w) {
			t.Errorf("autoconfig missing %s", w)
		}
	}
	a := AutodiscoverXML("a@example.com", "example.com", "mail.example.com")
	if !strings.Contains(a, "<Server>mail.example.com</Server>") || !strings.Contains(a, "a@example.com") {
		t.Error("autodiscover content")
	}
	if !strings.Contains(MobileConfig("a@example.com", "example.com", "mail.example.com"), "IncomingMailServerHostName") {
		t.Error("mobileconfig")
	}
	if strings.Contains(AutoconfigXML("x.com", "<evil>"), "<evil>") {
		t.Error("host must be escaped")
	}
}

func TestSettingsValidate(t *testing.T) {
	ok := Settings{DMARCPolicy: "none", MTASTSMode: "testing", DMARCRua: "dmarc@example.com", MailHost: "Mail.Example.com.", SPFIPs: []string{"203.0.113.5"}}
	if err := ok.Validate(); err != nil || ok.MailHost != "mail.example.com" {
		t.Fatalf("%v %q", err, ok.MailHost)
	}
	for _, bad := range []Settings{
		{DMARCPolicy: "x", MTASTSMode: "testing"},
		{DMARCPolicy: "none", MTASTSMode: "x"},
		{DMARCPolicy: "none", MTASTSMode: "none", DMARCRua: "a@b.com;p=none"},
		{DMARCPolicy: "none", MTASTSMode: "none", MailHost: "bad host"},
		{DMARCPolicy: "none", MTASTSMode: "none", SPFIPs: []string{"1.2.3.4 include:evil"}},
	} {
		if bad.Validate() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
