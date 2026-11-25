// Package dnsmail generates the mail DNS record plan for a verified org domain
// (MX, SPF, DKIM, DMARC, MTA-STS, TLS-RPT, autoconfig/autodiscover, SRV), checks
// it live against public resolvers, manages DKIM keys and serves the public
// client-configuration endpoints (DNS/email plan section F).
package dnsmail

import (
	"fmt"
	"sort"
	"strings"
)

// Record is one DNS record the customer creates at their DNS provider.
type Record struct {
	Kind     string `json:"kind"` // mx spf dkim dmarc mta-sts-txt mta-sts-host tls-rpt autoconfig autodiscover srv
	Name     string `json:"name"` // fully qualified, no trailing dot
	Type     string `json:"type"` // MX TXT CNAME SRV
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Required bool   `json:"required"`
	Note     string `json:"note,omitempty"`
}

// DKIMEntry is a published (or to-be-published) DKIM key.
type DKIMEntry struct {
	Selector  string `json:"selector"`
	Algo      string `json:"algo"`
	PublicKey string `json:"public_key"` // base64
	State     string `json:"state"`
}

// PlanInput is everything the generator needs. Zero values get defaults.
type PlanInput struct {
	Domain       string
	MailHost     string   // MX / IMAP / SMTP host, default mail.<domain>
	WebHost      string   // host serving autoconfig, MTA-STS policy and DAV (the Adrient host)
	SPFIPs       []string // optional ip4:/ip6: mechanisms for the MTA
	SPFInclude   string   // optional include: domain
	DKIM         []DKIMEntry
	DMARCPolicy  string // none|quarantine|reject, default none
	DMARCRua     string // default dmarc@<domain>
	MTASTSMode   string // none|testing|enforce, default testing
	MTASTSID     string // policy id, default "1"
	TLSRPTRua    string // default tlsrpt@<domain>
	TTL          int
	IncludeMTSTS bool
}

func (in *PlanInput) defaults() {
	in.Domain = strings.ToLower(strings.TrimSuffix(in.Domain, "."))
	if in.MailHost == "" {
		in.MailHost = "mail." + in.Domain
	}
	in.MailHost = strings.TrimSuffix(strings.ToLower(in.MailHost), ".")
	if in.WebHost == "" {
		in.WebHost = "workspace." + in.Domain
	}
	in.WebHost = strings.TrimSuffix(strings.ToLower(in.WebHost), ".")
	if in.DMARCPolicy == "" {
		in.DMARCPolicy = "none"
	}
	if in.DMARCRua == "" {
		in.DMARCRua = "dmarc@" + in.Domain
	}
	if in.MTASTSMode == "" {
		in.MTASTSMode = "testing"
	}
	if in.MTASTSID == "" {
		in.MTASTSID = "1"
	}
	if in.TLSRPTRua == "" {
		in.TLSRPTRua = "tlsrpt@" + in.Domain
	}
	if in.TTL == 0 {
		in.TTL = 3600
	}
}

// SPFValue builds the SPF record text.
func SPFValue(in PlanInput) string {
	in.defaults()
	parts := []string{"v=spf1", "mx"}
	ips := append([]string(nil), in.SPFIPs...)
	sort.Strings(ips)
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if strings.Contains(ip, ":") {
			parts = append(parts, "ip6:"+ip)
		} else {
			parts = append(parts, "ip4:"+ip)
		}
	}
	if in.SPFInclude != "" {
		parts = append(parts, "include:"+in.SPFInclude)
	}
	parts = append(parts, "-all")
	return strings.Join(parts, " ")
}

// DKIMValue builds the DKIM TXT value for a key.
func DKIMValue(k DKIMEntry) string {
	kt := "rsa"
	if k.Algo == "ed25519" {
		kt = "ed25519"
	}
	return "v=DKIM1; k=" + kt + "; p=" + k.PublicKey
}

// DMARCValue builds the DMARC TXT value.
func DMARCValue(policy, rua string) string {
	return "v=DMARC1; p=" + policy + "; rua=mailto:" + rua
}

// MTASTSTXT is the _mta-sts TXT value.
func MTASTSTXT(id string) string { return "v=STSv1; id=" + id }

// MTASTSPolicy is the body served at mta-sts.<domain>/.well-known/mta-sts.txt.
func MTASTSPolicy(mode, mailHost string) string {
	if mode == "" || mode == "none" {
		mode = "none"
	}
	return "version: STSv1\r\nmode: " + mode + "\r\nmx: " + mailHost + "\r\nmax_age: 86400\r\n"
}

// TLSRPTValue is the _smtp._tls TXT value.
func TLSRPTValue(rua string) string { return "v=TLSRPTv1; rua=mailto:" + rua }

// Plan returns the records for a domain, in display order.
func Plan(in PlanInput) []Record {
	in.defaults()
	d, ttl := in.Domain, in.TTL
	rec := []Record{
		{Kind: "mx", Name: d, Type: "MX", Value: "10 " + in.MailHost + ".", TTL: ttl, Required: true},
		{Kind: "spf", Name: d, Type: "TXT", Value: SPFValue(in), TTL: ttl, Required: true,
			Note: "Exactly one SPF record per domain; merge if you already have one."},
	}
	keys := append([]DKIMEntry(nil), in.DKIM...)
	sort.SliceStable(keys, func(i, j int) bool { return keys[i].State < keys[j].State })
	for _, k := range keys {
		if k.State == "retired" {
			continue
		}
		rec = append(rec, Record{Kind: "dkim", Name: k.Selector + "._domainkey." + d, Type: "TXT", Value: DKIMValue(k),
			TTL: ttl, Required: k.State == "active" || k.State == "next",
			Note: "Key state: " + k.State})
	}
	rec = append(rec,
		Record{Kind: "dmarc", Name: "_dmarc." + d, Type: "TXT", Value: DMARCValue(in.DMARCPolicy, in.DMARCRua), TTL: ttl, Required: true,
			Note: "Starts at p=none; the wizard suggests quarantine after 14 clean days."},
		Record{Kind: "mta-sts-txt", Name: "_mta-sts." + d, Type: "TXT", Value: MTASTSTXT(in.MTASTSID), TTL: ttl, Required: false},
		Record{Kind: "mta-sts-host", Name: "mta-sts." + d, Type: "CNAME", Value: in.WebHost + ".", TTL: ttl, Required: false,
			Note: "Adrient serves https://mta-sts." + d + "/.well-known/mta-sts.txt on this host."},
		Record{Kind: "tls-rpt", Name: "_smtp._tls." + d, Type: "TXT", Value: TLSRPTValue(in.TLSRPTRua), TTL: ttl, Required: false},
		Record{Kind: "autoconfig", Name: "autoconfig." + d, Type: "CNAME", Value: in.WebHost + ".", TTL: ttl, Required: false,
			Note: "Thunderbird and other clients find IMAP/SMTP settings automatically."},
		Record{Kind: "autodiscover", Name: "autodiscover." + d, Type: "CNAME", Value: in.WebHost + ".", TTL: ttl, Required: false,
			Note: "Outlook and Apple Mail discovery."},
	)
	srv := func(svc string, port int, target string) Record {
		return Record{Kind: "srv", Name: svc + "." + d, Type: "SRV", Value: fmt.Sprintf("0 1 %d %s.", port, target), TTL: ttl, Required: false}
	}
	rec = append(rec,
		srv("_imaps._tcp", 993, in.MailHost),
		srv("_submission._tcp", 587, in.MailHost),
		srv("_submissions._tcp", 465, in.MailHost),
		srv("_caldavs._tcp", 443, in.WebHost),
		srv("_carddavs._tcp", 443, in.WebHost),
	)
	return rec
}
