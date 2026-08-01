package storage

import (
	"context"
	"os"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestApplyMaturationReport(t *testing.T) {
	tmpDir := setupCriteriaTestRoot(t)

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
		objects.FieldKeyGraduationStatus:    "promoted",
	}
	if err := tx.Create(ctx, secCtx, obj); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	EnsureCLIObjectMutationVisibleForProvider(ctx, fs, tmpDir, []string{"maturation_report"})

	// Read
	_, err = fs.Read(ctx, secCtx, "MAT-74200")
	t.Logf("Read err after Create+Commit+Wait: %v", err)
	out, _ := os.ReadFile(tmpDir + "/docs/architecture/maturation_reports/.maturation_report.index")
	t.Logf("Index file: %s", string(out))
}
