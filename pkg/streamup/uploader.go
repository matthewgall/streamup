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
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// Uploader handles streaming multipart uploads to S3-compatible storage.
type Uploader struct {
	config     Config
	s3Client   *s3.Client
	partSize   int64
	uploadID   string
	ctx        context.Context
	cancel     context.CancelFunc

	// Progress tracking
	bytesUploaded atomic.Int64
	partsUploaded atomic.Int32

	// Checksum tracking
	checksum     string
	checksumHash hash.Hash
	checksumMu   sync.Mutex
}

// part represents a chunk of data to be uploaded.
type part struct {
	number int32
	data   []byte
}

// completedPart represents an uploaded part with its ETag.
type completedPart struct {
	number int32
	etag   string
	err    error
}

// New creates a new Uploader with the given configuration.
func New(cfg Config) (*Uploader, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Calculate optimal part size
	partSize, err := CalculateOptimalPartSize(
		cfg.FileSize,
		cfg.MaxMemoryMB,
		cfg.Workers,
		cfg.QueueSize,
		*cfg.ServiceLimits,
	)
	if err != nil {
		return nil, err
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(cfg.Context)

	// Create AWS credentials
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
		cancel()
		return nil, &UploadError{Operation: "config creation", Err: err}
	}

	// Create S3 client with custom endpoint if provided
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		// R2 requires path-style addressing
		if cfg.AccountID != "" {
			o.UsePathStyle = false
		}
	})

	return &Uploader{
		config:   cfg,
		s3Client: s3Client,
		partSize: partSize,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Upload streams data from the reader to S3 using multipart upload.
func (u *Uploader) Upload(reader io.Reader) error {
	// Initialize multipart upload
	if err := u.initializeMultipartUpload(); err != nil {
		return err
	}

	// Initialize checksum calculation if enabled
	if u.config.CalculateChecksum {
		switch u.config.ChecksumAlgorithm {
		case "md5":
			u.checksumHash = md5.New()
		case "sha256":
			u.checksumHash = sha256.New()
		}
	}

	// Ensure cleanup on error
	var uploadErr error
	defer func() {
		if uploadErr != nil {
			_ = u.Abort()
		}
	}()

	// Create channels for producer-consumer pattern
	partsChan := make(chan part, u.config.QueueSize)
	resultsChan := make(chan completedPart, u.config.QueueSize)

	// Start worker pool
	var workerWg sync.WaitGroup
	for i := 0; i < u.config.Workers; i++ {
		workerWg.Add(1)
		go u.uploadWorker(&workerWg, partsChan, resultsChan)
	}

	// Start result collector
	var collectorWg sync.WaitGroup
	completedParts := make([]types.CompletedPart, 0)
	var collectorErr error
	collectorWg.Add(1)
	go func() {
		defer collectorWg.Done()
		completedParts, collectorErr = u.collectResults(resultsChan)
	}()

	// Producer: read data and send parts
	uploadErr = u.produceparts(reader, partsChan)
	close(partsChan)

	// Wait for workers to finish
	workerWg.Wait()
	close(resultsChan)

	// Wait for collector
	collectorWg.Wait()

	if uploadErr != nil {
		return uploadErr
	}
	if collectorErr != nil {
		return collectorErr
	}

	// Complete the multipart upload
	if err := u.completeMultipartUpload(completedParts); err != nil {
		uploadErr = err
		return err
	}

	// Finalize checksum if enabled
	if u.checksumHash != nil {
		u.checksumMu.Lock()
		u.checksum = hex.EncodeToString(u.checksumHash.Sum(nil))
		u.checksumMu.Unlock()
	}

	return nil
}

// initializeMultipartUpload starts a new multipart upload.
func (u *Uploader) initializeMultipartUpload() error {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(u.config.Bucket),
		Key:    aws.String(u.config.Key),
	}

	// Set Content-Type (auto-detect if not provided)
	contentType := u.config.ContentType
	if contentType == "" {
		contentType = DetectContentType(u.config.Key)
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	// Set optional HTTP headers
	if u.config.ContentDisposition != "" {
		input.ContentDisposition = aws.String(u.config.ContentDisposition)
	}
	if u.config.ContentEncoding != "" {
		input.ContentEncoding = aws.String(u.config.ContentEncoding)
	}
	if u.config.ContentLanguage != "" {
		input.ContentLanguage = aws.String(u.config.ContentLanguage)
	}
	if u.config.CacheControl != "" {
		input.CacheControl = aws.String(u.config.CacheControl)
	}

	// Set custom metadata
	if len(u.config.Metadata) > 0 {
		input.Metadata = u.config.Metadata
	}

	resp, err := u.s3Client.CreateMultipartUpload(u.ctx, input)
	if err != nil {
		return &UploadError{Operation: "CreateMultipartUpload", Err: err}
	}

	u.uploadID = *resp.UploadId
	return nil
}

// produceParts reads data from the reader and sends parts to the workers.
func (u *Uploader) produceparts(reader io.Reader, partsChan chan<- part) error {
	buffer := make([]byte, u.partSize)
	var partNumber int32 = 1

	for {
		// Check for cancellation
		select {
		case <-u.ctx.Done():
			return u.ctx.Err()
		default:
		}

		// Read a chunk
		n, err := io.ReadFull(reader, buffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return &UploadError{Operation: "reading data", Err: err}
		}

		// If we read something, send it
		if n > 0 {
			// Hash the data if checksum is enabled
			if u.checksumHash != nil {
				u.checksumMu.Lock()
				u.checksumHash.Write(buffer[:n])
				u.checksumMu.Unlock()
			}

			// Create a copy of the data for this part
			partData := make([]byte, n)
			copy(partData, buffer[:n])

			select {
			case partsChan <- part{number: partNumber, data: partData}:
				partNumber++
			case <-u.ctx.Done():
				return u.ctx.Err()
			}
		}

		// Check if we're done
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}

	return nil
}

// isRetryableError determines if an error should be retried.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Network errors are retryable
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Syscall errors (connection refused, reset, etc.)
	var syscallErr syscall.Errno
	if errors.As(err, &syscallErr) {
		return true
	}

	// AWS API errors
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		// Retry on 5xx server errors and throttling
		switch apiErr.ErrorCode() {
		case "InternalError", "ServiceUnavailable", "SlowDown", "RequestTimeout":
			return true
		}
		// Check HTTP status code if available
		code := apiErr.ErrorCode()
		if len(code) >= 3 && code[0] == '5' {
			return true
		}
	}

	// Context errors should not be retried
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Default: retry on unknown errors (conservative approach)
	return true
}

// calculateBackoff calculates the backoff duration for a retry attempt using exponential backoff.
func (u *Uploader) calculateBackoff(attempt int) time.Duration {
	// Calculate exponential backoff: initialDelay * (multiplier ^ attempt)
	backoffMs := float64(u.config.RetryDelay) * math.Pow(float64(u.config.RetryMultiplier), float64(attempt))

	// Cap at max delay
	if backoffMs > float64(u.config.MaxRetryDelay) {
		backoffMs = float64(u.config.MaxRetryDelay)
	}

	return time.Duration(backoffMs) * time.Millisecond
}

// uploadWorker uploads parts from the channel with retry logic.
func (u *Uploader) uploadWorker(wg *sync.WaitGroup, partsChan <-chan part, resultsChan chan<- completedPart) {
	defer wg.Done()

	for p := range partsChan {
		// Check for cancellation
		select {
		case <-u.ctx.Done():
			resultsChan <- completedPart{number: p.number, err: u.ctx.Err()}
			continue
		default:
		}

		// Upload the part with retry logic
		var resp *s3.UploadPartOutput
		var err error

	retryLoop:
		for attempt := 0; attempt <= u.config.MaxRetries; attempt++ {
			// Check for cancellation before each attempt
			select {
			case <-u.ctx.Done():
				resultsChan <- completedPart{number: p.number, err: u.ctx.Err()}
				goto nextPart
			default:
			}

			// Attempt upload
			resp, err = u.s3Client.UploadPart(u.ctx, &s3.UploadPartInput{
				Bucket:     aws.String(u.config.Bucket),
				Key:        aws.String(u.config.Key),
				UploadId:   aws.String(u.uploadID),
				PartNumber: aws.Int32(p.number),
				Body:       bytes.NewReader(p.data),
			})

			// Success!
			if err == nil {
				break
			}

			// Check if error is retryable
			if !isRetryableError(err) {
				// Non-retryable error, fail immediately
				break
			}

			// Last attempt failed, don't sleep
			if attempt == u.config.MaxRetries {
				break
			}

			// Calculate backoff and sleep
			backoff := u.calculateBackoff(attempt)

			// Sleep with context awareness
			select {
			case <-time.After(backoff):
				// Continue to next retry
			case <-u.ctx.Done():
				err = u.ctx.Err()
				break retryLoop
			}
		}

		// Check final result
		if err != nil {
			resultsChan <- completedPart{number: p.number, err: err}
			continue
		}

		// Send successful result
		resultsChan <- completedPart{
			number: p.number,
			etag:   *resp.ETag,
			err:    nil,
		}

		// Update progress
		u.bytesUploaded.Add(int64(len(p.data)))
		u.partsUploaded.Add(1)

		// Call progress callback if provided
		if u.config.ProgressCallback != nil {
			u.config.ProgressCallback(u.bytesUploaded.Load(), u.partsUploaded.Load())
		}

	nextPart:
	}
}

// collectResults gathers ETags from completed uploads.
func (u *Uploader) collectResults(resultsChan <-chan completedPart) ([]types.CompletedPart, error) {
	var parts []types.CompletedPart

	for result := range resultsChan {
		if result.err != nil {
			return nil, &UploadError{
				Operation: fmt.Sprintf("uploading part %d", result.number),
				Err:       result.err,
			}
		}

		parts = append(parts, types.CompletedPart{
			PartNumber: aws.Int32(result.number),
			ETag:       aws.String(result.etag),
		})
	}

	// Sort parts by number (required by S3)
	sort.Slice(parts, func(i, j int) bool {
		return *parts[i].PartNumber < *parts[j].PartNumber
	})

	return parts, nil
}

// completeMultipartUpload finalizes the upload.
func (u *Uploader) completeMultipartUpload(parts []types.CompletedPart) error {
	_, err := u.s3Client.CompleteMultipartUpload(u.ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(u.config.Bucket),
		Key:      aws.String(u.config.Key),
		UploadId: aws.String(u.uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: parts,
		},
	})

	if err != nil {
		return &UploadError{Operation: "CompleteMultipartUpload", Err: err}
	}

	return nil
}

// Abort cancels the upload and cleans up any uploaded parts.
func (u *Uploader) Abort() error {
	u.cancel()

	if u.uploadID == "" {
		return nil // Nothing to abort
	}

	_, err := u.s3Client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(u.config.Bucket),
		Key:      aws.String(u.config.Key),
		UploadId: aws.String(u.uploadID),
	})

	if err != nil {
		return &UploadError{Operation: "AbortMultipartUpload", Err: err}
	}

	return nil
}

// GetProgress returns the current upload progress.
func (u *Uploader) GetProgress() (bytesUploaded int64, partsUploaded int32) {
	return u.bytesUploaded.Load(), u.partsUploaded.Load()
}

// GetChecksum returns the calculated checksum of the uploaded data.
// Returns empty string if checksum calculation was not enabled or upload not completed.
func (u *Uploader) GetChecksum() string {
	u.checksumMu.Lock()
	defer u.checksumMu.Unlock()
	return u.checksum
}
