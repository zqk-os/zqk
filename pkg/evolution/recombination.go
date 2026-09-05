// Traceability: BLI-SYM-011, BLI-SYM-013, REQ-SYM-007
package evolution

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// AutonomousSkillComposer defines the interface for combining two skills into a new one.
type AutonomousSkillComposer interface {
	Recombine(ctx context.Context, skillA, skillB map[string]any) (map[string]any, error)
}

// defaultSkillComposer is the default implementation of AutonomousSkillComposer.
type defaultSkillComposer struct{}

// NewAutonomousSkillComposer creates a new default AutonomousSkillComposer.
func NewAutonomousSkillComposer() AutonomousSkillComposer {
	return &defaultSkillComposer{}
}

// Recombine takes two skills and recombines them into a new skill.
func (c *defaultSkillComposer) Recombine(ctx context.Context, skillA, skillB map[string]any) (map[string]any, error) {
	if skillA == nil || skillB == nil {
		return nil, errfmt.Errorf("both skillA and skillB must be provided for recombination")
	}

	kindA, _ := skillA[objects.FieldKeyKind].(string)
	kindB, _ := skillB[objects.FieldKeyKind].(string)

	// In a real system, we might be more flexible, but for now we expect 'skill' kind.
	// We'll relax this check and just combine standard fields if they are missing.

	nameA, _ := skillA[objects.FieldKeyName].(string)
	if nameA == "" {
		nameA = "UnknownA"
	}
	nameB, _ := skillB[objects.FieldKeyName].(string)
	if nameB == "" {
		nameB = "UnknownB"
	}

	newID := fmt.Sprintf("skill-recombined-%d", time.Now().UnixNano())
	newName := fmt.Sprintf("%s+%s", nameA, nameB)

	// Recombine descriptions
	descA, _ := skillA[objects.FieldKeyDescription].(string)
	descB, _ := skillB[objects.FieldKeyDescription].(string)
	newDesc := fmt.Sprintf("Autonomously recombined skill. Derived from: %s and %s", descA, descB)

	// Combine components if they have an instructions field or similar
	instructionsA, _ := skillA[objects.FieldKeyInstructions].(string)
	instructionsB, _ := skillB[objects.FieldKeyInstructions].(string)

	newInstructions := ""
	if instructionsA != "" {
		newInstructions += "=== Inherited from " + nameA + " ===\n" + instructionsA + "\n"
	}
	if instructionsB != "" {
		newInstructions += "=== Inherited from " + nameB + " ===\n" + instructionsB + "\n"
	}

	if newInstructions == "" {
		newInstructions = "Autonomously generated placeholder instructions for recombined skill."
	}

	newSkill := map[string]any{
		objects.FieldKeyID:           newID,
		objects.FieldKeyKind:         "skill", // or whatever the skill kind is
		objects.FieldKeyName:         newName,
		objects.FieldKeyDescription:  newDesc,
		objects.FieldKeyInstructions: newInstructions,
		"autonomous_recombined":      true,
		"parent_skills":              []string{nameA, nameB},
	}

	if kindA != "" {
		newSkill[objects.FieldKeyKind] = kindA
	} else if kindB != "" {
		newSkill[objects.FieldKeyKind] = kindB
	}

	return newSkill, nil
}
