package streamup

import (
	"testing"
)

func TestDetectContentType(t *testing.T) {
	// Note: MIME types vary significantly across systems and Go versions.
	// We only test the most stable, widely-agreed-upon types.
	// System-dependent types (wav, exe, xml, gz, zip, etc.) are skipped.
	// Windows returns "application/x-zip-compressed" for .zip files while
	// Unix systems return "application/zip".
	tests := []struct {
		filename    string
		wantType    string
		description string
	}{
		// Common web types - very stable
		{"file.html", "text/html", "HTML file"},
		{"file.css", "text/css", "CSS file"},
		{"file.json", "application/json", "JSON file"},

		// Images - standardized
		{"image.png", "image/png", "PNG image"},
		{"image.jpg", "image/jpeg", "JPEG image"},
		{"image.gif", "image/gif", "GIF image"},

		// Documents - stable
		{"doc.pdf", "application/pdf", "PDF document"},

		// Binary - fallback
		{"file.bin", "application/octet-stream", "Binary file"},
		{"noextension", "application/octet-stream", "File without extension"},

		// Case insensitive
		{"FILE.JSON", "application/json", "Uppercase extension"},
		{"file.PDF", "application/pdf", "Uppercase PDF"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := DetectContentType(tt.filename)
			if got != tt.wantType {
				t.Errorf("DetectContentType(%q) = %q, want %q", tt.filename, got, tt.wantType)
			}
		})
	}
}

func TestGetContentEncoding(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"file.gz", "gzip"},
		{"file.gzip", "gzip"},
		{"file.br", "br"},
		{"file.zst", "zstd"},
		{"file.txt", ""},
		{"file.json", ""},
		{"noextension", ""},
		{"FILE.GZ", "gzip"}, // Case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := GetContentEncoding(tt.filename)
			if got != tt.want {
				t.Errorf("GetContentEncoding(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestShouldCompress(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		// Should compress
		{"text/html", true},
		{"text/css", true},
		{"text/plain", true},
		{"text/javascript", true},
		{"application/json", true},
		{"application/javascript", true},
		{"application/xml", true},
		{"application/xhtml+xml", true},
		{"application/ld+json", true},
		{"image/svg+xml", true},

		// Should not compress
		{"image/png", false},
		{"image/jpeg", false},
		{"video/mp4", false},
		{"audio/mpeg", false},
		{"application/zip", false},
		{"application/pdf", false},
		{"application/octet-stream", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := ShouldCompress(tt.contentType)
			if got != tt.want {
				t.Errorf("ShouldCompress(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}
