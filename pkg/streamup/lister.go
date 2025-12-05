package streamup

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ListConfig contains configuration for listing S3 objects.
type ListConfig struct {
	AccessKeyID     string // S3 access key ID
	SecretAccessKey string // S3 secret access key
	Bucket          string // S3 bucket name
	AccountID       string // Cloudflare R2 account ID (optional)
	Endpoint        string // Custom S3 endpoint (optional)
	Region          string // S3 region (default: auto for R2, us-east-1 for others)
	Prefix          string // Filter by prefix (optional)
	MaxKeys         int    // Maximum keys to return (default: 1000)
}

// Object represents an S3 object with metadata.
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// Lister handles listing objects in S3-compatible storage.
type Lister struct {
	config   ListConfig
	s3Client *s3.Client
}

// NewLister creates a new lister instance.
func NewLister(cfg ListConfig) (*Lister, error) {
	// Validate required fields
	if cfg.AccessKeyID == "" {
		return nil, fmt.Errorf("AccessKeyID is required")
	}
	if cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("SecretAccessKey is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	// Set default region
	if cfg.Region == "" {
		if cfg.AccountID != "" {
			cfg.Region = "auto" // R2 default
		} else {
			cfg.Region = "us-east-1" // S3 default
		}
	}

	// Set default max keys
	if cfg.MaxKeys == 0 {
		cfg.MaxKeys = 1000
	}

	// Construct endpoint if not provided
	if cfg.Endpoint == "" && cfg.AccountID != "" {
		// Cloudflare R2 endpoint format
		cfg.Endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
	}

	// Create AWS credentials
	ctx := context.Background()
	creds := credentials.NewStaticCredentialsProvider(
		cfg.AccessKeyID,
		cfg.SecretAccessKey,
		"",
	)

	// Create AWS config with custom User-Agent
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(creds),
		config.WithRegion(cfg.Region),
		config.WithAppID(UserAgent()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		// Use path-style addressing for R2 and custom endpoints
		if cfg.Endpoint != "" {
			o.UsePathStyle = true
		}
	})

	return &Lister{
		config:   cfg,
		s3Client: s3Client,
	}, nil
}

// List retrieves objects from the bucket.
func (l *Lister) List(ctx context.Context) ([]Object, error) {
	var objects []Object

	// Prepare list input
	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(l.config.Bucket),
		MaxKeys: aws.Int32(int32(l.config.MaxKeys)),
	}

	// Add prefix if specified
	if l.config.Prefix != "" {
		input.Prefix = aws.String(l.config.Prefix)
	}

	// List objects (paginated)
	paginator := s3.NewListObjectsV2Paginator(l.s3Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		// Convert to our Object type
		for _, obj := range page.Contents {
			objects = append(objects, Object{
				Key:          *obj.Key,
				Size:         *obj.Size,
				LastModified: *obj.LastModified,
			})
		}

		// Stop if we've reached max keys
		if len(objects) >= l.config.MaxKeys {
			break
		}
	}

	return objects, nil
}
