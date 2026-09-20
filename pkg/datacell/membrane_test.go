package datacell

import (
	"context"
	"testing"
)

type stubReadModel struct {
	rev uint64
}

func (s stubReadModel) BuiltAtSpecCacheRevision() uint64 { return s.rev }

func TestReadModelIsStale(t *testing.T) {
	t.Parallel()
	if !ReadModelIsStale(nil, 1) {
		t.Fatal("nil read model should be stale")
	}
	m := stubReadModel{rev: 5}
	if ReadModelIsStale(m, 5) {
		t.Fatal("same revision should not be stale")
	}
	if !ReadModelIsStale(m, 6) {
		t.Fatal("older built revision should be stale vs newer spec generation")
	}
}

func TestMaxSpecRevision(t *testing.T) {
	t.Parallel()
	if got := MaxSpecRevision(3, 7); got != 7 {
		t.Fatalf("MaxSpecRevision(3,7) = %d", got)
	}
	if got := MaxSpecRevision(9, 2); got != 9 {
		t.Fatalf("MaxSpecRevision(9,2) = %d", got)
	}
}

func TestMinimalMembrane_CoordinatorNilUsesNoop(t *testing.T) {
	t.Parallel()
	m := &MinimalMembrane{Profile: ProfileStream, Coord: nil}
	c := m.Coordinator()
	if c == nil {
		t.Fatal("Coordinator() returned nil")
	}
	if err := c.Enqueue(context.Background(), MaintenanceOp{Name: MaintenanceOpInvalidateCache}); err != nil {
		t.Fatal(err)
	}
}

func TestNoopCellCoordinator_Enqueue(t *testing.T) {
	t.Parallel()
	var c NoopCellCoordinator
	if err := c.Enqueue(context.Background(), MaintenanceOp{Name: "x", Detail: "y"}); err != nil {
		t.Fatal(err)
	}
}

func TestMembraneReadPaths_CLIHookProfilePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := &MinimalMembrane{Profile: ProfileStream, Coord: nil}
	p := MembraneReadPaths{Membrane: m, ProjectRoot: root}
	got := p.CLIHookProfilePath()
	want := CLIHookProfilePath(root)
	if got != want {
		t.Fatalf("CLIHookProfilePath() = %q want %q", got, want)
	}
	var zero MembraneReadPaths
	if zero.CLIHookProfilePath() != "" {
		t.Fatal("expected empty when membrane unset")
	}
}

func TestMembraneReadPaths_AllRuntimePaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := &MinimalMembrane{Profile: ProfileLightFile, Coord: nil}
	p := MembraneReadPaths{Membrane: m, ProjectRoot: root}
	got := p.AllRuntimePaths()
	want := AllRuntimePaths(root)
	if got != want {
		t.Fatalf("AllRuntimePaths() = %#v want %#v", got, want)
	}
	var zero MembraneReadPaths
	if zero.AllRuntimePaths() != (RuntimePaths{}) {
		t.Fatal("expected zero RuntimePaths when membrane unset")
	}
}

func TestRuntimeOrganismMembraneReadPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := RuntimeOrganismMembraneReadPaths(root)
	mm, ok := p.Membrane.(*MinimalMembrane)
	if !ok || mm.Profile != ProfileLightFile {
		t.Fatalf("want MinimalMembrane light_file, got %#v", p.Membrane)
	}
	if got := p.AllRuntimePaths(); got != AllRuntimePaths(root) {
		t.Fatalf("AllRuntimePaths() = %#v want %#v", got, AllRuntimePaths(root))
	}
}

func TestStreamMembraneReadPaths_StreamCurrentKindDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	kind := "audit_event"
	p := StreamMembraneReadPaths(root)
	if got := p.StreamCurrentKindDir(kind); got != StreamCurrentKindDir(root, kind) {
		t.Fatalf("StreamCurrentKindDir: got %q want %q", got, StreamCurrentKindDir(root, kind))
	}
}

func TestStreamMembraneReadPathsWithCoordinator(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cc := &countingCoordinator{}
	p := StreamMembraneReadPathsWithCoordinator(root, cc)
	mm, ok := p.Membrane.(*MinimalMembrane)
	if !ok || mm.Profile != ProfileStream {
		t.Fatalf("want MinimalMembrane stream, got %#v", p.Membrane)
	}
	if err := mm.Coordinator().Enqueue(context.Background(), MaintenanceOp{Name: MaintenanceOpRefreshSummary}); err != nil {
		t.Fatal(err)
	}
	if cc.calls != 1 {
		t.Fatalf("coordinator calls: %d", cc.calls)
	}
}

func TestCASEntityMembraneReadPaths_PrimaryDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := CASEntityMembraneReadPaths(root)
	if got := p.CASEntityPrimaryDir("backlog"); got != CASEntityPrimaryDir(root, "backlog") {
		t.Fatalf("CASEntityPrimaryDir: got %q want %q", got, CASEntityPrimaryDir(root, "backlog"))
	}
}
