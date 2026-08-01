package validation

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// GetCachedState retrieves cached validation state if available and fresh
// Note: stateCache has its own internal mutex, so no additional synchronization needed
func (av *AsyncValidator) GetCachedState(objectID string) (*ValidationState, bool) {
	return av.stateCache.Get(objectID)
}

// GetAllCachedStates retrieves all cached validation states
// Note: stateCache has its own internal mutex, so no additional synchronization needed
func (av *AsyncValidator) GetAllCachedStates() []*ValidationState {
	return av.stateCache.GetAll()
}

// ClearCache clears the validation state cache
// Note: stateCache has its own internal mutex, so no additional synchronization needed
func (av *AsyncValidator) ClearCache() error {
	return av.stateCache.Clear()
}

// InvalidateIntegrityIssues invalidates all cached states with integrity issues
// This forces re-validation of objects that had hash mismatches or other integrity problems
// Note: stateCache has its own internal mutex, so no additional synchronization needed
func (av *AsyncValidator) InvalidateIntegrityIssues() int {
	return av.stateCache.InvalidateByCategory("integrity")
}

// InvalidateCache invalidates the validation cache for a specific object
// This forces re-validation of the object on the next check
// Note: stateCache has its own internal mutex, so no additional synchronization needed
func (av *AsyncValidator) InvalidateCache(objectID string) {
	av.stateCache.Invalidate(objectID)
}

// ValidateNow validates an object immediately (bypasses queue, uses cache if available)
func (av *AsyncValidator) ValidateNow(ctx context.Context, objectID, objectKind, filePath string) (*ValidationState, error) {
	// Check cache first
	if state, ok := av.GetCachedState(objectID); ok {
		// Verify file hasn't changed
		currentChecksum := av.computeChecksum(filePath)
		if state.Checksum == currentChecksum {
			return state, nil
		}
	}

	// File changed or not in cache, validate now
	state, err := av.validateObject(ctx, objectID, objectKind, filePath)
	if err != nil {
		return nil, err
	}

	// Cache the result
	av.stateCache.Set(state)

	return state, nil
}

// ShouldEnqueue checks if an object needs validation (cache miss or stale)
// Returns true if object should be enqueued, false if cached and valid
// This allows batch enqueue optimization by checking cache before creating tasks
// PERFORMANCE: This function is called for every object during enqueue, so it must be fast.
// We skip expensive checksum computation here - that happens during actual validation.
// Note: This duplicates some logic from Enqueue() but allows batching enqueue operations
func (av *AsyncValidator) ShouldEnqueue(objectID, objectKind, filePath string) bool {
	// Check cache first - if valid cached state exists, skip enqueueing
	if state, ok := av.GetCachedState(objectID); ok {
		// Quick mtime check (fast - single stat call)
		info, err := os.Stat(filePath)
		if err == nil {
			// File exists - check if mtime changed (quick check)
			if !info.ModTime().After(state.LastValidated) {
				// File mtime hasn't changed - check for integrity issues
				hasIntegrityIssues := false
				for _, issue := range state.Issues {
					if issue.Category == "integrity" {
						hasIntegrityIssues = true
						break
					}
				}
				if !hasIntegrityIssues {
					// File likely unchanged and no integrity issues - use cached state
					// PERFORMANCE: Skip expensive checksum and hash registry checks here
					// These are deferred to Enqueue() which is called less frequently (only for cache hits)
					// This allows fast batch enqueue of cache misses
					return false // Cache hit, don't enqueue
				}
			}
		}

		// File mtime changed or stat failed - must re-validate
		// PERFORMANCE: Skip expensive checksum computation here - defer to validation time
		// The full Enqueue() will do the checksum check for cache hits, but for batch
		// optimization we're more permissive and just enqueue if mtime changed
		return true // mtime changed, should enqueue
	}

	// File changed or not in cache - should enqueue
	return true
}

// checkHashRegistryUpdated checks if the hash registry file was updated more recently than the cache entry
// This ensures cache invalidation when hash registry is updated by auto-fix operations
// Returns true if hash registry file mtime is after cache entry's LastValidated time
func (av *AsyncValidator) checkHashRegistryUpdated(objectKind, filePath string, cacheTime time.Time) bool {
	// Get the directory containing the file (hash registry is in the same directory)
	fileDir := filepath.Dir(filePath)

	// Hash registry file is named .<kind>.hashes (e.g., .audit_event.hashes)
	// The hash registry is stored in the same directory as the object files
	hashRegistryFile := filepath.Join(fileDir, "."+objectKind+".hashes")

	// Check if hash registry file exists and get its mtime
	info, err := os.Stat(hashRegistryFile)
	if err != nil {
		// Hash registry doesn't exist or can't be stat'd - assume not updated
		// This is fine - if there's no registry, there's nothing to check
		return false
	}

	// If hash registry file was modified after cache entry was created, cache is stale
	// This means the hash registry was updated (e.g., by auto-fix) and we need to re-validate
	return info.ModTime().After(cacheTime)
}
