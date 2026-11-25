package hr

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Pure attendance rules: classify one day from punches, the shift, holidays and
// leave. No database access.

// ShiftDef is the timing rules of one shift. Times are HH:MM local.
type ShiftDef struct {
	Start      string `json:"start_time"`
	End        string `json:"end_time"`
	GraceMin   int    `json:"grace_minutes"`
	EarlyMin   int    `json:"early_minutes"`
	FullMin    int    `json:"full_day_mins"`
	HalfMin    int    `json:"half_day_mins"`
	OTAfterMin int    `json:"ot_after_mins"`
}

// DefaultShift is used when an org has no shift configured.
var DefaultShift = ShiftDef{Start: "09:30", End: "18:30", GraceMin: 10, EarlyMin: 10, FullMin: 480, HalfMin: 240, OTAfterMin: 540}

// Day statuses.
const (
	DayPresent     = "present"
	DayHalf        = "half_day"
	DayAbsent      = "absent"
	DayLeave       = "leave"
	DayHoliday     = "holiday"
	DayWeeklyOff   = "weekly_off"
	DayMissingOut  = "missing_punch"
	DayInProgress  = "in_progress"
	DayUpcoming    = "upcoming"
	DayNotEmployed = "not_employed"
)

// DayInput is everything needed to classify one date.
type DayInput struct {
	Date      time.Time // midnight UTC label of the local date
	Employed  bool
	WeeklyOff bool // weekly off or roster day off
	Holiday   string
	Leave     string // "", "full", "first", "second"
	Punches   []time.Time
	Shift     ShiftDef
	Today     time.Time
	OTEnabled bool
	NowLocal  time.Time // current local time, to decide whether a lone punch is still in progress
	Location  *time.Location
}

// DayResult is the classified day.
type DayResult struct {
	Date      string  `json:"date"`
	Status    string  `json:"status"`
	FirstIn   string  `json:"first_in"`
	LastOut   string  `json:"last_out"`
	WorkedMin int     `json:"worked_min"`
	Late      bool    `json:"late"`
	LateMin   int     `json:"late_min"`
	EarlyOut  bool    `json:"early_out"`
	OTMin     int     `json:"ot_min"`
	Leave     string  `json:"leave"` // leave coverage: "", full, first, second
	Holiday   string  `json:"holiday"`
	LOP       float64 `json:"lop"` // attendance-originated loss of pay (days)
	Punches   int     `json:"punches"`
}

func hm(s string) (int, bool) {
	p := strings.Split(s, ":")
	if len(p) != 2 {
		return 0, false
	}
	h, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	if e1 != nil || e2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// ValidHM reports whether s is a HH:MM time.
func ValidHM(s string) bool { _, ok := hm(s); return ok }

func minutesOfDay(t time.Time) int { return t.Hour()*60 + t.Minute() }

// ClassifyDay derives the status of one date.
func ClassifyDay(in DayInput) DayResult {
	res := DayResult{Date: fmtDate(in.Date), Leave: in.Leave, Holiday: in.Holiday, Punches: len(in.Punches)}
	pts := append([]time.Time(nil), in.Punches...)
	sort.Slice(pts, func(i, j int) bool { return pts[i].Before(pts[j]) })
	if len(pts) > 0 {
		res.FirstIn = pts[0].Format("15:04")
	}
	if len(pts) > 1 {
		last := pts[len(pts)-1]
		res.LastOut = last.Format("15:04")
		res.WorkedMin = int(last.Sub(pts[0]).Minutes())
	}
	switch {
	case !in.Employed:
		res.Status = DayNotEmployed
		return res
	case in.Holiday != "":
		res.Status = DayHoliday
		return res
	case in.WeeklyOff:
		res.Status = DayWeeklyOff
		return res
	case in.Leave == "full":
		res.Status = DayLeave
		return res
	}
	sh := in.Shift
	if sh.FullMin == 0 {
		sh = DefaultShift
	}
	start, _ := hm(sh.Start)
	end, _ := hm(sh.End)
	if len(pts) > 0 {
		if m := minutesOfDay(pts[0]); m > start+sh.GraceMin {
			res.Late, res.LateMin = true, m-start
		}
	}
	required := sh.FullMin
	halfLeave := in.Leave == "first" || in.Leave == "second"
	if halfLeave {
		required = sh.HalfMin
	}
	future := in.Date.After(in.Today)
	isToday := in.Date.Equal(in.Today)
	switch {
	case len(pts) == 0:
		switch {
		case future:
			res.Status = DayUpcoming
		case isToday:
			res.Status = DayInProgress
		default:
			res.Status = DayAbsent
			res.LOP = 1
			if halfLeave {
				res.LOP = 0.5
			}
		}
	case len(pts) == 1:
		if isToday {
			res.Status = DayInProgress
		} else {
			res.Status = DayMissingOut
			res.LOP = 1
			if halfLeave {
				res.LOP = 0.5
			}
		}
	default:
		if m := minutesOfDay(pts[len(pts)-1]); m < end-sh.EarlyMin && res.WorkedMin < sh.FullMin {
			res.EarlyOut = true
		}
		switch {
		case res.WorkedMin >= required:
			res.Status = DayPresent
		case !halfLeave && res.WorkedMin >= sh.HalfMin:
			res.Status = DayHalf
			res.LOP = 0.5
		default:
			if isToday {
				res.Status = DayInProgress
			} else {
				res.Status = DayAbsent
				res.LOP = 1
				if halfLeave {
					res.LOP = 0.5
				}
			}
		}
		if in.OTEnabled && sh.OTAfterMin > 0 && res.WorkedMin > sh.OTAfterMin {
			res.OTMin = res.WorkedMin - sh.OTAfterMin
		}
	}
	return res
}

// MonthSummary totals a month of classified days.
type MonthSummary struct {
	Present, HalfDays, Absent, Leave, Holidays, WeeklyOffs float64
	LateCount, EarlyCount                                  int
	WorkedMin, OTMin                                       int
	LOP                                                    float64 // attendance-originated
}

// Summarise folds days into counts.
func Summarise(days []DayResult) MonthSummary {
	var s MonthSummary
	for _, d := range days {
		switch d.Status {
		case DayPresent:
			s.Present++
		case DayHalf:
			s.HalfDays++
			s.Present += 0.5
		case DayAbsent, DayMissingOut:
			s.Absent++
		case DayLeave:
			s.Leave++
		case DayHoliday:
			s.Holidays++
		case DayWeeklyOff:
			s.WeeklyOffs++
		}
		if d.Late {
			s.LateCount++
		}
		if d.EarlyOut {
			s.EarlyCount++
		}
		s.WorkedMin += d.WorkedMin
		s.OTMin += d.OTMin
		s.LOP += d.LOP
	}
	return s
}

// HaversineMetres is the great-circle distance between two coordinates.
func HaversineMetres(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLng := rad(lat2-lat1), rad(lng2-lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * R * math.Asin(math.Sqrt(a))
}

// ParseLogTime parses a biometric log timestamp in any of the common formats.
// Values without an offset are read in loc.
func ParseLogTime(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, f := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "02/01/2006 15:04:05", "02/01/2006 15:04", "02-01-2006 15:04:05", "02-01-2006 15:04", "2/1/2006 15:04"} {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised time %q", s)
}
