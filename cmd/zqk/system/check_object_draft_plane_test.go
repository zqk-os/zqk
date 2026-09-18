package system

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestWriteObjectDraftPlaneSummary(t *testing.T) {
	root := t.TempDir()
	var buf strings.Builder
	writeObjectDraftPlaneSummary(&buf, root)
	if !strings.Contains(buf.String(), "Pre-Membrane (Object Draft Plane)") ||
		!strings.Contains(buf.String(), "None awaiting crossing.") {
		t.Fatalf("expected explicit empty draft-plane table, got %q", buf.String())
	}

	id := "DOC-check-draft-01"
	p := storage.ObjectDraftPlanePath(root, "doc_entry", id)
	if err := fileutil.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(p, []byte("id: "+id+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	writeObjectDraftPlaneSummary(&buf, root)
	out := buf.String()
	if !strings.Contains(out, "Have Not Crossed the CAS Membrane") {
		t.Fatalf("missing section: %s", out)
	}
	if !strings.Contains(out, "Doc entry") || !strings.Contains(out, "1") {
		t.Fatalf("missing kind count: %s", out)
	}
	if !strings.Contains(out, id) {
		t.Fatalf("expected sample id in table, got: %s", out)
	}
	if !strings.Contains(out, "outside CAS") {
		t.Fatalf("draft plane must be distinguished from in-membrane layers: %s", out)
	}
	if !strings.Contains(out, paths.ObjectDraftsDir) {
		t.Fatalf("missing path: %s", out)
	}
}
