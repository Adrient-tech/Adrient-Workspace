package tickets

import (
	"sync"
	"time"
)

// abuseGuard is a sliding-window counter per key.
type abuseGuard struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newAbuseGuard(limit int, window time.Duration) *abuseGuard {
	return &abuseGuard{hits: map[string][]time.Time{}, limit: limit, window: window}
}

// Allow records an event and reports whether it is within the limit.
func (g *abuseGuard) Allow(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	cut := now.Add(-g.window)
	list := g.hits[key][:0]
	for _, t := range g.hits[key] {
		if t.After(cut) {
			list = append(list, t)
		}
	}
	if len(list) >= g.limit {
		g.hits[key] = list
		return false
	}
	g.hits[key] = append(list, now)
	if len(g.hits) > 20000 {
		for k, v := range g.hits {
			if len(v) == 0 || !v[len(v)-1].After(cut) {
				delete(g.hits, k)
			}
		}
	}
	return true
}
