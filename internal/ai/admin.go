package ai

import (
	"context"
	"errors"
	"time"
)

// ProviderInput is the admin's create/update body. Key semantics:
// nil = keep the stored key, "" = remove it, anything else = store it (sealed).
type ProviderInput struct {
	Provider
	APIKey *string `json:"api_key,omitempty"`
}

// ListProviders returns providers without keys.
func (g *Gateway) ListProviders(ctx context.Context, orgID string) ([]Provider, error) {
	ps, err := g.Store.ListProviders(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Provider, 0, len(ps))
	for _, p := range ps {
		out = append(out, g.publicProvider(p))
	}
	return out, nil
}

// SaveProvider creates (id "") or updates a provider.
func (g *Gateway) SaveProvider(ctx context.Context, c Caller, id string, in ProviderInput) (Provider, error) {
	p := in.Provider
	if err := p.Validate(); err != nil {
		return Provider{}, &APIError{Status: 400, Code: "bad_request", Message: err.Error()}
	}
	create := id == ""
	if create {
		p.ID = newID()
	} else {
		if _, err := g.Store.GetProvider(ctx, c.OrgID, id); err != nil {
			return Provider{}, err
		}
		p.ID = id
	}
	var sealed []byte
	keyChanged := false
	if in.APIKey != nil {
		keyChanged = true
		if *in.APIKey == "" {
			sealed = []byte{}
		} else {
			if len(*in.APIKey) > 4096 {
				return Provider{}, &APIError{Status: 400, Code: "bad_request", Message: "API key is too long"}
			}
			b, err := sealKey(g.Secret, c.OrgID, p.ID, *in.APIKey)
			if err != nil {
				return Provider{}, &APIError{Status: 400, Code: "no_server_key", Message: err.Error()}
			}
			sealed = b
		}
	} else if create {
		sealed = []byte{}
	}
	if err := g.Store.PutProvider(ctx, c.OrgID, p, sealed); err != nil {
		return Provider{}, err
	}
	action := "provider.update"
	if create {
		action = "provider.create"
	}
	g.audit(ctx, action, map[string]any{"resource_type": "ai_provider", "resource_id": p.ID, "name": p.Name, "kind": p.Kind,
		"base_url": p.BaseURL, "local": p.Local, "enabled": p.Enabled, "key_changed": keyChanged})
	saved, err := g.Store.GetProvider(ctx, c.OrgID, p.ID)
	if err != nil {
		return Provider{}, err
	}
	return g.publicProvider(saved), nil
}

// DeleteProvider removes a provider.
func (g *Gateway) DeleteProvider(ctx context.Context, c Caller, id string) error {
	if err := g.Store.DeleteProvider(ctx, c.OrgID, id); err != nil {
		return err
	}
	g.audit(ctx, "provider.delete", map[string]any{"resource_type": "ai_provider", "resource_id": id})
	return nil
}

// TestResult is the outcome of "Test connection".
type TestResult struct {
	OK        bool   `json:"ok"`
	Model     string `json:"model"`
	LatencyMS int64  `json:"latency_ms"`
	Reply     string `json:"reply,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TestProvider sends a tiny prompt to the provider's first (or given) model.
func (g *Gateway) TestProvider(ctx context.Context, c Caller, id, model string) (TestResult, error) {
	p, err := g.Store.GetProvider(ctx, c.OrgID, id)
	if err != nil {
		return TestResult{}, err
	}
	if model == "" && len(p.Models) > 0 {
		model = p.Models[0]
	}
	res := TestResult{Model: model}
	key, _, err := g.resolveKey(c.OrgID, p)
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	cl, err := g.client(p, key)
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	var reply string
	err = cl.Stream(ctx, CompletionRequest{Model: model, MaxTokens: 16,
		Messages: []Message{{Role: "user", Content: "Reply with the single word OK."}}}, func(e Event) error {
		reply += e.Delta
		return nil
	})
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		var pe *ProviderError
		if errors.As(err, &pe) {
			res.Error = pe.Error()
		} else {
			res.Error = "could not reach the provider"
		}
	} else {
		res.OK = true
		res.Reply = truncate(scrub(reply, key), 200)
	}
	g.audit(ctx, "provider.test", map[string]any{"resource_type": "ai_provider", "resource_id": id, "model": model, "ok": res.OK})
	return res, nil
}

// SaveSettings validates and stores settings.
func (g *Gateway) SaveSettings(ctx context.Context, c Caller, s Settings) (Settings, error) {
	if err := s.Validate(); err != nil {
		return s, &APIError{Status: 400, Code: "bad_request", Message: err.Error()}
	}
	for _, ref := range append([]string{s.DefaultModel}, mapValues(s.AppModels)...) {
		if ref == "" {
			continue
		}
		pid, model, ok := SplitModel(ref)
		if !ok {
			return s, &APIError{Status: 400, Code: "bad_request", Message: "models must be chosen from a provider"}
		}
		p, err := g.Store.GetProvider(ctx, c.OrgID, pid)
		if err != nil || !contains(p.Models, model) {
			return s, &APIError{Status: 400, Code: "bad_request", Message: "unknown model " + model}
		}
	}
	old, _ := g.Store.GetSettings(ctx, c.OrgID)
	if err := g.Store.SaveSettings(ctx, c.OrgID, c.UserID, s); err != nil {
		return s, err
	}
	g.audit(ctx, "settings.update", map[string]any{"resource_type": "ai_settings", "kill_switch": s.KillSwitch,
		"kill_switch_changed": old.KillSwitch != s.KillSwitch, "local_only": s.LocalOnly, "disabled_apps": s.DisabledApps,
		"default_model": s.DefaultModel, "org_daily_tokens": s.OrgDailyTokens, "user_daily_tokens": s.UserDailyTokens})
	return s, nil
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}
