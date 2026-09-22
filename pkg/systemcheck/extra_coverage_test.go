package systemcheck

import (
	"io"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupTestSpecDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	sampleSpec := `schema_version: "2.0.0"
ontology: "sample"
visibility: "public"
namespace: "zqk:sample"
id_prefixes:
  - "SMP-"
description: "Sample Spec"
traits: []
fields:
  title:
    type: string
    required: true
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "sample.yaml"), []byte(sampleSpec), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestViolationResolver_AllCategories(t *testing.T) {
	root := setupTestSpecDir(t)
	loader := objects.NewSpecLoader(filepath.Join(root, paths.ProcessInternalObjectSpecsDir))
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	resolver := NewViolationResolver(root, loader, logger)

	if path := resolver.GetObjectSpecPath("sample"); path != filepath.Join(root, paths.ProcessInternalObjectSpecsDir, "sample.yaml") {
		t.Errorf("unexpected spec path: %s", path)
	}

	result := &CheckResult{
		ObjectID:   "SMP-1",
		ObjectKind: "sample",
		FilePath:   "sample.yaml",
	}

	// 1. Spec not found
	missingRes := &CheckResult{ObjectKind: "nonexistent"}
	res, err := resolver.ResolveViolation(missingRes, Issue{Category: "instance_validation"})
	if err != nil || res.Resolved {
		t.Errorf("expected unresolved result when spec missing, got err: %v", err)
	}

	// 2. Instance validation issues
	ivTests := []struct {
		message    string
		confidence float64
		autoLink   bool
	}{
		{objects.FieldKeyPriorityPlanRef + " is missing", 0.7, true},
		{"milestone_ref is missing", 0.7, true},
		{objects.FieldKeyGoalRefs + " is required", 0.6, true},
		{"Invalid lifecycle status in object", 0.8, true},
		{"unrelated instance error", 0.0, false},
	}
	for _, tc := range ivTests {
		res, err := resolver.ResolveViolation(result, Issue{Category: "instance_validation", Message: tc.message})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.AutoLinkable != tc.autoLink {
			t.Errorf("msg %q: got AutoLinkable=%v, want %v", tc.message, res.AutoLinkable, tc.autoLink)
		}
		if res.Confidence != tc.confidence {
			t.Errorf("msg %q: got Confidence=%f, want %f", tc.message, res.Confidence, tc.confidence)
		}
	}

	// 3. Reference issues
	refRes, err := resolver.ResolveViolation(result, Issue{Category: "reference", Message: "field points to non-existent target"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refRes.Suggestions) == 0 {
		t.Error("expected suggestion for non-existent target")
	}

	refOther, _ := resolver.ResolveViolation(result, Issue{Category: "reference", Message: "other ref error"})
	if len(refOther.Suggestions) != 0 {
		t.Error("expected no suggestion for other ref error")
	}

	// 4. Lifecycle issues
	lcRes, err := resolver.ResolveViolation(result, Issue{Category: objects.KindLifecycle, Message: "Invalid lifecycle status detected"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !lcRes.AutoLinkable || lcRes.Confidence != 0.8 {
		t.Errorf("unexpected lcRes: %+v", lcRes)
	}

	lcOther, _ := resolver.ResolveViolation(result, Issue{Category: objects.KindLifecycle, Message: "other lifecycle error"})
	if lcOther.AutoLinkable {
		t.Error("expected not autolinkable for other lifecycle error")
	}

	// 5. Integrity issues
	integRes, err := resolver.ResolveViolation(result, Issue{Category: "integrity", Message: "Hash mismatch detected"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !integRes.AutoLinkable || integRes.Confidence != 0.9 {
		t.Errorf("unexpected integRes: %+v", integRes)
	}

	integOther, _ := resolver.ResolveViolation(result, Issue{Category: "integrity", Message: "other integrity error"})
	if integOther.AutoLinkable {
		t.Error("expected not autolinkable for other integrity error")
	}

	// 6. Unknown category
	unknownRes, err := resolver.ResolveViolation(result, Issue{Category: "unknown_cat"})
	if err != nil || unknownRes.AutoLinkable {
		t.Errorf("unexpected unknown category result: %+v", unknownRes)
	}
}

func TestViolationResolver_FilterResolvableViolations(t *testing.T) {
	root := setupTestSpecDir(t)
	loader := objects.NewSpecLoader(filepath.Join(root, paths.ProcessInternalObjectSpecsDir))
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	resolver := NewViolationResolver(root, loader, logger)

	snapshot := &CheckSnapshot{
		Metadata: CheckSnapshotMetadata{TotalObjects: 2},
		Results: []CheckResult{
			{
				ObjectID:   "SMP-1",
				ObjectKind: "sample",
				Issues: []Issue{
					// Confidence 0.9 (>= 0.8) -> filtered out
					{Category: "integrity", Message: "Hash mismatch detected"},
					// Confidence 0.6 (< 0.8) -> retained
					{Category: "instance_validation", Message: objects.FieldKeyGoalRefs + " is required"},
				},
			},
			{
				ObjectID:   "SMP-2",
				ObjectKind: "sample",
				Issues: []Issue{
					// Confidence 0.8 (>= 0.8) -> filtered out
					{Category: "instance_validation", Message: "Invalid lifecycle status"},
				},
			},
		},
	}

	filtered, err := resolver.FilterResolvableViolations(snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered.Results) != 1 {
		t.Fatalf("expected 1 result after filtering, got %d", len(filtered.Results))
	}
	if filtered.Results[0].ObjectID != "SMP-1" {
		t.Errorf("expected SMP-1 retained, got %s", filtered.Results[0].ObjectID)
	}
	if len(filtered.Results[0].Issues) != 1 {
		t.Errorf("expected 1 issue retained, got %d", len(filtered.Results[0].Issues))
	}
	if filtered.Metadata.TotalIssues != 1 {
		t.Errorf("expected TotalIssues=1, got %d", filtered.Metadata.TotalIssues)
	}
}

func TestCheckLifecycleWithLoader_Comprehensive(t *testing.T) {
	root := t.TempDir()
	lifecyclesDir := filepath.Join(root, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	lcContent := `kind: sample_item
initial_status: draft
statuses:
  - value: draft
    description: Draft state
  - value: active
    description: Active state
`
	if err := fileutil.WriteFile(filepath.Join(lifecyclesDir, "sample_item.yaml"), []byte(lcContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	loader := objects.NewLifecycleLoader("")

	// 1. Missing or empty status
	missingStatusObj := &parser.ParsedObject{
		ID:         "BLI-1",
		Kind:       "backlog_item",
		Properties: map[string]any{},
	}
	issues := CheckLifecycleWithLoader(missingStatusObj, "backlog_item", loader)
	if len(issues) == 0 || issues[0].Tier != 2 {
		t.Errorf("expected missing status Tier 2 issue, got: %+v", issues)
	}

	// 2. Valid status
	validStatusObj := &parser.ParsedObject{
		ID:   "BLI-1",
		Kind: "backlog_item",
		Properties: map[string]any{
			objects.FieldKeyStatus: "in_progress",
		},
	}
	issuesValid := CheckLifecycleWithLoader(validStatusObj, "backlog_item", loader)
	if len(issuesValid) != 0 {
		t.Errorf("expected 0 issues for valid status, got: %+v", issuesValid)
	}

	// 3. Invalid status
	invalidStatusObj := &parser.ParsedObject{
		ID:   "BLI-1",
		Kind: "backlog_item",
		Properties: map[string]any{
			objects.FieldKeyStatus: "unknown_bogus_status_xyz",
		},
	}
	issuesInvalid := CheckLifecycleWithLoader(invalidStatusObj, "backlog_item", loader)
	if len(issuesInvalid) == 0 || !issuesInvalid[0].AutoFixable {
		t.Errorf("expected autofixable issue for invalid status, got: %+v", issuesInvalid)
	}
}

func TestCheckRegistration_AllProvenanceChecks(t *testing.T) {
	// 1. Missing kind
	objMissingKind := &parser.ParsedObject{
		ID: "POL-1",
	}
	iss1 := CheckRegistration(objMissingKind, "policy")
	if len(iss1) == 0 {
		t.Error("expected missing kind issue")
	}

	// 2. Kind with extra text
	objExtraText := &parser.ParsedObject{
		ID:   "POL-1",
		Kind: "policy - extra draft",
		Properties: map[string]any{
			objects.FieldKeyCreatedBy: "ACC-1",
		},
	}
	iss2 := CheckRegistration(objExtraText, "policy")
	foundExtra := false
	for _, iss := range iss2 {
		if iss.Tier == 3 {
			foundExtra = true
		}
	}
	if !foundExtra {
		t.Error("expected Tier 3 kind extra text issue")
	}

	// 3. Kind mismatch
	objMismatch := &parser.ParsedObject{
		ID:   "POL-1",
		Kind: "backlog_item",
		Properties: map[string]any{
			objects.FieldKeyCreatedBy: "ACC-1",
		},
	}
	iss3 := CheckRegistration(objMismatch, "policy")
	foundMismatch := false
	for _, iss := range iss3 {
		if iss.Tier == 2 {
			foundMismatch = true
		}
	}
	if !foundMismatch {
		t.Error("expected kind mismatch issue")
	}

	// 4. Invalid ID format
	objInvalidID := &parser.ParsedObject{
		ID:   "INVALID-ID-FORMAT",
		Kind: "policy",
		Properties: map[string]any{
			objects.FieldKeyCreatedBy: "ACC-1",
		},
	}
	iss4 := CheckRegistration(objInvalidID, "policy")
	foundID := false
	for _, iss := range iss4 {
		if iss.Category == "registration" && iss.Tier == 2 {
			foundID = true
		}
	}
	if !foundID {
		t.Error("expected ID format mismatch issue")
	}
}
