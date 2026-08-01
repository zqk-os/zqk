package storage

import (
	"os"
	"path/filepath"
)

// SyncValidationStrategy implements synchronous file existence validation.
// This is the default strategy that validates each mapping by checking if the hash file exists.
//
// Performance characteristics:
// - O(n) where n = number of mappings
// - Each mapping requires one os.Stat call (~1-10μs per file on local storage)
// - Suitable for batch sizes up to ~1000 entries on local storage
//
// For higher volumes or slow storage (NFS, network), consider AsyncCacheValidationStrategy.
type SyncValidationStrategy struct {
	// metrics tracks validation performance for this strategy instance
	metrics *ValidationMetrics
}

// NewSyncValidationStrategy creates a new synchronous validation strategy.
func NewSyncValidationStrategy() *SyncValidationStrategy {
	return &SyncValidationStrategy{
		metrics: &ValidationMetrics{},
	}
}

// ValidateMappings checks each mapping synchronously by verifying the hash file exists.
// Returns only mappings where the corresponding hash file exists on disk.
func (s *SyncValidationStrategy) ValidateMappings(kindDir string, mappings map[string]string, bucketKeys map[string]string) (validMappings map[string]string, validBucketKeys map[string]string, staleCount int) {
	if len(mappings) == 0 {
		return mappings, bucketKeys, 0
	}

	// Pre-allocate with same capacity (most entries are usually valid)
	validMappings = make(map[string]string, len(mappings))
	if bucketKeys != nil {
		validBucketKeys = make(map[string]string, len(bucketKeys))
	}

	for objectID, hash := range mappings {
		// Determine file path: check bucket directory first if bucketKey exists
		var bucketKey string
		if bucketKeys != nil {
			bucketKey = bucketKeys[objectID]
		}

		hashFile := buildHashFilePath(kindDir, hash, bucketKey)

		// Check if file exists
		if _, err := os.Stat(hashFile); err == nil {
			// File exists - keep this mapping
			validMappings[objectID] = hash
			if bucketKeys != nil && bucketKey != emptyValue {
				validBucketKeys[objectID] = bucketKey
			}
		} else {
			// File doesn't exist - this is a stale entry
			staleCount++
		}
	}

	return validMappings, validBucketKeys, staleCount
}

// Start is a no-op for sync strategy (no background operations).
func (s *SyncValidationStrategy) Start(_ string) error {
	return nil
}

// Stop is a no-op for sync strategy (no background operations to stop).
func (s *SyncValidationStrategy) Stop() error {
	return nil
}

// Name returns the strategy name.
func (s *SyncValidationStrategy) Name() string {
	return "sync"
}

// GetMetrics returns the metrics for this strategy instance.
func (s *SyncValidationStrategy) GetMetrics() *ValidationMetrics {
	return s.metrics
}

// buildHashFilePath constructs the full path to a hash file.
// If bucketKey is provided, the file is in kindDir/bucketKey/hash.yaml
// Otherwise, the file is in kindDir/hash.yaml
func buildHashFilePath(kindDir, hash, bucketKey string) string {
	if bucketKey != emptyValue {
		return filepath.Join(kindDir, bucketKey, hash+".yaml")
	}
	return filepath.Join(kindDir, hash+".yaml")
}
