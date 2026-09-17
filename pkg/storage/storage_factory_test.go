package storage

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGraphProvider struct {
	enabled bool
}

func (m *mockGraphProvider) IsEnabled() bool {
	return m.enabled
}

func (m *mockGraphProvider) GetPool(ctx context.Context) (provider.ConnectionPool, error) {
	return nil, nil
}

func TestStorageFactory(t *testing.T) {
	tempDir := t.TempDir()
	SetupTestRootLikeSetupTestEnvironmentForExportTest(t, tempDir)

	SetGraphConnectionProvider(&mockGraphProvider{enabled: false})

	ctx := context.Background()
	factory, err := NewStorageFactory(ctx, tempDir)
	require.NoError(t, err)
	require.NotNil(t, factory)

	assert.Equal(t, tempDir, factory.GetProjectRoot())

	storage := factory.GetStorage()
	assert.NotNil(t, storage)

	// test GetStorageForObject
	obj := map[string]any{objects.FieldKeyKind: "backlog_item"}
	objStorage := factory.GetStorageForObject(obj)
	assert.NotNil(t, objStorage)

	// test GetStorageForKind
	kindStorage := factory.GetStorageForKind("backlog_item")
	assert.NotNil(t, kindStorage)
	assert.Equal(t, objStorage, kindStorage)

	// test isStrategicKind
	assert.True(t, isStrategicKind("backlog_item"))
	assert.False(t, isStrategicKind("unknown_kind"))

	// test GetPool
	pool := factory.GetPool()
	assert.Nil(t, pool) // file storage has no pool

	// test Shutdown
	err = factory.Shutdown(ctx)
	assert.NoError(t, err)

	// test IsGraphBackend
	assert.False(t, factory.IsGraphBackend("unknown"))

	// test IsFileBackend
	assert.True(t, factory.IsFileBackend("unknown"))

	// test UnwrapToFileObjectStorage
	fileStorage := UnwrapToFileObjectStorage(storage)
	assert.NotNil(t, fileStorage)
	assert.Nil(t, UnwrapToFileObjectStorage(nil))
}

func TestStorageFactory_ModeResolution(t *testing.T) {
	tempDir := t.TempDir()
	SetupTestRootLikeSetupTestEnvironmentForExportTest(t, tempDir)

	SetGraphConnectionProvider(&mockGraphProvider{enabled: true})
	t.Cleanup(func() {
		SetGraphConnectionProvider(nil)
	})

	ctx := context.Background()

	// 1. By default, when graph is enabled but NO hybrid_legacy opt-in env is set,
	// StorageFactory must default to FileObjectStorage.
	t.Setenv(zqkenv.StorageMode().Name(), "")
	t.Setenv(zqkenv.StorageModeHybridLegacy().Name(), "")

	factoryDefault, err := NewStorageFactory(ctx, tempDir)
	require.NoError(t, err)
	defaultStorage := UnwrapToFileObjectStorage(factoryDefault.GetStorage())
	require.NotNil(t, defaultStorage, "StorageFactory should default to FileObjectStorage even if graph is enabled")
	_, isHybrid := factoryDefault.GetStorage().(*HybridObjectStorage)
	assert.False(t, isHybrid)
	_, isProj := factoryDefault.GetStorage().(*FileFirstProjectionStorage)
	if m, ok := factoryDefault.GetStorage().(*MeshObjectStorage); ok && m != nil {
		_, isProj = m.local.(*FileFirstProjectionStorage)
		_, isHybrid = m.local.(*HybridObjectStorage)
	}
	assert.False(t, isHybrid, "default must not be hybrid_legacy")
	assert.False(t, isProj, "default must not auto-enable file+projection when graph is up")

	// 2. Opt-in via STORAGE_MODE_HYBRID_LEGACY=1
	t.Setenv(zqkenv.StorageModeHybridLegacy().Name(), "1")
	factoryHybrid1, err := NewStorageFactory(ctx, tempDir)
	require.NoError(t, err)
	h1 := factoryHybrid1.GetStorage()
	if m, ok := h1.(*MeshObjectStorage); ok && m != nil {
		h1 = m.local
	}
	_, isHybrid1 := h1.(*HybridObjectStorage)
	assert.True(t, isHybrid1, "StorageFactory should use HybridObjectStorage when STORAGE_MODE_HYBRID_LEGACY=1")

	// 3. Opt-in via STORAGE_MODE=hybrid_legacy
	t.Setenv(zqkenv.StorageModeHybridLegacy().Name(), "")
	t.Setenv(zqkenv.StorageMode().Name(), "hybrid_legacy")
	factoryHybrid2, err := NewStorageFactory(ctx, tempDir)
	require.NoError(t, err)
	h2 := factoryHybrid2.GetStorage()
	if m, ok := h2.(*MeshObjectStorage); ok && m != nil {
		h2 = m.local
	}
	_, isHybrid2 := h2.(*HybridObjectStorage)
	assert.True(t, isHybrid2, "StorageFactory should use HybridObjectStorage when STORAGE_MODE=hybrid_legacy")

	// 4. Opt-in via STORAGE_MODE=file+projection (B2)
	t.Setenv(zqkenv.StorageMode().Name(), "file+projection")
	factoryProj, err := NewStorageFactory(ctx, tempDir)
	require.NoError(t, err)
	p := factoryProj.GetStorage()
	if m, ok := p.(*MeshObjectStorage); ok && m != nil {
		p = m.local
	}
	_, isProj2 := p.(*FileFirstProjectionStorage)
	assert.True(t, isProj2, "StorageFactory should use FileFirstProjectionStorage when STORAGE_MODE=file+projection")
}

func TestGetFileObjectStorage(t *testing.T) {
	tempDir := t.TempDir()
	SetupTestRootLikeSetupTestEnvironmentForExportTest(t, tempDir)

	s, err := GetFileObjectStorage(tempDir)
	require.NoError(t, err)
	require.NotNil(t, s)

	sTest, err := GetFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	require.NotNil(t, sTest)
}
// tdd refresh
