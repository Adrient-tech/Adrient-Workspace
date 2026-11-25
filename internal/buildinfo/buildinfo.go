// Package buildinfo reports the product identity shown in the About dialog,
// GET /api/v1/version and `adrient-admin version`.
//
// The version comes from the embedded repo-root VERSION file. A -ldflags
// override (main.version) still wins when it is not the "0.0.0-dev" default.
// Commit and build date come from -ldflags or, failing that, the VCS data Go
// stamps into the binary.
package buildinfo

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"strings"

	adrient "github.com/Adrient-tech/Adrient-Workspace"
)

const (
	ProductName = "Adrient Workspace"
	Website     = "https://adrient.com"
	SupportMail = "info@adrient.com"
	Licence     = "MIT"
)

// Info is the JSON shape of GET /api/v1/version.
type Info struct {
	Product   string `json:"product"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	Website   string `json:"website"`
	Support   string `json:"support_email"`
	Licence   string `json:"licence"`
}

// Resolve merges the ldflags values with the embedded VERSION and VCS data.
func Resolve(ldVersion, ldCommit, ldDate string) Info {
	v := ldVersion
	if v == "" || v == "0.0.0-dev" {
		v = adrient.Version()
	}
	commit, date := ldCommit, ldDate
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if commit == "" || commit == "unknown" {
					commit = s.Value
				}
			case "vcs.time":
				if date == "" {
					date = s.Value
				}
			}
		}
	}
	if commit == "" {
		commit = "unknown"
	}
	if len(commit) > 7 && !strings.Contains(commit, " ") {
		commit = commit[:7]
	}
	if date == "" {
		date = "unknown"
	}
	return Info{ProductName, v, commit, date, Website, SupportMail, Licence}
}

// Handler serves the info as JSON. It is public: version numbers are not secret
// and the sign-in screen shows them.
func Handler(i Info) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		_ = json.NewEncoder(w).Encode(i)
	})
}
