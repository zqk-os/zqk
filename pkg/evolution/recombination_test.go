// Traceability: BLI-SYM-011, BLI-SYM-013, REQ-SYM-007
package evolution

import (
	"context"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestAutonomousSkillComposer_Recombine(t *testing.T) {
	composer := NewAutonomousSkillComposer()
	ctx := context.Background()

	t.Run("successful recombination", func(t *testing.T) {
		skillA := map[string]any{
			objects.FieldKeyKind:         "skill",
			objects.FieldKeyName:         "Coding",
			objects.FieldKeyDescription:  "Writes code.",
			objects.FieldKeyInstructions: "Write fast code.",
		}
		skillB := map[string]any{
			objects.FieldKeyKind:         "skill",
			objects.FieldKeyName:         "Testing",
			objects.FieldKeyDescription:  "Tests code.",
			objects.FieldKeyInstructions: "Write good tests.",
		}

		result, err := composer.Recombine(ctx, skillA, skillB)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result == nil {
			t.Fatal("expected non-nil result")
		}

		name, _ := result[objects.FieldKeyName].(string)
		if name != "Coding+Testing" {
			t.Errorf("expected name Coding+Testing, got %s", name)
		}

		desc, _ := result[objects.FieldKeyDescription].(string)
		if !strings.Contains(desc, "Writes code.") || !strings.Contains(desc, "Tests code.") {
			t.Errorf("description does not contain parent descriptions: %s", desc)
		}

		instr, _ := result[objects.FieldKeyInstructions].(string)
		if !strings.Contains(instr, "Write fast code.") || !strings.Contains(instr, "Write good tests.") {
			t.Errorf("instructions do not contain parent instructions: %s", instr)
		}

		isAutonomous, _ := result["autonomous_recombined"].(bool)
		if !isAutonomous {
			t.Errorf("expected autonomous_recombined to be true")
		}

		parents, _ := result["parent_skills"].([]string)
		if len(parents) != 2 || parents[0] != "Coding" || parents[1] != "Testing" {
			t.Errorf("expected parent_skills to be [Coding, Testing], got %v", parents)
		}
	})

	t.Run("nil skill handling", func(t *testing.T) {
		skillA := map[string]any{
			objects.FieldKeyKind: "skill",
			objects.FieldKeyName: "Coding",
		}

		_, err := composer.Recombine(ctx, skillA, nil)
		if err == nil {
			t.Error("expected error when skillB is nil")
		}

		_, err = composer.Recombine(ctx, nil, skillA)
		if err == nil {
			t.Error("expected error when skillA is nil")
		}
	})

	t.Run("missing fields recombination", func(t *testing.T) {
		skillA := map[string]any{}
		skillB := map[string]any{}

		result, err := composer.Recombine(ctx, skillA, skillB)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		name, _ := result[objects.FieldKeyName].(string)
		if name != "UnknownA+UnknownB" {
			t.Errorf("expected name UnknownA+UnknownB, got %s", name)
		}

		instr, _ := result[objects.FieldKeyInstructions].(string)
		if !strings.Contains(instr, "placeholder") {
			t.Errorf("expected placeholder instructions, got %s", instr)
		}
	})
}
