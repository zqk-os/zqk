package scheduler

// BLI-177483 inventory: setupTestEnvironment → testkit.PrepareIsolatedTempProject (RegisterTempProjectTeardown) in scheduler_test.go.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestRejectHostServiceOwnedBackgroundStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	root := t.TempDir()
	rootID := hostservice.RootID(root)
	unitLabel := hostservice.UnitLabel(rootID)
	if err := hostservice.SaveRegistry(&hostservice.Registry{Entries: []hostservice.Entry{
		{
			RootID:       rootID,
			AbsRoot:      root,
			UnitLabel:    unitLabel,
			InstalledAt:  time.Now(),
			DesiredState: hostservice.DesiredStateEnabled,
		},
	}}); err != nil {
		t.Fatal(err)
	}

	err := rejectHostServiceOwnedBackgroundStart(root)
	if err == nil || !strings.Contains(err.Error(), unitLabel) {
		t.Fatalf("expected host-service ownership rejection for %s, got %v", unitLabel, err)
	}
}

func TestGetSchedulerStatus_ThreadSafety(t *testing.T) {
	// Not t.Parallel(): setupTestEnvironment uses t.Setenv(ZQK_TEST_ROOT).
	// Test concurrent access to getSchedulerStatus
	testRoot, _, _ := setupTestEnvironment(t)

	ctx := cli.ContextForProjectRoot(testRoot)

	// Test concurrent calls
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("scheduler_test", "check status thread safety").
			StartSimple(func() {
				status, err := getSchedulerStatus(ctx)
				if err != nil {
					t.Errorf("getSchedulerStatus() error = %v", err)
				}
				// Verify status is valid (not nil, has project root)
				if status == nil {
					t.Error("getSchedulerStatus() returned nil status")
					return
				}
				if status.ProjectRoot == emptyValue {
					t.Error("getSchedulerStatus() returned empty project root")
				}
				done <- true
			})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGetSchedulerStatus_WithPIDFile(t *testing.T) {
	// Not t.Parallel(): setupTestEnvironment uses t.Setenv(ZQK_TEST_ROOT).
	// Test concurrent access when PID file exists
	testRoot, _, _ := setupTestEnvironment(t)

	// Write PID file using the same path as pkg/scheduler getPIDFilePath / paths.SchedulerPIDFilePath.
	pidFilePath := paths.SchedulerPIDFilePath(testRoot)
	schedulerDir := filepath.Dir(pidFilePath)
	if err := fileutil.MkdirAll(schedulerDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create scheduler directory: %v", err)
	}

	currentPID := os.Getpid()
	pidStr := strconv.Itoa(currentPID)
	if err := fileutil.WriteFile(pidFilePath, []byte(pidStr), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write PID file: %v", err)
	}
	defer fileutil.Remove(pidFilePath)

	ctx := cli.ContextForProjectRoot(testRoot)

	// Test concurrent calls with PID file
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("scheduler_test", "concurrent status check").
			StartSimple(func() {
				status, err := getSchedulerStatus(ctx)
				if err != nil {
					t.Errorf("getSchedulerStatus() error = %v", err)
				}
				if status == nil {
					t.Error("getSchedulerStatus() returned nil status")
					return
				}
				// All calls should see the same status (scheduler running)
				if !status.Running {
					t.Error("getSchedulerStatus() should detect scheduler as running")
				}
				done <- true
			})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestRequireSchedulerRunning_Concurrent(t *testing.T) {
	// Not t.Parallel(): setupTestEnvironment uses t.Setenv(ZQK_TEST_ROOT).
	// Test concurrent calls to requireSchedulerRunning
	testRoot, _, _ := setupTestEnvironment(t)

	ctx := cli.ContextForProjectRoot(testRoot)

	// Test concurrent calls when scheduler is not running
	// All should return the same error
	done := make(chan bool, 10)
	errors := make(chan error, 10)
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("scheduler_test", "require running concurrent").
			StartSimple(func() {
				_, err := requireSchedulerRunning(ctx)
				errors <- err
				done <- true
			})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// All should have returned the same error
	close(errors)
	firstErr := <-errors
	for err := range errors {
		if err == nil != (firstErr == nil) {
			t.Error("requireSchedulerRunning() returned inconsistent errors")
		}
		if err != nil && firstErr != nil && err.Error() != firstErr.Error() {
			t.Error("requireSchedulerRunning() returned different error messages")
		}
	}
}

func TestGetSchedulerInstance_Concurrent(t *testing.T) {
	// Not t.Parallel(): setupTestEnvironment uses t.Setenv(ZQK_TEST_ROOT).
	// Test concurrent calls to getSchedulerInstance
	testRoot, _, _ := setupTestEnvironment(t)

	ctx := cli.ContextForProjectRoot(testRoot)

	// Test concurrent calls when scheduler is not running
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("scheduler_test", "get instance concurrent").
			StartSimple(func() {
				_, _, err := getSchedulerInstance(ctx)
				if err == nil {
					t.Error("getSchedulerInstance() should return error when scheduler is not running")
				}
				done <- true
			})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
