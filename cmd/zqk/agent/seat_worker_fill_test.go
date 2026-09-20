package agent

import (
	"path/filepath"
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestFillSubmitCommand_kindGating(t *testing.T) {
	t.Parallel()
	if got := fillSubmitCommand(whatsnext.FillKindGhostRef); got == "" {
		t.Fatal("ghost_ref must submit")
	}
	if got := fillSubmitCommand(whatsnext.FillKindCheckCache); got == "" {
		t.Fatal("check_cache_missing must submit")
	}
	if got := fillSubmitCommand(whatsnext.FillKindMetrics); got != "" {
		t.Fatalf("metrics must not auto-submit: %q", got)
	}
	if got := fillSubmitCommand(whatsnext.FillKindDraftPlane); got != "" {
		t.Fatalf("draft must not auto-submit: %q", got)
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
	if err := fileutil.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if skip, _ := recentFillSubmit(root, whatsnext.FillKindGhostRef); skip {
		t.Fatal("expired mark must not skip")
	}
	if filepath.Base(path) != "fill-ghost_ref.json" {
		t.Fatalf("mark name %s", path)
	}
}
