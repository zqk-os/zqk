package newcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

func TestWriteDraft_file(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot(), dir)
	p := filepath.Join(dir, "out.yaml")
	cmd := &cobra.Command{}
	if err := writeDraft(cmd, p, "test", []byte("x"), cli.LastDraftScopeObject, "backlog_item"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "x" {
		t.Fatalf("read: %v %q", err, b)
	}
}
