package inventory

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
	"google.golang.org/protobuf/encoding/protojson"
)

// productsPage serves GET /api/v1/stock/products/page: a flat, keyset-paged
// product list (?q=&category=&location_id=&archived=1&low=1&limit=&cursor=).
// limit defaults to 50 and is capped at 100.
func (c *call) productsPage() {
	q := c.r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	f := ProductFilter{Query: q.Get("q"), Category: q.Get("category"), LocationID: q.Get("location_id"),
		ArchivedOnly: q.Get("archived") == "1", Flat: q.Get("flat") == "1", Limit: limit + 1}
	lowOnly := q.Get("low") == "1"
	if parts, err := pagekit.DecodeCursor(q.Get("cursor")); err == nil && len(parts) == 3 {
		f.AfterName, f.AfterLabel, f.AfterID = parts[0], parts[1], parts[2]
	}
	ps, err := c.h.repo.ListProducts(c.ctx, c.org, f)
	if err != nil {
		c.list("items", ps, err)
		return
	}
	// The cursor follows the raw rows, so a low-stock page may hold fewer
	// than limit items but paging never stops early.
	next := ""
	if len(ps) > limit {
		ps = ps[:limit]
		last := ps[limit-1]
		next = pagekit.EncodeCursor(strings.ToLower(last.Name), last.VariantLabel, last.ID)
	}
	if lowOnly {
		kept := ps[:0]
		for _, p := range ps {
			low := p.MinStock > 0 && p.TotalStock <= p.MinStock
			for _, v := range p.Variants {
				low = low || (v.MinStock > 0 && v.TotalStock <= v.MinStock)
			}
			if low {
				kept = append(kept, p)
			}
		}
		ps = kept
	}
	// Same JSON shape as the existing /api/v1/inventory/products reply.
	items := make([]json.RawMessage, 0, len(ps))
	mo := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
	for _, p := range ps {
		b, err := mo.Marshal(productToProto(p))
		if err != nil {
			c.list("items", nil, err)
			return
		}
		items = append(items, b)
	}
	pagekit.Write(c.w, http.StatusOK, pagekit.Envelope{Items: items, NextCursor: next})
}
