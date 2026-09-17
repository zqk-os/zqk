package testkit

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// schemaPlaneDirs are the four directories SeedSchemaPlane must populate. Listing them here
// rather than reusing the production slice means a future edit that drops one from the seeding
// loop fails this test instead of silently narrowing what callers receive.
var schemaPlaneDirs = []string{
	paths.ProcessInternalObjectSpecsDir,
	paths.ProcessInternalLifecyclesDir,
	paths.ProcessInternalTraitsDir,
	paths.ProcessInternalConfigsDir,
}

// TestSeedSchemaPlanePopulatesEveryPlaneDir is also the guard against reversed copy arguments.
// The helpers take (destination, source); swapping them copies the empty temp root over the
// real repository's specs, which is the defect recorded in BLI-REDACTED.
// Asserting that the temp root actually received files fails in that case, because a reversed
// copy leaves the temp root empty.
func TestSeedSchemaPlanePopulatesEveryPlaneDir(t *testing.T) {
	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{
		Kind:            "seed_plane",
		SeedSchemaPlane: true,
	})

	for _, dir := range schemaPlaneDirs {
		if countYAML(t, filepath.Join(proj.Root, dir)) == 0 {
			t.Errorf("%s: holds no YAML after seeding (reversed copy arguments look like this)", dir)
		}
	}
}

// TestSeedSchemaPlaneAllowsObjectCreate pins the behavior callers actually want. Specs alone
// produce a root where Create fails with "failed to read lifecycle file", so asserting on the
// files present is not enough: the plane has to be complete enough to write an object.
func TestSeedSchemaPlaneAllowsObjectCreate(t *testing.T) {
	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{
		Kind:            "seed_plane_create",
		SeedSchemaPlane: true,
	})
	if proj.FileStorage == nil {
		t.Fatal("expected file storage")
	}

	err := proj.FileStorage.Create(pkgctx.WithPromoteOnCreate(t.Context()), pkgctx.NewSystemSecurityContext(), map[string]any{
		objects.FieldKeyID:            "PER-seed-plane-test",
		objects.FieldKeyKind:          objects.KindPersona,
		objects.FieldKeyTitle:         "Seed plane persona",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	})
	if err != nil {
		t.Fatalf("create in seeded root: %v", err)
	}
}

// TestSeedSchemaPlaneRunsBeforeCallerStages protects the ordering the option depends on.
// Callers layer fixture-specific specs on top of the seeded copies, so seeding has to happen
// first; if it ran afterward it would overwrite the fixture a test just wrote.
func TestSeedSchemaPlaneRunsBeforeCallerStages(t *testing.T) {
	const fixtureName = "zzz_seed_order_probe.yaml"

	var sawSeededSpecs bool
	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{
		Kind:            "seed_plane_order",
		SeedSchemaPlane: true,
		AppendStagesBeforeStorage: func(root string) []NamedTestStep {
			return []NamedTestStep{{
				Name: "probe_seed_order",
				Fn: func() error {
					specs := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
					entries, err := fileutil.ReadDir(specs)
					sawSeededSpecs = err == nil && len(entries) > 0
					return fileutil.WriteFile(filepath.Join(specs, fixtureName), []byte("kind: object_spec\n"), 0o600)
				},
			}}
		},
	})

	if !sawSeededSpecs {
		t.Error("caller stage ran before seeding: specs were not present yet")
	}
	if _, err := fileutil.Stat(filepath.Join(proj.Root, paths.ProcessInternalObjectSpecsDir, fixtureName)); err != nil {
		t.Errorf("caller fixture did not survive seeding: %v", err)
	}
}

// TestDefaultRootSeedsOnlyHalfThePlane records what an unseeded root actually contains, which
// is what makes SeedSchemaPlane worth having.
//
// storage.ensureHermeticTestRootLayout backfills object_specs and lifecycles when it builds
// test storage, so those two look handled. It never copies traits or configs. That asymmetry is
// why the missing-lifecycle failures were confusing: they appeared only when the code under test
// built storage through the production constructor, which has no such net, and trait lookups in
// an isolated root quietly resolve against an empty directory either way.
//
// This test documents current behavior rather than endorsing it. If the backfill is ever widened
// or removed, it should fail and be updated deliberately.
func TestDefaultRootSeedsOnlyHalfThePlane(t *testing.T) {
	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{Kind: "seed_plane_default"})

	backfilled := map[string]bool{
		paths.ProcessInternalObjectSpecsDir: true,
		paths.ProcessInternalLifecyclesDir:  true,
		paths.ProcessInternalTraitsDir:      false,
		paths.ProcessInternalConfigsDir:     false,
	}
	for dir, wantPopulated := range backfilled {
		count := countYAML(t, filepath.Join(proj.Root, dir))
		if wantPopulated && count == 0 {
			t.Errorf("%s: expected the storage backfill to populate this, got 0 files", dir)
		}
		if !wantPopulated && count > 0 {
			t.Errorf("%s: now populated without SeedSchemaPlane (%d files); the backfill widened, "+
				"so this expectation needs revisiting", dir, count)
		}
	}
}

func countYAML(t *testing.T, dir string) int {
	t.Helper()
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return 0
	}
	var n int
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".yaml" {
			n++
		}
	}
	return n
}
