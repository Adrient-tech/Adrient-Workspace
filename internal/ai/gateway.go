package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Caller is the authenticated user making a request.
type Caller struct {
	OrgID    string
	UserID   string
	UserType string
	IsAdmin  bool
	// Allowed is the effective "ai.assistant" app control for this user
	// (per user type / group / IP rules in Admin > App controls).
	Allowed bool
	// CanOpenApp reports App access (Admin > App access); nil = allowed.
	CanOpenApp func(app string) bool
	// Can reports a capability (see policy.go); nil = none. Tools that need a
	// capability the caller lacks are not offered to the model.
	Can func(perm string) bool
}

func (c Caller) appOK(app string) bool {
	return app == "global" || c.CanOpenApp == nil || c.CanOpenApp(app)
}

// SearchHit is one permission-checked search result for "Ask Adrient".
type SearchHit struct {
	Type    string `json:"app"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

// Searcher runs the global search as the caller (org + user scoped).
type Searcher interface {
	Search(ctx context.Context, orgID, userID, query string, perType int) ([]SearchHit, error)
}

// DocAccess reports the caller's access to a document of app. known=false
// means the gateway has no checker for that app (the editor's own save path
// still enforces permissions).
type DocAccess func(ctx context.Context, c Caller, app, docID string) (read, write, known bool)

// APIError is a request failure with a stable code for the panel.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func apiErr(status int, code, msg string) *APIError {
	return &APIError{Status: status, Code: code, Message: msg}
}

// ChatRequest is the body of POST /api/v1/ai/chat (see web/app/src/lib/ai/types.ts).
type ChatRequest struct {
	App        string    `json:"app"`
	DocumentID string    `json:"document_id,omitempty"`
	Model      string    `json:"model,omitempty"`
	Messages   []Message `json:"messages"`
	Tools      []ToolDef `json:"tools,omitempty"`
	Context    *struct {
		Label string `json:"label"`
		Text  string `json:"text,omitempty"`
	} `json:"context,omitempty"`
	SystemHint string `json:"system_hint,omitempty"`
	// Attachments are files the user attached (e.g. from Drive); untrusted data.
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment is a text file attached to the conversation.
type Attachment struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// StreamEvent is one SSE event to the browser.
type StreamEvent struct {
	Type         string    `json:"type"`
	Text         string    `json:"text,omitempty"`
	Call         *ToolCall `json:"call,omitempty"`
	Title        string    `json:"title,omitempty"`
	URL          string    `json:"url,omitempty"`
	App          string    `json:"app,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	Code         string    `json:"code,omitempty"`
	Message      string    `json:"message,omitempty"`
	Finish       string    `json:"finish,omitempty"`
}

// Gateway is the AI service.
type Gateway struct {
	Store     Store
	Secret    string              // ADRIENT_SECRETS_KEY
	Env       func(string) string // os.Getenv by default
	HTTP      *http.Client
	Searcher  Searcher
	DocAccess DocAccess
	Audit     func(ctx context.Context, action string, detail map[string]any)
	Log       *slog.Logger
	// NewClient builds provider clients (tests swap it).
	NewClient func(p Provider, key string) (Client, error)
	Now       func() time.Time

	limiter *minuteLimiter
}

// NewGateway constructs a gateway with defaults.
func NewGateway(store Store, secret string) *Gateway {
	return &Gateway{Store: store, Secret: secret, limiter: newMinuteLimiter()}
}

func (g *Gateway) env(k string) string {
	if g.Env != nil {
		return g.Env(k)
	}
	return os.Getenv(k)
}

func (g *Gateway) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *Gateway) log() *slog.Logger {
	if g.Log != nil {
		return g.Log
	}
	return slog.Default()
}

func (g *Gateway) audit(ctx context.Context, action string, d map[string]any) {
	if g.Audit != nil {
		g.Audit(ctx, action, d)
	}
}

func (g *Gateway) client(p Provider, key string) (Client, error) {
	if g.NewClient != nil {
		return g.NewClient(p, key)
	}
	return NewClient(p, key, g.HTTP)
}

// resolveKey returns the provider's key: the stored sealed key, else key_env.
func (g *Gateway) resolveKey(orgID string, p Provider) (string, string, error) {
	if len(p.sealedKey) > 0 {
		k, err := openKey(g.Secret, orgID, p.ID, p.sealedKey)
		return k, "stored", err
	}
	if p.KeyEnv != "" && strings.HasPrefix(p.KeyEnv, KeyEnvPrefix) {
		if k := g.env(p.KeyEnv); k != "" {
			return k, "env", nil
		}
	}
	return "", "", nil
}

// publicProvider fills the computed key fields; the key itself never leaves.
func (g *Gateway) publicProvider(p Provider) Provider {
	p.HasKey, p.KeyHint = false, ""
	if len(p.sealedKey) > 0 {
		p.HasKey, p.KeyHint = true, "stored"
	} else if p.KeyEnv != "" && g.env(p.KeyEnv) != "" {
		p.HasKey, p.KeyHint = true, "env"
	}
	p.sealedKey = nil
	return p
}

func usable(s Settings, p Provider) bool { return p.Enabled && (!s.LocalOnly || p.Local) }

// resolveModel picks the provider and model for app.
func (g *Gateway) resolveModel(ctx context.Context, orgID string, s Settings, app, requested string) (Provider, string, *APIError) {
	ps, err := g.Store.ListProviders(ctx, orgID)
	if err != nil {
		return Provider{}, "", apiErr(500, "internal", "could not load AI providers")
	}
	find := func(ref string) (Provider, string, bool) {
		pid, model, ok := SplitModel(ref)
		if !ok {
			return Provider{}, "", false
		}
		for _, p := range ps {
			if p.ID == pid && usable(s, p) {
				for _, m := range p.Models {
					if m == model {
						return p, m, true
					}
				}
			}
		}
		return Provider{}, "", false
	}
	if requested != "" {
		if p, m, ok := find(requested); ok {
			return p, m, nil
		}
		return Provider{}, "", apiErr(400, "model_not_allowed", "that model is not enabled for your organisation")
	}
	for _, ref := range []string{s.AppModels[app], s.DefaultModel} {
		if p, m, ok := find(ref); ok {
			return p, m, nil
		}
	}
	for _, p := range ps {
		if usable(s, p) && len(p.Models) > 0 {
			return p, p.Models[0], nil
		}
	}
	if s.LocalOnly {
		return Provider{}, "", apiErr(503, "not_configured", "only local models are allowed and none is configured; ask your admin")
	}
	return Provider{}, "", apiErr(503, "not_configured", "AI is not configured; ask your admin")
}

// Status is GET /api/v1/ai/status.
type Status struct {
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
	Reason     string `json:"reason,omitempty"`
	Model      string `json:"model,omitempty"`
	LocalOnly  bool   `json:"local_only"`
	Notice     string `json:"notice,omitempty"`
	IsAdmin    bool   `json:"is_admin"`
}

// Status reports whether the assistant can run for the caller in app.
func (g *Gateway) Status(ctx context.Context, c Caller, app string) Status {
	st := Status{IsAdmin: c.IsAdmin}
	s, err := g.Store.GetSettings(ctx, c.OrgID)
	if err != nil {
		st.Reason = "not_configured"
		return st
	}
	st.LocalOnly = s.LocalOnly
	st.Notice = s.Notice
	if st.Notice == "" {
		st.Notice = DefaultNotice
	}
	p, m, aerr := g.resolveModel(ctx, c.OrgID, s, app, "")
	st.Configured = aerr == nil
	if aerr == nil {
		st.Model = p.Name + " · " + m
	}
	switch {
	case s.KillSwitch:
		st.Reason = "kill_switch"
	case !s.AppEnabled(app):
		st.Reason = "disabled_for_app"
	case !c.Allowed || !c.appOK(app):
		st.Reason = "disabled_for_user"
	case aerr != nil:
		st.Reason = "not_configured"
	default:
		st.Enabled = true
	}
	return st
}

var (
	toolNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	appNameRe  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
)

// prepared is a validated request ready to stream.
type prepared struct {
	caller     Caller
	app        string
	provider   Provider
	key        string
	req        CompletionRequest
	allowed    map[string]ToolDef
	redactions int
	sources    []SearchHit
}

// prepare runs every check that can fail before streaming starts.
func (g *Gateway) prepare(ctx context.Context, c Caller, r ChatRequest) (*prepared, *APIError) {
	if c.OrgID == "" || c.UserID == "" {
		return nil, apiErr(401, "unauthorized", "sign in first")
	}
	if !appNameRe.MatchString(r.App) {
		return nil, apiErr(400, "bad_request", "unknown app")
	}
	s, err := g.Store.GetSettings(ctx, c.OrgID)
	if err != nil {
		return nil, apiErr(500, "internal", "could not load AI settings")
	}
	if s.KillSwitch {
		return nil, apiErr(403, "kill_switch", "the assistant is switched off for your organisation")
	}
	if !s.AppEnabled(r.App) {
		return nil, apiErr(403, "disabled_for_app", "the assistant is not enabled for this app")
	}
	if !c.Allowed || !c.appOK(r.App) {
		return nil, apiErr(403, "disabled_for_user", "the assistant is not available for your account")
	}
	prov, model, aerr := g.resolveModel(ctx, c.OrgID, s, r.App, r.Model)
	if aerr != nil {
		return nil, aerr
	}
	perMin := s.UserPerMinute
	if perMin <= 0 {
		perMin = 20
	}
	if g.limiter == nil {
		g.limiter = newMinuteLimiter()
	}
	if !g.limiter.allow(c.OrgID+"|"+c.UserID, perMin) {
		return nil, apiErr(429, "rate_limited", "too many AI requests; wait a minute")
	}
	since := dayStart(g.now())
	if s.UserDailyTokens > 0 {
		if n, err := g.Store.TokensSince(ctx, c.OrgID, c.UserID, since); err == nil && n >= s.UserDailyTokens {
			return nil, apiErr(429, "quota", "you have used today's AI allowance")
		}
	}
	if s.OrgDailyTokens > 0 {
		if n, err := g.Store.TokensSince(ctx, c.OrgID, "", since); err == nil && n >= s.OrgDailyTokens {
			return nil, apiErr(429, "quota", "your organisation has used today's AI allowance")
		}
	}

	// Document permission: no read = no assistant on it; read-only = read tools only.
	writeOK := true
	if r.DocumentID != "" && g.DocAccess != nil {
		read, write, known := g.DocAccess(ctx, c, r.App, r.DocumentID)
		if known && !read {
			return nil, apiErr(403, "forbidden", "you do not have access to this document")
		}
		if known {
			writeOK = write
		}
	}

	// Tools declared by the browser.
	if len(r.Tools) > maxTools {
		return nil, apiErr(400, "bad_request", "too many tools")
	}
	allowed := map[string]ToolDef{}
	var tools []ToolDef
	for _, t := range r.Tools {
		if !toolNameRe.MatchString(t.Name) {
			return nil, apiErr(400, "bad_request", "invalid tool name")
		}
		if _, dup := allowed[t.Name]; dup {
			continue
		}
		if len(t.Parameters) == 0 {
			t.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		var obj map[string]any
		if len(t.Parameters) > maxToolSchema || json.Unmarshal(t.Parameters, &obj) != nil {
			return nil, apiErr(400, "bad_request", "tool "+t.Name+" has an invalid schema")
		}
		switch t.Risk {
		case "read", "write", "destructive":
		default:
			t.Risk = "write"
		}
		if !writeOK && t.Risk != "read" {
			continue
		}
		if !c.allowedByPolicy(t.Name) {
			continue
		}
		t.Description = truncate(t.Description, 1000)
		allowed[t.Name] = t
		tools = append(tools, t)
	}

	// Messages.
	if len(r.Messages) == 0 || len(r.Messages) > maxMessages {
		return nil, apiErr(400, "bad_request", "send between 1 and 80 messages")
	}
	total, redactions := 0, 0
	callIDs := map[string]bool{}
	stepTools := 0
	msgs := make([]Message, 0, len(r.Messages))
	for _, m := range r.Messages {
		if len(m.Content) > maxMessageChars {
			return nil, apiErr(400, "bad_request", "a message is too long")
		}
		total += len(m.Content)
		switch m.Role {
		case "user":
			stepTools = 0
			txt, n := Redact(m.Content)
			redactions += n
			msgs = append(msgs, Message{Role: "user", Content: txt})
		case "assistant":
			am := Message{Role: "assistant", Content: m.Content}
			for _, tc := range m.ToolCalls {
				if tc.ID == "" || !toolNameRe.MatchString(tc.Name) {
					return nil, apiErr(400, "bad_request", "invalid tool call in history")
				}
				callIDs[tc.ID] = true
				am.ToolCalls = append(am.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Name, Arguments: truncate(tc.Arguments, maxMessageChars)})
			}
			msgs = append(msgs, am)
		case "tool":
			if !callIDs[m.ToolCallID] {
				return nil, apiErr(400, "bad_request", "tool result without a matching tool call")
			}
			stepTools++
			if stepTools > maxSteps*4 {
				return nil, apiErr(400, "too_many_steps", "too many tool steps in one turn")
			}
			txt, n := Redact(truncate(m.Content, 6000))
			redactions += n
			msgs = append(msgs, Message{Role: "tool", ToolCallID: m.ToolCallID, Content: wrapData("tool result", txt)})
		default:
			return nil, apiErr(400, "bad_request", "invalid message role")
		}
	}
	if total > maxTotalChars {
		return nil, apiErr(400, "bad_request", "the conversation is too long; start a new chat")
	}
	if last := msgs[len(msgs)-1].Role; last == "assistant" {
		return nil, apiErr(400, "bad_request", "the last message must come from the user or a tool")
	}

	key, _, err := g.resolveKey(c.OrgID, prov)
	if err != nil {
		g.log().Warn("ai: provider key unavailable", "provider", prov.ID, "err", err.Error())
		return nil, apiErr(503, "not_configured", "the AI provider key cannot be read; ask your admin")
	}

	label, text := "", ""
	if r.Context != nil {
		label, text = r.Context.Label, r.Context.Text
	}
	hint := r.SystemHint
	if !writeOK {
		hint = strings.TrimSpace(hint + "\nThe user can only view this document, so you cannot change it.")
	}
	system, n := buildSystem(r.App, hint, label, text)
	redactions += n
	if len(r.Attachments) > 5 {
		return nil, apiErr(400, "bad_request", "attach at most 5 files")
	}
	budget := maxAttachChars
	for _, a := range r.Attachments {
		t, n := Redact(truncate(a.Text, budget))
		redactions += n
		budget -= len(t)
		system += "\n\nAttached file " + fmt.Sprintf("%q", truncate(a.Name, 200)) + ":\n" + wrapData("attached file", t)
		if budget <= 0 {
			break
		}
	}
	maxOut := s.MaxOutputTokens
	if maxOut <= 0 {
		maxOut = 2048
	}
	return &prepared{
		caller: c, app: r.App, provider: prov, key: key, allowed: allowed, redactions: redactions,
		req: CompletionRequest{Model: model, System: system, Messages: msgs, Tools: tools, MaxTokens: maxOut},
	}, nil
}

// run streams a prepared request and records usage and audit.
func (g *Gateway) run(ctx context.Context, p *prepared, emit func(StreamEvent) error) {
	for _, s := range p.sources {
		_ = emit(StreamEvent{Type: "source", Title: s.Title, URL: s.URL, App: s.Type})
	}
	in, out := 0, 0
	var called []string
	rejected := 0
	sentText := false
	status := "ok"
	cl, err := g.client(p.provider, p.key)
	if err == nil {
		err = cl.Stream(ctx, p.req, func(e Event) error {
			in += e.InputTokens
			out += e.OutputTokens
			switch {
			case e.Delta != "":
				sentText = true
				return emit(StreamEvent{Type: "delta", Text: e.Delta})
			case e.Call != nil:
				tc := *e.Call
				if _, ok := p.allowed[tc.Name]; !ok {
					rejected++
					return nil
				}
				if strings.TrimSpace(tc.Arguments) == "" {
					tc.Arguments = "{}"
				}
				var obj map[string]any
				if json.Unmarshal([]byte(tc.Arguments), &obj) != nil {
					rejected++
					return nil
				}
				if tc.ID == "" {
					tc.ID = "call_" + newID()[:8]
				}
				called = append(called, tc.Name)
				return emit(StreamEvent{Type: "tool_call", Call: &tc})
			}
			return nil
		})
	}
	if err != nil {
		status = "error"
		var pe *ProviderError
		msg := "the AI request failed"
		code := "provider_error"
		switch {
		case errors.As(err, &pe):
			msg = pe.Error()
		case errors.Is(err, context.Canceled):
			status, code, msg = "cancelled", "cancelled", "stopped"
		}
		g.log().Warn("ai: chat failed", "org", p.caller.OrgID, "app", p.app, "provider", p.provider.ID, "err", scrub(err.Error(), p.key))
		if code != "cancelled" {
			_ = emit(StreamEvent{Type: "error", Code: code, Message: scrub(msg, p.key)})
		}
	} else if rejected > 0 && len(called) == 0 && !sentText {
		_ = emit(StreamEvent{Type: "delta", Text: "_The assistant tried to use an action that is not available here._"})
	}
	// Usage is recorded even when the client went away.
	bg := context.WithoutCancel(ctx)
	if err := g.Store.LogUsage(bg, UsageRow{
		OrgID: p.caller.OrgID, UserID: p.caller.UserID, App: p.app, ProviderID: p.provider.ID, Model: p.req.Model,
		InputTokens: in, OutputTokens: out, CostUSD: costUSD(p.provider, in, out), Status: status,
	}); err != nil {
		g.log().Warn("ai: usage log", "err", err.Error())
	}
	g.audit(bg, "chat", map[string]any{
		"resource_type": "ai_" + p.app, "app": p.app, "provider_id": p.provider.ID, "model": p.req.Model,
		"input_tokens": in, "output_tokens": out, "tool_calls": called, "rejected_tool_calls": rejected,
		"redactions": p.redactions, "sources": len(p.sources), "status": status,
	})
	if err == nil {
		_ = emit(StreamEvent{Type: "usage", InputTokens: in, OutputTokens: out})
		_ = emit(StreamEvent{Type: "done", Finish: "stop"})
	}
}

// Chat validates r and streams the answer. A non-nil APIError means nothing was streamed.
func (g *Gateway) Chat(ctx context.Context, c Caller, r ChatRequest, emit func(StreamEvent) error) *APIError {
	p, aerr := g.prepare(ctx, c, r)
	if aerr != nil {
		return aerr
	}
	g.run(ctx, p, emit)
	return nil
}

// AskRequest is the body of POST /api/v1/ai/ask.
type AskRequest struct {
	Question string    `json:"question"`
	History  []Message `json:"history,omitempty"`
}

var askStop = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a an and are as at be by can could did do does find for from get give had has have how i in is it its me my of on or our please show tell that the their them there these this to us was we were what when where which who why will with would you your about any all last latest recent my mine summarise summarize list") {
		askStop[w] = true
	}
}

// askTerms picks up to 4 search terms from a question.
func askTerms(q string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !(r == '@' || r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || r > 127)
	}) {
		w = strings.Trim(w, ".-_")
		if len(w) < 3 || askStop[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) == 4 {
			break
		}
	}
	return out
}

// Ask answers a question from the caller's own data via the global search.
func (g *Gateway) Ask(ctx context.Context, c Caller, r AskRequest, emit func(StreamEvent) error) *APIError {
	q := strings.TrimSpace(r.Question)
	if q == "" || len(q) > 2000 {
		return apiErr(400, "bad_request", "ask a question (max 2000 characters)")
	}
	var hits []SearchHit
	if g.Searcher != nil && c.OrgID != "" && c.UserID != "" {
		seen := map[string]bool{}
		for _, term := range askTerms(q) {
			res, err := g.Searcher.Search(ctx, c.OrgID, c.UserID, term, 5)
			if err != nil {
				g.log().Warn("ai: ask search", "err", err.Error())
				continue
			}
			for _, h := range res {
				k := h.Type + "|" + h.ID
				if seen[k] || len(hits) >= 12 {
					continue
				}
				seen[k] = true
				hits = append(hits, h)
			}
		}
	}
	var b strings.Builder
	if len(hits) == 0 {
		b.WriteString("No matching items were found in the user's Drive, Mail, Calendar, Contacts or documents.")
	}
	for i, h := range hits {
		fmt.Fprintf(&b, "[%d] %s: %s\n%s\n\n", i+1, h.Type, truncate(h.Title, 200), truncate(h.Snippet, 500))
	}
	msgs := append([]Message{}, r.History...)
	if len(msgs) > 20 {
		msgs = msgs[len(msgs)-20:]
	}
	for _, m := range msgs {
		if m.Role != "user" && m.Role != "assistant" {
			return apiErr(400, "bad_request", "history may only hold user and assistant messages")
		}
	}
	msgs = append(msgs, Message{Role: "user", Content: q})
	cr := ChatRequest{App: "global", Messages: msgs}
	cr.Context = &struct {
		Label string `json:"label"`
		Text  string `json:"text,omitempty"`
	}{Label: "Search results", Text: b.String()}
	p, aerr := g.prepare(ctx, c, cr)
	if aerr != nil {
		return aerr
	}
	p.sources = hits
	g.run(ctx, p, emit)
	return nil
}
