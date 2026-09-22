// BLI-STARTER-COMMUNITY-035 / PRI-STARTER-COMMUNITY-035 coverage elevation
package evolution

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestFitnessAssessor_CompareCapsAndMoreShadow(t *testing.T) {
	t.Parallel()
	a := NewFitnessAssessor(storage.NewNoopObjectStorage())
	ctx := context.Background()
	score, err := a.Compare(ctx, "c1", nil, nil)
	if err != nil || score != 0.5 {
		t.Fatalf("%v %v", score, err)
	}
	shadow := []infrastructure.Event{{ObjectID: "1"}, {ObjectID: "2"}, {ObjectID: "3"}}
	real := []infrastructure.Event{{ObjectID: "1"}}
	score, err = a.Compare(ctx, "c2", shadow, real)
	if err != nil || score < 0.5 {
		t.Fatalf("%v %v", score, err)
	}
	score, err = a.Compare(ctx, "c3", shadow, nil)
	if err != nil || score > 1.0 {
		t.Fatalf("cap %v %v", score, err)
	}
}

func TestComposerAndFissionMonitor(t *testing.T) {
	t.Parallel()
	c := NewAutonomousSkillComposer()
	if _, err := c.Recombine(context.Background(), nil, map[string]any{}); err == nil {
		t.Fatal("nil skill")
	}
	out, err := c.Recombine(context.Background(), map[string]any{objects.FieldKeyName: "a"}, map[string]any{objects.FieldKeyKind: "skill", objects.FieldKeyName: "b", objects.FieldKeyInstructions: "do"})
	if err != nil || out[objects.FieldKeyName] == nil {
		t.Fatalf("%v %v", out, err)
	}
	fc := NewFissionController(storage.NewNoopObjectStorage(), nil, nil)
	if err := fc.Monitor(context.Background()); err != nil {
		t.Fatal(err)
	}
	rm := NewReputationManager(storage.NewNoopObjectStorage(), nil, nil)
	if rm.GetDiscount(context.Background(), "e") != 0 {
		t.Fatal("discount")
	}
}
