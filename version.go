// Package adrient is the module root. It only carries the product VERSION file
// (the single source of truth for the release number) embedded into the Go
// binaries, so no -ldflags are needed to know the version.
package adrient

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionFile string

// Version returns the release number from the repo-root VERSION file.
func Version() string {
	v := strings.TrimSpace(versionFile)
	if v == "" {
		return "0.0.0-dev"
	}
	return v
}
