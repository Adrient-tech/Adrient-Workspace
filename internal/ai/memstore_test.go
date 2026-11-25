package ai

import (
	"context"
	"sync"
	"time"
)

// memStore is an in-memory Store for tests.
type memStore struct {
	mu        sync.Mutex
	settings  map[string]Settings
	providers map[string]Provider // id -> provider (org in orgOf)
	orgOf     map[string]string
	usage     []UsageRow
	extraUsed int64 // pretend usage, for quota tests
}

func newMemStore() *memStore {
	return &memStore{settings: map[string]Settings{}, providers: map[string]Provider{}, orgOf: map[string]string{}}
}

func (m *memStore) GetSettings(_ context.Context, org string) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings[org], nil
}
func (m *memStore) SaveSettings(_ context.Context, org, _ string, s Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[org] = s
	return nil
}
func (m *memStore) ListProviders(_ context.Context, org string) ([]Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Provider
	for id, p := range m.providers {
		if m.orgOf[id] == org {
			out = append(out, p)
		}
	}
	return out, nil
}
func (m *memStore) GetProvider(_ context.Context, org, id string) (Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.providers[id]
	if !ok || m.orgOf[id] != org {
		return Provider{}, ErrNotFound
	}
	return p, nil
}
func (m *memStore) PutProvider(_ context.Context, org string, p Provider, sealed []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o, ok := m.orgOf[p.ID]; ok && o != org {
		return ErrNotFound
	}
	if old, ok := m.providers[p.ID]; ok && sealed == nil {
		p.sealedKey = old.sealedKey
	} else if len(sealed) > 0 {
		p.sealedKey = append([]byte(nil), sealed...)
	} else {
		p.sealedKey = nil
	}
	m.providers[p.ID] = p
	m.orgOf[p.ID] = org
	return nil
}
func (m *memStore) DeleteProvider(_ context.Context, org, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.orgOf[id] != org {
		return ErrNotFound
	}
	delete(m.providers, id)
	delete(m.orgOf, id)
	return nil
}
func (m *memStore) LogUsage(_ context.Context, u UsageRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage = append(m.usage, u)
	return nil
}
func (m *memStore) TokensSince(_ context.Context, org, user string, _ time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := m.extraUsed
	for _, u := range m.usage {
		if u.OrgID == org && (user == "" || u.UserID == user) {
			n += int64(u.InputTokens + u.OutputTokens)
		}
	}
	return n, nil
}
func (m *memStore) UsageSummary(_ context.Context, org, by string, _ time.Time) ([]UsageAgg, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	agg := map[string]*UsageAgg{}
	var out []UsageAgg
	for _, u := range m.usage {
		if u.OrgID != org {
			continue
		}
		k := u.App
		if by == "model" {
			k = u.Model
		}
		a := agg[k]
		if a == nil {
			a = &UsageAgg{Key: k}
			agg[k] = a
		}
		a.Requests++
		a.InputTokens += int64(u.InputTokens)
		a.OutputTokens += int64(u.OutputTokens)
	}
	for _, a := range agg {
		out = append(out, *a)
	}
	return out, nil
}
