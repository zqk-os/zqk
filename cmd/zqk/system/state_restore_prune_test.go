package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPruneOrphansSkipsCoreKernelDirs(t *testing.T) {
	root := t.TempDir()
	processDir := filepath.Join(root, paths.ProcessDir)
	wsDir := filepath.Join(processDir, objects.GetDirectoryFromKind(objects.KindWorkstream))
	metricDir := filepath.Join(processDir, "command_metrics")
	if err := fileutil.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(metricDir, 0o755); err != nil {
		t.Fatal(err)
	}
	coreFile := filepath.Join(wsDir, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.yaml")
	metricFile := filepath.Join(metricDir, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.yaml")
	for _, f := range []string{coreFile, metricFile} {
		if err := fileutil.WriteFile(f, []byte("id: X\nkind: y\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := &cobra.Command{}
	cmd.SetOut(os.Stdout)
	// expectedPaths empty → both look orphaned; core dir must be skipped
	if err := pruneOrphans(context.Background(), root, map[string]bool{}, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := fileutil.Stat(coreFile); err != nil {
		t.Fatalf("core workstream YAML must survive prune: %v", err)
	}
	if _, err := fileutil.Stat(metricFile); !fileutil.IsNotExist(err) {
		t.Fatalf("non-core orphan metric YAML should be pruned, still present: %v", err)
	}
}
