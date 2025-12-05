package streamup

import "fmt"

// ValidationError represents an error during configuration validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for %s: %s", e.Field, e.Message)
}

// UploadError represents an error during the upload process.
type UploadError struct {
	Operation string
	Err       error
}

func (e *UploadError) Error() string {
	return fmt.Sprintf("upload error during %s: %v", e.Operation, e.Err)
}

func (e *UploadError) Unwrap() error {
	return e.Err
}
