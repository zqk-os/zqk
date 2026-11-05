package system

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestSystemCheckExitCodeOnWarnings verifies GitHub Issue #953:
// 'zqk system check' exit code 3 on non-blocking warnings breaks standard pre-commit hooks.
//
// Verification requirements:
//  1. Without flag, exit code is 3 when warnings are present.
//  2. With --ignore-warnings (or --fail-level error), exit code is 0 when only warnings are present.
//  3. Exit code is 1 when errors are present regardless of --ignore-warnings.
func TestSystemCheckExitCodeOnWarnings(t *testing.T) {
	t.Parallel()

	warningOnlyResults := []CheckResult{
		{
			ObjectID:   "BLI-WARN-001",
			ObjectKind: objects.KindBacklogItem,
			Status:     objects.ObjectStatusPlanned,
			Issues: []Issue{
				{Category: "recommendation", Message: "minor style recommendation", Tier: 2},
				{Category: "hygiene", Message: "advisory naming convention", Tier: 3},
			},
		},
	}

	errorOnlyResults := []CheckResult{
		{
			ObjectID:   "BLI-ERR-001",
			ObjectKind: objects.KindBacklogItem,
			Status:     objects.ObjectStatusPlanned,
			Issues: []Issue{
				{Category: "preconditions", Message: "missing required specification", Tier: 1},
			},
		},
	}

	mixedResults := []CheckResult{
		{
			ObjectID:   "BLI-ERR-002",
			ObjectKind: objects.KindBacklogItem,
			Status:     objects.ObjectStatusPlanned,
			Issues: []Issue{
				{Category: "preconditions", Message: "blocking issue", Tier: 1},
			},
		},
		{
			ObjectID:   "BLI-WARN-002",
			ObjectKind: objects.KindBacklogItem,
			Status:     objects.ObjectStatusPlanned,
			Issues: []Issue{
				{Category: "recommendation", Message: "advisory warning", Tier: 2},
			},
		},
	}

	layer0Results := []CheckResult{
		{
			ObjectID:   "BLI-L0-001",
			ObjectKind: objects.KindBacklogItem,
			Status:     objects.ObjectStatusPlanned,
			Issues: []Issue{
				{Category: categoryIntegrity, Message: "hash mismatch in CAS", Tier: 1},
			},
		},
	}

	t.Run("DefaultWithoutFlags_ExitCode3OnWarnings", func(t *testing.T) {
		err := evaluateSystemCheckPristine(warningOnlyResults, nil, "")
		if err == nil {
			t.Fatal("expected error for warning results, got nil")
		}
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 3 {
			t.Fatalf("expected exit code 3, got %+v", err)
		}
	})

	t.Run("WithIgnoreWarnings_ExitCode0OnWarnings", func(t *testing.T) {
		opts := SystemCheckOptions{IgnoreWarnings: true}
		err := evaluateSystemCheckPristine(warningOnlyResults, nil, "", opts)
		if err != nil {
			t.Fatalf("expected nil (exit code 0) when ignoring warnings, got: %v", err)
		}
	})

	t.Run("WithFailLevelError_ExitCode0OnWarnings", func(t *testing.T) {
		opts := SystemCheckOptions{FailLevel: "error"}
		err := evaluateSystemCheckPristine(warningOnlyResults, nil, "", opts)
		if err != nil {
			t.Fatalf("expected nil (exit code 0) when fail-level is error, got: %v", err)
		}
	})

	t.Run("WithFailLevelBlocking_ExitCode0OnWarnings", func(t *testing.T) {
		opts := SystemCheckOptions{FailLevel: "blocking"}
		err := evaluateSystemCheckPristine(warningOnlyResults, nil, "", opts)
		if err != nil {
			t.Fatalf("expected nil (exit code 0) when fail-level is blocking, got: %v", err)
		}
	})

	t.Run("WithFailLevelWarning_ExitCode3OnWarnings", func(t *testing.T) {
		opts := SystemCheckOptions{FailLevel: "warning"}
		err := evaluateSystemCheckPristine(warningOnlyResults, nil, "", opts)
		if err == nil {
			t.Fatal("expected error when fail-level is warning, got nil")
		}
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 3 {
			t.Fatalf("expected exit code 3, got %+v", err)
		}
	})

	t.Run("WithErrors_ExitCode1RegardlessOfIgnoreWarnings", func(t *testing.T) {
		// Without flag
		err := evaluateSystemCheckPristine(errorOnlyResults, nil, "")
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 1 {
			t.Fatalf("expected exit code 1, got %+v", err)
		}

		// With --ignore-warnings
		errWithIgnore := evaluateSystemCheckPristine(errorOnlyResults, nil, "", SystemCheckOptions{IgnoreWarnings: true})
		if !errors.As(errWithIgnore, &scErr) || scErr.ExitCode() != 1 {
			t.Fatalf("expected exit code 1 with IgnoreWarnings=true, got %+v", errWithIgnore)
		}

		// With --fail-level error
		errWithFailLevel := evaluateSystemCheckPristine(errorOnlyResults, nil, "", SystemCheckOptions{FailLevel: "error"})
		if !errors.As(errWithFailLevel, &scErr) || scErr.ExitCode() != 1 {
			t.Fatalf("expected exit code 1 with FailLevel=error, got %+v", errWithFailLevel)
		}
	})

	t.Run("WithMixedErrorsAndWarnings_ExitCode1RegardlessOfIgnoreWarnings", func(t *testing.T) {
		opts := SystemCheckOptions{IgnoreWarnings: true}
		err := evaluateSystemCheckPristine(mixedResults, nil, "", opts)
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 1 {
			t.Fatalf("expected exit code 1 for mixed errors+warnings even with IgnoreWarnings=true, got %+v", err)
		}
	})

	t.Run("WithLayer0CAS_ExitCode2RegardlessOfIgnoreWarnings", func(t *testing.T) {
		opts := SystemCheckOptions{IgnoreWarnings: true}
		err := evaluateSystemCheckPristine(layer0Results, nil, "", opts)
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 2 {
			t.Fatalf("expected exit code 2 for Layer 0 even with IgnoreWarnings=true, got %+v", err)
		}
	})
}

// TestSystemCheckCommandFlagsExitCode tests the full outputResults path with cobra flags.
func TestSystemCheckCommandFlagsExitCode(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-output.json")
	ctx := cli.ContextForProjectAndProfile(tmpDir, "test")

	warningOnlyResults := []CheckResult{
		{
			ObjectID:   "BLI-WARN-100",
			ObjectKind: objects.KindBacklogItem,
			FilePath:   "test/path.yaml",
			Issues: []Issue{
				{Category: "recommendation", Message: "minor advisory warning", Tier: 2},
			},
		},
	}

	errorResults := []CheckResult{
		{
			ObjectID:   "BLI-ERR-100",
			ObjectKind: objects.KindBacklogItem,
			FilePath:   "test/path_err.yaml",
			Issues: []Issue{
				{Category: "preconditions", Message: "blocking precondition failure", Tier: 1},
			},
		},
	}

	t.Run("CmdDefault_ExitCode3OnWarnings", func(t *testing.T) {
		cmd := NewCheckCmd()
		if err := cmd.ParseFlags([]string{"--format", "json", "--output", outputPath}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		err := outputResults(cmd, ctx, warningOnlyResults, nil, nil, nil)
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 3 {
			t.Fatalf("expected exit code 3 without ignore flag, got: %v", err)
		}
	})

	t.Run("CmdIgnoreWarnings_ExitCode0OnWarnings", func(t *testing.T) {
		cmd := NewCheckCmd()
		if err := cmd.ParseFlags([]string{"--format", "json", "--output", outputPath, "--ignore-warnings"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		err := outputResults(cmd, ctx, warningOnlyResults, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected nil (exit code 0) with --ignore-warnings, got: %v", err)
		}
	})

	t.Run("CmdFailLevelError_ExitCode0OnWarnings", func(t *testing.T) {
		cmd := NewCheckCmd()
		if err := cmd.ParseFlags([]string{"--format", "json", "--output", outputPath, "--fail-level", "error"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		err := outputResults(cmd, ctx, warningOnlyResults, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected nil (exit code 0) with --fail-level error, got: %v", err)
		}
	})

	t.Run("CmdFailLevelWarning_ExitCode3OnWarnings", func(t *testing.T) {
		cmd := NewCheckCmd()
		if err := cmd.ParseFlags([]string{"--format", "json", "--output", outputPath, "--fail-level", "warning"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		err := outputResults(cmd, ctx, warningOnlyResults, nil, nil, nil)
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 3 {
			t.Fatalf("expected exit code 3 with --fail-level warning, got: %v", err)
		}
	})

	t.Run("CmdIgnoreWarnings_ExitCode1OnErrors", func(t *testing.T) {
		cmd := NewCheckCmd()
		if err := cmd.ParseFlags([]string{"--format", "json", "--output", outputPath, "--ignore-warnings"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		err := outputResults(cmd, ctx, errorResults, nil, nil, nil)
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 1 {
			t.Fatalf("expected exit code 1 with errors despite --ignore-warnings, got: %v", err)
		}
	})

	t.Run("CmdFailLevelError_ExitCode1OnErrors", func(t *testing.T) {
		cmd := NewCheckCmd()
		if err := cmd.ParseFlags([]string{"--format", "json", "--output", outputPath, "--fail-level", "error"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		err := outputResults(cmd, ctx, errorResults, nil, nil, nil)
		var scErr *SystemCheckError
		if !errors.As(err, &scErr) || scErr.ExitCode() != 1 {
			t.Fatalf("expected exit code 1 with errors despite --fail-level error, got: %v", err)
		}
	})
}
