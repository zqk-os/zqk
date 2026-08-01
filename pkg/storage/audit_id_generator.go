package storage

import (
	"context"
	"fmt"
	"regexp"

	"github.com/lanceman/zqk/pkg/objects"
	idgen "github.com/lanceman/zqk/pkg/storage/id_generation"
)

// DEPRECATED: The old global registry is no longer used
// Audit IDs now use the unified batch generator pool via GetBatchIDGenerator()

// GetAuditIDGenerator returns (or creates) a thread-safe audit ID generator for a specific audit directory
// Now uses the unified batch generator pool for consistency and efficiency
// If storageProvider is provided and uses CAS, the generator will also query CAS index for existing IDs
func GetAuditIDGenerator(ctx context.Context, auditDir string, storageProvider ...ObjectStorageProvider) *AuditIDGenerator {
	var casProvider idgen.CASIDProvider

	// If storage provider is available and uses CAS, create a CAS-aware provider function
	if len(storageProvider) > 0 && storageProvider[0] != nil {
		if fileStorage, ok := storageProvider[0].(*FileObjectStorage); ok && fileStorage != nil {
			if fileStorage.usesContentAddressableStorage(objects.KindAuditEvent) {
				// Create CAS provider function that queries CAS index.
				// For audit events, IDs are globally sequential across buckets, so we gather
				// all matching IDs from the CAS index (bucket filter skipped).
				casProvider = func(casCtx context.Context, kind, bucketDir, prefix string) ([]string, error) {
					cas, err := fileStorage.getContentAddressableStorage(kind)
					if err != nil {
						return nil, nil // No CAS available, return empty (file scan will handle it)
					}

					// Get all IDs from CAS index
					allIDs, err := cas.ListIDs()
					if err != nil {
						return nil, nil // Best effort - continue with file scan
					}

					// Filter IDs by prefix pattern
					prefixWithDash := prefix
					if len(prefixWithDash) == 0 || prefixWithDash[len(prefixWithDash)-1] != '-' {
						prefixWithDash += "-"
					}
					pattern := regexp.MustCompile(fmt.Sprintf(`^%s(\d+)$`, regexp.QuoteMeta(prefixWithDash)))

					var matchingIDs []string
					for _, id := range allIDs {
						if pattern.MatchString(id) {
							matchingIDs = append(matchingIDs, id)
						}
					}

					return matchingIDs, nil
				}
			}
		}
	}

	// Use unified batch generator pool (consistent with other ID generators)
	// Audit events use "AUD" prefix, 0 min digits (no padding), start at 1
	// sequenceFileDir is set to kindDir (auditDir) in batch generator creation for cross-process safe allocation
	generator := idgen.GetBatchIDGeneratorWithCAS(ctx, auditDir, objects.KindAuditEvent, "AUD", 0, 1, casProvider)

	// Wrap in AuditIDGenerator interface for backward compatibility
	return &AuditIDGenerator{
		generator: generator,
		auditDir:  auditDir,
	}
}

// AuditIDGenerator wraps BatchIDGenerator for audit-specific interface
type AuditIDGenerator struct {
	generator *idgen.BatchIDGenerator
	auditDir  string
}

// GenerateNextID generates a single audit ID (thread-safe).
// Delegates to the batch generator, which uses the sequence file when configured (cross-process safe).
func (g *AuditIDGenerator) GenerateNextID() (string, error) {
	return g.generator.GenerateNextID()
}

// GenerateBatchIDs generates multiple audit IDs in a single batch (thread-safe)
// Delegates to the underlying batch generator
func (g *AuditIDGenerator) GenerateBatchIDs(batchSize int) ([]string, error) {
	return g.generator.GenerateBatchIDs(batchSize)
}

// GetLastSequence returns the last sequence number used (for testing/debugging)
func (g *AuditIDGenerator) GetLastSequence() int {
	return g.generator.GetLastSequence()
}

// Reset forces a rescan on the next ID generation (for testing)
func (g *AuditIDGenerator) Reset() {
	g.generator.Reset()
}
