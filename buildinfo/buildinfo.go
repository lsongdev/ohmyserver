// Package buildinfo exposes values injected by the release build.
package buildinfo

import "strings"

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// ShortCommit returns a compact commit identifier for display.
func ShortCommit() string {
	if len(Commit) > 12 {
		return Commit[:12]
	}
	return Commit
}

// IsDevelopment reports whether this binary was built without release metadata.
func IsDevelopment() bool {
	return strings.TrimSpace(Version) == "" || Version == "dev"
}
