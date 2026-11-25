// Duration parsing, increment rounding, rate precedence, bill totals, matter numbering and conflict search
// scoring, with money as whole minor units.

// Package legal is practice management for a law firm or in-house legal team: clients, matters, deadlines, time
// and expenses, bills that become Invoicing documents, links to Drive, Signatures and Contracts, conflict checks
// and a matter status workflow. Money is stored as whole minor units (paise, cents); all arithmetic is integer.
package legal

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// MaxMinutes is a month of round-the-clock work; anything longer is a typo.
const MaxMinutes = 24 * 31 * 60

var (
	reClock  = regexp.MustCompile(`^(\d{1,3}):(\d{1,2})(?::(\d{1,2}))?$`)
	reHM     = regexp.MustCompile(`^(?:(\d*\.?\d+)\s*h(?:ours?|rs?)?)?\s*(?:(\d+)\s*m(?:in(?:utes?|s)?)?)?$`)
	reHMBare = regexp.MustCompile(`^(\d+)\s*h\s*(\d+)$`)
	reDec    = regexp.MustCompile(`^(\d*\.?\d+)$`)
)

// ParseDuration reads what a person typed into a duration field and returns whole minutes.
//
//	"1.5" or "1,5" -> 90      "1:12" -> 72     "1h 12m", "1h12" -> 72
//	"45m", "45 min" -> 45     "2h", "2 hrs" -> 120     ".3", ".3h" -> 18
func ParseDuration(in string) (int, bool) {
	m, ok := parseDuration(in)
	if !ok || m > MaxMinutes {
		return 0, false
	}
	return m, true
}

func roundHalfUp(f float64) int { return int(f + 0.5) }

func parseDuration(in string) (int, bool) {
	s := strings.ToLower(strings.TrimSpace(in))
	s = strings.Replace(s, ",", ".", 1)
	if s == "" {
		return 0, false
	}
	if m := reClock.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		if min > 59 {
			return 0, false
		}
		sec := 0
		if m[3] != "" {
			sec, _ = strconv.Atoi(m[3])
		}
		extra := 0
		if sec >= 30 {
			extra = 1
		}
		return h*60 + min + extra, true
	}
	if m := reHM.FindStringSubmatch(s); m != nil && (m[1] != "" || m[2] != "") {
		h, _ := strconv.ParseFloat(orZero(m[1]), 64)
		min, _ := strconv.Atoi(orZero(m[2]))
		return roundHalfUp(h*60) + min, true
	}
	if m := reHMBare.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		return h*60 + min, true
	}
	if m := reDec.FindStringSubmatch(s); m != nil {
		h, _ := strconv.ParseFloat(m[1], 64)
		return roundHalfUp(h * 60), true
	}
	return 0, false
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// RoundUpMinutes rounds minutes up to the billing increment (6 minutes = 0.1 hour by default). Zero stays zero.
func RoundUpMinutes(minutes, increment int) int {
	if minutes <= 0 {
		return 0
	}
	if increment < 1 {
		increment = 1
	}
	return (minutes + increment - 1) / increment * increment
}

// divRound returns round-half-up(a/b) for a >= 0, b > 0 (negative a rounds half away from zero).
func divRound(a, b int64) int64 {
	if a < 0 {
		return -divRound(-a, b)
	}
	return (2*a + b) / (2 * b)
}

// TimeValue is the value of a time entry in minor units: hours times the hourly rate, rounded half up.
func TimeValue(minutes int, rate int64) int64 { return divRound(int64(minutes)*rate, 60) }

// ExpenseValue is price x quantity (quantity in thousandths), rounded half up.
func ExpenseValue(price int64, qtyMilli int64) int64 { return divRound(price*qtyMilli, 1000) }

// BpOf returns amount x bp / 10000 rounded half up (bp = basis points, 1800 = 18%).
func BpOf(amount int64, bp int) int64 { return divRound(amount*int64(bp), 10000) }

// Rate chooses the hourly rate: the matter's own rate, then the client's, then the firm default.
func Rate(matterRate, clientRate, defaultRate int64) int64 {
	switch {
	case matterRate > 0:
		return matterRate
	case clientRate > 0:
		return clientRate
	}
	return defaultRate
}

// Line is one bill line, a snapshot of an entry.
type Line struct {
	EntryID     string `json:"entry_id,omitempty"`
	Kind        string `json:"kind"`
	Date        string `json:"date"`
	Description string `json:"description"`
	Minutes     int    `json:"minutes,omitempty"`
	Rate        int64  `json:"rate,omitempty"`
	Qty         string `json:"qty,omitempty"`
	Amount      int64  `json:"amount"`
}

// Totals are a bill's money figures.
type Totals struct {
	Fees     int64 `json:"fees"`
	Expenses int64 `json:"expenses"`
	Discount int64 `json:"discount"`
	Tax      int64 `json:"tax"`
	Total    int64 `json:"total"`
}

// BillTotals sums lines. The discount (basis points) applies to fees only; tax applies to fees after discount,
// and to expenses only when taxExpenses is set.
func BillTotals(lines []Line, discountBp, taxBp int, taxExpenses bool) Totals {
	var t Totals
	for _, l := range lines {
		if l.Kind == "expense" {
			t.Expenses += l.Amount
		} else {
			t.Fees += l.Amount
		}
	}
	disc := t.Fees
	if disc < 0 {
		disc = 0
	}
	if discountBp > 0 {
		t.Discount = BpOf(disc, min(discountBp, 10000))
	}
	taxable := t.Fees - t.Discount
	if taxExpenses {
		taxable += t.Expenses
	}
	if taxable > 0 && taxBp > 0 {
		t.Tax = BpOf(taxable, taxBp)
	}
	t.Total = t.Fees + t.Expenses - t.Discount + t.Tax
	return t
}

// ---- matter numbers and the status workflow -----------------------------------------------------------------

// DisplayNumber formats the matter number: "00012-Lopez", "00012" or "2026-00012".
func DisplayNumber(seq int, format, clientName, openDate string, now time.Time) string {
	n := fmt.Sprintf("%05d", seq)
	switch format {
	case "number":
		return n
	case "year-number":
		y := now.Year()
		if len(openDate) >= 4 {
			if v, err := strconv.Atoi(openDate[:4]); err == nil {
				y = v
			}
		}
		return fmt.Sprintf("%d-%s", y, n)
	}
	key := clientKey(clientName)
	if key == "" {
		return n
	}
	return n + "-" + key
}

// clientKey is the last word of a person's name (or first word of a company's), letters and digits only.
func clientKey(name string) string {
	f := strings.Fields(name)
	if len(f) == 0 {
		return ""
	}
	w := f[len(f)-1]
	var b strings.Builder
	for _, r := range w {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if r := []rune(out); len(r) > 24 {
		out = string(r[:24])
	}
	return out
}

// Statuses and the allowed moves between them.
var Statuses = []string{"intake", "open", "pending", "on_hold", "closed"}

var transitions = map[string][]string{
	"intake":  {"open", "closed"},
	"open":    {"pending", "on_hold", "closed"},
	"pending": {"open", "on_hold", "closed"},
	"on_hold": {"open", "pending", "closed"},
	"closed":  {"open"},
}

// CanMove reports whether a matter may move from one status to another.
func CanMove(from, to string) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// NextStatuses lists the statuses a matter may move to.
func NextStatuses(from string) []string { return append([]string(nil), transitions[from]...) }

// ---- conflict search -----------------------------------------------------------------------------------------

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// Normalize lower-cases and removes accents, so "José" matches "jose".
func Normalize(s string) string {
	out, _, err := transform.String(stripMarks, strings.ToLower(strings.TrimSpace(s)))
	if err != nil {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return strings.Join(strings.Fields(out), " ")
}

// Candidate is a person or company that may conflict: a client, a matter party or a contact.
type Candidate struct {
	Source string   `json:"source"` // client | party | contact
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Extra  []string `json:"extra,omitempty"` // company, emails, phones
	Role   string   `json:"role,omitempty"`
	Matter string   `json:"matter_id,omitempty"`
}

// ConflictScore rates how closely a candidate matches the query: 100 exact name, 70 starts with the first token
// and has all tokens, 60 has all tokens, 40 matches an email or phone or company, 0 no match.
func ConflictScore(query string, c Candidate) int {
	q := Normalize(strings.ReplaceAll(query, ",", " "))
	if len([]rune(q)) < 2 {
		return 0
	}
	tokens := strings.Fields(q)
	name := Normalize(strings.ReplaceAll(c.Name, ",", " "))
	switch {
	case name == q:
		return 100
	case allContain(name, tokens):
		if strings.HasPrefix(name, tokens[0]) {
			return 70
		}
		return 60
	}
	// Reordered "Smith, John" vs "John Smith" already works through tokens. A phone looks only at digits.
	qDigits := ""
	if isPhoneLike(q) {
		qDigits = digits(q)
	}
	for _, e := range c.Extra {
		ne := Normalize(e)
		if ne == "" {
			continue
		}
		if qDigits != "" && len(qDigits) >= 4 {
			d := digits(e)
			if len(d) >= 4 && (strings.Contains(d, qDigits) || (len(d) >= 7 && strings.HasSuffix(qDigits, d))) {
				return 40
			}
			continue
		}
		if strings.Contains(ne, q) || allContain(ne, tokens) {
			return 40
		}
	}
	return 0
}

var rePhone = regexp.MustCompile(`^[\d\s()+.-]+$`)

func isPhoneLike(q string) bool { return rePhone.MatchString(q) }

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func allContain(s string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, t := range tokens {
		if !strings.Contains(s, t) {
			return false
		}
	}
	return true
}

// Hit is one conflict-check result.
type Hit struct {
	Candidate
	Score int `json:"score"`
}

// RankHits scores candidates and returns those that match, best first, then by name.
func RankHits(query string, cands []Candidate) []Hit {
	var out []Hit
	for _, c := range cands {
		if s := ConflictScore(query, c); s > 0 {
			out = append(out, Hit{c, s})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return Normalize(out[i].Name) < Normalize(out[j].Name)
	})
	return out
}

// ---- dates ---------------------------------------------------------------------------------------------------

// ValidDate reports whether s is a real YYYY-MM-DD date.
func ValidDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// AddDays adds days to a YYYY-MM-DD date.
func AddDays(d string, n int) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

// DaysBetween is b - a in days.
func DaysBetween(a, b string) int {
	x, e1 := time.Parse("2006-01-02", a)
	y, e2 := time.Parse("2006-01-02", b)
	if e1 != nil || e2 != nil {
		return 0
	}
	return int(y.Sub(x).Hours() / 24)
}

// Urgency groups a deadline by how soon it is.
func Urgency(due, today string, done bool) string {
	if done {
		return "done"
	}
	d := DaysBetween(today, due)
	switch {
	case d < 0:
		return "overdue"
	case d <= 7:
		return "soon"
	}
	return "later"
}
