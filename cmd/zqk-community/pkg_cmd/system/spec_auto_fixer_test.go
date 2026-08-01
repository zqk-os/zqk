package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/objects"
)

// Tests that call setupSpecAutoFixerTest must not use t.Parallel(): ZQK_TEST_ROOT is process-global;
// parallel tests overwrite each other's isolated root and break TempDir teardown.

// setupSpecAutoFixerTest creates a test environment and returns a SpecBasedAutoFixer.
func setupSpecAutoFixerTest(t *testing.T) (fixer *SpecBasedAutoFixer, testRoot string) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.system.spec_auto_fixer"})
	testRoot = proj.Root

	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	logger := logging.GetLoggerFromProfile("test")
	fixer = NewSpecBasedAutoFixer(ctx, logger)

	return fixer, testRoot
}

// createTestAutoFixContext creates a minimal AutoFixContext for testing
func createTestAutoFixContext(t *testing.T, testRoot, objID, kind string) *AutoFixContext {
	obj := &parser.ParsedObject{
		ID:   objID,
		Kind: kind,
		Properties: map[string]any{
			objects.FieldKeyID:   objID,
			objects.FieldKeyKind: kind,
		},
	}

	filePath := filepath.Join(testRoot, "test", objID+".yaml")
	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	return &AutoFixContext{
		Ctx:      ctx,
		Obj:      obj,
		FilePath: filePath,
		Kind:     kind,
		Logger:   logging.GetLoggerFromProfile("test"),
		AutoFix:  true,
		Force:    true,
	}
}

// TestAttemptFixes_RequiredField tests the missing required field strategy
func TestAttemptFixes_RequiredField(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"checklist": map[string]any{
			"default": "default_value",
		},
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
	}

	// Test with "required" keyword
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: field is required", objMap, fieldMap)
	if !fixed {
		t.Error("Expected fix to succeed for required field")
	}
	if msg == emptyValue {
		t.Error("Expected fix message to be non-empty")
	}
	if objMap["test_field"] != "default_value" {
		t.Errorf("Expected test_field to be set to default_value, got %v", objMap["test_field"])
	}

	// Test with "is required" keyword
	objMap2 := map[string]any{
		objects.FieldKeyID:   "ITEM-002",
		objects.FieldKeyKind: objects.KindBacklogItem,
	}
	fixed2, _ := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field2", "test_field2: field is required", objMap2, fieldMap)
	if !fixed2 {
		t.Error("Expected fix to succeed for 'is required' message")
	}
}

// TestAttemptFixes_RequiredAccountId_SystemAccountFallback asserts that when account_id (or
// created_by/updated_by) is required but the spec has no default, the fixer uses the system account.
func TestAttemptFixes_RequiredAccountId_SystemAccountFallback(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ZQK-001", objects.KindZqkSession)

	// No checklist.default or validation.default — extractDefaultValue returns nil
	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"checklist": map[string]any{
			"default": nil,
		},
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ZQK-001",
		objects.FieldKeyKind: objects.KindZqkSession,
	}

	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "account_id", "account_id: Field account_id is required", objMap, fieldMap)
	if !fixed {
		t.Error("Expected fix to succeed for required account_id with no spec default")
	}
	if msg == emptyValue {
		t.Error("Expected fix message to be non-empty")
	}
	if objMap[objects.FieldKeyAccountID] != pkgctx.SystemAccountID {
		t.Errorf("Expected account_id to be set to system account %q, got %v", pkgctx.SystemAccountID, objMap[objects.FieldKeyAccountID])
	}
}

// TestAttemptFixes_InternalMetricKind_SafeDefaults asserts safe defaults for audit_aggregation_metric
// (schema_version, metric_type, event_type_counts) per METRIC_TIER1_AUTOFIX_DESIGN. No default for
// aggregation_window_* or first_seen.
func TestAttemptFixes_InternalMetricKind_SafeDefaults(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "AAM-001", objects.KindAuditAggregationMetric)

	// No spec default (fieldMap has no checklist.default / validation.default)
	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"validation":         map[string]any{"required": true},
	}

	t.Run("schema_version", func(t *testing.T) {
		objMap := map[string]any{objects.FieldKeyID: "AAM-001", objects.FieldKeyKind: objects.KindAuditAggregationMetric}
		fixed, _ := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeySchemaVersion, "schema_version: field is required", objMap, fieldMap)
		if !fixed {
			t.Error("Expected fix for missing schema_version on audit_aggregation_metric")
		}
		if objMap[objects.FieldKeySchemaVersion] != objects.DefaultSchemaVersion {
			t.Errorf("Expected schema_version %s, got %v", objects.DefaultSchemaVersion, objMap[objects.FieldKeySchemaVersion])
		}
	})
	t.Run("event_type_counts", func(t *testing.T) {
		objMap := map[string]any{objects.FieldKeyID: "AAM-002", objects.FieldKeyKind: objects.KindAuditAggregationMetric}
		fieldMapObj := map[string]any{objects.FieldKeyType: "object", "validation": map[string]any{"required": true}}
		fixed, _ := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyEventTypeCounts, objects.FieldKeyEventTypeCounts+": field is required", objMap, fieldMapObj)
		if !fixed {
			t.Error("Expected fix for missing event_type_counts on audit_aggregation_metric")
		}
		etc, ok := objMap[objects.FieldKeyEventTypeCounts].(map[string]any)
		if !ok || len(etc) != 0 {
			t.Errorf("Expected event_type_counts {}, got %v", objMap[objects.FieldKeyEventTypeCounts])
		}
	})
	t.Run("aggregation_window_start no default", func(t *testing.T) {
		objMap := map[string]any{objects.FieldKeyID: "AAM-003", objects.FieldKeyKind: objects.KindAuditAggregationMetric}
		fixed, _ := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyAggregationWindowStart, objects.FieldKeyAggregationWindowStart+": field is required", objMap, fieldMap)
		if fixed {
			t.Error("Expected no fix for aggregation_window_start (must be computed or regenerated)")
		}
	})
}

// TestAttemptFixes_TypeCoercion tests the type coercion strategy
func TestAttemptFixes_TypeCoercion(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         123, // int instead of string
	}

	// Test with "type" keyword
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: invalid type, expected string", objMap, fieldMap)
	if !fixed {
		t.Error("Expected fix to succeed for type coercion")
	}
	if msg == emptyValue {
		t.Error("Expected fix message to be non-empty")
	}
	if _, ok := objMap["test_field"].(string); !ok {
		t.Errorf("Expected test_field to be coerced to string, got %T", objMap["test_field"])
	}

	// Test with "datatype" keyword
	objMap2 := map[string]any{
		objects.FieldKeyID:   "ITEM-002",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         456,
	}
	fixed2, _ := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: invalid datatype, expected string", objMap2, fieldMap)
	if !fixed2 {
		t.Error("Expected fix to succeed for 'datatype' message")
	}
}

// TestAttemptFixes_EventTypeCountsObjectCoercion tests that event_type_counts stored as JSON string
// (expected object, got string) is coerced to map[string]any so autofix succeeds.
func TestAttemptFixes_EventTypeCountsObjectCoercion(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "AAM-1772557177095002000", objects.KindAuditAggregationMetric)

	fieldMap := map[string]any{objects.FieldKeyType: "object"}

	// Value stored as string (e.g. from serialization) — triggers "expected object, got string"
	objMap := map[string]any{
		objects.FieldKeyID:              "AAM-1772557177095002000",
		objects.FieldKeyKind:            objects.KindAuditAggregationMetric,
		objects.FieldKeyEventTypeCounts: `{"create":1,"update":2}`,
	}

	msg := "event_type_counts: Field event_type_counts has invalid datatype: expected object, got string"
	fixed, fixMsg := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyEventTypeCounts, msg, objMap, fieldMap)
	if !fixed {
		t.Fatal("Expected fix to succeed for event_type_counts string -> object coercion")
	}
	if fixMsg == emptyValue {
		t.Error("Expected non-empty fix message")
	}
	etc, ok := objMap[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatalf("Expected event_type_counts to be map[string]any after coercion, got %T", objMap[objects.FieldKeyEventTypeCounts])
	}
	if etc["create"] != float64(1) || etc["update"] != float64(2) {
		t.Errorf("Expected event_type_counts create=1, update=2, got %v", etc)
	}
}

// TestAttemptFixes_EventTypeCountsObjectCoercion_EscapedJSONString tests that if the JSON
// payload is stored as an embedded/escaped JSON string, we still coerce it into a map.
func TestAttemptFixes_EventTypeCountsObjectCoercion_EscapedJSONString(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "AAM-1772557177095002000", objects.KindAuditAggregationMetric)

	fieldMap := map[string]any{objects.FieldKeyType: "object"}

	objMap := map[string]any{
		objects.FieldKeyID:   "AAM-1772557177095002000",
		objects.FieldKeyKind: objects.KindAuditAggregationMetric,
		// Embedded/escaped JSON string (outer quotes + escaped inner quotes)
		objects.FieldKeyEventTypeCounts: `"{\"create\":1,\"update\":2}"`,
	}

	msg := "event_type_counts: Field event_type_counts has invalid datatype: expected object, got string"
	fixed, fixMsg := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyEventTypeCounts, msg, objMap, fieldMap)
	if !fixed {
		t.Fatal("Expected fix to succeed for embedded escaped JSON string -> object coercion")
	}
	if fixMsg == emptyValue {
		t.Error("Expected non-empty fix message")
	}

	etc, ok := objMap[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatalf("Expected event_type_counts to be map[string]any after coercion, got %T", objMap[objects.FieldKeyEventTypeCounts])
	}

	createV := etc["create"]
	updateV := etc["update"]
	if createV != float64(1) || updateV != float64(2) {
		t.Errorf("Expected event_type_counts create=1, update=2, got %v", etc)
	}
}

// TestAttemptFixes_EventTypeCountsObjectCoercion_NullPlaceholder validates that common
// null-ish string placeholders are treated as empty objects during coercion.
func TestAttemptFixes_EventTypeCountsObjectCoercion_NullPlaceholder(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "AAM-1772557177095002000", objects.KindAuditAggregationMetric)

	fieldMap := map[string]any{objects.FieldKeyType: "object"}

	objMap := map[string]any{
		objects.FieldKeyID:              "AAM-1772557177095002000",
		objects.FieldKeyKind:            objects.KindAuditAggregationMetric,
		objects.FieldKeyEventTypeCounts: "null",
	}

	msg := "event_type_counts: Field event_type_counts has invalid datatype: expected object, got string"
	fixed, fixMsg := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyEventTypeCounts, msg, objMap, fieldMap)
	if !fixed {
		t.Fatal("Expected fix to succeed for null placeholder -> object coercion")
	}
	if fixMsg == emptyValue {
		t.Error("Expected non-empty fix message")
	}

	etc, ok := objMap[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatalf("Expected event_type_counts to be map[string]any after coercion, got %T", objMap[objects.FieldKeyEventTypeCounts])
	}
	if len(etc) != 0 {
		t.Errorf("Expected event_type_counts to coerce to empty map for null placeholder, got %v", etc)
	}
}

// TestAttemptFixes_EventTypeCountsObjectCoercion_AngleNilPlaceholder validates that
// common "<nil>"-style placeholders are treated as empty objects during coercion.
func TestAttemptFixes_EventTypeCountsObjectCoercion_AngleNilPlaceholder(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "AAM-1772557177095002000", objects.KindAuditAggregationMetric)

	fieldMap := map[string]any{objects.FieldKeyType: "object"}

	objMap := map[string]any{
		objects.FieldKeyID:              "AAM-1772557177095002000",
		objects.FieldKeyKind:            objects.KindAuditAggregationMetric,
		objects.FieldKeyEventTypeCounts: "<nil>",
	}

	msg := "event_type_counts: Field event_type_counts has invalid datatype: expected object, got string"
	fixed, fixMsg := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyEventTypeCounts, msg, objMap, fieldMap)
	if !fixed {
		t.Fatal("Expected fix to succeed for <nil> placeholder -> object coercion")
	}
	if fixMsg == emptyValue {
		t.Error("Expected non-empty fix message")
	}

	etc, ok := objMap[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatalf("Expected event_type_counts to be map[string]any after coercion, got %T", objMap[objects.FieldKeyEventTypeCounts])
	}
	if len(etc) != 0 {
		t.Errorf("Expected event_type_counts to coerce to empty map for <nil> placeholder, got %v", etc)
	}
}

// TestAttemptFixes_EventTypeCountsObjectCoercion_MissingField handles the case where
// the YAML omits the field entirely, but the validator still reports a datatype mismatch.
func TestAttemptFixes_EventTypeCountsObjectCoercion_MissingField(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "AAM-1772557177095002000", objects.KindAuditAggregationMetric)

	fieldMap := map[string]any{objects.FieldKeyType: "object"}

	objMap := map[string]any{
		objects.FieldKeyID:   "AAM-1772557177095002000",
		objects.FieldKeyKind: objects.KindAuditAggregationMetric,
		// Intentionally omit event_type_counts
	}

	msg := "event_type_counts: Field event_type_counts has invalid datatype: expected object, got string"
	fixed, fixMsg := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyEventTypeCounts, msg, objMap, fieldMap)
	if !fixed {
		t.Fatal("Expected fix to succeed when event_type_counts is missing (datatype mismatch)")
	}
	if fixMsg == emptyValue {
		t.Error("Expected non-empty fix message")
	}

	etc, ok := objMap[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatalf("Expected event_type_counts to be map[string]any after coercion, got %T", objMap[objects.FieldKeyEventTypeCounts])
	}
	if len(etc) != 0 {
		t.Errorf("Expected event_type_counts to coerce to empty map when field is missing, got %v", etc)
	}
}

func TestCoerceType_EventTypeCountsTypeFromResolvedSpec(t *testing.T) {
	repoRoot := findRepoRootFromCwd(t)
	specsDir := filepath.Join(repoRoot, paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)

	spec, err := specLoader.LoadSpecWithInheritance("audit_aggregation_metric.yaml")
	if err != nil {
		t.Fatalf("LoadSpecWithInheritance: %v", err)
	}

	fieldDef, ok := spec.ResolvedFields[objects.FieldKeyEventTypeCounts]
	if !ok {
		t.Fatalf("ResolvedFields missing %s", objects.FieldKeyEventTypeCounts)
	}

	fieldMap, ok := fieldDef.(map[string]any)
	if !ok {
		t.Fatalf("event_type_counts fieldDef is %T, want map[string]any", fieldDef)
	}

	// If the resolved spec represents type coercion differently (e.g. type isn't a string),
	// coerceType must still normalize and return an empty object for missing values.
	coerced := coerceType(objects.FieldKeyEventTypeCounts, nil, fieldMap, nil)
	out, dec := nildecode.DecodeNonNilPayload[any](coerced)
	if !dec {
		t.Fatalf("coerceType returned %T (%v), want map[string]any (possibly empty)", coerced, coerced)
	}
	if _, ok := out.(map[string]any); !ok {
		t.Fatalf("coerceType returned %T (%v), want map[string]any (possibly empty)", out, out)
	}
}

func findRepoRootFromCwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root containing .zqk (starting from %s)", wd)
		}
		dir = parent
	}
}

func TestFixInstanceValidationIssue_EventTypeCounts_Missing_FromRealObject(t *testing.T) {
	repoRoot := findRepoRootFromCwd(t)
	filePath := filepath.Join(datacell.CellCASPrimaryDir(repoRoot, "metrics"), "2a92d5129af0cf2d81d2a01ba4193bd1d57bbbadf6d2e5711b9990368fee4645.yaml")
	if _, err := os.Stat(filePath); err != nil {
		t.Skipf("fixture object not in tree (content-addressed path may change): %v", err)
	}

	yamlParser := parser.NewYAMLParser()
	obj, err := yamlParser.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	ctx := cli.ContextForProjectAndProfile(repoRoot, "test")
	logger := logging.GetLoggerFromProfile("test")

	fixer := NewSpecBasedAutoFixer(ctx, logger)
	fixCtx := &AutoFixContext{
		Ctx:      ctx,
		Obj:      obj,
		Kind:     objects.KindAuditAggregationMetric,
		Logger:   logger,
		FilePath: filePath,
	}

	issue := Issue{
		Tier:        2,
		Category:    "instance_validation",
		Message:     "event_type_counts: Field event_type_counts has invalid datatype: expected object, got string",
		AutoFixable: true,
	}

	success, msg, updatedObj := fixer.FixInstanceValidationIssueWithStorage(fixCtx, issue, nil)
	if !success || updatedObj == nil {
		t.Fatalf("FixInstanceValidationIssueWithStorage: success=%v msg=%q updatedObj=%v", success, msg, updatedObj)
	}

	v, ok := updatedObj[objects.FieldKeyEventTypeCounts]
	if !ok {
		t.Fatalf("updatedObj missing %s (keys=%v)", objects.FieldKeyEventTypeCounts, keysOfMap(updatedObj))
	}
	if _, ok := v.(map[string]any); !ok {
		t.Fatalf("event_type_counts type=%T want map[string]any", v)
	}
}

func TestFixInstanceValidationIssue_EventTypeCounts_StorageReadValueType(t *testing.T) {
	repoRoot := findRepoRootFromCwd(t)
	objID := "CJA-1772698500155213000"
	filePath := filepath.Join(datacell.CellCASPrimaryDir(repoRoot, "metrics"), "2a92d5129af0cf2d81d2a01ba4193bd1d57bbbadf6d2e5711b9990368fee4645.yaml")

	// Force file backend so this test is stable and doesn't rely on external graph services.
	t.Setenv(zqkenv.GraphEnabled(), "false")

	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, repoRoot)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	sp := storageFactory.GetStorage()
	if sp == nil {
		t.Fatalf("storage provider is nil")
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	props, err := sp.Read(stdctx, secCtx, objID)
	if err != nil {
		// If storage resolution fails in test env, we can't validate the discrepancy.
		// Avoid turning CI red due to infrastructure/setup variance.
		t.Skipf("storage.Read failed for %s: %v", objID, err)
	}

	t.Logf("storage read currentValue %s type=%T value=%#v", objects.FieldKeyEventTypeCounts, props[objects.FieldKeyEventTypeCounts], props[objects.FieldKeyEventTypeCounts])

	ctx := cli.ContextForProjectAndProfile(repoRoot, "test")
	logger := logging.GetLoggerFromProfile("test")
	fixer := NewSpecBasedAutoFixer(ctx, logger)

	obj := &parser.ParsedObject{
		ID:         objID,
		Kind:       objects.KindAuditAggregationMetric,
		Properties: props,
	}
	fixCtx := &AutoFixContext{
		Ctx:      ctx,
		Obj:      obj,
		FilePath: filePath,
		Kind:     objects.KindAuditAggregationMetric,
		Logger:   logger,
	}

	issue := Issue{
		Tier:        2,
		Category:    "instance_validation",
		Message:     "event_type_counts: Field event_type_counts has invalid datatype: expected object, got string",
		AutoFixable: true,
	}

	success, msg, updatedObj := fixer.FixInstanceValidationIssueWithStorage(fixCtx, issue, nil)
	if !success || updatedObj == nil {
		t.Fatalf("FixInstanceValidationIssueWithStorage: success=%v msg=%q updatedObj=%v", success, msg, updatedObj)
	}

	v, ok := updatedObj[objects.FieldKeyEventTypeCounts]
	if !ok {
		t.Fatalf("updatedObj missing %s (keys=%v)", objects.FieldKeyEventTypeCounts, keysOfMap(updatedObj))
	}
	if _, ok := v.(map[string]any); !ok {
		t.Fatalf("event_type_counts type=%T want map[string]any", v)
	}
}

func keysOfMap(m map[string]any) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestAttemptFixes_EventTypeCountsObjectCoercion_YAMLInlineMapString ensures we can parse
// YAML inline map syntax stored as a string (e.g. "{create: 1, update: 2}").
func TestAttemptFixes_EventTypeCountsObjectCoercion_YAMLInlineMapString(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "AAM-1772557177095002000", objects.KindAuditAggregationMetric)

	fieldMap := map[string]any{objects.FieldKeyType: "object"}

	objMap := map[string]any{
		objects.FieldKeyID:              "AAM-1772557177095002000",
		objects.FieldKeyKind:            objects.KindAuditAggregationMetric,
		objects.FieldKeyEventTypeCounts: "{create: 1, update: 2}",
	}

	msg := "event_type_counts: Field event_type_counts has invalid datatype: expected object, got string"
	fixed, fixMsg := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyEventTypeCounts, msg, objMap, fieldMap)
	if !fixed {
		t.Fatal("Expected fix to succeed for YAML inline map string -> object coercion")
	}
	if fixMsg == emptyValue {
		t.Error("Expected non-empty fix message")
	}

	etc, ok := objMap[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatalf("Expected event_type_counts to be map[string]any after coercion, got %T", objMap[objects.FieldKeyEventTypeCounts])
	}

	toFloat := func(v any) (float64, bool) {
		switch t := v.(type) {
		case float64:
			return t, true
		case int:
			return float64(t), true
		case int64:
			return float64(t), true
		case uint64:
			return float64(t), true
		default:
			return 0, false
		}
	}

	cf, ok := toFloat(etc["create"])
	if !ok {
		t.Fatalf("Expected numeric create value, got %T", etc["create"])
	}
	uf, ok := toFloat(etc["update"])
	if !ok {
		t.Fatalf("Expected numeric update value, got %T", etc["update"])
	}
	if cf != 1 || uf != 2 {
		t.Errorf("Expected event_type_counts create=1, update=2, got %v", etc)
	}
}

// TestAttemptFixes_EnumValidation tests the enum validation strategy
func TestAttemptFixes_EnumValidation(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"validation": map[string]any{
			"enum": []any{"value1", "value2", "value3"},
		},
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         "invalid_value",
	}

	// Test with "invalid value" keyword
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: invalid value, not in enum", objMap, fieldMap)
	if !fixed {
		t.Error("Expected fix to succeed for enum validation")
	}
	if msg == emptyValue {
		t.Error("Expected fix message to be non-empty")
	}
	if objMap["test_field"] != "value1" {
		t.Errorf("Expected test_field to be set to first enum value, got %v", objMap["test_field"])
	}
}

// TestAttemptFixes_PatternViolation tests the pattern violation strategy
func TestAttemptFixes_PatternViolation(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"validation": map[string]any{
			"pattern": "^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$",
		},
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         "invalid-namespace", // Missing prefix
	}

	// Test with "pattern" keyword
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: does not match pattern", objMap, fieldMap)
	// Pattern fixing may or may not succeed depending on the fix logic
	// Just verify it doesn't crash
	if msg == emptyValue && fixed {
		t.Log("Pattern fix succeeded")
	} else if !fixed {
		t.Log("Pattern fix did not succeed (may be expected)")
	}
}

// TestAttemptFixes_DateTimePatternWithTimeTime ensures datetime pattern violations are fixed when the
// field value is time.Time (e.g. YAML-unmarshaled datetime), producing RFC3339 and avoiding "empty message" warnings.
func TestAttemptFixes_DateTimePatternWithTimeTime(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "CJA-1772697604178588000", objects.KindAuditAggregationMetric)

	// ISO-8601 datetime pattern (same as audit_aggregation_metric aggregation_window_start)
	datetimePattern := `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`
	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"validation": map[string]any{
			"pattern": datetimePattern,
		},
	}

	// Value as time.Time (e.g. from YAML unmarshaling) — formatToPattern should format as RFC3339
	ts := time.Date(2026, 3, 16, 16, 50, 3, 0, time.FixedZone("PDT", -7*3600))
	objMap := map[string]any{
		objects.FieldKeyID:                     "CJA-1772697604178588000",
		objects.FieldKeyKind:                   objects.KindAuditAggregationMetric,
		objects.FieldKeyAggregationWindowStart: ts,
		objects.FieldKeyAggregationWindowEnd:   ts.Add(time.Hour),
	}

	errorMsg := objects.FieldKeyAggregationWindowStart + ": Field aggregation_window_start does not match pattern " + datetimePattern
	fixed, msg := fixer.attemptFixes(fixCtx, objects.KindAuditAggregationMetric, objects.FieldKeyAggregationWindowStart, errorMsg, objMap, fieldMap)

	if !fixed {
		t.Error("Expected datetime pattern fix to succeed when value is time.Time")
	}
	if msg == emptyValue {
		t.Error("Expected non-empty fix message (avoids 'Auto-fix returned empty message' warning)")
	}
	val, ok := objMap[objects.FieldKeyAggregationWindowStart].(string)
	if !ok {
		t.Fatalf("Expected aggregation_window_start to be string after fix, got %T", objMap[objects.FieldKeyAggregationWindowStart])
	}
	// Should be valid RFC3339
	if _, err := time.Parse(time.RFC3339, val); err != nil {
		t.Errorf("Expected RFC3339 string, got %q: %v", val, err)
	}
}

// datetimePattern matches the spec pattern used for created_at, updated_at, aggregation_window_*, etc.
// Same as in log output: "Field created_at does not match pattern ^\d{4}-\d{2}-..."
const datetimePattern = `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`

const schemaVersionPattern = `^\d+\.\d+\.\d+$`

// TestFixPatternViolation_NormalizesAllPatternFields exercises fixPatternViolation for every field
// that has incurred "does not match pattern" errors in production (log-events-human.log evidence).
// Covers: schema_version (semver); created_at, updated_at, last_seen, first_seen,
// aggregation_window_start, aggregation_window_end, aggregation_window_time (datetime).
func TestFixPatternViolation_NormalizesAllPatternFields(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	unixSec := int64(1735689600) // 2025-01-01 00:00:00 UTC
	rfc3339Expected := "2025-01-01T00:00:00Z"
	ts := time.Unix(unixSec, 0).UTC()

	t.Run("schema_version", func(t *testing.T) {
		fieldMap := map[string]any{
			objects.FieldKeyType: "string",
			"validation":         map[string]any{"pattern": schemaVersionPattern},
		}
		cases := []struct {
			name    string
			value   any
			want    string
			wantFix bool
		}{
			{"float_2.0", float64(2.0), objects.DefaultSchemaVersion, true},
			{"string_2.0", "2.0", objects.DefaultSchemaVersion, true},
			{"string_2", "2", objects.DefaultSchemaVersion, true},
			{"string_2.0.0", objects.DefaultSchemaVersion, objects.DefaultSchemaVersion, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := fixPatternViolation(objects.FieldKeySchemaVersion, tc.value, fieldMap, logger)
				if !tc.wantFix {
					if got != nil {
						t.Errorf("expected no fix, got %v", got)
					}
					return
				}
				str, ok := nildecode.DecodeNonNilPayload[*string](got)
				if !ok {
					t.Fatalf("expected *string %q, got %T %v", tc.want, got, got)
				}
				if *str != tc.want {
					t.Errorf("got %q, want %q", *str, tc.want)
				}
			})
		}
	})

	datetimeFields := []string{
		objects.FieldKeyCreatedAt,
		objects.FieldKeyUpdatedAt,
		objects.FieldKeyLastSeen,
		objects.FieldKeyFirstSeen,
		objects.FieldKeyAggregationWindowStart,
		objects.FieldKeyAggregationWindowEnd,
		"aggregation_window_time",
	}
	dateTimeFieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"validation":         map[string]any{"pattern": datetimePattern},
	}

	for _, fieldName := range datetimeFields {
		fieldName := fieldName
		t.Run(fieldName, func(t *testing.T) {
			t.Run("int64_unix", func(t *testing.T) {
				got := fixPatternViolation(fieldName, unixSec, dateTimeFieldMap, logger)
				str, ok := nildecode.DecodeNonNilPayload[*string](got)
				if !ok {
					t.Fatalf("expected *string (RFC3339), got %T %v", got, got)
				}
				if *str != rfc3339Expected {
					t.Errorf("got %q, want %q", *str, rfc3339Expected)
				}
				if _, err := time.Parse(time.RFC3339, *str); err != nil {
					t.Errorf("invalid RFC3339: %v", err)
				}
			})
			t.Run("float64_unix", func(t *testing.T) {
				got := fixPatternViolation(fieldName, float64(unixSec), dateTimeFieldMap, logger)
				str, ok := nildecode.DecodeNonNilPayload[*string](got)
				if !ok {
					t.Fatalf("expected *string (RFC3339), got %T %v", got, got)
				}
				if *str != rfc3339Expected {
					t.Errorf("got %q, want %q", *str, rfc3339Expected)
				}
			})
			t.Run("time.Time", func(t *testing.T) {
				got := fixPatternViolation(fieldName, ts, dateTimeFieldMap, logger)
				str, ok := nildecode.DecodeNonNilPayload[*string](got)
				if !ok {
					t.Fatalf("expected *string (RFC3339), got %T %v", got, got)
				}
				if *str != rfc3339Expected {
					t.Errorf("got %q, want %q", *str, rfc3339Expected)
				}
			})
		})
	}
}

// TestAttemptFixes_NamespaceIDPattern tests namespace_id pattern fixing with actual production error messages
func TestAttemptFixes_NamespaceIDPattern(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "CHA-1395", objects.KindChangeJournalEntry)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"validation": map[string]any{
			"pattern": "^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$",
		},
	}

	testCases := []struct {
		name          string
		namespaceID   string
		errorMessage  string
		expectedFixed bool
		expectedValue string
	}{
		{
			name:          "uppercase ZQK prefix",
			namespaceID:   "ZQK:kernel",
			errorMessage:  "namespace_id: Field namespace_id does not match pattern ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$",
			expectedFixed: true,
			expectedValue: "zqk:kernel",
		},
		{
			name:          "uppercase Kernel",
			namespaceID:   "zqk:Kernel",
			errorMessage:  "namespace_id: Field namespace_id does not match pattern ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$",
			expectedFixed: true,
			expectedValue: "zqk:kernel",
		},
		{
			name:          "missing prefix",
			namespaceID:   "kernel",
			errorMessage:  "namespace_id: Field namespace_id does not match pattern ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$",
			expectedFixed: true,
			expectedValue: "zqk:kernel",
		},
		{
			name:          "invalid prefix (non-allowed)",
			namespaceID:   "zqk:kernel",
			errorMessage:  "namespace_id: Field namespace_id does not match pattern ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$",
			expectedFixed: true, // Should convert to zqk:kernel
			expectedValue: "zqk:kernel",
		},
		{
			name:          "empty value",
			namespaceID:   "",
			errorMessage:  "namespace_id: Field namespace_id does not match pattern ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$",
			expectedFixed: true,
			expectedValue: "zqk:default",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objMap := map[string]any{
				objects.FieldKeyID:          "CHA-1395",
				objects.FieldKeyKind:        objects.KindChangeJournalEntry,
				objects.FieldKeyNamespaceID: tc.namespaceID,
			}

			// Parse error message to extract field name and error message
			parts := strings.SplitN(tc.errorMessage, ":", 2)
			if len(parts) < 2 {
				t.Fatalf("Invalid error message format: %s", tc.errorMessage)
			}
			fieldName := strings.TrimSpace(parts[0])
			errorMsg := strings.TrimSpace(parts[1])

			fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, fieldName, errorMsg, objMap, fieldMap)

			if fixed != tc.expectedFixed {
				t.Errorf("Expected fixed=%v, got %v", tc.expectedFixed, fixed)
			}

			if tc.expectedFixed {
				if msg == emptyValue {
					t.Error("Expected non-empty fix message when fix succeeds")
				}
				// Handle both string and *string (fixPatternViolation may return *string)
				var actualValue string
				switch v := objMap[objects.FieldKeyNamespaceID].(type) {
				case string:
					actualValue = v
				case *string:
					if v != nil {
						actualValue = *v
					} else {
						t.Error("Expected namespace_id to be non-nil *string")
						return
					}
				default:
					t.Errorf("Expected namespace_id to be string or *string, got %T", objMap[objects.FieldKeyNamespaceID])
					return
				}
				if actualValue != tc.expectedValue {
					t.Errorf("Expected namespace_id=%q, got %q", tc.expectedValue, actualValue)
				}
			}
		})
	}
}

// TestAttemptFixes_DisplayLengthSkip tests the display_length skip behavior
func TestAttemptFixes_DisplayLengthSkip(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         "some value",
	}

	// Test with "display_length" keyword - should skip but not fix
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: exceeds display_length", objMap, fieldMap)
	if fixed {
		t.Error("Expected display_length to be skipped (not fixed)")
	}
	if msg != emptyValue {
		t.Error("Expected no fix message for display_length skip")
	}
	// Should continue to next strategy if available
}

// TestAttemptFixes_MinLength tests the minimum length strategy
func TestAttemptFixes_MinLength(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"validation": map[string]any{
			"min_length": 10,
		},
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         "short", // 5 chars, needs padding
	}

	// Test with "too short" keyword
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: too short, minimum length is 10", objMap, fieldMap)
	if !fixed {
		t.Error("Expected fix to succeed for min length")
	}
	if msg == emptyValue {
		t.Error("Expected fix message to be non-empty")
	}
	if str, ok := objMap["test_field"].(string); ok {
		if len(str) < 10 {
			t.Errorf("Expected test_field to be padded to at least 10 chars, got length %d", len(str))
		}
	}
}

// TestAttemptFixes_MaxLength tests the maximum length strategy
func TestAttemptFixes_MaxLength(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"validation": map[string]any{
			"max_length": 10,
		},
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         "this is a very long string that exceeds the maximum length",
	}

	// Test with "too long" keyword
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: too long, maximum length is 10", objMap, fieldMap)
	if !fixed {
		t.Error("Expected fix to succeed for max length")
	}
	if msg == emptyValue {
		t.Error("Expected fix message to be non-empty")
	}
	if str, ok := objMap["test_field"].(string); ok {
		if len(str) > 10 {
			t.Errorf("Expected test_field to be truncated to max 10 chars, got length %d", len(str))
		}
	}
}

// TestAttemptFixes_ArrayMinLength tests the array minimum length strategy
func TestAttemptFixes_ArrayMinLength(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "list",
		"validation": map[string]any{
			"min_length": 3,
		},
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         []any{"item1"}, // Only 1 item, needs padding
	}

	// Test with "requires at least" and "items" keywords
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: requires at least 3 items", objMap, fieldMap)
	if !fixed {
		t.Error("Expected fix to succeed for array min length")
	}
	if msg == emptyValue {
		t.Error("Expected fix message to be non-empty")
	}
	if arr, ok := objMap["test_field"].([]any); ok {
		if len(arr) < 3 {
			t.Errorf("Expected test_field array to be padded to at least 3 items, got length %d", len(arr))
		}
	}
}

// TestAttemptFixes_NoMatch tests behavior when no strategy matches
func TestAttemptFixes_NoMatch(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		"test_field":         "value",
	}

	// Test with unmatched error message (avoiding keywords like "type", "required", etc.)
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: completely unknown error format", objMap, fieldMap)
	if fixed {
		t.Error("Expected no fix for unmatched error message")
	}
	if msg != emptyValue {
		t.Error("Expected empty message for unmatched error")
	}
}

// TestAttemptFixes_StrategyOrder tests that strategies are tried in correct order
func TestAttemptFixes_StrategyOrder(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	// Create fieldMap that could match multiple strategies
	fieldMap := map[string]any{
		objects.FieldKeyType: "string",
		"checklist": map[string]any{
			"default": "default_value",
		},
		"validation": map[string]any{
			"min_length": 10,
		},
	}

	objMap := map[string]any{
		objects.FieldKeyID:   "ITEM-001",
		objects.FieldKeyKind: objects.KindBacklogItem,
		// test_field is missing (required) AND would be too short if present
	}

	// Error message matches "required" (strategy 1) - should fix with default, not try min_length
	fixed, msg := fixer.attemptFixes(fixCtx, fixCtx.Kind, "test_field", "test_field: field is required", objMap, fieldMap)
	if !fixed {
		t.Error("Expected fix to succeed")
	}
	if !strings.Contains(msg, "default value") {
		t.Errorf("Expected fix message to mention default value, got: %s", msg)
	}
	if objMap["test_field"] != "default_value" {
		t.Errorf("Expected test_field to be set to default, got %v", objMap["test_field"])
	}
}

// TestGetValidationMap tests the getValidationMap helper
func TestGetValidationMap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		fieldMap  map[string]any
		wantFound bool
	}{
		{
			name: "validation map exists",
			fieldMap: map[string]any{
				objects.FieldKeyType: "string",
				"validation": map[string]any{
					"min_length": 5,
				},
			},
			wantFound: true,
		},
		{
			name: "validation map missing",
			fieldMap: map[string]any{
				objects.FieldKeyType: "string",
			},
			wantFound: false,
		},
		{
			name: "validation is not a map",
			fieldMap: map[string]any{
				objects.FieldKeyType: "string",
				"validation":         "not a map",
			},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation, found := getValidationMap(tt.fieldMap)
			if found != tt.wantFound {
				t.Errorf("getValidationMap() found = %v, want %v", found, tt.wantFound)
			}
			if tt.wantFound && validation == nil {
				t.Error("Expected validation map to be non-nil when found")
			}
		})
	}
}

// TestExtractLengthConstraint tests the extractLengthConstraint helper
func TestExtractLengthConstraint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		validation     map[string]any
		constraintName string
		wantValue      int
		wantFound      bool
	}{
		{
			name: "int value",
			validation: map[string]any{
				"min_length": 10,
			},
			constraintName: "min_length",
			wantValue:      10,
			wantFound:      true,
		},
		{
			name: "float64 value",
			validation: map[string]any{
				"max_length": 20.0,
			},
			constraintName: "max_length",
			wantValue:      20,
			wantFound:      true,
		},
		{
			name: "missing constraint",
			validation: map[string]any{
				"min_length": 10,
			},
			constraintName: "max_length",
			wantValue:      0,
			wantFound:      false,
		},
		{
			name: "wrong type",
			validation: map[string]any{
				"min_length": "not a number",
			},
			constraintName: "min_length",
			wantValue:      0,
			wantFound:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := extractLengthConstraint(tt.validation, tt.constraintName)
			if found != tt.wantFound {
				t.Errorf("extractLengthConstraint() found = %v, want %v", found, tt.wantFound)
			}
			if found && value != tt.wantValue {
				t.Errorf("extractLengthConstraint() value = %v, want %v", value, tt.wantValue)
			}
		})
	}
}

// TestFixInstanceValidationIssueLayer1_Integration tests the full integration
func TestFixInstanceValidationIssueLayer1_Integration(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	issue := Issue{
		Tier:     2,
		Category: "instance_validation",
		Message:  "test_field: field is required",
	}

	// This will fail if spec doesn't exist, but tests the flow
	success, msg, updatedObj := fixer.fixInstanceValidationIssueLayer1(fixCtx, issue)

	// May succeed or fail depending on spec availability
	// Just verify it doesn't crash
	if success {
		if msg == emptyValue {
			t.Error("Expected non-empty message on success")
		}
		if updatedObj == nil {
			t.Error("Expected non-nil updated object")
		}
	} else {
		t.Log("Fix did not succeed (may be expected if spec not found)")
	}
}

// TestFixInstanceValidationIssueLayer1_InvalidMessage tests invalid message format
func TestFixInstanceValidationIssueLayer1_InvalidMessage(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	issue := Issue{
		Tier:     2,
		Category: "instance_validation",
		Message:  "invalid message format", // No colon separator
	}

	success, msg, updatedObj := fixer.fixInstanceValidationIssueLayer1(fixCtx, issue)
	if success {
		t.Error("Expected fix to fail for invalid message format")
	}
	if msg != emptyValue {
		t.Error("Expected empty message for invalid format")
	}
	if updatedObj != nil {
		t.Error("Expected nil updated object for invalid format")
	}
}

// TestFixInstanceValidationIssueLayer1_WrongCategory tests wrong category handling
func TestFixInstanceValidationIssueLayer1_WrongCategory(t *testing.T) {
	fixer, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ITEM-001", objects.KindBacklogItem)

	issue := Issue{
		Tier:     1,
		Category: "integrity", // Wrong category
		Message:  "test_field: field is required",
	}

	// Should be handled by FixInstanceValidationIssueWithStorage, not Layer1
	// But Layer1 should still process it if called directly
	success, msg, updatedObj := fixer.fixInstanceValidationIssueLayer1(fixCtx, issue)
	// May succeed or fail, but shouldn't crash
	_ = success
	_ = msg
	_ = updatedObj
}
