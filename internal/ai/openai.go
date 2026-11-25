package ai

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
)

// openAIClient speaks the OpenAI Chat Completions streaming API, which Ollama,
// llama.cpp server, vLLM, LM Studio, OpenRouter and Azure OpenAI also serve.
type openAIClient struct {
	p   Provider
	key string
	hc  httpDoer
}

type oaMsg struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}
type oaToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

func (c *openAIClient) endpoint(model string) (string, map[string]string) {
	if c.p.Kind == KindAzure {
		u := c.p.BaseURL + "/openai/deployments/" + url.PathEscape(model) + "/chat/completions?api-version=" + url.QueryEscape(c.p.APIVersion)
		return u, map[string]string{"api-key": c.key}
	}
	h := map[string]string{}
	if c.key != "" {
		h["Authorization"] = "Bearer " + c.key
	}
	return c.p.BaseURL + "/chat/completions", h
}

func (c *openAIClient) Stream(ctx context.Context, req CompletionRequest, emit func(Event) error) error {
	msgs := []oaMsg{}
	if req.System != "" {
		s := req.System
		msgs = append(msgs, oaMsg{Role: "system", Content: &s})
	}
	for _, m := range req.Messages {
		content := m.Content
		om := oaMsg{Role: m.Role, Content: &content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			var t oaToolCall
			t.ID, t.Type = tc.ID, "function"
			t.Function.Name, t.Function.Arguments = tc.Name, tc.Arguments
			om.ToolCalls = append(om.ToolCalls, t)
		}
		msgs = append(msgs, om)
	}
	body := map[string]any{
		"messages":       msgs,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if c.p.Kind != KindAzure {
		body["model"] = req.Model
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
				"name": t.Name, "description": t.Description, "parameters": t.Parameters,
			}})
		}
		body["tools"] = tools
	}
	u, hdr := c.endpoint(req.Model)
	rc, err := post(ctx, c.hc, u, hdr, body)
	if err != nil {
		return err
	}
	defer rc.Close()

	type partial struct{ id, name, args string }
	calls := map[int]*partial{}
	finish := ""
	err = sseData(rc, func(d []byte) error {
		var ch struct {
			Choices []struct {
				Delta struct {
					Content   string       `json:"content"`
					ToolCalls []oaToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				Prompt     int `json:"prompt_tokens"`
				Completion int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(d, &ch) != nil {
			return nil
		}
		if ch.Usage != nil {
			if err := emit(Event{InputTokens: ch.Usage.Prompt, OutputTokens: ch.Usage.Completion}); err != nil {
				return err
			}
		}
		for _, choice := range ch.Choices {
			if choice.Delta.Content != "" {
				if err := emit(Event{Delta: choice.Delta.Content}); err != nil {
					return err
				}
			}
			for i, tc := range choice.Delta.ToolCalls {
				idx := i
				if tc.Index != nil {
					idx = *tc.Index
				}
				p := calls[idx]
				if p == nil {
					p = &partial{}
					calls[idx] = p
				}
				if tc.ID != "" {
					p.id = tc.ID
				}
				p.name += tc.Function.Name
				p.args += tc.Function.Arguments
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finish = *choice.FinishReason
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		p := calls[i]
		if p.id == "" {
			p.id = "call_" + newID()[:8]
		}
		if err := emit(Event{Call: &ToolCall{ID: p.id, Name: p.name, Arguments: p.args}}); err != nil {
			return err
		}
	}
	if finish == "" {
		finish = "stop"
	}
	return emit(Event{Finish: finish})
}
