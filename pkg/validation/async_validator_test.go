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
		t.Fatalf(ConstMagic4545ee2f, err)
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
	t.Logf(ConstMagic42db53de, total, stale, withIssues, queueSize)
}

func TestAsyncValidator_CacheIntegration(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)

	// Create a test file
	testFile := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), "TEST-001.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}
	if err := fileutil.WriteFile(testFile, []byte(ConstMagicbd310509), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Enqueue task
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for processing
	time.Sleep(1 * time.Second)

	// Check if cached
	state, exists := validator.GetCachedState("TEST-001")
	if !exists {
		t.Error(ConstMagic4c28fd90)
	} else {
		t.Logf(ConstMagic6b374120, state.ObjectID, state.LastValidated)
	}
}

func TestAsyncValidator_ProgressReporting(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}

	for i := 1; i <= 5; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf(ConstMagic28b7580d, i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
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
			t.Logf(ConstMagic10a7a32e, completed)
			return
		case progress, ok := <-progressChan:
			if !ok {
				t.Logf(ConstMagicfdc20764, completed)
				return
			}
			if progress.Status == "completed" {
				completed++
				t.Logf(ConstMagic64ea538e, completed, progress.CurrentObject)
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
	testContent := ConstMagic4eb7aec2
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	// First enqueue - should validate (not in cache)
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for validation to complete
	ctx := pkgctx.NewSystemContext()
	_, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf(ConstMagice5562562, err)
	}

	// Second enqueue with same file - should use cache (checksum matches)
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Verify it was cached (progress channel should receive completion)
	// Note: GetProgress() returns a read-only channel
	progressChan := validator.GetProgress()
	select {
	case progress := <-progressChan:
		if progress.Status != "completed" {
			t.Errorf(ConstMagice3280f3e, progress.Status)
		}
	case <-time.After(100 * time.Millisecond):
		// No progress update - might be fine if channel is full or validator not started
	}

	// Third enqueue with modified file - should re-validate (checksum changed)
	modifiedContent := ConstMagic7295f825
	if err := fileutil.WriteFile(testFile, []byte(modifiedContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic1106fdcf, err)
	}

	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Should enqueue for validation (not use cache)
	_, err = validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf(ConstMagice5562562, err)
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
		t.Errorf(ConstMagic8b9b4823, queueSize)
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
		t.Errorf(ConstMagic6f2b1fc6, queueSize)
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
		if err := fileutil.WriteFile(p, []byte("id: "+name+ConstMagic181341d3), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic26a0c8fd, err)
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
		ValidationTask{ObjectID: "OVERWRITE-X", ObjectKind: "test_object", FilePath: ConstMagic0e1afbc2, Priority: 1},
		ValidationTask{ObjectID: "OVERWRITE-Y", ObjectKind: "test_object", FilePath: ConstMagic1645dd68, Priority: 1},
		ValidationTask{ObjectID: "OVERWRITE-Z", ObjectKind: "test_object", FilePath: ConstMagic253e2e64, Priority: 1},
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
		t.Errorf(ConstMagicaaaa7a0e, len(got), got)
	} else {
		for _, id := range got {
			if !wantSet[id] {
				t.Errorf(ConstMagic92de352c, id)
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
		t.Error(ConstMagic544c8565)
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
		t.Error(ConstMagic09de13e2)
	}
	if state != nil {
		t.Error(ConstMagic49455ea1)
	}

	// Create a test file and validate to populate cache
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := ConstMagic4eb7aec2
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	ctx := pkgctx.NewSystemContext()
	validatedState, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf(ConstMagice5562562, err)
	}

	// Now should be cached
	state, exists = validator.GetCachedState("TEST-001")
	if !exists {
		t.Error(ConstMagice76a0376)
	}
	if state == nil {
		t.Error(ConstMagic5e8fe4d7)
		return
	}
	if state.ObjectID != "TEST-001" {
		t.Errorf(ConstMagic31500924, state.ObjectID)
	}
	if state.Checksum != validatedState.Checksum {
		t.Errorf(ConstMagic1c13f732, state.Checksum, validatedState.Checksum)
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
		t.Errorf(ConstMagicacfc65de, len(states))
	}

	// Create and validate multiple objects
	ctx := pkgctx.NewSystemContext()
	for i := 1; i <= 3; i++ {
		testFile := filepath.Join(testRoot, fmt.Sprintf("test%d.yaml", i))
		testContent := fmt.Sprintf(ConstMagic1c46875d, i)
		if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}

		_, err := validator.ValidateNow(ctx, fmt.Sprintf("TEST-%03d", i), "test_object", testFile)
		if err != nil {
			t.Fatalf(ConstMagice5562562, err)
		}
	}

	// Should have 3 cached states
	states = validator.GetAllCachedStates()
	if len(states) != 3 {
		t.Errorf(ConstMagic0f897e46, len(states))
	}

	// Verify all states are present
	stateMap := make(map[string]bool)
	for _, state := range states {
		stateMap[state.ObjectID] = true
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("TEST-%03d", i)
		if !stateMap[id] {
			t.Errorf(ConstMagic353d34c2, id)
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
		t.Errorf(ConstMagicd8ba9b06, err)
	}

	// Starting again should return error
	if err := validator.Start(); err == nil {
		t.Error(ConstMagic3fe522ed)
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
		t.Errorf(ConstMagica1b8a21d, err)
	}

	// Start and then stop
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagicbc7e1d13, err)
	}

	if err := validator.Stop(); err != nil {
		t.Errorf(ConstMagica1b8a21d, err)
	}

	// Stop again should succeed (idempotent)
	if err := validator.Stop(); err != nil {
		t.Errorf(ConstMagica1b8a21d, err)
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
		t.Fatalf(ConstMagicbc7e1d13, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create a test file and enqueue
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := ConstMagic4eb7aec2
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for validation to complete by checking progress channel
	progressChan := validator.GetProgress()
	timeout := time.After(2 * time.Second)
	select {
	case <-progressChan:
		// Progress update received
	case <-timeout:
		t.Log(ConstMagic14fba9ac)
	}

	// Give a small buffer for the function to be called
	time.Sleep(50 * time.Millisecond)

	// Custom function should have been called
	if !called {
		t.Error(ConstMagicdecbd3f3)
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
		t.Errorf(ConstMagicb58f5e21, total)
	}
	if stale != 0 {
		t.Errorf(ConstMagic1a723515, stale)
	}
	if withIssues != 0 {
		t.Errorf(ConstMagic6b6b126d, withIssues)
	}
	if queueSize != 0 {
		t.Errorf(ConstMagicc0f56e32, queueSize)
	}

	// Create and validate an object
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := ConstMagic4eb7aec2
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	ctx := pkgctx.NewSystemContext()
	_, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf(ConstMagice5562562, err)
	}

	// Stats should reflect the validated object
	total, _, _, _ = validator.GetValidationStats()
	if total < 1 {
		t.Errorf(ConstMagic832ae6d5, total)
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
		t.Error(ConstMagic726cd772)
	}

	// Same data should produce same checksum
	checksum2 := validator.computeChecksumFromData(data1)
	if checksum1 != checksum2 {
		t.Errorf(ConstMagicad575e81, checksum1, checksum2)
	}

	// Different data should produce different checksum
	data2 := []byte(ConstMagicfb9ca3de)
	checksum3 := validator.computeChecksumFromData(data2)
	if checksum1 == checksum3 {
		t.Error(ConstMagic2ba9874e)
	}

	// Empty data should still produce a checksum
	emptyData := []byte{}
	checksum4 := validator.computeChecksumFromData(emptyData)
	if checksum4 == emptyValue {
		t.Error(ConstMagicac58757c)
	}
}

// TestAsyncValidator_ValidateObject tests the validateObject function
func TestAsyncValidator_ValidateObject(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Create a test file
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := ConstMagic4eb7aec2
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	ctx := pkgctx.NewSystemContext()

	// Test with valid file
	state, err := validator.validateObject(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf(ConstMagic86e06573, err)
	}
	if state == nil {
		t.Fatal(ConstMagic40900e07)
	}
	if state.ObjectID != "TEST-001" {
		t.Errorf(ConstMagic815e1047, state.ObjectID)
	}
	if state.ObjectKind != "test_object" {
		t.Errorf(ConstMagic98c35df3, state.ObjectKind)
	}
	if state.FilePath != testFile {
		t.Errorf(ConstMagic3a9a14ab, state.FilePath, testFile)
	}
	if state.Checksum == emptyValue {
		t.Error(ConstMagic46451ff0)
	}

	// Test with non-existent file
	nonExistentFile := filepath.Join(testRoot, ConstMagic8293f515)
	_, err = validator.validateObject(ctx, "TEST-002", "test_object", nonExistentFile)
	if err == nil {
		t.Error(ConstMagic99467610)
	}
}

// TestAsyncValidator_ValidateObjectWithData tests the validateObjectWithData function
func TestAsyncValidator_ValidateObjectWithData(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := []byte(ConstMagic4eb7aec2)
	ctx := pkgctx.NewSystemContext()

	// Test without validation function (fallback path)
	state, err := validator.validateObjectWithData(ctx, "TEST-001", "test_object", testFile, testContent)
	if err != nil {
		t.Fatalf(ConstMagic677a7cc5, err)
	}
	if state == nil {
		t.Fatal(ConstMagic9c5fc305)
	}
	if state.ObjectID != "TEST-001" {
		t.Errorf(ConstMagicf82f4dae, state.ObjectID)
	}
	if state.Checksum == emptyValue {
		t.Error(ConstMagic86595b91)
	}
	if state.Issues == nil {
		t.Error(ConstMagicff29c94d)
	}
	if state.Metadata == nil {
		t.Error(ConstMagic3ae53cf3)
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
		t.Fatalf(ConstMagic677a7cc5, err)
	}
	if state.ObjectID != "TEST-002" {
		t.Errorf(ConstMagic817e0787, state.ObjectID)
	}
	if state.Checksum != "custom-checksum" {
		t.Errorf(ConstMagicdf7a1f5a, state.Checksum)
	}

	// Test with validation function that returns error
	errorFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		return nil, errors.New(ConstMagice313d4b5)
	})
	validator.SetValidationFunc(errorFunc)

	_, err = validator.validateObjectWithData(ctx, "TEST-003", "test_object", testFile, testContent)
	if err == nil {
		t.Error(ConstMagic0c916669)
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
		t.Fatalf(ConstMagic677a7cc5, err)
	}
	if state.Checksum == emptyValue {
		t.Error(ConstMagic754562c4)
	}
}

// TestAsyncValidator_Enqueue_ProgressChannelFull tests behavior when progress channel is full
func TestAsyncValidator_Enqueue_ProgressChannelFull(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, 1*time.Second)

	// Create a test file
	testFile := filepath.Join(testRoot, "test.yaml")
	testContent := ConstMagic4eb7aec2
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	// Fill up the progress channel (if we can access it for writing)
	// Note: GetProgress() returns read-only channel, so we can't fill it directly
	// This test verifies that Enqueue handles full channel gracefully
	// by using a non-blocking select with default case

	// First, validate to populate cache
	ctx := pkgctx.NewSystemContext()
	_, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf(ConstMagice5562562, err)
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
	testContent := ConstMagic4eb7aec2
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	// Compute checksum
	checksum1 := validator.computeChecksum(testFile)
	if checksum1 == emptyValue {
		t.Error(ConstMagic25fdbe14)
	}

	// Same file should produce same checksum
	checksum2 := validator.computeChecksum(testFile)
	if checksum1 != checksum2 {
		t.Errorf(ConstMagicb9be2d32, checksum1, checksum2)
	}

	// Modified file should produce different checksum
	modifiedContent := ConstMagic7295f825
	if err := fileutil.WriteFile(testFile, []byte(modifiedContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic1106fdcf, err)
	}

	checksum3 := validator.computeChecksum(testFile)
	if checksum1 == checksum3 {
		t.Error(ConstMagic21682132)
	}

	// Non-existent file should return empty string
	nonExistentFile := filepath.Join(testRoot, ConstMagic8293f515)
	checksum4 := validator.computeChecksum(nonExistentFile)
	if checksum4 != emptyValue {
		t.Errorf(ConstMagicaee1f5bb, checksum4)
	}
}

func TestAsyncValidator_ValidateNow(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)

	// Create a test file
	testFile := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), "TEST-001.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}
	if err := fileutil.WriteFile(testFile, []byte(ConstMagicbd310509), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	// Validate immediately (bypasses queue)
	ctx := pkgctx.NewSystemContext()
	state, err := validator.ValidateNow(ctx, "TEST-001", "test_object", testFile)
	if err != nil {
		t.Fatalf(ConstMagic9d5d9264, err)
	}

	if state.ObjectID != "TEST-001" {
		t.Errorf(ConstMagicb40fbdb4, state.ObjectID)
	}

	// Should be cached now
	cachedState, exists := validator.GetCachedState("TEST-001")
	if !exists {
		t.Error(ConstMagic6279beb6)
	} else if cachedState.Checksum != state.Checksum {
		t.Errorf(ConstMagicab634d6e, state.Checksum, cachedState.Checksum)
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
