package filecas

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCycle12_Filecas_MetricsAndDarwinSync(t *testing.T) {
	// 1. noopStorageMetrics coverage
	var m noopStorageMetrics
	m.RecordCreate(time.Millisecond, true)
	m.RecordRead(time.Millisecond, false)
	m.RecordUpdate(time.Millisecond, true)
	m.RecordDelete(time.Millisecond, false)
	m.RecordIndexFileLock(true, time.Millisecond)
	m.RecordIndexSave(time.Millisecond, nil, 5)
	m.RecordSetMapping(time.Millisecond, nil, true)
	m.RecordIndexReload()
	m.RecordRemoveMapping(time.Millisecond, nil)

	// 2. noopLockStrategy & noopLockHandle
	strategy := noopLockStrategy{}
	handle, err := strategy.AcquireLock("/tmp/test.lock", time.Second)
	require.NoError(t, err)
	require.NoError(t, handle.Release())

	// 3. DarwinSyncQueue helpers
	_ = IsDarwinSyncQueueDrained()
	cnt := DarwinSyncQueuePendingCount()
	assert.GreaterOrEqual(t, cnt, int64(0))

	err = DrainDarwinSyncQueue(50 * time.Millisecond)
	assert.NoError(t, err)

	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()
	_ = DrainDarwinSyncQueueContext(ctxCancel)
}

func TestCycle12_Filecas_NegativeMissTracking(t *testing.T) {
	tmpDir := t.TempDir()
	cas := NewContentAddressableStorage(tmpDir, "test_kind", nil)
	require.NotNil(t, cas)

	// Initial check: not a negative miss
	assert.False(t, cas.isNegativeMiss("MISSING-001"))

	// Record negative miss
	cas.recordNegativeMiss("MISSING-001")
	assert.True(t, cas.isNegativeMiss("MISSING-001"))

	// Clear negative miss
	cas.clearNegativeMiss("MISSING-001")
	assert.False(t, cas.isNegativeMiss("MISSING-001"))
}
