package storage

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// BucketStrategy defines how objects of a given kind should be bucketed
// Multiple strategies can be active concurrently for the same object kind
// Note: This is different from BucketingStrategy (string enum) in bucketing_config.go
type BucketStrategy interface {
	// GetBucketKey returns the bucket key for an object
	// The key is used to group objects and determine which hash registry to use
	// Returns empty string if the object should not be bucketed
	GetBucketKey(obj map[string]any, filePath string) string

	// GetBucketDirectory returns the directory path where objects in this bucket should be stored
	// This is used for file system organization
	GetBucketDirectory(baseDir, bucketKey string) string

	// GetHashRegistryKey returns the key used for the hash registry pool
	// This should be unique per (kind, bucket) combination
	GetHashRegistryKey(kind, bucketKey string) string

	// Name returns a human-readable name for this strategy
	Name() string
}

// ChronoBucketStrategy buckets objects by time (e.g., monthly, weekly, daily, hourly, etc.)
type ChronoBucketStrategy struct {
	Field       string                          // Field name containing timestamp (e.g., "created_at")
	Format      string                          // Time format for directory (e.g., "2006-01" for monthly) or predefined granularity
	Granularity string                          // Predefined granularity: monthly, weekly, daily, hourly, half_hourly, qtr_hourly, tenths
	ParseFunc   func(string) (time.Time, error) // Function to parse timestamp from object
}

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

func (s *SizeBucketStrategy) GetBucketKey(obj map[string]any, filePath string) string {
	sizeVal, ok := obj[s.Field]
	if !ok {
		// Fallback to file size
		return s.getBucketKeyFromFileSize(filePath)
	}

	var size int64
	switch v := sizeVal.(type) {
	case int:
		size = int64(v)
	case int64:
		size = v
	case float64:
		size = int64(v)
	default:
		return s.getBucketKeyFromFileSize(filePath)
	}

	// Find matching range
	for _, r := range s.Ranges {
		if size >= r.Min && size <= r.Max {
			return r.Label
		}
	}

	return "unknown"
}

func (s *SizeBucketStrategy) getBucketKeyFromFileSize(filePath string) string {
	// Fallback: use actual file size
	// This would require file system access, so we'll return a default
	return "unknown"
}

func (s *SizeBucketStrategy) GetBucketDirectory(baseDir, bucketKey string) string {
	if bucketKey == emptyValue {
		return baseDir
	}
	return filepath.Join(baseDir, bucketKey)
}

func (s *SizeBucketStrategy) GetHashRegistryKey(kind, bucketKey string) string {
	return fmt.Sprintf("%s:size:%s", kind, bucketKey)
}

func (s *SizeBucketStrategy) Name() string {
	return fmt.Sprintf("size-%s", s.Field)
}

// CompositeBucketStrategy combines multiple strategies
// Objects are bucketed using all strategies, creating a composite key
type CompositeBucketStrategy struct {
	Strategies []BucketStrategy
	Separator  string // Separator between strategy keys (default: "/")
}

func (s *CompositeBucketStrategy) GetBucketKey(obj map[string]any, filePath string) string {
	if len(s.Strategies) == 0 {
		return ""
	}

	separator := s.Separator
	if separator == emptyValue {
		separator = "/"
	}

	keys := make([]string, 0, len(s.Strategies))
	for _, strategy := range s.Strategies {
		key := strategy.GetBucketKey(obj, filePath)
		if key != emptyValue {
			keys = append(keys, key)
		}
	}

	if len(keys) == 0 {
		return ""
	}

	// Combine keys with separator
	result := keys[0]
	for i := 1; i < len(keys); i++ {
		result = result + separator + keys[i]
	}
	return result
}

func (s *CompositeBucketStrategy) GetBucketDirectory(baseDir, bucketKey string) string {
	if bucketKey == emptyValue {
		return baseDir
	}
	// Composite keys may contain separators, split and join as path components
	parts := filepath.SplitList(bucketKey)
	if len(parts) == 0 {
		return baseDir
	}
	allParts := append([]string{baseDir}, parts...)
	return filepath.Join(allParts...)
}

func (s *CompositeBucketStrategy) GetHashRegistryKey(kind, bucketKey string) string {
	return fmt.Sprintf(FmtCompositeBucket, kind, bucketKey)
}

func (s *CompositeBucketStrategy) Name() string {
	names := make([]string, len(s.Strategies))
	for i, strategy := range s.Strategies {
		names[i] = strategy.Name()
	}
	return fmt.Sprintf("composite[%s]", fmt.Sprintf("%v", names))
}

// FirstLetterBucketStrategy buckets objects by the first character of a string field (e.g. title).
// Used for glossary_term: A -> agent_guidelines, P -> process data, etc. Bucket key is one lowercase letter (a-z) or "_" for non-letter/digit/empty.
type FirstLetterBucketStrategy struct {
	Field string // Field name to derive bucket from (e.g., "title")
}

func (s *FirstLetterBucketStrategy) GetBucketKey(obj map[string]any, _ string) string {
	val, ok := obj[s.Field]
	if !ok {
		return firstLetterBucketKey("")
	}
	str, ok := val.(string)
	if !ok {
		return firstLetterBucketKey("")
	}
	return firstLetterBucketKey(strings.TrimSpace(str))
}

func firstLetterBucketKey(s string) string {
	if s == emptyValue {
		return "_"
	}
	var first rune
	for _, first = range s {
		break
	}
	first = unicode.ToLower(first)
	if unicode.IsLetter(first) {
		return string(first)
	}
	if unicode.IsDigit(first) {
		return "0"
	}
	return "_"
}

func (s *FirstLetterBucketStrategy) GetBucketDirectory(baseDir, bucketKey string) string {
	if bucketKey == emptyValue {
		return baseDir
	}
	return filepath.Join(baseDir, bucketKey)
}

func (s *FirstLetterBucketStrategy) GetHashRegistryKey(kind, bucketKey string) string {
	return fmt.Sprintf(FmtFirstLetterBucket, kind, bucketKey)
}

func (s *FirstLetterBucketStrategy) Name() string {
	return "first-letter"
}

// PathBasedBucketStrategy buckets objects based on their file path
// This is the current default behavior (directory-based)
type PathBasedBucketStrategy struct {
	BaseDir string // Base directory for this kind
}

func (s *PathBasedBucketStrategy) GetBucketKey(obj map[string]any, filePath string) string {
	// Extract bucket from path (e.g., audit/2026-01/file.yaml -> 2026-01)
	relPath, err := filepath.Rel(s.BaseDir, filepath.Dir(filePath))
	if err != nil {
		return ""
	}

	// If relative path is ".", object is in base directory (not bucketed)
	if relPath == "." || relPath == emptyValue {
		return ""
	}

	// Return the first directory component as bucket key
	parts := filepath.SplitList(relPath)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func (s *PathBasedBucketStrategy) GetBucketDirectory(baseDir, bucketKey string) string {
	if bucketKey == emptyValue {
		return baseDir
	}
	return filepath.Join(baseDir, bucketKey)
}

func (s *PathBasedBucketStrategy) GetHashRegistryKey(kind, bucketKey string) string {
	return fmt.Sprintf("%s:path:%s", kind, bucketKey)
}

func (s *PathBasedBucketStrategy) Name() string {
	return "path-based"
}

// BucketStrategyRegistry manages bucketing strategies per object kind
// Supports multiple strategies per kind (e.g., chrono + state)
type BucketStrategyRegistry struct {
	strategies map[string][]BucketStrategy // kind -> []BucketStrategy
}

// NewBucketStrategyRegistry creates a new registry
func NewBucketStrategyRegistry() *BucketStrategyRegistry {
	return &BucketStrategyRegistry{
		strategies: make(map[string][]BucketStrategy),
	}
}

// RegisterStrategy registers a bucketing strategy for an object kind
// Multiple strategies can be registered for the same kind
func (r *BucketStrategyRegistry) RegisterStrategy(kind string, strategy BucketStrategy) {
	r.strategies[kind] = append(r.strategies[kind], strategy)
}

// GetStrategies returns all strategies for an object kind
func (r *BucketStrategyRegistry) GetStrategies(kind string) []BucketStrategy {
	return r.strategies[kind]
}

// GetBucketKey returns the composite bucket key for an object using all registered strategies
func (r *BucketStrategyRegistry) GetBucketKey(kind string, obj map[string]any, filePath string) string {
	strategies := r.GetStrategies(kind)
	if len(strategies) == 0 {
		// No strategies registered - use path-based as default
		return ""
	}

	if len(strategies) == 1 {
		// Single strategy
		return strategies[0].GetBucketKey(obj, filePath)
	}

	// Multiple strategies - use composite
	composite := &CompositeBucketStrategy{
		Strategies: strategies,
		Separator:  "/",
	}
	return composite.GetBucketKey(obj, filePath)
}

// GetHashRegistryKey returns the hash registry key for an object
func (r *BucketStrategyRegistry) GetHashRegistryKey(kind string, obj map[string]any, filePath string) string {
	strategies := r.GetStrategies(kind)
	if len(strategies) == 0 {
		// Default: use path-based key (current behavior)
		baseDir := filepath.Dir(filePath)
		relPath, err := filepath.Rel(baseDir, filepath.Dir(filePath))
		if err != nil || relPath == "." || relPath == emptyValue {
			return fmt.Sprintf("%s:", kind) // Non-bucketed
		}
		return fmt.Sprintf("%s:%s", kind, relPath)
	}

	bucketKey := r.GetBucketKey(kind, obj, filePath)
	if bucketKey == emptyValue {
		return fmt.Sprintf("%s:", kind) // Non-bucketed
	}

	if len(strategies) == 1 {
		return strategies[0].GetHashRegistryKey(kind, bucketKey)
	}

	// Multiple strategies - use composite
	return fmt.Sprintf(FmtCompositeBucket, kind, bucketKey)
}

// Helper functions

func extractTimeFromPath(filePath string) string {
	// Try to extract time from path (e.g., audit/2026-01/file.yaml -> 2026-01)
	dir := filepath.Dir(filePath)
	base := filepath.Base(dir)

	// Check if base looks like a time format (YYYY-MM, YYYY-MM-DD, etc.)
	if len(base) >= 7 && base[4] == '-' {
		return base
	}
	return ""
}

func normalizeBucketKey(key string) string {
	// Convert to lowercase and replace spaces with underscores
	result := ""
	for _, r := range key {
		if r == ' ' {
			result += "_"
		} else {
			result += string(r)
		}
	}
	return result
}

// Default strategies for common use cases

// NewMonthlyChronoStrategy creates a monthly chrono bucketing strategy
func NewMonthlyChronoStrategy(field string) *ChronoBucketStrategy {
	return &ChronoBucketStrategy{
		Field:  field,
		Format: "2006-01", // YYYY-MM
		ParseFunc: func(s string) (time.Time, error) {
			return time.Parse(time.RFC3339, s)
		},
	}
}

// NewYearlyChronoStrategy creates a yearly chrono bucketing strategy
func NewYearlyChronoStrategy(field string) *ChronoBucketStrategy {
	return &ChronoBucketStrategy{
		Field:  field,
		Format: "2006", // YYYY
		ParseFunc: func(s string) (time.Time, error) {
			return time.Parse(time.RFC3339, s)
		},
	}
}

// NewStatusStateStrategy creates a state-based bucketing strategy using status field
func NewStatusStateStrategy() *StateBucketStrategy {
	return &StateBucketStrategy{
		Field: "status",
	}
}
