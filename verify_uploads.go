package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	// Get credentials from environment
	accessKey := os.Getenv("S3_ACCESS_KEY_ID")
	secretKey := os.Getenv("S3_SECRET_ACCESS_KEY")
	bucket := os.Getenv("S3_BUCKET")
	accountID := os.Getenv("R2_ACCOUNT_ID")

	if accessKey == "" || secretKey == "" || bucket == "" || accountID == "" {
		fmt.Println("Error: Missing required environment variables")
		fmt.Println("Required: S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY, S3_BUCKET, R2_ACCOUNT_ID")
		os.Exit(1)
	}

	// Create AWS config
	ctx := context.Background()
	creds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(creds),
		config.WithRegion("auto"),
	)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Create S3 client with R2 endpoint
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	// List objects with "test/" prefix
	fmt.Printf("Verifying uploads in bucket: %s\n", bucket)
	fmt.Printf("R2 Endpoint: %s\n\n", endpoint)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("test/"),
	}

	result, err := s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		fmt.Printf("Error listing objects: %v\n", err)
		os.Exit(1)
	}

	if len(result.Contents) == 0 {
		fmt.Println("No objects found with prefix 'test/'")
		os.Exit(0)
	}

	fmt.Printf("Found %d objects:\n\n", len(result.Contents))
	fmt.Printf("%-50s %12s %s\n", "Key", "Size", "Last Modified")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────────")

	totalSize := int64(0)
	for _, obj := range result.Contents {
		sizeMB := float64(*obj.Size) / (1024 * 1024)
		fmt.Printf("%-50s %9.2f MB %s\n", *obj.Key, sizeMB, obj.LastModified.Format("2006-01-02 15:04:05"))
		totalSize += *obj.Size
	}

	fmt.Println("─────────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("Total: %d objects, %.2f MB\n", len(result.Contents), float64(totalSize)/(1024*1024))

	// Verify specific files we uploaded
	fmt.Println("\nVerifying expected uploads:")

	expectedFiles := map[string]int64{
		"test/streamup-test-10mb.bin": 10 * 1024 * 1024,
		"test/streamup-test-100mb.bin": 100 * 1024 * 1024,
		"test/streamup-test-50mb-constrained.bin": 50 * 1024 * 1024,
		"test/go-logo.png": 0, // Small file, size unknown
	}

	allFound := true
	for key, expectedSize := range expectedFiles {
		found := false
		var actualSize int64
		for _, obj := range result.Contents {
			if *obj.Key == key {
				found = true
				actualSize = *obj.Size
				break
			}
		}

		if !found {
			fmt.Printf("✗ %s - NOT FOUND\n", key)
			allFound = false
		} else if expectedSize > 0 {
			if actualSize == expectedSize {
				fmt.Printf("✓ %s - OK (%d bytes)\n", key, actualSize)
			} else {
				fmt.Printf("⚠ %s - SIZE MISMATCH (expected %d, got %d)\n", key, expectedSize, actualSize)
				allFound = false
			}
		} else {
			fmt.Printf("✓ %s - OK (%d bytes)\n", key, actualSize)
		}
	}

	if allFound {
		fmt.Println("\n✅ All uploads verified successfully!")
	} else {
		fmt.Println("\n❌ Some uploads failed verification")
		os.Exit(1)
	}
}
