package organizational

import (
	"context"
	"strings"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	orgdomain "github.com/zqk-os/zqk/pkg/domain/organizational"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqktime"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Sample org structure YAML for integration test (minimal valid set; IDs match ^[A-Z]+-\d{3,}$).
const integrationOrgStructureYAML = `
- id: ORG-901
  kind: organization
  namespace_id: domain:organizational
  schema_version: "` + objects.DefaultSchemaVersion + `"
  title: Integration Org
  status: active
  domain: custom
  spec_interpreter: default
  spec_context_broker: default
  organization_name: Integration Org
  division_refs: []
- id: DIV-901
  kind: division
  namespace_id: domain:organizational
  schema_version: "` + objects.DefaultSchemaVersion + `"
  title: Engineering
  status: active
  domain: custom
  spec_interpreter: default
  spec_context_broker: default
  division_name: Engineering
  parent_division_ref: null
  team_refs: []
`

// TestOrganizationalSync_Integration runs sync (ImportObjects) then verifies objects exist.
func TestOrganizationalSync_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	testRoot, _ := setupIntegrationTestWithSpecsOrganizational(t, "organizational-sync-integration")

	// Use test storage (no write-behind) so ImportObjects writes synchronously and List sees objects.
	store, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, store)
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, store)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Sync: import org structure (same logic as organizational sync command)
	objSlice, err := unmarshalOrgSync([]byte(integrationOrgStructureYAML), storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("unmarshalOrgSync: %v", err)
	}
	var validated []map[string]any
	for _, obj := range objSlice {
		kind, _ := obj[objects.FieldKeyKind].(string)
		if kind == emptyValue || !orgSyncAllowedKinds[kind] {
			continue
		}
		validated = append(validated, obj)
	}
	if len(validated) == 0 {
		t.Fatal("no validated objects")
	}
	dataToImport, err := marshalOrgSync(validated, storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("marshalOrgSync: %v", err)
	}
	result, err := storage.ImportObjects(ctx, store, secCtx, dataToImport, storage.ExportFormatYAML, storage.ImportOptions{
		Mode:            storage.ImportModeUpsert,
		ContinueOnError: false,
	})
	if err != nil {
		if strings.Contains(err.Error(), "no builders found for ontology") {
			t.Skip("skipping integration test: organization builder is not in open-core bundle")
		}
		t.Fatalf("ImportObjects: %v", err)
	}
	if result.Created+result.Updated == 0 && result.Failed > 0 {
		t.Fatalf("ImportObjects: created=%d updated=%d failed=%d", result.Created, result.Updated, result.Failed)
	}
	// organization/division origin is shovel-ready active — create is already CAS-visible.
	// Flush CAS index so List sees the imported objects (Create uses write-behind for CAS kinds)
	queue := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot)
	for _, kind := range []string{"organization", "division"} {
		if err := queue.FlushKind(kind, 5*time.Second); err != nil {
			t.Fatalf("FlushKind %s: %v", kind, err)
		}
	}

	// Verify: list organization and division
	storageCtx := pkgctx.NewStorageContext()
	listOrg, err := store.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: "organization", Limit: 10})
	if err != nil {
		t.Fatalf("List organization: %v", err)
	}
	if len(listOrg.Objects) < 1 {
		t.Errorf("expected at least 1 organization after sync, got %d", len(listOrg.Objects))
	}
	listDiv, err := store.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: "division", Limit: 10})
	if err != nil {
		t.Fatalf("List division: %v", err)
	}
	if len(listDiv.Objects) < 1 {
		t.Errorf("expected at least 1 division after sync, got %d", len(listDiv.Objects))
	}
}

// TestOrganizationalAnalyzeImpact_Integration creates an organizational_change then runs AnalyzeChange and verifies impact_analysis.
func TestOrganizationalAnalyzeImpact_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	testRoot, _ := setupIntegrationTestWithSpecsOrganizational(t, "organizational-analyze-integration")

	store, err := storage.NewFileObjectStorage(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, store)
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, store)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create an organizational_change object (ID must match ^[A-Z]+-\d{3,}$)
	changeID := "OCH-001"
	changeObj := map[string]any{
		objects.FieldKeyID:                 changeID,
		objects.FieldKeyKind:               objects.KindOrganizationalChange,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:              "Integration test change",
		objects.FieldKeyStatus:             objects.ObjectStatusProposed,
		objects.FieldKeyChangeType:         "division_restructure",
		objects.FieldKeyChangeDescription:  "Test for analyze-impact",
		objects.FieldKeyAffectedObjects:    map[string]any{},
		objects.FieldKeyImpactAnalysisRefs: []any{},
	}
	opCtx := pkgctx.WithCacheUpdate(ctx, changeID, "organizational_change", "")
	if err := store.Create(opCtx, secCtx, changeObj); err != nil {
		t.Fatalf("Create organizational_change: %v", err)
	}
	// Flush CAS index so reference validation can resolve OCH-001 when creating impact_analysis
	queue := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot)
	if err := queue.FlushKind("organizational_change", 5*time.Second); err != nil {
		t.Fatalf("FlushKind organizational_change: %v", err)
	}

	// Run impact analyzer
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	analyzer := orgdomain.NewImpactAnalyzer(storage.NewOrganizationalStorageAdapter(store), logger, secCtx)
	impactID, err := analyzer.AnalyzeChange(ctx, changeID)
	if err != nil {
		t.Fatalf("AnalyzeChange: %v", err)
	}
	if impactID == emptyValue {
		t.Fatal("AnalyzeChange returned empty impact ID")
	}

	// Verify impact_analysis exists
	_, err = store.Read(ctx, secCtx, impactID)
	if err != nil {
		t.Fatalf("Read impact_analysis %s: %v", impactID, err)
	}
}

// TestOrganizationalRecordChange_Integration creates an organizational_change using the same object shape
// and storage path as the record-change command, then verifies it can be read.
func TestOrganizationalRecordChange_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	testRoot, _ := setupIntegrationTestWithSpecsOrganizational(t, "organizational-record-change-integration")

	store, err := storage.NewFileObjectStorage(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, store)
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, store)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Same object shape as runRecordChange (record_change.go)
	changeID := "OCH-902"
	obj := map[string]any{
		objects.FieldKeyID:                 changeID,
		objects.FieldKeyKind:               objects.KindOrganizationalChange,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:              "Integration record-change test",
		objects.FieldKeyStatus:             objects.ObjectStatusProposed,
		objects.FieldKeyChangeType:         "division_restructure",
		objects.FieldKeyChangeDescription:  "Recorded via integration test",
		objects.FieldKeyAffectedObjects:    map[string]any{},
		objects.FieldKeyImpactAnalysisRefs: []string{},
		objects.FieldKeyChangeDate:         zqktime.NowRFC3339UTC(),
	}

	opCtx := pkgctx.WithCacheUpdate(ctx, changeID, "organizational_change", "")
	if err := store.Create(opCtx, secCtx, obj); err != nil {
		t.Fatalf("Create organizational_change (record-change path): %v", err)
	}

	// Verify object exists and has expected fields
	got, err := store.Read(ctx, secCtx, changeID)
	if err != nil {
		t.Fatalf("Read organizational_change %s: %v", changeID, err)
	}
	if g, _ := got[objects.FieldKeyID].(string); g != changeID {
		t.Errorf("Read id: got %q want %q", g, changeID)
	}
	if g, _ := got[objects.FieldKeyKind].(string); g != objects.KindOrganizationalChange {
		t.Errorf("Read kind: got %q want organizational_change", g)
	}
	if g, _ := got[objects.FieldKeyChangeType].(string); g != "division_restructure" {
		t.Errorf("Read change_type: got %q want division_restructure", g)
	}
}
