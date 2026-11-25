package ai

import (
	"context"
	"encoding/json"
	"net/url"
)

// geminiClient speaks the Google Gemini streamGenerateContent API (SSE).
type geminiClient struct {
	p   Provider
	key string
	hc  httpDoer
}

// geminiSchema drops JSON-Schema keywords Gemini rejects (additionalProperties).
func geminiSchema(raw json.RawMessage) any {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return map[string]any{"type": "object"}
	}
	var clean func(any) any
	clean = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, val := range t {
				if k == "additionalProperties" || k == "$schema" {
					continue
				}
				out[k] = clean(val)
			}
			return out
		case []any:
			for i := range t {
				t[i] = clean(t[i])
			}
			return t
		}
		return x
	}
	return clean(v)
}

func (c *geminiClient) Stream(ctx context.Context, req CompletionRequest, emit func(Event) error) error {
	names := map[string]string{} // tool call id -> name, for functionResponse
	var contents []map[string]any
	add := func(role string, part map[string]any) {
		if n := len(contents); n > 0 && contents[n-1]["role"] == role {
			contents[n-1]["parts"] = append(contents[n-1]["parts"].([]map[string]any), part)
			return
		}
		contents = append(contents, map[string]any{"role": role, "parts": []map[string]any{part}})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "assistant":
			if m.Content != "" {
				add("model", map[string]any{"text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				names[tc.ID] = tc.Name
				var args any = map[string]any{}
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				add("model", map[string]any{"functionCall": map[string]any{"name": tc.Name, "args": args}})
			}
		case "tool":
			var resp any
			if json.Unmarshal([]byte(m.Content), &resp) != nil {
				resp = m.Content
			}
			if _, ok := resp.(map[string]any); !ok {
				resp = map[string]any{"result": resp}
			}
			add("user", map[string]any{"functionResponse": map[string]any{"name": names[m.ToolCallID], "response": resp}})
		default:
			add("user", map[string]any{"text": m.Content})
		}
	}
	body := map[string]any{"contents": contents}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{"parts": []map[string]any{{"text": req.System}}}
	}
	if req.MaxTokens > 0 {
		body["generationConfig"] = map[string]any{"maxOutputTokens": req.MaxTokens}
	}
	if len(req.Tools) > 0 {
		var decls []map[string]any
		for _, t := range req.Tools {
			decls = append(decls, map[string]any{"name": t.Name, "description": t.Description, "parameters": geminiSchema(t.Parameters)})
		}
		body["tools"] = []map[string]any{{"functionDeclarations": decls}}
	}
	u := c.p.BaseURL + "/v1beta/models/" + url.PathEscape(req.Model) + ":streamGenerateContent?alt=sse"
	rc, err := post(ctx, c.hc, u, map[string]string{"x-goog-api-key": c.key}, body)
	if err != nil {
		return err
	}
	defer rc.Close()
	var calls []ToolCall
	finish := "stop"
	in, out := 0, 0
	err = sseData(rc, func(d []byte) error {
		var ch struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text         string `json:"text"`
						FunctionCall *struct {
							Name string          `json:"name"`
							Args json.RawMessage `json:"args"`
						} `json:"functionCall"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			Usage struct {
				In  int `json:"promptTokenCount"`
				Out int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}
		if json.Unmarshal(d, &ch) != nil {
			return nil
		}
		if ch.Usage.In > 0 {
			in = ch.Usage.In
		}
		if ch.Usage.Out > 0 {
			out = ch.Usage.Out
		}
		for _, cand := range ch.Candidates {
			for _, p := range cand.Content.Parts {
				if p.Text != "" {
					if err := emit(Event{Delta: p.Text}); err != nil {
						return err
					}
				}
				if p.FunctionCall != nil {
					args := string(p.FunctionCall.Args)
					if args == "" || args == "null" {
						args = "{}"
					}
					calls = append(calls, ToolCall{ID: "call_" + newID()[:8], Name: p.FunctionCall.Name, Arguments: args})
				}
			}
			if cand.FinishReason != "" {
				finish = cand.FinishReason
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := emit(Event{InputTokens: in, OutputTokens: out}); err != nil {
		return err
	}
	for i := range calls {
		if err := emit(Event{Call: &calls[i]}); err != nil {
			return err
		}
	}
	if len(calls) > 0 {
		finish = "tool_calls"
	}
	return emit(Event{Finish: finish})
}
