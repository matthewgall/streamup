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
