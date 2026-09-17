package filecas

import (
	"path/filepath"
	"strings"
	"testing"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestCalculateSHA256Hash_empty(t *testing.T) {
	t.Parallel()
	got := CalculateSHA256Hash(nil)
	const want = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Fatalf("hash(nil)=%q want %q", got, want)
	}
	if CalculateSHA256Hash([]byte{}) != want {
		t.Fatalf("hash(empty slice) drifted from SHA-256 of empty input")
	}
}

func TestVerifyContentHash(t *testing.T) {
	t.Parallel()
	content := []byte("filecas-helper")
	want := CalculateSHA256Hash(content)
	if err := VerifyContentHash(content, want); err != nil {
		t.Fatalf("VerifyContentHash matching: %v", err)
	}
	err := VerifyContentHash(content, strings.Repeat("0", 64))
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("mismatch error=%v", err)
	}
}

func TestProjectRootFromCASKindDir(t *testing.T) {
	t.Parallel()
	kindDir := filepath.Join("/proj", paths.ProcessDir, "backlog")
	if got := projectRootFromCASKindDir(kindDir); got != "/proj" {
		t.Fatalf("projectRootFromCASKindDir=%q want /proj", got)
	}
	indexPath := filepath.Join("/proj", paths.ProcessDir, "backlog", "index.json")
	if got := projectRootFromCASIndexPath(indexPath); got != "/proj" {
		t.Fatalf("projectRootFromCASIndexPath=%q want /proj", got)
	}
}
