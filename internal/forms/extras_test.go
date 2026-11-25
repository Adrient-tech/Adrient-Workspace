package forms

import (
	"strings"
	"testing"
)

func TestExtrasNormalize(t *testing.T) {
	e := Extras{ThemeColor: " #126C5A ", ThemeBG: "#F0F4F3"}
	if err := e.Normalize(); err != nil || e.ThemeColor != "#126C5A" {
		t.Fatalf("%v %+v", err, e)
	}
	if err := (&Extras{}).Normalize(); err != nil {
		t.Fatal("empty is the default look")
	}
	for _, bad := range []Extras{{ThemeColor: "red"}, {ThemeColor: "#12"}, {ThemeBG: "url(javascript:1)"}, {ThemeColor: "#GGGGGG"}} {
		if bad.Normalize() == nil {
			t.Fatalf("%+v should fail", bad)
		}
	}
}

func TestCleanEmails(t *testing.T) {
	got, err := CleanEmails([]string{" A@x.com", "a@x.com", "", "b@y.org"})
	if err != nil || len(got) != 2 || got[0] != "a@x.com" {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range [][]string{{"nope"}, {"Name <a@x.com>"}, {"a@x.com\nbcc:evil@x.com"}} {
		if _, err := CleanEmails(bad); err == nil {
			t.Fatalf("%v should fail", bad)
		}
	}
	many := make([]string, MaxSendRecipients+1)
	for i := range many {
		many[i] = "u" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26)) + "@x.com"
	}
	if _, err := CleanEmails(many); err == nil {
		t.Fatal("over the cap must fail")
	}
}

func TestInviteMessageEscapes(t *testing.T) {
	sub, text, htm := InviteMessage(`Survey <script>`, "Ann", "Hi\n<b>all</b>", `https://x.test/forms/d/1/viewform`)
	if !strings.Contains(sub, "Ann invited you") || !strings.Contains(text, "https://x.test/forms/d/1/viewform") {
		t.Fatal(sub, text)
	}
	if strings.Contains(htm, "<script>") || strings.Contains(htm, "<b>all") || !strings.Contains(htm, "&lt;b&gt;all") {
		t.Fatal("html not escaped: " + htm)
	}
}
