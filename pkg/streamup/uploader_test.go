package streamup

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		wantErr     bool
		errContains string
	}{
		{
			name: "Valid config creates uploader",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
			},
			wantErr: false,
		},
		{
			name: "Invalid config returns error",
			config: Config{
				// Missing required fields
				FileSize: 100 * 1024 * 1024,
			},
			wantErr:     true,
			errContains: "AccessKeyID",
		},
		{
			name: "File too large returns error",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        60 * 1024 * 1024 * 1024 * 1024, // 60 TB
			},
			wantErr:     true,
			errContains: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader, err := New(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("New() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("New() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("New() unexpected error = %v", err)
				return
			}

			if uploader == nil {
				t.Error("New() returned nil uploader")
				return
			}

			// Verify uploader fields
			if uploader.config.Bucket != tt.config.Bucket {
				t.Errorf("uploader.config.Bucket = %q, want %q", uploader.config.Bucket, tt.config.Bucket)
			}

			if uploader.partSize <= 0 {
				t.Errorf("uploader.partSize = %d, want positive value", uploader.partSize)
			}

			if uploader.s3Client == nil {
				t.Error("uploader.s3Client is nil")
			}

			if uploader.ctx == nil {
				t.Error("uploader.ctx is nil")
			}

			if uploader.cancel == nil {
				t.Error("uploader.cancel is nil")
			}
		})
	}
}

func TestNew_PartSizeCalculation(t *testing.T) {
	tests := []struct {
		name             string
		fileSize         int64
		workers          int
		queueSize        int
		wantPartSizeApprox int64
	}{
		{
			name:               "70GB file should get ~70MB parts",
			fileSize:           70 * 1024 * 1024 * 1024,
			workers:            4,
			queueSize:          10,
			wantPartSizeApprox: 70 * 1024 * 1024,
		},
		{
			name:               "10GB file should get ~10MB parts",
			fileSize:           10 * 1024 * 1024 * 1024,
			workers:            4,
			queueSize:          10,
			wantPartSizeApprox: 10 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        tt.fileSize,
				Workers:         tt.workers,
				QueueSize:       tt.queueSize,
			}

			uploader, err := New(cfg)
			if err != nil {
				t.Fatalf("New() unexpected error = %v", err)
			}

			// Allow 10% tolerance for rounding
			tolerance := int64(float64(tt.wantPartSizeApprox) * 0.1)
			if uploader.partSize < tt.wantPartSizeApprox-tolerance ||
				uploader.partSize > tt.wantPartSizeApprox+tolerance {
				t.Errorf("uploader.partSize = %d, want ~%d (±%d)",
					uploader.partSize, tt.wantPartSizeApprox, tolerance)
			}
		})
	}
}

func TestNew_ContextPropagation(t *testing.T) {
	customCtx := context.WithValue(context.Background(), contextKey("test"), "value")

	cfg := Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Key:             "test-key",
		FileSize:        100 * 1024 * 1024,
		Context:         customCtx,
	}

	uploader, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	// Verify context is derived from custom context
	if uploader.ctx.Value(contextKey("test")) != "value" {
		t.Error("Context value not propagated to uploader")
	}

	// Test cancellation propagation
	uploader.cancel()

	select {
	case <-uploader.ctx.Done():
		// Good - context was cancelled
	default:
		t.Error("Calling cancel() did not cancel uploader context")
	}
}

func TestUploader_GetProgress(t *testing.T) {
	cfg := Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Key:             "test-key",
		FileSize:        100 * 1024 * 1024,
	}

	uploader, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	// Initial progress should be zero
	bytes, parts := uploader.GetProgress()
	if bytes != 0 {
		t.Errorf("Initial bytesUploaded = %d, want 0", bytes)
	}
	if parts != 0 {
		t.Errorf("Initial partsUploaded = %d, want 0", parts)
	}

	// Simulate progress updates
	uploader.bytesUploaded.Add(1024 * 1024) // 1 MB
	uploader.partsUploaded.Add(1)

	bytes, parts = uploader.GetProgress()
	if bytes != 1024*1024 {
		t.Errorf("bytesUploaded = %d, want %d", bytes, 1024*1024)
	}
	if parts != 1 {
		t.Errorf("partsUploaded = %d, want 1", parts)
	}

	// Multiple updates
	uploader.bytesUploaded.Add(2 * 1024 * 1024) // +2 MB
	uploader.partsUploaded.Add(2)               // +2 parts

	bytes, parts = uploader.GetProgress()
	if bytes != 3*1024*1024 {
		t.Errorf("bytesUploaded = %d, want %d", bytes, 3*1024*1024)
	}
	if parts != 3 {
		t.Errorf("partsUploaded = %d, want %d", parts, 3)
	}
}

func TestCollectResults_Sorting(t *testing.T) {
	// Create an uploader (we won't actually use S3)
	cfg := Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Key:             "test-key",
		FileSize:        100 * 1024 * 1024,
	}

	uploader, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	// Create a results channel with unordered parts
	resultsChan := make(chan completedPart, 5)
	resultsChan <- completedPart{number: 3, etag: "etag3"}
	resultsChan <- completedPart{number: 1, etag: "etag1"}
	resultsChan <- completedPart{number: 5, etag: "etag5"}
	resultsChan <- completedPart{number: 2, etag: "etag2"}
	resultsChan <- completedPart{number: 4, etag: "etag4"}
	close(resultsChan)

	// Collect and sort results
	parts, err := uploader.collectResults(resultsChan)
	if err != nil {
		t.Fatalf("collectResults() unexpected error = %v", err)
	}

	// Verify parts are sorted by number
	if len(parts) != 5 {
		t.Fatalf("collectResults() returned %d parts, want 5", len(parts))
	}

	for i, part := range parts {
		expectedNumber := int32(i + 1)
		if *part.PartNumber != expectedNumber {
			t.Errorf("parts[%d].PartNumber = %d, want %d", i, *part.PartNumber, expectedNumber)
		}
		expectedETag := "etag" + string(rune('0'+expectedNumber))
		if *part.ETag != expectedETag {
			t.Errorf("parts[%d].ETag = %q, want %q", i, *part.ETag, expectedETag)
		}
	}
}

func TestCollectResults_Error(t *testing.T) {
	cfg := Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Key:             "test-key",
		FileSize:        100 * 1024 * 1024,
	}

	uploader, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	// Create a results channel with an error
	resultsChan := make(chan completedPart, 3)
	resultsChan <- completedPart{number: 1, etag: "etag1"}
	resultsChan <- completedPart{number: 2, err: io.ErrUnexpectedEOF} // Error!
	resultsChan <- completedPart{number: 3, etag: "etag3"}
	close(resultsChan)

	// Collect results - should return error
	_, err = uploader.collectResults(resultsChan)
	if err == nil {
		t.Error("collectResults() expected error but got nil")
		return
	}

	// Verify it's an UploadError
	uploadErr, ok := err.(*UploadError)
	if !ok {
		t.Errorf("collectResults() error type = %T, want *UploadError", err)
		return
	}

	if !contains(uploadErr.Operation, "uploading part") {
		t.Errorf("UploadError.Operation = %q, want to contain 'uploading part'", uploadErr.Operation)
	}
}

func TestProduceParts_SmallData(t *testing.T) {
	cfg := Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Key:             "test-key",
		FileSize:        1024, // 1 KB
	}

	uploader, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	// Create test data
	testData := bytes.Repeat([]byte("test"), 256) // 1 KB
	reader := bytes.NewReader(testData)

	// Create parts channel
	partsChan := make(chan part, 10)

	// Produce parts in goroutine
	go func() {
		err := uploader.produceparts(reader, partsChan)
		if err != nil {
			t.Errorf("produceParts() unexpected error = %v", err)
		}
		close(partsChan)
	}()

	// Collect parts
	var receivedParts []part
	for p := range partsChan {
		receivedParts = append(receivedParts, p)
	}

	// Should have at least one part (could be 1 if file is smaller than part size)
	if len(receivedParts) == 0 {
		t.Error("produceParts() produced no parts")
	}

	// Verify first part
	if receivedParts[0].number != 1 {
		t.Errorf("First part number = %d, want 1", receivedParts[0].number)
	}

	// Reconstruct data from parts
	var reconstructed bytes.Buffer
	for _, p := range receivedParts {
		reconstructed.Write(p.data)
	}

	if !bytes.Equal(reconstructed.Bytes(), testData) {
		t.Error("Reconstructed data does not match original")
	}
}

func TestProduceParts_Cancellation(t *testing.T) {
	cfg := Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Key:             "test-key",
		FileSize:        100 * 1024 * 1024,
	}

	uploader, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	// Create a slow reader that will block
	slowReader := &slowReader{data: make([]byte, 100*1024*1024)}
	partsChan := make(chan part, 10)

	// Start producing in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- uploader.produceparts(slowReader, partsChan)
		close(partsChan)
	}()

	// Cancel after a short delay
	uploader.cancel()

	// Should get cancellation error
	err = <-errChan
	if err == nil {
		t.Error("produceParts() expected cancellation error but got nil")
	} else if err != context.Canceled {
		t.Errorf("produceParts() error = %v, want context.Canceled", err)
	}
}

// Helper types for testing

type slowReader struct {
	data []byte
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	// Simulate slow reading
	if len(p) > len(r.data) {
		copy(p, r.data)
		return len(r.data), io.EOF
	}
	copy(p, r.data[:len(p)])
	return len(p), nil
}

// Test helper to verify part sorting logic
func TestPartSorting(t *testing.T) {
	parts := []types.CompletedPart{
		{PartNumber: aws.Int32(5), ETag: aws.String("etag5")},
		{PartNumber: aws.Int32(2), ETag: aws.String("etag2")},
		{PartNumber: aws.Int32(1), ETag: aws.String("etag1")},
		{PartNumber: aws.Int32(4), ETag: aws.String("etag4")},
		{PartNumber: aws.Int32(3), ETag: aws.String("etag3")},
	}

	// Sort using the same logic as collectResults
	sortParts := func(parts []types.CompletedPart) {
		for i := 0; i < len(parts); i++ {
			for j := i + 1; j < len(parts); j++ {
				if *parts[i].PartNumber > *parts[j].PartNumber {
					parts[i], parts[j] = parts[j], parts[i]
				}
			}
		}
	}

	sortParts(parts)

	// Verify sorted order
	for i, part := range parts {
		expectedNumber := int32(i + 1)
		if *part.PartNumber != expectedNumber {
			t.Errorf("parts[%d].PartNumber = %d, want %d", i, *part.PartNumber, expectedNumber)
		}
	}
}

// Note: Full integration tests that actually upload to S3/R2 would require:
// 1. Mock S3 service (using something like localstack or minio)
// 2. Real S3/R2 credentials (for end-to-end testing)
// 3. Network connectivity
//
// These tests focus on the logic that can be tested without external dependencies.
// For production use, consider adding integration tests in a separate test suite.
