package dnsmail

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
)

// DevResolver answers from a JSON file before falling back to Next. Local
// development and tests only; the server enables it solely when both
// ADRIENT_DEV_MODE=1 and ADRIENT_DEV_DNS_OVERRIDE (file path) are set.
//
//	{"txt":{"_dmarc.x.com":["v=DMARC1; p=none"]}, "mx":{"x.com":["10 mail.x.com."]},
//	 "srv":{"_imaps._tcp.x.com":["0 1 993 mail.x.com."]}, "cname":{"autoconfig.x.com":"w.x.com."},
//	 "fetch":{"https://mta-sts.x.com/.well-known/mta-sts.txt":"version: STSv1..."}}
type DevResolver struct {
	Path string
	Next Resolver
}

type devFile struct {
	TXT   map[string][]string `json:"txt"`
	MX    map[string][]string `json:"mx"`
	SRV   map[string][]string `json:"srv"`
	CNAME map[string]string   `json:"cname"`
	Fetch map[string]string   `json:"fetch"`
}

func (d DevResolver) load() (devFile, bool) {
	var f devFile
	b, err := os.ReadFile(d.Path)
	if err != nil || json.Unmarshal(b, &f) != nil {
		return f, false
	}
	return f, true
}

var errDevNone = errors.New("no such host")

func (d DevResolver) next() Resolver {
	if d.Next == nil {
		return noResolver{}
	}
	return d.Next
}

type noResolver struct{}

func (noResolver) LookupTXT(context.Context, string) ([]string, error) { return nil, errDevNone }
func (noResolver) LookupMX(context.Context, string) ([]MX, error)      { return nil, errDevNone }
func (noResolver) LookupSRV(context.Context, string) ([]SRV, error)    { return nil, errDevNone }
func (noResolver) LookupCNAME(context.Context, string) (string, error) { return "", errDevNone }

// LookupTXT implements Resolver.
func (d DevResolver) LookupTXT(ctx context.Context, n string) ([]string, error) {
	if f, ok := d.load(); ok {
		if v, ok := f.TXT[n]; ok {
			return v, nil
		}
		return nil, errDevNone
	}
	return d.next().LookupTXT(ctx, n)
}

// LookupMX implements Resolver.
func (d DevResolver) LookupMX(ctx context.Context, n string) ([]MX, error) {
	if f, ok := d.load(); ok {
		var out []MX
		for _, s := range f.MX[n] {
			p := strings.Fields(s)
			if len(p) == 2 {
				pr, _ := strconv.Atoi(p[0])
				out = append(out, MX{Host: p[1], Pref: uint16(pr)})
			}
		}
		if len(out) == 0 {
			return nil, errDevNone
		}
		return out, nil
	}
	return d.next().LookupMX(ctx, n)
}

// LookupSRV implements Resolver.
func (d DevResolver) LookupSRV(ctx context.Context, n string) ([]SRV, error) {
	if f, ok := d.load(); ok {
		var out []SRV
		for _, s := range f.SRV[n] {
			p := strings.Fields(s)
			if len(p) == 4 {
				a, _ := strconv.Atoi(p[0])
				b, _ := strconv.Atoi(p[1])
				c, _ := strconv.Atoi(p[2])
				out = append(out, SRV{Priority: uint16(a), Weight: uint16(b), Port: uint16(c), Target: p[3]})
			}
		}
		if len(out) == 0 {
			return nil, errDevNone
		}
		return out, nil
	}
	return d.next().LookupSRV(ctx, n)
}

// LookupCNAME implements Resolver.
func (d DevResolver) LookupCNAME(ctx context.Context, n string) (string, error) {
	if f, ok := d.load(); ok {
		if v, ok := f.CNAME[n]; ok {
			return v, nil
		}
		return "", errDevNone
	}
	return d.next().LookupCNAME(ctx, n)
}

// Fetcher returns a Fetcher backed by the file's "fetch" map (404 otherwise).
func (d DevResolver) Fetcher() Fetcher {
	return func(ctx context.Context, url string) (string, int, error) {
		if f, ok := d.load(); ok {
			if b, ok := f.Fetch[url]; ok {
				return b, 200, nil
			}
			return "", 404, nil
		}
		return HTTPFetcher(ctx, url)
	}
}
