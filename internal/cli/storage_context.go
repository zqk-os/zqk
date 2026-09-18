package cli

import (
	"context"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// storageProviderKey is the context key for the storage provider set by root PersistentPreRunE
// when root creates storage for commands that require it (e.g. object/internal).
type storageProviderKey struct{}

// WithStorageProvider attaches the storage provider to ctx so object commands can reuse it.
func WithStorageProvider(ctx context.Context, provider any) context.Context {
	if ctx == nil || provider == nil {
		return ctx
	}
	return context.WithValue(ctx, storageProviderKey{}, provider)
}

// GetStorageProvider returns the storage provider from ctx if set.
func GetStorageProvider(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	return ctx.Value(storageProviderKey{})
}

// StorageAvailableForOptionalUse returns true when an active storage service is already
// available for this invocation (storage was started by root and set in context).
// Use for ignorable operations (e.g. command audit event, best-effort metrics): only use
// storage when it is already running; never create storage for optional ops.
// Vital operations are gated by command requirements (root only starts storage when
// the command requires it); they must not create storage on the fly either—they run
// only when the command requested storage and root set it in context.
func StorageAvailableForOptionalUse(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return GetStorageProvider(cmd.Context()) != nil
}

// ErrStorageNotInContext is returned when project root is empty.
var ErrStorageNotInContext = errfmt.Errorf("storage not in context: commands that need storage require project root (run from project dir or set ZQK_TEST_ROOT)")

// Process-level cache: storage is keyed by project root so multiple object commands in the
// same process reuse one instance. Each CLI invocation is a new process, so we still create
// once per run; cross-invocation reuse (e.g. second shell run reusing first run's storage)
// would require a long-running daemon that holds storage and serves requests via IPC.
var (
	storageCache   map[string]storage.ObjectStorageProvider
	storageCacheMu sync.RWMutex
)

// OnStorageCreated is called when storage is first created for a project root (e.g. to wire synonym resolver).
// Root registers this in init so object commands don't need to create storage in PreRunE.
var OnStorageCreated func(storage.ObjectStorageProvider)

// RegisterStorageForProjectRoot seeds the process-level storage cache so GetObjectStorageForCommand
// returns this provider for the given project root. Used by instance bootstrap (e.g. MCP serve) so
// in-process CLI and MCP handlers share one storage instance per process.
func RegisterStorageForProjectRoot(projectRoot string, provider storage.ObjectStorageProvider) {
	if projectRoot == emptyValue || provider == nil {
		return
	}
	storageCacheMu.Lock()
	if storageCache == nil {
		storageCache = make(map[string]storage.ObjectStorageProvider)
	}
	storageCache[projectRoot] = provider
	storageCacheMu.Unlock()
}

// GetObjectStorageForProjectRoot returns the cached storage provider for the given project root, if any.
// Used by lifecycle hook handlers (e.g. priority-plan-complete updater) that run without a command.
// Returns (nil, false) when no storage is cached for that root (e.g. one-shot CLI that has not created storage yet).
func GetObjectStorageForProjectRoot(projectRoot string) (storage.ObjectStorageProvider, bool) {
	if projectRoot == emptyValue {
		return nil, false
	}
	storageCacheMu.RLock()
	defer storageCacheMu.RUnlock()
	if storageCache == nil {
		return nil, false
	}
	p, ok := storageCache[projectRoot]
	return p, ok && p != nil
}

// GetObjectStorageForCommand returns storage for commands that need it (e.g. object/internal). It checks (1) context
// (if root or any parent set storage for this run), (2) process cache (if a previous run already created
// storage for this project root). If neither has it, creates storage, wires via OnStorageCreated
// if set, caches by project root, and returns. First run pays full init; subsequent runs are fast.
// Walks up the command tree so nested subcommands (e.g. object bulk update) use the root's context.
func GetObjectStorageForCommand(cmd *cobra.Command, projectRoot string) (storage.ObjectStorageProvider, error) {
	for c := cmd; c != nil; c = c.Parent() {
		if p := GetStorageProvider(c.Context()); p != nil {
			return p.(storage.ObjectStorageProvider), nil
		}
	}
	if projectRoot == emptyValue {
		return nil, ErrStorageNotInContext
	}
	storageCacheMu.RLock()
	cached := storageCache[projectRoot]
	storageCacheMu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	storageCacheMu.Lock()
	defer storageCacheMu.Unlock()

	if storageCache == nil {
		storageCache = make(map[string]storage.ObjectStorageProvider)
	}

	if cached := storageCache[projectRoot]; cached != nil {
		return cached, nil
	}

	provider, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		return nil, err
	}
	if OnStorageCreated != nil {
		OnStorageCreated(provider)
	}

	storageCache[projectRoot] = provider
	return provider, nil
}
