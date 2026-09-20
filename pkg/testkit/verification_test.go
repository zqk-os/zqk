package testkit

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSignTestCaseCompletion(t *testing.T) {
	opts := &IsolatedTempProjectOptions{}
	p := PrepareIsolatedTempProject(t, opts)
	_ = testenvroot.CopyObjectSpecsFromProject(p.Root, "../..")

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// test_case has no planned; origin is draft, CAS-visible hop is active
	// (needs path_or_id + scope). TRACK: BLI-1785443942668406000-1ec5c811
	testCase := map[string]any{
		objects.FieldKeyKind:     objects.KindTestCase,
		objects.FieldKeyID:       "TEST-001",
		objects.FieldKeyTitle:    "Test case for sign completion",
		objects.FieldKeyStatus:   objects.ObjectStatusDraft,
		objects.FieldKeyPathOrID: "test_file_1.go",
		objects.FieldKeyScope:    "unit",
	}
	storage.CreateCASVisible(t, p.FileStorage, ctx, secCtx, testCase, objects.ObjectStatusActive)

	// Create some artifacts
	art1 := filepath.Join(p.Root, "test_file_1.go")
	art2 := filepath.Join(p.Root, "test_file_2.go")
	if err := fileutil.WriteStandardFile(art1, []byte("content 1")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(art2, []byte("content 2")); err != nil {
		t.Fatal(err)
	}

	// Run function under test
	err := SignTestCaseCompletion(ctx, "TEST-001", []string{"test_file_1.go", "test_file_2.go"})
	if err != nil {
		t.Fatalf("SignTestCaseCompletion failed: %v", err)
	}

	// Verify the object was updated using a new storage instance to bypass cache
	newStore, err := storage.NewFileObjectStorageForTest(p.Root)
	if err != nil {
		t.Fatalf("failed to create new store: %v", err)
	}
	RegisterStorageTestCleanup(t, p.Root, newStore)
	obj, err := newStore.Read(ctx, secCtx, "TEST-001")
	if err != nil {
		t.Fatalf("failed to read test_case: %v", err)
	}

	if status := objects.GetString(obj, objects.FieldKeyStatus); status != objects.ObjectStatusComplete {
		t.Errorf("expected status %q, got %q", objects.ObjectStatusComplete, status)
	}

	refs, ok := obj[objects.FieldKeyVerifiedArtifactRefs].([]any)
	if !ok || len(refs) != 2 {
		t.Errorf("expected verified_artifact_refs length 2, got %v", refs)
	}

	hash := objects.GetString(obj, "verification_hash")
	if hash == "" {
		t.Errorf("expected verification_hash to be set, got empty string")
	}
}

// tdd refresh
