package buildinfo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestResolveUsesVersionFile(t *testing.T) {
	raw, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(raw))
	if got := Resolve("0.0.0-dev", "", "").Version; got != want {
		t.Fatalf("version = %q, want %q (VERSION file)", got, want)
	}
	if got := Resolve("9.9.9", "abcdef123456", "2026-01-02").Version; got != "9.9.9" {
		t.Fatalf("ldflags override ignored: %q", got)
	}
	if c := Resolve("", "abcdef123456", "d").Commit; c != "abcdef1" {
		t.Fatalf("commit = %q", c)
	}
}

func TestHandler(t *testing.T) {
	h := Handler(Resolve("", "abc1234", "2026-11-21"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var i Info
	if err := json.Unmarshal(rec.Body.Bytes(), &i); err != nil {
		t.Fatal(err)
	}
	if i.Product != "Adrient Workspace" || i.Website != "https://adrient.com" || i.Support != "info@adrient.com" || i.Licence != "MIT" || i.Version == "" || i.Commit != "abc1234" {
		t.Fatalf("unexpected: %+v", i)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/version", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status %d", rec.Code)
	}
}

func TestChangelogMatchesVersion(t *testing.T) {
	raw, _ := os.ReadFile("../../VERSION")
	cl, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	want := "## " + strings.TrimSpace(string(raw))
	for _, l := range strings.Split(string(cl), "\n") {
		if strings.HasPrefix(l, "## ") {
			if strings.TrimSpace(l) != want {
				t.Fatalf("top CHANGELOG entry %q != %q", l, want)
			}
			return
		}
	}
	t.Fatal("no changelog entry")
}
