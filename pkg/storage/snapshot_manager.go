package storage

import (
	"github.com/zqk-os/zqk/pkg/datacell"

	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"gopkg.in/yaml.v3"
)

// SnapshotMetadata contains metadata captured during snapshot
type SnapshotMetadata struct {
	Timestamp         time.Time            // Snapshot timestamp
	ObjectIDs         []string             // Object IDs in snapshot
	ModificationTimes map[string]time.Time // Object ID -> modification time
	Hashes            map[string]string    // Object ID -> hash
	HashSources       map[string]string    // Object ID -> hash source ("registry" or "calculated")
}

// SnapshotManager coordinates snapshot lifecycle
type SnapshotManager struct {
	proxyStorage      *ProxyStorage
	underlyingStorage ObjectStorageProvider // For accessing hash registries
	queue             *SnapshotOperationQueue
	projectRoot       string
	mu                sync.Mutex
	active            bool
	snapshotTimestamp time.Time
	logger            logging.Logger
}

// NewSnapshotManager creates a new snapshot manager
func NewSnapshotManager(proxyStorage *ProxyStorage, queue *SnapshotOperationQueue, underlyingStorage ObjectStorageProvider, projectRoot string) *SnapshotManager {
	return &SnapshotManager{
		proxyStorage:      proxyStorage,
		underlyingStorage: underlyingStorage,
		queue:             queue,
		projectRoot:       projectRoot,
		logger:            logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// BeginSnapshot activates the proxy and records snapshot timestamp
func (sm *SnapshotManager) BeginSnapshot() (time.Time, error) {
	var timestamp time.Time
	var err error
	_ = concurrency.RunInLockOrLog(
		&sm.mu, locknames.LockNameSnapshotManagerBegin, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if sm.active {
				err = errfmt.Errorf(ConstMiscSnapshotAlreadyInProgress)
				return err
			}

			// Record snapshot timestamp (UTC, nanosecond precision)
			sm.snapshotTimestamp = time.Now().UTC()

			// Activate proxy (operations will be queued)
			sm.proxyStorage.BeginSnapshot()

			sm.active = true
			StorageLog(sm.logger).Info(LogEventStorageSnapshotInitiatedInfo).
				String("timestamp", sm.snapshotTimestamp.Format(time.RFC3339Nano)).
				Log()

			timestamp = sm.snapshotTimestamp
			return nil
		},
	)
	return timestamp, err
}

// CaptureMetadata captures critical metadata for objects while operations are queued
func (sm *SnapshotManager) CaptureMetadata(ctx context.Context, objectIDs []string) (*SnapshotMetadata, error) {
	var active bool
	var timestamp time.Time
	err := concurrency.RunInLockWithLogger(
		&sm.mu, locknames.LockNameSnapshotManagerCaptureCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			active = sm.active
			timestamp = sm.snapshotTimestamp
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscTimeoutCheckingSnapshotStatus).Wrap(err)
	}
	if !active {
		return nil, errfmt.Errorf(ConstMiscSnapshotNotActive)
	}

	// Capture metadata while operations are queued
	metadata := &SnapshotMetadata{
		Timestamp:         timestamp,
		ObjectIDs:         objectIDs,
		ModificationTimes: make(map[string]time.Time),
		Hashes:            make(map[string]string),
		HashSources:       make(map[string]string),
	}

	StorageLog(sm.logger).Info(LogEventStorageSnapshotCapturingMetadataInfo).
		Int("object_count", len(objectIDs)).
		String("timestamp", metadata.Timestamp.Format(time.RFC3339Nano)).
		Log()

	// Capture metadata for each object
	secCtx := pkgctx.NewSystemSecurityContext()
	for _, objectID := range objectIDs {
		// Read object (this will be queued if it has pending writes)
		obj, err := sm.proxyStorage.Read(ctx, secCtx, objectID)
		if err != nil {
			StorageLog(sm.logger).Warn(LogEventStorageSnapshotReadObjectMetadataWarn).
				ObjectID(objectID).
				WithError(err).
				Log()
			continue
		}

		// Get modification time
		if mtimeStr := objects.GetString(obj, objects.FieldKeyUpdatedAt); mtimeStr != "" {
			if mtime, err := time.Parse(ConstMisc20060102t150405z, mtimeStr); err == nil {
				metadata.ModificationTimes[objectID] = mtime
			}
		}

		// Get hash - try registry first, then calculate
		hash, hashSource, err := sm.getObjectHash(obj, objectID)
		if err == nil {
			metadata.Hashes[objectID] = hash
			metadata.HashSources[objectID] = hashSource
			if hashSource == "registry" {
				StorageLog(sm.logger).Debug(LogEventStorageSnapshotHashFromRegistryDebug).
					ObjectID(objectID).
					Log()
			} else {
				StorageLog(sm.logger).Debug(LogEventStorageSnapshotHashFromContentDebug).
					ObjectID(objectID).
					Log()
			}
		} else {
			StorageLog(sm.logger).Warn(LogEventStorageSnapshotGetHashWarn).
				ObjectID(objectID).
				WithError(err).
				Log()
		}
	}

	// Count hash sources
	registryCount := 0
	calculatedCount := 0
	for _, source := range metadata.HashSources {
		if source == "registry" {
			registryCount++
		} else {
			calculatedCount++
		}
	}

	StorageLog(sm.logger).Info(LogEventStorageSnapshotMetadataCapturedInfo).
		Int("objects", len(metadata.ObjectIDs)).
		Int("hashes", len(metadata.Hashes)).
		Int("mtimes", len(metadata.ModificationTimes)).
		Int(ConstMiscRegistryHashes, registryCount).
		Int(ConstMiscCalculatedHashes, calculatedCount).
		Log()

	return metadata, nil
}

// getObjectHash gets hash from registry if available, otherwise calculates it
// Returns: (hash, source, error) where source is "registry" or "calculated"
func (sm *SnapshotManager) getObjectHash(obj map[string]any, objectID string) (hash, source string, err error) {
	// First calculate hash to use for verification and as fallback
	calculatedHash, err := sm.calculateObjectHash(obj)
	if err != nil {
		return "", "", err
	}

	// Try to get hash from registry first (if file storage)
	if _, ok := sm.underlyingStorage.(*FileObjectStorage); ok {
		kind, _ := obj[objects.FieldKeyKind].(string)
		if kind != emptyValue {
			// Get kind directory
			processDir := datacell.ProcessPrimaryDir(sm.projectRoot)
			dirName := objects.GetDirectoryFromKind(kind)
			if dirName != emptyValue {
				kindDir := filepath.Join(processDir, dirName)

				// Try to load hash registry
				hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), kind, kindDir)
				if err := hashRegistry.Load(); err == nil {
					// Try multiple lookup strategies:
					// 1. Try ID-based filename (for non-CAS objects) - most common
					// 2. Try hash-based filename (for CAS objects stored with hash as filename)
					// 3. Try calculated hash as key directly (some registries use hash as key)

					// Strategy 1: ID-based filename (e.g., "SCH-001.yaml")
					idFilename := objectID + ".yaml"
					registryHash := hashRegistry.GetHash(idFilename)

					// Strategy 2: Hash-based filename (CAS - e.g., "fa8961297b6a....yaml")
					if registryHash == emptyValue {
						hashFilename := calculatedHash + ".yaml"
						registryHash = hashRegistry.GetHash(hashFilename)
					}

					// Strategy 3: Try calculated hash as key directly (some registries)
					if registryHash == emptyValue {
						registryHash = hashRegistry.GetHash(calculatedHash)
					}

					if registryHash != emptyValue {
						// Verify hash matches actual content
						if registryHash == calculatedHash {
							// Registry hash is valid
							return registryHash, "registry", nil
						} else {
							// Hash mismatch - registry is stale, use calculated
							StorageLog(sm.logger).Warn(LogEventStorageSnapshotHashMismatchUsingCalcWarn).
								ObjectID(objectID).
								String("registry_hash", registryHash).
								String(ConstMiscCalculatedHash, calculatedHash).
								Log()
							return calculatedHash, "calculated", nil
						}
					}
				}
			}
		}
	}

	// Fall back to calculated hash
	return calculatedHash, "calculated", nil
}

// calculateObjectHash calculates hash from object content
func (sm *SnapshotManager) calculateObjectHash(obj map[string]any) (string, error) {
	// Marshal to YAML and calculate hash
	data, err := yaml.Marshal(obj)
	if err != nil {
		return "", errfmt.Newf(ConstMiscFailedToMarshalObject).Wrap(err)
	}

	return calculateSHA256Hash(data), nil
}

// calculateSHA256Hash calculates SHA256 hash of data
func calculateSHA256Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// EndSnapshot deactivates the proxy and replays queued operations
func (sm *SnapshotManager) EndSnapshot(ctx context.Context) error {
	err := concurrency.RunInLockWithLogger(
		&sm.mu, locknames.LockNameSnapshotManagerEnd, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if !sm.active {
				return errfmt.Errorf(ConstMiscNoActiveSnapshot)
			}
			// Deactivate proxy first (stop queuing new operations)
			sm.proxyStorage.EndSnapshot()
			sm.active = false
			return nil
		},
	)
	if err != nil {
		return err
	}

	queueSize := sm.proxyStorage.GetQueueSize()
	StorageLog(sm.logger).Info(LogEventStorageSnapshotEndingInfo).
		Int(ConstMiscQueuedOperations, queueSize).
		Log()

	// Deactivate proxy (operations will execute normally)
	sm.proxyStorage.EndSnapshot()

	// Replay queued operations
	if queueSize > 0 {
		if err := sm.proxyStorage.ReplayQueuedOperations(ctx); err != nil {
			StorageLog(sm.logger).Warn(LogEventStorageSnapshotReplaySomeFailedWarn).
				WithError(err).
				Log()
			// Don't fail snapshot - operations were queued successfully
		}
	}

	sm.active = false
	sm.snapshotTimestamp = time.Time{}

	StorageLog(sm.logger).Info(LogEventStorageSnapshotCompletedInfo).
		Int(ConstMiscOperationsReplayed, queueSize).
		Log()

	return nil
}

// IsActive returns whether a snapshot is currently active
func (sm *SnapshotManager) IsActive() bool {
	var active bool
	_ = concurrency.RunInLockOrLog(
		&sm.mu, locknames.LockNameSnapshotManagerIsActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			active = sm.active
			return nil
		},
	)
	return active
}

// GetSnapshotTimestamp returns the current snapshot timestamp
func (sm *SnapshotManager) GetSnapshotTimestamp() time.Time {
	var timestamp time.Time
	_ = concurrency.RunInLockOrLog(
		&sm.mu, locknames.LockNameSnapshotManagerGetTimestamp, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			timestamp = sm.snapshotTimestamp
			return nil
		},
	)
	return timestamp
}
