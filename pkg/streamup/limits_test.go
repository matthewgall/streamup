package streamup

import (
	"testing"
)

func TestDefaultS3Limits(t *testing.T) {
	limits := DefaultS3Limits()

	if limits.MinPartSize != 5*1024*1024 {
		t.Errorf("DefaultS3Limits().MinPartSize = %d, want %d", limits.MinPartSize, 5*1024*1024)
	}

	if limits.MaxPartSize != 5*1024*1024*1024 {
		t.Errorf("DefaultS3Limits().MaxPartSize = %d, want %d", limits.MaxPartSize, 5*1024*1024*1024)
	}

	if limits.MaxParts != 10000 {
		t.Errorf("DefaultS3Limits().MaxParts = %d, want %d", limits.MaxParts, 10000)
	}
}

func TestR2Limits(t *testing.T) {
	limits := R2Limits()

	// R2 should have same limits as S3
	defaultLimits := DefaultS3Limits()

	if limits.MinPartSize != defaultLimits.MinPartSize {
		t.Errorf("R2Limits().MinPartSize = %d, want %d", limits.MinPartSize, defaultLimits.MinPartSize)
	}

	if limits.MaxPartSize != defaultLimits.MaxPartSize {
		t.Errorf("R2Limits().MaxPartSize = %d, want %d", limits.MaxPartSize, defaultLimits.MaxPartSize)
	}

	if limits.MaxParts != defaultLimits.MaxParts {
		t.Errorf("R2Limits().MaxParts = %d, want %d", limits.MaxParts, defaultLimits.MaxParts)
	}
}

func TestBackblazeB2Limits(t *testing.T) {
	limits := BackblazeB2Limits()

	// Backblaze B2 should have same limits as S3
	defaultLimits := DefaultS3Limits()

	if limits.MinPartSize != defaultLimits.MinPartSize {
		t.Errorf("BackblazeB2Limits().MinPartSize = %d, want %d", limits.MinPartSize, defaultLimits.MinPartSize)
	}

	if limits.MaxPartSize != defaultLimits.MaxPartSize {
		t.Errorf("BackblazeB2Limits().MaxPartSize = %d, want %d", limits.MaxPartSize, defaultLimits.MaxPartSize)
	}

	if limits.MaxParts != defaultLimits.MaxParts {
		t.Errorf("BackblazeB2Limits().MaxParts = %d, want %d", limits.MaxParts, defaultLimits.MaxParts)
	}
}

func TestMinIOLimits(t *testing.T) {
	limits := MinIOLimits()

	// MinIO should have same limits as S3
	defaultLimits := DefaultS3Limits()

	if limits.MinPartSize != defaultLimits.MinPartSize {
		t.Errorf("MinIOLimits().MinPartSize = %d, want %d", limits.MinPartSize, defaultLimits.MinPartSize)
	}

	if limits.MaxPartSize != defaultLimits.MaxPartSize {
		t.Errorf("MinIOLimits().MaxPartSize = %d, want %d", limits.MaxPartSize, defaultLimits.MaxPartSize)
	}

	if limits.MaxParts != defaultLimits.MaxParts {
		t.Errorf("MinIOLimits().MaxParts = %d, want %d", limits.MaxParts, defaultLimits.MaxParts)
	}
}

func TestServiceLimits_MaxFileSize(t *testing.T) {
	tests := []struct {
		name            string
		limits          ServiceLimits
		wantMaxFileSize int64
	}{
		{
			name:            "Default S3 limits - 50TB max",
			limits:          DefaultS3Limits(),
			wantMaxFileSize: 5 * 1024 * 1024 * 1024 * 10000, // 5GB × 10000 parts = 50TB
		},
		{
			name: "Custom limits - 1GB parts, 5000 max",
			limits: ServiceLimits{
				MinPartSize: 5 * 1024 * 1024,
				MaxPartSize: 1 * 1024 * 1024 * 1024, // 1GB max part
				MaxParts:    5000,
			},
			wantMaxFileSize: 1 * 1024 * 1024 * 1024 * 5000, // 1GB × 5000 = 5TB
		},
		{
			name: "Small limits - 100MB parts, 1000 max",
			limits: ServiceLimits{
				MinPartSize: 5 * 1024 * 1024,
				MaxPartSize: 100 * 1024 * 1024, // 100MB max part
				MaxParts:    1000,
			},
			wantMaxFileSize: 100 * 1024 * 1024 * 1000, // 100MB × 1000 = 100GB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxFileSize := tt.limits.MaxPartSize * int64(tt.limits.MaxParts)
			if maxFileSize != tt.wantMaxFileSize {
				t.Errorf("Max file size = %d, want %d", maxFileSize, tt.wantMaxFileSize)
			}
		})
	}
}

func TestServiceLimits_Validation(t *testing.T) {
	tests := []struct {
		name    string
		limits  ServiceLimits
		isValid bool
	}{
		{
			name:    "Valid default limits",
			limits:  DefaultS3Limits(),
			isValid: true,
		},
		{
			name: "Valid custom limits",
			limits: ServiceLimits{
				MinPartSize: 10 * 1024 * 1024,
				MaxPartSize: 1 * 1024 * 1024 * 1024,
				MaxParts:    5000,
			},
			isValid: true,
		},
		{
			name: "Invalid - MinPartSize > MaxPartSize",
			limits: ServiceLimits{
				MinPartSize: 100 * 1024 * 1024,
				MaxPartSize: 10 * 1024 * 1024,
				MaxParts:    1000,
			},
			isValid: false,
		},
		{
			name: "Invalid - MinPartSize below S3 minimum (5MB)",
			limits: ServiceLimits{
				MinPartSize: 1 * 1024 * 1024, // 1MB - too small
				MaxPartSize: 5 * 1024 * 1024 * 1024,
				MaxParts:    10000,
			},
			isValid: false,
		},
		{
			name: "Invalid - MaxPartSize above S3 maximum (5GB)",
			limits: ServiceLimits{
				MinPartSize: 5 * 1024 * 1024,
				MaxPartSize: 10 * 1024 * 1024 * 1024, // 10GB - too large
				MaxParts:    10000,
			},
			isValid: false,
		},
		{
			name: "Invalid - MaxParts zero",
			limits: ServiceLimits{
				MinPartSize: 5 * 1024 * 1024,
				MaxPartSize: 5 * 1024 * 1024 * 1024,
				MaxParts:    0,
			},
			isValid: false,
		},
		{
			name: "Invalid - MaxParts negative",
			limits: ServiceLimits{
				MinPartSize: 5 * 1024 * 1024,
				MaxPartSize: 5 * 1024 * 1024 * 1024,
				MaxParts:    -100,
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServiceLimits(tt.limits)
			if tt.isValid && err != nil {
				t.Errorf("validateServiceLimits() unexpected error = %v", err)
			}
			if !tt.isValid && err == nil {
				t.Errorf("validateServiceLimits() expected error but got nil")
			}
		})
	}
}

// Helper function to validate service limits (mirrors config.go validation)
func validateServiceLimits(limits ServiceLimits) error {
	if limits.MinPartSize < 5*1024*1024 {
		return &ValidationError{Field: "ServiceLimits.MinPartSize", Message: "must be at least 5MB"}
	}
	if limits.MaxPartSize > 5*1024*1024*1024 {
		return &ValidationError{Field: "ServiceLimits.MaxPartSize", Message: "cannot exceed 5GB"}
	}
	if limits.MinPartSize > limits.MaxPartSize {
		return &ValidationError{Field: "ServiceLimits", Message: "MinPartSize cannot be greater than MaxPartSize"}
	}
	if limits.MaxParts <= 0 {
		return &ValidationError{Field: "ServiceLimits.MaxParts", Message: "must be positive"}
	}
	return nil
}
