package calendar

import (
	"fmt"
	"strings"
	"time"
)

// ICSEvent is one VEVENT read from an iCalendar file.
type ICSEvent struct {
	UID         string
	Title       string
	Description string
	Location    string
	Start, End  time.Time
	AllDay      bool
	Recurrence  string // in our RRULE subset, "" when none or unsupported
}

// MaxICSEvents caps one import.
const MaxICSEvents = 2000

func icsUnfold(s string) []string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(out) > 0 {
			out[len(out)-1] += l[1:]
			continue
		}
		out = append(out, l)
	}
	return out
}

func icsText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n', 'N':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// splitProp splits "NAME;P=V:value" into name, params and value.
func splitProp(line string) (string, map[string]string, string) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", nil, ""
	}
	head, val := line[:i], line[i+1:]
	parts := strings.Split(head, ";")
	params := map[string]string{}
	for _, p := range parts[1:] {
		if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
			params[strings.ToUpper(kv[0])] = strings.Trim(kv[1], `"`)
		}
	}
	return strings.ToUpper(parts[0]), params, val
}

func icsTime(val string, params map[string]string) (t time.Time, allDay bool, err error) {
	val = strings.TrimSpace(val)
	if params["VALUE"] == "DATE" || len(val) == 8 {
		t, err = time.ParseInLocation("20060102", val, time.UTC)
		return t, true, err
	}
	if strings.HasSuffix(val, "Z") {
		t, err = time.Parse("20060102T150405Z", val)
		return t, false, err
	}
	loc := time.UTC
	if tz := params["TZID"]; tz != "" {
		if l, e := time.LoadLocation(tz); e == nil {
			loc = l
		}
	}
	t, err = time.ParseInLocation("20060102T150405", val, loc)
	return t.UTC(), false, err
}

// icsRRule converts an iCalendar RRULE to our subset; "" if it cannot be represented.
func icsRRule(v string) string {
	var out []string
	freq := ""
	for _, p := range strings.Split(v, ";") {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k, val := strings.ToUpper(kv[0]), kv[1]
		switch k {
		case "FREQ":
			freq = strings.ToUpper(val)
			switch freq {
			case "DAILY", "WEEKLY", "MONTHLY", "YEARLY":
				out = append(out, "FREQ="+freq)
			default:
				return ""
			}
		case "INTERVAL", "COUNT":
			out = append(out, k+"="+val)
		case "BYDAY":
			for _, d := range strings.Split(val, ",") { // reject "1MO" style ordinals
				if _, ok := weekdayCodes[strings.ToUpper(d)]; !ok {
					return ""
				}
			}
			out = append(out, "BYDAY="+val)
		case "UNTIL":
			t, _, err := icsTime(val, nil)
			if err != nil {
				return ""
			}
			out = append(out, "UNTIL="+t.UTC().Format(time.RFC3339))
		case "BYMONTHDAY", "BYMONTH", "BYSETPOS", "BYYEARDAY", "BYWEEKNO":
			return "" // not supported by our expander; import as a single event
		}
	}
	if freq == "" {
		return ""
	}
	return strings.Join(out, ";")
}

// ParseICS reads VEVENTs from iCalendar text. Unparseable events are skipped
// and counted in skipped.
func ParseICS(data string) (events []ICSEvent, skipped int) {
	var cur *ICSEvent
	var endSet bool
	var dur string
	for _, line := range icsUnfold(data) {
		u := strings.ToUpper(strings.TrimSpace(line))
		if u == "BEGIN:VEVENT" {
			cur = &ICSEvent{}
			endSet, dur = false, ""
			continue
		}
		if u == "END:VEVENT" && cur != nil {
			ok := !cur.Start.IsZero()
			if ok && !endSet {
				if d, e := parseDuration(dur); e == nil && d > 0 {
					cur.End = cur.Start.Add(d)
				} else if cur.AllDay {
					cur.End = cur.Start.Add(24 * time.Hour)
				} else {
					cur.End = cur.Start
				}
			}
			if ok && cur.End.Before(cur.Start) {
				ok = false
			}
			if ok && len(events) < MaxICSEvents {
				events = append(events, *cur)
			} else {
				skipped++
			}
			cur = nil
			continue
		}
		if cur == nil {
			continue
		}
		name, params, val := splitProp(line)
		switch name {
		case "UID":
			cur.UID = strings.TrimSpace(val)
		case "SUMMARY":
			cur.Title = icsText(val)
		case "DESCRIPTION":
			cur.Description = icsText(val)
		case "LOCATION":
			cur.Location = icsText(val)
		case "DTSTART":
			if t, ad, err := icsTime(val, params); err == nil {
				cur.Start, cur.AllDay = t, ad
			}
		case "DTEND":
			if t, _, err := icsTime(val, params); err == nil {
				cur.End, endSet = t, true
			}
		case "DURATION":
			dur = val
		case "RRULE":
			cur.Recurrence = icsRRule(val)
		}
	}
	return events, skipped
}

// parseDuration reads an RFC 5545 duration such as PT1H30M, P1D, P1W.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("bad duration")
	}
	var d time.Duration
	num, inTime := 0, false
	for _, c := range s[1:] {
		switch {
		case c == 'T':
			inTime = true
		case c >= '0' && c <= '9':
			num = num*10 + int(c-'0')
		default:
			switch c {
			case 'W':
				d += time.Duration(num) * 7 * 24 * time.Hour
			case 'D':
				d += time.Duration(num) * 24 * time.Hour
			case 'H':
				d += time.Duration(num) * time.Hour
			case 'M':
				if inTime {
					d += time.Duration(num) * time.Minute
				}
			case 'S':
				d += time.Duration(num) * time.Second
			default:
				return 0, fmt.Errorf("bad duration")
			}
			num = 0
		}
	}
	return d, nil
}

func icsEsc(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", "").Replace(s)
}

func foldLine(l string) string {
	var b strings.Builder
	for len(l) > 75 {
		cut := 75
		for cut > 0 && l[cut]&0xC0 == 0x80 {
			cut--
		}
		b.WriteString(l[:cut] + "\r\n ")
		l = l[cut:]
	}
	b.WriteString(l + "\r\n")
	return b.String()
}

// ourRRuleToICS converts our recurrence string to an iCalendar RRULE value.
func ourRRuleToICS(rec string) string {
	rule, ok := parseRecurrence(rec)
	if !ok {
		return ""
	}
	if rule.freq == "WEEKDAY" {
		return "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"
	}
	out := "FREQ=" + rule.freq
	if rule.interval > 1 {
		out += fmt.Sprintf(";INTERVAL=%d", rule.interval)
	}
	if len(rule.byDay) > 0 {
		var codes []string
		for _, c := range []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"} {
			if rule.byDay[weekdayCodes[c]] {
				codes = append(codes, c)
			}
		}
		out += ";BYDAY=" + strings.Join(codes, ",")
	}
	if rule.count > 0 {
		out += fmt.Sprintf(";COUNT=%d", rule.count)
	}
	if rule.hasUntil {
		out += ";UNTIL=" + rule.until.UTC().Format("20060102T150405Z")
	}
	return out
}

// BuildICS renders master events (not exception rows) as an iCalendar feed.
// Private events are exported as "Busy" so a shared feed never leaks details.
func BuildCalendarICS(name string, events []Event, now time.Time) string {
	var b strings.Builder
	w := func(l string) { b.WriteString(foldLine(l)) }
	w("BEGIN:VCALENDAR")
	w("VERSION:2.0")
	w("PRODID:-//Adrient Workspace//Calendar//EN")
	w("CALSCALE:GREGORIAN")
	w("X-WR-CALNAME:" + icsEsc(name))
	for _, e := range events {
		if e.RecurrenceParentID != "" || e.ItemType == ItemTypeTask {
			continue
		}
		w("BEGIN:VEVENT")
		w("UID:" + e.ID + "@adrient")
		w("DTSTAMP:" + now.UTC().Format("20060102T150405Z"))
		if e.AllDay {
			w("DTSTART;VALUE=DATE:" + e.StartAt.UTC().Format("20060102"))
			end := e.EndAt.UTC()
			if !end.After(e.StartAt) {
				end = e.StartAt.Add(24 * time.Hour)
			}
			w("DTEND;VALUE=DATE:" + end.Format("20060102"))
		} else {
			w("DTSTART:" + e.StartAt.UTC().Format("20060102T150405Z"))
			w("DTEND:" + e.EndAt.UTC().Format("20060102T150405Z"))
		}
		title, desc, loc := e.Title, e.Description, e.Location
		if e.Visibility == VisibilityPrivate {
			title, desc, loc = "Busy", "", ""
		}
		w("SUMMARY:" + icsEsc(title))
		if desc != "" {
			w("DESCRIPTION:" + icsEsc(desc))
		}
		if loc != "" {
			w("LOCATION:" + icsEsc(loc))
		}
		if e.Status == StatusFree {
			w("TRANSP:TRANSPARENT")
		}
		if r := ourRRuleToICS(e.Recurrence); r != "" {
			w("RRULE:" + r)
		}
		for _, m := range e.Reminders {
			w("BEGIN:VALARM")
			w("ACTION:DISPLAY")
			w("DESCRIPTION:" + icsEsc(title))
			w(fmt.Sprintf("TRIGGER:-PT%dM", m))
			w("END:VALARM")
		}
		w("END:VEVENT")
	}
	w("END:VCALENDAR")
	return b.String()
}
