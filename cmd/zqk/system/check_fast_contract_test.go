package system

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRefuseReducedSurfaceWithMutatingFlags(t *testing.T) {
	t.Parallel()

	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "check"}
		cmd.Flags().Bool(checkFlagFast, false, "")
		cmd.Flags().Bool(checkFlagCheckRefs, true, "")
		cmd.Flags().Bool(checkFlagAutoFix, false, "")
		cmd.Flags().Bool(checkFlagForce, false, "")
		return cmd
	}

	t.Run("allows_fast_alone", func(t *testing.T) {
		t.Parallel()
		cmd := newCmd()
		_ = cmd.Flags().Set(checkFlagFast, "true")
		if err := refuseReducedSurfaceWithMutatingFlags(cmd); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("allows_autofix_alone", func(t *testing.T) {
		t.Parallel()
		cmd := newCmd()
		_ = cmd.Flags().Set(checkFlagAutoFix, "true")
		if err := refuseReducedSurfaceWithMutatingFlags(cmd); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("coerces_fast_plus_autofix_to_full_surface", func(t *testing.T) {
		t.Parallel()
		cmd := newCmd()
		_ = cmd.Flags().Set(checkFlagFast, "true")
		_ = cmd.Flags().Set(checkFlagAutoFix, "true")
		if err := refuseReducedSurfaceWithMutatingFlags(cmd); err != nil {
			t.Fatalf("expected coerce, got %v", err)
		}
		if fast, _ := cmd.Flags().GetBool(checkFlagFast); fast {
			t.Fatal("expected --fast dropped when --auto-fix is set")
		}
		if refs, _ := cmd.Flags().GetBool(checkFlagCheckRefs); !refs {
			t.Fatal("expected --check-refs enabled when --auto-fix is set")
		}
	})

	t.Run("coerces_fast_plus_force_to_full_surface", func(t *testing.T) {
		t.Parallel()
		cmd := newCmd()
		_ = cmd.Flags().Set(checkFlagFast, "true")
		_ = cmd.Flags().Set(checkFlagForce, "true")
		if err := refuseReducedSurfaceWithMutatingFlags(cmd); err != nil {
			t.Fatalf("expected coerce, got %v", err)
		}
		if fast, _ := cmd.Flags().GetBool(checkFlagFast); fast {
			t.Fatal("expected --fast dropped when --force is set")
		}
	})

	t.Run("coerces_check_refs_false_plus_autofix", func(t *testing.T) {
		t.Parallel()
		cmd := newCmd()
		_ = cmd.Flags().Set(checkFlagCheckRefs, "false")
		_ = cmd.Flags().Set(checkFlagAutoFix, "true")
		if err := refuseReducedSurfaceWithMutatingFlags(cmd); err != nil {
			t.Fatalf("expected coerce, got %v", err)
		}
		if refs, _ := cmd.Flags().GetBool(checkFlagCheckRefs); !refs {
			t.Fatal("expected --check-refs re-enabled for --auto-fix")
		}
	})

	t.Run("allows_check_refs_true_plus_autofix", func(t *testing.T) {
		t.Parallel()
		cmd := newCmd()
		_ = cmd.Flags().Set(checkFlagCheckRefs, "true")
		_ = cmd.Flags().Set(checkFlagAutoFix, "true")
		if err := refuseReducedSurfaceWithMutatingFlags(cmd); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
}

func TestPrepareCompactCheckOutputData_partialCheck(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "check"}
	cmd.Flags().Bool(checkFlagFast, false, "")
	cmd.Flags().Bool(checkFlagCheckRefs, true, "")
	_ = cmd.Flags().Set(checkFlagFast, "true")

	raw, err := json.Marshal(PrepareCompactCheckOutputData(cmd, nil, 0, nil, t.TempDir()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		PartialCheck bool   `json:"partial_check"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.PartialCheck {
		t.Fatal("expected partial_check=true for --fast")
	}
	if !strings.Contains(got.Message, "PARTIAL CHECK") {
		t.Fatalf("expected PARTIAL CHECK message, got %q", got.Message)
	}
}

func TestPrepareCompactCheckOutputData_partialCheckViaCheckRefsFalse(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "check"}
	cmd.Flags().Bool(checkFlagFast, false, "")
	cmd.Flags().Bool(checkFlagCheckRefs, true, "")
	_ = cmd.Flags().Set(checkFlagCheckRefs, "false")

	raw, err := json.Marshal(PrepareCompactCheckOutputData(cmd, nil, 0, nil, t.TempDir()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		PartialCheck bool `json:"partial_check"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.PartialCheck {
		t.Fatal("expected partial_check=true for --check-refs=false")
	}
}
