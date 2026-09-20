package filecas

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCasHashFilenameRe(t *testing.T) {
	t.Parallel()
	hash := CalculateSHA256Hash([]byte("peek"))
	if !CasHashFilenameRe.MatchString(hash + ".yaml") {
		t.Fatalf("expected %s.yaml to match CAS filename regex", hash)
	}
	if CasHashFilenameRe.MatchString("not-a-hash.yaml") {
		t.Fatal("non-hash name should not match")
	}
}

func TestCasHashFilePeekObjectID_ignoresIndentedNestedID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := []byte(`created_at: "2026-08-12T03:28:31Z"
domains:
    - description: Test domain for comprehensive tests
      id: test_domain
      title: Test Domain
id: DOMAIN-REG-004
kind: domain_registry
`)
	path := filepath.Join(dir, CalculateSHA256Hash(body)+".yaml")
	if err := fileutil.WriteSecureFile(path, body); err != nil {
		t.Fatal(err)
	}
	if got := CasHashFilePeekObjectID(path); got != "DOMAIN-REG-004" {
		t.Fatalf("peekObjectID=%q want DOMAIN-REG-004", got)
	}
	if !CasHashFilePeekContainsObjectID(path, "DOMAIN-REG-004") {
		t.Fatal("expected contains match for top-level id")
	}
	if CasHashFilePeekContainsObjectID(path, "test_domain") {
		t.Fatal("indented nested id must not match")
	}
}

func TestCasHashFilePeekObjectID_quotedAndMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := []byte("id: \"OBJ-QUOTED-1\"\nkind: goal\n")
	path := filepath.Join(dir, CalculateSHA256Hash(body)+".yaml")
	if err := fileutil.WriteSecureFile(path, body); err != nil {
		t.Fatal(err)
	}
	if got := CasHashFilePeekObjectID(path); got != "OBJ-QUOTED-1" {
		t.Fatalf("quoted peek=%q", got)
	}
	if CasHashFilePeekObjectID(filepath.Join(dir, "missing.yaml")) != "" {
		t.Fatal("missing file should peek empty")
	}
}

func TestCasHashFilePeekObjectID_idAfterLongPrefix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	id := "CVS-1780000000000000000-peektest"
	var body strings.Builder
	for body.Len() < 9000 {
		body.WriteString("# filler line for test\n")
	}
	body.WriteString("id: ")
	body.WriteString(id)
	body.WriteString("\nkind: convergence_session\n")
	data := []byte(body.String())
	path := filepath.Join(dir, CalculateSHA256Hash(data)+".yaml")
	if err := fileutil.WriteSecureFile(path, data); err != nil {
		t.Fatal(err)
	}
	if !CasHashFilePeekContainsObjectID(path, id) {
		t.Fatal("expected id after 8KiB+ prefix")
	}
	if err := fileutil.RemoveFile(path); err != nil {
		t.Fatal(err)
	}
}
