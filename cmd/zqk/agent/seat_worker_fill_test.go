package agent

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestFillSubmitCommand_kindGating(t *testing.T) {
	t.Parallel()
	ghost := whatsnext.CompileFillItem(&whatsnext.KernelAmbience{GhostRefCount: 1})
	if ghost == nil || !ghost.AutoSubmit || strings.TrimSpace(ghost.SubmitArgs) == "" {
		t.Fatalf("ghost_ref must auto-submit: %+v", ghost)
	}
	cache := whatsnext.CompileFillItem(&whatsnext.KernelAmbience{Available: false})
	if cache == nil || !cache.AutoSubmit || !strings.Contains(cache.SubmitArgs, paths.ProjectDataDir) {
		t.Fatalf("check_cache_missing must auto-submit with project data dir: %+v", cache)
	}
	metrics := whatsnext.CompileFillItem(&whatsnext.KernelAmbience{
		Available: true,
		MetricsRollup: &whatsnext.MetricsRollupSnapshot{
			NextAdminAction: "zqk scheduler test-failures",
		},
	})
	if metrics != nil && metrics.AutoSubmit {
		t.Fatalf("metrics must not auto-submit: %+v", metrics)
	}
	draft := whatsnext.CompileFillItem(&whatsnext.KernelAmbience{Available: true, DraftPlaneTotal: 2})
	if draft != nil && draft.AutoSubmit {
		t.Fatalf("draft must not auto-submit: %+v", draft)
	}
}

func TestRecentFillSubmit_cooldown(t *testing.T) {
	root := t.TempDir()
	if skip, _ := recentFillSubmit(root, whatsnext.FillKindGhostRef); skip {
		t.Fatal("missing mark must not skip")
	}
	if err := writeFillSubmitMark(root, whatsnext.FillKindGhostRef, "SCH-test"); err != nil {
		t.Fatal(err)
	}
	if skip, msg := recentFillSubmit(root, whatsnext.FillKindGhostRef); !skip {
		t.Fatalf("fresh mark must skip, msg=%q", msg)
	}
	path := fillSubmitMarkPath(root, whatsnext.FillKindGhostRef)
	old := []byte(`{"schema":"zqk_kernel_fill_submit_v1","kind":"ghost_ref","dispatched_at":"` + time.Now().UTC().Add(-planOrchSubmitCooldown-time.Minute).Format(time.RFC3339) + `","detail":"old"}` + "\n")
	if err := fileutil.WriteFile(path, old, paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if skip, _ := recentFillSubmit(root, whatsnext.FillKindGhostRef); skip {
		t.Fatal("expired mark must not skip")
	}
	if filepath.Base(path) != "fill-ghost_ref.json" {
		t.Fatalf("mark name %s", path)
	}
}
