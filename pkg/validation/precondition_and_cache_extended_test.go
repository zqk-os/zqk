package validation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestValidationStateCache_ExtendedInvalidation(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewValidationStateCache(tmpDir, 50*time.Millisecond)

	s1 := &ValidationState{ObjectID: "OBJ-1", Checksum: "c1", LastValidated: time.Now().Add(-100 * time.Millisecond)}
	s2 := &ValidationState{ObjectID: "OBJ-2", Checksum: "c2", LastValidated: time.Now()}
	s3 := &ValidationState{ObjectID: "OTHER-3", Checksum: "c3", LastValidated: time.Now()}

	cache.Set(s1)
	cache.Set(s2)
	cache.Set(s3)

	stale := cache.GetStale()
	if len(stale) != 1 || stale[0].ObjectID != "OBJ-1" {
		t.Errorf("expected 1 stale entry (OBJ-1), got %d", len(stale))
	}

	cache.InvalidateByPattern("OBJ-")
	if _, ok := cache.Get("OBJ-1"); ok {
		t.Errorf("expected OBJ-1 invalidated")
	}
	if _, ok := cache.Get("OBJ-2"); ok {
		t.Errorf("expected OBJ-2 invalidated")
	}
	if _, ok := cache.Get("OTHER-3"); !ok {
		t.Errorf("expected OTHER-3 still in cache")
	}

	if err := cache.InvalidateAll(); err != nil {
		t.Fatalf("InvalidateAll failed: %v", err)
	}
	if _, ok := cache.Get("OTHER-3"); ok {
		t.Errorf("expected OTHER-3 cleared by InvalidateAll")
	}
}

func TestOverlayDSL_Extended(t *testing.T) {
	gv := NewGoValidator()
	options := &ValidationOptions{ProjectRoot: t.TempDir()}

	// Empty precondition
	handled, met := evalOverlayDSLStage(gv, "", nil, options)
	if !handled || !met {
		t.Errorf("expected empty precondition to be handled and met")
	}

	// Standard checks pass
	obj := map[string]any{objects.FieldKeyID: "BLI-1"}
	handled, met = evalOverlayDSLStage(gv, "standard checks pass", obj, options)
	if !handled || !met {
		t.Errorf("expected standard checks to pass with ID")
	}

	// Doc entry reachable
	objWithoutPath := map[string]any{}
	handled, met = evalOverlayDSLStage(gv, "target document file exists and is reachable on disk", objWithoutPath, options)
	if !handled || met {
		t.Errorf("expected doc reachable false for empty path")
	}

	tmpFile := filepath.Join(options.ProjectRoot, "doc.md")
	_ = os.WriteFile(tmpFile, []byte("content"), 0644)
	objWithPath := map[string]any{objects.FieldKeyPath: tmpFile}
	handled, met = evalOverlayDSLStage(gv, "target file reachable and readable", objWithPath, options)
	if !handled || !met {
		t.Errorf("expected doc reachable true for existing path")
	}

	// Metadata populated
	objMeta := map[string]any{
		objects.FieldKeyTitle:   "Title",
		objects.FieldKeySummary: "Summary",
		objects.FieldKeyPath:    tmpFile,
	}
	handled, met = evalOverlayDSLStage(gv, "title, summary, and path are populated", objMeta, options)
	if !handled || !met {
		t.Errorf("expected metadata populated to be met")
	}

	// Content hash
	handled, met = evalOverlayDSLStage(gv, "cryptographic content_hash computed and sealed", map[string]any{"content_hash": "abc"}, options)
	if !handled || !met {
		t.Errorf("expected content hash computed to be met")
	}
	handled, met = evalOverlayDSLStage(gv, "cryptographic content_hash matches target file on disk", map[string]any{"content_hash": ""}, options)
	if !handled || met {
		t.Errorf("expected content hash matches to be false for empty hash")
	}

	// Content size measured
	handled, met = evalOverlayDSLStage(gv, "content_size measured", map[string]any{"content_size": 128}, options)
	if !handled || !met {
		t.Errorf("expected content size int to pass")
	}
	handled, met = evalOverlayDSLStage(gv, "document content_size measured", map[string]any{"content_size": int64(256)}, options)
	if !handled || !met {
		t.Errorf("expected content size int64 to pass")
	}
	handled, met = evalOverlayDSLStage(gv, "content_size measured", map[string]any{"content_size": float64(512)}, options)
	if !handled || !met {
		t.Errorf("expected content size float64 to pass")
	}
	handled, met = evalOverlayDSLStage(gv, "content_size measured", map[string]any{"content_size": 0}, options)
	if !handled || met {
		t.Errorf("expected content size 0 to fail")
	}

	// Fields populated
	objPop := map[string]any{
		"field_a": "val",
		"field_b": []string{"item"},
	}
	handled, met = evalOverlayDSLStage(gv, "field_a, field_b are populated", objPop, options)
	if !handled || !met {
		t.Errorf("expected fields are populated to pass")
	}
	handled, met = evalOverlayDSLStage(gv, "field_a and field_b are populated", objPop, options)
	if !handled || !met {
		t.Errorf("expected fields 'and' populated to pass")
	}
	handled, met = evalOverlayDSLStage(gv, "field_missing is populated", objPop, options)
	if !handled || met {
		t.Errorf("expected field_missing to fail")
	}

	// Unrecognized
	handled, _ = evalOverlayDSLStage(gv, "unrecognized arbitrary constraint", objPop, options)
	if handled {
		t.Errorf("expected unrecognized to not be handled")
	}
}

func TestGoValidator_MinMaxLength_And_Trunk(t *testing.T) {
	gv := NewGoValidator()

	// validateMinLength
	if err := gv.validateMinLength("str", "short", 10); err == nil {
		t.Errorf("expected error for string shorter than minLength")
	}
	if err := gv.validateMinLength("str", "long enough", 5); err != nil {
		t.Errorf("expected nil for string >= minLength")
	}
	if err := gv.validateMinLength("arr", []string{"a"}, 2); err == nil {
		t.Errorf("expected error for slice shorter than minLength")
	}
	if err := gv.validateMinLength("arr", []string{"a", "b"}, 2); err != nil {
		t.Errorf("expected nil for slice >= minLength")
	}

	// validateMaxLength
	if err := gv.validateMaxLength("str", "too long text", 5); err == nil {
		t.Errorf("expected error for string longer than maxLength")
	}
	if err := gv.validateMaxLength("str", "ok", 5); err != nil {
		t.Errorf("expected nil for string <= maxLength")
	}
	if err := gv.validateMaxLength("arr", []string{"a", "b", "c"}, 2); err == nil {
		t.Errorf("expected error for slice longer than maxLength")
	}
	if err := gv.validateMaxLength("arr", []string{"a"}, 2); err != nil {
		t.Errorf("expected nil for slice <= maxLength")
	}

	// checkBranchNameIsAncestorOfTrunk
	obj := map[string]any{}
	if gv.checkBranchNameIsAncestorOfTrunk(obj, &ValidationOptions{ProjectRoot: "."}) {
		t.Errorf("expected false when branch_name is empty")
	}
	t.Setenv("ZQK_TEST_BYPASS_GITEVIDENCE", "1")
	if !gv.checkBranchNameIsAncestorOfTrunk(obj, &ValidationOptions{ProjectRoot: "."}) {
		t.Errorf("expected true when bypass is active")
	}
}

func TestGoValidator_LinkBack_And_ActiveRef(t *testing.T) {
	gv := NewGoValidator()
	options := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			if id == "BLI-1" {
				return "planned", nil
			}
			if id == "BLI-CANCELLED" {
				return "cancelled", nil
			}
			if id == "GOAL-1" {
				return "active", nil
			}
			if id == "TST-1" {
				return "draft", nil
			}
			return "planned", nil
		},
		DependentsLookup: func(id string) []string {
			if id == "CRIT-1" {
				return []string{"TST-1"}
			}
			return nil
		},
	}

	// evalLinkBackStage
	obj := map[string]any{
		objects.FieldKeyGoalRefs:        []string{"GOAL-1"},
		objects.FieldKeyBacklogItemRefs: []string{"BLI-1"},
	}
	handled, _ := evalLinkBackStage(gv, "backlog_item link back to goal", obj, options)
	if !handled {
		t.Errorf("expected link back to stage to be handled")
	}

	handled, _ = evalLinkBackStage(gv, "not a linkback", obj, options)
	if handled {
		t.Errorf("expected non-linkback not to be handled")
	}

	// collectActiveRefFields
	fields := collectActiveRefFields("must have active vision")
	if len(fields) == 0 || fields[0] != objects.FieldKeyVisionRef {
		t.Errorf("expected vision_ref from collectActiveRefFields")
	}

	fields = collectActiveRefFields("must have active milestone")
	if len(fields) == 0 || fields[0] != objects.FieldKeyMilestoneRefs {
		t.Errorf("expected milestone_refs from collectActiveRefFields")
	}

	// evalActiveRefStage
	objActive := map[string]any{
		"vision_ref": "VIS-1",
	}
	handled, _ = evalActiveRefStage(gv, "must have active vision_ref", objActive, options)
	if !handled {
		t.Errorf("expected evalActiveRefStage to be handled")
	}

	// checkTDDTestRedPhase
	objTDD := map[string]any{
		"criteria_refs": []string{"CRIT-1"},
	}
	passed := gv.checkTDDTestRedPhase(objTDD, options)
	if !passed {
		t.Errorf("expected checkTDDTestRedPhase to pass with TST-1 in draft status")
	}

	// checkTDDTestRedPhase nil options
	if gv.checkTDDTestRedPhase(objTDD, nil) {
		t.Errorf("expected false for nil options")
	}
}

func TestValidationTierConfig_GlobalAndTiers(t *testing.T) {
	cfg := GetGlobalValidationTierConfig()
	if cfg == nil {
		t.Fatalf("expected non-nil tier config")
	}

	tier := cfg.GetTierForRule("non_existent_rule_xyz")
	if tier != 2 {
		t.Errorf("expected default tier 2 for unknown rule, got %d", tier)
	}

	isBlocking := cfg.IsBlockingTier(1)
	if !isBlocking {
		t.Errorf("expected tier 1 to be blocking")
	}
	if cfg.IsBlockingTier(999) {
		t.Errorf("expected tier 999 to not be blocking")
	}
}

func TestNamespaceRegistry_loadDefaultNamespaces(t *testing.T) {
	nr := NewNamespaceRegistry("")
	nr.loadDefaultNamespaces()

	ns := nr.GetNamespaceForKind(objects.KindBacklogItem)
	if ns == "" {
		t.Errorf("expected non-empty namespace for backlog_item")
	}
}

func TestInstanceValidator_Semantic_And_Preconditions(t *testing.T) {
	iv := NewInstanceValidator(nil)

	// validateFieldSemanticType
	fieldDef := map[string]any{"semantic_type": "markdown"}
	warn := iv.validateFieldSemanticType("desc", "valid string", fieldDef)
	if warn != nil {
		t.Errorf("unexpected warning for markdown string: %v", warn)
	}

	fieldDefNoSemantic := map[string]any{"type": "string"}
	if iv.validateFieldSemanticType("desc", "test", fieldDefNoSemantic) != nil {
		t.Errorf("expected nil for fieldDef without semantic_type")
	}

	// checkPrecondition
	obj := map[string]any{
		"test_field":  "present_value",
		"empty_field": "",
		"bool_field":  true,
		"status":      "planned",
	}

	if !iv.checkPrecondition("test_field is set", obj) {
		t.Errorf("expected test_field is set to be true")
	}
	if iv.checkPrecondition("missing_field is set", obj) {
		t.Errorf("expected missing_field is set to be false")
	}
	if !iv.checkPrecondition("test_field is not empty", obj) {
		t.Errorf("expected test_field is not empty to be true")
	}
	if iv.checkPrecondition("empty_field is not empty", obj) {
		t.Errorf("expected empty_field is not empty to be false")
	}

	// validateLifecycleState with nil lifecycleLoader
	ivNil := NewInstanceValidatorWithLifecycle(nil, nil)
	errs, warns := ivNil.validateLifecycleState(objects.KindBacklogItem, "planned", "draft", obj)
	if len(errs) != 0 || len(warns) != 0 {
		t.Errorf("expected 0 errors/warns when lifecycleLoader is nil")
	}
}
