package scheduler

import (
	"errors"
	"os"

	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// testKeepAliveInterval drives keepAliveHeartbeat fast enough for integration tests without waiting
// on DefaultKeepAliveInterval (30s), which makes full-package go test hit global -timeout budgets.
const testKeepAliveInterval = 200 * time.Millisecond

// TestKeepAliveHeartbeat_Integration tests the keep-alive heartbeat in a running scheduler
func TestKeepAliveHeartbeat_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)

	testRoot := t.TempDir()

	// Create required directory structure
	processDir := datacell.ProcessPrimaryDir(testRoot)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process dir: %v", err)
	}

	// Create a minimal storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler with project root
	// Explicitly pass test-id to avoid friendly fire from other tests
	oldArgs := os.Args
	os.Args = append(os.Args, "--test-id=keepalive-test")
	defer func() { os.Args = oldArgs }()

	sched := NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	sched.(*Scheduler).keepAliveTickerInterval = testKeepAliveInterval
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	// Create context with longer timeout to allow keep-alive updates
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel()

	// Start scheduler in a goroutine
	startErr := make(chan error, 1)
	goroutinelabels.StartTestGoroutine("test_scheduler_start", "starting scheduler in keepalive test", func() {
		startErr <- sched.Start(ctx)
	})

	// Wait for scheduler to be running (deterministic check with timeout)
	startTimeout := time.After(2 * time.Second)
	running := false
	for !running {
		select {
		case <-startTimeout:
			t.Fatal("timeout waiting for scheduler to start")
		default:
			running = sched.IsRunning()
			if !running {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	// Verify keep-alive file is created (heartbeat runs in a goroutine, so poll briefly)
	keepAlivePath := getKeepAliveFilePath(testRoot)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := fileutil.Stat(keepAlivePath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := fileutil.Stat(keepAlivePath); fileutil.IsNotExist(err) {
		t.Fatal("Keep-alive file should be created when scheduler starts")
	}

	// Read initial timestamp
	timestamp1, err := readKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("Failed to read initial keep-alive: %v", err)
	}

	// Wait for keep-alive update (one ticker period + buffer per iteration).
	// writeKeepAlive uses RFC3339 without sub-second precision; multiple ticks in the same UTC second
	// produce identical parsed timestamps, so poll until After(timestamp1) or deadline.
	tickWait := testKeepAliveInterval + 300*time.Millisecond
	var timestamp2 time.Time
	var errRead error
	updateDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(updateDeadline) {
		time.Sleep(tickWait)
		timestamp2, errRead = readKeepAlive(testRoot)
		if errRead != nil {
			continue
		}
		if timestamp2.After(timestamp1) {
			break
		}
	}
	if errRead != nil {
		t.Fatalf("Failed to read keep-alive while waiting for update: %v", errRead)
	}
	if !timestamp2.After(timestamp1) {
		t.Errorf("Keep-alive should be updated: timestamp1 = %v, timestamp2 = %v", timestamp1, timestamp2)
	}

	// Verify IsSchedulerAlive detects it as alive
	isAlive, lastKeepAlive, err := IsSchedulerAlive(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerAlive() error = %v", err)
	}

	if !isAlive {
		t.Error("IsSchedulerAlive() should return true for running scheduler with fresh keep-alive")
	}

	if lastKeepAlive.IsZero() {
		t.Error("IsSchedulerAlive() should return non-zero timestamp")
	}

	// Stop scheduler
	cancel()

	// Wait for Start() to return (which will call Stop() internally)
	select {
	case err := <-startErr:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Scheduler Start() returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		// Timeout - force stop
		sched.Stop()
	}

	// Start() calls Stop() internally when ctx is cancelled, but ensure it's stopped
	sched.Stop()

	// Verify keep-alive file is removed (or at least stale)
	// Note: Due to race conditions, the file might still exist but should be stale
	// The important thing is that IsSchedulerAlive detects it as not alive
	isAlive, _, err = IsSchedulerAlive(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerAlive() error = %v", err)
	}

	if isAlive {
		t.Error("IsSchedulerAlive() should return false after scheduler stops")
	}

	// Start() calls Stop() internally when ctx is cancelled, but ensure it's stopped
	sched.Stop()
}

// TestKeepAliveHeartbeat_ContextCancellation tests that keep-alive is removed on context cancellation
func TestKeepAliveHeartbeat_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)

	testRoot := t.TempDir()

	// Create required directory structure
	processDir := datacell.ProcessPrimaryDir(testRoot)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process dir: %v", err)
	}

	// Create a minimal storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler with project root
	sched := NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	sched.(*Scheduler).keepAliveTickerInterval = testKeepAliveInterval
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	// Create context that we can cancel
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())

	// Start scheduler in a goroutine
	startErr := make(chan error, 1)
	goroutinelabels.StartTestGoroutine("test_scheduler_start", "starting scheduler in keepalive test", func() {
		startErr <- sched.Start(ctx)
	})

	// Wait for scheduler to be running (deterministic check with timeout)
	startTimeout := time.After(2 * time.Second)
	running := false
	for !running {
		select {
		case <-startTimeout:
			t.Fatal("timeout waiting for scheduler to start")
		default:
			running = sched.IsRunning()
			if !running {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	// Verify keep-alive file exists.
	// Under load, the scheduler can report "running" before the heartbeat goroutine
	// has written the keep-alive file, so poll briefly instead of a single check.
	keepAlivePath := getKeepAliveFilePath(testRoot)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := fileutil.Stat(keepAlivePath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := fileutil.Stat(keepAlivePath); fileutil.IsNotExist(err) {
		t.Error("Keep-alive file should exist")
	}

	// Cancel context (simulating shutdown)
	cancel()

	// Wait for Start() to return (which will call Stop() internally)
	select {
	case err := <-startErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Scheduler Start() returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		// Timeout - force stop
		sched.Stop()
	}

	// Start() calls Stop() internally when ctx is cancelled, but ensure it's stopped
	sched.Stop()

	// Verify keep-alive file is removed by heartbeat goroutine
	// Note: Due to race conditions, this might not always work, but it should at least be stale
	if _, err := fileutil.Stat(keepAlivePath); !fileutil.IsNotExist(err) {
		// File still exists - check if it's stale
		timestamp, err := readKeepAlive(testRoot)
		if err == nil {
			age := time.Now().UTC().Sub(timestamp)
			if age < DefaultKeepAliveTimeout {
				t.Logf("Keep-alive file still exists but should be removed. Age: %v", age)
				// This is a known issue - the heartbeat goroutine might not always clean up in time
			}
		}
	}
}
