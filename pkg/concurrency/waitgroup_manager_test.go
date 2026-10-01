package concurrency

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testWGObserver struct {
	created   atomic.Int32
	added     atomic.Int32
	done      atomic.Int32
	waited    atomic.Int32
	completed atomic.Int32
}

func (o *testWGObserver) OnGroupCreated(id, operation string)                { o.created.Add(1) }
func (o *testWGObserver) OnGroupAdd(id string, delta int)                    { o.added.Add(int32(delta)) }
func (o *testWGObserver) OnGroupDone(id string)                              { o.done.Add(1) }
func (o *testWGObserver) OnGroupWait(id string)                              { o.waited.Add(1) }
func (o *testWGObserver) OnGroupCompleted(id string, duration time.Duration) { o.completed.Add(1) }

func TestWaitGroupManager(t *testing.T) {
	mgr := NewWaitGroupManager()
	observer := &testWGObserver{}
	mgr.SetObserver(observer)

	t.Run("CreateGroup and Lifecycle", func(t *testing.T) {
		wg := mgr.CreateGroup("grp-1", "test-op")
		require.NotNil(t, wg)
		assert.Equal(t, 1, mgr.GroupCount())
		assert.Equal(t, int32(1), observer.created.Load())

		// Same group ID returns existing
		sameWG := mgr.CreateGroup("grp-1", "test-op")
		assert.Same(t, wg, sameWG)

		mgr.Add("grp-1", 1)
		assert.Equal(t, int32(1), observer.added.Load())

		mgr.Done("grp-1")
		assert.Equal(t, int32(1), observer.done.Load())

		mgr.Wait("grp-1")
		assert.Equal(t, int32(1), observer.waited.Load())
		assert.Equal(t, int32(1), observer.completed.Load())
	})

	t.Run("WaitWithTimeout and WaitWithContext", func(t *testing.T) {
		mgr.CreateGroup("grp-timeout", "timeout-op")
		mgr.Add("grp-timeout", 1)

		// Timeout case
		completed := mgr.WaitWithTimeout("grp-timeout", 10*time.Millisecond)
		assert.False(t, completed)

		// Context cancellation case
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		err := mgr.WaitWithContext(ctx, "grp-timeout")
		assert.ErrorIs(t, err, context.DeadlineExceeded)

		// Complete it
		mgr.Done("grp-timeout")
		completed = mgr.WaitWithTimeout("grp-timeout", 100*time.Millisecond)
		assert.True(t, completed)
	})

	t.Run("CleanupStaleGroups", func(t *testing.T) {
		cleanMgr := NewWaitGroupManager()
		cleanMgr.CreateGroup("grp-stale", "stale-op")
		assert.Equal(t, 1, cleanMgr.GroupCount())

		// Clean with small maxAge
		time.Sleep(2 * time.Millisecond)
		cleaned := cleanMgr.CleanupStaleGroups(1 * time.Millisecond)
		assert.Equal(t, 1, cleaned)
		assert.Equal(t, 0, cleanMgr.GroupCount())
	})
}
