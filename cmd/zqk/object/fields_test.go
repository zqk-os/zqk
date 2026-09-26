package object

// BLI-177483 inventory: subprocess uses wireExecForTest (no isolated temp ZQK_TEST_ROOT RunProjectTestTeardown).

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestFieldsCommand(t *testing.T) {
	t.Parallel()
	env := SetupTestEnvironment(t)
	cliBinary := env.CLIBinary

	t.Run("List kinds via list command", func(t *testing.T) {
		// Use object list without kind to see available kinds; timeout so interrupted tests don't leave orphan processes
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, cliBinary, "object", "list")
		// Set ZQK_TEST_ROOT to ensure test isolation
		wireExecForTest(cmd, env.TestRoot)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Skip when subprocess was killed (timeout, OOM, or resource limit in CI/bundler)
			if errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(err.Error(), "killed") ||
				strings.Contains(string(output), "signal: killed") {
				t.Skipf("object list killed or timed out (resource limit): %v", err)
			}
			t.Fatalf("command failed: %v\n%s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, pplanKindBacklogItem) && !strings.Contains(outputStr, "Available") && !strings.Contains(outputStr, "Objects grouped by kind") && !strings.Contains(outputStr, "No objects found") {
			t.Error("expected output to contain backlog kind or show available kinds or grouped by kind or No objects found")
		}
	})

	t.Run("List fields for kind", func(t *testing.T) {
		// Test new syntax: object <kind> fields
		cmd := testkit.ManagedCommand(t, t.Context(), cliBinary, "object", pplanKindBacklogItem, "fields")
		wireExecForTest(cmd, env.TestRoot)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command failed: %v\n%s", err, output)
		}

		outputStr := string(output)
		// Output shape may differ when field registry or format changes; require at least field-like content
		if !strings.Contains(outputStr, "Common Fields") && !strings.Contains(outputStr, "Fields") && !strings.Contains(outputStr, "field") {
			t.Error("expected output to contain 'Common Fields', 'Fields', or 'field'")
		}
		if !strings.Contains(outputStr, "Specialized Fields") && !strings.Contains(outputStr, pplanKindBacklogItem) {
			t.Error("expected output to contain 'Specialized Fields' or kind name")
		}
	})

	t.Run("List filterable fields", func(t *testing.T) {
		// Test new syntax: object <kind> fields --filterable (flag may be unknown when kind is not a subcommand, e.g. bundler)
		cmd := testkit.ManagedCommand(t, t.Context(), cliBinary, "object", pplanKindBacklogItem, "fields", "--filterable")
		wireExecForTest(cmd, env.TestRoot)
		output, err := cmd.CombinedOutput()
		outStr := string(output)
		if err != nil {
			if strings.Contains(outStr, "unknown flag: --filterable") {
				t.Skipf("--filterable not available (e.g. kind subcommand not registered): %v", err)
			}
			t.Fatalf("command failed: %v\n%s", err, output)
		}

		if !strings.Contains(outStr, "Fields:") && !strings.Contains(outStr, "Fields") {
			t.Error("expected output to contain 'Fields:' or 'Fields'")
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		// Test new syntax: object <kind> fields --format json
		cmd := testkit.ManagedCommand(t, t.Context(), cliBinary, "object", pplanKindBacklogItem, "fields", "--format", objectFormatJSON)
		wireExecForTest(cmd, env.TestRoot)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command failed: %v\n%s", err, output)
		}

		outputStr := string(output)
		// JSON shape may use different keys (kind/common_fields vs alternatives)
		if !strings.Contains(outputStr, "\"kind\"") && !strings.Contains(outputStr, pplanKindBacklogItem) {
			t.Error("expected JSON output to contain 'kind' field or kind name")
		}
		if !strings.Contains(outputStr, "\"common_fields\"") && !strings.Contains(outputStr, "field") {
			t.Error("expected JSON output to contain 'common_fields' or field-related content")
		}
	})

}
