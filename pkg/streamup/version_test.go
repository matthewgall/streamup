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
	"runtime"
	"strings"
	"testing"
)

func TestUserAgent(t *testing.T) {
	// Save original values
	origVersion := Version
	origGitCommit := GitCommit

	// Test with default values
	Version = "1.0.0"
	GitCommit = "dev"
	ua := UserAgent()
	expected := "streamup/1.0.0 (" + runtime.GOOS + "; " + runtime.GOARCH + ")"
	if ua != expected {
		t.Errorf("UserAgent() = %q, want %q", ua, expected)
	}

	// Test with git commit
	Version = "1.0.0"
	GitCommit = "abc123"
	ua = UserAgent()
	expected = "streamup/1.0.0 (" + runtime.GOOS + "; " + runtime.GOARCH + ") git-abc123"
	if ua != expected {
		t.Errorf("UserAgent() = %q, want %q", ua, expected)
	}

	// Test with empty git commit
	Version = "1.0.0"
	GitCommit = ""
	ua = UserAgent()
	expected = "streamup/1.0.0 (" + runtime.GOOS + "; " + runtime.GOARCH + ")"
	if ua != expected {
		t.Errorf("UserAgent() = %q, want %q", ua, expected)
	}

	// Restore original values
	Version = origVersion
	GitCommit = origGitCommit
}

func TestVersionString(t *testing.T) {
	// Save original values
	origVersion := Version
	origGitCommit := GitCommit
	origBuildDate := BuildDate

	// Test with dev build
	Version = "1.0.0"
	GitCommit = "dev"
	BuildDate = "unknown"
	vs := VersionString()
	if vs != "1.0.0" {
		t.Errorf("VersionString() = %q, want %q", vs, "1.0.0")
	}

	// Test with full build info
	Version = "1.0.0"
	GitCommit = "abc123"
	BuildDate = "2025-12-05_10:00:00"
	vs = VersionString()
	expected := "1.0.0 (commit abc123, built 2025-12-05_10:00:00)"
	if vs != expected {
		t.Errorf("VersionString() = %q, want %q", vs, expected)
	}

	// Restore original values
	Version = origVersion
	GitCommit = origGitCommit
	BuildDate = origBuildDate
}

func TestUserAgent_Format(t *testing.T) {
	ua := UserAgent()

	// Should start with "streamup/"
	if !strings.HasPrefix(ua, "streamup/") {
		t.Errorf("UserAgent() = %q, should start with 'streamup/'", ua)
	}

	// Should contain OS and architecture
	if !strings.Contains(ua, runtime.GOOS) {
		t.Errorf("UserAgent() = %q, should contain OS %q", ua, runtime.GOOS)
	}
	if !strings.Contains(ua, runtime.GOARCH) {
		t.Errorf("UserAgent() = %q, should contain architecture %q", ua, runtime.GOARCH)
	}
}

func TestUserAgent_RealBuild(t *testing.T) {
	// Test the actual built values (might be "dev" in tests)
	ua := UserAgent()

	// Should have the correct format
	parts := strings.Split(ua, " ")
	if len(parts) < 1 {
		t.Errorf("UserAgent() = %q, invalid format", ua)
	}

	// First part should be "streamup/version"
	if !strings.HasPrefix(parts[0], "streamup/") {
		t.Errorf("UserAgent() first part = %q, should start with 'streamup/'", parts[0])
	}
}
