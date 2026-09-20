package system

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestDetectAndConsumeSignificantCacheChange_Marker(t *testing.T) {
	root := t.TempDir()
	storage.ResetSignificantCacheChangeForTest(root)
	t.Cleanup(func() { storage.ResetSignificantCacheChangeForTest(root) })

	if _, ok := detectAndConsumeSignificantCacheChange(root); ok {
		t.Fatal("expected no significant change")
	}
	storage.NoteSignificantCacheChange(root, "test_marker")
	reason, ok := detectAndConsumeSignificantCacheChange(root)
	if !ok || reason != "test_marker" {
		t.Fatalf("reason=%q ok=%v", reason, ok)
	}
	if _, ok := detectAndConsumeSignificantCacheChange(root); ok {
		t.Fatal("marker should be consumed once")
	}
}

func TestHandleClearCache_AutoEnablesRefreshOnSignificantChange(t *testing.T) {
	root := t.TempDir()
	storage.ResetSignificantCacheChangeForTest(root)
	t.Cleanup(func() { storage.ResetSignificantCacheChangeForTest(root) })
	storage.NoteSignificantCacheChange(root, "unit_auto_refresh")

	cmd := &cobra.Command{Use: "check"}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("clear-cache", false, "")
	cmd.Flags().Bool("refresh-cache", false, "")

	checkCtx := &AsyncCheckContext{
		Cmd:         cmd,
		ProjectRoot: root,
		Logger:      getLoggerForSystemCheck(cmd, "system"),
	}
	checkCtx.AsyncValidator = GetAsyncValidator(context.Background(), root, 0)

	if err := handleClearCache(checkCtx); err != nil {
		t.Fatalf("handleClearCache: %v", err)
	}
	clear, _ := cmd.Flags().GetBool("clear-cache")
	refresh, _ := cmd.Flags().GetBool("refresh-cache")
	if !clear || !refresh {
		t.Fatalf("expected auto clear+refresh, clear=%v refresh=%v", clear, refresh)
	}
	if _, ok := storage.PeekSignificantCacheChange(root); ok {
		t.Fatal("marker should be consumed")
	}
}

func TestHandleClearCache_PendingBurstDoesNotClearValidation(t *testing.T) {
	root := t.TempDir()
	storage.ResetSignificantCacheChangeForTest(root)
	storage.ResetObjectIDCachePendingForTest()
	t.Cleanup(func() {
		storage.ResetSignificantCacheChangeForTest(root)
		storage.ResetObjectIDCachePendingForTest()
	})
	for i := 0; i < storage.SignificantCacheChangePendingThreshold; i++ {
		id := "GOAL-" + string(rune('A'+i))
		storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), id, "goal",
			filepath.Join(root, paths.ProcessDir, "goals", id+".yaml"), "test")
	}

	cmd := &cobra.Command{Use: "check"}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("clear-cache", false, "")
	cmd.Flags().Bool("refresh-cache", false, "")

	checkCtx := &AsyncCheckContext{
		Cmd:         cmd,
		ProjectRoot: root,
		Logger:      getLoggerForSystemCheck(cmd, "system"),
	}
	checkCtx.AsyncValidator = GetAsyncValidator(context.Background(), root, 0)

	if err := handleClearCache(checkCtx); err != nil {
		t.Fatalf("handleClearCache: %v", err)
	}
	clear, _ := cmd.Flags().GetBool("clear-cache")
	refresh, _ := cmd.Flags().GetBool("refresh-cache")
	if clear {
		t.Fatal("pending burst must not auto-enable --clear-cache")
	}
	if !refresh {
		t.Fatal("pending burst must still refresh object-id-cache")
	}
}
