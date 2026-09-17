package system

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCheckIntegrity_skipsObjectDraftPlane(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id := "BLI-draft-integrity-001"
	p := storage.ObjectDraftPlanePath(root, "backlog_item", id)
	if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	content := []byte("id: " + id + "\nkind: backlog_item\nstatus: exploring\n")
	if err := fileutil.WriteFile(p, content, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	ctx := cli.ContextForProjectRoot(root)
	parsed := &parser.ParsedObject{ID: id}
	issues, _ := checkIntegrityWithRegistryAndContent(ctx, parsed, p, "backlog_item", content, nil, nil)
	if len(issues) != 0 {
		t.Fatalf("draft plane must not raise CAS integrity issues, got %#v", issues)
	}
}
