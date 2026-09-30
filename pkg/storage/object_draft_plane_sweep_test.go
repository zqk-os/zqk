package storage

import (
	"testing"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)


func TestSweepObjectDraftPlane_dryRunAndDelete(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "DOC-1777000000000000000-sweepplane01"
	if err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(id, "Sweep me", "draft")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	path := ObjectDraftPlanePath(tmpDir, objects.KindDocEntry, id)
	if _, err := fileutil.Stat(path); err != nil {
		t.Fatalf("draft missing: %v", err)
	}

	dry, err := SweepObjectDraftPlane(tmpDir, ObjectDraftPlaneSweepOptions{
		Kind: objects.KindDocEntry, DryRun: true, All: true,
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if dry.Matched < 1 || dry.Deleted != 0 {
		t.Fatalf("dry matched=%d deleted=%d", dry.Matched, dry.Deleted)
	}

	applied, err := SweepObjectDraftPlane(tmpDir, ObjectDraftPlaneSweepOptions{
		Kind: objects.KindDocEntry, DryRun: false, All: true,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Deleted < 1 {
		t.Fatalf("expected delete, got %+v", applied)
	}
	if _, err := fileutil.Stat(path); !fileutil.IsNotExist(err) {
		t.Fatalf("draft file should be gone, err=%v", err)
	}
}

func TestMatchObjectDraftPlane_filters(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "DOC-1777000000000000000-matchplane01"
	if err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(id, "Match me", "draft")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	matched, skipped, _, err := MatchObjectDraftPlane(tmpDir, ObjectDraftPlaneMatchOptions{
		Kind: objects.KindDocEntry, IDPrefix: "DOC-", Max: 1,
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(matched) != 1 || matched[0].ID != id {
		t.Fatalf("matched=%+v skipped=%+v", matched, skipped)
	}
}

func TestSweepObjectDraftPlane_reconcilesDualPlaneDraft(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()
	id := "DOC-1777000000000000000-dualplane01"

	CreateCASVisible(t, fileStorage, ctx, secCtx, draftPlaneDocEntry(id, "Materialized", "active"), "active")
	draftData, err := yaml.Marshal(draftPlaneDocEntry(id, "Stale draft", "draft"))
	if err != nil {
		t.Fatalf("marshal draft: %v", err)
	}
	if err := fileStorage.WriteObjectToDraftPlane(id, objects.KindDocEntry, draftData); err != nil {
		t.Fatalf("write duplicate draft: %v", err)
	}

	result, err := SweepObjectDraftPlane(tmpDir, ObjectDraftPlaneSweepOptions{
		Kind: objects.KindDocEntry, IDPrefix: id, Max: 1,
	})
	if err != nil {
		t.Fatalf("sweep dual-plane draft: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected one draft deletion, got %+v", result)
	}
	if _, err := fileStorage.Read(ctx, secCtx, id); err != nil {
		t.Fatalf("CAS object was removed with draft: %v", err)
	}
}
