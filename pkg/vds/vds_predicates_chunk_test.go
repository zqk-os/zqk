// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package vds

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCycle236_ChunkFile_LoadAndSaveErrors(t *testing.T) {
	dir := t.TempDir()
	nonexistent := filepath.Join(dir, "does_not_exist.yaml")
	if _, err := LoadChunks(nonexistent); err == nil {
		t.Fatal("expected error loading nonexistent chunk file")
	}

	invalidYAML := filepath.Join(dir, "invalid.yaml")
	if err := fileutil.WriteFile(invalidYAML, []byte("invalid:\n  - : [}"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write invalid YAML: %v", err)
	}
	if _, err := LoadChunks(invalidYAML); err == nil {
		t.Fatal("expected error loading invalid YAML chunk file")
	}

	emptyChunksFile := filepath.Join(dir, "empty_chunks.yaml")
	if err := fileutil.WriteFile(emptyChunksFile, []byte("schema: zqk_vds_chunks_v1\nchunks: []\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write empty chunks YAML: %v", err)
	}
	if _, err := LoadChunks(emptyChunksFile); err == nil {
		t.Fatal("expected error loading chunk file with zero chunks")
	}

	targetDirFile := filepath.Join(dir, "file_as_dir")
	if err := fileutil.WriteFile(targetDirFile, []byte("dummy"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	badSavePath := filepath.Join(targetDirFile, "sub", "chunks.yaml")
	chunks := []Chunk{
		{ChunkID: "chk-1", Stage: "design", Claim: "c", RubricRef: "r"},
	}
	if err := SaveChunks(badSavePath, chunks); err == nil {
		t.Fatal("expected error saving chunks when parent directory creation fails")
	}
}

func TestCycle236_ChunkFile_SchemaAndCloning(t *testing.T) {
	dir := t.TempDir()
	noSchemaPath := filepath.Join(dir, "no_schema.yaml")
	content := `chunks:
  - chunk_id: chk-default-schema
    stage: design
    claim: test claim
    rubric_ref: rub-1
    dsl_checks:
      - object_exists:OBJ-1
    evidence_refs:
      - ref-1
    independent_verify: pending
`
	if err := fileutil.WriteFile(noSchemaPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	loaded, err := LoadChunks(noSchemaPath)
	if err != nil {
		t.Fatalf("LoadChunks failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(loaded))
	}
	if loaded[0].ChunkID != "chk-default-schema" {
		t.Fatalf("unexpected chunk ID: %s", loaded[0].ChunkID)
	}

	cloned := cloneChunks(loaded)
	if len(cloned) != len(loaded) {
		t.Fatalf("expected %d cloned chunks, got %d", len(loaded), len(cloned))
	}
	cloned[0].DSLChecks[0] = "mutated"
	if loaded[0].DSLChecks[0] == "mutated" {
		t.Fatal("expected cloneChunks deep copy to isolate DSLChecks slice")
	}
}

func TestCycle236_Chunk_ApplyIndependentVerifyYes_Fallback(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fallback_chunks.yaml")
	// Malformed structure where surgicalSetIndependentVerifyYes cannot match line format
	raw := "schema: zqk_vds_chunks_v1\nchunks:\n  - chunk_id: chk-fallback\n    stage: design\n"
	if err := fileutil.WriteFile(p, []byte(raw), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	n, err := ApplyIndependentVerifyYes(p, []string{"chk-fallback"})
	if err != nil {
		t.Fatalf("ApplyIndependentVerifyYes failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 updated chunk, got %d", n)
	}

	loaded, err := LoadChunks(p)
	if err != nil {
		t.Fatalf("failed to reload chunk file: %v", err)
	}
	if len(loaded) != 1 || loaded[0].IndependentVerify != "yes" {
		t.Fatalf("expected independent_verify=yes after fallback, got %+v", loaded)
	}
}

func TestCycle236_Chunk_ApplyIndependentVerifyYes_EdgeCases(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "chunks_edge.yaml")
	raw := `schema: zqk_vds_chunks_v1
chunks:
  - chunk_id: chk-already-yes
    stage: design
    independent_verify: yes
  - chunk_id: chk-pending
    stage: design
    independent_verify: pending
`
	if err := fileutil.WriteFile(p, []byte(raw), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	n0, err := ApplyIndependentVerifyYes(p, []string{"", "   "})
	if err != nil || n0 != 0 {
		t.Fatalf("expected 0 updates for blank IDs, got %d, err: %v", n0, err)
	}

	n1, err := ApplyIndependentVerifyYes(p, []string{"nonexistent-id"})
	if err != nil || n1 != 0 {
		t.Fatalf("expected 0 updates for missing ID, got %d, err: %v", n1, err)
	}

	n2, err := ApplyIndependentVerifyYes(p, []string{"chk-already-yes"})
	if err != nil || n2 != 0 {
		t.Fatalf("expected 0 updates for already verified ID, got %d, err: %v", n2, err)
	}

	if _, err := ApplyIndependentVerifyYes(filepath.Join(dir, "missing.yaml"), []string{"chk-1"}); err == nil {
		t.Fatal("expected error on missing chunk file")
	}
}

func TestCycle236_InitChunksFile_ForceAndExists(t *testing.T) {
	dir := t.TempDir()
	p, created, err := InitChunksFile(dir, false)
	if err != nil {
		t.Fatalf("InitChunksFile failed: %v", err)
	}
	if !created {
		t.Fatal("expected file to be created on first init")
	}

	p2, created2, err2 := InitChunksFile(dir, false)
	if err2 != nil {
		t.Fatalf("InitChunksFile second pass failed: %v", err2)
	}
	if created2 {
		t.Fatal("expected created=false when file already exists and force=false")
	}
	if p != p2 {
		t.Fatalf("paths mismatch: %s vs %s", p, p2)
	}

	_, created3, err3 := InitChunksFile(dir, true)
	if err3 != nil {
		t.Fatalf("InitChunksFile force failed: %v", err3)
	}
	if !created3 {
		t.Fatal("expected created=true when force=true")
	}

	// Error when directory cannot be created
	fileAsDir := filepath.Join(dir, "blocked_dir")
	if err := fileutil.WriteFile(fileAsDir, []byte("data"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocking file: %v", err)
	}
	if _, _, err := InitChunksFile(fileAsDir, false); err == nil {
		t.Fatal("expected error from InitChunksFile when parent dir creation fails")
	}
}

func TestCycle236_Profile_ResolveAndErrorBranches(t *testing.T) {
	dir := t.TempDir()
	qDir := QualityDir(dir)
	if expected := filepath.Join(dir, paths.DocsQualityDir); qDir != expected {
		t.Fatalf("expected %s, got %s", expected, qDir)
	}

	if _, err := LoadSpine(dir, "nonexistent_spine.yaml"); err == nil {
		t.Fatal("expected error loading missing spine")
	}

	badSpinePath := filepath.Join(dir, "bad_spine.yaml")
	if err := fileutil.WriteFile(badSpinePath, []byte("schema_version: 1\nstages: []\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write bad spine: %v", err)
	}
	if _, err := LoadSpine(dir, badSpinePath); err == nil {
		t.Fatal("expected error loading spine with no stages")
	}

	corruptSpinePath := filepath.Join(dir, "corrupt_spine.yaml")
	if err := fileutil.WriteFile(corruptSpinePath, []byte("stages: [invalid yaml"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write corrupt spine: %v", err)
	}
	if _, err := LoadSpine(dir, corruptSpinePath); err == nil {
		t.Fatal("expected error loading corrupt YAML spine")
	}

	corruptCustPath := filepath.Join(dir, "corrupt_cust.yaml")
	if err := fileutil.WriteFile(corruptCustPath, []byte("project_id: [broken"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write corrupt cust: %v", err)
	}
	if _, _, err := LoadCustomization(dir, corruptCustPath); err == nil {
		t.Fatal("expected error loading corrupt YAML customization")
	}
}

func TestCycle236_Profile_ResolveProfiles_CustomPathVariations(t *testing.T) {
	dir := t.TempDir()
	validCustPath := filepath.Join(dir, "custom.yaml")
	custContent := `project_id: test-project
test_execution:
  mode: scheduler
  scheduler_submit_example: "zqk scheduler submit"
`
	if err := fileutil.WriteFile(validCustPath, []byte(custContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write custom.yaml: %v", err)
	}

	validSpinePath := filepath.Join(dir, "spine.yaml")
	spineContent := fmt.Sprintf(`profile_id: spine-test
customization_profile_path: %s
stages:
  - id: intent_capture
    label: Intent Capture
    purpose: Capture intent
`, validCustPath)
	if err := fileutil.WriteFile(validSpinePath, []byte(spineContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write spine.yaml: %v", err)
	}

	spine, cust, err := ResolveProfiles(dir, validSpinePath, "")
	if err != nil {
		t.Fatalf("ResolveProfiles failed: %v", err)
	}
	if spine == nil || spine.ProfileID != "spine-test" {
		t.Fatalf("unexpected spine profile: %+v", spine)
	}
	if cust == nil || cust.ProjectID != "test-project" {
		t.Fatalf("unexpected customization profile: %+v", cust)
	}

	if _, _, err := ResolveProfiles(dir, "missing_spine.yaml", ""); err == nil {
		t.Fatal("expected error when spine fails to load")
	}
}

func TestCycle236_Providers_LookupAndEnabledEdgeCases(t *testing.T) {
	disabledFlag := false
	enabledFlag := true
	cfg := VendorProvidersConfig{
		SchemaVersion: 1,
		Default:       "ide",
		Providers: []VendorProvider{
			{ID: "ide", Format: FormatIDEMDC, Out: ".ide/rule.mdc", Enabled: &enabledFlag},
			{ID: "cursor", Format: FormatIDEMDC, Out: ".cursor/rules.mdc", Enabled: &disabledFlag},
			{ID: "broken", Format: "", Out: "out.mdc", Enabled: &enabledFlag},
			{ID: "no_out", Format: FormatIDEMDC, Out: "", Enabled: &enabledFlag},
		},
	}

	if _, err := LookupProvider(cfg, "cursor"); err == nil {
		t.Fatal("expected error looking up disabled provider")
	}
	if _, err := LookupProvider(cfg, "broken"); err == nil {
		t.Fatal("expected error looking up provider with missing format")
	}
	if _, err := LookupProvider(cfg, "no_out"); err == nil {
		t.Fatal("expected error looking up provider with missing out path")
	}
	if _, err := LookupProvider(cfg, "nonexistent"); err == nil {
		t.Fatal("expected error looking up nonexistent provider")
	}

	emptyCfg := VendorProvidersConfig{}
	if _, err := LookupProvider(emptyCfg, ""); err == nil {
		t.Fatal("expected error looking up empty provider with no default")
	}

	enabled := EnabledProviders(cfg)
	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled providers with out path (ide and broken), got %d", len(enabled))
	}

	var nilCust *Customization
	resolvedDefault := ResolveVendorProviders(nilCust)
	if resolvedDefault.Default != "ide" || len(resolvedDefault.Providers) == 0 {
		t.Fatalf("expected stock IDE provider configuration on nil customization")
	}

	custWithNoDefault := &Customization{
		VendorProviders: VendorProvidersConfig{
			Providers: []VendorProvider{
				{ID: "prov1", Format: FormatIDEMDC, Out: "prov1.mdc"},
			},
		},
	}
	resolvedCust := ResolveVendorProviders(custWithNoDefault)
	if resolvedCust.Default != "prov1" {
		t.Fatalf("expected default to fall back to first provider ID, got %s", resolvedCust.Default)
	}
}

func TestCycle236_Project_RenderProviderBody_ErrorsAndDetails(t *testing.T) {
	ctx := context.Background()
	provBadFormat := VendorProvider{ID: "p1", Format: "unsupported_fmt", Out: "out.txt"}
	opt := ProjectOptions{ProjectRoot: "/tmp"}
	if _, _, err := RenderProviderBody(ctx, provBadFormat, opt); err == nil {
		t.Fatal("expected error for unsupported format in RenderProviderBody")
	}

	provValid := VendorProvider{ID: "ide", Format: FormatIDEMDC, Out: ".ide/vds.mdc"}
	lookup := func(c context.Context, id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyTitle:      "Verifiable Decomposition Spine",
			objects.FieldKeyPolicyType: "blocking_in_progress",
			objects.FieldKeyEnforcement: map[string]any{
				objects.FieldKeySeverity: "error",
				"automated":              true,
			},
		}, nil
	}
	spine := &SpineProfile{
		Stages: []StageDef{
			{ID: "intent_capture", Purpose: "Capture mission"},
			{ID: "implement", Purpose: "Write code"},
		},
	}
	cust := &Customization{
		TestExecution: TestExecutionPrefs{Mode: "foreground"},
	}
	optWithDetails := ProjectOptions{
		ProjectRoot:   "/tmp",
		Lookup:        lookup,
		Spine:         spine,
		Customization: cust,
	}
	body, gls, err := RenderProviderBody(ctx, provValid, optWithDetails)
	if err != nil {
		t.Fatalf("RenderProviderBody failed: %v", err)
	}
	if !strings.Contains(body, "severity=error; automated evaluator") {
		t.Fatalf("body missing policy enforcement metadata: %s", body)
	}
	if !strings.Contains(body, "foreground") {
		t.Fatalf("body missing test execution mode: %s", body)
	}
	if gls.ResolvedBy == "" {
		t.Fatal("expected resolved glossary metadata")
	}
}

func TestCycle236_Project_WriteOrCheckProvider_DriftAndMissing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	if _, err := WriteOrCheckProvider(ctx, ProjectOptions{}, true, false); err == nil {
		t.Fatal("expected error when ProjectRoot is empty")
	}

	opt := ProjectOptions{
		ProjectRoot: dir,
		ProviderID:  "ide",
	}

	if _, err := WriteOrCheckProvider(ctx, opt, false, false); err == nil {
		t.Fatal("expected error when neither write nor check is requested")
	}

	if _, err := WriteOrCheckProvider(ctx, opt, false, true); err == nil {
		t.Fatal("expected error on check when target file is missing")
	}

	resWrite, err := WriteOrCheckProvider(ctx, opt, true, false)
	if err != nil {
		t.Fatalf("WriteOrCheckProvider write failed: %v", err)
	}
	if !resWrite.Wrote || !resWrite.CheckOK || resWrite.Differs {
		t.Fatalf("unexpected write result: %+v", resWrite)
	}

	resCheck, err := WriteOrCheckProvider(ctx, opt, false, true)
	if err != nil {
		t.Fatalf("WriteOrCheckProvider check failed on clean write: %v", err)
	}
	if !resCheck.CheckOK || resCheck.Differs {
		t.Fatalf("expected check OK and differs=false, got %+v", resCheck)
	}

	if err := fileutil.WriteFile(resWrite.OutPath, []byte("tampered content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to tamper file: %v", err)
	}
	if _, err := WriteOrCheckProvider(ctx, opt, false, true); err == nil {
		t.Fatal("expected drift error when file content differs")
	}
}

func TestCycle236_Project_WriteOrCheckAllProviders_EdgeCases(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	if _, err := WriteOrCheckAllProviders(ctx, ProjectOptions{}, true, false); err == nil {
		t.Fatal("expected error when ProjectRoot is empty")
	}

	opt := ProjectOptions{ProjectRoot: dir}
	if _, err := WriteOrCheckAllProviders(ctx, opt, false, false); err == nil {
		t.Fatal("expected error when neither write nor check is specified")
	}

	disabledFlag := false
	disabledCust := &Customization{
		VendorProviders: VendorProvidersConfig{
			Providers: []VendorProvider{
				{ID: "disabled_prov", Format: FormatIDEMDC, Out: "disabled.mdc", Enabled: &disabledFlag},
			},
		},
	}
	optDisabled := ProjectOptions{ProjectRoot: dir, Customization: disabledCust}
	if _, err := WriteOrCheckAllProviders(ctx, optDisabled, true, false); err == nil {
		t.Fatal("expected error when no enabled providers are configured")
	}

	// Check failure branch when check=true on missing files with write=false
	if _, err := WriteOrCheckAllProviders(ctx, opt, false, true); err == nil {
		t.Fatal("expected error when providers fail check")
	}

	batchRes, err := WriteOrCheckAllProviders(ctx, opt, true, true)
	if err != nil {
		t.Fatalf("WriteOrCheckAllProviders failed: %v", err)
	}
	if len(batchRes.Results) == 0 {
		t.Fatal("expected at least 1 provider result in batch")
	}
}

func TestCycle236_Predicates_PathAndObjectLookups(t *testing.T) {
	ctx := context.Background()
	opt := EvalOptions{}
	res := EvalPredicate(ctx, "", Chunk{}, opt)
	if res.OK || res.Detail != "empty predicate" {
		t.Fatalf("unexpected result for empty predicate: %+v", res)
	}

	resUnknown := EvalPredicate(ctx, "nonexistent_rule:arg", Chunk{}, opt)
	if resUnknown.OK || !strings.Contains(resUnknown.Detail, "unknown predicate") {
		t.Fatalf("unexpected result for unknown predicate: %+v", resUnknown)
	}

	resPathEmpty := EvalPredicate(ctx, "path_exists:", Chunk{}, opt)
	if resPathEmpty.OK || resPathEmpty.Detail != "missing path" {
		t.Fatalf("expected missing path detail, got %+v", resPathEmpty)
	}

	resPathNoRoot := EvalPredicate(ctx, "path_exists:foo.txt", Chunk{}, opt)
	if resPathNoRoot.OK || !resPathNoRoot.Skipped {
		t.Fatalf("expected skipped path_exists without project root, got %+v", resPathNoRoot)
	}

	dir := t.TempDir()
	existingFile := filepath.Join(dir, "exists.txt")
	if err := fileutil.WriteFile(existingFile, []byte("content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	optDir := EvalOptions{ProjectRoot: dir}
	resPathExists := EvalPredicate(ctx, "path_exists:exists.txt", Chunk{}, optDir)
	if !resPathExists.OK {
		t.Fatalf("expected path_exists OK, got %+v", resPathExists)
	}

	resPathMissing := EvalPredicate(ctx, "path_exists:missing.txt", Chunk{}, optDir)
	if resPathMissing.OK {
		t.Fatalf("expected path_exists missing to fail, got %+v", resPathMissing)
	}

	resObjEmpty := EvalPredicate(ctx, "object_exists:", Chunk{}, opt)
	if resObjEmpty.OK || resObjEmpty.Detail != "missing object id" {
		t.Fatalf("expected missing object id detail, got %+v", resObjEmpty)
	}

	resObjNoLookup := EvalPredicate(ctx, "object_exists:OBJ-1", Chunk{}, opt)
	if resObjNoLookup.OK || !resObjNoLookup.Skipped {
		t.Fatalf("expected skipped object_exists with nil lookup, got %+v", resObjNoLookup)
	}
}

func TestCycle236_Predicates_FieldNonemptyAndCriteria(t *testing.T) {
	ctx := context.Background()
	optNoLookup := EvalOptions{}

	resBadFormat := EvalPredicate(ctx, "field_nonempty:invalid_format", Chunk{}, optNoLookup)
	if resBadFormat.OK || !strings.Contains(resBadFormat.Detail, "want field_nonempty") {
		t.Fatalf("expected format failure for field_nonempty: %+v", resBadFormat)
	}

	resSkip := EvalPredicate(ctx, "field_nonempty:OBJ-1:title", Chunk{}, optNoLookup)
	if resSkip.OK || !resSkip.Skipped {
		t.Fatalf("expected skipped when lookup is nil: %+v", resSkip)
	}

	lookup := func(c context.Context, id string) (map[string]any, error) {
		if id == "ERR-1" {
			return nil, fmt.Errorf("lookup error for %s", id)
		}
		return map[string]any{
			"empty_str":                  "   ",
			"valid_str":                  "present",
			"empty_list":                 []any{},
			"valid_list":                 []any{"item"},
			"integer":                    123,
			objects.FieldKeyCriteriaRefs: []string{"CRIT-1"},
		}, nil
	}
	opt := EvalOptions{Lookup: lookup}

	if r := EvalPredicate(ctx, "field_nonempty:ERR-1:title", Chunk{}, opt); r.OK || !strings.Contains(r.Detail, "lookup error") {
		t.Fatalf("expected lookup error propagation, got %+v", r)
	}
	if r := EvalPredicate(ctx, "field_nonempty:OBJ-1:missing_field", Chunk{}, opt); r.OK || !strings.Contains(r.Detail, "field missing") {
		t.Fatalf("expected field missing detail, got %+v", r)
	}
	if r := EvalPredicate(ctx, "field_nonempty:OBJ-1:empty_str", Chunk{}, opt); r.OK || !strings.Contains(r.Detail, "field empty") {
		t.Fatalf("expected field empty detail, got %+v", r)
	}
	if r := EvalPredicate(ctx, "field_nonempty:OBJ-1:empty_list", Chunk{}, opt); r.OK || !strings.Contains(r.Detail, "field empty list") {
		t.Fatalf("expected field empty list detail, got %+v", r)
	}
	if r := EvalPredicate(ctx, "field_nonempty:OBJ-1:valid_str", Chunk{}, opt); !r.OK {
		t.Fatalf("expected valid string field to pass, got %+v", r)
	}
	if r := EvalPredicate(ctx, "field_nonempty:OBJ-1:valid_list", Chunk{}, opt); !r.OK {
		t.Fatalf("expected valid list field to pass, got %+v", r)
	}
	if r := EvalPredicate(ctx, "field_nonempty:OBJ-1:integer", Chunk{}, opt); !r.OK {
		t.Fatalf("expected integer field to pass, got %+v", r)
	}

	if r := EvalPredicate(ctx, "criteria_linked_or_acceptance_present:", Chunk{WorkObjectRef: ""}, opt); r.OK {
		t.Fatalf("expected failure on empty criteria ref: %+v", r)
	}
	if r := EvalPredicate(ctx, "criteria_linked_or_acceptance_present:OBJ-1", Chunk{}, opt); !r.OK {
		t.Fatalf("expected criteria present to pass, got %+v", r)
	}
}

func TestCycle236_Predicates_GitDiffOrWaiver(t *testing.T) {
	ctx := context.Background()
	chunkWithWaiver := Chunk{WaiverRef: "WAI-1"}
	resWaiver := EvalPredicate(ctx, "git_diff_nonempty_or_waiver", chunkWithWaiver, EvalOptions{})
	if !resWaiver.OK || resWaiver.Detail != "waiver_ref set" {
		t.Fatalf("expected waiver_ref set, got %+v", resWaiver)
	}

	resNoRoot := EvalPredicate(ctx, "git_diff_nonempty_or_waiver", Chunk{}, EvalOptions{})
	if resNoRoot.OK || resNoRoot.Detail != "project root unknown" {
		t.Fatalf("expected project root unknown detail, got %+v", resNoRoot)
	}

	dir := t.TempDir()
	resGitErr := EvalPredicate(ctx, "git_diff_nonempty_or_waiver", Chunk{}, EvalOptions{ProjectRoot: dir})
	if resGitErr.OK || !strings.Contains(resGitErr.Detail, "git status failed") {
		t.Fatalf("expected git status failure in non-git dir, got %+v", resGitErr)
	}

	if !nonemptyListOrString("nonempty") || nonemptyListOrString("  ") {
		t.Fatal("unexpected nonemptyListOrString string behavior")
	}
	if !nonemptyListOrString([]any{1}) || nonemptyListOrString([]any{}) {
		t.Fatal("unexpected nonemptyListOrString []any behavior")
	}
	if !nonemptyListOrString([]string{"a"}) || nonemptyListOrString([]string{}) {
		t.Fatal("unexpected nonemptyListOrString []string behavior")
	}
	if !nonemptyListOrString(true) || nonemptyListOrString(nil) {
		t.Fatal("unexpected nonemptyListOrString default behavior")
	}
}

func TestCycle236_Predicates_LintAndShell(t *testing.T) {
	ctx := context.Background()
	resNoLint := EvalPredicate(ctx, "lint_ok_per_customization", Chunk{}, EvalOptions{})
	if !resNoLint.OK || !resNoLint.Skipped {
		t.Fatalf("expected skipped lint when no commands configured: %+v", resNoLint)
	}

	custLint := &Customization{
		CodeStyle: CodeStylePrefs{
			LintCommands: []string{"true"},
		},
	}
	optLintNoExec := EvalOptions{Customization: custLint, RunCommands: false}
	resLintDeferred := EvalPredicate(ctx, "lint_ok_per_customization", Chunk{}, optLintNoExec)
	if resLintDeferred.OK || !resLintDeferred.Skipped {
		t.Fatalf("expected deferred lint when RunCommands is false: %+v", resLintDeferred)
	}

	dir := t.TempDir()
	optLintSuccess := EvalOptions{
		ProjectRoot:    dir,
		Customization:  custLint,
		RunCommands:    true,
		CommandTimeout: 5 * time.Second,
	}
	resLintPass := EvalPredicate(ctx, "lint_ok_per_customization", Chunk{}, optLintSuccess)
	if !resLintPass.OK {
		t.Fatalf("expected lint pass with true command: %+v", resLintPass)
	}

	custFail := &Customization{
		CodeStyle: CodeStylePrefs{
			LintCommands: []string{"false"},
		},
	}
	optLintFail := EvalOptions{
		ProjectRoot:    dir,
		Customization:  custFail,
		RunCommands:    true,
		CommandTimeout: 5 * time.Second,
	}
	resLintFail := EvalPredicate(ctx, "lint_ok_per_customization", Chunk{}, optLintFail)
	if resLintFail.OK {
		t.Fatalf("expected lint fail with false command: %+v", resLintFail)
	}
}

func TestCycle236_Predicates_TestsExecutionModes(t *testing.T) {
	ctx := context.Background()
	chunkSchedulerOK := Chunk{EvidenceRefs: []string{"SCH-1234", paths.LogsDir + "/run.log"}}
	chunkSchedulerMissing := Chunk{EvidenceRefs: []string{"other-ref"}}

	custSched := &Customization{
		TestExecution: TestExecutionPrefs{
			Mode:                   "scheduler",
			SchedulerSubmitExample: "zqk scheduler submit test",
		},
	}
	optSched := EvalOptions{Customization: custSched}

	if r := EvalPredicate(ctx, "tests_ok_per_customization", chunkSchedulerOK, optSched); !r.OK {
		t.Fatalf("expected scheduler tests to pass: %+v", r)
	}
	if r := EvalPredicate(ctx, "tests_ok_per_customization", chunkSchedulerMissing, optSched); r.OK || !strings.Contains(r.Detail, "zqk scheduler submit test") {
		t.Fatalf("expected scheduler hint with example, got %+v", r)
	}

	custSchedNoEx := &Customization{
		TestExecution: TestExecutionPrefs{Mode: "scheduler"},
	}
	if r := EvalPredicate(ctx, "tests_ok_per_customization", chunkSchedulerMissing, EvalOptions{Customization: custSchedNoEx}); r.OK || strings.Contains(r.Detail, "; submit e.g.") {
		t.Fatalf("expected scheduler hint without example: %+v", r)
	}

	custCI := &Customization{TestExecution: TestExecutionPrefs{Mode: "ci"}}
	optCI := EvalOptions{Customization: custCI}
	chunkCIOK := Chunk{EvidenceRefs: []string{"ci://github-actions/run/1"}}
	if r := EvalPredicate(ctx, "tests_ok_per_customization", chunkCIOK, optCI); !r.OK {
		t.Fatalf("expected CI test evidence to pass: %+v", r)
	}
	if r := EvalPredicate(ctx, "tests_ok_per_customization", chunkSchedulerMissing, optCI); r.OK {
		t.Fatalf("expected CI test evidence missing to fail: %+v", r)
	}

	custHybrid := &Customization{TestExecution: TestExecutionPrefs{Mode: "hybrid"}}
	optHybrid := EvalOptions{Customization: custHybrid}
	chunkGoTest := Chunk{EvidenceRefs: []string{"go test -v ./pkg/..."}}
	chunkShellTest := Chunk{EvidenceRefs: []string{"sh ./run_test.sh"}}
	if r := EvalPredicate(ctx, "tests_ok_per_customization", chunkGoTest, optHybrid); !r.OK {
		t.Fatalf("expected hybrid go test to pass: %+v", r)
	}
	if r := EvalPredicate(ctx, "tests_ok_per_customization", chunkShellTest, optHybrid); !r.OK {
		t.Fatalf("expected hybrid shell test to pass: %+v", r)
	}
	if r := EvalPredicate(ctx, "tests_ok_per_customization", chunkSchedulerMissing, optHybrid); r.OK {
		t.Fatalf("expected hybrid missing to fail: %+v", r)
	}
}

func TestCycle236_Predicates_StageGatesAndPublishAck(t *testing.T) {
	ctx := context.Background()
	chunkOperate := Chunk{Stage: "operate_release"}
	chunkDesign := Chunk{Stage: "design"}

	custGate := &Customization{
		Security: StageGatePrefs{
			RequiredForStages: []string{"operate_release"},
			ScanCommands:      []string{"true"},
		},
		Performance: StageGatePrefs{
			RequiredForStages: []string{"operate_release"},
			ScanCommands:      []string{"true"},
		},
		CICD: CICDPrefs{
			RequiredChecks:     []string{"build", "lint"},
			PublishRequiresAck: true,
		},
	}
	opt := EvalOptions{Customization: custGate}

	resGateNA := EvalPredicate(ctx, "security_gate_ok_or_na", chunkDesign, opt)
	if !resGateNA.OK || !resGateNA.Skipped {
		t.Fatalf("expected security gate NA for design stage: %+v", resGateNA)
	}
	resPerfNA := EvalPredicate(ctx, "performance_gate_ok_or_na", chunkDesign, opt)
	if !resPerfNA.OK || !resPerfNA.Skipped {
		t.Fatalf("expected performance gate NA for design stage: %+v", resPerfNA)
	}

	chunkWithAck := Chunk{
		Stage:        "operate_release",
		EvidenceRefs: []string{"public_push_ack"},
	}
	if r := EvalPredicate(ctx, "publish_ack_present_if_public", chunkWithAck, opt); !r.OK {
		t.Fatalf("expected publish ack present to pass: %+v", r)
	}
	if r := EvalPredicate(ctx, "publish_ack_present_if_public", chunkOperate, opt); r.OK {
		t.Fatalf("expected publish ack missing in operate_release to fail: %+v", r)
	}
	if r := EvalPredicate(ctx, "publish_ack_present_if_public", chunkDesign, opt); !r.OK || !r.Skipped {
		t.Fatalf("expected publish ack in non-operate stage to be skipped: %+v", r)
	}

	custNoAck := &Customization{CICD: CICDPrefs{PublishRequiresAck: false}}
	if r := EvalPredicate(ctx, "publish_ack_present_if_public", chunkOperate, EvalOptions{Customization: custNoAck}); !r.OK || !r.Skipped {
		t.Fatalf("expected publish ack skipped when not required: %+v", r)
	}

	chunkCIIncomplete := Chunk{EvidenceRefs: []string{"build"}}
	chunkCIComplete := Chunk{EvidenceRefs: []string{"build", "lint"}}
	if r := EvalPredicate(ctx, "ci_required_checks_green_or_na", chunkCIIncomplete, opt); r.OK {
		t.Fatalf("expected incomplete CI checks to fail: %+v", r)
	}
	if r := EvalPredicate(ctx, "ci_required_checks_green_or_na", chunkCIComplete, opt); !r.OK {
		t.Fatalf("expected complete CI checks to pass: %+v", r)
	}

	chunkNoSmoke := Chunk{EvidenceRefs: []string{"unit-test-only"}}
	if r := EvalPredicate(ctx, "smoke_or_integration_evidence_present", chunkNoSmoke, opt); r.OK || !strings.Contains(r.Detail, "lack smoke/integration footprint") {
		t.Fatalf("expected lack of footprint error, got %+v", r)
	}
}

func TestCycle236_PredicatesExtended_ExitCodeAndContent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	rEmptyCmd := EvalPredicate(ctx, "command_exit_code:", Chunk{}, EvalOptions{})
	if rEmptyCmd.OK || rEmptyCmd.Detail != "missing command string" {
		t.Fatalf("expected missing command string detail, got %+v", rEmptyCmd)
	}

	rDeferCmd := EvalPredicate(ctx, "command_exit_code:echo hello", Chunk{}, EvalOptions{RunCommands: false})
	if !rDeferCmd.OK || !rDeferCmd.Skipped {
		t.Fatalf("expected command deferred when RunCommands=false: %+v", rDeferCmd)
	}

	optRun := EvalOptions{ProjectRoot: dir, RunCommands: true, CommandTimeout: 5 * time.Second}
	rPassCmd := EvalPredicate(ctx, "command_exit_code:true", Chunk{}, optRun)
	if !rPassCmd.OK {
		t.Fatalf("expected command true to pass: %+v", rPassCmd)
	}
	rFailCmd := EvalPredicate(ctx, "command_exit_code:false", Chunk{}, optRun)
	if rFailCmd.OK {
		t.Fatalf("expected command false to fail: %+v", rFailCmd)
	}

	if r := EvalPredicate(ctx, "content_hash_matches:", Chunk{}, EvalOptions{}); r.OK {
		t.Fatalf("expected empty relPath to fail: %+v", r)
	}
	if r := EvalPredicate(ctx, "content_hash_matches:file.txt", Chunk{}, EvalOptions{}); r.OK || !r.Skipped {
		t.Fatalf("expected empty project root to skip content_hash_matches: %+v", r)
	}
	if r := EvalPredicate(ctx, "content_hash_matches:missing.txt", Chunk{}, optRun); r.OK {
		t.Fatalf("expected nonexistent target file to fail content_hash_matches: %+v", r)
	}

	testFile := filepath.Join(dir, "target.txt")
	if err := fileutil.WriteFile(testFile, []byte("data"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if r := EvalPredicate(ctx, "content_hash_matches:target.txt", Chunk{}, optRun); !r.OK {
		t.Fatalf("expected existing file to pass content_hash_matches: %+v", r)
	}

	if r := EvalPredicate(ctx, "content_size_positive:", Chunk{}, optRun); r.OK {
		t.Fatalf("expected empty target to fail content_size_positive: %+v", r)
	}
	if r := EvalPredicate(ctx, "content_size_positive:target.txt", Chunk{}, optRun); !r.OK {
		t.Fatalf("expected positive file size to pass: %+v", r)
	}

	zeroFile := filepath.Join(dir, "zero.txt")
	if err := fileutil.WriteFile(zeroFile, []byte{}, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write zero-byte file: %v", err)
	}
	if r := EvalPredicate(ctx, "content_size_positive:zero.txt", Chunk{}, optRun); r.OK {
		t.Fatalf("expected zero-byte file to fail content_size_positive: %+v", r)
	}
}

func TestCycle236_PredicatesExtended_ASTSemanticMatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	if r := EvalPredicate(ctx, "ast_semantic_match:onepart", Chunk{}, EvalOptions{}); r.OK {
		t.Fatalf("expected error on invalid arg format: %+v", r)
	}
	if r := EvalPredicate(ctx, "ast_semantic_match:pkg:no_raw_panics", Chunk{}, EvalOptions{}); r.OK || !r.Skipped {
		t.Fatalf("expected skipped on empty ProjectRoot: %+v", r)
	}

	pkgDir := filepath.Join(dir, "mypkg")
	if err := fileutil.MkdirAll(pkgDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	srcClean := `package mypkg
func SafeFunc() string {
	return "hello"
}
`
	if err := fileutil.WriteFile(filepath.Join(pkgDir, "clean.go"), []byte(srcClean), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write clean.go: %v", err)
	}

	opt := EvalOptions{ProjectRoot: dir}
	rClean := EvalPredicate(ctx, "ast_semantic_match:mypkg:no_raw_panics", Chunk{}, opt)
	if !rClean.OK {
		t.Fatalf("expected no_raw_panics to pass on clean code: %+v", rClean)
	}

	rSymPresent := EvalPredicate(ctx, "ast_semantic_match:mypkg:symbol_present:SafeFunc", Chunk{}, opt)
	if !rSymPresent.OK {
		t.Fatalf("expected symbol SafeFunc to be present: %+v", rSymPresent)
	}

	rSymAbsent := EvalPredicate(ctx, "ast_semantic_match:mypkg:symbol_absent:ForbiddenFunc", Chunk{}, opt)
	if !rSymAbsent.OK {
		t.Fatalf("expected symbol ForbiddenFunc to be absent: %+v", rSymAbsent)
	}

	rSymAbsentFail := EvalPredicate(ctx, "ast_semantic_match:mypkg:symbol_absent:SafeFunc", Chunk{}, opt)
	if rSymAbsentFail.OK {
		t.Fatalf("expected symbol_absent to fail for SafeFunc: %+v", rSymAbsentFail)
	}

	rUnknown := EvalPredicate(ctx, "ast_semantic_match:mypkg:unknown_constraint", Chunk{}, opt)
	if rUnknown.OK {
		t.Fatalf("expected unknown constraint to fail: %+v", rUnknown)
	}

	srcPanic := `package mypkg
func UnsafeFunc() {
	panic("crash")
}
`
	if err := fileutil.WriteFile(filepath.Join(pkgDir, "panic.go"), []byte(srcPanic), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write panic.go: %v", err)
	}
	rPanic := EvalPredicate(ctx, "ast_semantic_match:mypkg:no_raw_panics", Chunk{}, opt)
	if rPanic.OK {
		t.Fatalf("expected no_raw_panics to fail on panic call: %+v", rPanic)
	}
}

func TestCycle236_Evaluate_StageGatesAllBranches(t *testing.T) {
	ctx := context.Background()

	stages := []struct {
		stage    string
		setChunk func(c *Chunk, val string)
	}{
		{"intent_capture", func(c *Chunk, v string) { c.GateIntent = v }},
		{"design", func(c *Chunk, v string) { c.GateDesign = v }},
		{"implement", func(c *Chunk, v string) { c.GateImplement = v }},
		{"integrate_verify", func(c *Chunk, v string) { c.GateIntegrate = v }},
		{"operate_release", func(c *Chunk, v string) { c.GateOperate = v }},
	}

	for _, s := range stages {
		chPass := Chunk{
			ChunkID:           "chk-" + s.stage,
			Stage:             s.stage,
			Claim:             "Valid claim for " + s.stage,
			RubricRef:         "RUB-1",
			DSLChecks:         []string{"standard_checks_pass"},
			EvidenceRefs:      []string{"ref-1"},
			IndependentVerify: "yes",
		}
		s.setChunk(&chPass, "yes")
		resPass := evaluateChunk(ctx, chPass, nil, []string{"yes", "na"}, EvalOptions{})
		if resPass.Verdict != "PASS" {
			t.Fatalf("expected stage %s with gate=yes to pass, got %+v", s.stage, resPass)
		}

		chInvalid := chPass
		s.setChunk(&chInvalid, "not_yes_or_na")
		resInvalid := evaluateChunk(ctx, chInvalid, nil, []string{"yes", "na"}, EvalOptions{})
		if resInvalid.Verdict != "FAIL" {
			t.Fatalf("expected stage %s with invalid gate to fail, got %+v", s.stage, resInvalid)
		}
	}

	chUnknown := Chunk{
		ChunkID: "chk-unknown",
		Stage:   "invalid_stage",
		Claim:   "claim",
	}
	resUnknown := evaluateChunk(ctx, chUnknown, nil, []string{"yes", "na"}, EvalOptions{})
	if resUnknown.Verdict != "FAIL" {
		t.Fatalf("expected unknown stage chunk to fail: %+v", resUnknown)
	}

	// Missing mandatory fields: claim, rubric_ref, dsl_checks, evidence_refs
	baseChunk := Chunk{
		ChunkID:           "chk-mand",
		Stage:             "design",
		Claim:             "claim",
		RubricRef:         "rub",
		DSLChecks:         []string{"standard_checks_pass"},
		EvidenceRefs:      []string{"ref"},
		IndependentVerify: "yes",
	}

	noClaim := baseChunk
	noClaim.Claim = ""
	if r := evaluateChunk(ctx, noClaim, nil, []string{"yes", "na"}, EvalOptions{}); r.Verdict != "FAIL" {
		t.Fatalf("expected missing claim to fail: %+v", r)
	}

	noRubric := baseChunk
	noRubric.RubricRef = ""
	if r := evaluateChunk(ctx, noRubric, nil, []string{"yes", "na"}, EvalOptions{}); r.Verdict != "FAIL" {
		t.Fatalf("expected missing rubric to fail: %+v", r)
	}

	noDSL := baseChunk
	noDSL.DSLChecks = nil
	if r := evaluateChunk(ctx, noDSL, nil, []string{"yes", "na"}, EvalOptions{}); r.Verdict != "FAIL" {
		t.Fatalf("expected missing dsl checks to fail: %+v", r)
	}

	noEvidence := baseChunk
	noEvidence.EvidenceRefs = nil
	if r := evaluateChunk(ctx, noEvidence, nil, []string{"yes", "na"}, EvalOptions{}); r.Verdict != "FAIL" {
		t.Fatalf("expected missing evidence refs to fail: %+v", r)
	}
}

func TestCycle236_Evaluate_SuggestionsAndTruncation(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("expected short string unchanged, got %s", got)
	}
	longStr := "This is a very long string that should be truncated properly by the truncate function."
	if got := truncate(longStr, 15); len(got) == 0 || !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncated string with ellipsis, got %s", got)
	}

	strs := []string{"apple", "banana", "apple", "", "   ", "orange"}
	u := uniqStrings(strs)
	if len(u) != 3 || u[0] != "apple" || u[1] != "banana" || u[2] != "orange" {
		t.Fatalf("unexpected uniqStrings result: %v", u)
	}

	cust := &Customization{
		TestExecution: TestExecutionPrefs{
			Mode:                   "scheduler",
			SchedulerSubmitExample: "zqk scheduler submit test_job",
		},
	}
	badChunk := Chunk{
		ChunkID: "",
		Stage:   "invalid_stage",
	}
	preds := []PredicateResult{
		{Predicate: "object_exists:OBJ-1", OK: false, Detail: "missing"},
	}
	suggestions := suggestNext(badChunk, preds, EvalOptions{Customization: cust})
	if len(suggestions) < 3 {
		t.Fatalf("expected at least 3 suggestions, got %v", suggestions)
	}
}

func TestCycle236_Evaluate_VerdictsAndNilReceivers(t *testing.T) {
	ctx := context.Background()

	chPass := Chunk{
		ChunkID:           "chk-pass",
		Stage:             "design",
		Claim:             "claim pass",
		RubricRef:         "RUB-1",
		DSLChecks:         []string{"standard_checks_pass"},
		EvidenceRefs:      []string{"ref-1"},
		IndependentVerify: "yes",
	}
	chWaived := Chunk{
		ChunkID:           "chk-waived",
		Stage:             "design",
		Claim:             "claim waived",
		RubricRef:         "RUB-1",
		DSLChecks:         []string{"standard_checks_pass"},
		EvidenceRefs:      []string{"ref-1"},
		WaiverRef:         "WAI-123",
		IndependentVerify: "no",
	}
	chFail := Chunk{
		ChunkID:           "chk-fail",
		Stage:             "design",
		Claim:             "claim fail",
		RubricRef:         "RUB-1",
		DSLChecks:         []string{"object_exists:"},
		EvidenceRefs:      []string{"ref-1"},
		IndependentVerify: "invalid_verify_value",
	}

	repAllPass := Evaluate(ctx, []Chunk{chPass}, nil, nil, EvalOptions{})
	if repAllPass.Verdict != "PASS" || !repAllPass.Passed() {
		t.Fatalf("expected repAllPass to pass, got %+v", repAllPass)
	}

	repWaived := Evaluate(ctx, []Chunk{chPass, chWaived}, nil, nil, EvalOptions{})
	if repWaived.Verdict != "WAIVED" || !repWaived.Passed() {
		t.Fatalf("expected repWaived to have WAIVED verdict, got %+v", repWaived)
	}

	repFail := Evaluate(ctx, []Chunk{chPass, chFail}, nil, nil, EvalOptions{})
	if repFail.Verdict != "FAIL" || repFail.Passed() {
		t.Fatalf("expected repFail to fail, got %+v", repFail)
	}
	if len(repFail.FailedChunkIDs) != 1 || repFail.FailedChunkIDs[0] != "chk-fail" {
		t.Fatalf("expected chk-fail in FailedChunkIDs: %v", repFail.FailedChunkIDs)
	}
	passedIDs := repFail.PassedChunkIDs()
	if len(passedIDs) != 1 || passedIDs[0] != "chk-pass" {
		t.Fatalf("expected only chk-pass in PassedChunkIDs: %v", passedIDs)
	}

	var nilReport *Report
	if nilReport.Passed() {
		t.Fatal("nil report Passed() should return false")
	}
	if nilReport.PassedChunkIDs() != nil {
		t.Fatal("nil report PassedChunkIDs() should return nil")
	}
}

func TestCycle236_Report_RenderBriefsAndEscape(t *testing.T) {
	if got := escapeCell("hello|world\nnext"); got != "hello/world next" {
		t.Fatalf("unexpected escapeCell result: %s", got)
	}
	if got := RenderAgentBrief(nil); got != "" {
		t.Fatalf("expected empty string for nil report brief, got %s", got)
	}
	if got := RenderChecklistBrief(nil); got != "" {
		t.Fatalf("expected empty string for nil checklist brief, got %s", got)
	}

	rep := &Report{
		Verdict:  "FAIL",
		PolicyID: "POL-WORKFLOW-VDS",
		Tone:     "Test Tone",
		Culture:  []string{"Culture rule 1"},
		Chunks: []ChunkResult{
			{
				ChunkID: "chk-1",
				Stage:   "design",
				Verdict: "FAIL",
				Claim:   "Test claim",
				Checklist: []CheckItem{
					{ID: "c1", OK: true, Detail: "ok detail"},
					{ID: "c2", OK: false, Detail: "fail|detail"},
				},
				NextActions: []string{"Fix c2"},
			},
		},
		NextActions: []string{"Global action"},
	}
	brief := RenderAgentBrief(rep)
	if !strings.Contains(brief, "fail/detail") || !strings.Contains(brief, "Global action") {
		t.Fatalf("rendered brief missing expected sections: %s", brief)
	}

	checklist := BuildChecklist(nil, nil, GlossaryRefs{})
	clBrief := RenderChecklistBrief(checklist)
	if !strings.Contains(clBrief, "POL-WORKFLOW-VDS") || !strings.Contains(clBrief, "Chunk contract") {
		t.Fatalf("rendered checklist brief missing expected content: %s", clBrief)
	}
}

func TestCycle236_Glossary_MixedAndCultureVariations(t *testing.T) {
	ctx := context.Background()

	custMixed := &Customization{
		Glossary: GlossaryBinding{
			TermRef: "GLS-TERM-123",
		},
	}
	lookup := func(c context.Context, kind, title string) (string, error) {
		if title == GlossaryAcronymTitle {
			return "GLS-ACRONYM-456", nil
		}
		return "", nil
	}
	glsMixed := ResolveGlossary(ctx, custMixed, lookup)
	if glsMixed.ResolvedBy != "mixed" {
		t.Fatalf("expected ResolvedBy=mixed, got %s", glsMixed.ResolvedBy)
	}

	lineFull := CultureGlossaryLine(GlossaryRefs{
		TermRef:    "GLS-1",
		AcronymRef: "GLS-2",
	})
	if !strings.Contains(lineFull, "term_ref=GLS-1") || !strings.Contains(lineFull, "acronym_ref=GLS-2") {
		t.Fatalf("unexpected full glossary line: %s", lineFull)
	}

	lineTermOnly := CultureGlossaryLine(GlossaryRefs{
		TermRef: "GLS-1",
	})
	if !strings.Contains(lineTermOnly, "term_ref=GLS-1") || strings.Contains(lineTermOnly, "acronym_ref=") {
		t.Fatalf("unexpected term-only glossary line: %s", lineTermOnly)
	}

	if id := IDFromObject(nil); id != "" {
		t.Fatalf("expected empty id from nil object, got %s", id)
	}
	if id := IDFromObject(map[string]any{"other": "val"}); id != "" {
		t.Fatalf("expected empty id when missing id key, got %s", id)
	}
	if id := IDFromObject(map[string]any{objects.FieldKeyID: 12345}); id != "" {
		t.Fatalf("expected empty id when id is non-string, got %s", id)
	}
	if id := IDFromObject(map[string]any{objects.FieldKeyID: "OBJ-VALID"}); id != "OBJ-VALID" {
		t.Fatalf("expected OBJ-VALID, got %s", id)
	}
}

func TestCycle236_Concurrent_EvaluationAndPredicates(t *testing.T) {
	ctx := context.Background()
	const numGoroutines = 8
	const numIterations = 20

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*numIterations)

	for i := 0; i < numGoroutines; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine(
			fmt.Sprintf("vds_concurrent_worker_%d", workerID),
			"concurrent chunk evaluation test",
		).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				ch := Chunk{
					ChunkID:           fmt.Sprintf("chk-%d-%d", workerID, j),
					Stage:             "design",
					Claim:             fmt.Sprintf("Concurrent claim %d-%d", workerID, j),
					RubricRef:         "RUB-CONCURRENT",
					DSLChecks:         []string{"standard_checks_pass"},
					EvidenceRefs:      []string{fmt.Sprintf("evidence-%d-%d", workerID, j)},
					IndependentVerify: "yes",
				}
				cr := evaluateChunk(ctx, ch, nil, []string{"yes", "na"}, EvalOptions{})
				if cr.Verdict != "PASS" {
					errCh <- fmt.Errorf("worker %d iter %d expected PASS, got %s", workerID, j, cr.Verdict)
					return
				}
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent evaluation error: %v", err)
		}
	}
}
