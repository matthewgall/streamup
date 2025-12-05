package streamup

import (
	"mime"
	"path/filepath"
	"strings"
)

// Common content types that might not be in mime.TypeByExtension
var customContentTypes = map[string]string{
	".json":   "application/json",
	".jsonld": "application/ld+json",
	".map":    "application/json",
	".webp":   "image/webp",
	".woff":   "font/woff",
	".woff2":  "font/woff2",
	".ttf":    "font/ttf",
	".otf":    "font/otf",
	".eot":    "application/vnd.ms-fontobject",
	".md":     "text/markdown",
	".markdown": "text/markdown",
	".yml":    "text/yaml",
	".yaml":   "text/yaml",
	".toml":   "application/toml",
	".ts":     "application/typescript",
	".tsx":    "application/typescript",
	".mjs":    "application/javascript",
	".cjs":    "application/javascript",
	".pbf":    "application/octet-stream",
	".br":     "application/x-br",
	".zst":    "application/zstd",
}

// DetectContentType returns the MIME type for a given filename.
// It uses the standard library mime package and supplements with common web types.
func DetectContentType(filename string) string {
	// Get extension (lowercase)
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return "application/octet-stream"
	}

	// Check custom types first
	if ct, ok := customContentTypes[ext]; ok {
		return ct
	}

	// Use standard library
	ct := mime.TypeByExtension(ext)
	if ct != "" {
		// mime.TypeByExtension returns charset parameters, just get the type
		if idx := strings.Index(ct, ";"); idx > 0 {
			ct = strings.TrimSpace(ct[:idx])
		}
		return ct
	}

	// Default to binary
	return "application/octet-stream"
}

// GetContentEncoding returns the content encoding based on file extension.
// Returns empty string if no encoding is detected.
func GetContentEncoding(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".gz", ".gzip":
		return "gzip"
	case ".br":
		return "br"
	case ".zst":
		return "zstd"
	default:
		return ""
	}
}

// ShouldCompress returns true if the content type should be compressed during upload.
// This is useful for text-based content types.
func ShouldCompress(contentType string) bool {
	compressibleTypes := []string{
		"text/",
		"application/json",
		"application/javascript",
		"application/xml",
		"application/xhtml+xml",
		"application/ld+json",
		"image/svg+xml",
	}

	for _, prefix := range compressibleTypes {
		if strings.HasPrefix(contentType, prefix) {
			return true
		}
	}

	return false
}
