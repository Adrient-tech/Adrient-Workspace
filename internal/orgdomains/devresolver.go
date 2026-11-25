package orgdomains

import (
	"context"
	"encoding/json"
	"os"
)

// DevOverrideResolver answers TXT lookups from a JSON file
// ({"_adrient-challenge.example.com": ["adrient-verify=..."]}) before falling
// back to Next. It exists for local development and tests only; the server
// enables it solely when both ADRIENT_DEV_MODE=1 and ADRIENT_DEV_TXT_OVERRIDE
// (the file path) are set.
type DevOverrideResolver struct {
	Path string
	Next Resolver
}

// LookupTXT implements Resolver.
func (d DevOverrideResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if b, err := os.ReadFile(d.Path); err == nil {
		m := map[string][]string{}
		if json.Unmarshal(b, &m) == nil {
			if v, ok := m[name]; ok {
				return v, nil
			}
		}
	}
	if d.Next == nil {
		return nil, context.DeadlineExceeded
	}
	return d.Next.LookupTXT(ctx, name)
}
