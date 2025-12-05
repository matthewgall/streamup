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
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// DownloadConfig contains configuration for downloading from S3.
type DownloadConfig struct {
	AccessKeyID       string // S3 access key ID
	SecretAccessKey   string // S3 secret access key
	Bucket            string // S3 bucket name
	Key               string // Object key
	AccountID         string // Cloudflare R2 account ID (optional)
	Endpoint          string // Custom S3 endpoint (optional)
	Region            string // S3 region (default: auto for R2, us-east-1 for others)
	CalculateChecksum bool   // Calculate checksum during download (default: false)
	ChecksumAlgorithm string // Algorithm: "md5", "sha256" (default: "md5")
}

// Downloader handles streaming downloads from S3-compatible storage.
type Downloader struct {
	config           DownloadConfig
	s3Client         *s3.Client
	progressCallback func(downloaded int64)
	checksum         string
	checksumHash     hash.Hash
}

// ProgressCallback is called periodically during download with bytes downloaded.
type DownloadProgressCallback func(downloaded int64)

// NewDownloader creates a new downloader instance.
func NewDownloader(cfg DownloadConfig) (*Downloader, error) {
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
	if cfg.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	// Set default region
	if cfg.Region == "" {
		if cfg.AccountID != "" {
			cfg.Region = "auto" // R2 default
		} else {
			cfg.Region = "us-east-1" // S3 default
		}
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

	return &Downloader{
		config:   cfg,
		s3Client: s3Client,
	}, nil
}

// SetProgressCallback sets a callback to be called during download progress.
func (d *Downloader) SetProgressCallback(callback DownloadProgressCallback) {
	d.progressCallback = callback
}

// GetSize retrieves the size of the object without downloading it.
func (d *Downloader) GetSize(ctx context.Context) (int64, error) {
	// Use HeadObject to get metadata
	resp, err := d.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.config.Bucket),
		Key:    aws.String(d.config.Key),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get object metadata: %w", err)
	}

	if resp.ContentLength == nil {
		return 0, fmt.Errorf("object has no Content-Length")
	}

	return *resp.ContentLength, nil
}

// Download streams the object to the provided writer.
func (d *Downloader) Download(ctx context.Context, writer io.Writer) error {
	// Initialize checksum calculation if enabled
	if d.config.CalculateChecksum {
		if d.config.ChecksumAlgorithm == "" || d.config.ChecksumAlgorithm == "md5" {
			d.checksumHash = md5.New()
		} else if d.config.ChecksumAlgorithm == "sha256" {
			d.checksumHash = sha256.New()
		}
	}

	// Get the object
	resp, err := d.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(d.config.Bucket),
		Key:    aws.String(d.config.Key),
	})
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}
	defer resp.Body.Close()

	// Prepare writers (output + optional checksum + optional progress)
	writers := []io.Writer{writer}
	if d.checksumHash != nil {
		writers = append(writers, d.checksumHash)
	}
	multiWriter := io.MultiWriter(writers...)

	// Stream to writer with progress tracking
	if d.progressCallback != nil {
		// Wrap writer with progress tracking
		pw := &progressWriter{
			writer:   multiWriter,
			callback: d.progressCallback,
			written:  0,
		}
		_, err = io.Copy(pw, resp.Body)
	} else {
		// Direct copy without progress
		_, err = io.Copy(multiWriter, resp.Body)
	}

	if err != nil {
		return fmt.Errorf("failed to download object: %w", err)
	}

	// Finalize checksum if enabled
	if d.checksumHash != nil {
		d.checksum = hex.EncodeToString(d.checksumHash.Sum(nil))
	}

	return nil
}

// progressWriter wraps an io.Writer and calls a callback on each write.
type progressWriter struct {
	writer   io.Writer
	callback func(int64)
	written  int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.written += int64(n)
	if pw.callback != nil {
		pw.callback(pw.written)
	}
	return n, err
}

// GetChecksum returns the calculated checksum of the downloaded data.
// Returns empty string if checksum calculation was not enabled or download not completed.
func (d *Downloader) GetChecksum() string {
	return d.checksum
}
