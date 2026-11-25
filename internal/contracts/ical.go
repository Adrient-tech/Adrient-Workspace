package contracts

import (
	"fmt"
	"strings"
	"time"
)

func icsEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", "").Replace(s)
}

func icsFold(line string) string {
	if len(line) <= 73 {
		return line
	}
	var b strings.Builder
	for len(line) > 73 {
		cut := 73
		for cut > 0 && line[cut]&0xC0 == 0x80 { // do not split a UTF-8 sequence
			cut--
		}
		b.WriteString(line[:cut] + "\r\n ")
		line = line[cut:]
	}
	b.WriteString(line)
	return b.String()
}

// ICS renders every upcoming renewal, notice deadline and dated open obligation as an iCalendar file, with
// reminders 7 days and 1 day before. All-day events; no private notes are included.
func ICS(list []*Full, obs map[string][]Obligation, now time.Time, today Date, baseURL string) string {
	var b strings.Builder
	w := func(s string) { b.WriteString(icsFold(s) + "\r\n") }
	w("BEGIN:VCALENDAR")
	w("VERSION:2.0")
	w("PRODID:-//Adrient//Contracts//EN")
	w("CALSCALE:GREGORIAN")
	w("X-WR-CALNAME:Contract deadlines")
	stamp := now.UTC().Format("20060102T150405Z")
	ev := func(uid string, day Date, summary, desc, url string) {
		d, ok := parse(day)
		if !ok {
			return
		}
		w("BEGIN:VEVENT")
		w("UID:" + uid + "@adrient")
		w("DTSTAMP:" + stamp)
		w("DTSTART;VALUE=DATE:" + d.Format("20060102"))
		w("DTEND;VALUE=DATE:" + d.AddDate(0, 0, 1).Format("20060102"))
		w("SUMMARY:" + icsEscape(summary))
		if desc != "" {
			w("DESCRIPTION:" + icsEscape(desc))
		}
		if url != "" {
			w("URL:" + url)
		}
		for _, days := range []int{7, 1} {
			w("BEGIN:VALARM")
			w("ACTION:DISPLAY")
			w("DESCRIPTION:" + icsEscape(summary))
			w(fmt.Sprintf("TRIGGER:-P%dD", days))
			w("END:VALARM")
		}
		w("END:VEVENT")
	}
	for _, f := range list {
		if f.Status != "active" {
			continue
		}
		link := ""
		if baseURL != "" {
			link = baseURL + "/contracts?contract=" + f.ID
		}
		info := f.Contract.RenewalInfo(today, 60)
		if info.Date != "" && DaysBetween(today, info.Date) >= 0 {
			verb := "ends"
			if f.AutoRenew && !(f.Decision != nil && endingDecision(f.Decision.Kind) && f.Decision.ForTermEnd == info.Date) {
				verb = "renews"
			}
			ev(f.ID+"-end-"+info.Date, info.Date, "Contract "+verb+": "+f.Title, "Counterparty: "+f.PartyLabel, link)
			if info.NoticeDate != "" && DaysBetween(today, info.NoticeDate) >= 0 && !info.Handled {
				ev(f.ID+"-notice-"+info.NoticeDate, info.NoticeDate, "Notice deadline: "+f.Title, "Cancel or renegotiate by this date.", link)
			}
		}
		for _, o := range obs[f.ID] {
			if !o.Done && o.Due != "" && o.Owner != "them" {
				ev(o.ID+"-"+o.Due, o.Due, o.Title+" ("+f.Title+")", "", link)
			}
		}
	}
	w("END:VCALENDAR")
	return b.String()
}
