package validation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestAsyncValidator_BasicOperations(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Enqueue some tasks
	_ = validator.Enqueue("TEST-001", "test_object", "test1.yaml", 1)
	_ = validator.Enqueue("TEST-002", "test_object", "test2.yaml", 2)
	_ = validator.Enqueue("TEST-003", "test_object", "test3.yaml", 3)

	// Wait a bit for processing
	time.Sleep(500 * time.Millisecond)

	// Check stats
	total, stale, withIssues, queueSize := validator.GetValidationStats()
	t.Logf("Stats: total=%d, stale=%d, withIssues=%d, queueSize=%d", total, stale, withIssues, queueSize)
}

func TestAsyncValidator_CacheIntegration(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)

	// Create a test file
	testFile := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), "TEST-001.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := fileutil.WriteFile(testFile, []byte("id: TEST-001\nkind: test_object\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Enqueue task
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for processing
	time.Sleep(1 * time.Second)

	// Check if cached
	state, exists := validator.GetCachedState("TEST-001")
	if !exists {
		t.Error("expected TEST-001 to be cached after validation")
	} else {
		t.Logf("Cached state: ObjectID=%s, LastValidated=%v", state.ObjectID, state.LastValidated)
	}
}

func TestAsyncValidator_ProgressReporting(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	for i := 1; i <= 5; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf("id: TEST-%03d\nkind: test_object\n", i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
		_ = validator.Enqueue(fmt.Sprintf("TEST-%03d", i), "test_object", testFile, 1)
	}

	// Monitor progress
	progressChan := validator.GetProgress()
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	completed := 0
	for {
		select {
		case <-ctx.Done():
			t.Logf("Progress monitoring completed: %d tasks processed", completed)
			return
		case progress, ok := <-progressChan:
			if !ok {
				t.Logf("Progress channel closed: %d tasks processed", completed)
				return
			}
			if progress.Status == "completed" {
				completed++
				t.Logf("Progress: %d completed, current: %s", completed, progress.CurrentObject)
			}
		}
	}
}

// TestAsyncValidator_Enqueue_CacheOptimization tests the cache optimization in Enqueue
func TestAsyncValidator_Enqueue_CacheOptimization(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Create a test file
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := "kind: test_object\nid: TEST-001"
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// First enqueue - should validate (not in cache)
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for validation to complete
	ctx := pkgctx.NewSystemContext()
	_, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf("ValidateNow failed: %v", err)
	}

	// Second enqueue with same file - should use cache (checksum matches)
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Verify it was cached (progress channel should receive completion)
	// Note: GetProgress() returns a read-only channel
	progressChan := validator.GetProgress()
	select {
	case progress := <-progressChan:
		if progress.Status != "completed" {
			t.Errorf("expected cached status 'completed', got %q", progress.Status)
		}
	case <-time.After(100 * time.Millisecond):
		// No progress update - might be fine if channel is full or validator not started
	}

	// Third enqueue with modified file - should re-validate (checksum changed)
	modifiedContent := "kind: test_object\nid: TEST-001\nmodified: true"
	if err := fileutil.WriteFile(testFile, []byte(modifiedContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write modified file: %v", err)
	}

	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Should enqueue for validation (not use cache)
	_, err = validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf("ValidateNow failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
}

// TestAsyncValidator_EnqueueBatch tests the EnqueueBatch function
func TestAsyncValidator_EnqueueBatch(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 0, 1*time.Second)

	// Create batch of tasks
	tasks := []ValidationTask{
		{ObjectID: "TEST-001", ObjectKind: "test_object", FilePath: "test1.yaml", Priority: 1},
		{ObjectID: "TEST-002", ObjectKind: "test_object", FilePath: "test2.yaml", Priority: 2},
		{ObjectID: "TEST-003", ObjectKind: "test_object", FilePath: "test3.yaml", Priority: 3},
	}

	validator.EnqueueBatch(tasks)

	// Verify tasks were enqueued
	_, _, _, queueSize := validator.GetValidationStats()
	if queueSize != 3 {
		t.Errorf("EnqueueBatch() expected queue size 3, got %d", queueSize)
	}

	// Verify we can get progress updates
	progressChan := validator.GetProgress()
	select {
	case <-progressChan:
		// Progress update received
	case <-time.After(100 * time.Millisecond):
		// No progress update yet (may be processing)
	}
}

// TestAsyncValidator_EnqueueBatch_DedupesByObjectID verifies EnqueueBatch deduplicates by ObjectID (first wins).
func TestAsyncValidator_EnqueueBatch_DedupesByObjectID(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)
	defer func() { _ = validator.Stop() }() //nolint:errcheck

	// Batch with duplicate ObjectIDs (same ID, different paths - e.g. from discovery)
	tasks := []ValidationTask{
		{ObjectID: "AUD-001", ObjectKind: "audit_event", FilePath: "audit/p1.yaml", Priority: 1},
		{ObjectID: "AUD-001", ObjectKind: "audit_event", FilePath: "audit/p2.yaml", Priority: 1},
		{ObjectID: "AUD-001", ObjectKind: "audit_event", FilePath: "audit/p3.yaml", Priority: 1},
	}

	validator.EnqueueBatch(tasks)

	_, _, _, queueSize := validator.GetValidationStats()
	if queueSize != 1 {
		t.Errorf("EnqueueBatch() with duplicate ObjectIDs: expected queue size 1, got %d", queueSize)
	}
}

// TestAsyncValidator_EnqueueBatch_QueueOwnsTaskCopies verifies that EnqueueBatch copies tasks
// so the queue holds its own data. Caller reuses the slice (slice[:0] + append), which overwrites
// the backing array; if the queue stored pointers to slice elements, dequeued tasks would have
// overwritten data and the same object could be validated many times.
func TestAsyncValidator_EnqueueBatch_QueueOwnsTaskCopies(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, 1*time.Second)

	// Create files so worker ReadFile succeeds and validation func runs
	for _, name := range []string{"batch1-a.yaml", "batch1-b.yaml", "batch1-c.yaml"} {
		p := filepath.Join(testRoot, name)
		if err := fileutil.WriteFile(p, []byte("id: "+name+"\nkind: test_object\n"), paths.FilePerm644); err != nil {
			t.Fatalf("write test file: %v", err)
		}
	}

	// Slice that caller will reuse (same pattern as enqueueFilesAsDiscovered)
	tasks := make([]ValidationTask, 0, 100)
	tasks = append(tasks,
		ValidationTask{ObjectID: "BATCH1-A", ObjectKind: "test_object", FilePath: filepath.Join(testRoot, "batch1-a.yaml"), Priority: 1},
		ValidationTask{ObjectID: "BATCH1-B", ObjectKind: "test_object", FilePath: filepath.Join(testRoot, "batch1-b.yaml"), Priority: 1},
		ValidationTask{ObjectID: "BATCH1-C", ObjectKind: "test_object", FilePath: filepath.Join(testRoot, "batch1-c.yaml"), Priority: 1},
	)

	var processedIDs []string
	var mu sync.Mutex
	validator.SetValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, data []byte) (*ValidationState, error) {
		mu.Lock()
		processedIDs = append(processedIDs, objectID)
		mu.Unlock()
		return &ValidationState{ObjectID: objectID, ObjectKind: objectKind}, nil
	})
	validator.SetResultCallback(func(_ *ValidationState, _ error) {})

	if err := validator.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = validator.Stop() }()

	validator.EnqueueBatch(tasks)

	// Simulate caller reusing the slice (same backing array overwritten)
	tasks = tasks[:0]
	tasks = append(tasks,
		ValidationTask{ObjectID: "OVERWRITE-X", ObjectKind: "test_object", FilePath: "overwrite-x.yaml", Priority: 1},
		ValidationTask{ObjectID: "OVERWRITE-Y", ObjectKind: "test_object", FilePath: "overwrite-y.yaml", Priority: 1},
		ValidationTask{ObjectID: "OVERWRITE-Z", ObjectKind: "test_object", FilePath: "overwrite-z.yaml", Priority: 1},
	)
	_ = tasks // overwritten to simulate bug; if queue held slice pointers, processed IDs would be OVERWRITE-*

	// Wait for all 3 tasks to be processed (workers run validation func which appends to processedIDs)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(processedIDs)
		mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	got := append([]string(nil), processedIDs...)
	mu.Unlock()

	wantSet := map[string]bool{"BATCH1-A": true, "BATCH1-B": true, "BATCH1-C": true}
	if len(got) != 3 {
		t.Errorf("processed count: got %d (%v), want 3 (queue must own task copies)", len(got), got)
	} else {
		for _, id := range got {
			if !wantSet[id] {
				t.Errorf("processed ID %q is from overwritten slice (want only BATCH1-A, BATCH1-B, BATCH1-C)", id)
			}
		}
	}
}

// TestAsyncValidator_GetProgress tests the GetProgress function
func TestAsyncValidator_GetProgress(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// GetProgress should return a channel
	progressChan := validator.GetProgress()
	if progressChan == nil {
		t.Error("GetProgress() returned nil channel")
	}

	// Channel should be readable
	// (We can't test if it's writable since it's read-only)
	select {
	case <-progressChan:
		// Channel is readable (may have buffered messages)
	default:
		// Channel is empty, which is fine
	}
}

// TestAsyncValidator_GetCachedState tests the GetCachedState function
func TestAsyncValidator_GetCachedState(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Test getting non-existent state
	state, exists := validator.GetCachedState("NONEXISTENT-001")
	if exists {
		t.Error("GetCachedState() should return false for non-existent state")
	}
	if state != nil {
		t.Error("GetCachedState() should return nil state when not found")
	}

	// Create a test file and validate to populate cache
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := "kind: test_object\nid: TEST-001"
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	validatedState, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf("ValidateNow failed: %v", err)
	}

	// Now should be cached
	state, exists = validator.GetCachedState("TEST-001")
	if !exists {
		t.Error("GetCachedState() should return true for cached state")
	}
	if state == nil {
		t.Error("GetCachedState() should return non-nil state when found")
		return
	}
	if state.ObjectID != "TEST-001" {
		t.Errorf("GetCachedState() returned state with ObjectID %s, want TEST-001", state.ObjectID)
	}
	if state.Checksum != validatedState.Checksum {
		t.Errorf("GetCachedState() returned state with checksum %s, want %s", state.Checksum, validatedState.Checksum)
	}
}

// TestAsyncValidator_GetAllCachedStates tests the GetAllCachedStates function
func TestAsyncValidator_GetAllCachedStates(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Initially should be empty
	states := validator.GetAllCachedStates()
	if len(states) != 0 {
		t.Errorf("GetAllCachedStates() expected empty slice, got %d states", len(states))
	}

	// Create and validate multiple objects
	ctx := pkgctx.NewSystemContext()
	for i := 1; i <= 3; i++ {
		testFile := filepath.Join(testRoot, fmt.Sprintf("test%d.yaml", i))
		testContent := fmt.Sprintf("kind: test_object\nid: TEST-%03d", i)
		if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := validator.ValidateNow(ctx, fmt.Sprintf("TEST-%03d", i), "test_object", testFile)
		if err != nil {
			t.Fatalf("ValidateNow failed: %v", err)
		}
	}

	// Should have 3 cached states
	states = validator.GetAllCachedStates()
	if len(states) != 3 {
		t.Errorf("GetAllCachedStates() expected 3 states, got %d", len(states))
	}

	// Verify all states are present
	stateMap := make(map[string]bool)
	for _, state := range states {
		stateMap[state.ObjectID] = true
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("TEST-%03d", i)
		if !stateMap[id] {
			t.Errorf("GetAllCachedStates() missing state for %s", id)
		}
	}
}

// TestAsyncValidator_Start tests the Start function
func TestAsyncValidator_Start(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, 1*time.Second)

	// Start should succeed
	if err := validator.Start(); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}

	// Starting again should return error
	if err := validator.Start(); err == nil {
		t.Error("Start() expected error when already running, got nil")
	}

	// Clean up
	_ = validator.Stop() //nolint:errcheck // Test cleanup - errors are acceptable
}

// TestAsyncValidator_Stop tests the Stop function
func TestAsyncValidator_Stop(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, 1*time.Second)

	// Stop when not started should succeed
	if err := validator.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}

	// Start and then stop
	if err := validator.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if err := validator.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}

	// Stop again should succeed (idempotent)
	if err := validator.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

// TestAsyncValidator_SetValidationFunc tests the SetValidationFunc function
func TestAsyncValidator_SetValidationFunc(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Set a custom validation function
	called := false
	customFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		called = true
		return &ValidationState{
			ObjectID:   objectID,
			ObjectKind: objectKind,
			Checksum:   "test-checksum",
		}, nil
	})

	validator.SetValidationFunc(customFunc)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create a test file and enqueue
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := "kind: test_object\nid: TEST-001"
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for validation to complete by checking progress channel
	progressChan := validator.GetProgress()
	timeout := time.After(2 * time.Second)
	select {
	case <-progressChan:
		// Progress update received
	case <-timeout:
		t.Log("Timeout waiting for validation progress")
	}

	// Give a small buffer for the function to be called
	time.Sleep(50 * time.Millisecond)

	// Custom function should have been called
	if !called {
		t.Error("SetValidationFunc() custom function was not called")
	}
}

// TestAsyncValidator_GetValidationStats tests the GetValidationStats function
func TestAsyncValidator_GetValidationStats(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Initially should be empty
	total, stale, withIssues, queueSize := validator.GetValidationStats()
	if total != 0 {
		t.Errorf("GetValidationStats() total = %d, want 0", total)
	}
	if stale != 0 {
		t.Errorf("GetValidationStats() stale = %d, want 0", stale)
	}
	if withIssues != 0 {
		t.Errorf("GetValidationStats() withIssues = %d, want 0", withIssues)
	}
	if queueSize != 0 {
		t.Errorf("GetValidationStats() queueSize = %d, want 0", queueSize)
	}

	// Create and validate an object
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := "kind: test_object\nid: TEST-001"
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	_, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf("ValidateNow failed: %v", err)
	}

	// Stats should reflect the validated object
	total, _, _, _ = validator.GetValidationStats()
	if total < 1 {
		t.Errorf("GetValidationStats() total = %d, want >= 1", total)
	}
}

// TestAsyncValidator_ComputeChecksumFromData tests the computeChecksumFromData function
func TestAsyncValidator_ComputeChecksumFromData(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Test with valid data
	data1 := []byte("test content")
	checksum1 := validator.computeChecksumFromData(data1)
	if checksum1 == emptyValue {
		t.Error("computeChecksumFromData() returned empty string for valid data")
	}

	// Same data should produce same checksum
	checksum2 := validator.computeChecksumFromData(data1)
	if checksum1 != checksum2 {
		t.Errorf("computeChecksumFromData() returned different checksums for same data: %q vs %q", checksum1, checksum2)
	}

	// Different data should produce different checksum
	data2 := []byte("different content")
	checksum3 := validator.computeChecksumFromData(data2)
	if checksum1 == checksum3 {
		t.Error("computeChecksumFromData() returned same checksum for different data")
	}

	// Empty data should still produce a checksum
	emptyData := []byte{}
	checksum4 := validator.computeChecksumFromData(emptyData)
	if checksum4 == emptyValue {
		t.Error("computeChecksumFromData() returned empty string for empty data")
	}
}

// TestAsyncValidator_ValidateObject tests the validateObject function
func TestAsyncValidator_ValidateObject(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Create a test file
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := "kind: test_object\nid: TEST-001"
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx := pkgctx.NewSystemContext()

	// Test with valid file
	state, err := validator.validateObject(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf("validateObject() error = %v, want nil", err)
	}
	if state == nil {
		t.Fatal("validateObject() returned nil state")
	}
	if state.ObjectID != "TEST-001" {
		t.Errorf("validateObject() ObjectID = %s, want TEST-001", state.ObjectID)
	}
	if state.ObjectKind != "test_object" {
		t.Errorf("validateObject() ObjectKind = %s, want test_object", state.ObjectKind)
	}
	if state.FilePath != testFile {
		t.Errorf("validateObject() FilePath = %s, want %s", state.FilePath, testFile)
	}
	if state.Checksum == emptyValue {
		t.Error("validateObject() Checksum is empty")
	}

	// Test with non-existent file
	nonExistentFile := filepath.Join(testRoot, "nonexistent.yaml")
	_, err = validator.validateObject(ctx, "TEST-002", "test_object", nonExistentFile)
	if err == nil {
		t.Error("validateObject() expected error for non-existent file, got nil")
	}
}

// TestAsyncValidator_ValidateObjectWithData tests the validateObjectWithData function
func TestAsyncValidator_ValidateObjectWithData(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := []byte("kind: test_object\nid: TEST-001")
	ctx := pkgctx.NewSystemContext()

	// Test without validation function (fallback path)
	state, err := validator.validateObjectWithData(ctx, "TEST-001", "test_object", testFile, testContent)
	if err != nil {
		t.Fatalf("validateObjectWithData() error = %v, want nil", err)
	}
	if state == nil {
		t.Fatal("validateObjectWithData() returned nil state")
	}
	if state.ObjectID != "TEST-001" {
		t.Errorf("validateObjectWithData() ObjectID = %s, want TEST-001", state.ObjectID)
	}
	if state.Checksum == emptyValue {
		t.Error("validateObjectWithData() Checksum is empty")
	}
	if state.Issues == nil {
		t.Error("validateObjectWithData() Issues is nil")
	}
	if state.Metadata == nil {
		t.Error("validateObjectWithData() Metadata is nil")
	}

	// Test with validation function set
	customState := &ValidationState{
		ObjectID:   "TEST-002",
		ObjectKind: "test_object",
		Checksum:   "custom-checksum",
	}
	customFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		return customState, nil
	})
	validator.SetValidationFunc(customFunc)

	state, err = validator.validateObjectWithData(ctx, "TEST-002", "test_object", testFile, testContent)
	if err != nil {
		t.Fatalf("validateObjectWithData() error = %v, want nil", err)
	}
	if state.ObjectID != "TEST-002" {
		t.Errorf("validateObjectWithData() ObjectID = %s, want TEST-002", state.ObjectID)
	}
	if state.Checksum != "custom-checksum" {
		t.Errorf("validateObjectWithData() Checksum = %s, want custom-checksum", state.Checksum)
	}

	// Test with validation function that returns error
	errorFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		return nil, errors.New("validation failed")
	})
	validator.SetValidationFunc(errorFunc)

	_, err = validator.validateObjectWithData(ctx, "TEST-003", "test_object", testFile, testContent)
	if err == nil {
		t.Error("validateObjectWithData() expected error from validation function, got nil")
	}

	// Test with validation function that returns state without checksum (should be set automatically)
	noChecksumState := &ValidationState{
		ObjectID:   "TEST-004",
		ObjectKind: "test_object",
		Checksum:   "", // Empty checksum
	}
	noChecksumFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		return noChecksumState, nil
	})
	validator.SetValidationFunc(noChecksumFunc)

	state, err = validator.validateObjectWithData(ctx, "TEST-004", "test_object", testFile, testContent)
	if err != nil {
		t.Fatalf("validateObjectWithData() error = %v, want nil", err)
	}
	if state.Checksum == emptyValue {
		t.Error("validateObjectWithData() Checksum should be set automatically when empty")
	}
}

// TestAsyncValidator_Enqueue_ProgressChannelFull tests behavior when progress channel is full
func TestAsyncValidator_Enqueue_ProgressChannelFull(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Create a test file
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := "kind: test_object\nid: TEST-001"
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Fill up the progress channel (if we can access it for writing)
	// Note: GetProgress() returns read-only channel, so we can't fill it directly
	// This test verifies that Enqueue handles full channel gracefully
	// by using a non-blocking select with default case

	// First, validate to populate cache
	ctx := pkgctx.NewSystemContext()
	_, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf("ValidateNow failed: %v", err)
	}

	// Enqueue with cached state - progress update should be skipped if channel is full
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Should not block or error (uses cache, skips if channel full)
	time.Sleep(100 * time.Millisecond)
}

// TestAsyncValidator_ComputeChecksum tests the computeChecksum function
func TestAsyncValidator_ComputeChecksum(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Create a test file
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := "kind: test_object\nid: TEST-001"
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Compute checksum
	checksum1 := validator.computeChecksum(testFile)
	if checksum1 == emptyValue {
		t.Error("computeChecksum() returned empty string for valid file")
	}

	// Same file should produce same checksum
	checksum2 := validator.computeChecksum(testFile)
	if checksum1 != checksum2 {
		t.Errorf("computeChecksum() returned different checksums for same file: %q vs %q", checksum1, checksum2)
	}

	// Modified file should produce different checksum
	modifiedContent := "kind: test_object\nid: TEST-001\nmodified: true"
	if err := fileutil.WriteFile(testFile, []byte(modifiedContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write modified file: %v", err)
	}

	checksum3 := validator.computeChecksum(testFile)
	if checksum1 == checksum3 {
		t.Error("computeChecksum() returned same checksum for modified file")
	}

	// Non-existent file should return empty string
	nonExistentFile := filepath.Join(testRoot, "nonexistent.yaml")
	checksum4 := validator.computeChecksum(nonExistentFile)
	if checksum4 != emptyValue {
		t.Errorf("computeChecksum() returned %q for non-existent file, want empty string", checksum4)
	}
}

func TestAsyncValidator_ValidateNow(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)

	// Create a test file
	testFile := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), "TEST-001.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := fileutil.WriteFile(testFile, []byte("id: TEST-001\nkind: test_object\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Validate immediately (bypasses queue)
	ctx := pkgctx.NewSystemContext()
	state, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf("failed to validate: %v", err)
	}

	if state.ObjectID != "TEST-001" {
		t.Errorf("expected ObjectID TEST-001, got %s", state.ObjectID)
	}

	// Should be cached now
	cachedState, exists := validator.GetCachedState("TEST-001")
	if !exists {
		t.Error("expected state to be cached after ValidateNow")
	} else if cachedState.Checksum != state.Checksum {
		t.Errorf("expected checksum %s, got %s", state.Checksum, cachedState.Checksum)
	}
}

// TestAsyncValidator_NoProgressUpdatesDroppedUnderLoad verifies that progress updates are not dropped
// under concurrent load when the channel size is small but blocking timeout allows the reader to catch up.
func TestAsyncValidator_NoProgressUpdatesDroppedUnderLoad(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Use a small progress channel size (e.g. 50) to force pressure on the channel
	cfg := DefaultAsyncValidatorConfig()
	cfg.ProgressChannelSize = 50
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, 1*time.Hour, cfg)

	// Set validation function with minor delay
	validator.SetValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, data []byte) (*ValidationState, error) {
		time.Sleep(5 * time.Millisecond)
		return &ValidationState{
			ObjectID:   objectID,
			ObjectKind: objectKind,
			FilePath:   filePath,
		}, nil
	})

	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}

	// Create test files
	numTasks := 150
	var tasks []ValidationTask
	for i := 0; i < numTasks; i++ {
		testFile := filepath.Join(testRoot, fmt.Sprintf("test-%03d.yaml", i))
		if err := fileutil.WriteFile(testFile, []byte(fmt.Sprintf("id: TEST-%03d\nkind: test_kind\n", i)), paths.FilePerm644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		tasks = append(tasks, ValidationTask{
			ObjectID:   fmt.Sprintf("TEST-%03d", i),
			ObjectKind: "test_kind",
			FilePath:   testFile,
			Priority:   3,
		})
	}

	// Drain progress channel concurrently and count updates
	progressChan := validator.GetProgress()
	drainDone := make(chan int, 1)
	allReceived := make(chan struct{})
	go func() {
		count := 0
		for range progressChan {
			count++
			if count == numTasks {
				close(allReceived)
			}
		}
		drainDone <- count
	}()

	// Enqueue all tasks
	validator.EnqueueBatch(tasks)

	// Wait for all progress updates to be emitted and processed
	select {
	case <-allReceived:
		// Success
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for progress updates")
	}

	// Stop validator to close progress channel and join workers
	if err := validator.Stop(); err != nil {
		t.Fatalf("failed to stop validator: %v", err)
	}

	// Wait for drain goroutine to exit and get total count
	totalDrained := <-drainDone

	// Check that NO progress updates were dropped
	if totalDrained != numTasks {
		t.Errorf("Expected exactly %d progress updates, but got %d (some updates were dropped!)", numTasks, totalDrained)
	} else {
		t.Logf("Successfully received all %d progress updates under channel pressure without drops!", totalDrained)
	}
}
