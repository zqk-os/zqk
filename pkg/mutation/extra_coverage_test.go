// BLI-STARTER-COMMUNITY-034 / PRI-STARTER-COMMUNITY-034 coverage elevation
package mutation

import (
	"context"
	"errors"
	"testing"
)

type stubGate struct{ err error }

func (s stubGate) HilGate(context.Context, string, *Mutation) error { return s.err }

func TestMutation_FromMapAndIDs(t *testing.T) {
	t.Parallel()
	if OutputSchema() == "" {
		t.Fatal("schema")
	}
	mut, err := FromMap(map[string]any{
		"action":      string(ActionCreateNode),
		"target_kind": "backlog_item",
		"target_id":   "BLI-1",
	})
	if err != nil || mut.Action != ActionCreateNode {
		t.Fatalf("%+v %v", mut, err)
	}
	key := BuildIDKey("task", &mut)
	if key == "" {
		t.Fatal("id key")
	}
	if mut.ResultHashHint() == "" {
		t.Fatal("hash")
	}
}

func TestValidator_HilRoutes(t *testing.T) {
	t.Parallel()
	v := NewValidator(nil)
	ok := &Mutation{SafetyClass: SafetyWrite}
	if err := v.ValidateAndRoute(context.Background(), "t", ok); err != nil {
		t.Fatal(err)
	}
	need := &Mutation{SafetyClass: SafetyDestructive}
	if err := v.ValidateAndRoute(context.Background(), "t", need); err == nil {
		t.Fatal("nil gate")
	}
	v = NewValidator(stubGate{err: errors.New("deny")})
	if err := v.ValidateAndRoute(context.Background(), "t", need); err == nil {
		t.Fatal("gate err")
	}
	v = NewValidator(stubGate{})
	if err := v.ValidateAndRoute(context.Background(), "t", &Mutation{SafetyClass: SafetyHilRequired}); err != nil {
		t.Fatal(err)
	}
}
