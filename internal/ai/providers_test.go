package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These replay hand-written SSE in the documented shapes of each API. They
// check our request and parsing code, not the real services.

func collect(t *testing.T, c Client, req CompletionRequest) (string, []ToolCall, int, int) {
	t.Helper()
	var text string
	var calls []ToolCall
	in, out := 0, 0
	err := c.Stream(context.Background(), req, func(e Event) error {
		text += e.Delta
		if e.Call != nil {
			calls = append(calls, *e.Call)
		}
		in += e.InputTokens
		out += e.OutputTokens
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return text, calls, in, out
}

var toolReq = CompletionRequest{
	Model: "m", System: "sys", MaxTokens: 100,
	Messages: []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "t1", Name: "sheet_read_range", Arguments: `{"range":"A1"}`}}},
		{Role: "tool", ToolCallID: "t1", Content: `{"values":[[1]]}`},
	},
	Tools: []ToolDef{{Name: "sheet_read_range", Description: "d", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"range":{"type":"string"}}}`)}},
}

func TestAnthropicClient(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`event: message_start`, `data: {"type":"message_start","message":{"usage":{"input_tokens":12}}}`, ``,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, ``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`, ``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`, ``,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"sheet_read_range"}}`, ``,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"range\":"}}`, ``,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"B2\"}"}}`, ``,
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`, ``,
			`data: {"type":"message_stop"}`, ``,
		}, "\n"))
	}))
	defer srv.Close()
	c, _ := NewClient(Provider{Kind: KindAnthropic, BaseURL: srv.URL}, "k-anthropic", srv.Client())
	text, calls, in, out := collect(t, c, toolReq)
	if text != "Hello" || len(calls) != 1 || calls[0].ID != "toolu_1" || calls[0].Arguments != `{"range":"B2"}` || in != 12 || out != 7 {
		t.Fatalf("parsed %q %+v %d %d", text, calls, in, out)
	}
	if hdr.Get("x-api-key") != "k-anthropic" || hdr.Get("anthropic-version") == "" {
		t.Fatal("headers")
	}
	msgs := got["messages"].([]any)
	if got["system"] != "sys" || len(msgs) != 3 {
		t.Fatalf("request %v", got)
	}
	tr := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if tr["type"] != "tool_result" || tr["tool_use_id"] != "t1" {
		t.Fatalf("tool result block %v", tr)
	}
}

func TestGeminiClient(t *testing.T) {
	var got map[string]any
	var path, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, key = r.URL.String(), r.Header.Get("x-goog-api-key")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hi "}]}}]}`+"\n\n"+
			`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"sheet_read_range","args":{"range":"C3"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4}}`+"\n\n")
	}))
	defer srv.Close()
	c, _ := NewClient(Provider{Kind: KindGemini, BaseURL: srv.URL}, "k-gem", srv.Client())
	text, calls, in, out := collect(t, c, toolReq)
	if text != "Hi " || len(calls) != 1 || calls[0].Arguments != `{"range":"C3"}` || in != 9 || out != 4 {
		t.Fatalf("parsed %q %+v %d %d", text, calls, in, out)
	}
	if !strings.Contains(path, "/v1beta/models/m:streamGenerateContent?alt=sse") || key != "k-gem" {
		t.Fatalf("path %s key %s", path, key)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "additionalProperties") {
		t.Fatal("gemini schema must drop additionalProperties")
	}
	if !strings.Contains(string(raw), `"functionResponse":{"name":"sheet_read_range"`) {
		t.Fatalf("function response must carry the tool name: %s", raw)
	}
}

func TestAzureEndpoint(t *testing.T) {
	var path, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, key = r.URL.String(), r.Header.Get("api-key")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	c, _ := NewClient(Provider{Kind: KindAzure, BaseURL: srv.URL, APIVersion: "2024-10-21"}, "k-az", srv.Client())
	text, _, _, _ := collect(t, c, CompletionRequest{Model: "my-deploy", Messages: []Message{{Role: "user", Content: "x"}}})
	if text != "ok" || path != "/openai/deployments/my-deploy/chat/completions?api-version=2024-10-21" || key != "k-az" {
		t.Fatalf("%q %s %s", text, path, key)
	}
}
