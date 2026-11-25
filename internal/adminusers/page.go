package adminusers

import (
	"context"
	"net/http"
	"sort"
	"strconv"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
)

// OrgMemberPager returns one keyset page of the caller-org's member ids
// (identity-provider ids), ordered by name, plus the cursor of the next page
// ("" on the last page). It replaces the old fixed 200-member list.
type OrgMemberPager func(ctx context.Context, q, cursor string, limit int) (ids []string, next string, ok bool)

// WithOrgMemberPager injects the paged member resolver used by the user list.
func (h *Handler) WithOrgMemberPager(p OrgMemberPager) *Handler {
	h.orgPager = p
	return h
}

// pageParams reads limit (default 50, max 100) and cursor of the list request.
func pageParams(r *http.Request) (limit int, cursor string) {
	limit = 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = n
	}
	if limit > 100 {
		limit = 100
	}
	return limit, r.URL.Query().Get("cursor")
}

// orderLike sorts out to follow the order of ids (the provider returns its own order).
func orderLike(out []userOut, ids []string) {
	pos := make(map[string]int, len(ids))
	for i, id := range ids {
		pos[id] = i
	}
	sort.SliceStable(out, func(a, b int) bool { return pos[out[a].ID] < pos[out[b].ID] })
}

var _ = pagekit.CountCap
