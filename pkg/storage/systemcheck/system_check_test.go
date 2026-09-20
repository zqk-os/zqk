package systemcheck

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRunSystemCheckForHandCASAndDupIDs(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory for testing
	tempDir, err := fileutil.MkdirTemp("", "zqk-system-check-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	// Create dummy YAML files for testing
	// 1. Valid file
	validFile := filepath.Join(tempDir, "0000000000000000000000000000000000000000000000000000000000000000.yaml")
	validYaml := "id: VAL-001\nkind: test_kind\n"
	err = fileutil.WriteFile(validFile, []byte(validYaml), paths.FilePerm644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// 2. Duplicate ID file
	dupFile := filepath.Join(tempDir, "1111111111111111111111111111111111111111111111111111111111111111.yaml")
	dupYaml := "id: VAL-001\nkind: test_kind\n"
	err = fileutil.WriteFile(dupFile, []byte(dupYaml), paths.FilePerm644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// 3. Orphan path file (missing ID)
	orphanFile := filepath.Join(tempDir, "2222222222222222222222222222222222222222222222222222222222222222.yaml")
	orphanYaml := "kind: test_kind\n"
	err = fileutil.WriteFile(orphanFile, []byte(orphanYaml), paths.FilePerm644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	result, err := RunSystemCheckForHandCASAndDupIDs(ctx, tempDir, "test_kind")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if result.Violations != 2 {
		t.Errorf("expected 2 violations, got %d", result.Violations)
	}
	if len(result.Duplicates) != 1 {
		t.Errorf("expected 1 duplicate, got %d", len(result.Duplicates))
	}
	if len(result.OrphanPaths) != 1 {
		t.Errorf("expected 1 orphan path, got %d", len(result.OrphanPaths))
	}
}

func TestHandCASSystemCheckError(t *testing.T) {
	err := &HandCASSystemCheckError{
		Kind: "test_kind",
		Duplicates: []DuplicateIDEntry{
			{ObjectID: "ID-123"},
		},
		OrphanPaths: []string{"path1", "path2"},
	}

	expected := "hand-CAS check found violations in kind \"test_kind\": 1 duplicate IDs, 2 orphan paths"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestBatchRunSystemCheckForHandCASAndDupIDs(t *testing.T) {
	ctx := context.Background()
	kinds := []string{"kind1", "kind2"}

	tempDir, err := fileutil.MkdirTemp("", "zqk-system-check-batch-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	resolver := func(kind string) string {
		return tempDir
	}

	resultsCh, errorsCh := BatchRunSystemCheckForHandCASAndDupIDs(ctx, kinds, resolver)

	var results []*HandCASSystemCheckResult
	var errors []error

	for res := range resultsCh {
		results = append(results, res)
	}
	for err := range errorsCh {
		errors = append(errors, err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if len(errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errors))
	}
}
