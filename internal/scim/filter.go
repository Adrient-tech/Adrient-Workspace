package scim

import (
	"fmt"
	"regexp"
	"strings"
)

// Filter is the subset of SCIM filters that IdPs send in practice:
// a single `attr op "value"` (eq, ne, co, sw, ew) or `attr pr`.
type Filter struct {
	Attr  string // lower-cased
	Op    string
	Value string
}

var filterRe = regexp.MustCompile(`^\s*([A-Za-z0-9_.:\[\]]+)\s+(eq|ne|co|sw|ew|pr)(?:\s+(?:"((?:[^"\\]|\\.)*)"|(true|false|null)))?\s*$`)

// ParseFilter parses a SCIM filter expression. An empty expression gives nil.
func ParseFilter(expr string) (*Filter, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, nil
	}
	m := filterRe.FindStringSubmatch(expr)
	if m == nil {
		return nil, fmt.Errorf("unsupported filter %q (use: attr eq \"value\")", expr)
	}
	f := &Filter{Attr: strings.ToLower(m[1]), Op: m[2], Value: strings.ReplaceAll(m[3], `\"`, `"`)}
	if m[4] != "" {
		f.Value = m[4]
	}
	if f.Op != "pr" && m[3] == "" && m[4] == "" && !strings.Contains(expr, `""`) {
		return nil, fmt.Errorf("filter %q needs a value", expr)
	}
	return f, nil
}

// Match applies the filter to a value.
func (f *Filter) Match(v string) bool {
	if f == nil {
		return true
	}
	lv, lf := strings.ToLower(v), strings.ToLower(f.Value)
	switch f.Op {
	case "eq":
		return lv == lf
	case "ne":
		return lv != lf
	case "co":
		return strings.Contains(lv, lf)
	case "sw":
		return strings.HasPrefix(lv, lf)
	case "ew":
		return strings.HasSuffix(lv, lf)
	case "pr":
		return v != ""
	}
	return false
}
