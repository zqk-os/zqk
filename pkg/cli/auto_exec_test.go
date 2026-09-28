package cli

import (
	"context"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type autoExecMemStore struct {
	storage.ObjectStorageProvider
	mu   sync.Mutex
	objs map[string]map[string]any
}

func newAutoExecMemStore(items ...map[string]any) *autoExecMemStore {
	m := &autoExecMemStore{objs: make(map[string]map[string]any)}
	for _, it := range items {
		if id, ok := it[objects.FieldKeyID].(string); ok && id != "" {
			m.objs[id] = it
		}
	}
	return m
}

func (m *autoExecMemStore) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objs[id]
	if !ok {
		return nil, errfmt.Errorf("object %s not found", id)
	}
	cp := make(map[string]any, len(o))
	for k, v := range o {
		cp[k] = v
	}
	return cp, nil
}

func (m *autoExecMemStore) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objs[id]
	if !ok {
		return errfmt.Errorf("object %s not found", id)
	}
	for k, v := range updates {
		o[k] = v
	}
	return nil
}

func (m *autoExecMemStore) Create(ctx context.Context, sec *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		return errfmt.Errorf("missing id")
	}
	cp := make(map[string]any, len(obj))
	for k, v := range obj {
		cp[k] = v
	}
	m.objs[id] = cp
	return nil
}

func (m *autoExecMemStore) List(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []map[string]any
	for _, obj := range m.objs {
		if filter.Kind != "" && obj[objects.FieldKeyKind] != filter.Kind {
			continue
		}
		cp := make(map[string]any, len(obj))
		for k, v := range obj {
			cp[k] = v
		}
		out = append(out, cp)
	}
	return &storage.QueryResult{
		Objects: out,
	}, nil
}

func TestAutoExecPipeline_DiscoveryAndClaim(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	store := newAutoExecMemStore(
		map[string]any{
			objects.FieldKeyID:     "BLI-DISCOVER-001",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: "planned",
			objects.FieldKeyTitle:  "Discoverable Backlog Item",
			"priority":             "high",
		},
	)

	pipeline := NewAutoExecPipeline(store)
	opts := AutoExecOptions{
		Claimant:   "agent:test-seat",
		PersonaRef: "PER-COMMUNITY-SOFTWARE-ENGINEER",
		RunVerify:  false,
	}

	res, err := pipeline.Execute(ctx, sec, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TargetBLIID != "BLI-DISCOVER-001" {
		t.Fatalf("expected BLI-DISCOVER-001, got %q", res.TargetBLIID)
	}
	if res.Status != "in_progress" {
		t.Fatalf("expected status in_progress, got %q", res.Status)
	}
	if res.Claimant != "agent:test-seat" {
		t.Fatalf("expected claimant agent:test-seat, got %q", res.Claimant)
	}

	updated, err := store.Read(ctx, sec, "BLI-DISCOVER-001")
	if err != nil {
		t.Fatalf("failed to read updated BLI: %v", err)
	}
	if updated[objects.FieldKeyStatus] != "in_progress" {
		t.Errorf("BLI status = %v, want in_progress", updated[objects.FieldKeyStatus])
	}
	if updated[objects.FieldKeyClaimedBy] != "agent:test-seat" {
		t.Errorf("BLI claimed_by = %v, want agent:test-seat", updated[objects.FieldKeyClaimedBy])
	}
}

func TestAutoExecPipeline_TargetedBLI(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	store := newAutoExecMemStore(
		map[string]any{
			objects.FieldKeyID:     "BLI-TARGET-001",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: "planned",
			objects.FieldKeyTitle:  "Targeted Item",
		},
		map[string]any{
			objects.FieldKeyID:     "BLI-TARGET-002",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: "planned",
			objects.FieldKeyTitle:  "Other Item",
		},
	)

	pipeline := NewAutoExecPipeline(store)
	res, err := pipeline.Execute(ctx, sec, AutoExecOptions{
		TargetID: "BLI-TARGET-002",
		Claimant: "agent:seat-2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetBLIID != "BLI-TARGET-002" {
		t.Fatalf("expected BLI-TARGET-002, got %q", res.TargetBLIID)
	}
}

func TestAutoExecPipeline_DryRun(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	store := newAutoExecMemStore(
		map[string]any{
			objects.FieldKeyID:     "BLI-DRY-001",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: "planned",
			objects.FieldKeyTitle:  "Dry Run Item",
		},
	)

	pipeline := NewAutoExecPipeline(store)
	res, err := pipeline.Execute(ctx, sec, AutoExecOptions{
		TargetID: "BLI-DRY-001",
		DryRun:   true,
		Claimant: "agent:dry-seat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != "dry_run" {
		t.Fatalf("expected status dry_run, got %q", res.Status)
	}

	// Verify store was NOT updated
	original, _ := store.Read(ctx, sec, "BLI-DRY-001")
	if original[objects.FieldKeyStatus] != "planned" {
		t.Errorf("BLI was modified in dry-run mode: %v", original[objects.FieldKeyStatus])
	}
}

func TestAutoExecPipeline_VerificationAndLatch(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	store := newAutoExecMemStore(
		map[string]any{
			objects.FieldKeyID:     "BLI-LATCH-001",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: "in_progress",
			objects.FieldKeyTitle:  "Pipeline Latch Item",
			"criteria_refs":        []any{"CRIT-LATCH-001"},
		},
		map[string]any{
			objects.FieldKeyID:     "CRIT-LATCH-001",
			objects.FieldKeyKind:   objects.KindCriteria,
			objects.FieldKeyStatus: "originated",
			objects.FieldKeyTitle:  "Criteria to Latch",
		},
		map[string]any{
			objects.FieldKeyID:     "TST-LATCH-001",
			objects.FieldKeyKind:   objects.KindTestCase,
			objects.FieldKeyStatus: "originated",
			objects.FieldKeyTitle:  "Test Case to Verify",
			"criteria_refs":        []any{"CRIT-LATCH-001"},
			"backlog_item_refs":    []any{"BLI-LATCH-001"},
		},
	)

	pipeline := NewAutoExecPipeline(store)
	pipeline.Verifier = func(ctx context.Context, testID, pathOrID string) (bool, string, error) {
		return true, "mock test pass", nil
	}

	res, err := pipeline.Execute(ctx, sec, AutoExecOptions{
		TargetID:  "BLI-LATCH-001",
		Claimant:  "agent:test-seat",
		RunVerify: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "complete" {
		t.Fatalf("expected status complete, got %q", res.Status)
	}
	if len(res.VerifiedTestIDs) != 1 || res.VerifiedTestIDs[0] != "TST-LATCH-001" {
		t.Errorf("verified tests = %v, want [TST-LATCH-001]", res.VerifiedTestIDs)
	}
	if len(res.LatchedCritIDs) != 1 || res.LatchedCritIDs[0] != "CRIT-LATCH-001" {
		t.Errorf("latched criteria = %v, want [CRIT-LATCH-001]", res.LatchedCritIDs)
	}

	// Verify criteria transitioned to complete
	crit, _ := store.Read(ctx, sec, "CRIT-LATCH-001")
	if crit[objects.FieldKeyStatus] != "complete" {
		t.Errorf("criteria status = %v, want complete", crit[objects.FieldKeyStatus])
	}

	// Verify BLI transitioned to complete
	bli, _ := store.Read(ctx, sec, "BLI-LATCH-001")
	if bli[objects.FieldKeyStatus] != "complete" {
		t.Errorf("BLI status = %v, want complete", bli[objects.FieldKeyStatus])
	}
}
