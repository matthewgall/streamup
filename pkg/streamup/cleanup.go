package streamup

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// IncompleteUpload represents an incomplete multipart upload.
type IncompleteUpload struct {
	Key          string
	UploadID     string
	Initiated    time.Time
	StorageClass types.StorageClass
}

// CleanupConfig holds configuration for cleanup operations.
type CleanupConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	AccountID       string // For R2
	Endpoint        string
	Region          string

	// Filters
	Prefix     string        // Only clean uploads with this prefix
	OlderThan  time.Duration // Only clean uploads older than this
	MaxResults int           // Maximum number of uploads to return (0 = all)

	// Options
	DryRun bool // If true, only list uploads without aborting
}

// CleanupResult represents the result of a cleanup operation.
type CleanupResult struct {
	TotalFound   int
	TotalAborted int
	Errors       []error
	Uploads      []IncompleteUpload
}

// ListIncompleteUploads lists all incomplete multipart uploads in a bucket.
func ListIncompleteUploads(ctx context.Context, cfg CleanupConfig) ([]IncompleteUpload, error) {
	s3Client, err := createS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}

	var uploads []IncompleteUpload
	var continuationToken *string

	cutoffTime := time.Time{}
	if cfg.OlderThan > 0 {
		cutoffTime = time.Now().Add(-cfg.OlderThan)
	}

	for {
		input := &s3.ListMultipartUploadsInput{
			Bucket: aws.String(cfg.Bucket),
		}

		if cfg.Prefix != "" {
			input.Prefix = aws.String(cfg.Prefix)
		}

		if continuationToken != nil {
			input.KeyMarker = continuationToken
		}

		if cfg.MaxResults > 0 {
			input.MaxUploads = aws.Int32(int32(cfg.MaxResults))
		}

		result, err := s3Client.ListMultipartUploads(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list multipart uploads: %w", err)
		}

		for _, upload := range result.Uploads {
			// Apply age filter
			if cfg.OlderThan > 0 && upload.Initiated != nil {
				if upload.Initiated.After(cutoffTime) {
					continue
				}
			}

			incomplete := IncompleteUpload{
				Key:      *upload.Key,
				UploadID: *upload.UploadId,
			}

			if upload.Initiated != nil {
				incomplete.Initiated = *upload.Initiated
			}

			if upload.StorageClass != "" {
				incomplete.StorageClass = upload.StorageClass
			}

			uploads = append(uploads, incomplete)

			// Stop if we've reached max results
			if cfg.MaxResults > 0 && len(uploads) >= cfg.MaxResults {
				return uploads, nil
			}
		}

		// Check if there are more results
		if result.IsTruncated == nil || !*result.IsTruncated {
			break
		}

		continuationToken = result.NextKeyMarker
	}

	return uploads, nil
}

// CleanupIncompleteUploads aborts incomplete multipart uploads.
func CleanupIncompleteUploads(ctx context.Context, cfg CleanupConfig) (*CleanupResult, error) {
	// List incomplete uploads
	uploads, err := ListIncompleteUploads(ctx, cfg)
	if err != nil {
		return nil, err
	}

	result := &CleanupResult{
		TotalFound: len(uploads),
		Uploads:    uploads,
	}

	// If dry-run, just return the list
	if cfg.DryRun {
		return result, nil
	}

	// Abort each upload
	s3Client, err := createS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}

	for _, upload := range uploads {
		_, err := s3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(cfg.Bucket),
			Key:      aws.String(upload.Key),
			UploadId: aws.String(upload.UploadID),
		})

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to abort %s (upload ID: %s): %w", upload.Key, upload.UploadID, err))
		} else {
			result.TotalAborted++
		}
	}

	return result, nil
}

// createS3Client creates an S3 client for cleanup operations.
func createS3Client(ctx context.Context, cfg CleanupConfig) (*s3.Client, error) {
	// Create credentials
	creds := credentials.NewStaticCredentialsProvider(
		cfg.AccessKeyID,
		cfg.SecretAccessKey,
		"",
	)

	// Set region default
	region := cfg.Region
	if region == "" {
		if cfg.AccountID != "" {
			region = "auto" // R2 default
		} else {
			region = "us-east-1" // S3 default
		}
	}

	// Set endpoint default
	endpoint := cfg.Endpoint
	if endpoint == "" && cfg.AccountID != "" {
		endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
	}

	// Create AWS config with custom User-Agent
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(creds),
		config.WithRegion(region),
		config.WithAppID(UserAgent()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return s3Client, nil
}
