package streamup

import (
	"fmt"
	"runtime"
)

// Version information - can be overridden at build time using ldflags
var (
	// Version is the semantic version number
	Version = "1.0.0"

	// GitCommit is the git commit hash (injected at build time)
	GitCommit = "dev"

	// BuildDate is the build timestamp (injected at build time)
	BuildDate = "unknown"
)

// UserAgent returns the HTTP User-Agent string for streamup.
// Format: streamup/version (OS; Arch) git-commit
func UserAgent() string {
	agent := fmt.Sprintf("streamup/%s (%s; %s)", Version, runtime.GOOS, runtime.GOARCH)

	if GitCommit != "" && GitCommit != "dev" {
		agent += fmt.Sprintf(" git-%s", GitCommit)
	}

	return agent
}

// VersionString returns a human-readable version string.
func VersionString() string {
	if GitCommit != "" && GitCommit != "dev" {
		return fmt.Sprintf("%s (commit %s, built %s)", Version, GitCommit, BuildDate)
	}
	return Version
}
