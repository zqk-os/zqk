package service_test

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/service"
)

func TestNewManager_AutoDetect(t *testing.T) {
	mgr := service.NewManager()
	require.NotNil(t, mgr)
	require.NotNil(t, mgr.Adapter())

	if runtime.GOOS == "darwin" {
		assert.Equal(t, "launchd", mgr.Adapter().Name())
	}
}

func TestManager_LifecycleWithMock(t *testing.T) {
	mock := service.NewMockAdapter("test-mock", true)
	mgr := service.NewManager(service.WithAdapter(mock))
	assert.Equal(t, "test-mock", mgr.Adapter().Name())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	spec := service.ServiceSpec{
		ID:         "com.example.managed",
		Executable: "/usr/bin/sample",
	}

	// 1. Install
	err := mgr.Install(ctx, spec)
	require.NoError(t, err)

	// 2. Status initially stopped
	st, err := mgr.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, service.StateStopped, st.State)

	// 3. Start
	err = mgr.Start(ctx, spec.ID)
	require.NoError(t, err)

	st, err = mgr.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, service.StateRunning, st.State)

	// 4. Restart
	err = mgr.Restart(ctx, spec.ID)
	require.NoError(t, err)

	// 5. Stop
	err = mgr.Stop(ctx, spec.ID)
	require.NoError(t, err)

	// 6. CleanupLegacy
	cleaned, err := mgr.CleanupLegacy(ctx, []string{"com.example.managed"})
	require.NoError(t, err)
	assert.Equal(t, []string{"com.example.managed"}, cleaned)
}

func TestManager_ConcurrencyThreadSafety(t *testing.T) {
	mock := service.NewMockAdapter("threadsafe-mock", true)
	mgr := service.NewManager(service.WithAdapter(mock))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	concurrency := 16

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			spec := service.ServiceSpec{
				ID:         assert.AnError.Error() + string(rune(id+65)),
				Executable: "/bin/true",
			}
			_ = mgr.Install(ctx, spec)
			_, _ = mgr.Status(ctx, spec.ID)
		}(i)
	}

	wg.Wait()
}
