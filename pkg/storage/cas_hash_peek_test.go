package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Regression: convergence_session (and similar) YAML can place top-level id: after a large
// activity_log; a fixed 8KiB peek missed the id and CAS discovery failed to index the object.
func TestCasHashFilePeekContainsObjectID_IDAfterLongPrefix(t *testing.T) {
	dir := t.TempDir()
	id := "CONV-EXAMPLE"
	var body strings.Builder
	for body.Len() < 9000 {
		body.WriteString("# filler line for test\n")
	}
	body.WriteString("id: ")
	body.WriteString(id)
	body.WriteString("\nkind: convergence_session\n")
	data := []byte(body.String())
	hash := CalculateSHA256Hash(data)
	path := filepath.Join(dir, hash+".yaml")
	if err := fileutil.WriteSecureFile(path, data); err != nil {
		t.Fatal(err)
	}
	if !casHashFilePeekContainsObjectID(path, id) {
		t.Fatal("expected embedded id to match when it appears after 8KiB+ of content")
	}
}
