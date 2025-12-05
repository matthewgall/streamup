package streamup

import (
	"fmt"
	"math"
)

const (
	// Target number of parts for optimal performance
	targetParts = 1000
	// Round to nearest MB for clean numbers
	mbSize = 1024 * 1024
)

// CalculateOptimalPartSize determines the best part size for a given file size
// while respecting service limits and optional memory constraints.
//
// The algorithm aims for approximately 1000 parts for best performance, which
// provides a good balance between API overhead and parallelism, and makes
// progress tracking intuitive (1% per part).
//
// Parameters:
//   - fileSize: Total size of the file to upload in bytes
//   - maxMemoryMB: Optional memory limit in MB (0 = no limit)
//   - workers: Number of concurrent upload workers
//   - queueSize: Size of the part queue buffer
//   - limits: Service-specific constraints (min/max part size, max parts)
//
// Returns:
//   - Optimal part size in bytes (rounded to nearest MB)
//   - Error if file size exceeds service limits
//
// Memory Formula: partSize × (workers + queueSize) = total RAM usage
// Default: partSize × (4 + 10) = partSize × 14
//
// Examples:
//   - 70 GB file → 70 MB parts → 1000 parts → ~1 GB RAM
//   - 500 GB file → 500 MB parts → 1000 parts → ~7 GB RAM
//   - 5 TB file → 5 GB parts → 1000 parts → ~70 GB RAM
//   - With 2GB limit: 500 GB → 146 MB parts → 3425 parts → ~2 GB RAM
func CalculateOptimalPartSize(fileSize int64, maxMemoryMB, workers, queueSize int, limits ServiceLimits) (int64, error) {
	// Validate service limits first
	if err := limits.Validate(); err != nil {
		return 0, err
	}

	// Check if file size exceeds service maximum
	maxFileSize := limits.MaxFileSize()
	if fileSize > maxFileSize {
		return 0, fmt.Errorf("file size %d bytes exceeds service limit of %d bytes (%d GB)",
			fileSize, maxFileSize, maxFileSize/(1024*1024*1024))
	}

	// Calculate ideal part size for target number of parts
	// Target ~1000 parts for optimal performance
	idealPartSize := fileSize / targetParts

	// If we have a memory constraint, calculate the maximum part size allowed
	var memoryConstrainedPartSize int64
	if maxMemoryMB > 0 {
		// Memory formula: partSize × (workers + queueSize) = total RAM
		totalSlots := workers + queueSize
		memoryConstrainedPartSize = int64(maxMemoryMB) * mbSize / int64(totalSlots)
	}

	// Start with the ideal part size
	partSize := idealPartSize

	// Apply memory constraint if specified
	if maxMemoryMB > 0 && partSize > memoryConstrainedPartSize {
		partSize = memoryConstrainedPartSize
	}

	// Round to nearest MB for clean numbers
	partSize = roundToNearestMB(partSize)

	// Enforce minimum part size
	if partSize < limits.MinPartSize {
		partSize = limits.MinPartSize
	}

	// Enforce maximum part size
	if partSize > limits.MaxPartSize {
		partSize = limits.MaxPartSize
	}

	// Calculate how many parts we'll actually need
	actualParts := int(math.Ceil(float64(fileSize) / float64(partSize)))

	// Ensure we don't exceed the maximum parts limit
	if actualParts > limits.MaxParts {
		// Need to increase part size to stay under max parts
		partSize = int64(math.Ceil(float64(fileSize) / float64(limits.MaxParts)))
		partSize = roundToNearestMB(partSize)

		// Verify this doesn't exceed max part size
		if partSize > limits.MaxPartSize {
			return 0, fmt.Errorf("file size %d bytes cannot be uploaded with given limits (would require part size %d MB, max is %d MB)",
				fileSize, partSize/mbSize, limits.MaxPartSize/mbSize)
		}
	}

	return partSize, nil
}

// roundToNearestMB rounds a size to the nearest megabyte.
func roundToNearestMB(size int64) int64 {
	remainder := size % mbSize
	if remainder < mbSize/2 {
		return size - remainder
	}
	return size + (mbSize - remainder)
}

// CalculateMemoryUsage estimates total memory usage for given parameters.
func CalculateMemoryUsage(partSize int64, workers, queueSize int) int64 {
	return partSize * int64(workers+queueSize)
}

// CalculatePartCount calculates the number of parts needed for a file.
func CalculatePartCount(fileSize, partSize int64) int {
	return int(math.Ceil(float64(fileSize) / float64(partSize)))
}
