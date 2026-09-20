package system

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestParseReferenceIssueMessage(t *testing.T) {
	msg := "Referenced object PRI-TDE-WAVE-7 (kind: priority_plan) in field priority_plan_ref does not exist in object cache\n  Cache key used: PRI-TDE-WAVE-7"
	id, kind, field := parseReferenceIssueMessage(msg)
	if id != "PRI-TDE-WAVE-7" || kind != "priority_plan" || field != "priority_plan_ref" {
		t.Fatalf("got id=%q kind=%q field=%q", id, kind, field)
	}
}

func TestUnlinkMissingRefFromObjectProps(t *testing.T) {
	obj := map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-MISSING",
		objects.FieldKeyCriteriaRefs:    []any{"CRIT-KEEP", "CRIT-GONE"},
	}
	if !unlinkMissingRefFromObjectProps(obj, objects.FieldKeyPriorityPlanRef, "PRI-MISSING") {
		t.Fatal("expected scalar unlink")
	}
	if _, ok := obj[objects.FieldKeyPriorityPlanRef]; ok {
		t.Fatalf("scalar ref should be removed, got %v", obj[objects.FieldKeyPriorityPlanRef])
	}
	if !unlinkMissingRefFromObjectProps(obj, "criteria_refs", "CRIT-GONE") {
		t.Fatal("expected list unlink")
	}
	got, ok := obj[objects.FieldKeyCriteriaRefs].([]string)
	if !ok || len(got) != 1 || got[0] != "CRIT-KEEP" {
		t.Fatalf("list unlink got %T %v", obj[objects.FieldKeyCriteriaRefs], obj[objects.FieldKeyCriteriaRefs])
	}
	if unlinkMissingRefFromObjectProps(obj, objects.FieldKeyPriorityPlanRef, "PRI-MISSING") {
		t.Fatal("expected false when field already gone")
	}
}

func TestProcessEmptyReferenceFieldIssue(t *testing.T) {
	_, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "PRI-EMPTY", objects.KindPriorityPlan)
	fixCtx.AutoFix = true
	fixCtx.Obj.Properties[objects.FieldKeyTeamConfigurationRef] = ""
	msg := processEmptyReferenceFieldIssue(fixCtx, Issue{
		Message: "team_configuration_ref: Semantic type validation: ontology validation failed: reference_type: reference cannot be empty",
	})
	if msg == "" || !strings.Contains(msg, "Unset empty reference") {
		t.Fatalf("msg=%q", msg)
	}
	if _, ok := fixCtx.Obj.Properties[objects.FieldKeyTeamConfigurationRef]; ok {
		t.Fatal("expected field removed")
	}
}

func TestProcessInvalidLifecycleStatusIssue(t *testing.T) {
	_, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ORG-BAD", objects.KindOrganization)
	fixCtx.AutoFix = true
	fixCtx.Obj.Properties[objects.FieldKeyStatus] = "approved"
	msg := processInvalidLifecycleStatusIssue(fixCtx, Issue{
		Category: objects.KindLifecycle,
		Message:  "Invalid lifecycle status 'approved' for kind 'organization'",
	})
	// organization has no error status → origin (conceptual)
	if msg == "" || !strings.Contains(msg, "lifecycle origin") {
		t.Fatalf("msg=%q", msg)
	}
	if fixCtx.Obj.Properties[objects.FieldKeyStatus] != "conceptual" {
		t.Fatalf("status=%v want conceptual", fixCtx.Obj.Properties[objects.FieldKeyStatus])
	}
}

func TestProcessInvalidLifecycleStatusIssue_DemotesToErrorWhenSupported(t *testing.T) {
	_, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "ATK-BAD-COMPLETE", objects.KindAgentTask)
	fixCtx.AutoFix = true
	fixCtx.Obj.Properties[objects.FieldKeyStatus] = "complete"
	msg := processInvalidLifecycleStatusIssue(fixCtx, Issue{
		Category: objects.KindLifecycle,
		Message:  "Invalid lifecycle status 'complete' for kind 'agent_task'",
	})
	if msg == "" || !strings.Contains(msg, "Demoted status") || !strings.Contains(msg, "error") {
		t.Fatalf("msg=%q", msg)
	}
	if fixCtx.Obj.Properties[objects.FieldKeyStatus] != "error" {
		t.Fatalf("status=%v want error", fixCtx.Obj.Properties[objects.FieldKeyStatus])
	}
	notes, _ := fixCtx.Obj.Properties[objects.FieldKeyNotes].(string)
	if !strings.Contains(notes, "Invalid lifecycle status 'complete'") {
		t.Fatalf("notes missing demote reason: %q", notes)
	}
}

func TestProcessInvalidLifecycleStatusIssue_SkipsWhenAlreadyValid(t *testing.T) {
	_, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "QUE-OK", "question")
	fixCtx.AutoFix = true
	fixCtx.Obj.Properties[objects.FieldKeyStatus] = "resolved"
	msg := processInvalidLifecycleStatusIssue(fixCtx, Issue{
		Category: objects.KindLifecycle,
		Message:  "Invalid lifecycle status 'archived' for kind 'question'",
	})
	if msg != "" {
		t.Fatalf("expected no rewrite for already-valid status, got %q", msg)
	}
	if fixCtx.Obj.Properties[objects.FieldKeyStatus] != "resolved" {
		t.Fatalf("status=%v want resolved", fixCtx.Obj.Properties[objects.FieldKeyStatus])
	}
}

func TestProcessReferenceIssue_AutoFixDoesNotUnlink(t *testing.T) {
	_, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "BLI-REF-1", objects.KindBacklogItem)
	fixCtx.AutoFix = true
	fixCtx.Force = false
	fixCtx.Force = false
	fixCtx.Obj.Properties[objects.FieldKeyPriorityPlanRef] = "PRI-DOES-NOT-EXIST"
	issue := Issue{
		Tier:        1,
		Category:    "reference",
		AutoFixable: false,
		Message:     "Referenced object PRI-DOES-NOT-EXIST (kind: priority_plan) in field priority_plan_ref does not exist in object cache",
	}
	msg := processReferenceIssue(fixCtx, issue)
	if msg != "" {
		t.Fatalf("auto-fix must not unlink missing refs, got %q", msg)
	}
	if got := fixCtx.Obj.Properties[objects.FieldKeyPriorityPlanRef]; got != "PRI-DOES-NOT-EXIST" {
		t.Fatalf("ref must remain, got %v", got)
	}
}

func TestProcessReferenceIssue_UnlinkMissingRequiresForce(t *testing.T) {
	_, testRoot := setupSpecAutoFixerTest(t)
	fixCtx := createTestAutoFixContext(t, testRoot, "BLI-REF-1", objects.KindBacklogItem)
	fixCtx.Force = true
	fixCtx.Obj.Properties[objects.FieldKeyPriorityPlanRef] = "PRI-DOES-NOT-EXIST"
	issue := Issue{
		Tier:        1,
		Category:    "reference",
		AutoFixable: false,
		Message:     "Referenced object PRI-DOES-NOT-EXIST (kind: priority_plan) in field priority_plan_ref does not exist in object cache",
	}
	msg := processReferenceIssue(fixCtx, issue)
	if msg == "" || !strings.Contains(msg, "Unlinked missing reference") || !strings.Contains(msg, "priority_plan_ref") {
		t.Fatalf("unexpected msg %q", msg)
	}
	if _, ok := fixCtx.Obj.Properties[objects.FieldKeyPriorityPlanRef]; ok {
		t.Fatalf("expected ref cleared, got %v", fixCtx.Obj.Properties[objects.FieldKeyPriorityPlanRef])
	}
}

func TestDuplicateAutoFixRefusesSilentDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "332e2a2e.yaml")
	if err := fileutil.WriteStandardFile(path, []byte("id: CRIT-KEEP\nkind: criteria\n")); err != nil {
		t.Fatal(err)
	}
	fixCtx := createTestAutoFixContext(t, dir, "CRIT-KEEP", objects.KindCriteria)
	fixCtx.FilePath = path
	msg := processIssueForAutoFix(fixCtx, Issue{
		Category:    "registration",
		Message:     "Duplicate object ID 'CRIT-KEEP' detected. This file appears to be a duplicate of 'other.yaml' (which is newer).",
		AutoFixable: true,
	}, nil, nil, nil)
	if !strings.Contains(msg, "refused to auto-delete purported duplicate") {
		t.Fatalf("msg=%q", msg)
	}
	if !fileutil.Exists(path) {
		t.Fatal("core kind CAS file must remain")
	}
}

type mockUpdateStorageForUnsetTest struct {
	storage.NoopObjectStorage
	lastUpdates map[string]any
}

func (m *mockUpdateStorageForUnsetTest) Update(_ context.Context, _ *pkgctx.SecurityContext, _ string, updates map[string]any) error {
	m.lastUpdates = updates
	return nil
}

func TestApplySpecFix_UnsetsRemovedFields(t *testing.T) {
	mockStore := &mockUpdateStorageForUnsetTest{}
	fixCtx := createTestAutoFixContext(t, t.TempDir(), "BLI-TEST-UNSET", objects.KindBacklogItem)
	originalProps := map[string]any{
		objects.FieldKeyID:              "BLI-TEST-UNSET",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Test item",
		objects.FieldKeyStatus:          objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:       "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:       "ACC-TEST",
		objects.FieldKeyUpdatedAt:       "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:       "ACC-TEST",
		objects.FieldKeyPriorityPlanRef: "PRI-OLD",
	}
	updatedProps := map[string]any{
		objects.FieldKeyID:            "BLI-TEST-UNSET",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Test item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
	}

	applied := applySpecFix(fixCtx, originalProps, updatedProps, mockStore)
	if !applied {
		t.Fatal("expected applySpecFix to succeed")
	}
	if v, exists := mockStore.lastUpdates[objects.FieldKeyPriorityPlanRef]; !exists || !storage.IsFieldUnset(v) {
		t.Fatalf("expected priority_plan_ref to be marked as FieldUnset, got %v (exists=%v)", v, exists)
	}
}

func TestProcessDuplicateReferenceIssue_CrossField(t *testing.T) {
	fixCtx := createTestAutoFixContext(t, t.TempDir(), "BLI-TEST-DUP", objects.KindBacklogItem)
	fixCtx.AutoFix = true

	// Case 1: related_object_refs vs priority_plan_ref -> should remove from related_object_refs
	fixCtx.Obj.Properties["priority_plan_ref"] = "PRI-123"
	fixCtx.Obj.Properties["related_object_refs"] = []any{"PRI-123", "BLI-OTHER"}

	msg := processDuplicateReferenceIssue(fixCtx, Issue{
		Category: "instance_validation",
		Message:  `related_object_refs: duplicate reference "PRI-123": target ID appears in both priority_plan_ref and related_object_refs`,
	})
	if msg == "" || !strings.Contains(msg, "Removed duplicate reference") {
		t.Fatalf("expected removal message, got: %q", msg)
	}
	if fixCtx.Obj.Properties["priority_plan_ref"] != "PRI-123" {
		t.Fatalf("expected priority_plan_ref to remain PRI-123, got: %v", fixCtx.Obj.Properties["priority_plan_ref"])
	}
	relRefs, ok := fixCtx.Obj.Properties["related_object_refs"].([]any)
	if !ok || len(relRefs) != 1 || relRefs[0] != "BLI-OTHER" {
		t.Fatalf("expected related_object_refs to contain only BLI-OTHER, got: %v", fixCtx.Obj.Properties["related_object_refs"])
	}

	// Case 2: related_object_refs becomes empty -> should unset related_object_refs
	fixCtx.Obj.Properties["requirement_refs"] = []string{"REQ-456"}
	fixCtx.Obj.Properties["related_object_refs"] = []string{"REQ-456"}
	msg2 := processDuplicateReferenceIssue(fixCtx, Issue{
		Category: "instance_validation",
		Message:  `requirement_refs: duplicate reference "REQ-456": target ID appears in both related_object_refs and requirement_refs`,
	})
	if msg2 == "" || !strings.Contains(msg2, "cleared empty") {
		t.Fatalf("expected cleared empty message, got: %q", msg2)
	}
	if _, exists := fixCtx.Obj.Properties["related_object_refs"]; exists {
		t.Fatalf("expected related_object_refs to be cleared, still exists: %v", fixCtx.Obj.Properties["related_object_refs"])
	}

	// Case 3: assignee_persona_ref vs persona_refs -> should remove from persona_refs
	fixCtx.Obj.Properties["assignee_persona_ref"] = "PER-DEV"
	fixCtx.Obj.Properties["persona_refs"] = []string{"PER-DEV", "PER-TEST"}
	msg3 := processDuplicateReferenceIssue(fixCtx, Issue{
		Category: "instance_validation",
		Message:  `persona_refs: duplicate reference "PER-DEV": target ID appears in both assignee_persona_ref and persona_refs`,
	})
	if msg3 == "" || !strings.Contains(msg3, "persona_refs") {
		t.Fatalf("expected persona_refs removal, got: %q", msg3)
	}
	if fixCtx.Obj.Properties["assignee_persona_ref"] != "PER-DEV" {
		t.Fatalf("expected assignee_persona_ref preserved, got: %v", fixCtx.Obj.Properties["assignee_persona_ref"])
	}
	personaRefs := fixCtx.Obj.Properties["persona_refs"].([]string)
	if len(personaRefs) != 1 || personaRefs[0] != "PER-TEST" {
		t.Fatalf("expected persona_refs to be [PER-TEST], got: %v", personaRefs)
	}

	// Case 4: milestone_ref vs milestone_refs -> should remove singular milestone_ref
	fixCtx.Obj.Properties["milestone_ref"] = "MIL-001"
	fixCtx.Obj.Properties["milestone_refs"] = []string{"MIL-001"}
	msg4 := processDuplicateReferenceIssue(fixCtx, Issue{
		Category: "instance_validation",
		Message:  `milestone_ref: duplicate reference "MIL-001": target ID appears in both milestone_refs and milestone_ref`,
	})
	if msg4 == "" || !strings.Contains(msg4, "milestone_ref") {
		t.Fatalf("expected milestone_ref removal, got: %q", msg4)
	}
	if _, exists := fixCtx.Obj.Properties["milestone_ref"]; exists {
		t.Fatalf("expected milestone_ref to be cleared, got: %v", fixCtx.Obj.Properties["milestone_ref"])
	}
	if len(fixCtx.Obj.Properties["milestone_refs"].([]string)) != 1 {
		t.Fatalf("expected milestone_refs to remain, got: %v", fixCtx.Obj.Properties["milestone_refs"])
	}

	// Case 5: criteria_refs vs evidence_refs with CRIT-* target -> should remove from evidence_refs
	fixCtx.Obj.Properties["criteria_refs"] = []string{"CRIT-001"}
	fixCtx.Obj.Properties["evidence_refs"] = []string{"CRIT-001"}
	msg5 := processDuplicateReferenceIssue(fixCtx, Issue{
		Category: "instance_validation",
		Message:  `evidence_refs: duplicate reference "CRIT-001": target ID appears in both criteria_refs and evidence_refs`,
	})
	if msg5 == "" || !strings.Contains(msg5, "evidence_refs") {
		t.Fatalf("expected evidence_refs removal, got: %q", msg5)
	}
	if _, exists := fixCtx.Obj.Properties["evidence_refs"]; exists {
		t.Fatalf("expected evidence_refs to be cleared, got: %v", fixCtx.Obj.Properties["evidence_refs"])
	}
}

func TestProcessDuplicateReferenceIssue_WithinField(t *testing.T) {
	fixCtx := createTestAutoFixContext(t, t.TempDir(), "BLI-TEST-DUP2", objects.KindBacklogItem)
	fixCtx.AutoFix = true

	fixCtx.Obj.Properties["persona_refs"] = []string{"PER-1", "PER-2", "PER-1", "PER-3"}
	msg := processDuplicateReferenceIssue(fixCtx, Issue{
		Category: "instance_validation",
		Message:  `persona_refs: duplicate reference "PER-1" within persona_refs`,
	})
	if msg == "" || !strings.Contains(msg, "Deduplicated reference") {
		t.Fatalf("expected deduplication message, got: %q", msg)
	}
	result := fixCtx.Obj.Properties["persona_refs"].([]string)
	expected := []string{"PER-1", "PER-2", "PER-3"}
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
}

func TestProcessDuplicateReferenceIssue_ThreeWayCascade(t *testing.T) {
	fixCtx := createTestAutoFixContext(t, t.TempDir(), "BLI-TEST-DUP3", objects.KindBacklogItem)
	fixCtx.AutoFix = true

	// Object contains DEC-001 in technical_spec_refs, related_object_refs, and decision_refs
	targetID := "DEC-1785930071988960000-364a5796"
	fixCtx.Obj.Properties["technical_spec_refs"] = []string{targetID}
	fixCtx.Obj.Properties["related_object_refs"] = []string{targetID, "DOC-001"}
	fixCtx.Obj.Properties["decision_refs"] = []string{targetID}

	// Issue 1: technical_spec_refs vs decision_refs -> removes from technical_spec_refs due to DEC- affinity
	msg1 := processDuplicateReferenceIssue(fixCtx, Issue{
		Category: "instance_validation",
		Message:  fmt.Sprintf(`decision_refs: duplicate reference "%s": target ID appears in both technical_spec_refs and decision_refs`, targetID),
	})
	if msg1 == "" || !strings.Contains(msg1, "technical_spec_refs") {
		t.Fatalf("expected technical_spec_refs removal, got: %q", msg1)
	}
	if _, exists := fixCtx.Obj.Properties["technical_spec_refs"]; exists {
		t.Fatalf("expected technical_spec_refs to be cleared, got: %v", fixCtx.Obj.Properties["technical_spec_refs"])
	}

	// Issue 2: technical_spec_refs vs related_object_refs -> technical_spec_refs already cleared,
	// cascades to related_object_refs vs decision_refs and removes from related_object_refs!
	msg2 := processDuplicateReferenceIssue(fixCtx, Issue{
		Category: "instance_validation",
		Message:  fmt.Sprintf(`related_object_refs: duplicate reference "%s": target ID appears in both technical_spec_refs and related_object_refs`, targetID),
	})
	if msg2 == "" || !strings.Contains(msg2, "related_object_refs") {
		t.Fatalf("expected related_object_refs removal, got: %q", msg2)
	}

	// Now DEC-001 must ONLY be in decision_refs!
	decRefs, ok := fixCtx.Obj.Properties["decision_refs"].([]string)
	if !ok || len(decRefs) != 1 || decRefs[0] != targetID {
		t.Fatalf("expected targetID in decision_refs, got: %v", fixCtx.Obj.Properties["decision_refs"])
	}

	relRefs, ok := fixCtx.Obj.Properties["related_object_refs"].([]string)
	if !ok || len(relRefs) != 1 || relRefs[0] != "DOC-001" {
		t.Fatalf("expected only DOC-001 in related_object_refs, got: %v", fixCtx.Obj.Properties["related_object_refs"])
	}
}
