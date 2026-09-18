package objects

import (
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestSpecLoader_FileMtimeDetection tests that cache detects file changes via mtime
func TestSpecLoader_FileMtimeDetection(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	// Create initial spec file
	specFile := filepath.Join(specsDir, "test_spec.yaml")
	initialContent := `ontology: test_spec
schema_version: "1.0"
fields:
  field1:
    type: string
`
	if err := fileutil.WriteFile(specFile, []byte(initialContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	loader := NewSpecLoader(specsDir)

	// Load spec (should cache it)
	spec1, err := loader.LoadSpecWithInheritance("test_spec.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}
	if spec1 == nil {
		t.Fatal("Spec is nil")
	}

	// Load again (should use cache)
	spec2, err := loader.LoadSpecWithInheritance("test_spec.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}
	if spec1 != spec2 {
		t.Error("Second load should return cached spec (same pointer)")
	}

	// Wait a bit to ensure mtime changes
	time.Sleep(10 * time.Millisecond)

	// Modify file (simulate another process changing it)
	updatedContent := `ontology: test_spec
schema_version: "1.0"
fields:
  field1:
    type: string
  field2:
    type: integer
`
	if err := fileutil.WriteFile(specFile, []byte(updatedContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to update spec file: %v", err)
	}

	// Load again (should detect change and reload)
	spec3, err := loader.LoadSpecWithInheritance("test_spec.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}
	if spec3 == nil {
		t.Fatal("Spec is nil")
	}

	// Verify new field is present (proves it was reloaded)
	if _, ok := spec3.ResolvedFields["field2"]; !ok {
		t.Error("Updated spec should have field2 (proves cache was invalidated)")
	}
}

// TestSpecLoader_InvalidateSpec tests per-entry invalidation
func TestSpecLoader_InvalidateSpec(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	// Create two spec files
	spec1File := filepath.Join(specsDir, "spec1.yaml")
	spec2File := filepath.Join(specsDir, "spec2.yaml")

	spec1Content := `ontology: spec1
schema_version: "1.0"
fields:
  field1:
    type: string
`
	spec2Content := `ontology: spec2
schema_version: "1.0"
fields:
  field2:
    type: string
`

	if err := fileutil.WriteFile(spec1File, []byte(spec1Content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec1: %v", err)
	}
	if err := fileutil.WriteFile(spec2File, []byte(spec2Content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec2: %v", err)
	}

	loader := NewSpecLoader(specsDir)

	// Load both specs
	_, err := loader.LoadSpecWithInheritance("spec1.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec1: %v", err)
	}
	_, err = loader.LoadSpecWithInheritance("spec2.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec2: %v", err)
	}

	// Invalidate only spec1
	loader.InvalidateSpec("spec1")

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Modify spec1
	updatedSpec1Content := `ontology: spec1
schema_version: "1.0"
fields:
  field1:
    type: string
  field3:
    type: boolean
`
	if err := fileutil.WriteFile(spec1File, []byte(updatedSpec1Content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to update spec1: %v", err)
	}

	// Load spec1 (should reload)
	spec1, err := loader.LoadSpecWithInheritance("spec1.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec1: %v", err)
	}
	if _, ok := spec1.ResolvedFields["field3"]; !ok {
		t.Error("Spec1 should have field3 after invalidation and reload")
	}

	// Load spec2 (should still be cached)
	spec2a, err := loader.LoadSpecWithInheritance("spec2.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec2: %v", err)
	}
	spec2b, err := loader.LoadSpecWithInheritance("spec2.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec2: %v", err)
	}
	if spec2a != spec2b {
		t.Error("Spec2 should still be cached (same pointer)")
	}
}

// TestSpecLoader_InvalidateSpecByFile tests invalidation by file path
func TestSpecLoader_InvalidateSpecByFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	specFile := filepath.Join(specsDir, "test_spec.yaml")
	content := `ontology: test_spec
schema_version: "1.0"
fields:
  field1:
    type: string
`
	if err := fileutil.WriteFile(specFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	loader := NewSpecLoader(specsDir)

	// Load spec
	_, err := loader.LoadSpecWithInheritance("test_spec.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}

	// Invalidate by file path
	loader.InvalidateSpecByFile(specFile)

	// Wait and modify file
	time.Sleep(10 * time.Millisecond)
	updatedContent := `ontology: test_spec
schema_version: "1.0"
fields:
  field1:
    type: string
  field2:
    type: integer
`
	if err := fileutil.WriteFile(specFile, []byte(updatedContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to update spec file: %v", err)
	}

	// Load again (should reload)
	spec, err := loader.LoadSpecWithInheritance("test_spec.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}
	if _, ok := spec.ResolvedFields["field2"]; !ok {
		t.Error("Spec should have field2 after invalidation and reload")
	}
}

func TestSpecLoader_SpecCacheRevision(t *testing.T) {
	t.Parallel()
	var nilLoader *SpecLoader
	if nilLoader.SpecCacheRevision() != 0 {
		t.Fatalf("nil *SpecLoader SpecCacheRevision want 0")
	}

	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	loader := NewSpecLoader(specsDir)
	if g := loader.SpecCacheRevision(); g != 0 {
		t.Fatalf("initial revision want 0, got %d", g)
	}
	loader.ClearCache()
	if g := loader.SpecCacheRevision(); g != 1 {
		t.Fatalf("after ClearCache want 1, got %d", g)
	}
	loader.InvalidateSpec("nosuch")
	if g := loader.SpecCacheRevision(); g != 2 {
		t.Fatalf("after InvalidateSpec want 2, got %d", g)
	}
	specFile := filepath.Join(specsDir, "rev.yaml")
	if err := fileutil.WriteFile(specFile, []byte("ontology: rev\nschema_version: \"1.0\"\nfields: {}\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	loader.InvalidateSpecByFile(specFile)
	if g := loader.SpecCacheRevision(); g != 3 {
		t.Fatalf("after InvalidateSpecByFile want 3, got %d", g)
	}
}

// TestSpecLoader_ConcurrentAccess tests concurrent access patterns
func TestSpecLoader_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	// Create multiple spec files
	for i := 0; i < 10; i++ {
		specFile := filepath.Join(specsDir, "spec"+string(rune('0'+i))+".yaml")
		content := `ontology: spec` + string(rune('0'+i)) + `
schema_version: "1.0"
fields:
  field1:
    type: string
`
		if err := fileutil.WriteFile(specFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("Failed to write spec file: %v", err)
		}
	}

	loader := NewSpecLoader(specsDir)

	// Concurrent reads
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("objects_test", "concurrent spec load").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					specFile := "spec" + strconv.Itoa(idx) + ".yaml"
					_, err := loader.LoadSpecWithInheritance(specFile)
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

// TestSpecLoader_DoubleCheckedLocking tests double-checked locking behavior
func TestSpecLoader_DoubleCheckedLocking(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	specFile := filepath.Join(specsDir, "test_spec.yaml")
	content := `ontology: test_spec
schema_version: "1.0"
fields:
  field1:
    type: string
`
	if err := fileutil.WriteFile(specFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	loader := NewSpecLoader(specsDir)

	// Multiple goroutines trying to load the same spec simultaneously
	// Should result in only one actual load, others use cached version
	var wg sync.WaitGroup
	results := make(chan *Spec, 10)

	for i := 0; i < 10; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_spec_loader_%d", i), fmt.Sprintf("loading spec %d", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				spec, err := loader.LoadSpecWithInheritance("test_spec.yaml")
				if err != nil {
					t.Errorf("Failed to load spec: %v", err)
					return
				}
				results <- spec
			})
	}

	wg.Wait()
	close(results)

	// All results should be the same pointer (proves double-checked locking worked)
	var firstSpec *Spec
	count := 0
	for spec := range results {
		count++
		if firstSpec == nil {
			firstSpec = spec
		} else if spec != firstSpec {
			t.Errorf("Expected all specs to be same pointer (double-checked locking), got different pointer at iteration %d", count)
		}
	}

	if count != 10 {
		t.Errorf("Expected 10 results, got %d", count)
	}
}
