package testrunner_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testrunner"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestContaminationDiff_AllChanged(t *testing.T) {
	t.Parallel()

	diff := testrunner.ContaminationDiff{
		Added:    []string{"b.yaml", "a.yaml"},
		Modified: []string{"mod.yaml"},
		Deleted:  []string{"del.yaml"},
	}

	if !diff.HasContamination() {
		t.Fatalf("expected HasContamination to be true")
	}

	changed := diff.AllChanged()
	if len(changed) != 4 {
		t.Fatalf("expected 4 changed items, got %d", len(changed))
	}

	// Verify items are sorted
	expectedPrefixes := []string{"[added]", "[added]", "[deleted]", "[modified]"}
	for i, prefix := range expectedPrefixes {
		if !strings.HasPrefix(changed[i], prefix) {
			t.Errorf("expected item %d to have prefix %s, got %s", i, prefix, changed[i])
		}
	}
}

func TestSnapshotPlane_EdgeCases(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// 1. Non-existent directory
	_, err := testrunner.SnapshotPlane(tempDir, "non_existent_dir")
	if err == nil {
		t.Errorf("expected error for non-existent directory")
	}

	// 2. Path is a file, not a directory
	filePath := filepath.Join(tempDir, "regular_file.txt")
	if err := fileutil.WriteFile(filePath, []byte("hello"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	_, err = testrunner.SnapshotPlane(tempDir, "regular_file.txt")
	if err == nil {
		t.Errorf("expected error when plane path is not a directory")
	}

	// 3. Directory with non-YAML/JSON files and JSON files
	dirPath := filepath.Join(tempDir, "mixed_dir")
	if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(dirPath, "ignore.txt"), []byte("txt"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write ignore file: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(dirPath, "spec.json"), []byte(`{"a": 1}`), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write json spec: %v", err)
	}

	snap, err := testrunner.SnapshotPlane(tempDir, "mixed_dir")
	if err != nil {
		t.Fatalf("unexpected snapshot error: %v", err)
	}
	if len(snap) != 1 || snap["spec.json"] == "" {
		t.Errorf("expected only spec.json in snapshot, got %v", snap)
	}
}

func TestRunWithContaminationCheck_EmptyCommand(t *testing.T) {
	t.Parallel()

	opts := testrunner.ContaminationCheckOptions{
		ProjectRoot: t.TempDir(),
		PlanePath:   ".",
	}

	exitCode, diff, err := testrunner.RunWithContaminationCheck(context.Background(), opts, nil, nil, nil)
	if err == nil || exitCode != 2 {
		t.Errorf("expected error and exit code 2, got code %d, err %v", exitCode, err)
	}
	if diff.HasContamination() {
		t.Errorf("expected no contamination")
	}
}

func TestResolveInvocations_CommandBuildingVariations(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// 1. Nil testCase
	_, err := testrunner.ResolveInvocations(nil, tempDir)
	if err == nil {
		t.Errorf("expected error for nil testCase")
	}

	// 2. Executable script (.sh)
	shScript := filepath.Join(tempDir, "run_test.sh")
	if err := fileutil.WriteFile(shScript, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	tcScript := map[string]any{
		"criteria_refs": []string{"CRIT-SH-1"},
		"path_or_id":    shScript,
	}
	invs, err := testrunner.ResolveInvocations(tcScript, tempDir)
	if err != nil {
		t.Fatalf("ResolveInvocations failed: %v", err)
	}
	if len(invs) != 1 || !strings.Contains(invs[0].Command, "bash ") {
		t.Errorf("expected bash invocation for .sh script, got %v", invs)
	}

	// 3. Executable non-sh file
	binScript := filepath.Join(tempDir, "binary_runner")
	if err := fileutil.WriteFile(binScript, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	tcBin := map[string]any{
		"criteria_refs": []string{"CRIT-BIN-1"},
		"path_or_id":    binScript,
	}
	invsBin, err := testrunner.ResolveInvocations(tcBin, tempDir)
	if err != nil {
		t.Fatalf("ResolveInvocations failed: %v", err)
	}
	if len(invsBin) != 1 || strings.Contains(invsBin[0].Command, "bash ") {
		t.Errorf("expected direct invocation for non-sh executable, got %v", invsBin)
	}

	// 4. Go test with filter colon syntax "file_test.go:TestSpecific"
	goTestWithFilter := map[string]any{
		"criteria_refs": []string{"CRIT-FILTER-1"},
		"path_or_id":    "pkg/testrunner/test_runner_test.go:TestSpecific",
	}
	invsFilter, err := testrunner.ResolveInvocations(goTestWithFilter, tempDir)
	if err != nil {
		t.Fatalf("ResolveInvocations failed: %v", err)
	}
	if len(invsFilter) != 1 || !strings.Contains(invsFilter[0].Command, "-run '^TestSpecific$'") {
		t.Errorf("expected -run TestSpecific, got %s", invsFilter[0].Command)
	}

	// 5. Plain Go package path
	goPkg := map[string]any{
		"criteria_refs": []string{"CRIT-PKG-1"},
		"path_or_id":    "pkg/testrunner/...",
	}
	invsPkg, err := testrunner.ResolveInvocations(goPkg, tempDir)
	if err != nil {
		t.Fatalf("ResolveInvocations failed: %v", err)
	}
	if len(invsPkg) != 1 || !strings.Contains(invsPkg[0].Command, "go test -v ./pkg/testrunner/...") {
		t.Errorf("expected go test -v ./pkg/testrunner/..., got %s", invsPkg[0].Command)
	}

	// 6. Default echo verification command when path_or_id is empty
	emptyPathTC := map[string]any{
		"criteria_refs": []string{"CRIT-EMPTY-1"},
	}
	invsEmpty, err := testrunner.ResolveInvocations(emptyPathTC, tempDir)
	if err != nil {
		t.Fatalf("ResolveInvocations failed: %v", err)
	}
	if len(invsEmpty) != 1 || !strings.Contains(invsEmpty[0].Command, "echo 'Verifying CRIT-EMPTY-1'") {
		t.Errorf("expected echo 'Verifying CRIT-EMPTY-1', got %s", invsEmpty[0].Command)
	}

	// 7. Unprefixed verification suite command
	unprefixedSuiteTC := map[string]any{
		"criteria_refs":       []string{"CRIT-UNPRE-1"},
		"verification_suites": []string{"go test -v ./pkg/foo"},
	}
	invsUnprefixed, err := testrunner.ResolveInvocations(unprefixedSuiteTC, tempDir)
	if err != nil {
		t.Fatalf("ResolveInvocations failed: %v", err)
	}
	if len(invsUnprefixed) != 1 || invsUnprefixed[0].Command != "go test -v ./pkg/foo" {
		t.Errorf("expected unprefixed suite to map to first criterion, got %v", invsUnprefixed)
	}
}

func TestRunTestCase_ValidationErrors(t *testing.T) {
	t.Parallel()

	// 1. Nil storage provider
	_, err := testrunner.RunTestCase(context.Background(), nil, "", "TC-1", testrunner.RunOptions{})
	if err == nil {
		t.Errorf("expected error for nil storage provider")
	}

	// 2. Empty testCaseID
	// We need a dummy storage provider or non-nil
	_, err = testrunner.RunTestCase(context.Background(), nil, "", "", testrunner.RunOptions{})
	if err == nil {
		t.Errorf("expected error for empty test_case id")
	}
}
