//go:build !zqk_omit_workpack

package workpack

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestRegisterAndLinkedRoot_Comprehensive tests Register with nil and valid cobra command.
func TestRegisterAndLinkedRoot_Comprehensive(t *testing.T) {
	origLinked := linkedRoot
	origEnabled := enabled
	defer func() {
		linkedRoot = origLinked
		enabled = origEnabled
	}()

	linkedRoot = false
	enabled = false
	Register(nil)
	if LinkedRoot() {
		t.Fatalf("expected LinkedRoot to be false after Register(nil)")
	}
	if !Enabled() {
		t.Fatalf("expected pack to be enabled after Register(nil)")
	}

	cmd := &cobra.Command{Use: "test-root"}
	Register(cmd)
	if !LinkedRoot() {
		t.Fatalf("expected LinkedRoot to be true after Register(cmd)")
	}
}

// TestVerifyOwnedKinds_Direct tests verifyOwnedKinds directly.
func TestVerifyOwnedKinds_Direct(t *testing.T) {
	specs, err := verifyOwnedKinds()
	if err != nil {
		t.Fatalf("verifyOwnedKinds failed in repo root: %v", err)
	}
	if len(specs) != len(Kinds()) {
		t.Fatalf("verifyOwnedKinds returned %d specs, want %d", len(specs), len(Kinds()))
	}
}

// TestRedundancyReport_StringAndSortedCopy tests String formatting and deterministic sorted output.
func TestRedundancyReport_StringAndSortedCopy(t *testing.T) {
	cleanRep := &RedundancyReport{
		Path: "packs/work/specs/goal.yaml",
	}
	if got := cleanRep.String(); got != fmt.Sprintf(msgSpecClean, cleanRep.Path) {
		t.Fatalf("clean report string mismatch: got %q want %q", got, fmt.Sprintf(msgSpecClean, cleanRep.Path))
	}

	violRep := &RedundancyReport{
		Path:       "packs/work/specs/bad.yaml",
		Redundant:  []string{"status", "created_at"},
		Duplicates: []string{"duplicate_field", "another_dup"},
	}
	violStr := violRep.String()
	wantRedundant := "created_at status"
	wantDuplicates := "another_dup duplicate_field"
	expected := fmt.Sprintf(msgSpecViolate, "packs/work/specs/bad.yaml", wantRedundant, wantDuplicates)
	if violStr != expected {
		t.Fatalf("violating report string mismatch:\ngot:  %q\nwant: %q", violStr, expected)
	}

	// Direct sortedCopy test
	raw := []string{"zebra", "apple", "mango"}
	sorted := sortedCopy(raw)
	if !reflect.DeepEqual(sorted, []string{"apple", "mango", "zebra"}) {
		t.Fatalf("sortedCopy mismatch: got %v", sorted)
	}
	// Verify sortedCopy makes a copy without mutating the original
	if raw[0] != "zebra" {
		t.Fatalf("sortedCopy mutated original slice: %v", raw)
	}
}

// TestUnique_AppendAndDeduplicate tests unique helper covering both branches.
func TestUnique_AppendAndDeduplicate(t *testing.T) {
	initial := []string{"alpha", "beta"}
	// Branch 1: element already present
	res1 := unique(initial, "beta")
	if len(res1) != 2 || res1[0] != "alpha" || res1[1] != "beta" {
		t.Fatalf("unique with existing item failed: %v", res1)
	}
	// Branch 2: element not present
	res2 := unique(res1, "gamma")
	if len(res2) != 3 || res2[2] != "gamma" {
		t.Fatalf("unique with new item failed: %v", res2)
	}
}

// TestUniqueMany_Variations tests uniqueMany helper for empty, unique, and duplicated elements.
func TestUniqueMany_Variations(t *testing.T) {
	// Empty slice
	if len(uniqueMany(nil)) != 0 {
		t.Fatalf("expected empty slice for nil")
	}
	if len(uniqueMany([]string{})) != 0 {
		t.Fatalf("expected empty slice for empty")
	}

	// Single element
	single := uniqueMany([]string{"foo"})
	if !reflect.DeepEqual(single, []string{"foo"}) {
		t.Fatalf("single element mismatch: %v", single)
	}

	// Multiple elements with duplicates and out of order
	input := []string{"delta", "alpha", "beta", "alpha", "gamma", "delta", "delta", "beta"}
	got := uniqueMany(input)
	want := []string{"alpha", "beta", "delta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueMany mismatch:\ngot:  %v\nwant: %v", got, want)
	}
}

// TestCollectFieldNames_TopLevelNonMapAndMetaKeys tests collectFieldNames handling non-map values
// and meta-keys when doc does not have an explicit fields mapping.
func TestCollectFieldNames_TopLevelNonMapAndMetaKeys(t *testing.T) {
	doc := map[string]any{
		"ontology":       "goal",              // meta key
		"plane":          "transactional",     // meta key
		"description":    "A high level goal", // meta key
		"non_map_scalar": "some-string-value", // non-map top-level value
		"non_map_slice":  []string{"a", "b"},  // non-map top-level value
		"custom_field": map[string]any{ // valid map top-level field
			"type": "string",
		},
		"another_field": map[string]any{ // valid map top-level field
			"type": "integer",
		},
	}
	names := collectFieldNames(doc)
	want := []string{"another_field", "custom_field"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("collectFieldNames mismatch:\ngot:  %v\nwant: %v", names, want)
	}
}

// TestAuditSpec_EdgeCases tests AuditSpec with empty canonical fields and multiple edge cases.
func TestAuditSpec_EdgeCases(t *testing.T) {
	// Empty field names or field names consisting solely of separators (resulting in empty canonical string)
	doc := map[string]any{
		"fields": map[string]any{
			"":            map[string]any{"type": "string"},
			"___":         map[string]any{"type": "string"},
			"--":          map[string]any{"type": "string"},
			"valid_field": map[string]any{"type": "string"},
		},
	}
	report := AuditSpec(doc)
	if !report.Clean() {
		t.Fatalf("expected clean report for valid_field with empty separator names, got %+v", report)
	}

	// Duplicate base field and duplicate custom field in same spec
	docWithBoth := map[string]any{
		"fields": map[string]any{
			"status":      map[string]any{"type": "string"},
			"Status":      map[string]any{"type": "string"}, // duplicate AND redundant base field
			"custom_one":  map[string]any{"type": "string"},
			"custom-one":  map[string]any{"type": "string"}, // duplicate custom field
			"archived_by": map[string]any{"type": "string"}, // redundant base field
		},
	}
	repBoth := AuditSpec(docWithBoth)
	if repBoth.Clean() {
		t.Fatalf("expected violations for docWithBoth, got clean")
	}
	if !reflect.DeepEqual(wantList(repBoth.Redundant), wantList([]string{"archived_by", "status"})) {
		t.Fatalf("redundant mismatch: %v", repBoth.Redundant)
	}
	if !reflect.DeepEqual(wantList(repBoth.Duplicates), wantList([]string{"custom_one", "status"})) {
		t.Fatalf("duplicates mismatch: %v", repBoth.Duplicates)
	}
}

// TestDecodeSpecDocument_InvalidAndEmptyYAML tests decodeSpecDocument with malformed and empty data.
func TestDecodeSpecDocument_InvalidAndEmptyYAML(t *testing.T) {
	// Invalid YAML
	badYAML := []byte("::: this is not valid yaml :::")
	doc1 := decodeSpecDocument(badYAML)
	if doc1 == nil || len(doc1) != 0 {
		t.Fatalf("expected empty map for invalid YAML, got %v", doc1)
	}

	// Empty YAML string
	doc2 := decodeSpecDocument([]byte(""))
	if doc2 == nil || len(doc2) != 0 {
		t.Fatalf("expected empty map for empty YAML, got %v", doc2)
	}

	// Only whitespace / comments
	doc3 := decodeSpecDocument([]byte("# Just a comment\n\n"))
	if doc3 == nil || len(doc3) != 0 {
		t.Fatalf("expected empty map for comment YAML, got %v", doc3)
	}

	// Valid YAML
	validYAML := []byte("ontology: goal\nplane: transactional\n")
	doc4 := decodeSpecDocument(validYAML)
	if doc4["ontology"] != "goal" || doc4["plane"] != "transactional" {
		t.Fatalf("valid YAML decode mismatch: %v", doc4)
	}
}

// TestVerifiedPaths_NilAndNonNil tests VerifiedPaths when global map is nil and populated.
func TestVerifiedPaths_NilAndNonNil(t *testing.T) {
	// Preserve original state
	origSpecs := verifiedSpecs
	defer func() { verifiedSpecs = origSpecs }()

	verifiedSpecs = nil
	paths := VerifiedPaths()
	if paths == nil || len(paths) != 0 {
		t.Fatalf("expected empty map for nil verifiedSpecs, got %v", paths)
	}

	verifiedSpecs = map[string]string{
		"goal": "packs/work/specs/goal.yaml",
	}
	paths2 := VerifiedPaths()
	if len(paths2) != 1 || paths2["goal"] != "packs/work/specs/goal.yaml" {
		t.Fatalf("paths2 mismatch: %v", paths2)
	}

	// Mutating returned map must not mutate internal state
	paths2["goal"] = "mutated"
	if verifiedSpecs["goal"] != "packs/work/specs/goal.yaml" {
		t.Fatalf("VerifiedPaths did not return a safe copy")
	}
}

// TestVerifiedSpec_NilAndLookup tests VerifiedSpec lookup behavior.
func TestVerifiedSpec_NilAndLookup(t *testing.T) {
	origSpecs := verifiedSpecs
	defer func() { verifiedSpecs = origSpecs }()

	verifiedSpecs = nil
	if path, ok := VerifiedSpec("goal"); ok || path != "" {
		t.Fatalf("expected false, empty for nil verifiedSpecs, got %q, %v", path, ok)
	}

	verifiedSpecs = map[string]string{
		"goal": "packs/work/specs/goal.yaml",
	}
	if path, ok := VerifiedSpec("goal"); !ok || path != "packs/work/specs/goal.yaml" {
		t.Fatalf("expected goal spec, got %q, %v", path, ok)
	}
	if path, ok := VerifiedSpec("unknown"); ok || path != "" {
		t.Fatalf("expected false for unknown kind, got %q, %v", path, ok)
	}
}

// TestVerifyError tests VerifyError returns current verifyErr.
func TestVerifyError(t *testing.T) {
	origErr := verifyErr
	defer func() { verifyErr = origErr }()

	testErr := fmt.Errorf("sample verification error")
	verifyErr = testErr
	if VerifyError() != testErr {
		t.Fatalf("VerifyError() mismatch: got %v want %v", VerifyError(), testErr)
	}

	verifyErr = nil
	if VerifyError() != nil {
		t.Fatalf("expected nil VerifyError, got %v", VerifyError())
	}
}

// TestAuditAll_EnabledAndDisabled tests AuditAll in disabled, corrupted, and clean states.
func TestAuditAll_EnabledAndDisabled(t *testing.T) {
	origEnabled := enabled
	origSpecs := verifiedSpecs
	origErr := verifyErr
	defer func() {
		enabled = origEnabled
		verifiedSpecs = origSpecs
		verifyErr = origErr
	}()

	// Case 1: Disabled pack returns error
	enabled = false
	verifyErr = fmt.Errorf("verification failed previously")
	rep, err := AuditAll()
	if err == nil {
		t.Fatalf("expected error from AuditAll when pack is disabled")
	}
	if rep != nil {
		t.Fatalf("expected nil report on AuditAll failure")
	}

	// Case 2: Enabled pack with an invalid spec path
	enabled = true
	verifyErr = nil
	verifiedSpecs = map[string]string{
		"missing_kind": filepath.Join(t.TempDir(), "nonexistent.yaml"),
	}
	rep2, err2 := AuditAll()
	if err2 == nil {
		t.Fatalf("expected error from AuditAll when spec file is missing")
	}
	if rep2 != nil {
		t.Fatalf("expected nil report on missing spec file")
	}

	// Case 3: Enabled pack with empty kind key (skips empty kind)
	tempDir := t.TempDir()
	validSpecPath := filepath.Join(tempDir, "goal.yaml")
	specContent := "ontology: goal\nfields:\n  goal_summary:\n    type: string\n"
	if err := fileutil.WriteFile(validSpecPath, []byte(specContent), 0o600); err != nil {
		t.Fatal(err)
	}
	verifiedSpecs = map[string]string{
		"":     validSpecPath, // empty kind
		"goal": validSpecPath,
	}
	rep3, err3 := AuditAll()
	if err3 != nil {
		t.Fatalf("unexpected error from AuditAll with valid spec: %v", err3)
	}
	if !rep3.Clean() {
		t.Fatalf("expected clean report from rep3, got %+v", rep3)
	}

	// Case 4: Full canonical AuditAll against real repository files
	Enable()
	if !Enabled() {
		t.Fatalf("failed to Enable workpack")
	}
	canonicalRep, err := AuditAll()
	if err != nil {
		t.Fatalf("canonical AuditAll failed: %v", err)
	}
	// AuditAll successfully audits all verified specs and reports findings deterministically
	if canonicalRep.Path != SpecDir {
		t.Fatalf("expected report path %q, got %q", SpecDir, canonicalRep.Path)
	}
	if len(canonicalRep.Duplicates) != 0 {
		t.Fatalf("expected zero duplicate field declarations in canonical specs, got: %v", canonicalRep.Duplicates)
	}
}

// TestVerifyKindFiles_MissingLifecycleFile tests verifyKindFiles when a kind requiring lifecycle is missing it.
func TestVerifyKindFiles_MissingLifecycleFile(t *testing.T) {
	tempRoot := t.TempDir()
	specDir := filepath.Join(tempRoot, "specs")
	lifecycleDir := filepath.Join(tempRoot, "lifecycles")
	if err := fileutil.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(lifecycleDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create spec for goal
	goalSpec := filepath.Join(specDir, "goal.yaml")
	if err := fileutil.WriteFile(goalSpec, []byte("ontology: goal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Intentionally do NOT create goal_lifecycle.yaml

	_, err := verifyKindFiles(tempRoot, []string{"goal"}, "specs", "lifecycles")
	if err == nil {
		t.Fatalf("expected error for missing lifecycle file, got nil")
	}
	if !strings.Contains(err.Error(), "lifecycle") {
		t.Fatalf("expected error mentioning lifecycle, got %v", err)
	}

	// Verify kindsWithoutLifecycle (e.g. objects.KindWorkInterval) succeed without lifecycle file
	intervalSpec := filepath.Join(specDir, objects.KindWorkInterval+".yaml")
	if err := fileutil.WriteFile(intervalSpec, []byte("ontology: work_interval\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec, err := verifyKindFiles(tempRoot, []string{objects.KindWorkInterval}, "specs", "lifecycles")
	if err != nil {
		t.Fatalf("expected kindsWithoutLifecycle to pass without lifecycle file, got %v", err)
	}
	if rec[objects.KindWorkInterval] != intervalSpec {
		t.Fatalf("recorded spec mismatch: got %q want %q", rec[objects.KindWorkInterval], intervalSpec)
	}
}

// TestModuleRoot_ClimbAndNotFound tests moduleRoot when climbing directories and when go.mod is absent or foreign.
func TestModuleRoot_ClimbAndNotFound(t *testing.T) {
	origWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.Chdir(origWd) }()

	// Normal call should succeed and return root
	root, err := moduleRoot()
	if err != nil {
		t.Fatalf("expected moduleRoot to find root, got %v", err)
	}
	if _, err := fileutil.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("moduleRoot returned invalid root: %v", err)
	}

	// Test in a temp directory without go.mod where search fails
	emptyTemp := t.TempDir()
	if err := fileutil.Chdir(emptyTemp); err != nil {
		t.Fatal(err)
	}
	_, notFoundErr := moduleRoot()
	if notFoundErr == nil {
		t.Fatalf("expected error from moduleRoot in empty dir, got nil")
	}

	// Test in a temp directory with a foreign go.mod (not module github.com/zqk-os/zqk)
	foreignDir := filepath.Join(t.TempDir(), "foreign")
	subDir := filepath.Join(foreignDir, "sub", "deep")
	if err := fileutil.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreignGoMod := filepath.Join(foreignDir, "go.mod")
	if err := fileutil.WriteFile(foreignGoMod, []byte("module github.com/foreign/project\n\ngo 1.22\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.Chdir(subDir); err != nil {
		t.Fatal(err)
	}
	_, foreignErr := moduleRoot()
	if foreignErr == nil {
		t.Fatalf("expected error from moduleRoot with foreign go.mod, got nil")
	}

	// Test when working directory is removed (triggers Getwd error)
	delDir := filepath.Join(t.TempDir(), "to_delete")
	if err := fileutil.MkdirAll(delDir, 0o755); err == nil {
		if err := fileutil.Chdir(delDir); err == nil {
			_ = fileutil.Remove(delDir)
			_, _ = moduleRoot()
		}
	}
}

// TestEnable_FallbackOutsideDevRepo tests Enable fallback path when moduleRoot fails outside the repository.
func TestEnable_FallbackOutsideDevRepo(t *testing.T) {
	origWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	origEnabled := enabled
	origSpecs := verifiedSpecs
	origErr := verifyErr
	defer func() {
		_ = fileutil.Chdir(origWd)
		enabled = origEnabled
		verifiedSpecs = origSpecs
		verifyErr = origErr
	}()

	emptyTemp := t.TempDir()
	if err := fileutil.Chdir(emptyTemp); err != nil {
		t.Fatal(err)
	}

	enabled = false
	verifiedSpecs = nil
	verifyErr = nil

	Enable()
	if !Enabled() {
		t.Fatalf("expected pack to be enabled in fallback mode")
	}
	if VerifyError() != nil {
		t.Fatalf("expected VerifyError to be nil in fallback mode, got %v", VerifyError())
	}
	for _, k := range Kinds() {
		path, ok := VerifiedSpec(k)
		if !ok || path != k+".yaml" {
			t.Fatalf("expected fallback spec %s.yaml for kind %s, got %q (ok=%v)", k, k, path, ok)
		}
	}
}

// TestWorkpack_ConcurrentSafety verifies thread-safety of pure and read-heavy functions across goroutines.
func TestWorkpack_ConcurrentSafety(t *testing.T) {
	// Initialize pack
	Enable()

	const workers = 20
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("workpack_coverage_test", fmt.Sprintf("worker %d", workerID)).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// NormalizeField concurrent calls
				_ = NormalizeField(fmt.Sprintf("CamelCaseField_%d_%d", workerID, j))
				_ = NormalizeField("multi__sep--test")

				// BaseFieldSet
				base := BaseFieldSet()
				if !base["id"] || !base["created_at"] {
					t.Errorf("worker %d: missing base fields", workerID)
					return
				}

				// Kinds and BuilderCount
				kinds := Kinds()
				if len(kinds) != BuilderCount() {
					t.Errorf("worker %d: kind count mismatch", workerID)
					return
				}

				// VerifiedPaths and VerifiedSpec
				paths := VerifiedPaths()
				if len(paths) == 0 {
					t.Errorf("worker %d: empty paths", workerID)
					return
				}
				for _, k := range kinds {
					if _, ok := VerifiedSpec(k); !ok {
						t.Errorf("worker %d: missing verified spec for %s", workerID, k)
						return
					}
				}

				// AuditSpec concurrent evaluations
				doc := map[string]any{
					"ontology": "goal",
					"fields": map[string]any{
						"title":       map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
					},
				}
				report := AuditSpec(doc)
				if !report.Clean() {
					t.Errorf("worker %d: unexpected dirty report: %+v", workerID, report)
					return
				}
			}
		})
	}

	wg.Wait()
}
