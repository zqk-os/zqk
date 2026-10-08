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

func TestSubmitKernelFill_EarlyReturns(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	root := t.TempDir()

	// 1. Nil fill item
	res, err := submitKernelFill(ctx, root, nil)
	if err != nil || res != "" {
		t.Fatalf("expected empty nil, got %q, %v", res, err)
	}

	// 2. AutoSubmit false
	res, err = submitKernelFill(ctx, root, &whatsnext.FillItem{AutoSubmit: false})
	if err != nil || res != "" {
		t.Fatalf("expected empty nil, got %q, %v", res, err)
	}

	// 3. Empty SubmitArgs
	res, err = submitKernelFill(ctx, root, &whatsnext.FillItem{AutoSubmit: true, SubmitArgs: "   "})
	if err != nil || res != "" {
		t.Fatalf("expected empty nil, got %q, %v", res, err)
	}

	// 4. Recent submit cooldown
	if err := writeFillSubmitMark(root, "ghost_ref", "SCH-123"); err != nil {
		t.Fatal(err)
	}
	res, err = submitKernelFill(ctx, root, &whatsnext.FillItem{
		Kind:       "ghost_ref",
		AutoSubmit: true,
		SubmitArgs: "some command",
	})
	if err != nil || !strings.HasPrefix(res, "already_submitted") {
		t.Fatalf("expected already_submitted, got %q, %v", res, err)
	}
}

func TestSchedulerCallbackNotify(t *testing.T) {
	t.Parallel()
	got := schedulerCallbackNotify("zqk", "/tmp/log.json")
	want := "zqk callback notify --log-file /tmp/log.json"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSubmitKernelFill_Execution(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()

	binDir := filepath.Join(root, "bin")
	_ = fileutil.MkdirAll(binDir, 0755)
	mockBin := filepath.Join(binDir, "mock_zqk.sh")
	script := "#!/bin/sh\ncase \"$*\" in *TRIGGER_MOCK_FAILURE*) echo \"mock failure\" >&2; exit 1;; esac\necho \"SCH-SUBMITTED-999\"\n"
	if err := fileutil.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ZQK_BIN", mockBin)

	// 1. Success execution
	fillSuccess := &whatsnext.FillItem{
		Kind:       "test_fill",
		AutoSubmit: true,
		SubmitArgs: "object draft sweep",
	}
	res, err := submitKernelFill(ctx, root, fillSuccess)
	if err != nil {
		t.Fatalf("submitKernelFill failed: %v", err)
	}
	if !strings.Contains(res, "SCH-SUBMITTED-999") {
		t.Fatalf("expected SCH-SUBMITTED-999, got %q", res)
	}

	// 2. Failure execution
	fillFail := &whatsnext.FillItem{
		Kind:       "test_fail_fill",
		AutoSubmit: true,
		SubmitArgs: "TRIGGER_MOCK_FAILURE",
	}
	_, err = submitKernelFill(ctx, root, fillFail)
	if err == nil {
		t.Fatal("expected error from failed submitKernelFill, got nil")
	}
}
