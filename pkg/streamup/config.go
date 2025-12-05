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
	"context"
	"fmt"
)

// ProgressCallback is called periodically during upload to report progress.
// bytesUploaded: total bytes uploaded so far
// partsUploaded: number of parts successfully uploaded
type ProgressCallback func(bytesUploaded int64, partsUploaded int32)

// Config holds the configuration for an S3 multipart upload.
type Config struct {
	// S3 Credentials
	AccessKeyID     string
	SecretAccessKey string

	// S3 Location
	Bucket string // S3 bucket name
	Key    string // Object key (path) in the bucket

	// File Information
	FileSize int64 // Total file size in bytes (required for optimization)

	// Service Configuration
	AccountID string // Required for Cloudflare R2, ignored for other services
	Endpoint  string // Optional custom endpoint (e.g., "s3.amazonaws.com")
	Region    string // Optional region (default: "auto" for R2, "us-east-1" for others)

	// Upload Tuning
	Workers       int            // Number of concurrent upload workers (default: 4)
	QueueSize     int            // Size of part queue buffer (default: 10)
	MaxMemoryMB   int            // Optional memory limit in MB (0 = no limit)
	ServiceLimits *ServiceLimits // Optional service-specific limits (nil = use S3 defaults)

	// Retry Configuration
	MaxRetries      int // Maximum retry attempts per part (default: 3)
	RetryDelay      int // Initial retry delay in milliseconds (default: 1000)
	MaxRetryDelay   int // Maximum retry delay in milliseconds (default: 30000)
	RetryMultiplier int // Backoff multiplier (default: 2)

	// Object Metadata
	ContentType        string            // MIME type (auto-detected if empty)
	ContentDisposition string            // Content-Disposition header
	ContentEncoding    string            // Content-Encoding (e.g., "gzip")
	ContentLanguage    string            // Content-Language
	CacheControl       string            // Cache-Control header
	Metadata           map[string]string // Custom metadata key-value pairs

	// Progress Tracking
	ProgressCallback ProgressCallback // Optional callback for progress updates

	// Checksum
	CalculateChecksum bool   // Calculate checksum during upload (default: true)
	ChecksumAlgorithm string // Algorithm: "md5", "sha256" (default: "md5")

	// Context
	Context context.Context // Optional context for cancellation (default: background)
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	// Required fields
	if c.AccessKeyID == "" {
		return &ValidationError{Field: "AccessKeyID", Message: "required"}
	}
	if c.SecretAccessKey == "" {
		return &ValidationError{Field: "SecretAccessKey", Message: "required"}
	}
	if c.Bucket == "" {
		return &ValidationError{Field: "Bucket", Message: "required"}
	}
	if c.Key == "" {
		return &ValidationError{Field: "Key", Message: "required"}
	}
	if c.FileSize <= 0 {
		return &ValidationError{Field: "FileSize", Message: "must be greater than 0"}
	}

	// Apply defaults
	if c.Workers <= 0 {
		c.Workers = defaultWorkers
	}
	if c.QueueSize <= 0 {
		c.QueueSize = defaultQueueSize
	}

	// Apply retry defaults
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3 // Default: 3 retries
	}
	if c.RetryDelay <= 0 {
		c.RetryDelay = 1000 // Default: 1 second
	}
	if c.MaxRetryDelay <= 0 {
		c.MaxRetryDelay = 30000 // Default: 30 seconds
	}
	if c.RetryMultiplier <= 0 {
		c.RetryMultiplier = 2 // Default: 2x backoff
	}

	// Apply checksum defaults
	if c.ChecksumAlgorithm == "" {
		c.ChecksumAlgorithm = "md5" // Default: MD5
	}
	// Validate checksum algorithm
	if c.ChecksumAlgorithm != "md5" && c.ChecksumAlgorithm != "sha256" {
		return &ValidationError{
			Field:   "ChecksumAlgorithm",
			Message: "must be 'md5' or 'sha256'",
		}
	}

	// Validate or set service limits
	if c.ServiceLimits == nil {
		limits := DefaultS3Limits()
		c.ServiceLimits = &limits
	} else {
		if err := c.ServiceLimits.Validate(); err != nil {
			return err
		}
	}

	// Check file size against service limits
	maxFileSize := c.ServiceLimits.MaxFileSize()
	if c.FileSize > maxFileSize {
		return &ValidationError{
			Field: "FileSize",
			Message: fmt.Sprintf("exceeds service limit of %d bytes (%d GB)",
				maxFileSize, maxFileSize/(1024*1024*1024)),
		}
	}

	// Set context default
	if c.Context == nil {
		c.Context = context.Background()
	}

	// Set region default
	if c.Region == "" {
		if c.AccountID != "" {
			c.Region = "auto" // R2 default
		} else {
			c.Region = "us-east-1" // S3 default
		}
	}

	// Auto-detect R2 endpoint if AccountID provided but Endpoint is not
	if c.AccountID != "" && c.Endpoint == "" {
		c.Endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", c.AccountID)
	}

	return nil
}

// GetEndpoint returns the S3 endpoint URL to use.
func (c *Config) GetEndpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	// Default to AWS S3
	return fmt.Sprintf("https://s3.%s.amazonaws.com", c.Region)
}
