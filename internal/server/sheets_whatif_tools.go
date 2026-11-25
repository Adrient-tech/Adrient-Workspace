package server

// What-if endpoints (wave 4): data table, scenarios and Solver-lite. Like goal
// seek they read the stored workbook (or the live copy posted in "data"),
// compute on a copy and never write it. Reading the sheet is enough: these
// tools only report numbers, the editor writes any accepted result itself
// through the normal (permission-checked) save path.
//
//	POST /api/v1/sheets/d/{id}/datatable  sheets.DataTableRequest → sheets.DataTableResult
//	POST /api/v1/sheets/d/{id}/scenarios  sheets.ScenarioRequest  → sheets.ScenarioResult
//	POST /api/v1/sheets/d/{id}/solver     sheets.SolverRequest    → sheets.SolverResult

import (
	"errors"
	"net/http"

	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sheets"
)

// whatIfAction returns the sheet id and tool name for a what-if path.
func whatIfAction(path string) (string, string, bool) {
	for _, a := range []string{"datatable", "scenarios", "solver"} {
		if id, ok := sheetsActionID(path, a); ok {
			return id, a, true
		}
	}
	return "", "", false
}

func whatIfStatus(err error) int {
	switch {
	case errors.Is(err, sheets.ErrNotWorkbook), errors.Is(err, sheets.ErrWhatIfInput), errors.Is(err, sheets.ErrWhatIfTooMany),
		errors.Is(err, sheets.ErrGoalSeekChanging), errors.Is(err, sheets.ErrGoalSeekFormula), errors.Is(err, sheets.ErrGoalSeekCell):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func serveSheetsWhatIf(w http.ResponseWriter, r *http.Request, id, action string, repo *sheets.Repository, grants *sharing.Repository) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s, status := readableSheet(r, id, repo, grants)
	if status != 0 {
		http.Error(w, http.StatusText(status), status)
		return
	}
	var (
		out interface{}
		err error
	)
	switch action {
	case "datatable":
		var req sheets.DataTableRequest
		if !readBody(w, r, &req) {
			return
		}
		if req.Data == "" {
			req.Data = s.Data
		}
		out, err = sheets.DataTable(req)
	case "scenarios":
		var req sheets.ScenarioRequest
		if !readBody(w, r, &req) {
			return
		}
		if req.Data == "" {
			req.Data = s.Data
		}
		out, err = sheets.Scenarios(req)
	case "solver":
		var req sheets.SolverRequest
		if !readBody(w, r, &req) {
			return
		}
		if req.Data == "" {
			req.Data = s.Data
		}
		out, err = sheets.Solve(req)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), whatIfStatus(err))
		return
	}
	writeJSON(w, out)
}
