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
	"errors"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/aws/smithy-go"
)

// Mock API error type for testing
type mockAPIError struct {
	code string
}

func (e *mockAPIError) ErrorCode() string    { return e.code }
func (e *mockAPIError) ErrorMessage() string { return "mock error" }
func (e *mockAPIError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultUnknown
}
func (e *mockAPIError) Error() string { return e.code + ": " + e.ErrorMessage() }

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		retryable bool
	}{
		{
			name:      "Nil error",
			err:       nil,
			retryable: false,
		},
		{
			name:      "Network timeout error",
			err:       &net.DNSError{IsTimeout: true},
			retryable: true,
		},
		{
			name:      "Connection refused",
			err:       syscall.ECONNREFUSED,
			retryable: true,
		},
		{
			name:      "API InternalError",
			err:       &mockAPIError{code: "InternalError"},
			retryable: true,
		},
		{
			name:      "API ServiceUnavailable",
			err:       &mockAPIError{code: "ServiceUnavailable"},
			retryable: true,
		},
		{
			name:      "API SlowDown (throttling)",
			err:       &mockAPIError{code: "SlowDown"},
			retryable: true,
		},
		{
			name:      "API RequestTimeout",
			err:       &mockAPIError{code: "RequestTimeout"},
			retryable: true,
		},
		{
			name:      "Generic 5xx error",
			err:       &mockAPIError{code: "500InternalServerError"},
			retryable: true,
		},
		{
			name:      "Generic error (conservative approach)",
			err:       errors.New("unknown error"),
			retryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			if got != tt.retryable {
				t.Errorf("isRetryableError() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name            string
		retryDelay      int
		maxRetryDelay   int
		retryMultiplier int
		attempt         int
		wantMin         time.Duration
		wantMax         time.Duration
	}{
		{
			name:            "First retry - 1s base delay, 2x multiplier",
			retryDelay:      1000,
			maxRetryDelay:   30000,
			retryMultiplier: 2,
			attempt:         0,
			wantMin:         1000 * time.Millisecond,
			wantMax:         1000 * time.Millisecond,
		},
		{
			name:            "Second retry - 2s delay",
			retryDelay:      1000,
			maxRetryDelay:   30000,
			retryMultiplier: 2,
			attempt:         1,
			wantMin:         2000 * time.Millisecond,
			wantMax:         2000 * time.Millisecond,
		},
		{
			name:            "Third retry - 4s delay",
			retryDelay:      1000,
			maxRetryDelay:   30000,
			retryMultiplier: 2,
			attempt:         2,
			wantMin:         4000 * time.Millisecond,
			wantMax:         4000 * time.Millisecond,
		},
		{
			name:            "Fourth retry - 8s delay",
			retryDelay:      1000,
			maxRetryDelay:   30000,
			retryMultiplier: 2,
			attempt:         3,
			wantMin:         8000 * time.Millisecond,
			wantMax:         8000 * time.Millisecond,
		},
		{
			name:            "Very high attempt - capped at max delay",
			retryDelay:      1000,
			maxRetryDelay:   10000,
			retryMultiplier: 2,
			attempt:         10,
			wantMin:         10000 * time.Millisecond,
			wantMax:         10000 * time.Millisecond,
		},
		{
			name:            "Custom initial delay - 500ms",
			retryDelay:      500,
			maxRetryDelay:   30000,
			retryMultiplier: 2,
			attempt:         0,
			wantMin:         500 * time.Millisecond,
			wantMax:         500 * time.Millisecond,
		},
		{
			name:            "Custom multiplier - 3x",
			retryDelay:      1000,
			maxRetryDelay:   30000,
			retryMultiplier: 3,
			attempt:         2,
			wantMin:         9000 * time.Millisecond, // 1000 * 3^2 = 9000
			wantMax:         9000 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				AccessKeyID:     "test",
				SecretAccessKey: "test",
				Bucket:          "test",
				Key:             "test",
				FileSize:        1000,
				RetryDelay:      tt.retryDelay,
				MaxRetryDelay:   tt.maxRetryDelay,
				RetryMultiplier: tt.retryMultiplier,
			}

			uploader, err := New(cfg)
			if err != nil {
				t.Fatalf("Failed to create uploader: %v", err)
			}

			backoff := uploader.calculateBackoff(tt.attempt)

			if backoff < tt.wantMin || backoff > tt.wantMax {
				t.Errorf("calculateBackoff() = %v, want between %v and %v", backoff, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestRetryConfigDefaults(t *testing.T) {
	cfg := Config{
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		Bucket:          "test",
		Key:             "test",
		FileSize:        1000,
		// Don't set retry values - should get defaults
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}

	// Check defaults were applied
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.RetryDelay != 1000 {
		t.Errorf("RetryDelay = %d, want 1000", cfg.RetryDelay)
	}
	if cfg.MaxRetryDelay != 30000 {
		t.Errorf("MaxRetryDelay = %d, want 30000", cfg.MaxRetryDelay)
	}
	if cfg.RetryMultiplier != 2 {
		t.Errorf("RetryMultiplier = %d, want 2", cfg.RetryMultiplier)
	}
}

func TestRetryConfigCustom(t *testing.T) {
	cfg := Config{
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		Bucket:          "test",
		Key:             "test",
		FileSize:        1000,
		MaxRetries:      5,
		RetryDelay:      500,
		MaxRetryDelay:   60000,
		RetryMultiplier: 3,
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}

	// Check custom values were preserved
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
	if cfg.RetryDelay != 500 {
		t.Errorf("RetryDelay = %d, want 500", cfg.RetryDelay)
	}
	if cfg.MaxRetryDelay != 60000 {
		t.Errorf("MaxRetryDelay = %d, want 60000", cfg.MaxRetryDelay)
	}
	if cfg.RetryMultiplier != 3 {
		t.Errorf("RetryMultiplier = %d, want 3", cfg.RetryMultiplier)
	}
}

func TestBackoffProgression(t *testing.T) {
	cfg := Config{
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		Bucket:          "test",
		Key:             "test",
		FileSize:        1000,
		RetryDelay:      1000,  // 1 second
		MaxRetryDelay:   30000, // 30 seconds
		RetryMultiplier: 2,
	}

	uploader, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create uploader: %v", err)
	}

	expectedBackoffs := []time.Duration{
		1 * time.Second,  // 1s * 2^0
		2 * time.Second,  // 1s * 2^1
		4 * time.Second,  // 1s * 2^2
		8 * time.Second,  // 1s * 2^3
		16 * time.Second, // 1s * 2^4
		30 * time.Second, // capped at max
		30 * time.Second, // capped at max
	}

	for attempt, expected := range expectedBackoffs {
		backoff := uploader.calculateBackoff(attempt)
		if backoff != expected {
			t.Errorf("Attempt %d: calculateBackoff() = %v, want %v", attempt, backoff, expected)
		}
	}
}
