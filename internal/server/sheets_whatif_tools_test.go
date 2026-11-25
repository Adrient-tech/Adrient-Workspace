package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/sheets"
)

func TestWhatIfAction(t *testing.T) {
	for _, tc := range []struct{ path, id, action string }{
		{"/api/v1/sheets/d/abc/datatable", "abc", "datatable"},
		{"/api/v1/sheets/d/abc/scenarios", "abc", "scenarios"},
		{"/api/v1/sheets/d/abc/solver", "abc", "solver"},
		{"/api/v1/sheets/d/abc/goalseek", "", ""},
		{"/api/v1/sheets/d//solver", "", ""},
	} {
		id, a, ok := whatIfAction(tc.path)
		if ok != (tc.id != "") || id != tc.id || a != tc.action {
			t.Errorf("whatIfAction(%q) = %q %q %v", tc.path, id, a, ok)
		}
	}
}

func TestWhatIfStatus(t *testing.T) {
	if whatIfStatus(sheets.ErrWhatIfInput) != http.StatusBadRequest || whatIfStatus(sheets.ErrNotWorkbook) != http.StatusBadRequest {
		t.Fatal("input errors must be 400")
	}
	if whatIfStatus(errors.New("boom")) != http.StatusInternalServerError {
		t.Fatal("other errors must be 500")
	}
}

func TestWhatIfRejectsGetAndAnonymous(t *testing.T) {
	rec := httptest.NewRecorder()
	serveSheetsWhatIf(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sheets/d/x/solver", nil), "x", "solver", nil, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	serveSheetsWhatIf(rec, httptest.NewRequest(http.MethodPost, "/api/v1/sheets/d/x/solver", nil), "x", "solver", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous POST = %d", rec.Code)
	}
}
