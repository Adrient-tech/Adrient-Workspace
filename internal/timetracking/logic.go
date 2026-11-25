// Package timetracking is the Adrient Time tracking module: timers and
// timesheets by client and project, billing rates with rounding, invoices in
// Invoicing and hours exported to HR payroll. logic.go is pure.
package timetracking

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ParseDuration reads "1:30", "90m", "1.5h", "1h 20m", "45", "2h30" into
// seconds. A bare integer above 24 means minutes, otherwise hours. ok is
// false when the text is not a duration.
func ParseDuration(in string) (int, bool) {
	s := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(in, ",", ".")))
	if s == "" {
		return 0, false
	}
	if strings.HasPrefix(s, ".") {
		s = "0" + s
	}
	if m := regexp.MustCompile(`^(\d+):([0-5]?\d)(?::([0-5]?\d))?$`).FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		se, _ := strconv.Atoi(m[3])
		return h*3600 + mi*60 + se, true
	}
	if m := regexp.MustCompile(`^:([0-5]?\d)$`).FindStringSubmatch(s); m != nil {
		mi, _ := strconv.Atoi(m[1])
		return mi * 60, true
	}
	if regexp.MustCompile(`^\d+(?:\.\d+)?$`).MatchString(s) {
		f, _ := strconv.ParseFloat(s, 64)
		if f == math.Trunc(f) && f > 24 {
			return int(f) * 60, true
		}
		return int(math.Round(f * 3600)), true
	}
	if m := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*h(?:ours?|rs?)?(?:\s*(\d+(?:\.\d+)?)\s*m(?:in(?:ute)?s?)?|\s+(\d+(?:\.\d+)?)|(\d+(?:\.\d+)?))?$`).FindStringSubmatch(s); m != nil {
		h, _ := strconv.ParseFloat(m[1], 64)
		var mi float64
		for _, x := range m[2:] {
			if x != "" {
				mi, _ = strconv.ParseFloat(x, 64)
				break
			}
		}
		return int(math.Round(h*3600 + mi*60)), true
	}
	if m := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*m(?:in(?:ute)?s?)?$`).FindStringSubmatch(s); m != nil {
		f, _ := strconv.ParseFloat(m[1], 64)
		return int(math.Round(f * 60)), true
	}
	if m := regexp.MustCompile(`^(\d+)\s*s(?:ec(?:ond)?s?)?$`).FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, true
	}
	return 0, false
}

// RoundSeconds rounds seconds up to the next multiple of minutes (0 = off).
func RoundSeconds(seconds, minutes int) int {
	if minutes <= 0 || seconds <= 0 {
		if seconds < 0 {
			return 0
		}
		return seconds
	}
	step := minutes * 60
	return (seconds + step - 1) / step * step
}

// Client is a customer billed for projects. ContactID links to Contacts.
type Client struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	ContactID   string   `json:"contact_id"`
	Email       string   `json:"email"`
	Color       string   `json:"color"`
	Currency    string   `json:"currency"`
	Rate        *float64 `json:"rate"`
	Rounding    *int     `json:"rounding"`
	MinMinutes  *int     `json:"minimum_minutes"`
	TaxRate     *float64 `json:"tax_rate"`
	TermsDays   *int     `json:"terms_days"`
	Archived    bool     `json:"archived"`
	Unbilled    float64  `json:"unbilled_amount,omitempty"`
	UnbilledSec int      `json:"unbilled_seconds,omitempty"`
}

// Project groups entries; billable by default; may have a budget.
type Project struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ClientID     string   `json:"client_id"`
	Color        string   `json:"color"`
	Rate         *float64 `json:"rate"`
	Billable     bool     `json:"billable"`
	BudgetHours  float64  `json:"budget_hours"`
	BudgetAmount float64  `json:"budget_amount"`
	Archived     bool     `json:"archived"`
}

// Settings are the organisation defaults.
type Settings struct {
	DefaultRate    float64            `json:"default_rate"`
	Currency       string             `json:"currency"`
	Rounding       int                `json:"rounding"`
	MinMinutes     int                `json:"minimum_minutes"`
	TaxRate        float64            `json:"tax_rate"`
	TermsDays      int                `json:"terms_days"`
	PayrollRate    float64            `json:"payroll_rate"`
	PayrollRates   map[string]float64 `json:"payroll_rates"` // HR employee id -> pay per hour
	WarnLongDayHrs float64            `json:"long_day_hours"`
}

// Entry is one block of time by one person on one day.
type Entry struct {
	ID            string   `json:"id"`
	UserID        string   `json:"user_id"`
	UserName      string   `json:"user_name,omitempty"`
	ProjectID     string   `json:"project_id"`
	Date          string   `json:"date"`
	Seconds       int      `json:"seconds"`
	Billable      bool     `json:"billable"`
	Task          string   `json:"task"`
	Note          string   `json:"note"`
	Start         string   `json:"start"`
	End           string   `json:"end"`
	Rate          *float64 `json:"rate"`
	InvoiceID     string   `json:"invoice_id"`
	LockedRate    *float64 `json:"locked_rate,omitempty"`
	LockedRound   *int     `json:"locked_rounding,omitempty"`
	LockedMinimum *int     `json:"locked_minimum,omitempty"`
	Billed        int      `json:"billed_seconds"`
	Amount        *float64 `json:"amount,omitempty"`
}

// Resolver turns entries into billing numbers using the rate cascade
// entry override > project > client > organisation default.
type Resolver struct {
	Settings Settings
	Projects map[string]Project
	Clients  map[string]Client
}

// Rate is the hourly rate that applies to the entry.
func (r Resolver) Rate(e Entry) float64 {
	if e.InvoiceID != "" && e.LockedRate != nil {
		return *e.LockedRate
	}
	if e.Rate != nil {
		return *e.Rate
	}
	p := r.Projects[e.ProjectID]
	if p.Rate != nil {
		return *p.Rate
	}
	if c, ok := r.Clients[p.ClientID]; ok && c.Rate != nil {
		return *c.Rate
	}
	return r.Settings.DefaultRate
}

func (r Resolver) rounding(e Entry) int {
	if e.InvoiceID != "" && e.LockedRound != nil {
		return *e.LockedRound
	}
	if c, ok := r.Clients[r.Projects[e.ProjectID].ClientID]; ok && c.Rounding != nil {
		return *c.Rounding
	}
	return r.Settings.Rounding
}

func (r Resolver) minimum(e Entry) int {
	if e.InvoiceID != "" && e.LockedMinimum != nil {
		return *e.LockedMinimum
	}
	if c, ok := r.Clients[r.Projects[e.ProjectID].ClientID]; ok && c.MinMinutes != nil {
		return *c.MinMinutes
	}
	return r.Settings.MinMinutes
}

// Billable reports whether the entry bills: the entry flag and the project flag.
func (r Resolver) Billable(e Entry) bool {
	p, ok := r.Projects[e.ProjectID]
	return e.Billable && (!ok || p.Billable)
}

// BilledSeconds is the time that bills: rounded up to the increment and at
// least the minimum. Zero for non-billable entries.
func (r Resolver) BilledSeconds(e Entry) int {
	if !r.Billable(e) || e.Seconds <= 0 {
		return 0
	}
	return max(RoundSeconds(e.Seconds, r.rounding(e)), r.minimum(e)*60)
}

// Amount is the money the entry earns (billed hours times rate, 2 decimals).
func (r Resolver) Amount(e Entry) float64 {
	return round2(float64(r.BilledSeconds(e)) / 3600 * r.Rate(e))
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// Hours converts seconds to hours with 2 decimals.
func Hours(sec int) float64 { return round2(float64(sec) / 3600) }
