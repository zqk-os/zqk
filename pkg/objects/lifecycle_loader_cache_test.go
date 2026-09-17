package objects

import (
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestLifecycleLoader_FileMtimeDetection tests that cache detects file changes via mtime
func TestLifecycleLoader_FileMtimeDetection(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, "lifecycles")
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create lifecycles dir: %v", err)
	}

	// Create initial lifecycle file
	lifecycleFile := filepath.Join(lifecyclesDir, "test_kind_lifecycle.yaml")
	initialContent := `object_type: test_kind
statuses:
  - value: initial
    display: Initial
    initial: true
transitions:
  - from: initial
    to: next
    description: Move to next
`
	if err := fileutil.WriteFile(lifecycleFile, []byte(initialContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write lifecycle file: %v", err)
	}

	loader := NewLifecycleLoader(lifecyclesDir)

	// Load lifecycle (should cache it)
	lifecycle1, err := loader.LoadLifecycle("test_kind")
	if err != nil {
		t.Fatalf("Failed to load lifecycle: %v", err)
	}
	if lifecycle1 == nil {
		t.Fatal("Lifecycle is nil")
	}

	// Load again (should use cache)
	lifecycle2, err := loader.LoadLifecycle("test_kind")
	if err != nil {
		t.Fatalf("Failed to load lifecycle: %v", err)
	}
	if lifecycle1 != lifecycle2 {
		t.Error("Second load should return cached lifecycle (same pointer)")
	}

	// Wait a bit to ensure mtime changes
	time.Sleep(10 * time.Millisecond)

	// Modify file (simulate another process changing it)
	updatedContent := `object_type: test_kind
statuses:
  - value: initial
    display: Initial
    initial: true
  - value: next
    display: Next
    initial: false
transitions:
  - from: initial
    to: next
    description: Move to next
`
	if err := fileutil.WriteFile(lifecycleFile, []byte(updatedContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to update lifecycle file: %v", err)
	}

	// Load again (should detect change and reload)
	lifecycle3, err := loader.LoadLifecycle("test_kind")
	if err != nil {
		t.Fatalf("Failed to load lifecycle: %v", err)
	}
	if lifecycle3 == nil {
		t.Fatal("Lifecycle is nil")
	}

	// Verify new status is present (proves it was reloaded)
	foundNext := false
	for _, status := range lifecycle3.Statuses {
		if status.Value == "next" {
			foundNext = true
			break
		}
	}
	if !foundNext {
		t.Error("Updated lifecycle should have 'next' status (proves cache was invalidated)")
	}
}

// TestLifecycleLoader_InvalidateLifecycle tests per-entry invalidation
func TestLifecycleLoader_InvalidateLifecycle(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, "lifecycles")
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create lifecycles dir: %v", err)
	}

	// Create two lifecycle files
	lifecycle1File := filepath.Join(lifecyclesDir, "kind1_lifecycle.yaml")
	lifecycle2File := filepath.Join(lifecyclesDir, "kind2_lifecycle.yaml")

	lifecycle1Content := `object_type: kind1
statuses:
  - value: status1
    display: Status 1
    initial: true
`
	lifecycle2Content := `object_type: kind2
statuses:
  - value: status2
    display: Status 2
    initial: true
`

	if err := fileutil.WriteFile(lifecycle1File, []byte(lifecycle1Content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write lifecycle1: %v", err)
	}
	if err := fileutil.WriteFile(lifecycle2File, []byte(lifecycle2Content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write lifecycle2: %v", err)
	}

	loader := NewLifecycleLoader(lifecyclesDir)

	// Load both lifecycles
	_, err := loader.LoadLifecycle("kind1")
	if err != nil {
		t.Fatalf("Failed to load lifecycle1: %v", err)
	}
	_, err = loader.LoadLifecycle("kind2")
	if err != nil {
		t.Fatalf("Failed to load lifecycle2: %v", err)
	}

	// Invalidate only kind1
	loader.InvalidateLifecycle("kind1")

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Modify lifecycle1
	updatedLifecycle1Content := `object_type: kind1
statuses:
  - value: status1
    display: Status 1
    initial: true
  - value: status3
    display: Status 3
    initial: false
`
	if err := fileutil.WriteFile(lifecycle1File, []byte(updatedLifecycle1Content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to update lifecycle1: %v", err)
	}

	// Load lifecycle1 (should reload)
	lifecycle1, err := loader.LoadLifecycle("kind1")
	if err != nil {
		t.Fatalf("Failed to load lifecycle1: %v", err)
	}
	foundStatus3 := false
	for _, status := range lifecycle1.Statuses {
		if status.Value == "status3" {
			foundStatus3 = true
			break
		}
	}
	if !foundStatus3 {
		t.Error("Lifecycle1 should have status3 after invalidation and reload")
	}

	// Load lifecycle2 (should still be cached)
	lifecycle2a, err := loader.LoadLifecycle("kind2")
	if err != nil {
		t.Fatalf("Failed to load lifecycle2: %v", err)
	}
	lifecycle2b, err := loader.LoadLifecycle("kind2")
	if err != nil {
		t.Fatalf("Failed to load lifecycle2: %v", err)
	}
	if lifecycle2a != lifecycle2b {
		t.Error("Lifecycle2 should still be cached (same pointer)")
	}
}

// TestLifecycleLoader_ConcurrentAccess tests concurrent access patterns
func TestLifecycleLoader_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, "lifecycles")
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create lifecycles dir: %v", err)
	}

	// Create multiple lifecycle files
	for i := 0; i < 10; i++ {
		lifecycleFile := filepath.Join(lifecyclesDir, "kind"+string(rune('0'+i))+"_lifecycle.yaml")
		content := `object_type: kind` + string(rune('0'+i)) + `
statuses:
  - value: status1
    display: Status 1
    initial: true
`
		if err := fileutil.WriteFile(lifecycleFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("Failed to write lifecycle file: %v", err)
		}
	}

	loader := NewLifecycleLoader(lifecyclesDir)

	// Concurrent reads
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("objects_test", "concurrent lifecycle load").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					kind := "kind" + strconv.Itoa(idx)
					_, err := loader.LoadLifecycle(kind)
					if err != nil {
						errors <- err
					}
				}
			}(i)
		})
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

// TestLifecycleLoader_DoubleCheckedLocking tests double-checked locking behavior
func TestLifecycleLoader_DoubleCheckedLocking(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, "lifecycles")
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create lifecycles dir: %v", err)
	}

	lifecycleFile := filepath.Join(lifecyclesDir, "test_kind_lifecycle.yaml")
	content := `object_type: test_kind
statuses:
  - value: initial
    display: Initial
    initial: true
`
	if err := fileutil.WriteFile(lifecycleFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write lifecycle file: %v", err)
	}

	loader := NewLifecycleLoader(lifecyclesDir)

	// Multiple goroutines trying to load the same lifecycle simultaneously
	// Should result in only one actual load, others use cached version
	var wg sync.WaitGroup
	results := make(chan *Lifecycle, 10)

	for i := 0; i < 10; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_lifecycle_loader_%d", i), fmt.Sprintf("loading lifecycle %d", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				lifecycle, err := loader.LoadLifecycle("test_kind")
				if err != nil {
					t.Errorf("Failed to load lifecycle: %v", err)
					return
				}
				results <- lifecycle
			})
	}

	wg.Wait()
	close(results)

	// All results should be the same pointer (proves double-checked locking worked)
	var firstLifecycle *Lifecycle
	count := 0
	for lifecycle := range results {
		count++
		if firstLifecycle == nil {
			firstLifecycle = lifecycle
		} else if lifecycle != firstLifecycle {
			t.Errorf("Expected all lifecycles to be same pointer (double-checked locking), got different pointer at iteration %d", count)
		}
	}

	if count != 10 {
		t.Errorf("Expected 10 results, got %d", count)
	}
}
