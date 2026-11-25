package ai

import (
	"sync"
	"time"
)

// minuteLimiter is an in-memory sliding one-minute window per key. It is per
// process; with several replicas each replica enforces the limit on its own
// (the daily token quota is in Postgres and is shared).
type minuteLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	now  func() time.Time
}

func newMinuteLimiter() *minuteLimiter {
	return &minuteLimiter{hits: map[string][]time.Time{}, now: time.Now}
}

// allow records a hit for key and reports whether it is within limit per minute.
func (l *minuteLimiter) allow(key string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cut := now.Add(-time.Minute)
	h := l.hits[key]
	i := 0
	for i < len(h) && h[i].Before(cut) {
		i++
	}
	h = h[i:]
	if len(h) >= limit {
		l.hits[key] = h
		return false
	}
	l.hits[key] = append(h, now)
	if len(l.hits) > 10000 { // drop idle keys
		for k, v := range l.hits {
			if len(v) == 0 || v[len(v)-1].Before(cut) {
				delete(l.hits, k)
			}
		}
	}
	return true
}

// dayStart is the start of the current UTC day (quota window).
func dayStart(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// costUSD estimates the cost of a call from the provider's prices per 1M tokens.
func costUSD(p Provider, in, out int) float64 {
	return (float64(in)*p.PriceIn + float64(out)*p.PriceOut) / 1e6
}
