package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestStorageExtended_Wave2_SemanticSnapshot tests all paths in semantic_snapshot.go
func TestStorageExtended_SemanticSnapshot(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. Error when library is nil
	_, err := storagepkg.CreateSemanticSnapshot(nil, nil, time.Now(), logger)
	if err == nil {
		t.Errorf("expected error when library is nil")
	}

	// 2. Library with incomplete brand (missing token or name)
	libIncomplete := &storagepkg.SemanticLibrary{
		Version: "1.0.0",
		Brands: map[string]storagepkg.BrandDef{
			"empty_token": {Name: "BrandOne", SemanticToken: ""},
			"empty_name":  {Name: "", SemanticToken: "@b:two"},
		},
	}
	files := map[string][]byte{
		"file1.txt": []byte("Hello BrandOne world"),
	}
	snapIncomplete, err := storagepkg.CreateSemanticSnapshot(files, libIncomplete, time.Now(), logger)
	if err != nil {
		t.Fatalf("CreateSemanticSnapshot with incomplete brand failed: %v", err)
	}
	if snapIncomplete == nil || len(snapIncomplete.Files) != 1 {
		t.Fatalf("expected 1 file in snapshot")
	}

	// 3. Complete brand compression and expansion:
	// Plain, possessive ('s), camelCase, and kebab-case
	lib := &storagepkg.SemanticLibrary{
		Version: "1.0.0",
		Brands: map[string]storagepkg.BrandDef{
			"zqk": {
				ID:            "zqk",
				Name:          "Zqk",
				SemanticToken: "@b:zqk",
			},
		},
		CompressionOptions: &storagepkg.CompressionOptions{
			CaseSensitive: false,
		},
	}

	testContent := "Welcome to Zqk! Zqk's engine is ZqkConfig and zqk-based microservice."
	filesComplete := map[string][]byte{
		"test.md": []byte(testContent),
	}
	now := time.Now()
	snapshot, err := storagepkg.CreateSemanticSnapshot(filesComplete, lib, now, logger)
	if err != nil {
		t.Fatalf("CreateSemanticSnapshot failed: %v", err)
	}

	if snapshot.Header.Checksum == "" {
		t.Errorf("expected checksum in header")
	}
	if snapshot.Header.FileCount != 1 {
		t.Errorf("expected file count 1, got %d", snapshot.Header.FileCount)
	}

	// Expand semantic snapshot
	expanded, err := storagepkg.ExpandSemanticSnapshot(snapshot)
	if err != nil {
		t.Fatalf("ExpandSemanticSnapshot failed: %v", err)
	}
	expandedContent, ok := expanded["test.md"]
	if !ok {
		t.Fatalf("expected test.md in expanded files")
	}
	if len(expandedContent) == 0 {
		t.Errorf("expected non-empty expanded content")
	}

	// 4. Expand error cases
	if _, err := storagepkg.ExpandSemanticSnapshot(nil); err == nil {
		t.Errorf("expected error expanding nil snapshot")
	}
	if _, err := storagepkg.ExpandSemanticSnapshot(&storagepkg.SemanticSnapshot{Header: nil, Library: nil}); err == nil {
		t.Errorf("expected error expanding snapshot with nil library")
	}

	// 5. SubstituteSemanticTokens
	if err := storagepkg.SubstituteSemanticTokens(nil, "@b:zqk", "@b:new"); err == nil {
		t.Errorf("expected error substituting tokens on nil snapshot")
	}
	if err := storagepkg.SubstituteSemanticTokens(snapshot, "", "@b:new"); err == nil {
		t.Errorf("expected error on empty fromToken")
	}
	if err := storagepkg.SubstituteSemanticTokens(snapshot, "@b:zqk", ""); err == nil {
		t.Errorf("expected error on empty toToken")
	}
	if err := storagepkg.SubstituteSemanticTokens(snapshot, "@b:zqk", "@b:newzqk"); err != nil {
		t.Fatalf("SubstituteSemanticTokens failed: %v", err)
	}

	// 6. Write and Read SemanticSnapshot
	tmpFile := filepath.Join(t.TempDir(), "sub", "test.ssnap")
	if err := storagepkg.WriteSemanticSnapshot(nil, tmpFile); err == nil {
		t.Errorf("expected error writing nil snapshot")
	}
	if err := storagepkg.WriteSemanticSnapshot(snapshot, tmpFile); err != nil {
		t.Fatalf("WriteSemanticSnapshot failed: %v", err)
	}

	readSnap, err := storagepkg.ReadSemanticSnapshot(tmpFile)
	if err != nil {
		t.Fatalf("ReadSemanticSnapshot failed: %v", err)
	}
	if readSnap.Header.Checksum != snapshot.Header.Checksum {
		t.Errorf("checksum mismatch: %s vs %s", readSnap.Header.Checksum, snapshot.Header.Checksum)
	}

	// Nonexistent file
	if _, err := storagepkg.ReadSemanticSnapshot(tmpFile + ".none"); err == nil {
		t.Errorf("expected error reading nonexistent file")
	}

	// Corrupt checksum test
	corruptFile := filepath.Join(t.TempDir(), "corrupt.ssnap")
	snapshot.Header.Checksum = "invalid-checksum"
	_ = storagepkg.WriteSemanticSnapshot(snapshot, corruptFile)
	// Modify the file directly so header has bad checksum
	badData := []byte("header:\n  checksum: wrong-checksum\n  format_version: 1.0.0\nfiles:\n- path: f.txt\n  content_delta: hi\n")
	_ = fileutil.WriteFile(corruptFile, badData, 0644)
	if _, err := storagepkg.ReadSemanticSnapshot(corruptFile); err == nil {
		t.Errorf("expected error reading snapshot with checksum mismatch")
	}
}

// TestStorageExtended_Wave2_DarwinCASVisibilityWait tests darwin_cas_visibility_wait.go
func TestStorageExtended_DarwinCASVisibilityWait(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// 1. PostFlushDarwinCASVisibilityIfNeeded - empty project root
	if err := storagepkg.PostFlushDarwinCASVisibilityIfNeededForTest(ctx, "", []string{"priority_plan"}); err != nil {
		t.Errorf("unexpected error on empty projectRoot: %v", err)
	}

	// 2. PostFlushDarwinCASVisibilityIfNeeded - empty kinds / stream kinds
	if err := storagepkg.PostFlushDarwinCASVisibilityIfNeededForTest(ctx, tmpDir, []string{"", "audit_event"}); err != nil {
		t.Errorf("unexpected error on stream kind or empty kind: %v", err)
	}

	// 3. DarwinCASVisibility on non-darwin platform returns nil early
	if runtime.GOOS != "darwin" {
		t.Skip("skipping darwin-specific visibility wait on non-darwin")
	}

	// Setup directories for a kind
	kind := objects.KindPriorityPlan
	dirName := objects.GetDirectoryFromKind(kind)
	kindDir := datacell.CellCASPrimaryDir(tmpDir, dirName)
	_ = os.MkdirAll(kindDir, 0755)

	// Create empty index
	indexPath := filepath.Join(kindDir, "."+kind+".index")
	_ = fileutil.WriteFile(indexPath, []byte(`{"mappings":{}}`), 0644)

	// Visibility wait with immediate deadline
	deadline := time.Now().Add(100 * time.Millisecond)
	err := storagepkg.DarwinWaitCASKindVisibleForTest(ctx, tmpDir, kind, deadline)
	if err != nil {
		t.Logf("DarwinWaitCASKindVisible returned: %v (tolerated on empty index)", err)
	}

	// Test darwinKindDirYAMLFingerprint
	fp1, err := storagepkg.DarwinKindDirYAMLFingerprintForTest(kindDir)
	if err != nil {
		t.Errorf("darwinKindDirYAMLFingerprint failed: %v", err)
	}
	if fp1 != "" {
		t.Errorf("expected empty fingerprint for dir with no yaml files, got %s", fp1)
	}

	// Add a yaml file
	yamlPath := filepath.Join(kindDir, "PRI-1.yaml")
	_ = fileutil.WriteFile(yamlPath, []byte("id: PRI-1\nkind: priority_plan"), 0644)
	fp2, err := storagepkg.DarwinKindDirYAMLFingerprintForTest(kindDir)
	if err != nil {
		t.Errorf("darwinKindDirYAMLFingerprint failed: %v", err)
	}
	if fp2 == "" {
		t.Errorf("expected non-empty fingerprint after writing yaml")
	}

	// Test darwinWaitKindDirYAMLFingerprintStable
	err = storagepkg.DarwinWaitKindDirYAMLFingerprintStableForTest(ctx, kindDir, time.Now().Add(200*time.Millisecond))
	if err != nil {
		t.Errorf("darwinWaitKindDirYAMLFingerprintStable failed: %v", err)
	}

	// Test darwinSleepPoll with canceled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = storagepkg.DarwinSleepPollForTest(cancCtx, 50*time.Millisecond)
	if err == nil {
		t.Errorf("expected error from canceled context in darwinSleepPoll")
	}
}
