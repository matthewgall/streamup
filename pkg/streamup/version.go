// Copyright 2025 Matthew Gall <me@matthewgall.dev>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
