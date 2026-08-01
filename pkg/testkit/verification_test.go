package testkit

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestSignTestCaseCompletion(t *testing.T) {
	opts := &IsolatedTempProjectOptions{}
	p := PrepareIsolatedTempProject(t, opts)
	_ = testenvroot.CopyObjectSpecsFromProject(p.Root, "../..")

	ctx := context.Background()
	secCtx := pkgctx.NewSecurityContext("account:system", []string{"admin"}, []string{"read:*", "write:*"})

	// Create a test case object
	testCase := map[string]any{
		objects.FieldKeyKind:   "test_case",
		objects.FieldKeyID:     "TEST-001",
		objects.FieldKeyStatus: "planned",
	}
	err := p.FileStorage.Create(ctx, secCtx, testCase)
	if err != nil {
		t.Fatalf("failed to create test_case: %v", err)
	}

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
	err = SignTestCaseCompletion(ctx, "TEST-001", []string{"test_file_1.go", "test_file_2.go"})
	if err != nil {
		t.Fatalf("SignTestCaseCompletion failed: %v", err)
	}

	// Verify the object was updated using a new storage instance to bypass cache
	newStore, err := storage.NewFileObjectStorageForTest(p.Root)
	if err != nil {
		t.Fatalf("failed to create new store: %v", err)
	}
	obj, err := newStore.Read(ctx, secCtx, "TEST-001")
	if err != nil {
		t.Fatalf("failed to read test_case: %v", err)
	}

	if status := objects.GetString(obj, objects.FieldKeyStatus); status != "complete" {
		t.Errorf("expected status 'complete', got %q", status)
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
