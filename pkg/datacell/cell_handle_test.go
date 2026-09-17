package datacell

import (
	"context"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestNewCellHandle_validation(t *testing.T) {
	t.Parallel()
	if _, err := NewCellHandle("", ProfileStream, nil); err == nil {
		t.Fatal("want error")
	}
	if _, err := NewCellHandle(t.TempDir(), StorageProfile("x"), nil); err == nil {
		t.Fatal("want error")
	}
}

func TestCellHandle_EnqueueMaintenance_stewardCoordinatorPersistsJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	coord, err := NewStewardMaintenanceCoordinator(root, ProfileStream, nil)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewCellHandle(root, ProfileStream, coord)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := h.EnqueueMaintenance(ctx, MaintenanceOp{Name: MaintenanceOpInvalidateCache, Detail: "cell-handle"}); err != nil {
		t.Fatal(err)
	}
	b, err := fileutil.ReadFile(StewardEnqueueJSONLPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), MaintenanceOpInvalidateCache) {
		t.Fatalf("steward jsonl: %q", string(b))
	}
	rp := h.MembraneReadPaths()
	if !rp.active() {
		t.Fatal("membrane inactive")
	}
}

func TestCellHandle_MembraneReadPaths_streamUsesCoordinator(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var called int
	coord := CoordinatorFunc(func(ctx context.Context, op MaintenanceOp) error {
		called++
		return nil
	})
	h, err := NewCellHandle(root, ProfileStream, coord)
	if err != nil {
		t.Fatal(err)
	}
	rp := h.MembraneReadPaths()
	mm, ok := rp.Membrane.(*MinimalMembrane)
	if !ok || mm.Coord == nil {
		t.Fatalf("membrane %#v", rp.Membrane)
	}
	_ = mm.Coord.Enqueue(context.Background(), MaintenanceOp{Name: MaintenanceOpRefreshSummary})
	if called != 1 {
		t.Fatalf("called %d", called)
	}
}

func TestMembraneReadPathsForProfile_lightFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rp := MembraneReadPathsForProfile(root, ProfileLightFile, nil)
	if !rp.active() || rp.Membrane.StorageProfile() != ProfileLightFile {
		t.Fatalf("paths %#v", rp)
	}
}

func TestMembraneReadPathsForProfile_cas(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rp := MembraneReadPathsForProfile(root, ProfileCASEntity, nil)
	if !rp.active() || rp.Membrane.StorageProfile() != ProfileCASEntity {
		t.Fatalf("paths %#v", rp)
	}
}

// CoordinatorFunc adapts a function to [CellCoordinator] for tests.
type CoordinatorFunc func(ctx context.Context, op MaintenanceOp) error

func (f CoordinatorFunc) Enqueue(ctx context.Context, op MaintenanceOp) error {
	if f == nil {
		return nil
	}
	return f(ctx, op)
}
