package chat

import "testing"

func TestTypingTargets(t *testing.T) {
	got := typingTargets([]string{"a", "b", "c"}, "b")
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("got %v", got)
	}
	if got := typingTargets(nil, "x"); got == nil || len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestCanPost(t *testing.T) {
	ch := TChannel{Announce: true, CreatedBy: "boss"}
	if !CanPost(ch, "boss", false) || CanPost(ch, "x", false) || !CanPost(ch, "x", true) {
		t.Fatal("announce rules")
	}
	if !CanPost(TChannel{CreatedBy: "boss"}, "x", false) {
		t.Fatal("normal channel")
	}
}
