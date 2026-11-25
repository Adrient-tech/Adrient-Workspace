// Package pagekit holds the shared pieces of server-side listing: parsing the
// standard query parameters (limit, cursor, page, sort, order, q), opaque
// keyset cursors, the {items, next_cursor, total_estimate} envelope, capped
// counts and streaming CSV export. Handlers for big tables use it so every
// list answers the same way and no list can return an unbounded result.
package pagekit

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// Limits shared by every list endpoint.
const (
	DefaultLimit = 50
	MaxLimit     = 200
	// CountCap bounds exact counting: above it the envelope reports
	// total_estimate = CountCap with total_capped = true ("10,000+").
	CountCap = 10000
	// LegacyCap is the default cap for older endpoints that returned everything.
	LegacyCap = 100
)

// Params are the standard list parameters.
type Params struct {
	Limit  int
	Cursor []string // decoded keyset cursor, nil on the first page
	Page   int      // 1-based, used only by offset-paged endpoints
	Sort   string
	Desc   bool
	Q      string
	Filter map[string]string
}

// Parse reads the standard parameters. allowedSort lists the sortable column
// keys (the first is the default); an unknown sort falls back to the default.
// filterKeys lists the filter parameters to copy into Params.Filter.
func Parse(r *http.Request, defLimit, maxLimit int, allowedSort []string, filterKeys ...string) Params {
	q := r.URL.Query()
	if defLimit <= 0 {
		defLimit = DefaultLimit
	}
	if maxLimit <= 0 {
		maxLimit = MaxLimit
	}
	p := Params{Limit: defLimit, Page: 1, Q: strings.TrimSpace(q.Get("q")), Filter: map[string]string{}}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		p.Limit = n
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 0 {
		p.Page = n
	}
	if c := q.Get("cursor"); c != "" {
		if parts, err := DecodeCursor(c); err == nil {
			p.Cursor = parts
		}
	}
	if len(allowedSort) > 0 {
		p.Sort = allowedSort[0]
		for _, s := range allowedSort {
			if s == q.Get("sort") {
				p.Sort = s
			}
		}
	}
	p.Desc = strings.EqualFold(q.Get("order"), "desc")
	for _, k := range filterKeys {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			p.Filter[k] = v
		}
	}
	return p
}

// Offset is the row offset for page-based endpoints.
func (p Params) Offset() int { return (p.Page - 1) * p.Limit }

// EncodeCursor packs keyset values into an opaque URL-safe string.
func EncodeCursor(parts ...string) string {
	b, _ := json.Marshal(parts)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor reverses EncodeCursor.
func DecodeCursor(s string) ([]string, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var parts []string
	if err := json.Unmarshal(b, &parts); err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, errors.New("empty cursor")
	}
	return parts, nil
}

// Envelope is the standard list response.
type Envelope struct {
	Items         any    `json:"items"`
	NextCursor    string `json:"next_cursor,omitempty"`
	TotalEstimate *int64 `json:"total_estimate,omitempty"`
	TotalCapped   bool   `json:"total_capped,omitempty"`
}

// Write sends the envelope as JSON.
func Write(w http.ResponseWriter, status int, e Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
}

// Counter runs a cheap count query; it is satisfied by a closure over the pool.
type Counter func(ctx context.Context, sql string, args ...any) (int64, error)

// CappedCount counts the rows matching "FROM ... WHERE ..." up to CountCap.
// from is the text after "SELECT 1 " (table, joins and WHERE); the inner LIMIT
// stops the scan, so a huge table costs at most CountCap+1 index entries.
func CappedCount(ctx context.Context, count Counter, from string, args ...any) (total int64, capped bool, err error) {
	n, err := count(ctx, "SELECT count(*) FROM (SELECT 1 "+from+" LIMIT "+strconv.Itoa(CountCap+1)+") c", args...)
	if err != nil {
		return 0, false, err
	}
	if n > CountCap {
		return CountCap, true, nil
	}
	return n, false, nil
}

// SetTotal fills the envelope's count fields.
func (e *Envelope) SetTotal(total int64, capped bool) {
	e.TotalEstimate = &total
	e.TotalCapped = capped
}

// CSVWriter streams CSV rows with the response flushed in chunks so an export
// of millions of rows never sits in memory.
type CSVWriter struct {
	w  *csv.Writer
	f  http.Flusher
	n  int
	ok bool
}

// NewCSV starts a CSV download named filename with the given header row.
func NewCSV(w http.ResponseWriter, filename string, header []string) *CSVWriter {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeFilename(filename)+`"`)
	c := &CSVWriter{w: csv.NewWriter(w)}
	c.f, c.ok = w.(http.Flusher)
	_ = c.w.Write(header)
	return c
}

// Row writes one row. Cells starting with a formula character are prefixed so
// a spreadsheet does not run them.
func (c *CSVWriter) Row(cells ...string) {
	for i, s := range cells {
		if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
			cells[i] = "'" + s
		}
	}
	_ = c.w.Write(cells)
	c.n++
	if c.n%1000 == 0 {
		c.w.Flush()
		if c.ok {
			c.f.Flush()
		}
	}
}

// Close flushes the remaining rows.
func (c *CSVWriter) Close() {
	c.w.Flush()
	if c.ok {
		c.f.Flush()
	}
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '"' || r == '\\' || r < 32 || r == '/' {
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "export.csv"
	}
	return b.String()
}

// EscapeLike escapes LIKE wildcards in user input.
func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
