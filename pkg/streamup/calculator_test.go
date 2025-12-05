package streamup

import (
	"testing"
)

func TestCalculateOptimalPartSize(t *testing.T) {
	tests := []struct {
		name         string
		fileSize     int64
		maxMemoryMB  int
		workers      int
		queueSize    int
		limits       ServiceLimits
		wantPartSize int64
		wantErr      bool
		errContains  string
	}{
		{
			name:         "70GB OSM planet - should target ~1000 parts",
			fileSize:     70 * 1024 * 1024 * 1024, // 70 GB
			maxMemoryMB:  0,                        // No memory constraint
			workers:      4,
			queueSize:    10,
			limits:       DefaultS3Limits(),
			wantPartSize: 70 * 1024 * 1024, // 70 MB parts
			wantErr:      false,
		},
		{
			name:         "500GB file - should target ~1000 parts",
			fileSize:     500 * 1024 * 1024 * 1024, // 500 GB
			maxMemoryMB:  0,
			workers:      4,
			queueSize:    10,
			limits:       DefaultS3Limits(),
			wantPartSize: 500 * 1024 * 1024, // 500 MB parts
			wantErr:      false,
		},
		{
			name:         "5TB file - should use max 5GB parts",
			fileSize:     5 * 1024 * 1024 * 1024 * 1024, // 5 TB
			maxMemoryMB:  0,
			workers:      4,
			queueSize:    10,
			limits:       DefaultS3Limits(),
			wantPartSize: 5 * 1024 * 1024 * 1024, // 5 GB parts (max)
			wantErr:      false,
		},
		{
			name:         "Small 100MB file - should use min 5MB parts",
			fileSize:     100 * 1024 * 1024, // 100 MB
			maxMemoryMB:  0,
			workers:      4,
			queueSize:    10,
			limits:       DefaultS3Limits(),
			wantPartSize: 5 * 1024 * 1024, // 5 MB parts (min)
			wantErr:      false,
		},
		{
			name:         "Memory constrained 500GB - 2GB RAM limit",
			fileSize:     500 * 1024 * 1024 * 1024, // 500 GB
			maxMemoryMB:  2048,                      // 2 GB RAM
			workers:      4,
			queueSize:    10,
			limits:       DefaultS3Limits(),
			wantPartSize: 146 * 1024 * 1024, // ~146 MB (2048MB / 14 slots)
			wantErr:      false,
		},
		{
			name:         "File too large for service limits",
			fileSize:     60 * 1024 * 1024 * 1024 * 1024, // 60 TB (exceeds 50TB limit)
			maxMemoryMB:  0,
			workers:      4,
			queueSize:    10,
			limits:       DefaultS3Limits(),
			wantPartSize: 0,
			wantErr:      true,
			errContains:  "exceeds service limit",
		},
		{
			name:         "Custom service limits - 10MB min",
			fileSize:     100 * 1024 * 1024 * 1024, // 100 GB
			maxMemoryMB:  0,
			workers:      4,
			queueSize:    10,
			limits: ServiceLimits{
				MinPartSize: 10 * 1024 * 1024,       // 10 MB min
				MaxPartSize: 5 * 1024 * 1024 * 1024, // 5 GB max
				MaxParts:    5000,                    // 5000 parts max
			},
			wantPartSize: 100 * 1024 * 1024, // 100 MB parts
			wantErr:      false,
		},
		{
			name:         "Very small file - 1MB",
			fileSize:     1 * 1024 * 1024, // 1 MB
			maxMemoryMB:  0,
			workers:      4,
			queueSize:    10,
			limits:       DefaultS3Limits(),
			wantPartSize: 5 * 1024 * 1024, // 5 MB min
			wantErr:      false,
		},
		{
			name:         "Exact target parts - 1000 parts at 10GB",
			fileSize:     10 * 1024 * 1024 * 1024, // 10 GB
			maxMemoryMB:  0,
			workers:      4,
			queueSize:    10,
			limits:       DefaultS3Limits(),
			wantPartSize: 10 * 1024 * 1024, // 10 MB parts = exactly 1000 parts
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPartSize, err := CalculateOptimalPartSize(
				tt.fileSize,
				tt.maxMemoryMB,
				tt.workers,
				tt.queueSize,
				tt.limits,
			)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CalculateOptimalPartSize() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("CalculateOptimalPartSize() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("CalculateOptimalPartSize() unexpected error = %v", err)
				return
			}

			// Allow for rounding differences (within 10% tolerance)
			tolerance := int64(float64(tt.wantPartSize) * 0.1)
			if gotPartSize < tt.wantPartSize-tolerance || gotPartSize > tt.wantPartSize+tolerance {
				t.Errorf("CalculateOptimalPartSize() = %d, want %d (±%d)", gotPartSize, tt.wantPartSize, tolerance)
			}

			// Verify part size is within service limits
			if gotPartSize < tt.limits.MinPartSize {
				t.Errorf("CalculateOptimalPartSize() = %d, below minimum %d", gotPartSize, tt.limits.MinPartSize)
			}
			if gotPartSize > tt.limits.MaxPartSize {
				t.Errorf("CalculateOptimalPartSize() = %d, above maximum %d", gotPartSize, tt.limits.MaxPartSize)
			}

			// Verify total parts won't exceed limit
			totalParts := (tt.fileSize + gotPartSize - 1) / gotPartSize
			if totalParts > int64(tt.limits.MaxParts) {
				t.Errorf("CalculateOptimalPartSize() would create %d parts, exceeds max %d", totalParts, tt.limits.MaxParts)
			}
		})
	}
}

func TestCalculateOptimalPartSize_MemoryConstraints(t *testing.T) {
	tests := []struct {
		name        string
		fileSize    int64
		maxMemoryMB int
		workers     int
		queueSize   int
		wantMaxRAM  int64 // Maximum RAM usage in bytes
	}{
		{
			name:        "2GB RAM constraint with 70GB file",
			fileSize:    70 * 1024 * 1024 * 1024,
			maxMemoryMB: 2048,
			workers:     4,
			queueSize:   10,
			wantMaxRAM:  2048 * 1024 * 1024,
		},
		{
			name:        "512MB RAM constraint with 100GB file",
			fileSize:    100 * 1024 * 1024 * 1024,
			maxMemoryMB: 512,
			workers:     4,
			queueSize:   10,
			wantMaxRAM:  512 * 1024 * 1024,
		},
		{
			name:        "8GB RAM constraint with 1TB file",
			fileSize:    1024 * 1024 * 1024 * 1024,
			maxMemoryMB: 8192,
			workers:     8,
			queueSize:   20,
			wantMaxRAM:  8192 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			partSize, err := CalculateOptimalPartSize(
				tt.fileSize,
				tt.maxMemoryMB,
				tt.workers,
				tt.queueSize,
				DefaultS3Limits(),
			)
			if err != nil {
				t.Fatalf("CalculateOptimalPartSize() error = %v", err)
			}

			// Calculate actual RAM usage: partSize × (workers + queueSize)
			totalSlots := int64(tt.workers + tt.queueSize)
			actualRAM := partSize * totalSlots

			// Allow 5% tolerance for MB rounding effects
			tolerance := int64(float64(tt.wantMaxRAM) * 0.05)
			if actualRAM > tt.wantMaxRAM+tolerance {
				t.Errorf("RAM usage = %d bytes (%.2f GB), exceeds limit %d bytes (%.2f GB) with 5%% tolerance",
					actualRAM, float64(actualRAM)/(1024*1024*1024),
					tt.wantMaxRAM, float64(tt.wantMaxRAM)/(1024*1024*1024))
			}
		})
	}
}

func TestCalculateOptimalPartSize_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		fileSize    int64
		maxMemoryMB int
		workers     int
		queueSize   int
		limits      ServiceLimits
		wantErr     bool
		errContains string
	}{
		{
			name:        "Invalid limits - min > max",
			fileSize:    10 * 1024 * 1024 * 1024,
			maxMemoryMB: 0,
			workers:     4,
			queueSize:   10,
			limits: ServiceLimits{
				MinPartSize: 100 * 1024 * 1024,
				MaxPartSize: 10 * 1024 * 1024,
				MaxParts:    10000,
			},
			wantErr:     true,
			errContains: "cannot be greater than MaxPartSize",
		},
		{
			name:        "Invalid limits - zero max parts",
			fileSize:    10 * 1024 * 1024 * 1024,
			maxMemoryMB: 0,
			workers:     4,
			queueSize:   10,
			limits: ServiceLimits{
				MinPartSize: 5 * 1024 * 1024,
				MaxPartSize: 5 * 1024 * 1024 * 1024,
				MaxParts:    0,
			},
			wantErr:     true,
			errContains: "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculateOptimalPartSize(
				tt.fileSize,
				tt.maxMemoryMB,
				tt.workers,
				tt.queueSize,
				tt.limits,
			)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CalculateOptimalPartSize() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("CalculateOptimalPartSize() error = %v, want error containing %q", err, tt.errContains)
				}
			} else if err != nil {
				t.Errorf("CalculateOptimalPartSize() unexpected error = %v", err)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
