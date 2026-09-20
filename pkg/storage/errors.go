package storage

import "errors"

var (
	ErrInvalidID               = errors.New("invalid object ID")
	ErrPermissionDeniedStorage = errors.New("permission denied")
	ErrInvalidSchema           = errors.New("invalid schema")
	ErrTimeout                 = errors.New("storage operation timed out")
	ErrOptimisticLockConflict  = errors.New("optimistic lock conflict: object modified concurrently")
	ErrIntegrityViolation      = errors.New("referential integrity violation")
	ErrCASCorrupted            = errors.New("content-addressable storage checksum verification failed")
	ErrInvalidObjectKind       = errors.New("unregistered or invalid object kind")
	ErrStorageClosed           = errors.New("storage engine is closed")
	ErrNamespaceAccessDenied   = errors.New("namespace isolation access denied")
)
