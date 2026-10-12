package scheduler

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

func TestJobStateRegistry_WriteThroughCacheAndMTimeInvalidation(t *testing.T) {
	tempDir := t.TempDir()
	regInt := NewJobStateRegistry(tempDir)
	reg, ok := regInt.(*JobStateRegistry)
	if !ok {
		t.Fatalf("expected *JobStateRegistry, got %T", regInt)
	}

	jobID := "job-cache-test-1"
	executionID := "exec-100"

	// 1. Register execution (in_progress)
	if err := reg.RegisterExecution(jobID, executionID, 12345); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	statePath := reg.stateFilePathForJobExecution(jobID, executionID)

	// Verify cached file state exists
	fi, err := fileutil.Stat(statePath)
	if err != nil {
		t.Fatalf("stat state file failed: %v", err)
	}

	cached := reg.getCachedFileState(statePath, fi.ModTime(), fi.Size())
	if cached == nil {
		t.Fatalf("expected cached file state after write, got nil")
	}
	if cached.ExecutionID != executionID || cached.State != jobExecutionStateInProgress {
		t.Fatalf("unexpected cached state: %+v", cached)
	}

	// 2. Read through cached reader - should return identical state
	st, ok := reg.readCachedJobExecutionStateFile(statePath)
	if !ok || st == nil {
		t.Fatalf("readCachedJobExecutionStateFile failed")
	}
	if st.ExecutionID != executionID {
		t.Fatalf("expected executionID %s, got %s", executionID, st.ExecutionID)
	}

	// 3. Mutate file directly on disk and change modtime (simulating external update)
	time.Sleep(10 * time.Millisecond) // Ensure distinct modtime
	st.PolicyReason = "external-worker-update"
	b, err := yaml.Marshal(st)
	if err != nil {
		t.Fatalf("yaml marshal failed: %v", err)
	}
	if err := fileutil.WriteFile(statePath, b, paths.FilePerm644); err != nil {
		t.Fatalf("write direct file failed: %v", err)
	}

	// The old cache entry has old modtime/size, so readCachedJobExecutionStateFile must invalidate and reload
	updatedSt, ok := reg.readCachedJobExecutionStateFile(statePath)
	if !ok || updatedSt == nil {
		t.Fatalf("readCachedJobExecutionStateFile after external update failed")
	}
	if updatedSt.PolicyReason != "external-worker-update" {
		t.Fatalf("expected invalidated and reloaded state with reason 'external-worker-update', got %q", updatedSt.PolicyReason)
	}

	// 4. Complete execution - verify write-through updates cache
	if err := reg.CompleteExecution(jobID, executionID, "success"); err != nil {
		t.Fatalf("CompleteExecution failed: %v", err)
	}

	completedSt, ok := reg.readCachedJobExecutionStateFile(statePath)
	if !ok || completedSt == nil {
		t.Fatalf("readCachedJobExecutionStateFile after CompleteExecution failed")
	}
	if completedSt.State != jobExecutionStateCompleted {
		t.Fatalf("expected completed state, got %s", completedSt.State)
	}

	// 5. Test cache eviction
	reg.evictCachedFileState(statePath)
	fiAfter, _ := fileutil.Stat(statePath)
	if reg.getCachedFileState(statePath, fiAfter.ModTime(), fiAfter.Size()) != nil {
		t.Fatalf("expected nil after cache eviction")
	}
}
