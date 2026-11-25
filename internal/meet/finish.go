package meet

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Finisher ends a meeting once and tells everyone who needs to know. The
// storage change and both fan-outs are injected so the logic is testable.
type Finisher struct {
	Repo *Repository
	// Publish pushes a live event to every open workspace tab of the org so
	// Chat and Calendar views update without a reload.
	Publish func(orgID string, event map[string]any)
	// Notify tells one participant in the notification center.
	Notify func(ctx context.Context, orgID, userID string, info EndInfo)
}

// Finish ends the running session of roomID. A repeated call for the same end
// (host ends, then the room empties) is a no-op. It returns the end summary and
// whether this call ended the meeting.
func (f *Finisher) Finish(ctx context.Context, roomID, reason string) (EndInfo, bool) {
	info, ok, err := f.Repo.EndSession(ctx, roomID, reason)
	if err != nil {
		slog.Warn("meet: end session", "room", roomID, "err", err)
		return EndInfo{}, false
	}
	if !ok {
		return EndInfo{}, false
	}
	if f.Publish != nil {
		f.Publish(info.Room.OrgID, EndedEvent(info))
	}
	if f.Notify != nil {
		for _, uid := range info.Participants {
			f.Notify(ctx, info.Room.OrgID, uid, info)
		}
	}
	return info, true
}

// EndedEvent is the live event pushed when a meeting ends.
func EndedEvent(info EndInfo) map[string]any {
	return map[string]any{
		"type":              "meet_ended",
		"room_id":           info.Room.ID,
		"code":              info.Room.Code,
		"name":              info.Room.Name,
		"ended_at":          info.EndedAt.UTC().Format(time.RFC3339),
		"duration_s":        info.DurationS,
		"participant_count": len(info.Participants),
		"reason":            info.Room.EndedReason,
	}
}

// EndedBody is the notification text: duration and head count.
func EndedBody(info EndInfo) string {
	name := info.Room.Name
	if name == "" {
		name = "The meeting"
	}
	n := len(info.Participants)
	who := "1 participant"
	if n != 1 {
		who = fmt.Sprintf("%d participants", n)
	}
	return fmt.Sprintf("%s ended after %s · %s", name, FormatDuration(info.DurationS), who)
}

// FormatDuration renders seconds as "1h 05m", "12m 03s" or "45s".
func FormatDuration(s int) string {
	switch {
	case s >= 3600:
		return fmt.Sprintf("%dh %02dm", s/3600, s%3600/60)
	case s >= 60:
		return fmt.Sprintf("%dm %02ds", s/60, s%60)
	}
	return fmt.Sprintf("%ds", s)
}
