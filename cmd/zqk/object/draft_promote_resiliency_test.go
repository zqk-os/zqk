package object

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/spf13/cobra"
)

func setupTestProjectForDraftPromote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Setup required process dirs
	for _, sub := range []string{
		paths.ProcessInternalObjectSpecsDir,
		paths.ProcessInternalLifecyclesDir,
		paths.ProcessDir,
		filepath.Join(paths.ProcessDir, "requirements"),
		filepath.Join(paths.ProcessDir, "criteria"),
		filepath.Join(paths.ProcessDir, "backlog_items"),
		storage.ObjectDraftPlaneRoot(root),
	} {
		if err := fileutil.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestPromoteObjectIDs_ContextCancellationGracefulStop(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment uses process-global ZQK_TEST_ROOT.
	_ = SetupTestEnvironment(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled context

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	// When context is canceled, promoteObjectIDs should exit without infinite loops or unbounded panic
	err = promoteObjectIDs(cmd, proc, []string{"BLI-1", "BLI-2", "BLI-3"})
	if err == nil {
		t.Fatal("expected error on canceled context, got nil")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context cancellation error, got: %v", err)
	}
}

func TestDraftPromote_AdaptiveBatchingDataStructures(t *testing.T) {
	t.Parallel()

	// Verify batch size and error bound constants
	if defaultDraftPromoteBatchSize != 10 {
		t.Fatalf("expected defaultDraftPromoteBatchSize=10, got %d", defaultDraftPromoteBatchSize)
	}
	if maxBoundedDraftErrors != 50 {
		t.Fatalf("expected maxBoundedDraftErrors=50, got %d", maxBoundedDraftErrors)
	}
}

func TestDraftPromote_TimeoutRuleRegistered(t *testing.T) {
	t.Parallel()

	root := paths.FindNearestProjectRoot(".")
	configPath := filepath.Join(root, "config", "command_timeouts.yaml")
	content, err := fileutil.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read command_timeouts.yaml: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "object draft promote") {
		t.Fatal("command_timeouts.yaml must contain rule for 'object draft promote'")
	}
	if !strings.Contains(text, "child_max_timeout_exempt: true") {
		t.Fatal("command_timeouts.yaml rule for 'object draft promote' must be child_max_timeout_exempt")
	}
}
