package hostservice_test

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRootIDStableAcrossSamePath(t *testing.T) {
	t.Parallel()
	a := hostservice.RootID("/tmp/proj-a")
	b := hostservice.RootID("/tmp/proj-a")
	if a != b || a == "" {
		t.Fatalf("RootID unstable: %q vs %q", a, b)
	}
	c := hostservice.RootID("/tmp/proj-b")
	if a == c {
		t.Fatal("different roots must not share root_id")
	}
}

func TestRegistryUpsertFindOrphans(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone")
	present := filepath.Join(dir, "alive")
	if err := fileutil.MkdirAll(present, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := &hostservice.Registry{}
	reg.UpsertEntry(hostservice.Entry{
		RootID:       "r1",
		AbsRoot:      missing,
		UnitLabel:    "u1",
		DesiredState: hostservice.DesiredStateEnabled,
	})
	reg.UpsertEntry(hostservice.Entry{
		RootID:       "r2",
		AbsRoot:      present,
		UnitLabel:    "u2",
		DesiredState: hostservice.DesiredStateEnabled,
	})
	e, ok := reg.FindByRootID("r1")
	if !ok || e.AbsRoot != missing {
		t.Fatalf("FindByRootID failed: %+v", e)
	}
	// Orphans uses LoadRegistry from home — unit-test Upsert/Find only here.
	if _, ok := reg.FindByAbsRoot(present); !ok {
		t.Fatal("FindByAbsRoot present")
	}
}
