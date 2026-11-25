package pagekit

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCursorRoundTrip(t *testing.T) {
	c := EncodeCursor("2026-01-01T00:00:00Z", "abc")
	got, err := DecodeCursor(c)
	if err != nil || len(got) != 2 || got[1] != "abc" {
		t.Fatalf("round trip = %v %v", got, err)
	}
	if _, err := DecodeCursor("not base64 !!"); err == nil {
		t.Fatal("garbage cursor must fail")
	}
}

func TestParseCapsAndSort(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?limit=99999&sort=evil&order=desc&q=%20ana%20&page=3&f=1", nil)
	p := Parse(r, 50, 200, []string{"name", "created"}, "f")
	if p.Limit != 200 || p.Sort != "name" || !p.Desc || p.Q != "ana" || p.Page != 3 || p.Filter["f"] != "1" {
		t.Fatalf("parsed %+v", p)
	}
	if p.Offset() != 400 {
		t.Fatalf("offset %d", p.Offset())
	}
	if q := Parse(httptest.NewRequest("GET", "/x?limit=-4", nil), 50, 200, nil); q.Limit != 50 {
		t.Fatalf("negative limit gave %d", q.Limit)
	}
}

func TestCSVNeutralisesFormulas(t *testing.T) {
	w := httptest.NewRecorder()
	c := NewCSV(w, `a"b.csv`, []string{"h"})
	c.Row("=SUM(A1)")
	c.Row("ok")
	c.Close()
	body := w.Body.String()
	if !strings.Contains(body, "'=SUM(A1)") || strings.Contains(w.Header().Get("Content-Disposition"), `a"b`) {
		t.Fatalf("csv %q header %q", body, w.Header().Get("Content-Disposition"))
	}
}
