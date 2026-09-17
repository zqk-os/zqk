package whatsnext

import (
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestPeekCAPStage_DoesNotAdvance(t *testing.T) {
	root := t.TempDir()
	EnsureCAPStage(root, "cap_stage_grooming")
	if got := PeekCAPStage(root); got != "cap_stage_grooming" {
		t.Fatalf("peek1=%q", got)
	}
	if got := PeekCAPStage(root); got != "cap_stage_grooming" {
		t.Fatalf("peek2 advanced unexpectedly to %q", got)
	}
	b, err := fileutil.ReadFile(capCyclePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "cap_stage_grooming" {
		t.Fatalf("on disk=%q", string(b))
	}
}

func TestAdvanceCAPStage_MovesForward(t *testing.T) {
	root := t.TempDir()
	EnsureCAPStage(root, "cap_stage_grooming")
	next := AdvanceCAPStage(root, "cap_stage_grooming")
	if next != "cap_stage_orchestrating" {
		t.Fatalf("next=%q", next)
	}
	if PeekCAPStage(root) != "cap_stage_orchestrating" {
		t.Fatalf("peek after advance=%q", PeekCAPStage(root))
	}
}

func TestAdvanceCAPStage_NoOpWhenPointerMoved(t *testing.T) {
	root := t.TempDir()
	EnsureCAPStage(root, "cap_stage_grooming")
	EnsureCAPStage(root, "cap_stage_metrics")
	next := AdvanceCAPStage(root, "cap_stage_grooming")
	if next != "cap_stage_metrics" {
		t.Fatalf("expected hold on metrics, got %q", next)
	}
}

func TestSelectCAPInstruction_StarvePinsGrooming(t *testing.T) {
	root := t.TempDir()
	EnsureCAPStage(root, "cap_stage_metrics")
	got := SelectCAPInstruction(root, map[string]int{})
	if got != capStageGrooming {
		t.Fatalf("got %q", got)
	}
	if PeekCAPStage(root) != capStageGrooming {
		t.Fatalf("disk=%q", PeekCAPStage(root))
	}
}

func TestSelectCAPInstruction_WithWorkPeeks(t *testing.T) {
	root := t.TempDir()
	EnsureCAPStage(root, "cap_stage_metrics")
	got := SelectCAPInstruction(root, map[string]int{"planned": 3})
	if got != "cap_stage_metrics" {
		t.Fatalf("got %q", got)
	}
}

func TestSwarmStarved(t *testing.T) {
	if !SwarmStarved(nil) || !SwarmStarved(map[string]int{"completed": 5}) {
		t.Fatal("expected starved")
	}
	if SwarmStarved(map[string]int{"planned": 1}) {
		t.Fatal("expected not starved")
	}
	if SwarmStarved(map[string]int{"validated": 2}) {
		t.Fatal("validated inventory should not count as starved")
	}
}
