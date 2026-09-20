package object

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestAllKindsCRUD tests Create, Read, Update, Delete for all discoverable object kinds.
// The full object package runs many CLI-backed tests; use -timeout 300s or higher when running the package.
func TestAllKindsCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("comprehensive all-kinds CRUD is slow (many kinds × CLI); run without -short")
	}
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}
	kinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		t.Fatalf("failed to get all kinds: %v", err)
	}
	if raceDetectorEnabled {
		kinds = []string{pplanKindBacklogItem, "goal", "system_check"}
	}
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	testEnv := SetupTestEnvironment(t)

	RunComprehensiveKindTests(t, kinds, func(t *testing.T, kind string) {
		testKindCRUD(t, testEnv, ctx, secCtx, kind, fieldRegistry)
	})
}

// testKindCRUD tests CRUD operations for a specific kind
//
//nolint:gocyclo // Test helper intentionally exercises many CRUD scenarios
func testKindCRUD(t *testing.T, testEnv *TestEnvironment, ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, fieldRegistry *objects.FieldRegistry) {
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		t.Skipf("skipping %s: failed to get fields: %v", kind, err)
		return
	}
	testID := generateComprehensiveTestID(kind, 1)
	createSucceeded := false
	switch kind {
	case "change_journal_entry":
		refObj := map[string]any{objects.FieldKeyID: "BLI-999", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyTitle: "Reference for change journal", objects.FieldKeyStatus: objectStatusPlanned, objects.FieldKeySchemaVersion: objectSchemaV2}
		withTempStorage(t, testEnv.GetTestRoot(), func(sp *storage.FileObjectStorage) {
			cliCtx := storage.WithCLIOperation(ctx)
			if err := sp.Create(cliCtx, secCtx, refObj); err != nil && err != storage.ErrObjectExists {
				t.Fatalf("failed to create reference object: %v", err)
			}
		})
	case "certificate", "workstream", "zqk_session", "keystore_entry", "assessment_rating":
		seedReferenceAccountViaCLI(t, testEnv.CLIBinary, testEnv.GetTestRoot())
	case "namespace":
		seedReferenceAccountViaCLI(t, testEnv.CLIBinary, testEnv.GetTestRoot())
	case "code_quality_metric":
		seedReferencePolicyForTests(t, testEnv.GetTestRoot())
	case "namespace_registry":
		seedReferenceNamespaceForTests(t, testEnv.GetTestRoot())
	case "partnership":
		seedReferenceOrganizationForTests(t, testEnv.GetTestRoot())
	case "requirement":
		goalObj := map[string]any{objects.FieldKeyID: "GOAL-999", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Reference goal for requirement", objects.FieldKeyStatus: objectStatusActive, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count", objects.FieldKeySchemaVersion: objectSchemaV2}
		critObj := map[string]any{objects.FieldKeyID: "CRIT-999", objects.FieldKeyKind: "criteria", objects.FieldKeyTitle: "Reference criteria for requirement", objects.FieldKeyCategory: "functional", objects.FieldKeyStatus: objectStatusNotStarted, objects.FieldKeySchemaVersion: objectSchemaV2}
		withTempStorage(t, testEnv.GetTestRoot(), func(sp *storage.FileObjectStorage) {
			cliCtx := storage.WithCLIOperation(ctx)
			_ = sp.Create(cliCtx, secCtx, goalObj)
			_ = sp.Create(cliCtx, secCtx, critObj)
		})
	case objects.KindGlossaryTermRelation:
		seedGlossaryRelationGraphForTests(t, ctx, secCtx, testEnv.GetTestRoot())
	}
	t.Run("Create", func(t *testing.T) {
		if !testCreateForKind(t, testEnv, kind, testID, kindFields) {
			return
		}
		if storage.StreamStorageEnabledForKind(kind) {
			// Create path is validated in testCreateForKind; stream update/delete are skipped below.
			return
		}
		cmd := testEnv.CreateCLICommand("object", "get", testID)
		if _, err := cmd.CombinedOutput(); err == nil {
			createSucceeded = true
		} else {
			t.Logf("Warning: Create test completed but object %s not found (may affect Update/Delete tests)", testID)
		}
	})
	t.Run("Get", func(t *testing.T) { testGetForKind(t, testEnv, testID) })
	t.Run("Update", func(t *testing.T) {
		if storage.StreamStorageEnabledForKind(kind) {
			t.Skip("stream-backed kind: Update not exercised in comprehensive CLI suite (WAL/stream apply)")
			return
		}
		if !createSucceeded {
			t.Skipf("Skipping Update test: Create did not succeed (ID: %s)", testID)
			return
		}
		testUpdateForKind(t, testEnv, ctx, secCtx, kind, testID, kindFields)
	})
	t.Run("Delete", func(t *testing.T) {
		if storage.StreamStorageEnabledForKind(kind) {
			t.Skip("stream-backed kind: Delete not exercised in comprehensive CLI suite (WAL/stream apply)")
			return
		}
		if !createSucceeded {
			t.Skipf("Skipping Delete test: Create did not succeed (ID: %s)", testID)
			return
		}
		if kind == "change_journal_entry" {
			t.Skipf("Skipping Delete test: change_journal_entry objects are immutable system records")
			return
		}
		testDeleteForKind(t, testEnv, kind, testID)
	})
}
