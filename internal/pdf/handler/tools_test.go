package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestToolsRemoveContentValidatesInput(t *testing.T) {
	h := NewToolsHandler()
	cases := []struct {
		name, regions, body string
		want                int
	}{
		{"no regions", "", "%PDF-1.7", http.StatusBadRequest},
		{"empty list", "[]", "%PDF-1.7", http.StatusBadRequest},
		{"not a pdf", `[{"page":1,"x":0,"y":0,"w":1,"h":1}]`, "hello", http.StatusBadRequest},
		{"broken pdf", `[{"page":1,"x":0,"y":0,"w":1,"h":1}]`, "%PDF-1.7 garbage", http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/api/tools/remove-content?regions="+url.QueryEscape(c.regions), strings.NewReader(c.body))
		rec := httptest.NewRecorder()
		h.RemoveContent(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.want)
		}
	}
}

func TestToolsCompressRejectsNonPDF(t *testing.T) {
	rec := httptest.NewRecorder()
	NewToolsHandler().Compress(rec, httptest.NewRequest("POST", "/api/tools/compress", strings.NewReader("nope")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestSignaturesRequireUser(t *testing.T) {
	h := NewSignaturesHandler(nil)
	for _, f := range []http.HandlerFunc{h.List, h.Create, h.Delete} {
		rec := httptest.NewRecorder()
		f(rec, httptest.NewRequest("GET", "/api/user/signatures", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", rec.Code)
		}
	}
}

func TestToolsPageFontsValidatesPage(t *testing.T) {
	rec := httptest.NewRecorder()
	NewToolsHandler().PageFonts(rec, httptest.NewRequest("POST", "/api/tools/page-fonts?page=0", strings.NewReader("%PDF-1.7")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}
