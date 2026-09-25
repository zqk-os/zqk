package storage

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestFileObjectTransaction_AtomicRollback(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	fs, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fs.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, fs)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Pre-create an existing object A
	objA := map[string]any{
		objects.FieldKeyID:                  "MAT-74201",
		objects.FieldKeyKind:                "maturation_report",
		objects.FieldKeyTitle:               "Original Maturation Report A",
		objects.FieldKeyFitnessScore:        float64(70),
		objects.FieldKeyComponentID:         "COMP-00001",
		objects.FieldKeyObservationDuration: "10d",
		objects.FieldKeyGraduationStatus:    objects.ObjectStatusPromoted,
	}
	if err := fs.Create(ctx, secCtx, objA); err != nil {
		t.Fatalf("failed to pre-create objA: %v", err)
	}

	// 2. Begin a multi-operation transaction
	tx, err := fs.BeginTransaction(ctx)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	fileTx, ok := tx.(*FileObjectTransaction)
	if !ok {
		t.Fatalf("expected *FileObjectTransaction, got %T", tx)
	}

	// Queue Update for objA
	updateData := map[string]any{
		objects.FieldKeyTitle: "Mutated Title A",
	}
	if err := tx.Update(ctx, secCtx, "MAT-74201", updateData); err != nil {
		t.Fatalf("failed to queue update for objA: %v", err)
	}

	// Queue Create for objB
	objB := map[string]any{
		objects.FieldKeyID:                  "MAT-74202",
		objects.FieldKeyKind:                "maturation_report",
		objects.FieldKeyTitle:               "New Maturation Report B",
		objects.FieldKeyFitnessScore:        float64(90),
		objects.FieldKeyComponentID:         "COMP-00002",
		objects.FieldKeyObservationDuration: "20d",
		objects.FieldKeyGraduationStatus:    objects.ObjectStatusPromoted,
	}
	if err := tx.Create(ctx, secCtx, objB); err != nil {
		t.Fatalf("failed to queue create for objB: %v", err)
	}

	// Queue an operation that will fail during storage.Create
	fileTx.ops = append(fileTx.ops, fileTransactionOp{
		opType: OpCreate,
		id:     "MAT-FAIL-001",
		kind:   "unregistered_invalid_kind_fails_create",
		obj: map[string]any{
			objects.FieldKeyID:   "MAT-FAIL-001",
			objects.FieldKeyKind: "unregistered_invalid_kind_fails_create",
		},
	})

	// 3. Attempt to apply the transaction through commitStageApplyPerOpMixed
	_, applyErr := fileTx.commitStageApplyPerOpMixed(ctx, secCtx)
	if applyErr == nil {
		t.Fatalf("expected applyErr on invalid objC, got nil")
	}

	// 4. Verify Atomic Rollback:
	// - ObjA must have its original title restored ("Original Maturation Report A")
	readA, err := fs.Read(ctx, secCtx, "MAT-74201")
	if err != nil {
		t.Fatalf("failed to read objA after rollback: %v", err)
	}
	if title, _ := readA[objects.FieldKeyTitle].(string); title != "Original Maturation Report A" {
		t.Fatalf("atomicity violation! objA title not reverted, got %q, want 'Original Maturation Report A'", title)
	}

	// - ObjB must have been deleted during rollback
	readB, err := fs.Read(ctx, secCtx, "MAT-74202")
	if err == nil && readB != nil {
		t.Fatalf("atomicity violation! objB was partially applied and not removed during rollback: readB = %#v", readB)
	}
	if !errors.Is(err, ErrObjectNotFound) && err != nil {
		t.Fatalf("expected ErrObjectNotFound for objB, got %v", err)
	}
}
