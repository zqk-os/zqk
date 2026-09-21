package storage

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

type GraphConnectionProvider interface {
	IsEnabled() bool
	GetPool(context.Context) (provider.ConnectionPool, error)
}

var defaultGraphConnectionProvider GraphConnectionProvider

func SetGraphConnectionProvider(p GraphConnectionProvider) {
	defaultGraphConnectionProvider = p
}

type StorageFactory struct {
	defaultStorage       ObjectStorageProvider
	providers            map[string]ObjectStorageProvider
	projectRoot          string
	providerLookupsTotal atomic.Int64
	defaultLookupsTotal  atomic.Int64
}

var (
	factoriesCreatedTotal atomic.Int64
)

// GetStorageFactoryStats returns lifetime counters for created factories, provider lookups, and default lookups.
func (f *StorageFactory) GetStorageFactoryStats() (created, providerLookups, defaultLookups int64) {
	c := factoriesCreatedTotal.Load()
	if f == nil {
		return c, 0, 0
	}
	return c, f.providerLookupsTotal.Load(), f.defaultLookupsTotal.Load()
}

func NewStorageFactory(ctx context.Context, projectRoot string) (*StorageFactory, error) {
	factoriesCreatedTotal.Add(1)
	factory := &StorageFactory{
		projectRoot: projectRoot,
		providers:   make(map[string]ObjectStorageProvider),
	}

	// Storage mode: default = file SSOT. Opt-in:
	//   STORAGE_MODE_HYBRID_LEGACY=1|true|yes OR STORAGE_MODE=hybrid_legacy
	//   STORAGE_MODE=file+projection
	// Env names are brand-prefixed via pkg/zqkenv.
	modeEnv := strings.ToLower(strings.TrimSpace(config.SystemStorageMode().OrDefault("file")))
	isHybridLegacy := modeEnv == "hybrid_legacy" || config.SystemStorageModeHybridLegacy().OrDefault(false)

	useMockGraph := config.StorageMockGraph().OrDefault(false)
	sockPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerSubdir, "rpcpool.sock")
	socketAlive := false
	if _, statErr := fileutil.Stat(sockPath); statErr == nil {
		if conn, err := net.Dial("unix", sockPath); err == nil {
			_ = conn.Close()
			socketAlive = true
		}
	}
	graphEnabled := (defaultGraphConnectionProvider != nil && defaultGraphConnectionProvider.IsEnabled()) || useMockGraph || socketAlive

	var primary ObjectStorageProvider
	if graphEnabled && isHybridLegacy {
		graphStorage := NewLazyGraphStorage(defaultGraphConnectionProvider, projectRoot, useMockGraph)
		fileStorage, err := GetFileObjectStorage(projectRoot, &FileObjectStorageOptions{InitContext: ctx})
		if err != nil {
			return nil, errfmt.Newf(ConstMiscFailedToCreateFileStorage).Wrap(err)
		}
		// Transitional hybrid dual-write (graph primary, file secondary); opt-in only.
		primary = NewHybridObjectStorage(graphStorage, fileStorage)
	} else if modeEnv == "file+projection" {
		// Opt-in only — graph availability alone must not enable projection.
		fileStorage, err := GetFileObjectStorage(projectRoot, &FileObjectStorageOptions{InitContext: ctx})
		if err != nil {
			return nil, errfmt.Newf(ConstMiscFailedToCreateFileStorage).Wrap(err)
		}
		graphStorage := NewLazyGraphStorage(defaultGraphConnectionProvider, projectRoot, useMockGraph)
		primary = NewFileFirstProjectionStorage(fileStorage, graphStorage)
	} else {
		fileStorage, err := GetFileObjectStorage(projectRoot, &FileObjectStorageOptions{InitContext: ctx})
		if err != nil {
			return nil, errfmt.Newf(ConstMiscFailedToCreateFileStorage).Wrap(err)
		}
		primary = fileStorage
	}

	factory.defaultStorage = primary

	// Check if we are running in MCP context (which means we are already serving a query)
	// If so, do NOT wrap with MeshObjectStorage to prevent infinite recursion
	isMCPContext := zqkenv.MCPAccountID().Get() != ""
	if loggingCtx := pkgctx.GetLoggingContext(ctx); loggingCtx != nil && loggingCtx.Profile == pkgctx.ProfileMCP {
		isMCPContext = true
	}

	var meshStorage *MeshObjectStorage
	if !isMCPContext {
		// Create Mesh Overlay for strategic visibility
		// Pass 'primary' as the local provider for data.
		meshStorage = NewMeshObjectStorage(primary, nil)
	}

	// Discover all object kinds from spec index
	idx := objects.TryLoadSpecIndexForProjectRoot(projectRoot)
	if idx != nil {
		for kind := range idx.Kinds {
			// Only overlay specific kinds for performance and privacy
			if meshStorage != nil && isStrategicKind(kind) {
				factory.providers[kind] = meshStorage
			} else {
				factory.providers[kind] = primary
			}
		}
	} else {
		// Fallback
		if meshStorage != nil {
			factory.providers["backlog_item"] = meshStorage
			factory.providers["goal"] = meshStorage
			factory.providers["priority_plan"] = meshStorage
		} else {
			factory.providers["backlog_item"] = primary
			factory.providers["goal"] = primary
			factory.providers["priority_plan"] = primary
		}
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Debug(ConstMiscStoragefactoryInitialized).Log()
	return factory, nil
}

func isStrategicKind(kind string) bool {
	switch kind {
	case "backlog_item", "goal", "milestone", "priority_plan", "workstream", "agent_skill", "requirement", "roadmap", "vision", ConstMiscCapacityAdvertisement, "remote_kernel":
		return true
	default:
		return false
	}
}

func (f *StorageFactory) GetStorageForObject(obj map[string]any) ObjectStorageProvider {
	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind != "" {
		if provider, ok := f.providers[kind]; ok {
			return provider
		}
	}

	if profile := objects.GetString(obj, ConstMiscStewardshipProfile); profile == "active" {
		if kind != "" {
			if provider, ok := f.providers[kind]; ok {
				return provider
			}
		}
	}
	return f.defaultStorage
}

func (f *StorageFactory) GetStorageForKind(kind string) ObjectStorageProvider {
	if provider, ok := f.providers[kind]; ok {
		f.providerLookupsTotal.Add(1)
		return provider
	}
	f.defaultLookupsTotal.Add(1)
	return f.defaultStorage
}

func (f *StorageFactory) GetStorage() ObjectStorageProvider { return f.defaultStorage }

func (f *StorageFactory) GetPool() provider.ConnectionPool {
	if p, ok := f.defaultStorage.(interface {
		GetPool() provider.ConnectionPool
	}); ok {
		return p.GetPool()
	}
	return nil
}

func (f *StorageFactory) GetProjectRoot() string {
	return f.projectRoot
}

// Shutdown shuts down the underlying default storage provider if it supports it.
func (f *StorageFactory) Shutdown(ctx context.Context) error {
	if f.defaultStorage != nil {
		if s, ok := f.defaultStorage.(interface{ Shutdown(context.Context) error }); ok {
			return s.Shutdown(ctx)
		}
	}
	return nil
}

// IsGraphBackend returns true if graph backend is active for a given kind
func (f *StorageFactory) IsGraphBackend(kind string) bool {
	_, ok := f.providers[kind]
	return ok
}

// IsFileBackend returns true if file backend is active for a given kind
func (f *StorageFactory) IsFileBackend(kind string) bool {
	provider, ok := f.providers[kind]
	if !ok {
		provider = f.defaultStorage
	}
	return UnwrapToFileObjectStorage(provider) != nil
}

func GetFileObjectStorage(projectRoot string, opts ...*FileObjectStorageOptions) (*FileObjectStorage, error) {
	return NewFileObjectStorage(projectRoot, opts...)
}

func GetFileObjectStorageForTest(projectRoot string) (*FileObjectStorage, error) {
	return NewFileObjectStorageForTest(projectRoot)
}

// UnderlyingObjectStorageProvider is implemented by decorator/wrapper providers that
// embed or forward to another ObjectStorageProvider. UnwrapToFileObjectStorage uses
// this so CLI-layer decorators (e.g. SemanticStorageDecorator) stay file-reachable.
type UnderlyingObjectStorageProvider interface {
	UnderlyingObjectStorageProvider() ObjectStorageProvider
}

// UnwrapToFileObjectStorage recursively unwraps MeshObjectStorage, HybridObjectStorage, etc., to find and return the underlying *FileObjectStorage.
func UnwrapToFileObjectStorage(p ObjectStorageProvider) *FileObjectStorage {
	if p == nil {
		return nil
	}
	if fs, ok := p.(*FileObjectStorage); ok {
		return fs
	}
	// Generic decorator unwrap (CLI SemanticStorageDecorator, future wrappers).
	// TRACK: BLI-1785723654802038000-b14064bc — state-restore broke when Processor
	// wrapped storage in SemanticStorageDecorator without an Unwrap path.
	if u, ok := p.(UnderlyingObjectStorageProvider); ok && u != nil {
		if inner := u.UnderlyingObjectStorageProvider(); inner != nil && inner != p {
			if fs := UnwrapToFileObjectStorage(inner); fs != nil {
				return fs
			}
		}
	}
	// Try HybridObjectStorage
	if h, ok := p.(*HybridObjectStorage); ok && h != nil {
		if fs := UnwrapToFileObjectStorage(h.primary); fs != nil {
			return fs
		}
		if fs := UnwrapToFileObjectStorage(h.secondary); fs != nil {
			return fs
		}
	}
	// Try FileFirstProjectionStorage
	if fp, ok := p.(*FileFirstProjectionStorage); ok && fp != nil {
		return fp.file
	}
	// Try MeshObjectStorage
	if m, ok := p.(*MeshObjectStorage); ok && m != nil {
		return UnwrapToFileObjectStorage(m.local)
	}
	// Try TransactionalStorageWrapper
	if t, ok := p.(*TransactionalStorageWrapper); ok && t != nil {
		return UnwrapToFileObjectStorage(t.underlying)
	}
	// Try BatchingObjectStorage (swarm write batching decorator)
	if b, ok := p.(*BatchingObjectStorage); ok && b != nil {
		return UnwrapToFileObjectStorage(b.ObjectStorageProvider)
	}
	// RoutingObjectStorage delegates per-kind; probe a file-backed kind without
	// calling GetStorage() (that can re-enter the same Routing/Batching tip).
	if r, ok := p.(*RoutingObjectStorage); ok && r != nil {
		if f := r.GetStorageFactory(); f != nil {
			return UnwrapToFileObjectStorage(f.GetStorageForKind(objects.KindGoal))
		}
	}
	return nil
}

// UnwrapToFileFirstProjection finds a FileFirstProjectionStorage under common wrappers.
func UnwrapToFileFirstProjection(p ObjectStorageProvider) *FileFirstProjectionStorage {
	if p == nil {
		return nil
	}
	if fp, ok := p.(*FileFirstProjectionStorage); ok {
		return fp
	}
	if u, ok := p.(UnderlyingObjectStorageProvider); ok && u != nil {
		if inner := u.UnderlyingObjectStorageProvider(); inner != nil && inner != p {
			if fp := UnwrapToFileFirstProjection(inner); fp != nil {
				return fp
			}
		}
	}
	if m, ok := p.(*MeshObjectStorage); ok && m != nil {
		return UnwrapToFileFirstProjection(m.local)
	}
	if t, ok := p.(*TransactionalStorageWrapper); ok && t != nil {
		return UnwrapToFileFirstProjection(t.underlying)
	}
	if b, ok := p.(*BatchingObjectStorage); ok && b != nil {
		return UnwrapToFileFirstProjection(b.ObjectStorageProvider)
	}
	if r, ok := p.(*RoutingObjectStorage); ok && r != nil {
		if f := r.GetStorageFactory(); f != nil {
			return UnwrapToFileFirstProjection(f.GetStorageForKind(objects.KindGoal))
		}
	}
	return nil
}
