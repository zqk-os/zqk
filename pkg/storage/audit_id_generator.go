package storage

import (
	"context"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/audit"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	idgen "github.com/lanceman/zqk/pkg/storage/id_generation"
)

var (
	_ audit.IDAllocator = (*AuditIDGenerator)(nil)
	_ audit.CASIDIndex  = (*filecas.ContentAddressableStorage)(nil)
)

// DEPRECATED: The old global registry is no longer used
// Audit IDs now use the unified batch generator pool via GetBatchIDGenerator()

// GetCASIDProvider returns a CAS-aware or stream-aware idgen.CASIDProvider for the given storageProvider and kind,
// or nil if neither is enabled or storageProvider is not a FileObjectStorage.
func GetCASIDProvider(storageProvider ObjectStorageProvider, kind string) idgen.CASIDProvider {
	if storageProvider == nil {
		return nil
	}
	if fileStorage, ok := storageProvider.(*FileObjectStorage); ok && fileStorage != nil {
		if StreamStorageEnabledForKind(kind) {
			return func(casCtx context.Context, k, bucketDir, prefix string) ([]string, error) {
				allIDs := ListStreamIDsFromPersistentRegistry(fileStorage.projectRoot, k)
				return audit.NumericPrefixedIDs(prefix, allIDs), nil
			}
		}
		if fileStorage.usesContentAddressableStorage(kind) {
			return func(casCtx context.Context, k, bucketDir, prefix string) ([]string, error) {
				cas, err := fileStorage.getContentAddressableStorage(k)
				if err != nil {
					return nil, nil
				}
				return audit.ListNumericPrefixedIDs(cas, prefix)
			}
		}
	}
	return nil
}

// GetAuditIDGenerator returns (or creates) a thread-safe audit ID generator for a specific audit directory
// Now uses the unified batch generator pool for consistency and efficiency
// If storageProvider is provided and uses CAS, the generator will also query CAS index for existing IDs
func GetAuditIDGenerator(ctx context.Context, auditDir string, storageProvider ...ObjectStorageProvider) *AuditIDGenerator {
	var casProvider idgen.CASIDProvider

	// If storage provider is available and uses CAS, create a CAS-aware provider function
	if len(storageProvider) > 0 && storageProvider[0] != nil {
		casProvider = GetCASIDProvider(storageProvider[0], objects.KindAuditEvent)
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
