package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestSeedStarterKernelGraph_WritesOrg(t *testing.T) {
	dir := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	if err := seedStarterKernelGraph(dir, logger); err != nil {
		t.Fatal(err)
	}
	orgDir := filepath.Join(dir, ".zqk", "process", objects.GetDirectoryFromKind(objects.KindOrganization))
	entries, err := os.ReadDir(orgDir)
	if err != nil {
		t.Fatalf("starter org dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected starter organization CAS file")
	}
}
