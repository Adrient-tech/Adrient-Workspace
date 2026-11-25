package ai

import (
	"context"
	"encoding/json"
)

// anthropicClient speaks the Anthropic Messages API with streaming.
type anthropicClient struct {
	p   Provider
	key string
	hc  httpDoer
}

func (c *anthropicClient) Stream(ctx context.Context, req CompletionRequest, emit func(Event) error) error {
	type block = map[string]any
	var msgs []map[string]any
	add := func(role string, b block) {
		if n := len(msgs); n > 0 && msgs[n-1]["role"] == role {
			msgs[n-1]["content"] = append(msgs[n-1]["content"].([]block), b)
			return
		}
		msgs = append(msgs, map[string]any{"role": role, "content": []block{b}})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			add("user", block{"type": "tool_result", "tool_use_id": m.ToolCallID, "content": m.Content})
		case "assistant":
			if m.Content != "" {
				add("assistant", block{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				var in any = map[string]any{}
				if tc.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Arguments), &in)
				}
				add("assistant", block{"type": "tool_use", "id": tc.ID, "name": tc.Name, "input": in})
			}
		default:
			add("user", block{"type": "text", "text": m.Content})
		}
	}
	max := req.MaxTokens
	if max <= 0 {
		max = 2048
	}
	body := map[string]any{"model": req.Model, "max_tokens": max, "stream": true, "messages": msgs}
	if req.System != "" {
		body["system"] = req.System
	}
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Parameters})
		}
		body["tools"] = tools
	}
	rc, err := post(ctx, c.hc, c.p.BaseURL+"/v1/messages", map[string]string{
		"x-api-key": c.key, "anthropic-version": "2023-06-01",
	}, body)
	if err != nil {
		return err
	}
	defer rc.Close()

	type pending struct{ id, name, args string }
	open := map[int]*pending{}
	var order []*pending
	finish := "stop"
	err = sseData(rc, func(d []byte) error {
		var ev struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Message struct {
				Usage struct {
					In int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				Out int `json:"output_tokens"`
			} `json:"usage"`
			Error struct {
				Type string `json:"type"`
			} `json:"error"`
		}
		if json.Unmarshal(d, &ev) != nil {
			return nil
		}
		switch ev.Type {
		case "message_start":
			return emit(Event{InputTokens: ev.Message.Usage.In})
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				p := &pending{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				open[ev.Index] = p
				order = append(order, p)
			}
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				return emit(Event{Delta: ev.Delta.Text})
			}
			if ev.Delta.Type == "input_json_delta" {
				if p := open[ev.Index]; p != nil {
					p.args += ev.Delta.PartialJSON
				}
			}
		case "message_delta":
			if ev.Delta.StopReason == "tool_use" {
				finish = "tool_calls"
			} else if ev.Delta.StopReason != "" {
				finish = ev.Delta.StopReason
			}
			return emit(Event{OutputTokens: ev.Usage.Out})
		case "error":
			return &ProviderError{Msg: "stream error (" + ev.Error.Type + ")"}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, p := range order {
		args := p.args
		if args == "" {
			args = "{}"
		}
		if err := emit(Event{Call: &ToolCall{ID: p.id, Name: p.name, Arguments: args}}); err != nil {
			return err
		}
	}
	return emit(Event{Finish: finish})
}
