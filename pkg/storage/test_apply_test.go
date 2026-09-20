package storage

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestApplyMaturationReport(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	fs, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = fs.Shutdown(context.Background()) }()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	tx, err := fs.BeginTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}

	obj := map[string]any{
		objects.FieldKeyID:                  "MAT-74200",
		objects.FieldKeyKind:                "maturation_report",
		objects.FieldKeyTitle:               "Test Maturation Report",
		objects.FieldKeyFitnessScore:        float64(85),
		objects.FieldKeyComponentID:         "COMP-12345",
		objects.FieldKeyObservationDuration: "30d",
		objects.FieldKeyGraduationStatus:    objects.ObjectStatusPromoted,
	}
	if err := tx.Create(ctx, secCtx, obj); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	_ = EnsureCLIObjectMutationVisibleForProvider(ctx, fs, tmpDir, []string{"maturation_report"})

	// Read
	_, err = fs.Read(ctx, secCtx, "MAT-74200")
	if err != nil {
		t.Fatalf("failed to read object MAT-74200: %v", err)
	}
}
