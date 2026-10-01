package newcmd

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestWriteDraft_file(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), dir)
	p := filepath.Join(dir, "out.yaml")
	cmd := &cobra.Command{}
	if err := writeDraft(cmd, p, "test", []byte("x"), cli.LastDraftScopeObject, "backlog_item"); err != nil {
		t.Fatal(err)
	}
	b, err := fileutil.ReadFile(p)
	if err != nil || string(b) != "x" {
		t.Fatalf("read: %v %q", err, b)
	}
}
