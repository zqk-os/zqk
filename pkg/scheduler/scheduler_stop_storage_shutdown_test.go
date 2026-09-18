package scheduler

import (
	"context"
	"sync/atomic"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

type storageShutdownSpy struct {
	storagepkg.ObjectStorageProvider
	called atomic.Int32
}

func (s *storageShutdownSpy) Shutdown(ctx context.Context) error {
	s.called.Store(1)
	if sd, ok := s.ObjectStorageProvider.(interface {
		Shutdown(context.Context) error
	}); ok {
		return sd.Shutdown(ctx)
	}
	return nil
}

func TestScheduler_Stop_CallsConcreteStorageShutdown(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	// Permission gate: Stop() requires manage:scheduler.
	secCtx := pkgctx.NewSecurityContext(
		"ACC-TEST",
		[]string{"developer"},
		[]string{"manage:scheduler"},
	)
	sched.SetSecurityContext(secCtx)

	// Force Stop() to go past the "not running" early return.
	sched.runningMu.Lock()
	sched.running = true
	sched.runningMu.Unlock()

	spy := &storageShutdownSpy{ObjectStorageProvider: sched.storage}
	sched.storage = spy

	sched.Stop()

	if spy.called.Load() != 1 {
		t.Fatalf("expected storage Shutdown() to be called exactly once, got %d", spy.called.Load())
	}
}
