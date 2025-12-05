package streamup

import (
	"errors"
	"testing"
)

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		message string
		want    string
	}{
		{
			name:    "AccessKeyID validation error",
			field:   "AccessKeyID",
			message: "required",
			want:    "validation error for AccessKeyID: required",
		},
		{
			name:    "FileSize validation error",
			field:   "FileSize",
			message: "must be greater than 0",
			want:    "validation error for FileSize: must be greater than 0",
		},
		{
			name:    "ServiceLimits validation error",
			field:   "ServiceLimits.MinPartSize",
			message: "must be at least 5MB",
			want:    "validation error for ServiceLimits.MinPartSize: must be at least 5MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ValidationError{
				Field:   tt.field,
				Message: tt.message,
			}

			got := err.Error()
			if got != tt.want {
				t.Errorf("ValidationError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidationError_IsError(t *testing.T) {
	err := &ValidationError{
		Field:   "TestField",
		Message: "test message",
	}

	// Verify it implements error interface
	var _ error = err

	if err.Error() == "" {
		t.Error("ValidationError.Error() returned empty string")
	}
}

func TestUploadError_Error(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		err       error
		want      string
	}{
		{
			name:      "CreateMultipartUpload error",
			operation: "CreateMultipartUpload",
			err:       errors.New("access denied"),
			want:      "upload error during CreateMultipartUpload: access denied",
		},
		{
			name:      "UploadPart error",
			operation: "UploadPart",
			err:       errors.New("connection timeout"),
			want:      "upload error during UploadPart: connection timeout",
		},
		{
			name:      "CompleteMultipartUpload error",
			operation: "CompleteMultipartUpload",
			err:       errors.New("invalid part ETag"),
			want:      "upload error during CompleteMultipartUpload: invalid part ETag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &UploadError{
				Operation: tt.operation,
				Err:       tt.err,
			}

			got := err.Error()
			if got != tt.want {
				t.Errorf("UploadError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUploadError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	uploadErr := &UploadError{
		Operation: "TestOperation",
		Err:       originalErr,
	}

	unwrapped := uploadErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("UploadError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}

	// Verify errors.Is works with wrapped errors
	if !errors.Is(uploadErr, originalErr) {
		t.Error("errors.Is() should recognize wrapped error")
	}
}

func TestUploadError_IsError(t *testing.T) {
	err := &UploadError{
		Operation: "TestOperation",
		Err:       errors.New("test error"),
	}

	// Verify it implements error interface
	var _ error = err

	if err.Error() == "" {
		t.Error("UploadError.Error() returned empty string")
	}
}

func TestUploadError_ErrorWrapping(t *testing.T) {
	// Test error wrapping chain
	baseErr := errors.New("network error")
	uploadErr := &UploadError{
		Operation: "UploadPart",
		Err:       baseErr,
	}

	// Test errors.Unwrap
	if errors.Unwrap(uploadErr) != baseErr {
		t.Error("errors.Unwrap() failed to unwrap UploadError")
	}

	// Test errors.Is with chain
	if !errors.Is(uploadErr, baseErr) {
		t.Error("errors.Is() should recognize base error in chain")
	}

	// Test with wrapped wrapped error
	var sentinelErr = errors.New("sentinel")
	wrappedErr := &UploadError{
		Operation: "Inner",
		Err:       sentinelErr,
	}
	outerErr := &UploadError{
		Operation: "Outer",
		Err:       wrappedErr,
	}

	if !errors.Is(outerErr, sentinelErr) {
		t.Error("errors.Is() should recognize sentinel in nested wrapped errors")
	}
}

func TestErrorTypes_AsInterface(t *testing.T) {
	// Verify both error types can be used as error interface
	var err error

	err = &ValidationError{Field: "test", Message: "test"}
	if err.Error() == "" {
		t.Error("ValidationError should implement error interface")
	}

	err = &UploadError{Operation: "test", Err: errors.New("test")}
	if err.Error() == "" {
		t.Error("UploadError should implement error interface")
	}
}

func TestErrorTypes_Nil(t *testing.T) {
	// Test nil error values
	var validationErr *ValidationError
	var uploadErr *UploadError

	if validationErr != nil {
		t.Error("Nil ValidationError should be nil")
	}

	if uploadErr != nil {
		t.Error("Nil UploadError should be nil")
	}

	// Test that nil pointers still have Error() method (though it would panic if called)
	// This is just to verify the type signature
	var _ error = validationErr
	var _ error = uploadErr
}

func TestUploadError_NilUnwrap(t *testing.T) {
	// Test unwrap with nil inner error
	uploadErr := &UploadError{
		Operation: "TestOperation",
		Err:       nil,
	}

	unwrapped := uploadErr.Unwrap()
	if unwrapped != nil {
		t.Errorf("UploadError.Unwrap() with nil Err = %v, want nil", unwrapped)
	}
}
