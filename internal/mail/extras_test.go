package mail

import (
	"strings"
	"testing"
)

func TestValidateSearch(t *testing.T) {
	if _, _, err := ValidateSearch(" ", "from:a"); err == nil {
		t.Fatal("empty name must fail")
	}
	if _, _, err := ValidateSearch("x", " "); err == nil {
		t.Fatal("empty query must fail")
	}
	if _, _, err := ValidateSearch(strings.Repeat("a", 81), "q"); err == nil {
		t.Fatal("long name must fail")
	}
	n, q, err := ValidateSearch(" Boss ", " from:boss has:attachment ")
	if err != nil || n != "Boss" || q != "from:boss has:attachment" {
		t.Fatalf("got %q %q %v", n, q, err)
	}
}

func TestExtrasMatch(t *testing.T) {
	e := &Extras{}
	for _, p := range []string{"/api/v1/mail/saved-searches", "/api/v1/mail/signatures/", "/api/v1/mail/signatures/abc"} {
		if !e.Match(p) {
			t.Errorf("%s should match", p)
		}
	}
	if e.Match("/api/v1/mail/messages") {
		t.Error("messages must not match")
	}
}
