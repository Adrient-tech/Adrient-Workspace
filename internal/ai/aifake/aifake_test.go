package aifake

import "testing"

func TestDirectiveCallsToolsInOrder(t *testing.T) {
	user := Message{Role: "user", Content: "do it\n@@tool a_one {\"x\":1}\n@@tool b_two"}
	r := Request{Messages: []Message{user}, Tools: []string{"a_one", "b_two"}}
	if got := DefaultScript(r); len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "a_one" || got.ToolCalls[0].Arguments != `{"x":1}` {
		t.Fatalf("step0: %+v", got)
	}
	r.Messages = append(r.Messages, Message{Role: "assistant"}, Message{Role: "tool", Content: "ok1"})
	if got := DefaultScript(r); len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "b_two" || got.ToolCalls[0].Arguments != "{}" {
		t.Fatalf("step1: %+v", got)
	}
	r.Messages = append(r.Messages, Message{Role: "assistant"}, Message{Role: "tool", Content: "ok2"})
	if got := DefaultScript(r); len(got.ToolCalls) != 0 || got.Text != "Done: ok2" {
		t.Fatalf("final: %+v", got)
	}
	r2 := Request{Messages: []Message{user}, Tools: []string{"b_two"}}
	if got := DefaultScript(r2); len(got.ToolCalls) != 0 {
		t.Fatalf("undeclared tool must not be called: %+v", got)
	}
}
