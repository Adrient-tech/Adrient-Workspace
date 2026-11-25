package pos

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
)

// salesPage serves GET /api/v1/pos/x/sales/page: keyset-paged sales, newest
// first (?status=&q=&since=&until=&register_id=&limit=&cursor=). limit
// defaults to 50, cap 100. Items have the same JSON shape as the RPC reply.
func (h *ExtHandler) salesPage(w http.ResponseWriter, r *http.Request, orgID string) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	req := &adrientv1.ListPosSalesRequest{Status: q.Get("status"), Query: q.Get("q"), Since: q.Get("since"),
		Until: q.Get("until"), RegisterId: q.Get("register_id"), CustomerId: q.Get("customer_id"), Limit: int32(limit + 1)}
	var bt time.Time
	var bid string
	if parts, err := pagekit.DecodeCursor(q.Get("cursor")); err == nil && len(parts) == 2 {
		if t, err := time.Parse(time.RFC3339, parts[0]); err == nil {
			bt, bid = t, parts[1]
		}
	}
	sales, err := h.repo.listSales(r.Context(), orgID, req, bt, bid)
	if err != nil {
		fail(w, err)
		return
	}
	next := ""
	if len(sales) > limit {
		sales = sales[:limit]
		last := sales[limit-1]
		at := last.GetCompletedAt()
		if at == "" {
			at = last.GetCreatedAt()
		}
		next = pagekit.EncodeCursor(at, last.GetId())
	}
	mo := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
	items := make([]json.RawMessage, 0, len(sales))
	for _, s := range sales {
		b, err := mo.Marshal(s)
		if err != nil {
			fail(w, err)
			return
		}
		items = append(items, b)
	}
	pagekit.Write(w, http.StatusOK, pagekit.Envelope{Items: items, NextCursor: next})
}
