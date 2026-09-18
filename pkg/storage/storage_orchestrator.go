package storage

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// StorageOrchestrator coordinates operations across multiple storage backends
// Tracks pending operations to prevent undesirable actions on objects with pending updates
type StorageOrchestrator struct {
	backends    map[string]ObjectStorageProvider // backend name -> provider
	pendingOps  map[string]*PendingStorageOps    // objectID -> pending operations across all backends
	mu          sync.RWMutex
	logger      *logging.EventLogger
	hashManager *DeferredHashManager
}

// PendingStorageOps tracks pending operations for an object across all storage backends
type PendingStorageOps struct {
	ObjectID   string
	BackendOps map[string][]string // backend name -> list of operation IDs
	LastUpdate time.Time
	mu         sync.RWMutex
}

// NewStorageOrchestrator creates a new storage orchestrator
func NewStorageOrchestrator() *StorageOrchestrator {
	return &StorageOrchestrator{
		backends:   make(map[string]ObjectStorageProvider),
		pendingOps: make(map[string]*PendingStorageOps),
		logger:     logging.NewEventLogger(pkgctx.NewSystemContext()),
	}
}

// RegisterBackend registers a storage backend with the orchestrator
func (so *StorageOrchestrator) RegisterBackend(name string, backend ObjectStorageProvider) {
	_ = concurrency.RunInLockOrLog(&so.mu, locknames.LockNameStorageOrchestratorRegisterBackend, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		so.backends[name] = backend
		if _, ok := backend.(*FileObjectStorage); ok {
			if so.hashManager == nil {
				so.hashManager = GetDeferredHashManager(backend)
			}
		}
		return nil
	})
}

// GetBackend returns a storage backend by name
func (so *StorageOrchestrator) GetBackend(name string) (ObjectStorageProvider, error) {
	var backend ObjectStorageProvider
	var exists bool
	err := concurrency.RunInRLockWithLogger(&so.mu, locknames.LockNameStorageOrchestratorGetBackend, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		backend, exists = so.backends[name]
		return nil
	})
	if err != nil {
		return nil, errfmt.Newf(ConstMiscGettingBackend).Wrap(err)
	}
	if !exists {
		return nil, errfmt.Errorf(ConstMiscBackendNotFoundS, name)
	}
	return backend, nil
}

// GetDefaultBackend returns the default storage backend (file storage)
func (so *StorageOrchestrator) GetDefaultBackend() (ObjectStorageProvider, error) {
	var result ObjectStorageProvider
	var found bool
	err := concurrency.RunInRLockWithLogger(&so.mu, locknames.LockNameStorageOrchestratorGetDefaultBackend, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		for name, backend := range so.backends {
			if _, ok := backend.(*FileObjectStorage); ok {
				result = backend
				found = true
				return nil
			}
			if name != emptyValue && !found {
				result = backend
				found = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, errfmt.Newf(ConstMiscGettingDefaultBackend).Wrap(err)
	}
	if !found {
		return nil, errfmt.Errorf(ConstMiscNoStorageBackendAvailable)
	}
	return result, nil
}

// RegisterOperation registers a pending operation that affects an object
func (so *StorageOrchestrator) RegisterOperation(backendName, objectID, operationID string) {
	var pending *PendingStorageOps
	err := concurrency.RunInLockWithLogger(&so.mu, locknames.LockNameStorageOrchestratorRegisterOperation, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var exists bool
		pending, exists = so.pendingOps[objectID]
		if !exists {
			pending = &PendingStorageOps{
				ObjectID:   objectID,
				BackendOps: make(map[string][]string),
				LastUpdate: time.Now(),
			}
			so.pendingOps[objectID] = pending
		}
		return nil
	})
	if err != nil {
		return
	}

	_ = concurrency.RunInLockOrLog(&pending.mu, locknames.LockNamePendingStorageOpsRegister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if pending.BackendOps[backendName] == nil {
			pending.BackendOps[backendName] = make([]string, 0)
		}
		found := false
		for _, opID := range pending.BackendOps[backendName] {
			if opID == operationID {
				found = true
				break
			}
		}
		if !found {
			pending.BackendOps[backendName] = append(pending.BackendOps[backendName], operationID)
			pending.LastUpdate = time.Now()
		}
		return nil
	})
}

// CompleteOperation marks an operation as complete
func (so *StorageOrchestrator) CompleteOperation(backendName, objectID, operationID string) error {
	ctx := pkgctx.NewSystemContext()
	var pending *PendingStorageOps
	var exists bool
	var allComplete bool
	err := concurrency.RunInLockWithLogger(&so.mu, locknames.LockNameStorageOrchestratorCompleteOpCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var ok bool
		pending, ok = so.pendingOps[objectID]
		exists = ok
		return nil
	})
	if err != nil {
		return err
	}

	if !exists {
		return so.updateHashIfReady(ctx, objectID)
	}

	_ = concurrency.RunInLockOrLog(&pending.mu, locknames.LockNameStorageOrchestratorCompleteOpUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if backendOps, ok := pending.BackendOps[backendName]; ok {
			newOps := make([]string, 0, len(backendOps))
			for _, opID := range backendOps {
				if opID != operationID {
					newOps = append(newOps, opID)
				}
			}
			if len(newOps) == 0 {
				delete(pending.BackendOps, backendName)
			} else {
				pending.BackendOps[backendName] = newOps
			}
		}
		allComplete = true
		for _, backendOps := range pending.BackendOps {
			if len(backendOps) > 0 {
				allComplete = false
				break
			}
		}
		return nil
	})

	if allComplete {
		_ = concurrency.RunInLockOrLog(&so.mu, locknames.LockNameStorageOrchestratorCompleteOpRemove, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			delete(so.pendingOps, objectID)
			return nil
		})
		return so.updateHashIfReady(ctx, objectID)
	}

	return nil
}

// HasPendingOperations returns true if an object has pending operations in any backend
func (so *StorageOrchestrator) HasPendingOperations(objectID string) bool {
	var pending *PendingStorageOps
	var exists bool
	_ = concurrency.RunInRLockOrLog(&so.mu, locknames.LockNameStorageOrchestratorHasPendingCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var ok bool
		pending, ok = so.pendingOps[objectID]
		exists = ok
		return nil
	})
	if !exists {
		return false
	}
	var hasPending bool
	_ = concurrency.RunInRLockOrLog(&pending.mu, locknames.LockNameStorageOrchestratorHasPendingScan, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		for _, backendOps := range pending.BackendOps {
			if len(backendOps) > 0 {
				hasPending = true
				break
			}
		}
		return nil
	})
	return hasPending
}

// GetPendingOperations returns the list of pending operations for an object across all backends
func (so *StorageOrchestrator) GetPendingOperations(objectID string) map[string][]string {
	var result map[string][]string
	_ = concurrency.RunInRLockOrLog(&so.mu, locknames.LockNameStorageOrchestratorGetPending, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		pending, exists := so.pendingOps[objectID]
		if !exists {
			result = make(map[string][]string)
			return nil
		}
		_ = concurrency.RunInRLockOrLog(&pending.mu, locknames.LockNameStorageOrchestratorGetPendingNested, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			result = make(map[string][]string)
			for backend, ops := range pending.BackendOps {
				result[backend] = make([]string, len(ops))
				copy(result[backend], ops)
			}
			return nil
		})
		return nil
	})
	return result
}

// PreventOperation prevents an operation if the object has pending operations
// Returns error if operation should be prevented
func (so *StorageOrchestrator) PreventOperation(objectID, operationType string) error {
	if so.HasPendingOperations(objectID) {
		pending := so.GetPendingOperations(objectID)
		return errfmt.Errorf(ConstMiscCannotPerformSOnObjectSPendingOperations,
			operationType, objectID, pending)
	}
	return nil
}

// updateHashIfReady updates the hash for an object if it's ready (no pending operations)
//
//nolint:unparam // ctx parameter is kept for API consistency
func (so *StorageOrchestrator) updateHashIfReady(_ context.Context, objectID string) error {
	// Only update hash for file storage backends
	if so.hashManager == nil {
		return nil // No hash manager available
	}

	// Check if object is ready (no pending operations in hash manager)
	if !so.hashManager.IsObjectReady(objectID) {
		return nil // Still has pending operations in hash manager
	}

	// Hash manager will handle the update
	return nil
}

// GetBackendForObject determines which backend(s) manage an object
func (so *StorageOrchestrator) GetBackendForObject(objectID string) []string {
	var backends []string
	_ = concurrency.RunInRLockOrLog(&so.mu, locknames.LockNameStorageOrchestratorGetBackendForObject, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		backends = make([]string, 0)
		for name := range so.backends {
			if _, ok := so.backends[name].(*FileObjectStorage); ok {
				backends = append(backends, name)
			}
		}
		return nil
	})
	return backends
}

// Global storage orchestrator instance
var globalStorageOrchestrator *StorageOrchestrator
var globalStorageOrchestratorOnce sync.Once

// GetStorageOrchestrator returns the global storage orchestrator instance
func GetStorageOrchestrator() *StorageOrchestrator {
	globalStorageOrchestratorOnce.Do(func() {
		globalStorageOrchestrator = NewStorageOrchestrator()
	})
	return globalStorageOrchestrator
}
