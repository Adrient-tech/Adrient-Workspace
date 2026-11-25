package honeypot

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
)

// feedParams reads ?limit= (default 100, max 500) and ?cursor= (from next_cursor).
func feedParams(r *http.Request) (limit int, before time.Time, beforeID string) {
	q := r.URL.Query()
	limit, _ = strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if parts, err := pagekit.DecodeCursor(q.Get("cursor")); err == nil && len(parts) == 2 {
		if t, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
			before, beforeID = t, parts[1]
		}
	}
	return limit, before, beforeID
}

// nextAlertCursor returns the cursor after the last kept alert when a look-ahead row exists.
func nextAlertCursor(rows []Alert, limit int) string {
	if len(rows) <= limit {
		return ""
	}
	last := rows[limit-1]
	return pagekit.EncodeCursor(last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID)
}

// trimAlerts drops the look-ahead row and returns the page.
func trimAlerts(rows *[]Alert, limit int) []Alert {
	if len(*rows) > limit {
		*rows = (*rows)[:limit]
	}
	return *rows
}
