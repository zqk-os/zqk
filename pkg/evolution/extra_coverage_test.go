// BLI-STARTER-COMMUNITY-037 / PRI-STARTER-COMMUNITY-037 coverage elevation
package evolution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

type pulseSpine struct {
	infrastructure.SpinalSpine
	published []infrastructure.Event
	err       error
}

func (s *pulseSpine) Publish(_ context.Context, event infrastructure.Event) error {
	s.published = append(s.published, event)
	return s.err
}

type scoreStore struct {
	storage.NoopObjectStorage
	objs    map[string]map[string]any
	readErr error
}

func (s *scoreStore) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	if s.objs == nil {
		s.objs = map[string]map[string]any{}
	}
	if obj, ok := s.objs[id]; ok {
		return obj, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (s *scoreStore) Create(_ context.Context, _ *storage.SecurityContext, obj map[string]any) error {
	if s.objs == nil {
		s.objs = map[string]map[string]any{}
	}
	id, _ := obj[objects.FieldKeyID].(string)
	s.objs[id] = obj
	return nil
}

func (s *scoreStore) Update(_ context.Context, _ *storage.SecurityContext, id string, updates map[string]any) error {
	cur, ok := s.objs[id]
	if !ok {
		return storage.ErrObjectNotFound
	}
	for k, v := range updates {
		cur[k] = v
	}
	return nil
}

func TestExtraReputationAndFissionMonitor(t *testing.T) {
	ctx := context.Background()
	spine := &pulseSpine{}
	store := &scoreStore{}
	signer, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewReputationManager(store, spine, signer)

	if d := mgr.GetDiscount(ctx, "missing"); d != 0 {
		t.Fatalf("missing discount = %v", d)
	}
	if err := mgr.UpdateScore(ctx, "ent-1", 2, 3, 4, "", ""); err != nil {
		t.Fatal(err)
	}
	if store.objs["REP-ent-1"] == nil {
		t.Fatal("expected create path")
	}
	if d := mgr.GetDiscount(ctx, "ent-1"); d <= 0 {
		t.Fatalf("discount = %v", d)
	}

	store.objs["REP-ent-1"][objects.FieldKeyLastCalculationAt] = zqktime.NowRFC3339UTC()
	if err := mgr.UpdateScore(ctx, "ent-1", 1, 1, 1, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(spine.published) == 0 {
		t.Fatal("expected reputation pulse")
	}

	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	store.objs["REP-ent-1"][objects.FieldKeyLastCalculationAt] = old
	if err := mgr.UpdateScore(ctx, "ent-1", 0.2, 0.2, 0.2, "", ""); err != nil {
		t.Fatal(err)
	}

	if err := mgr.UpdateScore(ctx, "ent-2", 1, 1, 1, "bad-sig", "bad-key"); err == nil {
		t.Fatal("expected invalid signature")
	}

	unsigned := NewReputationManager(store, nil, nil)
	if err := unsigned.UpdateScore(ctx, "ent-3", 0, 0, 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if unsigned.GetDiscount(ctx, "ent-3") != 0 {
		t.Fatal("zero score should discount 0")
	}

	other := NewReputationManager(&scoreStore{readErr: errors.New("io")}, nil, nil)
	if err := other.UpdateScore(ctx, "x", 1, 1, 1, "", ""); err == nil {
		t.Fatal("expected read error")
	}

	fc := NewFissionController(store, spine, signer)
	if err := fc.Monitor(ctx); err != nil {
		t.Fatal(err)
	}
	store.objs["system_vitality"] = map[string]any{objects.FieldKeyProjectConfidenceScore: 90}
	if err := fc.Monitor(ctx); err != nil {
		t.Fatal(err)
	}
	fc.isDividing = true
	if err := fc.Trigger(ctx); err != nil {
		t.Fatal(err)
	}

	if score := mgr.calculateScore(-1, -1, -1); score != 0 {
		t.Fatalf("negative score = %v", score)
	}
	if score := mgr.calculateScore(1e9, 1e9, 1e9); score != 1 {
		t.Fatalf("capped score = %v", score)
	}

	assessor := NewFitnessAssessor(store)
	if _, err := assessor.Compare(ctx, "c1", nil, nil); err != nil {
		t.Fatal(err)
	}
	shadow := []infrastructure.Event{{ObjectID: "1"}, {ObjectID: "2"}}
	real := []infrastructure.Event{{ObjectID: "1"}}
	if _, err := assessor.Compare(ctx, "c2", shadow, real); err != nil {
		t.Fatal(err)
	}

	composer := NewAutonomousSkillComposer()
	got, err := composer.Recombine(ctx, map[string]any{objects.FieldKeyKind: "skill", objects.FieldKeyName: "A"}, map[string]any{objects.FieldKeyName: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if got[objects.FieldKeyKind] != "skill" {
		t.Fatalf("kind = %v", got[objects.FieldKeyKind])
	}
	got, err = composer.Recombine(ctx, map[string]any{objects.FieldKeyName: "A"}, map[string]any{objects.FieldKeyKind: "other", objects.FieldKeyName: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if got[objects.FieldKeyKind] != "other" {
		t.Fatalf("kindB fallback = %v", got[objects.FieldKeyKind])
	}
}
