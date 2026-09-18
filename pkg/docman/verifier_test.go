package docman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestVerifier_VerifyAndAutoSeal(t *testing.T) {
	tmpDir := t.TempDir()
	relDoc := "docs/architecture/test_doc.md"
	fullDocPath := filepath.Join(tmpDir, relDoc)
	if err := os.MkdirAll(filepath.Dir(fullDocPath), 0750); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	content := "# Architecture Title\nOriginal content line."
	if err := fileutil.WriteFile(fullDocPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write doc: %v", err)
	}

	h := sha256.Sum256([]byte(content))
	correctHash := hex.EncodeToString(h[:])
	correctSize := int64(len(content))

	mockStorage := newMockDocStorageProvider()

	// 1. Unsealed active doc
	doc1 := map[string]any{
		objects.FieldKeyID:     "DOC-TEST-001",
		objects.FieldKeyKind:   objects.KindDocEntry,
		objects.FieldKeyStatus: "active",
		objects.FieldKeyPath:   relDoc,
	}
	_ = mockStorage.Create(context.Background(), nil, doc1)

	verifier := NewVerifier(mockStorage, tmpDir)

	// Verify unsealed without auto-seal -> should report unsealed
	res, err := verifier.Verify(context.Background(), "debug", VerifyOptions{
		IDs: []string{"DOC-TEST-001"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Unsealed != 1 || len(res.Violations) != 1 {
		t.Fatalf("expected 1 unsealed violation, got res: %+v", res)
	}

	// Verify with auto-seal -> should seal and pass
	resAuto, err := verifier.Verify(context.Background(), "debug", VerifyOptions{
		IDs:      []string{"DOC-TEST-001"},
		AutoSeal: true,
	})
	if err != nil {
		t.Fatalf("unexpected error during auto-seal: %v", err)
	}
	if resAuto.AutoSealed != 1 || resAuto.Passed != 1 || len(resAuto.Violations) != 0 {
		t.Fatalf("expected auto-seal success, got: %+v", resAuto)
	}

	// Verify the object in storage now has hash and size
	updatedObj, err := mockStorage.Read(context.Background(), nil, "DOC-TEST-001")
	if err != nil {
		t.Fatalf("failed to read updated doc: %v", err)
	}
	if updatedObj["content_hash"] != correctHash {
		t.Errorf("expected hash %s, got %v", correctHash, updatedObj["content_hash"])
	}
	if updatedObj["content_size"] != correctSize {
		t.Errorf("expected size %d, got %v", correctSize, updatedObj["content_size"])
	}

	// Tamper file content to create drift
	tamperedContent := "# Architecture Title\nTampered line."
	if err := fileutil.WriteFile(fullDocPath, []byte(tamperedContent), 0644); err != nil {
		t.Fatalf("failed to tamper doc: %v", err)
	}

	// Verify drift detection without auto-seal
	resDrift, err := verifier.Verify(context.Background(), "debug", VerifyOptions{
		IDs: []string{"DOC-TEST-001"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resDrift.Drifted != 1 || len(resDrift.Violations) == 0 {
		t.Fatalf("expected drift violation, got: %+v", resDrift)
	}
}
