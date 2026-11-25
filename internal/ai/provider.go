package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message is one chat message in provider-neutral form.
type Message struct {
	Role       string     `json:"role"` // user | assistant | tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolDef is a function the model may call.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	// Risk is "read", "write" or "destructive" (from the browser contract).
	Risk string `json:"risk,omitempty"`
}

// ToolCall is a call requested by the model; Arguments is JSON text.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// CompletionRequest is what the gateway sends to a provider.
type CompletionRequest struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int
}

// Event is one streamed provider event. Exactly one field group is set.
type Event struct {
	Delta        string
	Call         *ToolCall
	InputTokens  int
	OutputTokens int
	Finish       string
}

// httpDoer is the subset of *http.Client the clients use.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client streams one completion.
type Client interface {
	Stream(ctx context.Context, req CompletionRequest, emit func(Event) error) error
}

// ProviderError is a provider failure safe to show (no body, no key).
type ProviderError struct {
	Status int
	Msg    string
}

func (e *ProviderError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("AI provider returned HTTP %d%s", e.Status, e.Msg)
	}
	return "AI provider error: " + e.Msg
}

// NewClient builds the client for a provider with its resolved key.
func NewClient(p Provider, key string, hc *http.Client) (Client, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	switch p.Kind {
	case KindOpenAI, KindAzure:
		return &openAIClient{p: p, key: key, hc: hc}, nil
	case KindAnthropic:
		return &anthropicClient{p: p, key: key, hc: hc}, nil
	case KindGemini:
		return &geminiClient{p: p, key: key, hc: hc}, nil
	}
	return nil, errors.New("unknown provider kind")
}

// post sends JSON and returns the streaming body, mapping HTTP errors to ProviderError.
func post(ctx context.Context, hc httpDoer, url string, hdr map[string]string, body any) (io.ReadCloser, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, &ProviderError{Msg: "bad provider URL"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ProviderError{Msg: "could not reach the provider"}
	}
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		hint := ""
		switch resp.StatusCode {
		case 401, 403:
			hint = " (check the API key)"
		case 404:
			hint = " (check the base URL and model name)"
		case 429:
			hint = " (provider rate limit or quota)"
		}
		return nil, &ProviderError{Status: resp.StatusCode, Msg: hint}
	}
	return resp.Body, nil
}

// sseData calls fn with each SSE data payload.
func sseData(r io.Reader, fn func(data []byte) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	var buf bytes.Buffer
	flush := func() error {
		if buf.Len() == 0 {
			return nil
		}
		d := bytes.TrimSpace(buf.Bytes())
		buf.Reset()
		if len(d) == 0 || string(d) == "[DONE]" {
			return nil
		}
		return fn(d)
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if buf.Len() > 0 {
				buf.WriteByte('\n')
			}
			buf.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := flush(); err != nil {
		return err
	}
	return sc.Err()
}
