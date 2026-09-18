package system

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestNewAlignCmd(t *testing.T) {
	t.Parallel()
	cmd := NewAlignCmd()
	if cmd == nil {
		t.Fatal("NewAlignCmd() returned nil")
	}
	if cmd.Use != "align" {
		t.Errorf("expected Use \"align\", got %q", cmd.Use)
	}
	if cmd.Short == emptyValue {
		t.Error("command should have a short description")
	}
}

func TestAlignCommandFlags(t *testing.T) {
	t.Parallel()
	cmd := NewAlignCmd()
	for _, name := range []string{"gaps", "goal", "score", "dashboard"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("command should have --%s flag", name)
		}
	}
}

func TestGoalRefsFromObject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		obj  map[string]any
		want int
	}{
		{"nil goal_refs", map[string]any{objects.FieldKeyID: "BLI-1"}, 0},
		{"empty slice", map[string]any{objects.FieldKeyGoalRefs: []any{}}, 0},
		{"one ref", map[string]any{objects.FieldKeyGoalRefs: []any{"GOAL-001"}}, 1},
		{"two refs", map[string]any{objects.FieldKeyGoalRefs: []any{"GOAL-001", "GOAL-002"}}, 2},
		{"string slice", map[string]any{objects.FieldKeyGoalRefs: []string{"GOAL-A"}}, 1},
		{"empty string omitted", map[string]any{objects.FieldKeyGoalRefs: []any{"GOAL-001", ""}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := GoalRefsFromObject(tt.obj)
			if len(refs) != tt.want {
				t.Errorf("goalRefsFromObject() got %d refs, want %d", len(refs), tt.want)
			}
		})
	}
}

func TestRoundTwo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   float64
		want float64
	}{
		{0, 0},
		{17.5, 17.5},
		{17.501, 17.5},
		{17.505, 17.51},
		{17.509, 17.51},
		{100, 100},
	}
	for _, tt := range tests {
		got := RoundTwo(tt.in)
		if got != tt.want {
			t.Errorf("roundTwo(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
