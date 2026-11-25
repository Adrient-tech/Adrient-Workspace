package keep

import (
	"bytes"
	"testing"
	"time"
)

func TestSniffImage(t *testing.T) {
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 16)...)
	jpg := append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 16)...)
	gif := []byte("GIF89a......")
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 8)...)
	for name, b := range map[string][]byte{"png": png, "jpeg": jpg, "gif": gif, "webp": webp} {
		if SniffImage(b) == "" {
			t.Errorf("%s should be allowed", name)
		}
	}
	for name, b := range map[string][]byte{
		"svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"html": []byte("<html><script>alert(1)</script></html>"),
		"text": []byte("hello world"),
		"exe":  append([]byte("MZ"), make([]byte, 32)...),
		"empty": {},
	} {
		if m := SniffImage(b); m != "" {
			t.Errorf("%s must be refused, got %s", name, m)
		}
	}
	// A script body behind a PNG name is still refused: only content decides.
	if SniffImage(bytes.Repeat([]byte("<script>"), 4)) != "" {
		t.Error("script content must be refused")
	}
}

func TestNotesDue(t *testing.T) {
	now := time.Date(2030, 1, 7, 10, 0, 0, 0, time.UTC)
	rows := []dueNote{
		{ID: "now", At: now},
		{ID: "recent", At: now.Add(-20 * time.Minute)},
		{ID: "old", At: now.Add(-45 * time.Minute)},
		{ID: "future", At: now.Add(time.Minute)},
	}
	got := map[string]bool{}
	for _, n := range notesDue(rows, now, 30*time.Minute) {
		got[n.ID] = true
	}
	if len(got) != 2 || !got["now"] || !got["recent"] {
		t.Fatalf("got %v", got)
	}
}

func TestFirstLine(t *testing.T) {
	if firstLine("\n  \n Buy milk \nmore") != "Buy milk" {
		t.Fatal("first non-empty line")
	}
	if len([]rune(firstLine(string(bytes.Repeat([]byte("x"), 200))))) != 80 {
		t.Fatal("must cap at 80")
	}
	if firstLine("  ") != "" {
		t.Fatal("blank gives empty")
	}
}
