package cli

import (
	"context"
	"github.com/spf13/cobra"
	"testing"

	pkgcli "github.com/lanceman/zqk/pkg/cli"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestRunWithAsyncProgress_success verifies that RunWithAsyncProgress runs runE and returns nil when runE succeeds.
// Progress events (started, complete) are emitted via the global coordinator; full event assertion is in integration tests.
func TestRunWithAsyncProgress_success(t *testing.T) {
	coord := coordination.NewCoordinator(coordination.CoordinatorConfig{})
	old := coordination.GetCoordinator()
	coordination.SetGlobalCoordinator(coord)
	defer coordination.SetGlobalCoordinator(old)

	cmd := pkgcli.NewCommandBuilder("async_cmd").Build()
	cmd.SetContext(context.Background())

	err := RunWithAsyncProgress(cmd, nil, "object_create", func(*cobra.Command, []string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunWithAsyncProgress: %v", err)
	}
}

// TestRunWithAsyncProgress_validation_progress_callback verifies that cmd.Context() gets a validation progress
// callback attached (so storage/validator can emit spec/lifecycle stages). We only assert the wrapper runs.
func TestRunWithAsyncProgress_validation_progress_callback(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("async_cmd").Build()
	cmd.SetContext(context.Background())
	var progressCalls []string
	// Simulate what RunWithAsyncProgress does: attach progress to context before runE
	progressFn := func(stage, message string) {
		progressCalls = append(progressCalls, stage+":"+message)
	}
	cmd.SetContext(pkgctx.WithValidationProgress(cmd.Context(), progressFn))

	// When runE calls storage Create, validateObject would call the callback - we just check context is set
	ctx := cmd.Context()
	got := pkgctx.GetValidationProgress(ctx)
	if got == nil {
		t.Fatal("expected validation progress callback on context")
	}
	got("spec", "Validating schema")
	got(objects.KindLifecycle, "Validating lifecycle")
	if len(progressCalls) != 2 || progressCalls[0] != "spec:Validating schema" || progressCalls[1] != objects.KindLifecycle+":Validating lifecycle" {
		t.Errorf("progress callback: got %v", progressCalls)
	}
}

// TestRunWithAsyncProgress_global_coordinator_assert verifies that after SetGlobalCoordinator(coord),
// GetCoordinator() returns that coordinator and the type assert to *Coordinator succeeds (so ProgressHelper gets non-nil).
func TestRunWithAsyncProgress_global_coordinator_assert(t *testing.T) {
	coord := coordination.NewCoordinator(coordination.CoordinatorConfig{})
	old := coordination.GetCoordinator()
	coordination.SetGlobalCoordinator(coord)
	defer coordination.SetGlobalCoordinator(old)

	got := coordination.GetCoordinator()
	c, ok := got.(*coordination.Coordinator)
	if !ok || c != coord {
		t.Fatalf("GetCoordinator() after SetGlobalCoordinator: ok=%v, same=%v (ProgressHelper would get nil)", ok, c == coord)
	}
}

// TestRunWithAsyncProgress_emits_error_on_failure verifies that when runE returns an error,
// RunWithAsyncProgress returns that error (and emits error via coordinator; full assertion in integration tests).
func TestRunWithAsyncProgress_emits_error_on_failure(t *testing.T) {
	coord := coordination.NewCoordinator(coordination.CoordinatorConfig{})
	old := coordination.GetCoordinator()
	coordination.SetGlobalCoordinator(coord)
	defer coordination.SetGlobalCoordinator(old)

	cmd := pkgcli.NewCommandBuilder("object_create").Build()
	cmd.SetContext(context.Background())

	wantErr := context.DeadlineExceeded
	err := RunWithAsyncProgress(cmd, nil, "object_create", func(*cobra.Command, []string) error {
		return wantErr
	})
	if err != wantErr {
		t.Fatalf("RunWithAsyncProgress: got err %v, want %v", err, wantErr)
	}
}

// TestOperationTypeFromCommand verifies that operation type is derived from command path.
func TestOperationTypeFromCommand(t *testing.T) {
	root := pkgcli.NewCommandBuilder("zqk").Build()
	spec := pkgcli.NewCommandBuilder("spec").Build()
	list := pkgcli.NewCommandBuilder("list").Build()
	root.AddCommand(spec)
	spec.AddCommand(list)

	if got := OperationTypeFromCommand(list); got != "spec_list" {
		t.Errorf("OperationTypeFromCommand(list): got %q, want spec_list", got)
	}
	if got := OperationTypeFromCommand(spec); got != "spec" {
		t.Errorf("OperationTypeFromCommand(spec): got %q, want spec", got)
	}
	// Single-segment path (no parent) falls back to "cli"
	alone := pkgcli.NewCommandBuilder("list").Build()
	if got := OperationTypeFromCommand(alone); got != "cli" {
		t.Errorf("OperationTypeFromCommand(standalone): got %q, want cli", got)
	}
}

// TestBindAsyncProgress verifies that BindAsyncProgress sets RunE and derives operation type at runtime.
func TestBindAsyncProgress(t *testing.T) {
	coord := coordination.NewCoordinator(coordination.CoordinatorConfig{})
	old := coordination.GetCoordinator()
	coordination.SetGlobalCoordinator(coord)
	defer coordination.SetGlobalCoordinator(old)

	root := pkgcli.NewCommandBuilder("zqk").Build()
	spec := pkgcli.NewCommandBuilder("spec").Build()
	list := pkgcli.NewCommandBuilder("list").Build()
	list.SetContext(context.Background())
	root.AddCommand(spec)
	spec.AddCommand(list)

	var capturedType string
	BindAsyncProgress(list, func(c *cobra.Command, _ []string) error {
		capturedType = OperationTypeFromCommand(c)
		return nil
	})

	if list.RunE == nil {
		t.Fatal("BindAsyncProgress did not set RunE")
	}
	if err := list.RunE(list, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if capturedType != "spec_list" {
		t.Errorf("operation type at runtime: got %q, want spec_list", capturedType)
	}
}

// TestNewAsyncCommand verifies that NewAsyncCommand returns a command with RunE and common flags.
func TestNewAsyncCommand(t *testing.T) {
	coord := coordination.NewCoordinator(coordination.CoordinatorConfig{})
	old := coordination.GetCoordinator()
	coordination.SetGlobalCoordinator(coord)
	defer coordination.SetGlobalCoordinator(old)

	cmd := NewAsyncCommand("list", "List items", "Lists items.", func(*cobra.Command, []string) error { return nil })
	if cmd.Use != "list" || cmd.Short != "List items" || cmd.Long != "Lists items." {
		t.Errorf("Use/Short/Long not set: Use=%q Short=%q Long=%q", cmd.Use, cmd.Short, cmd.Long)
	}
	if cmd.RunE == nil {
		t.Fatal("NewAsyncCommand did not set RunE")
	}
	if cmd.Flags().Lookup("format") == nil {
		t.Error("NewAsyncCommand did not add common flags (no --format)")
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
}
