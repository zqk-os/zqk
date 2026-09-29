package cas_test

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/storage/filecas"

	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCasHashFilePeekObjectID_IgnoresIndentedNestedID(t *testing.T) {
	dir := t.TempDir()
	// domain_registry fixtures embed domains[].id before the object id — must not dual-CAS on test_domain.
	body := []byte(`created_at: "2026-08-12T03:28:31Z"
domains:
    - description: Test domain for comprehensive tests
      id: test_domain
      title: Test Domain
id: DOMAIN-REG-004
kind: domain_registry
status: active
title: 'Fixture: domain_registry #4'
`)
	path := filepath.Join(dir, storage.CalculateSHA256Hash(body)+".yaml")
	if err := fileutil.WriteSecureFile(path, body); err != nil {
		t.Fatal(err)
	}
	if got := filecas.CasHashFilePeekObjectID(path); got != "DOMAIN-REG-004" {
		t.Fatalf("peekObjectID=%q want DOMAIN-REG-004", got)
	}
}

// Regression: convergence_session (and similar) YAML can place top-level id: after a large
// activity_log; a fixed 8KiB peek missed the id and CAS discovery failed to index the object.
func TestCasHashFilePeekContainsObjectID_IDAfterLongPrefix(t *testing.T) {
	dir := t.TempDir()
	id := "CVS-1776080000000000000-a1b2c3d4"
	var body strings.Builder
	for body.Len() < 9000 {
		body.WriteString("# filler line for test\n")
	}
	body.WriteString("id: ")
	body.WriteString(id)
	body.WriteString("\nkind: convergence_session\n")
	data := []byte(body.String())
	hash := storage.CalculateSHA256Hash(data)
	path := filepath.Join(dir, hash+".yaml")
	if err := fileutil.WriteSecureFile(path, data); err != nil {
		t.Fatal(err)
	}
	if !filecas.CasHashFilePeekContainsObjectID(path, id) {
		t.Fatal("expected embedded id to match when it appears after 8KiB+ of content")
	}
	if got := filecas.CasHashFilePeekObjectID(path); got != id {
		t.Fatalf("peekObjectID=%q want %q", got, id)
	}
}

func TestRemoveOrphanCASFilesForObjectIDs_singleWalk(t *testing.T) {
	dir := t.TempDir()
	ids := []string{"SCH-run-a", "SCH-run-b", "SCH-run-keep"}
	paths := map[string]string{}
	for _, id := range ids {
		body := []byte("id: " + id + "\nkind: scheduler_job\n")
		hash := storage.CalculateSHA256Hash(body)
		p := filepath.Join(dir, hash+".yaml")
		if err := fileutil.WriteSecureFile(p, body); err != nil {
			t.Fatal(err)
		}
		paths[id] = p
	}
	want := map[string]struct{}{"SCH-run-a": {}, "SCH-run-b": {}}
	storage.RemoveOrphanCASFilesForObjectIDsForTest(want, dir)
	if _, err := fileutil.Stat(paths["SCH-run-a"]); !fileutil.IsNotExist(err) {
		t.Fatalf("SCH-run-a should be removed: %v", err)
	}
	if _, err := fileutil.Stat(paths["SCH-run-b"]); !fileutil.IsNotExist(err) {
		t.Fatalf("SCH-run-b should be removed: %v", err)
	}
	if _, err := fileutil.Stat(paths["SCH-run-keep"]); err != nil {
		t.Fatalf("keep should remain: %v", err)
	}
}
