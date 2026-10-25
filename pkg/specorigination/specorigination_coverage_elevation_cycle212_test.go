// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package specorigination

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// helper to set up a comprehensive test repo for spec origination
func setupCycle212TestRepo(t *testing.T, ontology, namespace string, prefixes []string) string {
	t.Helper()
	root := t.TempDir()

	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	configsDir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	processDir := filepath.Join(root, paths.ProcessInternalDir)

	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create specs dir: %v", err)
	}
	if err := fileutil.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create process dir: %v", err)
	}

	specContent := fmt.Sprintf(`schema_version: "2.0.0"
ontology: "%s"
visibility: "public"
namespace: "%s"
description: "Cycle 212 test spec"
traits: []
fields:
  name:
    type: string
    required: true
`, ontology, namespace)

	if len(prefixes) > 0 {
		var prefixLines strings.Builder
		prefixLines.WriteString("id_prefixes:\n")
		for _, p := range prefixes {
			prefixLines.WriteString(fmt.Sprintf("  - %q\n", p))
		}
		specContent += prefixLines.String()
	}

	specPath := filepath.Join(specsDir, ontology+".yaml")
	if err := fileutil.WriteFile(specPath, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	kmContent := `$schema: kind_mappings_schema.json

backends:
  default:
    kind_to_directory:
      seed_kind: "seed_kinds"
    directory_to_kind:
      seed_kinds: "seed_kind"
`
	if err := fileutil.WriteFile(filepath.Join(configsDir, paths.KindMappingsConfigFile), []byte(kmContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write kind mappings: %v", err)
	}

	idpContent := `$schema: id_prefixes_schema.json

kind_to_prefixes:
  seed_kind:
    - "SDK-"
`
	if err := fileutil.WriteFile(filepath.Join(configsDir, paths.IdPrefixesConfigFile), []byte(idpContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write id prefixes: %v", err)
	}

	nsContent := fmt.Sprintf(`$schema: namespaces_schema.json

namespaces:
  "%s":
    description: "Namespace for testing"
    kinds:
      - "seed_kind"
`, namespace)
	if err := fileutil.WriteFile(filepath.Join(configsDir, paths.NamespacesConfigFile), []byte(nsContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write namespaces: %v", err)
	}

	return root
}

// helper to install a mock admin binary in sibling directory
func installMockAdminScript(t *testing.T, scriptBody string) (string, func()) {
	t.Helper()
	exe, err := fileutil.Executable()
	if err != nil {
		t.Fatalf("fileutil.Executable error: %v", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatalf("EvalSymlinks error: %v", err)
	}

	adminPath := filepath.Join(filepath.Dir(exe), adminBinaryName)
	if err := fileutil.WriteFile(adminPath, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("failed to write mock admin binary: %v", err)
	}

	cleanup := func() {
		if rmErr := fileutil.Remove(adminPath); rmErr != nil && !fileutil.IsNotExist(rmErr) {
			t.Errorf("failed to clean up mock admin: %v", rmErr)
		}
	}
	return adminPath, cleanup
}

func TestCycle212_ValidateOptions_Comprehensive(t *testing.T) {
	testCases := []struct {
		name        string
		opts        Options
		expectError bool
		errContains string
	}{
		{
			name:        "missing project root",
			opts:        Options{ProjectRoot: "", Ontology: "valid_stem"},
			expectError: true,
			errContains: "ProjectRoot is required",
		},
		{
			name:        "missing ontology",
			opts:        Options{ProjectRoot: "/tmp/test", Ontology: ""},
			expectError: true,
			errContains: "Ontology is required",
		},
		{
			name:        "invalid ontology uppercase",
			opts:        Options{ProjectRoot: "/tmp/test", Ontology: "BadStem"},
			expectError: true,
			errContains: "must match",
		},
		{
			name:        "invalid ontology leading digit",
			opts:        Options{ProjectRoot: "/tmp/test", Ontology: "1stem"},
			expectError: true,
			errContains: "must match",
		},
		{
			name:        "invalid ontology hyphen",
			opts:        Options{ProjectRoot: "/tmp/test", Ontology: "bad-stem"},
			expectError: true,
			errContains: "must match",
		},
		{
			name:        "valid lowercase with underscores",
			opts:        Options{ProjectRoot: "/tmp/test", Ontology: "valid_stem_123"},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOptions(tc.opts)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for case %q, got nil", tc.name)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestCycle212_AuthoredFieldValidationSpec(t *testing.T) {
	if got := authoredFieldValidationSpec(nil); got != nil {
		t.Fatalf("expected nil for nil spec, got %v", got)
	}

	spec := &objects.Spec{
		Ontology: "test_kind",
		Fields: map[string]any{
			"custom_field": map[string]any{"type": "string"},
		},
		ResolvedFields: map[string]any{
			"custom_field":   map[string]any{"type": "string"},
			"parent_field_1": map[string]any{"type": "int"},
			"parent_field_2": map[string]any{"type": "bool"},
		},
	}

	scoped := authoredFieldValidationSpec(spec)
	if scoped == nil {
		t.Fatal("expected non-nil scoped spec")
	}
	if len(scoped.ResolvedFields) != 1 {
		t.Fatalf("expected 1 field in ResolvedFields, got %d", len(scoped.ResolvedFields))
	}
	if _, ok := scoped.ResolvedFields["custom_field"]; !ok {
		t.Errorf("expected custom_field to be retained in ResolvedFields")
	}
	if _, ok := scoped.ResolvedFields["parent_field_1"]; ok {
		t.Errorf("parent_field_1 should not be in authored ResolvedFields")
	}
}

func TestCycle212_StageNamesAndConstants(t *testing.T) {
	if PipelineKind != "spec.origination" {
		t.Fatalf("PipelineKind mismatch: %q", PipelineKind)
	}

	stages := []string{
		StageIngest,
		StageNormalize,
		StageDecide,
		StageCommit,
		StageCrossMembrane,
		StageMaterializeIndexes,
		StageTriggerSideEffects,
		StageFinalize,
	}

	seen := make(map[string]bool)
	for _, stage := range stages {
		if stage == "" {
			t.Fatal("empty stage name constant detected")
		}
		if seen[stage] {
			t.Fatalf("duplicate stage name: %q", stage)
		}
		seen[stage] = true
	}
}

func TestCycle212_TruncateRunes_Boundaries(t *testing.T) {
	if got := truncateRunes("anything", 0); got != "" {
		t.Fatalf("expected empty string for maxRunes=0, got %q", got)
	}
	if got := truncateRunes("anything", -5); got != "" {
		t.Fatalf("expected empty string for negative maxRunes, got %q", got)
	}
	if got := truncateRunes("hello", 5); got != "hello" {
		t.Fatalf("expected exact match for maxRunes=len, got %q", got)
	}
	if got := truncateRunes("hello world", 5); got != "hello…" {
		t.Fatalf("expected truncated with ellipsis, got %q", got)
	}
}

func TestCycle212_AdminTriggerBinaryCandidates_Variations(t *testing.T) {
	// Testing adminTriggerBinaryPath
	if got := adminTriggerBinaryPath("/usr/bin/zqk-admin"); got != "/usr/bin/zqk-admin" {
		t.Fatalf("unexpected adminTriggerBinaryPath: %q", got)
	}
	if got := adminTriggerBinaryPath("/usr/bin/zqk-admin.exe"); got != "/usr/bin/zqk-admin.exe" {
		t.Fatalf("unexpected adminTriggerBinaryPath for exe: %q", got)
	}
	if got := adminTriggerBinaryPath("/usr/bin/zqk"); got != "/usr/bin/zqk-admin" {
		t.Fatalf("unexpected adminTriggerBinaryPath for sibling: %q", got)
	}

	// Testing adminTriggerBinaryCandidates with workshop structure
	tempRoot := t.TempDir()
	workshopBin := filepath.Join(tempRoot, paths.ProjectDataDir, paths.WorkshopBinDir)
	if err := fileutil.MkdirAll(workshopBin, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create workshop bin dir: %v", err)
	}
	workshopExe := filepath.Join(workshopBin, "zqk-stable")
	candidates := adminTriggerBinaryCandidates(workshopExe)
	expectedFallback := filepath.Join(tempRoot, paths.RepoBinDir, adminBinaryName)

	found := false
	for _, c := range candidates {
		if c == expectedFallback {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected candidate list to include repository fallback %q, got: %v", expectedFallback, candidates)
	}
}

func TestCycle212_ResolveAdminTriggerBinary_CandidateFiltering(t *testing.T) {
	exe, err := fileutil.Executable()
	if err != nil {
		t.Fatalf("fileutil.Executable error: %v", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatalf("EvalSymlinks error: %v", err)
	}

	// 1. Candidate is a directory -> skipped
	dirCandidate := filepath.Join(filepath.Dir(exe), adminBinaryName)
	if err := fileutil.MkdirAll(dirCandidate, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create dir candidate: %v", err)
	}
	_, resolveErr := resolveAdminTriggerBinary()
	if resolveErr == nil {
		t.Fatal("expected error when admin binary candidate is a directory")
	}
	if err := fileutil.RemoveAll(dirCandidate); err != nil {
		t.Fatalf("failed to remove dir candidate: %v", err)
	}

	// 2. Candidate is valid executable file -> resolved successfully
	_, cleanup := installMockAdminScript(t, "#!/bin/sh\nexit 0\n")
	defer cleanup()

	admin, err := resolveAdminTriggerBinary()
	if err != nil {
		t.Fatalf("expected successful resolution of mock admin, got: %v", err)
	}
	if !strings.HasSuffix(admin, adminBinaryName) {
		t.Fatalf("expected resolved binary to end with %s, got %s", adminBinaryName, admin)
	}
}

func TestCycle212_BuildSpecOriginationPipeline_LoggerHandling(t *testing.T) {
	root := setupCycle212TestRepo(t, "unit_pipe", "zqk:unit", []string{"UP-"})

	// With nil logger -> builds pipeline using default logger
	opts := Options{
		ProjectRoot: root,
		Ontology:    "unit_pipe",
		DryRun:      true,
	}
	p1, err := BuildSpecOriginationPipeline(nil, opts)
	if err != nil {
		t.Fatalf("BuildSpecOriginationPipeline with nil logger failed: %v", err)
	}
	if p1 == nil {
		t.Fatal("expected non-nil pipeline")
	}

	// With custom logger
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	p2, err := BuildSpecOriginationPipeline(logger, opts)
	if err != nil {
		t.Fatalf("BuildSpecOriginationPipeline with logger failed: %v", err)
	}
	if p2 == nil {
		t.Fatal("expected non-nil pipeline")
	}
}

func TestCycle212_StageIngest_ValidateOptionsFailure(t *testing.T) {
	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}
	opts := Options{
		ProjectRoot: "", // triggers ValidateOptions error inside stageIngest
		Ontology:    "test_ontology",
	}

	_, err := stageIngest(pctx, opts)
	if err == nil {
		t.Fatal("expected stageIngest to fail on invalid options")
	}
	if !strings.Contains(err.Error(), "ProjectRoot is required") {
		t.Errorf("expected ProjectRoot required error, got: %v", err)
	}
}

func TestCycle212_StageNormalize_SpecLoadFailure(t *testing.T) {
	root := setupCycle212TestRepo(t, "norm_fail", "zqk:norm", nil)
	// Write malformed YAML into spec file
	specPath := filepath.Join(root, paths.ProcessInternalObjectSpecsDir, "norm_fail.yaml")
	if err := fileutil.WriteFile(specPath, []byte("broken: [unclosed yaml"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write broken YAML: %v", err)
	}

	st := &State{
		Opts: Options{
			ProjectRoot: root,
			Ontology:    "norm_fail",
		},
		Loader:   objects.NewSpecLoader(root),
		SpecPath: specPath,
	}

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	_, err := stageNormalize(pctx, st)
	if err == nil {
		t.Fatal("expected stageNormalize error on malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "load spec") {
		t.Errorf("expected error containing 'load spec', got: %v", err)
	}
}

func TestCycle212_StageCommit_DirectoryValidation(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create specs dir: %v", err)
	}

	// Make specPath a directory
	dirSpecPath := filepath.Join(specsDir, "dir_spec.yaml")
	if err := fileutil.MkdirAll(dirSpecPath, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory spec: %v", err)
	}

	st := &State{
		SpecPath: dirSpecPath,
	}

	_, err := stageCommit(nil, st)
	if err == nil {
		t.Fatal("expected error when spec path is a directory")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("expected error mentioning directory, got %v", err)
	}
}

func TestCycle212_StageCrossMembrane_NilAndEmptySpecs(t *testing.T) {
	t.Run("nil spec error", func(t *testing.T) {
		st := &State{
			Opts: Options{DryRun: false, ProjectRoot: "/tmp"},
			Spec: nil,
		}
		_, err := stageCrossMembrane(nil, st)
		if err == nil || !strings.Contains(err.Error(), "spec is nil") {
			t.Fatalf("expected 'spec is nil' error, got: %v", err)
		}
	})

	t.Run("empty ontology skipped", func(t *testing.T) {
		st := &State{
			Opts: Options{DryRun: false, ProjectRoot: "/tmp"},
			Spec: &objects.Spec{Ontology: ""},
		}
		res, err := stageCrossMembrane(nil, st)
		if err != nil {
			t.Fatalf("expected nil error for empty ontology, got: %v", err)
		}
		if res != st {
			t.Fatal("expected state returned unchanged")
		}
	})
}

func TestCycle212_StageCrossMembrane_FailClosedConfigs(t *testing.T) {
	t.Run("missing kind mappings file causes fail-closed", func(t *testing.T) {
		root := t.TempDir()
		st := &State{
			Opts: Options{DryRun: false, ProjectRoot: root},
			Spec: &objects.Spec{Ontology: "test_kind"},
		}
		_, err := stageCrossMembrane(nil, st)
		if err == nil || !strings.Contains(err.Error(), "fail-closed: load kind_mappings") {
			t.Fatalf("expected fail-closed load kind_mappings error, got: %v", err)
		}
	})

	t.Run("missing id prefixes file causes fail-closed when prefixes present", func(t *testing.T) {
		root := setupCycle212TestRepo(t, "idp_test", "zqk:idp", nil)
		idpPath := filepath.Join(root, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
		if err := fileutil.Remove(idpPath); err != nil {
			t.Fatalf("failed to remove id_prefixes file: %v", err)
		}

		st := &State{
			Opts: Options{DryRun: false, ProjectRoot: root},
			Spec: &objects.Spec{
				Ontology:   "idp_test",
				IDPrefixes: []string{"IDP-"},
			},
		}
		_, err := stageCrossMembrane(nil, st)
		if err == nil || !strings.Contains(err.Error(), "fail-closed: load id_prefixes") {
			t.Fatalf("expected fail-closed load id_prefixes error, got: %v", err)
		}
	})

	t.Run("missing namespaces file causes fail-closed when namespace present", func(t *testing.T) {
		root := setupCycle212TestRepo(t, "ns_test", "zqk:ns", nil)
		nsPath := filepath.Join(root, paths.ProcessInternalConfigsDir, paths.NamespacesConfigFile)
		if err := fileutil.Remove(nsPath); err != nil {
			t.Fatalf("failed to remove namespaces file: %v", err)
		}

		st := &State{
			Opts: Options{DryRun: false, ProjectRoot: root},
			Spec: &objects.Spec{
				Ontology:  "ns_test",
				Namespace: "zqk:ns",
			},
		}
		_, err := stageCrossMembrane(nil, st)
		if err == nil || !strings.Contains(err.Error(), "fail-closed: load namespaces") {
			t.Fatalf("expected fail-closed load namespaces error, got: %v", err)
		}
	})

	t.Run("namespace not found in config causes error", func(t *testing.T) {
		root := setupCycle212TestRepo(t, "ns_mismatch", "zqk:known", nil)
		st := &State{
			Opts: Options{DryRun: false, ProjectRoot: root},
			Spec: &objects.Spec{
				Ontology:  "ns_mismatch",
				Namespace: "zqk:unknown_namespace",
			},
		}
		_, err := stageCrossMembrane(nil, st)
		if err == nil || !strings.Contains(err.Error(), "not found in config") {
			t.Fatalf("expected namespace not found error, got: %v", err)
		}
	})
}

func TestCycle212_StageCrossMembrane_AtomicCommitFailure(t *testing.T) {
	root := setupCycle212TestRepo(t, "commit_fail", "zqk:cf", nil)
	kmPath := filepath.Join(root, paths.ProcessInternalConfigsDir, paths.KindMappingsConfigFile)

	st := &State{
		Opts: Options{
			ProjectRoot: root,
			Ontology:    "commit_fail",
			DryRun:      false,
		},
		Spec: &objects.Spec{
			Ontology: "commit_fail",
		},
	}

	// First verify that normal dry run is fine
	stDry := &State{
		Opts: Options{ProjectRoot: root, Ontology: "commit_fail", DryRun: true},
		Spec: &objects.Spec{Ontology: "commit_fail"},
	}
	if _, err := stageCrossMembrane(nil, stDry); err != nil {
		t.Fatalf("dry run cross membrane failed: %v", err)
	}

	// Overwrite kind_mappings with a read-only directory right after load
	// By testing that atomic commit catches write failures:
	origContent, err := fileutil.ReadFile(kmPath)
	if err != nil {
		t.Fatalf("failed reading kind mappings: %v", err)
	}
	defer func() {
		if writeErr := fileutil.WriteFile(kmPath, origContent, paths.FilePerm644); writeErr != nil {
			t.Errorf("cleanup write failed: %v", writeErr)
		}
	}()

	// If we set a broken namespace doc that fails mutation:
	nsPath := filepath.Join(root, paths.ProcessInternalConfigsDir, paths.NamespacesConfigFile)
	brokenDoc := "namespaces: not_a_map\n"
	if err := fileutil.WriteFile(nsPath, []byte(brokenDoc), paths.FilePerm644); err != nil {
		t.Fatalf("failed writing broken namespace: %v", err)
	}
	st.Spec.Namespace = "zqk:cf"
	_, err = stageCrossMembrane(nil, st)
	if err == nil {
		t.Fatal("expected error when namespace mutation fails")
	}
	if !strings.Contains(err.Error(), "fail-closed: mutate namespaces") {
		t.Errorf("expected fail-closed mutate namespaces error, got: %v", err)
	}
}

func TestCycle212_YAMLConfigDoc_LoadWrite(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("load nonexistent file", func(t *testing.T) {
		_, err := loadYAMLConfigDoc(filepath.Join(tempDir, "missing.yaml"))
		if err == nil {
			t.Fatal("expected error loading missing file")
		}
	})

	t.Run("load invalid yaml", func(t *testing.T) {
		badPath := filepath.Join(tempDir, "bad.yaml")
		if err := fileutil.WriteFile(badPath, []byte("bad: [unclosed"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write bad file: %v", err)
		}
		_, err := loadYAMLConfigDoc(badPath)
		if err == nil {
			t.Fatal("expected error parsing invalid YAML")
		}
	})

	t.Run("load without schema header", func(t *testing.T) {
		noSchemaPath := filepath.Join(tempDir, "no_schema.yaml")
		content := "key: value\ncount: 42\n"
		if err := fileutil.WriteFile(noSchemaPath, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write no_schema file: %v", err)
		}
		doc, err := loadYAMLConfigDoc(noSchemaPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.schema != "" {
			t.Fatalf("expected empty schema, got %q", doc.schema)
		}
		if doc.doc["key"] != "value" {
			t.Fatalf("expected key=value, got %v", doc.doc["key"])
		}
	})

	t.Run("write to directory path fails", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "is_a_dir")
		if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		doc := &yamlDoc{doc: map[string]any{"k": "v"}}
		err := writeYAMLConfigDoc(dirPath, doc)
		if err == nil {
			t.Fatal("expected error writing to directory path")
		}
	})
}

func TestCycle212_StageMaterializeIndexes_RefreshAndErrors(t *testing.T) {
	root := setupCycle212TestRepo(t, "mat_kind", "zqk:mat", []string{"MK-"})

	t.Run("materialize spec index and field keys successfully", func(t *testing.T) {
		if err := fileutil.MkdirAll(filepath.Join(root, "pkg", "objects"), paths.DirPerm755); err != nil {
			t.Fatalf("failed to create pkg/objects: %v", err)
		}
		pctx := &pipeline.Context{
			Ctx:     pkgctx.NewSystemContext(),
			Outcome: make(map[string]any),
		}
		st := &State{
			Opts: Options{
				ProjectRoot:              root,
				Ontology:                 "mat_kind",
				DryRun:                   false,
				SkipMaterializeSpecIndex: false,
				MaterializeFieldKeys:     true,
			},
		}

		_, err := stageMaterializeIndexes(pctx, st)
		if err != nil {
			t.Fatalf("stageMaterializeIndexes failed: %v", err)
		}
		if pctx.Outcome[pipeline.OutcomeKeySpecIndexRefreshed] != true {
			t.Errorf("expected spec index refreshed true")
		}
		if pctx.Outcome[pipeline.OutcomeKeyFieldKeysWritten] != true {
			t.Errorf("expected field keys written true")
		}
	})

	t.Run("refresh spec index error on invalid project root", func(t *testing.T) {
		st := &State{
			Opts: Options{
				ProjectRoot:              "",
				Ontology:                 "mat_kind",
				DryRun:                   false,
				SkipMaterializeSpecIndex: false,
			},
		}
		_, err := stageMaterializeIndexes(nil, st)
		if err == nil || !strings.Contains(err.Error(), "refresh spec index") {
			t.Fatalf("expected refresh spec index error, got: %v", err)
		}
	})

	t.Run("materialize field keys error when write path is blocked", func(t *testing.T) {
		subRoot := setupCycle212TestRepo(t, "mat_block", "zqk:mat", nil)
		blockedPath := filepath.Join(subRoot, "pkg", "objects", "field_keys.go")
		if err := fileutil.MkdirAll(blockedPath, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create directory at field_keys.go path: %v", err)
		}

		st := &State{
			Opts: Options{
				ProjectRoot:              subRoot,
				Ontology:                 "mat_block",
				DryRun:                   false,
				SkipMaterializeSpecIndex: true,
				MaterializeFieldKeys:     true,
			},
		}
		_, err := stageMaterializeIndexes(nil, st)
		if err == nil || !strings.Contains(err.Error(), "write field_keys.go") {
			t.Fatalf("expected write field_keys.go error, got: %v", err)
		}
	})
}

func TestCycle212_StageTrigger_Success(t *testing.T) {
	root := setupCycle212TestRepo(t, "trig_succ", "zqk:trig", []string{"TRG-"})
	bldrDir := filepath.Join(root, "pkg", "specbuilder", "bldr_v2")
	if err := fileutil.MkdirAll(bldrDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create bldr_v2 dir: %v", err)
	}

	_, cleanup := installMockAdminScript(t, "#!/bin/sh\nexit 0\n")
	defer cleanup()

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}
	st := &State{
		Opts: Options{
			ProjectRoot:  root,
			Ontology:     "trig_succ",
			DryRun:       false,
			ApplyTrigger: true,
		},
	}

	res, err := stageTrigger(pctx, st)
	if err != nil {
		t.Fatalf("stageTrigger failed with mock admin: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil state")
	}
	if pctx.Outcome[pipeline.OutcomeKeyTriggerExecuted] != true {
		t.Errorf("expected trigger executed to be true")
	}
}

func TestCycle212_StageTrigger_Failures(t *testing.T) {
	root := setupCycle212TestRepo(t, "trig_fail", "zqk:trig", []string{"TRG-"})
	bldrDir := filepath.Join(root, "pkg", "specbuilder", "bldr_v2")
	if err := fileutil.MkdirAll(bldrDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create bldr_v2 dir: %v", err)
	}

	t.Run("mock admin exits with error output", func(t *testing.T) {
		_, cleanup := installMockAdminScript(t, "#!/bin/sh\necho 'simulated admin error'\nexit 1\n")
		defer cleanup()

		st := &State{
			Opts: Options{
				ProjectRoot:  root,
				Ontology:     "trig_fail",
				DryRun:       false,
				ApplyTrigger: true,
			},
		}

		_, err := stageTrigger(nil, st)
		if err == nil {
			t.Fatal("expected stageTrigger error when mock admin fails")
		}
		if !strings.Contains(err.Error(), "simulated admin error") {
			t.Errorf("expected error message to contain simulated admin error, got: %v", err)
		}
	})

	t.Run("mock admin exits silently with error", func(t *testing.T) {
		_, cleanup := installMockAdminScript(t, "#!/bin/sh\nexit 2\n")
		defer cleanup()

		st := &State{
			Opts: Options{
				ProjectRoot:  root,
				Ontology:     "trig_fail",
				DryRun:       false,
				ApplyTrigger: true,
			},
		}

		_, err := stageTrigger(nil, st)
		if err == nil {
			t.Fatal("expected stageTrigger error when mock admin fails")
		}
		if !strings.Contains(err.Error(), "spec origination trigger step") {
			t.Errorf("expected step failure error, got: %v", err)
		}
	})

	t.Run("generateSpecBuilder fails on missing ontology", func(t *testing.T) {
		st := &State{
			Opts: Options{
				ProjectRoot:  root,
				Ontology:     "missing_kind_ontology",
				DryRun:       false,
				ApplyTrigger: true,
			},
		}

		_, err := stageTrigger(nil, st)
		if err == nil {
			t.Fatal("expected error when spec does not exist for generateSpecBuilder")
		}
	})
}

func TestCycle212_GenerateSpecBuilder_ReserveConstantsFailure(t *testing.T) {
	root := setupCycle212TestRepo(t, "bldr_fail", "zqk:bf", nil)
	versionDir := filepath.Join(root, "pkg", "specbuilder", "bldr_v2")
	if err := fileutil.MkdirAll(versionDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create version dir: %v", err)
	}

	// Write invalid Go file ending with _constants.go so ReserveConstantsFromDir fails
	badGoFile := filepath.Join(versionDir, "sibling_constants.go")
	if err := fileutil.WriteFile(badGoFile, []byte("package bldr_v2\n\nfunc broken( {\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write broken Go file: %v", err)
	}

	err := generateSpecBuilder(root, "bldr_fail")
	if err == nil {
		t.Fatal("expected generateSpecBuilder to fail on invalid go file in versionDir")
	}
	if !strings.Contains(err.Error(), "reserve_spec_constants failed") {
		t.Errorf("expected reserve_spec_constants failure, got: %v", err)
	}
}

func TestCycle212_StageFinalize_ValidationFormatting(t *testing.T) {
	root := setupCycle212TestRepo(t, "val_format", "zqk:val", nil)
	loader := objects.NewSpecLoader(root)

	multiErrSpec := &objects.Spec{
		Ontology: "val_format",
		Fields: map[string]any{
			"f1": map[string]any{"type": ""},
			"f2": map[string]any{"type": 12345},
		},
	}

	st := &State{
		Opts: Options{
			ProjectRoot:            root,
			Ontology:               "val_format",
			SkipFinalizeValidation: false,
		},
		Loader: loader,
		Spec:   multiErrSpec,
	}

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	_, err := stageFinalize(pctx, st)
	if err == nil {
		t.Fatal("expected validation failure")
	}
	if !strings.Contains(err.Error(), "spec validation failed") {
		t.Errorf("expected 'spec validation failed' prefix, got: %v", err)
	}
	if pctx.Outcome[pipeline.OutcomeKeyValidationErrorCount].(int) < 1 {
		t.Errorf("expected validation error count > 0")
	}
}

func TestCycle212_StageFinalize_ValidSpecSuccess(t *testing.T) {
	root := setupCycle212TestRepo(t, "val_clean", "zqk:val", nil)
	loader := objects.NewSpecLoader(root)

	cleanSpec := &objects.Spec{
		Ontology: "val_clean",
		Fields:   map[string]any{},
		Traits:   []string{},
	}

	st := &State{
		Opts: Options{
			ProjectRoot:            root,
			Ontology:               "val_clean",
			SkipFinalizeValidation: false,
		},
		Loader: loader,
		Spec:   cleanSpec,
	}

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	res, err := stageFinalize(pctx, st)
	if err != nil {
		t.Fatalf("expected stageFinalize to succeed on clean spec, got: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil state returned from clean stageFinalize")
	}
}

func TestCycle212_ConcurrentPipelineExecution(t *testing.T) {
	const workerCount = 5
	var wg sync.WaitGroup
	errorsCh := make(chan error, workerCount)

	for i := 0; i < workerCount; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(
			fmt.Sprintf("worker-%d", workerID),
			"concurrent spec origination pipeline execution",
		).WithWaitGroup(&wg).StartSimple(func() {
			root := setupCycle212TestRepo(t, fmt.Sprintf("conc_item_%d", workerID), "zqk:conc", nil)
			opts := Options{
				ProjectRoot:              root,
				Ontology:                 fmt.Sprintf("conc_item_%d", workerID),
				DryRun:                   true,
				SkipFinalizeValidation:   true,
				SkipMaterializeSpecIndex: true,
			}
			pctx := &pipeline.Context{
				Ctx:     pkgctx.NewSystemContext(),
				Outcome: make(map[string]any),
			}
			st, err := Run(pctx, nil, opts)
			if err != nil {
				errorsCh <- fmt.Errorf("worker %d Run failed: %w", workerID, err)
				return
			}
			if st == nil {
				errorsCh <- fmt.Errorf("worker %d returned nil state", workerID)
				return
			}
		})
	}

	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		t.Errorf("concurrent pipeline error: %v", err)
	}
}

func TestCycle212_ContextCancellation(t *testing.T) {
	root := setupCycle212TestRepo(t, "cancel_test", "zqk:cancel", nil)
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	cancel()

	pctx := &pipeline.Context{
		Ctx:     ctx,
		Outcome: make(map[string]any),
	}

	opts := Options{
		ProjectRoot:              root,
		Ontology:                 "cancel_test",
		DryRun:                   true,
		SkipFinalizeValidation:   true,
		SkipMaterializeSpecIndex: true,
	}

	_, err := Run(pctx, nil, opts)
	if err == nil {
		t.Fatal("expected error running pipeline with cancelled context")
	}
}
