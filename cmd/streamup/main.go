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

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/matthewgall/streamup/pkg/streamup"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

// Version information - injected at build time via ldflags
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func init() {
	// Set package version variables from main
	streamup.Version = version
	streamup.GitCommit = commit
	streamup.BuildDate = buildDate
}

var (
	// S3 Configuration
	accessKeyID     string
	secretAccessKey string
	bucket          string

	// Service Configuration
	accountID string
	endpoint  string
	region    string

	// Input Configuration
	stdinSize int64

	// Upload Tuning
	workers     int
	queueSize   int
	maxMemory   int
	minPartSize int64
	maxPartSize int64
	maxParts    int

	// Retry Configuration
	maxRetries      int
	retryDelay      int
	maxRetryDelay   int
	retryMultiplier int

	// Object Metadata
	contentType        string
	contentDisposition string
	contentEncoding    string
	contentLanguage    string
	cacheControl       string
	metadata           []string // Key=value pairs

	// Checksum
	calculateChecksum bool
	checksumAlgorithm string

	// Output Configuration
	quiet bool
)

var rootCmd = &cobra.Command{
	Use:   "streamup",
	Short: "Stream large files to S3-compatible storage",
	Long: `streamup - Stream large files (1GB-50TB) to S3-compatible storage without touching disk

Efficiently upload massive files using multipart uploads with constant memory usage.
Works with Cloudflare R2, AWS S3, Backblaze B2, MinIO, and any S3-compatible service.

Environment Variables:
  S3_ACCESS_KEY_ID      S3 access key ID
  S3_SECRET_ACCESS_KEY  S3 secret access key
  S3_BUCKET             S3 bucket name
  S3_ENDPOINT           Custom S3 endpoint
  S3_REGION             S3 region
  R2_ACCOUNT_ID         Cloudflare R2 account ID (R2 only)`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Load from environment variables if flags are not set
		// This happens AFTER flag parsing, so env vars won't show in --help
		if accessKeyID == "" {
			accessKeyID = os.Getenv("S3_ACCESS_KEY_ID")
		}
		if secretAccessKey == "" {
			secretAccessKey = os.Getenv("S3_SECRET_ACCESS_KEY")
		}
		if bucket == "" {
			bucket = os.Getenv("S3_BUCKET")
		}
		if accountID == "" {
			accountID = os.Getenv("R2_ACCOUNT_ID")
		}
		if endpoint == "" {
			endpoint = os.Getenv("S3_ENDPOINT")
		}
		if region == "" {
			region = os.Getenv("S3_REGION")
		}
	},
}

var uploadCmd = &cobra.Command{
	Use:   "upload <key> <source>",
	Short: "Upload a file to S3-compatible storage",
	Long: `Upload a file to S3-compatible storage using streaming multipart upload.

The <key> argument specifies the object key (path) in the bucket.
The <source> argument can be:
  - A local file path (e.g., /path/to/file.dat)
  - A URL to download and stream (e.g., https://example.com/file.dat)
  - A dash "-" to read from stdin (requires --size flag)

Examples:
  # Upload a local file to Cloudflare R2
  streamup upload backups/data.zip data.zip

  # Download from URL and stream to R2 (zero disk usage)
  streamup upload osm/planet.osm.pbf https://planet.openstreetmap.org/pbf/planet-latest.osm.pbf

  # Upload to AWS S3
  streamup upload backups/data.zip data.zip --endpoint s3.amazonaws.com --region us-west-2

  # Upload from stdin with known size
  pg_dump mydb | gzip | streamup upload backups/db.sql.gz - --size 5000000000

  # Memory-constrained upload
  streamup upload large.dat /data/large.dat --max-memory 1024`,
	Args: cobra.ExactArgs(2),
	RunE: runUpload,
}

var downloadCmd = &cobra.Command{
	Use:   "download <key> [output]",
	Short: "Download a file from S3-compatible storage",
	Long: `Download a file from S3-compatible storage with streaming.

The <key> argument specifies the object key (path) in the bucket.
The [output] argument is optional:
  - Omit or use "-" to write to stdout
  - Specify a file path to save locally

Examples:
  # Download to stdout
  streamup download backups/data.zip | tar -xz

  # Download to file
  streamup download backups/data.zip ./data.zip

  # Download from AWS S3
  streamup download backups/data.zip ./data.zip --endpoint s3.amazonaws.com --region us-west-2

  # Pipe to another command
  streamup download backups/db.sql.gz - | gunzip | psql mydb`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runDownload,
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for streamup.

To load completions:

Bash:
  $ source <(streamup completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ streamup completion bash > /etc/bash_completion.d/streamup
  # macOS:
  $ streamup completion bash > $(brew --prefix)/etc/bash_completion.d/streamup

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ streamup completion zsh > "${fpath[1]}/_streamup"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ streamup completion fish | source

  # To load completions for each session, execute once:
  $ streamup completion fish > ~/.config/fish/completions/streamup.fish

PowerShell:
  PS> streamup completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> streamup completion powershell > streamup.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		checkUpdates, _ := cmd.Flags().GetBool("check-updates")

		fmt.Printf("streamup %s\n", GetVersion())
		if commit != "none" {
			fmt.Printf("Commit: %s\n", commit)
		}
		if buildDate != "unknown" {
			fmt.Printf("Built: %s\n", buildDate)
		}
		fmt.Printf("User-Agent: %s\n", GetUserAgent())

		if checkUpdates {
			PrintUpdateNotification()
		}
	},
}

var (
	// Cleanup command flags
	cleanupPrefix    string
	cleanupOlderThan string
	cleanupMaxResults int
	cleanupDryRun    bool
	cleanupForce     bool
)

var (
	// List command flags
	listMaxKeys int
)

var listCmd = &cobra.Command{
	Use:   "list [prefix]",
	Short: "List objects in S3-compatible storage",
	Long: `List objects in an S3-compatible bucket.

The optional [prefix] argument filters objects by prefix (path).

Examples:
  # List all objects
  streamup list

  # List objects with prefix
  streamup list backups/

  # List with max results
  streamup list --max-keys 100

  # List specific prefix
  streamup list test/`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up incomplete multipart uploads",
	Long: `Clean up incomplete multipart uploads in an S3-compatible bucket.

Incomplete multipart uploads continue to consume storage and accrue costs until they
are completed or aborted. This command lists and optionally aborts incomplete uploads.

Use --dry-run to preview what would be deleted without actually deleting anything.

Examples:
  # List incomplete uploads (dry-run)
  streamup cleanup --dry-run

  # Clean up uploads older than 24 hours
  streamup cleanup --older-than 24h

  # Clean up uploads with specific prefix
  streamup cleanup --prefix "backups/" --older-than 7d

  # Force cleanup without confirmation
  streamup cleanup --older-than 24h --force`,
	RunE: runCleanup,
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(versionCmd)

	// Global S3 Configuration flags (shared across all commands)
	// Note: We don't set defaults from env vars here to avoid exposing secrets in --help
	rootCmd.PersistentFlags().StringVar(&accessKeyID, "access-key", "", "S3 access key ID")
	rootCmd.PersistentFlags().StringVar(&secretAccessKey, "secret-key", "", "S3 secret access key")
	rootCmd.PersistentFlags().StringVar(&bucket, "bucket", "", "S3 bucket name")

	// Global Service Configuration flags (shared across all commands)
	rootCmd.PersistentFlags().StringVar(&accountID, "account-id", "", "Cloudflare R2 account ID (R2 only)")
	rootCmd.PersistentFlags().StringVar(&endpoint, "endpoint", "", "Custom S3 endpoint")
	rootCmd.PersistentFlags().StringVar(&region, "region", "", "S3 region")

	// Input Configuration flags
	uploadCmd.Flags().Int64VarP(&stdinSize, "size", "s", 0, "File size in bytes (required when reading from stdin)")

	// Upload Tuning flags
	uploadCmd.Flags().IntVarP(&workers, "workers", "w", 4, "Number of concurrent upload workers")
	uploadCmd.Flags().IntVar(&queueSize, "queue", 10, "Part queue buffer size")
	uploadCmd.Flags().IntVar(&maxMemory, "max-memory", 0, "Maximum memory usage in MB (0 = no limit)")
	uploadCmd.Flags().Int64Var(&minPartSize, "min-part-size", 5*1024*1024, "Minimum part size in bytes")
	uploadCmd.Flags().Int64Var(&maxPartSize, "max-part-size", 5*1024*1024*1024, "Maximum part size in bytes")
	uploadCmd.Flags().IntVar(&maxParts, "max-parts", 10000, "Maximum number of parts")

	// Retry Configuration flags
	uploadCmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum retry attempts per part")
	uploadCmd.Flags().IntVar(&retryDelay, "retry-delay", 1000, "Initial retry delay in milliseconds")
	uploadCmd.Flags().IntVar(&maxRetryDelay, "max-retry-delay", 30000, "Maximum retry delay in milliseconds")
	uploadCmd.Flags().IntVar(&retryMultiplier, "retry-multiplier", 2, "Backoff multiplier for retries")

	// Object Metadata flags
	uploadCmd.Flags().StringVar(&contentType, "content-type", "", "Content-Type (auto-detected if not set)")
	uploadCmd.Flags().StringVar(&contentDisposition, "content-disposition", "", "Content-Disposition header")
	uploadCmd.Flags().StringVar(&contentEncoding, "content-encoding", "", "Content-Encoding (e.g., gzip, br)")
	uploadCmd.Flags().StringVar(&contentLanguage, "content-language", "", "Content-Language")
	uploadCmd.Flags().StringVar(&cacheControl, "cache-control", "", "Cache-Control header")
	uploadCmd.Flags().StringArrayVar(&metadata, "metadata", nil, "Custom metadata (key=value, repeatable)")

	// Checksum flags
	uploadCmd.Flags().BoolVar(&calculateChecksum, "checksum", true, "Calculate checksum during upload")
	uploadCmd.Flags().StringVar(&checksumAlgorithm, "checksum-algorithm", "md5", "Checksum algorithm (md5, sha256)")

	// Output Configuration flags
	uploadCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress progress output")

	// Version command flags
	versionCmd.Flags().Bool("check-updates", false, "Check for available updates on GitHub")

	// Download command flags (reuse checksum flags from upload)
	downloadCmd.Flags().BoolVar(&calculateChecksum, "checksum", true, "Calculate checksum during download")
	downloadCmd.Flags().StringVar(&checksumAlgorithm, "checksum-algorithm", "md5", "Checksum algorithm (md5, sha256)")

	// List command flags
	listCmd.Flags().IntVar(&listMaxKeys, "max-keys", 1000, "Maximum number of keys to return")

	// Cleanup command flags
	cleanupCmd.Flags().StringVar(&cleanupPrefix, "prefix", "", "Only cleanup uploads with this prefix")
	cleanupCmd.Flags().StringVar(&cleanupOlderThan, "older-than", "", "Only cleanup uploads older than duration (e.g., 24h, 7d)")
	cleanupCmd.Flags().IntVar(&cleanupMaxResults, "max-results", 0, "Maximum number of uploads to list (0 = all)")
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "List uploads without deleting")
	cleanupCmd.Flags().BoolVar(&cleanupForce, "force", false, "Skip confirmation prompt")
}

func runUpload(cmd *cobra.Command, args []string) error {
	// Parse positional arguments
	key := args[0]
	source := args[1]

	// Validate S3 key
	if err := validateS3Key(key); err != nil {
		return fmt.Errorf("invalid S3 key: %w", err)
	}

	// Validate required configuration
	if accessKeyID == "" {
		return fmt.Errorf("S3_ACCESS_KEY_ID or --access-key is required")
	}
	if secretAccessKey == "" {
		return fmt.Errorf("S3_SECRET_ACCESS_KEY or --secret-key is required")
	}
	if bucket == "" {
		return fmt.Errorf("S3_BUCKET or --bucket is required")
	}

	// Determine input source type and open reader
	var reader io.Reader
	var fileSize int64
	var err error

	if source == "-" {
		// Read from stdin
		if stdinSize <= 0 {
			return fmt.Errorf("--size flag is required when reading from stdin")
		}
		reader = os.Stdin
		fileSize = stdinSize
		if !quiet {
			fmt.Fprintf(os.Stderr, "Reading from stdin\n")
		}
	} else if isURL(source) {
		// Download from URL
		if !quiet {
			fmt.Fprintf(os.Stderr, "Downloading from %s\n", source)
		}
		reader, fileSize, err = openURL(source)
		if err != nil {
			return fmt.Errorf("failed to open URL: %w", err)
		}
		defer reader.(io.ReadCloser).Close()
	} else {
		// Validate local file path
		if err := validateFilePath(source); err != nil {
			return fmt.Errorf("invalid file path: %w", err)
		}

		// Open local file
		reader, fileSize, err = openFile(source)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer reader.(io.ReadCloser).Close()
	}

	// Create service limits
	limits := streamup.ServiceLimits{
		MinPartSize: minPartSize,
		MaxPartSize: maxPartSize,
		MaxParts:    maxParts,
	}

	// Parse metadata key=value pairs
	metadataMap := make(map[string]string)
	for _, kv := range metadata {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid metadata format %q, expected key=value", kv)
		}

		// Validate metadata
		if err := validateMetadata(parts[0], parts[1]); err != nil {
			return fmt.Errorf("invalid metadata %q: %w", kv, err)
		}

		metadataMap[parts[0]] = parts[1]
	}

	// Create uploader configuration
	cfg := streamup.Config{
		AccessKeyID:        accessKeyID,
		SecretAccessKey:    secretAccessKey,
		Bucket:             bucket,
		Key:                key,
		FileSize:           fileSize,
		AccountID:          accountID,
		Endpoint:           endpoint,
		Region:             region,
		Workers:            workers,
		QueueSize:          queueSize,
		MaxMemoryMB:        maxMemory,
		ServiceLimits:      &limits,
		MaxRetries:         maxRetries,
		RetryDelay:         retryDelay,
		MaxRetryDelay:      maxRetryDelay,
		RetryMultiplier:    retryMultiplier,
		ContentType:        contentType,
		ContentDisposition: contentDisposition,
		ContentEncoding:    contentEncoding,
		ContentLanguage:    contentLanguage,
		CacheControl:       cacheControl,
		Metadata:           metadataMap,
		CalculateChecksum:  calculateChecksum,
		ChecksumAlgorithm:  checksumAlgorithm,
	}

	// Create progress bar if not quiet
	var bar *progressbar.ProgressBar
	if !quiet {
		bar = progressbar.DefaultBytes(
			fileSize,
			"Uploading",
		)
		cfg.ProgressCallback = func(bytesUploaded int64, partsUploaded int32) {
			bar.Set64(bytesUploaded)
		}
	}

	// Create uploader
	uploader, err := streamup.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create uploader: %w", err)
	}

	// Start upload
	err = uploader.Upload(reader)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	// Finish progress bar
	if bar != nil {
		bar.Finish()
		fmt.Fprintf(os.Stderr, "✓ Upload completed successfully\n")

		// Display checksum if calculated
		if calculateChecksum {
			checksum := uploader.GetChecksum()
			if checksum != "" {
				fmt.Fprintf(os.Stderr, "  %s: %s\n", checksumAlgorithm, checksum)
			}
		}
	}

	return nil
}

func runDownload(cmd *cobra.Command, args []string) error {
	// Parse positional arguments
	key := args[0]
	output := "-" // Default to stdout
	if len(args) == 2 {
		output = args[1]
	}

	// Validate S3 key
	if err := validateS3Key(key); err != nil {
		return fmt.Errorf("invalid S3 key: %w", err)
	}

	// Validate required configuration
	if accessKeyID == "" {
		return fmt.Errorf("S3_ACCESS_KEY_ID or --access-key is required")
	}
	if secretAccessKey == "" {
		return fmt.Errorf("S3_SECRET_ACCESS_KEY or --secret-key is required")
	}
	if bucket == "" {
		return fmt.Errorf("S3_BUCKET or --bucket is required")
	}

	// Determine if writing to stdout
	toStdout := output == "-"

	// Progress should be suppressed if writing to stdout (to avoid polluting output)
	// or if quiet flag is set
	showProgress := !toStdout && !quiet

	// Create downloader
	ctx := context.Background()
	downloader, err := streamup.NewDownloader(streamup.DownloadConfig{
		AccessKeyID:       accessKeyID,
		SecretAccessKey:   secretAccessKey,
		Bucket:            bucket,
		Key:               key,
		AccountID:         accountID,
		Endpoint:          endpoint,
		Region:            region,
		CalculateChecksum: calculateChecksum,
		ChecksumAlgorithm: checksumAlgorithm,
	})
	if err != nil {
		return fmt.Errorf("failed to create downloader: %w", err)
	}

	// Get object metadata first to show size
	size, err := downloader.GetSize(ctx)
	if err != nil {
		return fmt.Errorf("failed to get object size: %w", err)
	}

	// Open output writer
	var writer io.Writer
	if toStdout {
		writer = os.Stdout
	} else {
		// Validate output file path
		if err := validateFilePath(output); err != nil {
			return fmt.Errorf("invalid output path: %w", err)
		}

		f, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		writer = f
	}

	// Create progress bar if showing progress
	var bar *progressbar.ProgressBar
	if showProgress {
		bar = progressbar.DefaultBytes(
			size,
			"Downloading",
		)
		downloader.SetProgressCallback(func(downloaded int64) {
			bar.Set64(downloaded)
		})
	}

	// Download
	err = downloader.Download(ctx, writer)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Finish progress bar
	if bar != nil {
		bar.Finish()
		fmt.Fprintf(os.Stderr, "✓ Download completed successfully\n")

		// Display checksum if calculated
		if calculateChecksum {
			checksum := downloader.GetChecksum()
			if checksum != "" {
				fmt.Fprintf(os.Stderr, "  %s: %s\n", checksumAlgorithm, checksum)
			}
		}
	}

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	// Parse optional prefix argument
	prefix := ""
	if len(args) == 1 {
		prefix = args[0]
	}

	// Validate required configuration
	if accessKeyID == "" {
		return fmt.Errorf("S3_ACCESS_KEY_ID or --access-key is required")
	}
	if secretAccessKey == "" {
		return fmt.Errorf("S3_SECRET_ACCESS_KEY or --secret-key is required")
	}
	if bucket == "" {
		return fmt.Errorf("S3_BUCKET or --bucket is required")
	}

	// Create lister
	ctx := context.Background()
	lister, err := streamup.NewLister(streamup.ListConfig{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Bucket:          bucket,
		AccountID:       accountID,
		Endpoint:        endpoint,
		Region:          region,
		Prefix:          prefix,
		MaxKeys:         listMaxKeys,
	})
	if err != nil {
		return fmt.Errorf("failed to create lister: %w", err)
	}

	// List objects
	objects, err := lister.List(ctx)
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	// Display results
	if len(objects) == 0 {
		if prefix != "" {
			fmt.Fprintf(os.Stderr, "No objects found with prefix %q\n", prefix)
		} else {
			fmt.Fprintf(os.Stderr, "No objects found in bucket\n")
		}
		return nil
	}

	// Print header
	fmt.Printf("%-60s %12s  %s\n", "Key", "Size", "Last Modified")
	fmt.Printf("%s\n", strings.Repeat("-", 100))

	// Print objects
	var totalSize int64
	for _, obj := range objects {
		// Format size
		sizeStr := formatSize(obj.Size)

		// Format date
		dateStr := obj.LastModified.Format("2006-01-02 15:04:05")

		// Truncate key if too long
		key := obj.Key
		if len(key) > 60 {
			key = key[:57] + "..."
		}

		fmt.Printf("%-60s %12s  %s\n", key, sizeStr, dateStr)
		totalSize += obj.Size
	}

	// Print summary
	fmt.Printf("%s\n", strings.Repeat("-", 100))
	fmt.Printf("Total: %d objects, %s\n", len(objects), formatSize(totalSize))

	return nil
}

// formatSize formats a byte count in a human-readable way.
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// isURL checks if a string is a valid URL.
func isURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// validateFilePath validates a local file path for security issues.
func validateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("file path contains null bytes")
	}

	// Clean the path to resolve . and .. elements
	cleanPath := path

	// Check if path tries to escape current directory using ../
	// Allow absolute paths, but warn about suspicious patterns
	if strings.Contains(path, "..") {
		return fmt.Errorf("file path contains '..' which could indicate path traversal attempt")
	}

	// Check for control characters
	for i, r := range cleanPath {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return fmt.Errorf("file path contains control character at position %d", i)
		}
	}

	return nil
}

// validateMetadata validates metadata key-value pairs to prevent injection attacks.
func validateMetadata(key, value string) error {
	// Validate key
	if key == "" {
		return fmt.Errorf("metadata key cannot be empty")
	}

	// Check key length (AWS limit is 128 characters)
	if len(key) > 128 {
		return fmt.Errorf("metadata key too long (max 128 chars): %d chars", len(key))
	}

	// Check value length (AWS limit is 256 characters)
	if len(value) > 256 {
		return fmt.Errorf("metadata value too long (max 256 chars): %d chars", len(value))
	}

	// Check for null bytes in key
	if strings.Contains(key, "\x00") {
		return fmt.Errorf("metadata key contains null bytes")
	}

	// Check for null bytes in value
	if strings.Contains(value, "\x00") {
		return fmt.Errorf("metadata value contains null bytes")
	}

	// Check for control characters in key
	for i, r := range key {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("metadata key contains control character at position %d", i)
		}
	}

	// Check for newlines and carriage returns in value (CRLF injection)
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("metadata value contains newline characters (potential header injection)")
	}

	return nil
}

// validateS3Key validates that an S3 key is safe to use.
// It checks for path traversal sequences, null bytes, and control characters.
func validateS3Key(key string) error {
	if key == "" {
		return fmt.Errorf("S3 key cannot be empty")
	}

	// Check length (S3 maximum is 1024 bytes)
	if len(key) > 1024 {
		return fmt.Errorf("S3 key too long (max 1024 bytes): %d bytes", len(key))
	}

	// Check for null bytes
	if strings.Contains(key, "\x00") {
		return fmt.Errorf("S3 key contains null bytes")
	}

	// Check for control characters (0x00-0x1F, 0x7F)
	for i, r := range key {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("S3 key contains control character at position %d (code: 0x%02X)", i, r)
		}
	}

	// Check for path traversal sequences
	if strings.Contains(key, "../") || strings.Contains(key, "..\\") {
		return fmt.Errorf("S3 key contains path traversal sequence")
	}

	// Check for leading/trailing whitespace (often indicates mistakes)
	if strings.TrimSpace(key) != key {
		return fmt.Errorf("S3 key has leading or trailing whitespace")
	}

	// Warn about double slashes (not forbidden, but often a mistake)
	if strings.Contains(key, "//") {
		// This is just a warning, not an error - double slashes are valid in S3
		fmt.Fprintf(os.Stderr, "Warning: S3 key contains double slashes (//), is this intentional?\n")
	}

	return nil
}

// isPrivateIP checks if an IP address is in a private range.
func isPrivateIP(ip net.IP) bool {
	// Private IPv4 ranges
	privateRanges := []string{
		"10.0.0.0/8",        // Private network
		"172.16.0.0/12",     // Private network
		"192.168.0.0/16",    // Private network
		"127.0.0.0/8",       // Loopback
		"169.254.0.0/16",    // Link-local (AWS/GCP/Azure metadata)
		"0.0.0.0/8",         // Current network
		"224.0.0.0/4",       // Multicast
		"240.0.0.0/4",       // Reserved
		"fc00::/7",          // IPv6 private
		"fe80::/10",         // IPv6 link-local
		"::1/128",           // IPv6 loopback
		"ff00::/8",          // IPv6 multicast
	}

	for _, cidr := range privateRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

// validateURL performs SSRF protection by validating that a URL doesn't point to private networks.
func validateURL(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow HTTP and HTTPS
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (only http and https are allowed)", u.Scheme)
	}

	// Extract hostname
	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("URL must have a hostname")
	}

	// Block localhost variations
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return fmt.Errorf("access to localhost is not allowed")
	}

	// Resolve hostname to IP addresses
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname: %w", err)
	}

	// Check if any resolved IP is in a private range
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("access to private IP addresses is not allowed: %s resolves to %s", hostname, ip.String())
		}
	}

	return nil
}

// openFile opens a local file and returns its reader and size.
func openFile(path string) (io.Reader, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}

	return f, stat.Size(), nil
}

// openURL opens a URL and returns its reader and size.
// It first makes a HEAD request to verify the URL exists and get the Content-Length,
// then makes a GET request to stream the actual content.
func openURL(url string) (io.Reader, int64, error) {
	// Validate URL to prevent SSRF attacks
	if err := validateURL(url); err != nil {
		return nil, 0, fmt.Errorf("URL validation failed: %w", err)
	}

	// First, make a HEAD request to check if the URL exists and get size
	headResp, err := http.Head(url)
	if err != nil {
		return nil, 0, fmt.Errorf("HEAD request failed: %w", err)
	}
	headResp.Body.Close()

	// Check HTTP status from HEAD request
	if headResp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("URL check failed: HTTP %d: %s", headResp.StatusCode, headResp.Status)
	}

	// Get Content-Length from HEAD response
	size := headResp.ContentLength
	if size < 0 {
		return nil, 0, fmt.Errorf("server did not provide Content-Length header")
	}

	// Now make the actual GET request to download
	resp, err := http.Get(url)
	if err != nil {
		return nil, 0, fmt.Errorf("GET request failed: %w", err)
	}

	// Verify GET response is also OK
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Verify Content-Length matches HEAD response
	if resp.ContentLength >= 0 && resp.ContentLength != size {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("Content-Length mismatch: HEAD returned %d, GET returned %d", size, resp.ContentLength)
	}

	return resp.Body, size, nil
}

func runCleanup(cmd *cobra.Command, args []string) error {
	// Validate required configuration
	if accessKeyID == "" {
		return fmt.Errorf("S3_ACCESS_KEY_ID or --access-key is required")
	}
	if secretAccessKey == "" {
		return fmt.Errorf("S3_SECRET_ACCESS_KEY or --secret-key is required")
	}
	if bucket == "" {
		return fmt.Errorf("S3_BUCKET or --bucket is required")
	}

	// Parse older-than duration if provided
	var olderThan time.Duration
	if cleanupOlderThan != "" {
		var err error
		olderThan, err = time.ParseDuration(cleanupOlderThan)
		if err != nil {
			return fmt.Errorf("invalid --older-than duration %q: %w", cleanupOlderThan, err)
		}
	}

	// Create cleanup configuration
	cfg := streamup.CleanupConfig{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Bucket:          bucket,
		AccountID:       accountID,
		Endpoint:        endpoint,
		Region:          region,
		Prefix:          cleanupPrefix,
		OlderThan:       olderThan,
		MaxResults:      cleanupMaxResults,
		DryRun:          cleanupDryRun,
	}

	// Run cleanup
	ctx := context.Background()
	result, err := streamup.CleanupIncompleteUploads(ctx, cfg)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	// Display results
	if result.TotalFound == 0 {
		fmt.Fprintf(os.Stderr, "No incomplete multipart uploads found.\n")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Found %d incomplete multipart upload(s):\n\n", result.TotalFound)

	// Display table of uploads
	fmt.Fprintf(os.Stderr, "%-60s %-40s %-20s\n", "Key", "Upload ID", "Initiated")
	fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat("-", 120))
	for _, upload := range result.Uploads {
		fmt.Fprintf(os.Stderr, "%-60s %-40s %-20s\n",
			truncate(upload.Key, 60),
			upload.UploadID,
			upload.Initiated.Format("2006-01-02 15:04:05"))
	}
	fmt.Fprintf(os.Stderr, "\n")

	// If dry-run, just list and exit
	if cleanupDryRun {
		fmt.Fprintf(os.Stderr, "Dry-run mode: no uploads were deleted.\n")
		fmt.Fprintf(os.Stderr, "Run without --dry-run to actually delete these uploads.\n")
		return nil
	}

	// Ask for confirmation unless --force
	if !cleanupForce {
		fmt.Fprintf(os.Stderr, "This will abort %d incomplete upload(s). Are you sure? (yes/no): ", result.TotalFound)
		var response string
		fmt.Scanln(&response)
		if response != "yes" && response != "y" {
			fmt.Fprintf(os.Stderr, "Aborted.\n")
			return nil
		}
	}

	// Display results
	if result.TotalAborted > 0 {
		fmt.Fprintf(os.Stderr, "✓ Successfully aborted %d upload(s)\n", result.TotalAborted)
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\n⚠ Encountered %d error(s):\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
		return fmt.Errorf("cleanup completed with errors")
	}

	return nil
}

// truncate truncates a string to a maximum length, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func main() {
	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
