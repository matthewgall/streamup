package streamup

const (
	// S3 standard constraints
	defaultMinPartSize int64 = 5 * 1024 * 1024      // 5 MB
	defaultMaxPartSize int64 = 5 * 1024 * 1024 * 1024 // 5 GB
	defaultMaxParts    int   = 10000

	// Memory calculation multiplier (workers + queue size)
	defaultWorkers   = 4
	defaultQueueSize = 10
)

// ServiceLimits defines the constraints for S3-compatible multipart uploads.
// Different services may have different limits, so this allows customization.
type ServiceLimits struct {
	MinPartSize int64 // Minimum part size in bytes (default: 5MB)
	MaxPartSize int64 // Maximum part size in bytes (default: 5GB)
	MaxParts    int   // Maximum parts per upload (default: 10000)
}

// DefaultS3Limits returns the standard S3 multipart upload limits.
// These limits are used by AWS S3 and most S3-compatible services.
func DefaultS3Limits() ServiceLimits {
	return ServiceLimits{
		MinPartSize: defaultMinPartSize,
		MaxPartSize: defaultMaxPartSize,
		MaxParts:    defaultMaxParts,
	}
}

// R2Limits returns Cloudflare R2 multipart upload limits.
// R2 follows the same limits as standard S3.
func R2Limits() ServiceLimits {
	return DefaultS3Limits()
}

// BackblazeB2Limits returns Backblaze B2 multipart upload limits.
// B2 follows the same limits as standard S3.
func BackblazeB2Limits() ServiceLimits {
	return DefaultS3Limits()
}

// MinIOLimits returns MinIO multipart upload limits.
// MinIO defaults follow the same limits as standard S3.
// Note: MinIO is configurable by administrators and may vary per installation.
func MinIOLimits() ServiceLimits {
	return DefaultS3Limits()
}

// Validate checks if the service limits are valid according to S3 constraints.
func (l ServiceLimits) Validate() error {
	if l.MinPartSize < defaultMinPartSize {
		return &ValidationError{
			Field:   "MinPartSize",
			Message: "must be at least 5MB (S3 minimum)",
		}
	}

	if l.MaxPartSize > defaultMaxPartSize {
		return &ValidationError{
			Field:   "MaxPartSize",
			Message: "cannot exceed 5GB (S3 maximum)",
		}
	}

	if l.MinPartSize > l.MaxPartSize {
		return &ValidationError{
			Field:   "MinPartSize",
			Message: "cannot be greater than MaxPartSize",
		}
	}

	if l.MaxParts <= 0 || l.MaxParts > defaultMaxParts {
		return &ValidationError{
			Field:   "MaxParts",
			Message: "must be positive and not exceed 10000",
		}
	}

	return nil
}

// MaxFileSize returns the maximum file size that can be uploaded with these limits.
func (l ServiceLimits) MaxFileSize() int64 {
	return l.MaxPartSize * int64(l.MaxParts)
}
