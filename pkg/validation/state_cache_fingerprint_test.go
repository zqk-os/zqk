package validation

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func seedValidationCacheWithState(t *testing.T, root string) *ValidationStateCache {
	t.Helper()
	cache := NewValidationStateCache(root, time.Hour)
	cache.Set(&ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	})
	if err := cache.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return cache
}

func writeProjectFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := fileutil.MkdirAll(filepath.Dir(full), paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll %s: %v", rel, err)
	}
	if err := fileutil.WriteFile(full, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile %s: %v", rel, err)
	}
}

func assertCacheHoldsTest001(t *testing.T, root string) {
	t.Helper()
	cache := NewValidationStateCache(root, time.Hour)
	if err := cache.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cache.Get("TEST-001"); !ok {
		t.Fatal("expected TEST-001 to remain cached")
	}
}

func assertCacheDroppedTest001(t *testing.T, root string) {
	t.Helper()
	cache := NewValidationStateCache(root, time.Hour)
	if err := cache.Load(); err != nil {
		t.Fatalf("Load after fingerprint change: %v", err)
	}
	if _, ok := cache.Get("TEST-001"); ok {
		t.Fatal("expected cache drop after checker fingerprint change; --clear-cache must not be required")
	}
	if _, err := fileutil.Stat(cache.cacheFile); err == nil {
		t.Fatal("expected stale validation_cache.json removed after fingerprint mismatch")
	}
}

func TestRunningExecutableFingerprint_HasPathSizeMtime(t *testing.T) {
	t.Parallel()
	got := runningExecutableFingerprint()
	if got == emptyValue {
		t.Fatal("expected running executable identity")
	}
	if strings.Count(got, ":") < 2 {
		t.Fatalf("expected path:size:mtime, got %q", got)
	}
}

func TestValidationFingerprintGlobs_IncludeUnlistedCheckerAndParser(t *testing.T) {
	t.Parallel()
	root := registerZQKTestRootForTest(t)
	writeProjectFile(t, root, "cmd/zqk/system/check_references_helpers.go", "package system\n")
	writeProjectFile(t, root, "pkg/migration/parser/parser.go", "package parser\n")
	writeProjectFile(t, root, ".zqk/specs/lifecycles/policy_lifecycle.yaml", "id: policy\n")
	rels := validationFingerprintRelPaths(root)
	want := []string{
		"cmd/zqk/system/check_references_helpers.go",
		".zqk/specs/lifecycles/policy_lifecycle.yaml",
		"pkg/migration/parser/parser.go",
	}
	for _, w := range want {
		if !slices.Contains(rels, w) {
			t.Fatalf("glob fingerprint missing %s (allowlist-only checksum would stay stale)", w)
		}
	}
}

// TestValidationStateCache_UnlistedCheckerFileInvalidates is the Layer 1
// incident: check_references_helpers.go was not on ValidationCodeChecksumFiles,
// so rebuilding checker logic left stale hits until --clear-cache.
func TestValidationStateCache_UnlistedCheckerFileInvalidates(t *testing.T) {
	t.Parallel()
	root := registerZQKTestRootForTest(t)
	rel := "cmd/zqk/system/check_references_helpers.go"
	writeProjectFile(t, root, rel, "package system\n// original\n")
	seedValidationCacheWithState(t, root)
	assertCacheHoldsTest001(t, root)

	writeProjectFile(t, root, rel, "package system\n// skip branch_ref\n")
	assertCacheDroppedTest001(t, root)
}

func TestValidationStateCache_PolicyLifecycleYAMLInvalidates(t *testing.T) {
	t.Parallel()
	root := registerZQKTestRootForTest(t)
	rel := ".zqk/specs/lifecycles/policy_lifecycle.yaml"
	writeProjectFile(t, root, rel, "id: policy\nstatus: draft\n")
	seedValidationCacheWithState(t, root)
	assertCacheHoldsTest001(t, root)

	writeProjectFile(t, root, rel, "id: policy\nstatus: enforced\n")
	assertCacheDroppedTest001(t, root)
}

func TestValidationStateCache_ParserChangeInvalidates(t *testing.T) {
	t.Parallel()
	root := registerZQKTestRootForTest(t)
	rel := "pkg/migration/parser/parser.go"
	writeProjectFile(t, root, rel, "package parser\n// original\n")
	seedValidationCacheWithState(t, root)
	assertCacheHoldsTest001(t, root)

	writeProjectFile(t, root, rel, "package parser\n// skip branch_ref\n")
	assertCacheDroppedTest001(t, root)
}

func TestValidationStateCache_EmptyCodeChecksumInvalidates(t *testing.T) {
	t.Parallel()
	root := registerZQKTestRootForTest(t)
	writeProjectFile(t, root, "cmd/zqk/system/check_instance_validation_helpers.go", "package system\n")
	cache := seedValidationCacheWithState(t, root)
	payload := map[string]any{
		objects.FieldKeyVersion: ValidationCacheVersion,
		"updated":               time.Now().UTC(),
		"by_kind":               map[string]any{},
		"states": []*ValidationState{{
			ObjectID:      "TEST-001",
			ObjectKind:    "test_object",
			FilePath:      "test.yaml",
			LastValidated: time.Now(),
			Checksum:      "abc123",
			Issues:        []ValidationIssue{},
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := fileutil.WriteFile(cache.cacheFile, data, paths.FilePerm644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	assertCacheDroppedTest001(t, root)
}
