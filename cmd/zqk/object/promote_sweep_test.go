package object

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	testSampleTitle       = "Test Sweep Sample Object"
	testSampleDescription = "Testing post-promotion clean sweep and atomic CAS direct origination."
)

func TestDirectCASOrigination(t *testing.T) {
	env := SetupTestEnvironment(t)
	projectRoot := env.TestRoot

	stor, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Direct CAS create using WithPromoteOnCreate
	objData := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       testSampleTitle,
		objects.FieldKeyDescription: testSampleDescription,
		"category":                  "acceptance",
	}

	createCtx := pkgctx.WithPromoteOnCreate(ctx)
	if err := stor.Create(createCtx, secCtx, objData); err != nil {
		t.Fatalf("direct CAS create failed: %v", err)
	}

	id, _ := objData[objects.FieldKeyID].(string)
	if id == "" {
		t.Fatalf("expected non-empty ID")
	}

	// Verify object exists in CAS
	readObj, err := stor.Read(ctx, secCtx, id)
	if err != nil || readObj == nil {
		t.Fatalf("expected object to exist in CAS: %v", err)
	}

	// Verify object does NOT exist on the draft plane
	if storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, id) {
		t.Fatalf("expected object %s NOT to exist on draft plane after direct CAS origination", id)
	}
}

func TestPromoteSweepsDraftFile(t *testing.T) {
	env := SetupTestEnvironment(t)
	projectRoot := env.TestRoot

	id := "BLI-SAMPLE-SWEEP-001"
	draftPath := storage.ObjectDraftPlanePath(projectRoot, objects.KindBacklogItem, id)
	_ = fileutil.MkdirAll(filepath.Dir(draftPath), 0755)
	if err := fileutil.WriteFile(draftPath, []byte("title: test"), 0644); err != nil {
		t.Fatalf("failed to write draft file: %v", err)
	}

	if !storage.ObjectDraftPlaneExists(projectRoot, objects.KindBacklogItem, id) {
		t.Fatalf("expected draft file to exist before sweep")
	}

	// 2. Perform delete/sweep of the draft file
	if err := storage.DeleteObjectDraftFile(projectRoot, objects.KindBacklogItem, id); err != nil {
		t.Fatalf("DeleteObjectDraftFile failed: %v", err)
	}

	// 3. Verify draft file no longer exists
	if _, statErr := fileutil.Stat(draftPath); !fileutil.IsNotExist(statErr) {
		t.Fatalf("expected draft file at %s to be removed", draftPath)
	}

	if storage.ObjectDraftPlaneExists(projectRoot, objects.KindBacklogItem, id) {
		t.Fatalf("expected ObjectDraftPlaneExists to report false")
	}
}
