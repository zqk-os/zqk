package storage

import (
	"fmt"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestCriteriaCreateParksWithoutCategory_CASMembraneRefuses(t *testing.T) {
	testRoot := setupCriteriaTestRoot(t)
	fos, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	defer func() { _ = fos.Shutdown(t.Context()) }()
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(testRoot, fos)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := t.Context()
	sys := pkgctx.NewSystemSecurityContext()

	t.Run("create without category parks on draft plane", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-nocat%d", time.Now().UnixNano())
		if err := fos.Create(ctx, sys, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          objects.KindCriteria,
			objects.FieldKeyTitle:         "no category " + id,
			objects.FieldKeyDescription:   "Substantive criteria description for testing category minting.",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create without category should succeed (draft plane): %v", err)
		}
		if !fos.objectDraftPlaneExists(objects.KindCriteria, id) {
			t.Fatal("expected draft-plane park; create must not write hash CAS")
		}
		if cas, casErr := fos.getContentAddressableStorage(objects.KindCriteria); casErr == nil && cas != nil {
			if _, hashErr := cas.GetHashForID(id); hashErr == nil {
				t.Fatal("create without category must not register a CAS id→hash")
			}
		}
	})

	t.Run("CAS membrane refuses promote without category", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-prom%d", time.Now().UnixNano())
		promoteCtx := pkgctx.WithPromoteOnCreate(ctx)
		err := fos.Create(promoteCtx, sys, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          objects.KindCriteria,
			objects.FieldKeyTitle:         "promote no category " + id,
			objects.FieldKeyDescription:   "Substantive criteria description for testing category minting.",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		})
		if err == nil {
			t.Fatal("expected --promote / CAS persist without category to fail")
		}
		if !strings.Contains(err.Error(), objects.FieldKeyCategory) {
			t.Fatalf("CAS membrane error should name category, got %v", err)
		}
		if fos.objectDraftPlaneExists(objects.KindCriteria, id) {
			t.Fatal("refused CAS persist must not leave a draft ghost")
		}
	})

	t.Run("update with category leaves draft into CAS", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-fill%d", time.Now().UnixNano())
		if err := fos.Create(ctx, sys, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          objects.KindCriteria,
			objects.FieldKeyTitle:         "fill category " + id,
			objects.FieldKeyDescription:   "Substantive criteria description for testing category minting.",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := fos.Update(ctx, sys, id, map[string]any{objects.FieldKeyCategory: "acceptance"}); err != nil {
			t.Fatalf("update category (materialize): %v", err)
		}
		if fos.objectDraftPlaneExists(objects.KindCriteria, id) {
			t.Fatal("after category is set, object should leave the draft plane")
		}
		got, err := fos.Read(t.Context(), sys, id)
		if err != nil {
			t.Fatalf("read after materialize: %v", err)
		}
		if cat, _ := got[objects.FieldKeyCategory].(string); cat != "acceptance" {
			t.Fatalf("category=%v", got[objects.FieldKeyCategory])
		}
	})

	t.Run("remap type onto category", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-type%d", time.Now().UnixNano())
		if err := fos.Create(ctx, sys, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          objects.KindCriteria,
			objects.FieldKeyTitle:         "type alias " + id,
			objects.FieldKeyDescription:   "Substantive criteria description for testing category minting.",
			objects.FieldKeyType:          "acceptance",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create with type=: %v", err)
		}
		got, err := fos.Read(t.Context(), sys, id)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if cat, _ := got[objects.FieldKeyCategory].(string); cat != "acceptance" {
			t.Fatalf("category=%v", got[objects.FieldKeyCategory])
		}
		if _, ok := got[objects.FieldKeyType]; ok {
			t.Fatal("type alias should not persist")
		}
		if fos.objectDraftPlaneExists(objects.KindCriteria, id) {
			t.Fatal("categorized create (via type alias) should persist to CAS, not draft")
		}
	})

	t.Run("create with category writes hash CAS", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-hascat%d", time.Now().UnixNano())
		if err := fos.Create(ctx, sys, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          objects.KindCriteria,
			objects.FieldKeyTitle:         "has category " + id,
			objects.FieldKeyDescription:   "Substantive criteria description for testing category minting.",
			objects.FieldKeyCategory:      "acceptance",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create with category: %v", err)
		}
		if fos.objectDraftPlaneExists(objects.KindCriteria, id) {
			t.Fatal("categorized create should not park on draft")
		}
		if cas, casErr := fos.getContentAddressableStorage(objects.KindCriteria); casErr == nil && cas != nil {
			if _, hashErr := cas.GetHashForID(id); hashErr != nil {
				t.Fatalf("categorized create should register CAS id→hash: %v", hashErr)
			}
		}
	})

	t.Run("promote with type alias materializes", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-promtype%d", time.Now().UnixNano())
		if err := fos.Create(pkgctx.WithPromoteOnCreate(ctx), sys, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          objects.KindCriteria,
			objects.FieldKeyTitle:         "promote type " + id,
			objects.FieldKeyDescription:   "Substantive criteria description for testing category minting.",
			objects.FieldKeyType:          "compliance",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("promote with type=: %v", err)
		}
		if fos.objectDraftPlaneExists(objects.KindCriteria, id) {
			t.Fatal("promote with remapped category should write CAS")
		}
	})

	t.Run("title-only update cannot sneak uncategorized draft into CAS", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-sneak%d", time.Now().UnixNano())
		if err := fos.Create(ctx, sys, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          objects.KindCriteria,
			objects.FieldKeyTitle:         "sneak " + id,
			objects.FieldKeyDescription:   "Substantive criteria description for testing category minting.",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create: %v", err)
		}
		err := fos.Update(ctx, sys, id, map[string]any{objects.FieldKeyTitle: "still no category"})
		if err == nil {
			t.Fatal("expected update without category to refuse CAS materialize")
		}
		if !strings.Contains(err.Error(), objects.FieldKeyCategory) {
			t.Fatalf("refuse should name category, got %v", err)
		}
		if !fos.objectDraftPlaneExists(objects.KindCriteria, id) {
			t.Fatal("failed update must leave the object on the draft plane")
		}
	})

	t.Run("WriteObjectRaw without category parks draft", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-raw%d", time.Now().UnixNano())
		raw := []byte(fmt.Sprintf("id: %s\nkind: criteria\ntitle: raw %s\ndescription: Substantive criteria description for testing category minting.\nstatus: awaiting_verification\n", id, id))
		if err := fos.WriteObjectRaw(ctx, objects.KindCriteria, id, raw); err != nil {
			t.Fatalf("WriteObjectRaw: %v", err)
		}
		if !fos.objectDraftPlaneExists(objects.KindCriteria, id) {
			t.Fatal("raw write without category must park on draft")
		}
	})

	t.Run("remap criteria_type onto category", func(t *testing.T) {
		id := fmt.Sprintf("CRIT-ct%d", time.Now().UnixNano())
		if err := fos.Create(ctx, sys, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          objects.KindCriteria,
			objects.FieldKeyTitle:         "criteria_type alias " + id,
			objects.FieldKeyDescription:   "Substantive criteria description for testing category minting.",
			objects.FieldKeyCriteriaType:  "security",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create with criteria_type=: %v", err)
		}
		got, err := fos.Read(t.Context(), sys, id)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if cat, _ := got[objects.FieldKeyCategory].(string); cat != "security" {
			t.Fatalf("category=%v", got[objects.FieldKeyCategory])
		}
		if _, ok := got[objects.FieldKeyCriteriaType]; ok {
			t.Fatal("criteria_type alias should not persist")
		}
	})
}
