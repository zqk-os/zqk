// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package scenario

import (
	"bytes"
	stdcontext "context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

type scenarioMockStorage struct {
	storage.ObjectStorageProvider
	mu       sync.Mutex
	objects  map[string]map[string]any
	createFn func(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
	readFn   func(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	updateFn func(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
}

func newScenarioMockStorage() *scenarioMockStorage {
	return &scenarioMockStorage{
		ObjectStorageProvider: storage.NewNoopObjectStorage(),
		objects:               make(map[string]map[string]any),
	}
}

func (m *scenarioMockStorage) Create(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if m.createFn != nil {
		return m.createFn(ctx, secCtx, obj)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := obj[objects.FieldKeyID].(string)
	if !ok || id == "" {
		return errors.New("missing id")
	}
	m.objects[id] = obj
	return nil
}

func (m *scenarioMockStorage) Read(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readFn != nil {
		return m.readFn(ctx, secCtx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, exists := m.objects[id]
	if !exists {
		return nil, errors.New("object not found: " + id)
	}
	return obj, nil
}

func (m *scenarioMockStorage) Update(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, secCtx, id, updates)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, exists := m.objects[id]
	if !exists {
		return errors.New("object not found: " + id)
	}
	for k, v := range updates {
		obj[k] = v
	}
	return nil
}

func TestLoadBundle_ValidationAndErrors(t *testing.T) {
	t.Parallel()

	// Invalid YAML
	_, err := LoadBundle(strings.NewReader("api_version: [invalid yaml"))
	if err == nil {
		t.Fatal("expected error on invalid YAML syntax, got nil")
	}

	// Missing api_version
	noAPIVer := "kind: scenario_bundle\nmetadata:\n  name: test\n"
	_, err = LoadBundle(strings.NewReader(noAPIVer))
	if err == nil || !strings.Contains(err.Error(), errMissingAPIVersion) {
		t.Fatalf("expected error %q, got: %v", errMissingAPIVersion, err)
	}

	// Missing kind
	noKind := "api_version: v1\nmetadata:\n  name: test\n"
	_, err = LoadBundle(strings.NewReader(noKind))
	if err == nil || !strings.Contains(err.Error(), errMissingKind) {
		t.Fatalf("expected error %q, got: %v", errMissingKind, err)
	}

	// Valid bundle
	validYAML := `
api_version: v1
kind: scenario_bundle
metadata:
  name: valid-bundle
  description: Valid scenario bundle test
  labels:
    tier: unit
objects:
  goals:
    - id_hint: GOAL-001
      title: "Goal Title"
steps:
  - id: step-1
    description: "First step"
    commands:
      - echo "hello"
`
	b, err := LoadBundle(strings.NewReader(validYAML))
	if err != nil {
		t.Fatalf("unexpected error on valid bundle: %v", err)
	}
	if b.Metadata.Name != "valid-bundle" {
		t.Errorf("expected bundle name 'valid-bundle', got %q", b.Metadata.Name)
	}
	if len(b.Objects.Goals) != 1 || len(b.Steps) != 1 {
		t.Fatalf("expected 1 goal and 1 step, got %d goals, %d steps", len(b.Objects.Goals), len(b.Steps))
	}
}

func TestDraftBundleYAML_EdgeCases(t *testing.T) {
	t.Parallel()

	// Empty name and empty description defaults
	defaultData, err := DraftBundleYAML("", "")
	if err != nil {
		t.Fatalf("DraftBundleYAML default returned error: %v", err)
	}
	bDefault, err := LoadBundle(bytes.NewReader(defaultData))
	if err != nil {
		t.Fatalf("LoadBundle on default draft bundle failed: %v", err)
	}
	if bDefault.Metadata.Name != "draft-bundle" {
		t.Errorf("expected default name 'draft-bundle', got %q", bDefault.Metadata.Name)
	}
	if !strings.Contains(bDefault.Metadata.Description, "Edit this bundle") {
		t.Errorf("expected default description, got %q", bDefault.Metadata.Description)
	}

	// Custom name and description
	customData, err := DraftBundleYAML("custom-scenario", "custom-description-text")
	if err != nil {
		t.Fatalf("DraftBundleYAML custom returned error: %v", err)
	}
	bCustom, err := LoadBundle(bytes.NewReader(customData))
	if err != nil {
		t.Fatalf("LoadBundle on custom draft bundle failed: %v", err)
	}
	if bCustom.Metadata.Name != "custom-scenario" || bCustom.Metadata.Description != "custom-description-text" {
		t.Errorf("custom draft metadata mismatch: %+v", bCustom.Metadata)
	}
}

func TestCreateOrder_RefMappingAndCycles(t *testing.T) {
	t.Parallel()

	// refFieldNameToTargetKind branches
	if got := refFieldNameToTargetKind("goal_refs"); got != "goal" {
		t.Errorf("refFieldNameToTargetKind('goal_refs') = %q, want 'goal'", got)
	}
	if got := refFieldNameToTargetKind("requirement_ref"); got != "requirement" {
		t.Errorf("refFieldNameToTargetKind('requirement_ref') = %q, want 'requirement'", got)
	}
	if got := refFieldNameToTargetKind("some_other_field"); got != "" {
		t.Errorf("refFieldNameToTargetKind('some_other_field') = %q, want empty", got)
	}

	// getRefTargetKinds with nil idx
	nilKinds := getRefTargetKinds(nil, "goal", map[string]bool{"requirement": true})
	if len(nilKinds) != 0 {
		t.Errorf("expected empty map for nil idx, got %v", nilKinds)
	}

	// BundleKindsForTraceability
	kinds := BundleKindsForTraceability()
	if len(kinds) != 7 {
		t.Errorf("BundleKindsForTraceability length = %d, want 7", len(kinds))
	}

	// topologicalOrderWithCycles with non-standard kinds causing cycle (fallbackKind branch)
	customKinds := []string{"zeta_kind", "alpha_kind"}
	customDeps := map[string]map[string]struct{}{
		"zeta_kind":  {"alpha_kind": {}},
		"alpha_kind": {"zeta_kind": {}},
	}
	order, deferred := topologicalOrderWithCycles(customKinds, customDeps)
	if len(order) != 2 {
		t.Fatalf("expected order length 2 for custom cycle, got %d (%v)", len(order), order)
	}
	if len(deferred) != 0 {
		t.Errorf("expected 0 deferred refs, got %v", deferred)
	}
}

func TestApplyTraceability_PureHelpers(t *testing.T) {
	t.Parallel()

	// resolveRef & resolveRefs
	hintMap := map[string]string{"HINT_1": "OBJ-100", "HINT_2": "OBJ-200"}
	if got := resolveRef("HINT_1", hintMap); got != "OBJ-100" {
		t.Errorf("resolveRef('HINT_1') = %q, want 'OBJ-100'", got)
	}
	if got := resolveRef("UNKNOWN", hintMap); got != "UNKNOWN" {
		t.Errorf("resolveRef('UNKNOWN') = %q, want 'UNKNOWN'", got)
	}
	if got := resolveRefs(nil, hintMap); got != nil {
		t.Errorf("resolveRefs(nil) = %v, want nil", got)
	}
	multi := resolveRefs([]string{"HINT_1", "HINT_2", "PASS_THROUGH"}, hintMap)
	if len(multi) != 3 || multi[0] != "OBJ-100" || multi[1] != "OBJ-200" || multi[2] != "PASS_THROUGH" {
		t.Errorf("resolveRefs returned unexpected slice: %v", multi)
	}

	// finalID
	if got := finalID("EXPLICIT-1", "HINT-1"); got != "EXPLICIT-1" {
		t.Errorf("finalID with explicit = %q, want 'EXPLICIT-1'", got)
	}
	if got := finalID("", "HINT-1"); got != "HINT-1" {
		t.Errorf("finalID with hint = %q, want 'HINT-1'", got)
	}
	if got := finalID("", ""); got != "" {
		t.Errorf("finalID empty = %q, want empty", got)
	}

	// freshInstanceBuilder invalid kind
	_, err := freshInstanceBuilder("completely_nonexistent_kind")
	if err == nil {
		t.Fatal("expected error from freshInstanceBuilder for nonexistent kind, got nil")
	}

	// extractStringList
	if got := extractStringList(nil); got != nil {
		t.Errorf("extractStringList(nil) = %v, want nil", got)
	}
	if got := extractStringList([]string{"a", "b"}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("extractStringList([]string) = %v, want ['a', 'b']", got)
	}
	anySlice := []any{"item1", "", 42, "item2"}
	if got := extractStringList(anySlice); len(got) != 2 || got[0] != "item1" || got[1] != "item2" {
		t.Errorf("extractStringList([]any) = %v, want ['item1', 'item2']", got)
	}

	// containsString
	if !containsString([]string{"one", "two"}, "two") {
		t.Errorf("containsString want true for 'two'")
	}
	if containsString([]string{"one", "two"}, "three") {
		t.Errorf("containsString want false for 'three'")
	}
}

func TestApplyScenarioBundle_PreflightAndOptionErrors(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()

	// Empty projectRoot
	_, err := ApplyScenarioBundle(ctx, "", strings.NewReader(""), ApplyObjectsOnly)
	if err == nil || !strings.Contains(err.Error(), "projectRoot is required") {
		t.Fatalf("expected projectRoot error, got: %v", err)
	}

	// Invalid ApplyMode
	tempDir := t.TempDir()
	validYAML := "api_version: v1\nkind: scenario_bundle\nmetadata:\n  name: test-mode\n"
	_, err = ApplyScenarioBundle(ctx, tempDir, strings.NewReader(validYAML), ApplyMode(42))
	if err == nil || !strings.Contains(err.Error(), "unsupported apply mode") {
		t.Fatalf("expected unsupported apply mode error, got: %v", err)
	}

	// Invalid bundle reader
	_, err = ApplyScenarioBundle(ctx, tempDir, strings.NewReader("corrupted:::yaml"), ApplyObjectsOnly)
	if err == nil {
		t.Fatal("expected error decoding corrupted bundle in ApplyScenarioBundle, got nil")
	}

	// ApplyOptions anchor context propagation
	mockStore := newScenarioMockStorage()
	opts := &ApplyOptions{
		Storage:           mockStore,
		TargetObjectID:    "REQ-ANCHOR-100",
		TargetKind:        "requirement",
		TargetTitle:       "Anchor Req Title",
		TargetPreExisted:  true,
		LinkedExistingIDs: []string{"GOAL-ROOT-001"},
		LinkageNote:       "Automated linkage verification",
	}
	summary, err := ApplyScenarioBundle(ctx, tempDir, strings.NewReader(validYAML), ApplyObjectsOnly, opts)
	if err != nil {
		t.Fatalf("unexpected error applying with custom options: %v", err)
	}
	if summary.TargetObjectID != "REQ-ANCHOR-100" ||
		summary.TargetKind != "requirement" ||
		summary.TargetTitle != "Anchor Req Title" ||
		!summary.TargetPreExisted ||
		summary.LinkageNote != "Automated linkage verification" ||
		len(summary.LinkedExistingIDs) != 1 {
		t.Errorf("summary anchor context was not properly propagated: %+v", summary)
	}
}

func TestScenarioSummaryDir_LoadAndEmit(t *testing.T) {
	t.Parallel()

	// scenarioSummaryDir sanitization
	dir := scenarioSummaryDir("/root", "name/with/slashes\\and\\backslashes")
	if strings.Contains(dir, "/") && strings.Contains(filepath.Base(dir), "/") {
		t.Errorf("scenarioSummaryDir did not sanitize slashes: %s", dir)
	}
	if filepath.Base(dir) != "name-with-slashes-and-backslashes" {
		t.Errorf("expected sanitized base name, got: %s", filepath.Base(dir))
	}
	defaultDir := scenarioSummaryDir("/root", "   ")
	if filepath.Base(defaultDir) != "default" {
		t.Errorf("expected 'default' base name for whitespace, got: %s", filepath.Base(defaultDir))
	}

	// LoadScenarioSummary preflight errors
	if _, err := LoadScenarioSummary("", "bundle"); err == nil {
		t.Error("expected error for empty projectRoot in LoadScenarioSummary")
	}
	if _, err := LoadScenarioSummary("/root", ""); err == nil {
		t.Error("expected error for empty bundleName in LoadScenarioSummary")
	}
	tempDir := t.TempDir()
	if _, err := LoadScenarioSummary(tempDir, "nonexistent"); err == nil {
		t.Error("expected error loading non-existent summary")
	}

	// Invalid JSON handling in LoadScenarioSummary
	summaryFolder := scenarioSummaryDir(tempDir, "bad-json")
	if err := fileutil.MkdirAll(summaryFolder, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir summary folder: %v", err)
	}
	badFile := filepath.Join(summaryFolder, "scenario-summary.json")
	if err := fileutil.WriteFile(badFile, []byte("{invalid-json"), paths.FilePerm600); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	if _, err := LoadScenarioSummary(tempDir, "bad-json"); err == nil {
		t.Error("expected error unmarshaling corrupted summary JSON")
	}

	// Round-trip EmitScenarioSummary and LoadScenarioSummary
	sum := &BundleSummary{
		ProjectRoot: tempDir,
		BundleName:  "round-trip-bundle",
		HintToID:    map[string]string{"HINT_A": "VAL_A"},
	}
	if err := EmitScenarioSummary(tempDir, sum); err != nil {
		t.Fatalf("EmitScenarioSummary failed: %v", err)
	}
	loaded, err := LoadScenarioSummary(tempDir, "round-trip-bundle")
	if err != nil {
		t.Fatalf("LoadScenarioSummary failed: %v", err)
	}
	if loaded.BundleName != sum.BundleName || loaded.HintToID["HINT_A"] != "VAL_A" {
		t.Errorf("loaded summary mismatch: %+v", loaded)
	}

	// emitScenarioSummary wrapper
	if err := emitScenarioSummary(tempDir, sum); err != nil {
		t.Fatalf("emitScenarioSummary failed: %v", err)
	}
}

func TestApplySchedulerJobFixtures_ErrorBranches(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	mockStore := newScenarioMockStorage()

	// Missing template
	missingTemplate := []SchedulerJobFixture{{IDHint: "SCH-1", Template: nil}}
	sum := &BundleSummary{HintToID: make(map[string]string)}
	err := applySchedulerJobFixtures(ctx, mockStore, secCtx, missingTemplate, sum)
	if err == nil || !strings.Contains(err.Error(), "fixture missing template") {
		t.Fatalf("expected missing template error, got: %v", err)
	}

	// Missing ID, IDHint, and Template.id
	missingID := []SchedulerJobFixture{{Template: map[string]any{"title": "No ID Job"}}}
	err = applySchedulerJobFixtures(ctx, mockStore, secCtx, missingID, sum)
	if err == nil || !strings.Contains(err.Error(), "must specify id, id_hint, or template.id") {
		t.Fatalf("expected missing id error, got: %v", err)
	}

	// Template with id field and explicit kind
	validFixture := []SchedulerJobFixture{
		{
			Template: map[string]any{
				objects.FieldKeyID:   "SCH-FROM-TMPL",
				objects.FieldKeyKind: objects.KindSchedulerJob,
				"title":              "Job From Template ID",
			},
		},
	}
	err = applySchedulerJobFixtures(ctx, mockStore, secCtx, validFixture, sum)
	if err != nil {
		t.Fatalf("unexpected error applying valid fixture: %v", err)
	}
	if len(sum.CreatedSchedulerJobIDs) != 1 || sum.CreatedSchedulerJobIDs[0] != "SCH-FROM-TMPL" {
		t.Errorf("fixture with template.id not applied correctly: %+v", sum)
	}

	// Storage create error
	mockStore.createFn = func(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
		return errors.New("simulated storage create failure")
	}
	err = applySchedulerJobFixtures(ctx, mockStore, secCtx, validFixture, sum)
	if err == nil || !strings.Contains(err.Error(), "simulated storage create failure") {
		t.Fatalf("expected storage create error, got: %v", err)
	}
}

func TestExecuteBundleSteps_Branches(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	tempDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Step with empty commands and empty command string (skipped)
	bundleWithEmptyCommands := &Bundle{
		Metadata: BundleMeta{Name: "step-test"},
		Steps: []BundleStep{
			{ID: "empty-step", Commands: []string{}},
			{Commands: []string{"   ", "\t"}},
		},
	}
	if err := executeBundleSteps(ctx, tempDir, bundleWithEmptyCommands, logger, nil, nil); err != nil {
		t.Fatalf("executeBundleSteps on empty commands returned error: %v", err)
	}

	// Step without ID (exercises fmt.Sprintf("step-%d", i+1)) and with custom stdout/stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	bundleWithCommands := &Bundle{
		Metadata: BundleMeta{Name: "step-test"},
		Steps: []BundleStep{
			{
				Description: "Step without ID",
				Commands:    []string{"echo step_output_ok"},
			},
		},
	}
	err := executeBundleSteps(ctx, tempDir, bundleWithCommands, logger, &stdoutBuf, &stderrBuf)
	if err != nil {
		t.Fatalf("executeBundleSteps failed: %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "step_output_ok") {
		t.Errorf("stdout did not capture command output: %s", stdoutBuf.String())
	}

	// Failing command with custom stdout/stderr
	failingBundle := &Bundle{
		Metadata: BundleMeta{Name: "step-fail"},
		Steps: []BundleStep{
			{
				ID:       "failing-step",
				Commands: []string{"exit 7"},
			},
		},
	}
	var failOut, failErr bytes.Buffer
	err = executeBundleSteps(ctx, tempDir, failingBundle, logger, &failOut, &failErr)
	if err == nil || !strings.Contains(err.Error(), "failing-step command") {
		t.Fatalf("expected failure error with stdout/stderr, got: %v", err)
	}

	// Failing command without custom stdout/stderr (CombinedOutput branch)
	err = executeBundleSteps(ctx, tempDir, failingBundle, logger, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "failing-step command") {
		t.Fatalf("expected failure error via CombinedOutput, got: %v", err)
	}
}

func TestApplyTraceability_EntityCreationValidationErrors(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	mockStore := newScenarioMockStorage()
	sum := &BundleSummary{HintToID: make(map[string]string)}

	// applyGoals missing id and id_hint
	if err := applyGoals(ctx, mockStore, secCtx, []GoalTemplate{{Title: "No ID"}}, sum); err == nil {
		t.Error("expected error for goal missing id and id_hint")
	}

	// applyRequirements missing id and id_hint
	if err := applyRequirements(ctx, mockStore, secCtx, []RequirementTemplate{{Title: "No ID"}}, sum); err == nil {
		t.Error("expected error for requirement missing id and id_hint")
	}

	// applyRequirements missing goal_refs
	if err := applyRequirements(ctx, mockStore, secCtx, []RequirementTemplate{{ID: "REQ-1", Title: "No Goals"}}, sum); err == nil {
		t.Error("expected error for requirement missing goal_refs")
	}

	// applyCriteria missing id and id_hint
	if err := applyCriteria(ctx, mockStore, secCtx, []CriteriaTemplate{{Title: "No ID"}}, sum); err == nil {
		t.Error("expected error for criteria missing id and id_hint")
	}

	// applyTestCases missing id and id_hint
	if err := applyTestCases(ctx, mockStore, secCtx, []TestCaseTemplate{{Title: "No ID"}}, sum); err == nil {
		t.Error("expected error for test_case missing id and id_hint")
	}

	// applyBacklogItems missing id and id_hint
	if err := applyBacklogItems(ctx, mockStore, secCtx, []BacklogTemplate{{Title: "No ID"}}, sum); err == nil {
		t.Error("expected error for backlog_item missing id and id_hint")
	}

	// applyDocEntries missing id and id_hint
	if err := applyDocEntries(ctx, mockStore, secCtx, []DocEntryTemplate{{Title: "No ID"}}, sum); err == nil {
		t.Error("expected error for doc_entry missing id and id_hint")
	}

	// applyConvergenceSessions missing id and id_hint
	if err := applyConvergenceSessions(ctx, mockStore, secCtx, []ConvergenceSessionTemplate{{Title: "No ID"}}, sum); err == nil {
		t.Error("expected error for convergence_session missing id and id_hint")
	}
}

func TestApplyTraceability_StorageCreationErrors(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	failingStore := newScenarioMockStorage()
	failingStore.createFn = func(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
		return errors.New("storage creation forced failure")
	}
	sum := &BundleSummary{HintToID: make(map[string]string)}

	if err := applyGoals(ctx, failingStore, secCtx, []GoalTemplate{{ID: "G-1", Title: "G"}}, sum); err == nil {
		t.Error("expected error when storage.Create fails on goal")
	}
	if err := applyRequirements(ctx, failingStore, secCtx, []RequirementTemplate{{ID: "R-1", Title: "R", GoalRefs: []string{"G-1"}}}, sum); err == nil {
		t.Error("expected error when storage.Create fails on requirement")
	}
	if err := applyCriteria(ctx, failingStore, secCtx, []CriteriaTemplate{{ID: "C-1", Title: "C"}}, sum); err == nil {
		t.Error("expected error when storage.Create fails on criteria")
	}
	if err := applyTestCases(ctx, failingStore, secCtx, []TestCaseTemplate{{ID: "T-1", Title: "T"}}, sum); err == nil {
		t.Error("expected error when storage.Create fails on test_case")
	}
	if err := applyBacklogItems(ctx, failingStore, secCtx, []BacklogTemplate{{ID: "B-1", Title: "B"}}, sum); err == nil {
		t.Error("expected error when storage.Create fails on backlog_item")
	}
	if err := applyDocEntries(ctx, failingStore, secCtx, []DocEntryTemplate{{ID: "D-1", Title: "D"}}, sum); err == nil {
		t.Error("expected error when storage.Create fails on doc_entry")
	}
	if err := applyConvergenceSessions(ctx, failingStore, secCtx, []ConvergenceSessionTemplate{{ID: "S-1", Title: "S"}}, sum); err == nil {
		t.Error("expected error when storage.Create fails on convergence_session")
	}
}

func TestLinkCriteriaTemplatesOntoRequirements_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	mockStore := newScenarioMockStorage()

	// Criteria template index out of bounds
	sum := &BundleSummary{CreatedCriteriaIDs: []string{}}
	critTemplates := []CriteriaTemplate{{RequirementRef: "REQ-1"}}
	err := linkCriteriaTemplatesOntoRequirements(ctx, mockStore, secCtx, critTemplates, sum)
	if err == nil || !strings.Contains(err.Error(), "has no created ID") {
		t.Fatalf("expected out of bounds error, got: %v", err)
	}

	// Requirement read error
	sum.CreatedCriteriaIDs = []string{"CRIT-1"}
	err = linkCriteriaTemplatesOntoRequirements(ctx, mockStore, secCtx, critTemplates, sum)
	if err == nil || !strings.Contains(err.Error(), "read requirement") {
		t.Fatalf("expected requirement read error, got: %v", err)
	}

	// Requirement already contains criteria ID (no-op check)
	mockStore.objects["REQ-1"] = map[string]any{
		objects.FieldKeyID:           "REQ-1",
		objects.FieldKeyCriteriaRefs: []string{"CRIT-1"},
	}
	err = linkCriteriaTemplatesOntoRequirements(ctx, mockStore, secCtx, critTemplates, sum)
	if err != nil {
		t.Fatalf("unexpected error when criteria already linked: %v", err)
	}

	// Requirement update error
	mockStore.objects["REQ-2"] = map[string]any{
		objects.FieldKeyID:           "REQ-2",
		objects.FieldKeyCriteriaRefs: []string{},
	}
	mockStore.updateFn = func(ctx stdcontext.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
		return errors.New("simulated update failure")
	}
	critTemplates2 := []CriteriaTemplate{{RequirementRefs: []string{"REQ-2"}}}
	err = linkCriteriaTemplatesOntoRequirements(ctx, mockStore, secCtx, critTemplates2, sum)
	if err == nil || !strings.Contains(err.Error(), "simulated update failure") {
		t.Fatalf("expected requirement update error, got: %v", err)
	}
}

func TestApplyTraceability_FullFieldCoreObjects(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	mockStore := newScenarioMockStorage()
	sum := &BundleSummary{HintToID: make(map[string]string)}

	goals := []GoalTemplate{
		{
			ID:          "GOAL-FULL-001",
			IDHint:      "G-FULL",
			Title:       "Custom Goal",
			Description: "Custom Goal Description",
			Status:      "active",
			Authority:   "architecture_council",
			Target:      "100",
		},
	}
	if err := applyGoals(ctx, mockStore, secCtx, goals, sum); err != nil {
		t.Fatalf("applyGoals failed: %v", err)
	}

	reqs := []RequirementTemplate{
		{
			ID:              "REQ-FULL-001",
			IDHint:          "R-FULL",
			Title:           "Custom Req",
			Body:            "Custom Requirement Body",
			Status:          "approved",
			GoalRefs:        []string{"G-FULL"},
			Criteria:        []string{"C-FULL"},
			Priority:        "p1",
			PriorityPlanRef: "PRI-CYCLE-001",
			DocEntryRefs:    []string{"DOC-FULL-001"},
		},
	}
	if err := applyRequirements(ctx, mockStore, secCtx, reqs, sum); err != nil {
		t.Fatalf("applyRequirements failed: %v", err)
	}

	crits := []CriteriaTemplate{
		{
			ID:               "CRIT-FULL-001",
			IDHint:           "C-FULL",
			Title:            "Custom Criteria",
			Description:      "Custom Criteria Description",
			Status:           "verified",
			Category:         "boundary",
			ValidationMethod: "automated_test",
		},
	}
	if err := applyCriteria(ctx, mockStore, secCtx, crits, sum); err != nil {
		t.Fatalf("applyCriteria failed: %v", err)
	}

	if len(sum.CreatedGoalIDs) != 1 || len(sum.CreatedRequirementIDs) != 1 || len(sum.CreatedCriteriaIDs) != 1 {
		t.Errorf("core objects summary counts incomplete: %+v", sum)
	}
}

func TestApplyTraceability_ExecutionObjects(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	mockStore := newScenarioMockStorage()
	sum := &BundleSummary{HintToID: map[string]string{
		"R-FULL": "REQ-FULL-001",
		"C-FULL": "CRIT-FULL-001",
		"G-FULL": "GOAL-FULL-001",
	}}

	tcs := []TestCaseTemplate{
		{
			ID:              "TST-FULL-001",
			IDHint:          "T-FULL",
			Title:           "Custom Test Case",
			Status:          "active",
			KindUnderTest:   "requirement",
			TestFunctions:   []string{"TestApplyTraceability_ExecutionObjects"},
			PathOrID:        "pkg/scenario/scenario_coverage_elevation_cycle205_test.go",
			RequirementRefs: []string{"R-FULL"},
			CriteriaRefs:    []string{"C-FULL"},
		},
	}
	if err := applyTestCases(ctx, mockStore, secCtx, tcs, sum); err != nil {
		t.Fatalf("applyTestCases failed: %v", err)
	}

	blis := []BacklogTemplate{
		{
			ID:              "BLI-FULL-001",
			IDHint:          "B-FULL",
			Title:           "Custom BLI",
			Description:     "Custom BLI Description",
			Status:          "in_progress",
			Priority:        "p0",
			PriorityTier:    "urgent",
			KindUnderTest:   "subsystem",
			RequirementRefs: []string{"R-FULL"},
			CriteriaRefs:    []string{"C-FULL"},
			TestCaseRefs:    []string{"T-FULL"},
			MilestoneRefs:   []string{"MS-RELEASE-1"},
			GoalRefs:        []string{"G-FULL"},
			DocEntryRefs:    []string{"DOC-FULL-001"},
		},
	}
	if err := applyBacklogItems(ctx, mockStore, secCtx, blis, sum); err != nil {
		t.Fatalf("applyBacklogItems failed: %v", err)
	}

	if len(sum.CreatedTestCaseIDs) != 1 || len(sum.CreatedBacklogItemIDs) != 1 {
		t.Errorf("execution objects summary counts incomplete: %+v", sum)
	}
}

func TestApplyTraceability_DocAndSessionObjects(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	mockStore := newScenarioMockStorage()
	sum := &BundleSummary{HintToID: map[string]string{"R-FULL": "REQ-FULL-001"}}

	contentSearchableFalse := false
	docEntries := []DocEntryTemplate{
		{
			ID:                "DOC-FULL-001",
			IDHint:            "D-FULL",
			Title:             "Architecture Spec",
			Summary:           "System operational architecture",
			Path:              "docs/guides/SPEC.md",
			Group:             "architecture",
			Category:          "guide",
			Status:            "published",
			ContentSearchable: &contentSearchableFalse,
			RequirementRefs:   []string{"R-FULL"},
		},
	}
	if err := applyDocEntries(ctx, mockStore, secCtx, docEntries, sum); err != nil {
		t.Fatalf("applyDocEntries failed: %v", err)
	}

	sessions := []ConvergenceSessionTemplate{
		{
			ID:               "CVS-FULL-001",
			IDHint:           "S-FULL",
			Title:            "Convergence Session 1",
			Status:           "in_progress",
			CurrentPhase:     "c2_align",
			OutcomeCharacter: "positive",
			DeltaAssessment:  "low_drift",
			DesiredEndState:  "All tests green and validated",
			Hypothesis:       "Elevating unit coverage reduces regressions",
			IterationProcess: "TDD red-green-refactor loop",
			NextAction:       "Execute verify suite",
			FlowVariant:      "autonomous",
			GlossaryTermRef:  "GLS-CONVERGENCE",
			AutomationHooks:  "hook_on_pass",
			RequirementRefs:  []string{"R-FULL"},
		},
	}
	if err := applyConvergenceSessions(ctx, mockStore, secCtx, sessions, sum); err != nil {
		t.Fatalf("applyConvergenceSessions failed: %v", err)
	}

	if len(sum.CreatedDocEntryIDs) != 1 || len(sum.CreatedConvergenceSessionIDs) != 1 {
		t.Errorf("doc and session objects summary counts incomplete: %+v", sum)
	}
}

func TestFlushKind_Execution(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	// flushKind should execute without panic even on an empty tempDir
	flushKind(tempDir, objects.KindGoal, logger)
}

func TestSetOriginIfNeeded(t *testing.T) {
	t.Parallel()
	builder, err := freshInstanceBuilder(objects.KindGoal)
	if err != nil {
		t.Fatalf("freshInstanceBuilder failed: %v", err)
	}
	setOriginIfNeeded(builder)
	builder.SetID("GOAL-TEST-ORIGIN")
	builder.SetField(objects.FieldKeyTitle, "Origin Test")
	builder.SetField(objects.FieldKeyAuthority, "owner")
	builder.SetField(objects.FieldKeyTarget, "1")
	builder.SetField(objects.FieldKeyDescription, "Testing origin fields")
	obj, err := builder.Build()
	if err != nil {
		t.Fatalf("builder.Build failed: %v", err)
	}
	if obj[objects.FieldKeyOriginProject] != validation.DefaultOriginProject {
		t.Errorf("origin_project mismatch: %v", obj[objects.FieldKeyOriginProject])
	}
	if obj[objects.FieldKeyOriginSystem] != validation.DefaultOriginSystem {
		t.Errorf("origin_system mismatch: %v", obj[objects.FieldKeyOriginSystem])
	}
}

func TestApplyScenarioBundle_ConcurrentExecution(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()
	concurrencyCount := 4
	var wg sync.WaitGroup
	errCh := make(chan error, concurrencyCount)

	for i := 0; i < concurrencyCount; i++ {
		workerID := i
		workerName := fmt.Sprintf("scenario_worker_%d", workerID)
		workerPurpose := fmt.Sprintf("concurrent scenario apply worker %d", workerID)

		goroutinelabels.NewGoroutine(workerName, workerPurpose).
			WithWaitGroup(&wg).
			StartSimple(func() {
				tempRoot := t.TempDir()
				mockStore := newScenarioMockStorage()
				bundleYAML := fmt.Sprintf(`
api_version: v1
kind: scenario_bundle
metadata:
  name: concurrent-bundle-%d
objects:
  fixtures:
    scheduler_jobs:
      - id_hint: SCH-JOB-%d
        template:
          id: SCH-JOB-%d
          title: "Worker Job %d"
          kind: scheduler_job
`, workerID, workerID, workerID, workerID)

				opts := &ApplyOptions{Storage: mockStore}
				summary, err := ApplyScenarioBundle(ctx, tempRoot, strings.NewReader(bundleYAML), ApplyObjectsOnly, opts)
				if err != nil {
					errCh <- fmt.Errorf("worker %d apply error: %w", workerID, err)
					return
				}
				if len(summary.CreatedSchedulerJobIDs) != 1 {
					errCh <- fmt.Errorf("worker %d unexpected job count: %d", workerID, len(summary.CreatedSchedulerJobIDs))
					return
				}
			})
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent execution error: %v", err)
	}
}

func assertApplyBundleFails(t *testing.T, ctx stdcontext.Context, bundleYAML string) {
	t.Helper()
	tempRoot := t.TempDir()
	mockStore := newScenarioMockStorage()
	opts := &ApplyOptions{Storage: mockStore}
	_, err := ApplyScenarioBundle(ctx, tempRoot, strings.NewReader(bundleYAML), ApplyObjectsOnly, opts)
	if err == nil {
		t.Fatal("expected error applying bundle, got nil")
	}
}

func TestApplyScenarioBundle_ObjectApplicationFailures(t *testing.T) {
	t.Parallel()
	ctx := stdcontext.Background()

	cases := []struct {
		name       string
		bundleYAML string
	}{
		{"goal failure", "api_version: v1\nkind: scenario_bundle\nmetadata:\n  name: f-g\nobjects:\n  goals:\n    - title: NoID\n"},
		{"criteria failure", "api_version: v1\nkind: scenario_bundle\nmetadata:\n  name: f-c\nobjects:\n  criteria:\n    - title: NoID\n"},
		{"requirement failure", "api_version: v1\nkind: scenario_bundle\nmetadata:\n  name: f-r\nobjects:\n  requirements:\n    - title: NoID\n"},
		{"test_case failure", "api_version: v1\nkind: scenario_bundle\nmetadata:\n  name: f-t\nobjects:\n  test_cases:\n    - title: NoID\n"},
		{"backlog_item failure", "api_version: v1\nkind: scenario_bundle\nmetadata:\n  name: f-b\nobjects:\n  backlog_items:\n    - title: NoID\n"},
		{"doc_entry failure", "api_version: v1\nkind: scenario_bundle\nmetadata:\n  name: f-d\nobjects:\n  doc_entries:\n    - title: NoID\n"},
		{"convergence_session failure", "api_version: v1\nkind: scenario_bundle\nmetadata:\n  name: f-s\nobjects:\n  convergence_sessions:\n    - title: NoID\n"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertApplyBundleFails(t, ctx, tc.bundleYAML)
		})
	}
}

func TestEmitScenarioSummary_MkdirFailure(t *testing.T) {
	t.Parallel()
	tempRoot := t.TempDir()
	scenariosFile := filepath.Join(tempRoot, paths.ProjectDataDir, "scenarios")
	if err := fileutil.MkdirAll(filepath.Dir(scenariosFile), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir parent failed: %v", err)
	}
	if err := fileutil.WriteFile(scenariosFile, []byte("blocker"), paths.FilePerm600); err != nil {
		t.Fatalf("write blocker file failed: %v", err)
	}
	sum := &BundleSummary{BundleName: "test-blocker"}
	err := EmitScenarioSummary(tempRoot, sum)
	if err == nil {
		t.Error("expected error when EmitScenarioSummary cannot create scenario dir")
	}
}
