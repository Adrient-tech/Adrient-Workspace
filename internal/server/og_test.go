package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOGForRoutes(t *testing.T) {
	cases := []struct{ path, site, wantTitle, wantImg string }{
		{"/", "Adrient", "Adrient", defaultOGImage},
		{"/", "Adrient Platform", "Adrient Platform", defaultOGImage},
		{"/games", "Adrient", "Games", "/games-og.png"},
		{"/games/snake.html", "Adrient", "Games", "/games-og.png"},
		{"/drive", "Adrient", "Drive", "/adrient-og.png"},
		{"/unknown-route", "Adrient", "Adrient", defaultOGImage},
	}
	for _, c := range cases {
		gotTitle, gotImg := ogFor(c.path, c.site)
		if gotTitle != c.wantTitle || gotImg != c.wantImg {
			t.Errorf("ogFor(%q,%q) = (%q,%q), want (%q,%q)",
				c.path, c.site, gotTitle, gotImg, c.wantTitle, c.wantImg)
		}
	}
}

func TestInjectOGMeta(t *testing.T) {
	shell := []byte("<!doctype html><html><head><title>Workspace</title></head><body></body></html>")

	// Homepage on workspace.adrient.com uses the configured site name as the og:title.
	r := httptest.NewRequest("GET", "https://workspace.adrient.com/", nil)
	out := string(injectOGMeta(shell, r, "Adrient Platform"))
	for _, want := range []string{
		`<title>Adrient Platform</title>`,
		`property="og:title" content="Adrient Platform"`,
		`property="og:site_name" content="Adrient Platform"`,
		`content="Your own workspace. Your data. Your infrastructure."`,
		`property="og:url" content="https://workspace.adrient.com/"`,
		`property="og:image" content="https://workspace.adrient.com/adrient-og.png"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("homepage injection missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<title>Workspace</title>") {
		t.Error("original <title> not replaced")
	}
	// A shell carrying static og:* defaults must not end up with duplicates.
	shellWithDefaults := []byte(`<html><head><title>Adrient</title>` +
		`<meta property="og:title" content="Adrient" />` +
		`<meta name="twitter:card" content="summary_large_image" /></head></html>`)
	out3 := string(injectOGMeta(shellWithDefaults, r, "Adrient"))
	if strings.Count(out3, `property="og:title"`) != 1 {
		t.Errorf("expected exactly one og:title, got %d:\n%s",
			strings.Count(out3, `property="og:title"`), out3)
	}

	// /games gets the Games title + game image, with the site appended.
	r2 := httptest.NewRequest("GET", "https://adrient.com/games", nil)
	out2 := string(injectOGMeta(shell, r2, "Adrient"))
	for _, want := range []string{
		`<title>Games · Adrient</title>`,
		`property="og:title" content="Games · Adrient"`,
		`property="og:image" content="https://adrient.com/games-og.png"`,
	} {
		if !strings.Contains(out2, want) {
			t.Errorf("/games injection missing %q in:\n%s", want, out2)
		}
	}
}
