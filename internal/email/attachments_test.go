package email

import (
	"strings"
	"testing"
)

func TestBuildMIMEWithAttachment(t *testing.T) {
	m := Message{To: []string{"a@b.c"}, Subject: "PO", Text: "see attached", HTML: "<p>see attached</p>",
		Attachments: []Attachment{{Filename: "PO-0001.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4 test")}}}
	got := string(buildMIME("Adrient <noreply@adrient.com>", m))
	for _, want := range []string{"multipart/mixed", "multipart/alternative", "filename=\"PO-0001.pdf\"", "Content-Transfer-Encoding: base64", "JVBERi0xLjQgdGVzdA=="} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "--\r\n") {
		t.Fatal("not terminated")
	}
	plain := string(buildMIME("x@y.z", Message{To: []string{"a@b.c"}, Subject: "s", Text: "t"}))
	if strings.Contains(plain, "multipart/mixed") {
		t.Fatal("wrapped a message without attachments")
	}
}
