package asynccheck

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestCalculateMaxConcurrentWorkers(t *testing.T) {
	if workers := CalculateMaxConcurrentWorkers(0); workers != 1 {
		t.Errorf("expected 1 worker for 0 kinds, got %d", workers)
	}
	if workers := CalculateMaxConcurrentWorkers(2); workers < 1 || workers > 2 {
		t.Errorf("expected between 1 and 2 workers, got %d", workers)
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"", false},
		{"0123456789abcdef", true},
		{"ABCDEF", true},
		{"0123456789abcdefABCDEF", true},
		{"xyz123", false},
		{"0123456789abcdefg", false},
	}
	for _, tc := range tests {
		if got := IsHexString(tc.input); got != tc.expected {
			t.Errorf("IsHexString(%q) = %v, expected %v", tc.input, got, tc.expected)
		}
	}
}

func TestExtractObjectIDFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("system")

	// 1. Non-CAS standard file name (e.g. BLI-123.yaml)
	standardFile := filepath.Join(tmpDir, "BLI-123.yaml")
	if err := fileutil.WriteFile(standardFile, []byte("kind: backlog_item\nid: BLI-123\n"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	id := ExtractObjectIDFromFileWithContext(context.Background(), standardFile, "backlog_item", logger)
	if id != "BLI-123" {
		t.Errorf("expected BLI-123, got %q", id)
	}

	// 2. CAS file (64 hex characters + .yaml)
	casHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	casFile := filepath.Join(tmpDir, casHash+".yaml")
	if err := fileutil.WriteFile(casFile, []byte("kind: criteria\nid: CRIT-999\n"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write cas file: %v", err)
	}
	casID := ExtractObjectIDFromFileWithContext(context.Background(), casFile, "criteria", logger)
	if casID != "CRIT-999" {
		t.Errorf("expected CRIT-999, got %q", casID)
	}
}

func TestFilterFilesByIDs(t *testing.T) {
	files := []ScannedFile{
		{ObjectID: "OBJ-1", Kind: "goal", Path: "/a/1"},
		{ObjectID: "OBJ-2", Kind: "goal", Path: "/a/2"},
		{ObjectID: "OBJ-3", Kind: "goal", Path: "/a/3"},
	}
	// Empty targets returns all
	if filtered := FilterFilesByIDs(files, nil); len(filtered) != 3 {
		t.Errorf("expected 3 files, got %d", len(filtered))
	}
	// Targeted filter
	filtered := FilterFilesByIDs(files, []string{"OBJ-1", "OBJ-3"})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 files, got %d", len(filtered))
	}
	if filtered[0].ObjectID != "OBJ-1" || filtered[1].ObjectID != "OBJ-3" {
		t.Errorf("unexpected filtered results: %v", filtered)
	}
}

func TestDeduplicateFilesByObjectID(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	files := []ScannedFile{
		{ObjectID: "OBJ-1", Kind: "goal", Path: "/a/1"},
		{ObjectID: "OBJ-1", Kind: "goal", Path: "/a/1-duplicate"},
		{ObjectID: "OBJ-2", Kind: "goal", Path: "/a/2"},
	}
	deduped := DeduplicateFilesByObjectID(files, logger)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 files, got %d", len(deduped))
	}
	if deduped[0].ObjectID != "OBJ-1" || deduped[1].ObjectID != "OBJ-2" {
		t.Errorf("unexpected deduped results: %v", deduped)
	}
}

func TestCollectDiscoveryResults(t *testing.T) {
	ch := make(chan []ScannedFile, 2)
	ch <- []ScannedFile{{ObjectID: "A"}, {ObjectID: "B"}}
	ch <- []ScannedFile{{ObjectID: "C"}}
	close(ch)

	results := CollectDiscoveryResults(ch)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestIsTransientCacheCoherenceIssue(t *testing.T) {
	if !IsTransientCacheCoherenceIssue("CacheLag") {
		t.Errorf("expected CacheLag to be transient")
	}
	if !IsTransientCacheCoherenceIssue("cache_coherence") {
		t.Errorf("expected cache_coherence to be transient")
	}
	if IsTransientCacheCoherenceIssue("integrity") {
		t.Errorf("integrity should not be transient cache coherence")
	}
}

func TestShouldUseCachedState(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "GOAL-1.yaml")
	if err := fileutil.WriteFile(filePath, []byte("id: GOAL-1\n"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	state := &validation.ValidationState{
		ObjectID:      "GOAL-1",
		ObjectKind:    "goal",
		FilePath:      filePath,
		LastValidated: time.Now().Add(1 * time.Minute),
		Issues:        nil,
	}

	if !ShouldUseCachedState("GOAL-1", filePath, state, false) {
		t.Errorf("expected clean state to be usable")
	}

	// Integrity issue invalidates
	stateWithIntegrity := &validation.ValidationState{
		ObjectID:      "GOAL-1",
		ObjectKind:    "goal",
		FilePath:      filePath,
		LastValidated: time.Now().Add(1 * time.Minute),
		Issues:        []validation.ValidationIssue{{Category: "integrity"}},
	}
	if ShouldUseCachedState("GOAL-1", filePath, stateWithIntegrity, false) {
		t.Errorf("expected integrity issue to invalidate cache hit")
	}
}

func TestScanObjectFilesWithContext_Walk(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("system")

	// Create test object files
	f1 := filepath.Join(tmpDir, "GOAL-1.yaml")
	f2 := filepath.Join(tmpDir, "GOAL-2.yml")
	nonYaml := filepath.Join(tmpDir, "ignored.txt")

	if err := fileutil.WriteFile(f1, []byte("id: GOAL-1\nkind: goal\n"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := fileutil.WriteFile(f2, []byte("id: GOAL-2\nkind: goal\n"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := fileutil.WriteFile(nonYaml, []byte("not yaml"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write non-yaml file: %v", err)
	}

	files, err := ScanObjectFilesWithContext(context.Background(), tmpDir, "goal", logger, nil)
	if err != nil {
		t.Fatalf("ScanObjectFilesWithContext failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	idMap := make(map[string]bool)
	for _, f := range files {
		idMap[f.ObjectID] = true
	}
	if !idMap["GOAL-1"] || !idMap["GOAL-2"] {
		t.Errorf("expected GOAL-1 and GOAL-2, got %v", idMap)
	}

	// Test legacy wrapper ScanObjectFiles
	legacyFiles, err := ScanObjectFiles(tmpDir, "goal")
	if err != nil {
		t.Fatalf("ScanObjectFiles failed: %v", err)
	}
	if len(legacyFiles) != 2 {
		t.Fatalf("expected 2 files from ScanObjectFiles, got %d", len(legacyFiles))
	}
}

func TestScanObjectFilesWithContext_CancelledContext(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("system")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ScanObjectFilesWithContext(ctx, tmpDir, "goal", logger, nil)
	if err == nil {
		t.Fatalf("expected error from cancelled context, got nil")
	}
}
