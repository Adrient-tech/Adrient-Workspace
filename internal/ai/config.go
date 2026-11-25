// Package ai is the provider-agnostic AI gateway behind the assistant panel
// (docs/ai.md, docs/services/ai.md). It keeps model provider keys on the
// server, applies org settings, quotas and rate limits, wraps document content
// as untrusted data, validates tool calls against what the browser declared,
// and logs usage without message content.
package ai

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Provider kinds.
const (
	KindOpenAI    = "openai"    // any OpenAI-compatible /chat/completions endpoint (OpenAI, Ollama, llama.cpp, vLLM, OpenRouter, LM Studio)
	KindAzure     = "azure"     // Azure OpenAI deployments (api-key header, api-version query)
	KindAnthropic = "anthropic" // Anthropic Messages API
	KindGemini    = "gemini"    // Google Gemini generateContent API
)

// KeyEnvPrefix limits which environment variables a provider may read its key
// from, so an admin cannot point a provider at an unrelated server secret.
const KeyEnvPrefix = "ADRIENT_AI_"

// Provider is one configured model endpoint. The key is never part of the JSON.
type Provider struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	BaseURL    string   `json:"base_url"`
	Models     []string `json:"models"`
	KeyEnv     string   `json:"key_env,omitempty"`
	APIVersion string   `json:"api_version,omitempty"` // azure
	// Local marks an endpoint that runs on the organisation's own machines
	// (Ollama, llama.cpp, vLLM). "Local models only" allows only these.
	Local   bool `json:"local"`
	Enabled bool `json:"enabled"`
	// USD per 1M tokens, for the cost estimate in the usage log.
	PriceIn  float64 `json:"price_in_per_m,omitempty"`
	PriceOut float64 `json:"price_out_per_m,omitempty"`
	// HasKey is computed for the admin UI: a stored or env key exists.
	HasKey bool `json:"has_key"`
	// KeyHint is "stored", "env" or "" (never any part of the key itself).
	KeyHint string `json:"key_source"`

	sealedKey []byte
}

// Settings are the org's AI settings (adrient.ai_settings.settings).
type Settings struct {
	// KillSwitch turns the assistant off for everyone in the org, at once.
	KillSwitch bool `json:"kill_switch"`
	// LocalOnly allows only providers marked Local.
	LocalOnly bool `json:"local_only"`
	// DefaultModel is "<provider id>/<model>".
	DefaultModel string `json:"default_model"`
	// AppModels overrides the model per app ("sheets": "<provider id>/<model>").
	AppModels map[string]string `json:"app_models,omitempty"`
	// Apps lists apps where the assistant is switched off (missing = on).
	DisabledApps []string `json:"disabled_apps,omitempty"`
	// Notice is the data-handling notice shown in every panel.
	Notice string `json:"notice"`
	// Daily token budgets (input + output); 0 = unlimited.
	OrgDailyTokens  int64 `json:"org_daily_tokens"`
	UserDailyTokens int64 `json:"user_daily_tokens"`
	// UserPerMinute caps requests per user per minute; 0 = default (20).
	UserPerMinute int `json:"user_per_minute"`
	// MaxOutputTokens per model call; 0 = default (2048).
	MaxOutputTokens int `json:"max_output_tokens"`
}

// Apps that can host the assistant (shown in the admin page).
// Only apps that host an assistant are listed, so every switch in the admin page does something.
var Apps = []string{"sheets", "docs", "slides", "mail", "calendar", "drive", "chat", "contacts", "tasks", "keep", "forms", "crm", "hr", "talent", "shifts", "timetracking", "fieldservice", "fleet", "accounting", "inventory", "invoicing", "pos", "projects", "tickets", "records", "wiki", "contracts", "signatures", "legal", "captable", "podcasts", "admin", "global"}

// DefaultNotice is shown until the admin writes their own.
const DefaultNotice = "Your question and the parts of the open document the assistant needs are sent to the AI provider your organisation configured. Secrets such as passwords and API keys are removed first."

// AppEnabled reports whether the assistant is on for app.
func (s Settings) AppEnabled(app string) bool {
	for _, a := range s.DisabledApps {
		if a == app {
			return false
		}
	}
	return true
}

var (
	idRe    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	modelRe = regexp.MustCompile(`^[A-Za-z0-9._:/@+-]{1,200}$`)
	envRe   = regexp.MustCompile(`^ADRIENT_AI_[A-Z0-9_]{1,64}$`)
	appRe   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
)

// Validate checks a provider before saving.
func (p *Provider) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 100 {
		return errors.New("name is required (max 100 characters)")
	}
	switch p.Kind {
	case KindOpenAI, KindAzure, KindAnthropic, KindGemini:
	default:
		return errors.New("kind must be openai, azure, anthropic or gemini")
	}
	p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if p.BaseURL == "" {
		switch p.Kind {
		case KindAnthropic:
			p.BaseURL = "https://api.anthropic.com"
		case KindGemini:
			p.BaseURL = "https://generativelanguage.googleapis.com"
		case KindOpenAI:
			p.BaseURL = "https://api.openai.com/v1"
		default:
			return errors.New("base_url is required for Azure (https://<resource>.openai.azure.com)")
		}
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return errors.New("base_url must be an http(s) URL without credentials")
	}
	if u.Scheme == "http" && !isPrivateHost(u.Hostname()) {
		return errors.New("plain http is only allowed for local/private addresses; use https")
	}
	if len(p.Models) == 0 || len(p.Models) > 50 {
		return errors.New("list 1 to 50 model names")
	}
	for i, m := range p.Models {
		m = strings.TrimSpace(m)
		if !modelRe.MatchString(m) {
			return fmt.Errorf("invalid model name %q", m)
		}
		p.Models[i] = m
	}
	if p.KeyEnv != "" && !envRe.MatchString(p.KeyEnv) {
		return errors.New("key_env must look like ADRIENT_AI_<NAME>")
	}
	if p.PriceIn < 0 || p.PriceOut < 0 {
		return errors.New("prices cannot be negative")
	}
	if p.Kind == KindAzure && p.APIVersion == "" {
		p.APIVersion = "2024-10-21"
	}
	return nil
}

// Validate checks settings before saving.
func (s *Settings) Validate() error {
	if s.DefaultModel != "" && !modelRe.MatchString(s.DefaultModel) {
		return errors.New("invalid default model")
	}
	for app, m := range s.AppModels {
		if !appRe.MatchString(app) || (m != "" && !modelRe.MatchString(m)) {
			return fmt.Errorf("invalid model for app %q", app)
		}
	}
	for _, a := range s.DisabledApps {
		if !appRe.MatchString(a) {
			return fmt.Errorf("invalid app %q", a)
		}
	}
	if len(s.Notice) > 2000 {
		return errors.New("notice is too long (max 2000 characters)")
	}
	if s.OrgDailyTokens < 0 || s.UserDailyTokens < 0 || s.UserPerMinute < 0 || s.UserPerMinute > 600 {
		return errors.New("limits must be positive (per-minute at most 600)")
	}
	if s.MaxOutputTokens < 0 || s.MaxOutputTokens > 32000 {
		return errors.New("max output tokens must be between 0 and 32000")
	}
	return nil
}

// isPrivateHost reports loopback, private and link-local hosts (and "localhost").
func isPrivateHost(h string) bool {
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".local") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// SplitModel splits "<provider id>/<model>"; the model part may contain "/".
func SplitModel(ref string) (providerID, model string, ok bool) {
	i := strings.IndexByte(ref, '/')
	if i <= 0 || i == len(ref)-1 {
		return "", "", false
	}
	if !idRe.MatchString(ref[:i]) {
		return "", "", false
	}
	return ref[:i], ref[i+1:], true
}
