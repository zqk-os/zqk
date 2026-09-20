// Package storage: process-wide ObjectWAL sharing per project root.
// Multiple FileObjectStorage instances must not each OpenFile(object.wal); compaction
// renames the path and leaves deleted-inode FDs open (file-handle explosion).

package wal

import (
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

type objectWALRegistryEntry struct {
	wal  *ObjectWAL
	refs int
}

var (
	objectWALRegistryMu sync.Mutex
	objectWALRegistry   = map[string]*objectWALRegistryEntry{}
)

// AcquireObjectWAL returns the shared ObjectWAL for projectRoot, creating it on first acquire.
// Callers must ReleaseObjectWAL when the storage that acquired it is shut down.
// Prevents per-NewFileObjectStorage OpenFile(object.wal) FD leaks under CompactWAL rename.
func AcquireObjectWAL(projectRoot string) (*ObjectWAL, error) {
	if projectRoot == emptyValue {
		return NewObjectWAL(projectRoot)
	}
	objectWALRegistryMu.Lock()
	defer objectWALRegistryMu.Unlock()
	if e, ok := objectWALRegistry[projectRoot]; ok && e != nil && e.wal != nil {
		e.refs++
		return e.wal, nil
	}
	wal, err := NewObjectWAL(projectRoot)
	if err != nil {
		return nil, err
	}
	objectWALRegistry[projectRoot] = &objectWALRegistryEntry{wal: wal, refs: 1}
	return wal, nil
}

// ReleaseObjectWAL drops one reference. When refs hit zero, closes the WAL and removes the entry.
func ReleaseObjectWAL(projectRoot string, wal *ObjectWAL) error {
	if projectRoot == emptyValue || wal == nil {
		if wal != nil {
			return wal.Close()
		}
		return nil
	}
	objectWALRegistryMu.Lock()
	defer objectWALRegistryMu.Unlock()
	e, ok := objectWALRegistry[projectRoot]
	if !ok || e == nil || e.wal != wal {
		// Not the registered instance (tests / orphan) — close locally.
		return wal.Close()
	}
	e.refs--
	if e.refs > 0 {
		return nil
	}
	delete(objectWALRegistry, projectRoot)
	return e.wal.Close()
}

// ObjectWALRefCountForTest returns live refs for projectRoot (0 if none). Test helper only.
func ObjectWALRefCountForTest(projectRoot string) int {
	return objectWALRefCountForTest(projectRoot)
}

func objectWALRefCountForTest(projectRoot string) int {
	objectWALRegistryMu.Lock()
	defer objectWALRegistryMu.Unlock()
	if e := objectWALRegistry[projectRoot]; e != nil {
		return e.refs
	}
	return 0
}

// writeBehindOwnerRegistry ensures at most one ObjectWriteBehindWorker per project root.
var (
	writeBehindOwnerMu sync.Mutex
	writeBehindOwners  = map[string]any{}
)

// TryClaimWriteBehindOwner returns true if owner becomes the sole write-behind owner for the root.
func TryClaimWriteBehindOwner(projectRoot string, owner any) bool {
	if projectRoot == emptyValue || owner == nil {
		return false
	}
	writeBehindOwnerMu.Lock()
	defer writeBehindOwnerMu.Unlock()
	if existing, ok := writeBehindOwners[projectRoot]; ok && existing != nil && existing != owner {
		return false
	}
	writeBehindOwners[projectRoot] = owner
	return true
}

// ReleaseWriteBehindOwner releases ownership for the root.
func ReleaseWriteBehindOwner(projectRoot string, owner any) {
	if projectRoot == emptyValue || owner == nil {
		return
	}
	writeBehindOwnerMu.Lock()
	defer writeBehindOwnerMu.Unlock()
	if writeBehindOwners[projectRoot] == owner {
		delete(writeBehindOwners, projectRoot)
	}
}

func LogWriteBehindOwnerSkipped(projectRoot string) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn(LogEventStorageWriteBehindOwnerSkipped).
		String("project_root", projectRoot).
		String("hint", "another FileObjectStorage already owns write-behind for this root; sharing ObjectWAL only").
		Log()
}
