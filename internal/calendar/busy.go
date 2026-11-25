package calendar

import (
	"context"
	"strings"
	"time"
)

// BusySpan is a time range during which a person is not available.
type BusySpan struct {
	Start, End time.Time
}

// BusyFor returns the busy spans of one person in [min, max): events they own
// or were invited to (and did not decline) that are marked busy, including
// expanded recurring instances. Tasks and "free" events never block time.
// Exception overrides replace the occurrence they cover.
func (r *Repository) BusyFor(ctx context.Context, orgID, ownerID, ownerEmail string, min, max time.Time) ([]BusySpan, error) {
	list, err := r.List(ctx, orgID, min, max)
	if err != nil {
		return nil, err
	}
	declined := map[string]bool{}
	mine := func(e Event) bool {
		if e.OwnerID == ownerID {
			return true
		}
		for _, a := range e.Attendees {
			if strings.EqualFold(a, ownerEmail) {
				return true
			}
		}
		return false
	}
	// Attendee declines are stored per event; look them up for invited events.
	for _, e := range list {
		if e.OwnerID != ownerID && mine(e) {
			atts, err := r.ListAttendees(ctx, e.ID)
			if err != nil {
				return nil, err
			}
			for _, a := range atts {
				if strings.EqualFold(a.Email, ownerEmail) && a.ResponseStatus == "declined" {
					declined[e.ID] = true
				}
			}
		}
	}
	return busySpans(list, min, max, mine, declined), nil
}

// busySpans is the pure part of BusyFor (testable without a database).
func busySpans(list []Event, min, max time.Time, mine func(Event) bool, declined map[string]bool) []BusySpan {
	type key struct{ master, orig string }
	overrides := map[key]Event{}
	var masters []Event
	for _, e := range list {
		if e.RecurrenceParentID != "" {
			k := key{master: e.RecurrenceParentID}
			if e.OriginalStart != nil {
				k.orig = e.OriginalStart.UTC().Format(time.RFC3339)
			}
			overrides[k] = e
			continue
		}
		masters = append(masters, e)
	}
	var out []BusySpan
	add := func(e Event) {
		if !mine(e) || declined[e.ID] || declined[e.RecurringEventID] {
			return
		}
		if e.ItemType == ItemTypeTask || e.Status == StatusFree {
			return
		}
		if e.EndAt.After(min) && e.StartAt.Before(max) {
			out = append(out, BusySpan{Start: e.StartAt, End: e.EndAt})
		}
	}
	for _, m := range masters {
		for _, inst := range expandEvent(m, min, max) {
			if inst.RecurringEventID != "" {
				k := key{master: inst.RecurringEventID, orig: inst.StartAt.UTC().Format(time.RFC3339)}
				if ex, ok := overrides[k]; ok {
					add(ex)
					delete(overrides, k)
					continue
				}
			}
			add(inst)
		}
	}
	for _, ex := range overrides {
		add(ex)
	}
	return out
}
