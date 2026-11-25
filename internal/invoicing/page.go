package invoicing

import (
	"net/http"
	"strconv"
)

// documentsPage serves GET /documents: ?limit= (default 100, max 500) &page=
// plus the filters. The reply keeps "documents" and adds items / next_cursor.
func (h *HTTPHandler) documentsPage(w http.ResponseWriter, r *http.Request, orgID string) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	docs, err := h.repo.ListDocuments(r.Context(), orgID, DocFilter{Kind: q.Get("kind"), Status: q.Get("status"),
		Query: q.Get("q"), ContactID: q.Get("contact_id"), Trash: q.Get("trash") == "1",
		Limit: limit + 1, Offset: (page - 1) * limit})
	if err != nil {
		replyKey(w, "documents", docs, err)
		return
	}
	next := ""
	if len(docs) > limit {
		docs, next = docs[:limit], strconv.Itoa(page+1)
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs, "items": docs, "next_cursor": next})
}
