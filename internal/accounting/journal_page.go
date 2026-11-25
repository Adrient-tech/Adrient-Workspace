package accounting

import (
	"net/http"
	"strconv"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
)

// journalPage serves GET /journals with keyset paging:
// ?limit= (default 100, max 500) &cursor= (from next_cursor) plus the filters.
// The reply keeps the old "journals" key and adds items / next_cursor.
func (h *HTTPHandler) journalPage(w http.ResponseWriter, r *http.Request, orgID string) {
	q := r.URL.Query()
	lim, _ := strconv.Atoi(q.Get("limit"))
	if lim <= 0 {
		lim = 100
	}
	if lim > 500 {
		lim = 500
	}
	f := JournalFilter{From: q.Get("from"), To: q.Get("to"), Query: q.Get("q"), Source: q.Get("source"),
		ContactID: q.Get("contact_id"), AccountID: q.Get("account_id"), Limit: lim + 1}
	if parts, err := pagekit.DecodeCursor(q.Get("cursor")); err == nil && len(parts) == 3 {
		n, _ := strconv.ParseInt(parts[1], 10, 64)
		f.AfterDate, f.AfterNumber, f.AfterID = parts[0], n, parts[2]
	}
	js, err := h.repo.ListJournals(r.Context(), orgID, f)
	if err != nil {
		replyKey(w, "journals", js, err)
		return
	}
	next := ""
	if len(js) > lim {
		js = js[:lim]
		last := js[lim-1]
		next = pagekit.EncodeCursor(last.Date, strconv.FormatInt(last.Number, 10), last.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"journals": js, "items": js, "next_cursor": next})
}

// bankLinesPage serves GET /bank/lines: ?limit= (default 100, max 500) &page=.
func (h *HTTPHandler) bankLinesPage(w http.ResponseWriter, r *http.Request, orgID string) {
	q := r.URL.Query()
	lim, _ := strconv.Atoi(q.Get("limit"))
	if lim <= 0 {
		lim = 100
	}
	if lim > 500 {
		lim = 500
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	l, err := h.repo.ListBankLinesPage(r.Context(), orgID, q.Get("account_id"), q.Get("status"), lim+1, (page-1)*lim)
	if err != nil {
		replyKey(w, "lines", l, err)
		return
	}
	next := ""
	if len(l) > lim {
		l, next = l[:lim], strconv.Itoa(page+1)
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": l, "items": l, "next_cursor": next})
}
