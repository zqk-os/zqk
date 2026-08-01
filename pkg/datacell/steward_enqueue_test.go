package datacell

import (
	"os"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestAppendStewardEnqueueRecord_roundTripPathSegments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rec := BuildStewardEnqueueRecord(ProfileStream, MaintenanceOp{Name: MaintenanceOpRefreshSummary, Detail: "t"})
	if err := AppendStewardEnqueueRecord(root, rec); err != nil {
		t.Fatal(err)
	}
	got := StewardEnqueueJSONLPath(root)
	wantSuffix := paths.ProjectDataDir + "/" + paths.LogsDir + "/" + paths.DataCellLogsSubdir + "/" + paths.StewardEnqueueJSONLFile
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("path %q should end with %q", got, wantSuffix)
	}
}

func TestDrainStewardEnqueueLog_removesFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	rec := BuildStewardEnqueueRecord(ProfileStream, MaintenanceOp{Name: MaintenanceOpInvalidateCache})
	if err := AppendStewardEnqueueRecord(root, rec); err != nil {
		t.Fatal(err)
	}
	path := StewardEnqueueJSONLPath(root)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	n, err := DrainStewardEnqueueLog(root, log, "")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("drained=%d", n)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed: %v", err)
	}
}
