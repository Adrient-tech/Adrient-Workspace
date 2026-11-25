// Package aifake is a small scripted OpenAI-compatible chat server for tests
// and local browser runs. It does not run any model: replies come from a
// Script. It streams like the real API (SSE chunks, tool_calls deltas, usage).
package aifake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// ToolCall is a scripted tool call.
type ToolCall struct {
	Name      string
	Arguments string
}

// Turn is one scripted model reply.
type Turn struct {
	Text      string
	ToolCalls []ToolCall
	// Status, when set, makes the server fail with that HTTP status.
	Status int
}

// Message is the subset of a chat message the script sees.
type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

// Request is a recorded request.
type Request struct {
	Path          string
	Authorization string
	Model         string
	Messages      []Message
	Tools         []string
	Raw           string
}

// LastUser returns the last user message.
func (r Request) LastUser() string {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == "user" {
			return r.Messages[i].Content
		}
	}
	return ""
}

// System returns the system prompt.
func (r Request) System() string {
	if len(r.Messages) > 0 && r.Messages[0].Role == "system" {
		return r.Messages[0].Content
	}
	return ""
}

// StepsSinceUser counts tool results after the last user message.
func (r Request) StepsSinceUser() int {
	n := 0
	for i := len(r.Messages) - 1; i >= 0 && r.Messages[i].Role != "user"; i-- {
		if r.Messages[i].Role == "assistant" {
			n++
		}
	}
	return n
}

// HasTool reports whether the request declared tool name.
func (r Request) HasTool(name string) bool {
	for _, t := range r.Tools {
		if t == name {
			return true
		}
	}
	return false
}

// Script decides the reply to a request.
type Script func(Request) Turn

// Server is the fake. Key, when set, must match the bearer token.
type Server struct {
	Script Script
	Key    string

	mu   sync.Mutex
	reqs []Request
}

// New returns a server running script (DefaultScript when nil).
func New(script Script, key string) *Server {
	if script == nil {
		script = DefaultScript
	}
	return &Server{Script: script, Key: key}
}

// Requests returns the recorded requests.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.reqs...)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/models") && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"fake-model","object":"model"}]}`))
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
		Tools    []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	var raw strings.Builder
	if err := json.NewDecoder(teeReader(r, &raw)).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"bad json"}}`, 400)
		return
	}
	req := Request{Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Model: body.Model, Messages: body.Messages, Raw: raw.String()}
	for _, t := range body.Tools {
		req.Tools = append(req.Tools, t.Function.Name)
	}
	s.mu.Lock()
	s.reqs = append(s.reqs, req)
	s.mu.Unlock()
	if s.Key != "" && req.Authorization != "Bearer "+s.Key {
		http.Error(w, `{"error":{"message":"invalid api key"}}`, 401)
		return
	}
	turn := s.Script(req)
	if turn.Status != 0 {
		http.Error(w, `{"error":{"message":"scripted failure"}}`, turn.Status)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fl, _ := w.(http.Flusher)
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if fl != nil {
			fl.Flush()
		}
	}
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk", "model": body.Model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
	}
	send(chunk(map[string]any{"role": "assistant"}, nil))
	for _, word := range splitKeep(turn.Text) {
		send(chunk(map[string]any{"content": word}, nil))
	}
	for i, tc := range turn.ToolCalls {
		id := fmt.Sprintf("call_%d_%d", len(req.Messages), i)
		// Split the arguments over two chunks like real servers do.
		half := len(tc.Arguments) / 2
		send(chunk(map[string]any{"tool_calls": []any{map[string]any{"index": i, "id": id, "type": "function",
			"function": map[string]any{"name": tc.Name, "arguments": tc.Arguments[:half]}}}}, nil))
		send(chunk(map[string]any{"tool_calls": []any{map[string]any{"index": i,
			"function": map[string]any{"arguments": tc.Arguments[half:]}}}}, nil))
	}
	finish := "stop"
	if len(turn.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	send(chunk(map[string]any{}, finish))
	in := 0
	for _, m := range body.Messages {
		in += len(m.Content) / 4
	}
	send(map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk", "choices": []any{},
		"usage": map[string]any{"prompt_tokens": in, "completion_tokens": len(turn.Text)/4 + 10*len(turn.ToolCalls)}})
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func splitKeep(s string) []string {
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, ' ')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

type tee struct {
	r   *http.Request
	buf *strings.Builder
}

func (t tee) Read(p []byte) (int, error) {
	n, err := t.r.Body.Read(p)
	t.buf.Write(p[:n])
	return n, err
}

func teeReader(r *http.Request, b *strings.Builder) tee { return tee{r: r, buf: b} }

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// DefaultScript is the demo script used for the browser run:
//   - "budget" in Sheets: read A1:D20, write a monthly budget with formulas,
//     bold the header, then summarise.
//   - "clear" in Sheets: ask to clear A1:D20 (a destructive tool, confirmed in the panel).
//   - Docs "summar...": read the document then answer.
//   - Slides "outline": add slides.
//   - anything else: echo a short reply naming the selection label.
func DefaultScript(r Request) Turn {
	if t, ok := directiveTurn(r); ok {
		return t
	}
	q := strings.ToLower(r.LastUser())
	step := r.StepsSinceUser()
	switch {
	case strings.Contains(q, "budget") && r.HasTool("sheet_write_cells"):
		switch step {
		case 0:
			return Turn{Text: "I'll look at the sheet first. ", ToolCalls: []ToolCall{{Name: "sheet_read_range", Arguments: `{"range":"A1:D20"}`}}}
		case 1:
			return Turn{Text: "Writing the budget. ", ToolCalls: []ToolCall{{Name: "sheet_write_cells", Arguments: mustJSON(map[string]any{
				"start": "A1",
				"values": [][]any{
					{"Monthly budget", "", ""},
					{"Category", "Type", "Amount (INR)"},
					{"Salary", "Income", 85000},
					{"Freelance", "Income", 15000},
					{"Rent", "Expense", 25000},
					{"Groceries", "Expense", 9000},
					{"Utilities", "Expense", 4000},
					{"Transport", "Expense", 3500},
					{"Savings", "Expense", 20000},
					{"Total income", "", `=SUMIF(B3:B9,"Income",C3:C9)`},
					{"Total expenses", "", `=SUMIF(B3:B9,"Expense",C3:C9)`},
					{"Balance", "", "=C10-C11"},
				},
			})}}}
		case 2:
			return Turn{ToolCalls: []ToolCall{{Name: "sheet_format_range", Arguments: `{"range":"A2:C2","bold":true,"background":"#126C5A","color":"#FFFFFF"}`}}}
		default:
			return Turn{Text: "Done. I added a **monthly budget** in A1:C12 with income and expense rows, and totals in C10:C12 (`SUMIF` formulas, so they update when you change amounts). Use **Undo** next to each step to revert it."}
		}
	case strings.Contains(q, "clear") && r.HasTool("sheet_clear_range"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "sheet_clear_range", Arguments: `{"range":"A1:D20"}`}}}
		}
		last := r.Messages[len(r.Messages)-1].Content
		if strings.Contains(last, "declined") {
			return Turn{Text: "OK, I left the cells unchanged."}
		}
		return Turn{Text: "Cleared A1:D20."}
	case strings.Contains(q, "summar") && r.HasTool("doc_read"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "doc_read", Arguments: `{"scope":"document"}`}}}
		}
		return Turn{Text: "Summary: the document is short; its main points are listed above."}
	case strings.Contains(q, "stalled") && r.HasTool("crm_stalled_deals"):
		if step == 0 {
			return Turn{Text: "Checking the pipeline. ", ToolCalls: []ToolCall{{Name: "crm_stalled_deals", Arguments: `{}`}}}
		}
		return Turn{Text: "These deals have sat in their stage too long. **Suggested next step:** call the contact on the oldest one this week and log it."}
	case strings.Contains(q, "log a call") && r.HasTool("crm_log_activity"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "crm_find_deals", Arguments: `{"limit":1}`}}}
		}
		if step == 1 {
			return Turn{ToolCalls: []ToolCall{{Name: "crm_log_activity", Arguments: `{"deal_id":"` + firstID(r) + `","kind":"call","summary":"Called to confirm the proposal (logged by the assistant)"}`}}}
		}
		return Turn{Text: "Done: I logged the call on the deal."}
	case strings.Contains(q, "leave") && r.HasTool("hr_my_leave_balance"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "hr_my_leave_balance", Arguments: `{}`}}}
		}
		return Turn{Text: "Here are your leave balances from your HR record (see the tool result above)."}
	case strings.Contains(q, "spend") && r.HasTool("acct_spend"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "acct_spend", Arguments: `{"topic":"rent"}`}}}
		}
		return Turn{Text: "The figures above come straight from the profit and loss report; I did not calculate anything myself."}
	case strings.Contains(q, "low") && r.HasTool("inv_low_stock"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "inv_low_stock", Arguments: `{}`}}}
		}
		return Turn{Text: "These products are at or below their minimum. I can draft a purchase order for the ones with a supplier."}
	case strings.Contains(q, "summar") && r.HasTool("pos_day_summary"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "pos_day_summary", Arguments: `{}`}}}
		}
		return Turn{Text: "Today's sales summary is above."}
	case strings.Contains(q, "audit") && r.HasTool("admin_audit_summary"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "admin_audit_summary", Arguments: `{}`}}}
		}
		return Turn{Text: "That is the audit digest. I cannot change any setting; apply recommendations in Admin by hand."}
	case strings.Contains(q, "outline") && r.HasTool("slide_add_slides"):
		if step == 0 {
			return Turn{ToolCalls: []ToolCall{{Name: "slide_add_slides", Arguments: `{"slides":[{"title":"Problem","bullets":["Teams juggle many tools"]},{"title":"Solution","bullets":["One open workspace"]}]}`}}}
		}
		return Turn{Text: "Added 2 slides."}
	}
	label := ""
	if s := r.System(); strings.Contains(s, "current selection is ") {
		label = strings.SplitN(strings.SplitN(s, "current selection is ", 2)[1], "\n", 2)[0]
	}
	if label != "" {
		return Turn{Text: "This is the fake test model (no real AI). You asked: \"" + r.LastUser() + "\". Selection: " + strings.TrimSuffix(label, ":") + "."}
	}
	return Turn{Text: "This is the fake test model (no real AI). You asked: \"" + r.LastUser() + "\"."}
}

// directiveTurn runs scripted tool calls written into the user message as
// lines "@@tool NAME {json args}". Step n calls the n-th directive (if the
// request declares that tool); after the last one the reply is a short
// confirmation. It lets browser runs exercise every app's tools without a model.
func directiveTurn(r Request) (Turn, bool) {
	var calls []ToolCall
	for _, line := range strings.Split(r.LastUser(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "@@tool ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "@@tool "))
		name, args, _ := strings.Cut(rest, " ")
		if args == "" {
			args = "{}"
		}
		calls = append(calls, ToolCall{Name: name, Arguments: args})
	}
	if len(calls) == 0 {
		return Turn{}, false
	}
	step := r.StepsSinceUser()
	if step < len(calls) {
		if !r.HasTool(calls[step].Name) {
			return Turn{Text: "Tool " + calls[step].Name + " is not available here."}, true
		}
		return Turn{Text: "Running " + calls[step].Name + ". ", ToolCalls: []ToolCall{calls[step]}}, true
	}
	last := r.Messages[len(r.Messages)-1].Content
	if strings.Contains(last, "declined") {
		return Turn{Text: "OK, I left everything unchanged."}, true
	}
	return Turn{Text: "Done: " + strings.ReplaceAll(last, "\n", " ")}, true
}

// firstID pulls the first "id" out of the last tool result (used by the scripted CRM scenario).
func firstID(r Request) string {
	last := r.Messages[len(r.Messages)-1].Content
	i := strings.Index(last, `"id":"`)
	if i < 0 {
		return ""
	}
	rest := last[i+6:]
	if j := strings.IndexByte(rest, '"'); j >= 0 {
		return rest[:j]
	}
	return ""
}
