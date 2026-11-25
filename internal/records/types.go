// (the 27 field types, their options and the text parsing rules); written afresh in Go.

// Package records is an Airtable-style database: bases of tables with typed
// fields, linked records, computed fields and several views per table.
package records

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// The 27 field types.
const (
	TText        = "text"
	TLongText    = "long_text"
	TAttachment  = "attachment"
	TCheckbox    = "checkbox"
	TSelect      = "single_select"
	TMulti       = "multi_select"
	TDate        = "date"
	TPhone       = "phone"
	TEmail       = "email"
	TURL         = "url"
	TNumber      = "number"
	TCurrency    = "currency"
	TPercent     = "percent"
	TDuration    = "duration"
	TRating      = "rating"
	TBarcode     = "barcode"
	TLink        = "link"
	TLookup      = "lookup"
	TRollup      = "rollup"
	TCount       = "count"
	TFormula     = "formula"
	TAutonumber  = "autonumber"
	TCreatedTime = "created_time"
	TModifiedAt  = "modified_time"
	TCreatedBy   = "created_by"
	TModifiedBy  = "modified_by"
	TButton      = "button"
)

// AllTypes lists every field type in menu order.
var AllTypes = []string{TText, TLongText, TAttachment, TCheckbox, TSelect, TMulti, TDate, TPhone, TEmail, TURL, TNumber, TCurrency,
	TPercent, TDuration, TRating, TBarcode, TLink, TLookup, TRollup, TCount, TFormula, TAutonumber, TCreatedTime, TModifiedAt,
	TCreatedBy, TModifiedBy, TButton}

var computedTypes = map[string]bool{TLookup: true, TRollup: true, TCount: true, TFormula: true, TAutonumber: true,
	TCreatedTime: true, TModifiedAt: true, TCreatedBy: true, TModifiedBy: true, TButton: true}

// IsComputed reports whether values of the type are derived, never stored in the record.
func IsComputed(t string) bool { return computedTypes[t] }

// ValidType reports whether t is a field type.
func ValidType(t string) bool {
	for _, x := range AllTypes {
		if x == t {
			return true
		}
	}
	return false
}

// Choice is one option of a select field.
type Choice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// Field describes one column.
type Field struct {
	ID          string         `json:"id"`
	TableID     string         `json:"table_id"`
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Options     map[string]any `json:"options"`
	Position    int            `json:"position"`
}

// RollupFns are the rollup functions.
var RollupFns = []string{"sum", "avg", "min", "max", "count", "count_all", "count_unique", "concat", "unique", "and", "or", "earliest", "latest"}

func (f Field) optStr(k string) string {
	s, _ := f.Options[k].(string)
	return s
}

func (f Field) optInt(k string, def int) int {
	switch v := f.Options[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

func (f Field) optBool(k string) bool {
	b, _ := f.Options[k].(bool)
	return b
}

// Choices returns the select options.
func (f Field) Choices() []Choice {
	raw, _ := f.Options["choices"].([]any)
	out := make([]Choice, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]any)
		id, _ := m["id"].(string)
		name, _ := m["name"].(string)
		col, _ := m["color"].(string)
		if id != "" {
			out = append(out, Choice{ID: id, Name: name, Color: col})
		}
	}
	return out
}

func (f Field) choiceByName(n string) (Choice, bool) {
	for _, c := range f.Choices() {
		if strings.EqualFold(c.Name, strings.TrimSpace(n)) || c.ID == n {
			return c, true
		}
	}
	return Choice{}, false
}

// Precision is the decimal places of number, currency and percent fields.
func (f Field) Precision() int {
	p := f.optInt("precision", -1)
	if p < 0 {
		switch f.Type {
		case TCurrency:
			return 2
		case TPercent:
			return 0
		}
		return 0
	}
	if p > 8 {
		return 8
	}
	return p
}

// Attachment is a file stored in Drive.
type Attachment struct {
	FileID string `json:"file_id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Mime   string `json:"mime"`
}

var (
	reEmail    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	reNewlines = regexp.MustCompile(`\s*[\r\n]+\s*`)
	rePhone    = regexp.MustCompile(`^[0-9+()\-.\s/ext#*]{3,40}$`)
)

// MaxTextLen bounds one text value.
const MaxTextLen = 100000

// ValidateField checks a field definition (not its references to other fields).
func ValidateField(f Field) error {
	name := strings.TrimSpace(f.Name)
	if name == "" || utf8.RuneCountInString(name) > 120 {
		return knowkit.Invalid("field name must be 1 to 120 characters")
	}
	if strings.ContainsAny(name, "{}") {
		return knowkit.Invalid("field names cannot contain { or }")
	}
	if !ValidType(f.Type) {
		return knowkit.Invalid("unknown field type %q", f.Type)
	}
	switch f.Type {
	case TSelect, TMulti:
		seen := map[string]bool{}
		for _, c := range f.Choices() {
			n := strings.ToLower(strings.TrimSpace(c.Name))
			if n == "" || seen[n] {
				return knowkit.Invalid("option names must be unique and not empty")
			}
			seen[n] = true
		}
	case TRating:
		if m := f.optInt("max", 5); m < 1 || m > 10 {
			return knowkit.Invalid("rating maximum must be between 1 and 10")
		}
	case TNumber, TCurrency, TPercent:
		if p := f.optInt("precision", 0); p < 0 || p > 8 {
			return knowkit.Invalid("precision must be between 0 and 8")
		}
	case TRollup:
		fn := f.optStr("fn")
		ok := false
		for _, x := range RollupFns {
			ok = ok || x == fn
		}
		if !ok {
			return knowkit.Invalid("rollup function %q is not known", fn)
		}
	case TFormula:
		if _, err := ParseFormula(f.optStr("formula")); err != nil {
			return knowkit.Invalid("formula: %v", err)
		}
	}
	return nil
}

// Coerce checks and normalises a JSON value for storage in a field of the
// given type. nil, "" and empty lists all mean "empty" and return nil.
// Select values may be option ids or names; names are mapped to ids.
func Coerce(f Field, v any) (any, error) {
	if IsComputed(f.Type) {
		return nil, knowkit.Invalid("%q is calculated and cannot be edited", f.Name)
	}
	switch x := v.(type) {
	case int:
		v = float64(x)
	case int64:
		v = float64(x)
	case json.Number:
		n, _ := x.Float64()
		v = n
	}
	if v == nil {
		return nil, nil
	}
	bad := func(what string) (any, error) {
		return nil, knowkit.Invalid("%q: %s", f.Name, what)
	}
	switch f.Type {
	case TText, TLongText, TBarcode, TPhone, TEmail, TURL:
		s, ok := v.(string)
		if !ok {
			if n, isNum := v.(float64); isNum {
				s = fmtNum(n)
			} else {
				return bad("expected text")
			}
		}
		s = strings.ReplaceAll(s, "\x00", "")
		if f.Type != TLongText {
			s = strings.TrimSpace(reNewlines.ReplaceAllString(s, " "))
		}
		if s == "" {
			return nil, nil
		}
		if utf8.RuneCountInString(s) > MaxTextLen {
			return bad("text is too long")
		}
		switch f.Type {
		case TEmail:
			if _, err := mail.ParseAddress(s); err != nil || !reEmail.MatchString(s) {
				return bad("not a valid email address")
			}
		case TURL:
			if !strings.Contains(s, "://") && !strings.HasPrefix(s, "mailto:") {
				s = "https://" + s
			}
			u, err := url.Parse(s)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "mailto") || (u.Host == "" && u.Scheme != "mailto") {
				return bad("not a valid URL (http, https or mailto)")
			}
		case TPhone:
			if !rePhone.MatchString(s) {
				return bad("not a valid phone number")
			}
		}
		return s, nil
	case TCheckbox:
		switch x := v.(type) {
		case bool:
			if !x {
				return nil, nil
			}
			return true, nil
		case string:
			if parseBool(x) {
				return true, nil
			}
			return nil, nil
		case float64:
			if x != 0 {
				return true, nil
			}
			return nil, nil
		}
		return bad("expected true or false")
	case TNumber, TCurrency, TPercent, TDuration:
		var n float64
		switch x := v.(type) {
		case float64:
			n = x
		case string:
			if strings.TrimSpace(x) == "" {
				return nil, nil
			}
			p, err := parseNumberText(f, x)
			if err != nil {
				return bad("not a number")
			}
			n = p
		default:
			return bad("expected a number")
		}
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1e15 {
			return bad("number is out of range")
		}
		digits := f.Precision()
		if _, set := f.Options["precision"]; !set && f.Type == TNumber {
			digits = 8
		}
		return roundTo(n, digits+boolInt(f.Type == TPercent)*2, ""), nil
	case TRating:
		n, ok := v.(float64)
		if s, isStr := v.(string); isStr {
			p, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			n, ok = p, err == nil
		}
		if !ok || n != math.Trunc(n) {
			return bad("expected a whole number")
		}
		if n <= 0 {
			return nil, nil
		}
		if int(n) > f.optInt("max", 5) {
			return bad(fmt.Sprintf("rating is above the maximum of %d", f.optInt("max", 5)))
		}
		return n, nil
	case TDate:
		s, ok := v.(string)
		if !ok {
			return bad("expected a date text")
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		t, ok := parseDate(s)
		if !ok {
			return bad("not a valid date (use YYYY-MM-DD)")
		}
		if f.optBool("include_time") {
			return t.UTC().Format(time.RFC3339), nil
		}
		return t.Format("2006-01-02"), nil
	case TSelect:
		s, ok := v.(string)
		if !ok {
			if l, isList := v.([]any); isList && len(l) == 1 {
				s, ok = l[0].(string)
			}
		}
		if !ok {
			return bad("expected one option")
		}
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		c, found := f.choiceByName(s)
		if !found {
			return bad(fmt.Sprintf("%q is not one of the options", s))
		}
		return c.ID, nil
	case TMulti:
		var items []any
		switch x := v.(type) {
		case []any:
			items = x
		case string:
			for _, p := range splitList(x) {
				items = append(items, p)
			}
		default:
			return bad("expected a list of options")
		}
		seen := map[string]bool{}
		out := []any{}
		for _, it := range items {
			s, ok := it.(string)
			if !ok {
				return bad("expected option names")
			}
			if strings.TrimSpace(s) == "" {
				continue
			}
			c, found := f.choiceByName(s)
			if !found {
				return bad(fmt.Sprintf("%q is not one of the options", s))
			}
			if !seen[c.ID] {
				seen[c.ID] = true
				out = append(out, c.ID)
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	case TAttachment:
		l, ok := v.([]any)
		if !ok {
			return bad("expected a list of files")
		}
		out := []any{}
		for _, it := range l {
			m, ok := it.(map[string]any)
			if !ok {
				return bad("expected file objects")
			}
			id, _ := m["file_id"].(string)
			if !knowkit.IsUUID(id) {
				return bad("a file needs a Drive file_id")
			}
			name, _ := m["name"].(string)
			mime, _ := m["mime"].(string)
			size, _ := m["size"].(float64)
			out = append(out, map[string]any{"file_id": id, "name": name, "size": size, "mime": mime})
		}
		if len(out) == 0 {
			return nil, nil
		}
		if len(out) > 50 {
			return bad("at most 50 files per cell")
		}
		return out, nil
	case TLink:
		return nil, knowkit.Invalid("%q links are set with the record's links, not as a value", f.Name)
	}
	return bad("unsupported")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "y", "1", "x", "checked", "on", "✓", "✔":
		return true
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseNumberText reads "1,234.50", "$12", "12%", "1:30" (duration).
func parseNumberText(f Field, s string) (float64, error) {
	s = strings.TrimSpace(s)
	if f.Type == TDuration && strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		if len(parts) > 3 {
			return 0, fmt.Errorf("bad duration")
		}
		total := 0.0
		for _, p := range parts {
			n, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if err != nil || n < 0 {
				return 0, fmt.Errorf("bad duration")
			}
			total = total*60 + n
		}
		if len(parts) == 2 { // h:mm
			total *= 60
		}
		return total, nil
	}
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, s[1:len(s)-1]
	}
	pct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(s, "%")
	s = strings.Map(func(r rune) rune {
		if r == ',' || r == ' ' || r == '$' || r == '€' || r == '£' || r == '₹' || r == '¥' {
			return -1
		}
		return r
	}, s)
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if neg {
		n = -n
	}
	if pct {
		n /= 100
	}
	return n, nil
}

// ParseText turns pasted or imported text into a value for the field.
// Select options that do not exist yet are reported through newChoices.
func ParseText(f Field, s string) (val any, newChoices []string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil, nil
	}
	switch f.Type {
	case TCheckbox:
		v, err := Coerce(f, parseBool(s)) // never fails
		return v, nil, err
	case TSelect:
		if _, ok := f.choiceByName(s); !ok {
			return nil, []string{s}, nil
		}
	case TMulti:
		for _, p := range splitList(s) {
			if _, ok := f.choiceByName(p); !ok {
				newChoices = append(newChoices, p)
			}
		}
		if len(newChoices) > 0 {
			return nil, newChoices, nil
		}
	case TNumber, TCurrency, TPercent, TDuration:
		n, perr := parseNumberText(f, s)
		if perr != nil {
			return nil, nil, knowkit.Invalid("%q is not a number", s)
		}
		if f.Type == TPercent && !strings.Contains(s, "%") {
			n /= 100 // "50" in a percent field means 50%
		}
		v, cerr := Coerce(f, n)
		return v, nil, cerr
	case TAttachment, TLink:
		return nil, nil, knowkit.Invalid("%s cells cannot be pasted as text", f.Type)
	}
	v, err := Coerce(f, s)
	return v, nil, err
}

// Display renders a stored (or computed) value as plain text.
func Display(f Field, v any, linkNames func(any) string) string {
	if v == nil {
		return ""
	}
	switch f.Type {
	case TCheckbox:
		if b, _ := v.(bool); b {
			return "✓"
		}
		return ""
	case TSelect:
		if id, ok := v.(string); ok {
			for _, c := range f.Choices() {
				if c.ID == id {
					return c.Name
				}
			}
		}
		return toStr(v)
	case TMulti:
		l, _ := v.([]any)
		names := []string{}
		for _, e := range l {
			id, _ := e.(string)
			n := id
			for _, c := range f.Choices() {
				if c.ID == id {
					n = c.Name
				}
			}
			names = append(names, n)
		}
		return strings.Join(names, ", ")
	case TNumber, TCurrency:
		n, _ := v.(float64)
		s := strconv.FormatFloat(n, 'f', f.Precision(), 64)
		if f.Type == TCurrency {
			return f.currencySymbol() + s
		}
		return s
	case TPercent:
		n, _ := v.(float64)
		return strconv.FormatFloat(n*100, 'f', f.Precision(), 64) + "%"
	case TDuration:
		n, _ := v.(float64)
		sec := int64(math.Round(n))
		neg := ""
		if sec < 0 {
			neg, sec = "-", -sec
		}
		return fmt.Sprintf("%s%d:%02d:%02d", neg, sec/3600, sec%3600/60, sec%60)
	case TRating:
		n, _ := v.(float64)
		return strings.Repeat("★", int(n))
	case TAttachment:
		l, _ := v.([]any)
		names := []string{}
		for _, e := range l {
			if m, ok := e.(map[string]any); ok {
				n, _ := m["name"].(string)
				names = append(names, n)
			}
		}
		return strings.Join(names, ", ")
	case TLink:
		if linkNames != nil {
			return linkNames(v)
		}
	case TCreatedTime, TModifiedAt:
		if t, ok := v.(time.Time); ok {
			return t.UTC().Format(time.RFC3339)
		}
	}
	if l, ok := v.([]any); ok {
		parts := make([]string, len(l))
		for i, e := range l {
			parts[i] = toStr(e)
		}
		return strings.Join(parts, ", ")
	}
	if fe, ok := v.(FormulaError); ok {
		return "#ERROR! " + fe.Msg
	}
	return toStr(v)
}

func (f Field) currencySymbol() string {
	if s := f.optStr("symbol"); s != "" {
		return s
	}
	return "₹"
}
