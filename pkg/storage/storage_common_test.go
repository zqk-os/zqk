package storage

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestEnsureObjectMetadata_legacySystemActorBecomesACC(t *testing.T) {
	t.Parallel()
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"write:*"})
	obj := map[string]any{
		objects.FieldKeyID:        "BLI-TEST-UPDATED-BY",
		objects.FieldKeyKind:      objects.KindBacklogItem,
		objects.FieldKeyCreatedBy: "system",
	}
	EnsureObjectMetadata(t.Context(), obj, secCtx, true, nil)
	gotUpdated, _ := obj[objects.FieldKeyUpdatedBy].(string)
	if gotUpdated != pkgctx.SystemAccountID {
		t.Fatalf("updated_by = %q, want %s", gotUpdated, pkgctx.SystemAccountID)
	}
	gotCreated, _ := obj[objects.FieldKeyCreatedBy].(string)
	if gotCreated != pkgctx.SystemAccountID {
		t.Fatalf("created_by = %q, want %s", gotCreated, pkgctx.SystemAccountID)
	}
}

func TestCheckPermission_legacySystemAlias(t *testing.T) {
	t.Parallel()
	secCtx := pkgctx.NewSecurityContext("system", nil, nil)
	if err := CheckPermission(secCtx, "write", objects.KindBacklogItem); err != nil {
		t.Fatalf("system alias should retain system write access: %v", err)
	}
}
