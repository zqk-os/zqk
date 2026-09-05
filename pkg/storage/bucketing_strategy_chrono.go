// Extracted from bucketing_strategy.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"fmt"
	"path/filepath"
)

func (s *ChronoBucketStrategy) GetBucketKey(obj map[string]any, filePath string) string {
	// Extract timestamp from object
	timestampVal, ok := obj[s.Field]
	if !ok {
		// Fallback to file modification time or current time
		return ""
	}

	timestampStr, ok := timestampVal.(string)
	if !ok {
		return ""
	}

	parsedTime, err := s.ParseFunc(timestampStr)
	if err != nil {
		// Fallback: try to extract from file path (e.g., audit/2026-01/)
		return extractTimeFromPath(filePath)
	}

	// Use granularity if specified (handles special cases like weekly, rounding)
	if s.Granularity != emptyValue {
		return formatTimeByGranularity(parsedTime, s.Granularity)
	}

	// Otherwise use the format string directly
	if s.Format != emptyValue {
		return parsedTime.Format(s.Format)
	}

	// Default to monthly if neither specified
	return parsedTime.Format("2006-01")
}

func (s *ChronoBucketStrategy) GetBucketDirectory(baseDir, bucketKey string) string {
	if bucketKey == emptyValue {
		return baseDir
	}

	// For granularities with time components, create nested directories
	// e.g., hourly: 2026-01-02/15, half_hourly: 2026-01-02/15:04
	if s.Granularity != emptyValue {
		return s.getNestedBucketDirectory(baseDir, bucketKey)
	}

	// For custom format strings, check if they contain time components
	if s.Format != emptyValue && len(bucketKey) >= 13 && bucketKey[10] == 'T' {
		// Has time component - use nested structure
		datePart := bucketKey[:10]
		timePart := bucketKey[11:]
		return filepath.Join(baseDir, datePart, timePart)
	}

	return filepath.Join(baseDir, bucketKey)
}

// getNestedBucketDirectory creates nested directory structure for time-based bucketing
// e.g., hourly: 2026-01-02/15, half_hourly: 2026-01-02/15:04
func (s *ChronoBucketStrategy) getNestedBucketDirectory(baseDir, bucketKey string) string {
	switch s.Granularity {
	case "hourly":
		// bucketKey format: "2006-01-02T15" -> "2006-01-02/15"
		if len(bucketKey) >= 13 && bucketKey[10] == 'T' {
			datePart := bucketKey[:10]
			hourPart := bucketKey[11:]
			return filepath.Join(baseDir, datePart, hourPart)
		}
	case "half_hourly", "qtr_hourly":
		// bucketKey format: "2006-01-02T15:04" -> "2006-01-02/15:04"
		if len(bucketKey) >= 16 && bucketKey[10] == 'T' {
			datePart := bucketKey[:10]
			timePart := bucketKey[11:]
			return filepath.Join(baseDir, datePart, timePart)
		}
	case "tenths":
		// bucketKey format: "2006-01-02T15:04:05.9" -> "2006-01-02/15:04:05.9"
		if len(bucketKey) >= 20 && bucketKey[10] == 'T' {
			datePart := bucketKey[:10]
			timePart := bucketKey[11:]
			return filepath.Join(baseDir, datePart, timePart)
		}
	case "weekly":
		// bucketKey format: "2026-W01" -> flat structure (no nesting needed)
		return filepath.Join(baseDir, bucketKey)
	case "daily", "monthly":
		// Flat structure for daily and monthly
		return filepath.Join(baseDir, bucketKey)
	}

	// Default: flat structure
	return filepath.Join(baseDir, bucketKey)
}

func (s *ChronoBucketStrategy) GetHashRegistryKey(kind, bucketKey string) string {
	return fmt.Sprintf("%s:chrono:%s", kind, bucketKey)
}

func (s *ChronoBucketStrategy) Name() string {
	if s.Granularity != emptyValue {
		return fmt.Sprintf("chrono-%s", s.Granularity)
	}
	if s.Format != emptyValue {
		return fmt.Sprintf("chrono-%s", s.Format)
	}
	return StrategyChronoDefault
}

// StateBucketStrategy buckets objects by state/status field
type StateBucketStrategy struct {
	Field string // Field name containing state (e.g., "status", "state")
}

func (s *StateBucketStrategy) GetBucketKey(obj map[string]any, filePath string) string {
	stateVal, ok := obj[s.Field]
	if !ok {
		return "unknown"
	}

	stateStr, ok := stateVal.(string)
	if !ok {
		return "unknown"
	}

	// Normalize state (lowercase, replace spaces with underscores)
	return normalizeBucketKey(stateStr)
}

func (s *StateBucketStrategy) GetBucketDirectory(baseDir, bucketKey string) string {
	if bucketKey == emptyValue {
		return baseDir
	}
	return filepath.Join(baseDir, bucketKey)
}

func (s *StateBucketStrategy) GetHashRegistryKey(kind, bucketKey string) string {
	return fmt.Sprintf("%s:state:%s", kind, bucketKey)
}

func (s *StateBucketStrategy) Name() string {
	return fmt.Sprintf("state-%s", s.Field)
}

// SizeBucketStrategy buckets objects by size ranges
type SizeBucketStrategy struct {
	Field  string      // Field name containing size (e.g., "size", "file_size")
	Ranges []SizeRange // Size ranges for bucketing
	Unit   string      // Unit for size (e.g., "bytes", "kb", "mb")
}

type SizeRange struct {
	Min   int64
	Max   int64
	Label string // Bucket label (e.g., "small", "medium", "large")
}
