package audit

import (
	"context"
	"sync"
)

// Note lets a handler enrich the audit row the surrounding LogAPI wrapper (or
// the gRPC interceptor) is about to write: the affected entity, its id and a
// short human detail. It is concurrency-safe and a no-op when ctx carries no
// note, so domain code can call Annotate unconditionally.
type Note struct {
	mu           sync.Mutex
	ResourceType string
	ResourceID   string
	Details      []string
}

type noteKey struct{}

// WithNote returns a context carrying a fresh Note.
func WithNote(ctx context.Context) (context.Context, *Note) {
	n := &Note{}
	return context.WithValue(ctx, noteKey{}, n), n
}

// Annotate records the entity, id and detail text on the request's Note. The
// last non-empty entity/id wins; detail strings accumulate (max 8).
func Annotate(ctx context.Context, resourceType, resourceID, detail string) {
	n, _ := ctx.Value(noteKey{}).(*Note)
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if resourceType != "" {
		n.ResourceType = resourceType
	}
	if resourceID != "" {
		n.ResourceID = resourceID
	}
	if detail != "" && len(n.Details) < 8 {
		if len(detail) > 300 {
			detail = detail[:300]
		}
		n.Details = append(n.Details, detail)
	}
}

// apply merges the note into an event.
func (n *Note) apply(e *Event) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ResourceType != "" {
		e.ResourceType = n.ResourceType
	}
	if n.ResourceID != "" {
		e.ResourceID = n.ResourceID
	}
	if len(n.Details) > 0 {
		if e.Detail == nil {
			e.Detail = map[string]any{}
		}
		e.Detail["note"] = append([]string(nil), n.Details...)
	}
}
