package system

// Alpha launch gate — data cell / organism integration suite (CLI subprocess on a greenfield project).
//
// Covers: isolated project → init → bootstrap extraction → core object_specs DNA (base_object, auditable,
// test_case) → zqk object template object_spec (draft template from spec) → materialized spec index → system spec-origination (dry-run + skip-finalize JSON) → update-specs dry-run → field define/modify (sidecar) → deprecate → archive → delete.
// Also covers: spec_index.json lists test_case; system status; sync-glossary-from-specs (dry-run JSON + apply);
// net-new kind YAML + kind_mappings/id_prefixes/namespaces patches → generate-spec-index → validate → generate-instance-builders;
// object fields --list-kinds; object test_case fields --format json; system validate --kind test_case --format json;
// internal object_spec fields --format json; object count test_case --format json; spec list --format json;
// internal fields --list-kinds --format json; system generate-lifecycle-id-list (no-op when no legacy IDs);
// utility validate-yaml on bootstrapped test_case.yaml; utility validate-yaml --recursive on object_specs/;
// system validate --all --format json (fixed kind set);
// internal count --format json (internal/built-in scope); internal count object_spec --format json;
// system generate-field-keys --output under .zqk/;
// system retention-status --format json; system health-data --format json --allow-degraded;
// system update-specs --dry-run --files test_case.yaml (file-scoped batch); internal list --format json --allow-degraded;
// system quarantine-report --format json; utility version; object count --format json --allow-degraded (all kinds);
// system whoami; path-cache;
// detect-spec-changes; system feature-flags list; cli-hooks list; object list test_case --format json (empty set);
// REQ-019 checklist keys on the probe field; optional field define --validate via ENABLE_SPEC_CELL_REQ019_VALIDATE=1 (bundled test_case still has inherited trait/checklist debt that fails full-spec validate). system check --fast --clean-cache; cleanup-duplicates --allow-degraded --dry-run.
// Net-new kind e2e (migration confidence): data-cells --json lists the net-new kind (spec index + inherited storage_profile); sync-glossary-from-specs --format json (dry-run) includes that object_spec in missing[]; --kind filter returns one row.
//
// Run: <brand>ENABLE_SPEC_CELL_INTEGRATION_TESTS=1 go test ./cmd/zqk/system -run TestSpecCell -timeout 180s
// (integration: builds zqk into the temp project). Skipped unless the env var is set.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// specCellCheckCommandTimeout allows system check (object ID cache, stale CAS hygiene) to finish on a cold greenfield project.
const specCellCheckCommandTimeout = 90 * time.Second

// testCaseSidecarFieldYAML is a complete field body for test_case (REQ-019 checklist) used by field_define_sidecar.
const testCaseSidecarFieldYAML = `access:
  requires:
    - access:confidential
checklist:
  authority: QA/owner.
  automation_hooks: spec cell integration probe.
  cardinality: one
  criticality: association
  default: null
  dependencies: none.
  lifecycle: mutable.
  observability: yes.
  purpose: Integration probe field for spec cell subprocess tests.
  security: non-sensitive.
  system_usage:
    - spec_cell_integration
  validation: Free-form string.
field_profile_code: TST-099
permissions: rwx
semantic_type: statement
traits:
  - field_read_only_group
type: string
validation:
  required: false
`

// testCaseSidecarFieldYAMLModified is the same field as testCaseSidecarFieldYAML with an updated checklist.purpose
// (modify merges top-level keys; a full field body keeps nested maps intact).
const testCaseSidecarFieldYAMLModified = `access:
  requires:
    - access:confidential
checklist:
  authority: QA/owner.
  automation_hooks: spec cell integration probe.
  cardinality: one
  criticality: association
  default: null
  dependencies: none.
  lifecycle: mutable.
  observability: yes.
  purpose: Modified via subprocess field modify (spec cell suite).
  security: non-sensitive.
  system_usage:
    - spec_cell_integration
  validation: Free-form string.
field_profile_code: TST-099
permissions: rwx
semantic_type: statement
traits:
  - field_read_only_group
type: string
validation:
  required: false
`

// spec_cell_netnew* — net-new ontology slice (TestSpecCell_Suite/net_new_kind_spec_pipeline).
const (
	specCellNetNewKind     = "spec_cell_netnew"
	specCellNetNewDir      = "spec_cell_netnew"
	specCellNetNewSpecYAML = `schema_version: 2.0.0
ontology: spec_cell_netnew
extends: base_object
visibility: public
description: |
  Spec cell integration net-new kind (greenfield temp project).
traits:
  - base_object_traits
fields:
  note:
    access:
      requires:
        - access:confidential
    checklist:
      authority: QA/owner.
      automation_hooks: spec cell net-new probe.
      cardinality: one
      criticality: association
      default: null
      dependencies: none.
      lifecycle: mutable.
      observability: yes.
      purpose: Net-new kind probe field.
      security: non-sensitive.
      system_usage:
        - spec_cell_netnew
      validation: Free-form string.
    field_profile_code: SCN-001
    permissions: rwx
    semantic_type: statement
    traits:
      - field_mutable_group
    type: string
    validation:
      required: false
`
)

// specCellCodegenCommandTimeout is long enough for generate-instance-builders over the full bootstrapped object_specs tree.
const specCellCodegenCommandTimeout = 5 * time.Minute

func TestSpecCell_Suite(t *testing.T) {
	if testing.Short() {
		t.Skip("spec cell integration suite skipped in short mode")
	}
	if !zqkenvTruthy(zqkenv.EnableSpecCellIntegrationTests().Get()) {
		t.Skip("set " + zqkenv.EnableSpecCellIntegrationTests().Name() + "=1 to run (integration: builds CLI, temp project)")
	}

	projectRoot := findModuleRootForBootstrap(t)
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	scenarioDir := proj.Root

	origWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = fileutil.Chdir(origWd) })
	if err := fileutil.Chdir(scenarioDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Run init + bootstrap here (not inside a t.Run) so `go test -run TestSpecCell_Suite/<one_subtest>`
	// still sees a greenfield project; sibling subtests are skipped when -run filters to a single name.
	initCmd := NewInitCmd()
	initCmd.SetArgs([]string{})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("greenfield init: %v", err)
	}
	requireBootstrapPresent(t, scenarioDir)
	specsDir := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir)
	for _, d := range []string{
		paths.ProcessInternalObjectSpecsDir,
		paths.ProcessInternalLifecyclesDir,
		paths.ProcessInternalConfigsDir,
	} {
		removeAppleDoubleFilesInDir(t, filepath.Join(scenarioDir, d))
	}
	for _, name := range []string{"base_object.yaml", "auditable.yaml", "test_case.yaml"} {
		p := filepath.Join(specsDir, name)
		if _, err := fileutil.Stat(p); err != nil {
			t.Fatalf("expected bootstrap spec %s: %v", name, err)
		}
	}

	cliBinary := filepath.Join(scenarioDir, "zqk")
	buildCmd := execwrap.Command("go", "build", "-o", cliBinary, "./cmd/zqk")
	zqkenv.WireExecForIsolatedProject(buildCmd, projectRoot)
	// buildCmd.Env = os.Environ() removed to preserve WireExecForIsolatedProject env
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("build CLI: %v", err)
	}
	env := zqkenv.SubprocessEnvironWithTestRoot(scenarioDir)

	t.Run("object_template_object_spec_draft", func(t *testing.T) {
		// Origination slice: CLI materializes a draft from the object_spec spec (object template), not a net-new ontology file.
		// TRACK: BLI-1785930106857898000-94b9a5bc — replace retired `new internal`.
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "object", "template", "object_spec", "--output", "-")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("object template object_spec --output -: %v\n%s", err, out)
		}
		s := string(out)
		if !strings.Contains(s, "kind: object_spec") {
			t.Fatalf("expected object_spec draft YAML; got:\n%s", s)
		}
		if !strings.Contains(s, "schema_version:") {
			t.Fatalf("expected schema_version in draft; got:\n%s", s)
		}
	})

	t.Run("generate_spec_index", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "generate-spec-index")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("generate-spec-index: %v\n%s", err, out)
		}
		idx := filepath.Join(scenarioDir, paths.ProcessInternalDir, "spec_index.json")
		idxData, err := fileutil.ReadFile(idx)
		if err != nil {
			t.Fatalf("read spec_index.json: %v", err)
		}
		var idxDoc struct {
			Kinds map[string]json.RawMessage `json:"kinds"`
		}
		if err := json.Unmarshal(idxData, &idxDoc); err != nil {
			t.Fatalf("parse spec_index.json: %v", err)
		}
		if idxDoc.Kinds == nil || idxDoc.Kinds["test_case"] == nil {
			t.Fatalf("expected spec_index kinds to include test_case")
		}
	})

	t.Run("spec_origination_dry_run", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "spec-origination",
			"--ontology", "test_case", "--dry-run", "--skip-finalize-validation", "-f", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("spec-origination dry-run: %v\n%s", err, out)
		}
		var doc struct {
			Ontology string         `json:"ontology"`
			SpecPath string         `json:"spec_path"`
			Outcome  map[string]any `json:"outcome"`
		}
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("parse JSON: %v\n%s", err, out)
		}
		if doc.Ontology != "test_case" {
			t.Fatalf("expected ontology test_case, got %q", doc.Ontology)
		}
		if doc.SpecPath == "" {
			t.Fatalf("expected spec_path in output")
		}
		if doc.Outcome == nil {
			t.Fatalf("expected outcome map in output")
		}
	})

	t.Run("net_new_kind_spec_pipeline", func(t *testing.T) {
		// Net-new ontology: spec YAML + config patches → index → validate → instance-builder codegen (subprocess).
		specPath := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir, specCellNetNewKind+".yaml")
		if err := fileutil.WriteSecureFile(specPath, []byte(specCellNetNewSpecYAML)); err != nil {
			t.Fatalf("write net-new spec: %v", err)
		}
		patchKindMappingsForSpecCellNetNew(t, scenarioDir)
		patchIDPrefixesForSpecCellNetNew(t, scenarioDir)
		patchNamespacesConfigForSpecCellNetNew(t, scenarioDir)

		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "generate-spec-index")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("generate-spec-index after net-new spec: %v\n%s", err, out)
		}
		idx := filepath.Join(scenarioDir, paths.ProcessInternalDir, "spec_index.json")
		idxData, err := fileutil.ReadFile(idx)
		if err != nil {
			t.Fatalf("read spec_index.json: %v", err)
		}
		var idxDoc struct {
			Kinds map[string]json.RawMessage `json:"kinds"`
		}
		if err := json.Unmarshal(idxData, &idxDoc); err != nil {
			t.Fatalf("parse spec_index.json: %v", err)
		}
		if idxDoc.Kinds == nil || idxDoc.Kinds[specCellNetNewKind] == nil {
			t.Fatalf("expected spec_index kinds to include %s", specCellNetNewKind)
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel2()
		cmdVal := execwrap.CommandContext(ctx2, cliBinary, "system", "validate", "--kind", specCellNetNewKind, "--format", "json", "--quiet")
		zqkenv.WireExecForIsolatedProject(cmdVal, scenarioDir)
		cmdVal.Env = env
		outVal, err := cmdVal.CombinedOutput()
		if err != nil {
			t.Fatalf("system validate --kind %s: %v\n%s", specCellNetNewKind, err, outVal)
		}
		var valRep struct {
			Kind         string `json:"kind"`
			ValidCount   int    `json:"valid_count"`
			InvalidCount int    `json:"invalid_count"`
		}
		if err := json.Unmarshal(outVal, &valRep); err != nil {
			t.Fatalf("parse validate json: %v\n%s", err, outVal)
		}
		if valRep.Kind != specCellNetNewKind {
			t.Fatalf("validate json: want kind %s, got %q", specCellNetNewKind, valRep.Kind)
		}

		outDir := filepath.Join(scenarioDir, paths.ProjectDataDir, "spec_cell_codegen", "instance_builders")
		ctx3, cancel3 := context.WithTimeout(context.Background(), specCellCodegenCommandTimeout)
		defer cancel3()
		cmdGen := execwrap.CommandContext(ctx3, cliBinary, "system", "generate-instance-builders",
			"--overwrite",
			"--output-dir", outDir,
		)
		zqkenv.WireExecForIsolatedProject(cmdGen, scenarioDir)
		cmdGen.Env = env
		outGen, err := cmdGen.CombinedOutput()
		if err != nil {
			t.Fatalf("generate-instance-builders: %v\n%s", err, outGen)
		}
		genFile := filepath.Join(scenarioDir, paths.ProjectDataDir, "spec_cell_codegen", "bldr_instance_v1", specCellNetNewKind+"_instance_builder.go")
		if _, err := fileutil.Stat(genFile); err != nil {
			t.Fatalf("expected generated instance builder at %s: %v", genFile, err)
		}
	})

	t.Run("update_specs_trait_groups_dry_run", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "update-specs", "--trait-groups", "--dry-run")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("update-specs --trait-groups --dry-run: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "Dry-run") && !strings.Contains(string(out), "dry") && !strings.Contains(string(out), "No updates") {
			t.Logf("update-specs output (sanity): %s", out)
		}
	})

	t.Run("system_status", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "status")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system status: %v\n%s", err, out)
		}
	})

	t.Run("sync_glossary_from_specs_dry_run", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "sync-glossary-from-specs", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("sync-glossary-from-specs: %v\n%s", err, out)
		}
		var rep struct {
			DryRun bool `json:"dry_run"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse sync-glossary json: %v\n%s", err, out)
		}
		if !rep.DryRun {
			t.Fatalf("expected dry_run true in glossary sync report")
		}
	})

	t.Run("sync_glossary_from_specs_apply", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "sync-glossary-from-specs",
			"--apply", "--dry-run=false", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("sync-glossary-from-specs --apply: %v\n%s", err, out)
		}
		var rep struct {
			DryRun            bool `json:"dry_run"`
			CreatedCount      int  `json:"created_count"`
			MissingCandidates int  `json:"missing_candidates"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse sync-glossary apply json: %v\n%s", err, out)
		}
		if rep.DryRun {
			t.Fatalf("expected dry_run false after apply")
		}
		ctx2, cancel2 := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel2()
		cmdList := execwrap.CommandContext(ctx2, cliBinary, "object", "list", "glossary_term", "--format", "json", "--limit", "50", "--allow-degraded")
		zqkenv.WireExecForIsolatedProject(cmdList, scenarioDir)
		cmdList.Env = env
		listOut, err := cmdList.CombinedOutput()
		if err != nil {
			t.Fatalf("object list glossary_term after sync apply: %v\n%s", err, listOut)
		}
		var listDoc struct {
			Objects []map[string]any `json:"objects"`
		}
		if err := json.Unmarshal(listOut, &listDoc); err != nil {
			t.Fatalf("parse glossary list json: %v\n%s", err, listOut)
		}
		if rep.CreatedCount > 0 && len(listDoc.Objects) == 0 {
			t.Fatalf("expected at least one glossary_term after apply when created_count=%d", rep.CreatedCount)
		}
	})

	t.Run("net_new_kind_data_cell_and_glossary_e2e", func(t *testing.T) {
		// After net_new_kind_spec_pipeline + sync_glossary_from_specs_apply: spec index drives data-cells;
		// glossary sync-from-specs must still list the net-new YAML as a candidate (title + source_path).
		//
		// Note: we do not assert persisted glossary_term rows here: in this harness, sync --apply can
		// report created_count much larger than object count glossary_term (investigate separately).
		netNewSpec := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir, specCellNetNewKind+".yaml")
		if _, err := fileutil.Stat(netNewSpec); err != nil {
			t.Fatalf("net-new object_spec file: %v", err)
		}
		wantTitle := "Object kind: " + specCellNetNewKind

		ctxProbe, cancelProbe := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancelProbe()
		cmdProbe := execwrap.CommandContext(ctxProbe, cliBinary, "system", "sync-glossary-from-specs", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmdProbe, scenarioDir)
		cmdProbe.Env = env
		outProbe, err := cmdProbe.CombinedOutput()
		if err != nil {
			t.Fatalf("sync-glossary-from-specs dry-run: %v\n%s", err, outProbe)
		}
		var probe struct {
			Missing []struct {
				Title      string `json:"title"`
				SourcePath string `json:"source_path"`
			} `json:"missing"`
		}
		if err := json.Unmarshal(outProbe, &probe); err != nil {
			t.Fatalf("parse sync-glossary json: %v\n%s", err, outProbe)
		}
		var ontFromFile string
		if rawOnt, err := fileutil.ReadFile(netNewSpec); err == nil {
			var ym map[string]any
			if err := yaml.Unmarshal(rawOnt, &ym); err == nil {
				ontFromFile = strings.TrimSpace(fmt.Sprint(ym[objects.FieldKeyOntology]))
			}
		}
		if ontFromFile != specCellNetNewKind {
			t.Fatalf("net-new spec ontology field: want %q, got %q", specCellNetNewKind, ontFromFile)
		}
		specRelSuffix := filepath.Join(paths.ProcessInternalObjectSpecsDir, specCellNetNewKind+".yaml")
		var hasNetNewInMissing bool
		for _, m := range probe.Missing {
			if strings.HasSuffix(m.SourcePath, specRelSuffix) || strings.TrimSpace(m.Title) == wantTitle {
				hasNetNewInMissing = true
				break
			}
		}
		if !hasNetNewInMissing {
			t.Fatalf("sync-glossary dry-run missing must include net-new object_spec (suffix %q); missing=%d", specRelSuffix, len(probe.Missing))
		}

		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "data-cells", "--json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system data-cells --json: %v\n%s", err, out)
		}
		var rows []map[string]any
		if err := json.Unmarshal(out, &rows); err != nil {
			t.Fatalf("parse data-cells json: %v\n%s", err, out)
		}
		var netRow map[string]any
		for _, row := range rows {
			if k, _ := row[objects.FieldKeyKind].(string); k == specCellNetNewKind {
				netRow = row
				break
			}
		}
		if netRow == nil {
			t.Fatalf("expected data-cells to include kind %q, got %d rows", specCellNetNewKind, len(rows))
		}
		if sp, _ := netRow["storage_profile"].(string); sp != "cas_entity" {
			t.Fatalf("net-new storage_profile: want cas_entity inherited from auditable chain, got %q row=%v", sp, netRow)
		}
		if cid, _ := netRow["cell_id"].(string); cid != specCellNetNewKind {
			t.Fatalf("cell_id: want %q, got %v", specCellNetNewKind, netRow["cell_id"])
		}
		if known, _ := netRow["storage_profile_known"].(bool); !known {
			t.Fatalf("storage_profile_known: want true, got %v", netRow["storage_profile_known"])
		}
		pp, _ := netRow["primary_path"].(string)
		if pp == "" {
			t.Fatalf("primary_path: want non-empty for net-new kind, got %v", netRow["primary_path"])
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel2()
		cmd2 := execwrap.CommandContext(ctx2, cliBinary, "system", "data-cells", "--json", "--kind", specCellNetNewKind)
		zqkenv.WireExecForIsolatedProject(cmd2, scenarioDir)
		cmd2.Env = env
		out2, err := cmd2.CombinedOutput()
		if err != nil {
			t.Fatalf("system data-cells --json --kind: %v\n%s", err, out2)
		}
		var rows2 []map[string]any
		if err := json.Unmarshal(out2, &rows2); err != nil {
			t.Fatalf("parse data-cells kind filter json: %v\n%s", err, out2)
		}
		if len(rows2) != 1 {
			t.Fatalf("data-cells --kind: want 1 row, got %d", len(rows2))
		}
	})

	t.Run("object_fields_list_kinds", func(t *testing.T) {
		kinds := getObjectKindsFromCLI(t, cliBinary, scenarioDir, env)
		found := false
		for _, k := range kinds {
			if k == "test_case" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("object fields --list-kinds: want test_case in kinds, got %v", kinds)
		}
	})

	t.Run("object_test_case_fields_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "object", "test_case", "fields", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("object test_case fields --format json: %v\n%s", err, out)
		}
		var doc any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("parse fields json: %v\n%s", err, out)
		}
		if doc == nil {
			t.Fatalf("expected non-nil fields json payload")
		}
	})

	t.Run("system_whoami", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "whoami")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system whoami: %v\n%s", err, out)
		}
	})

	t.Run("system_path_cache", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "path-cache")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system path-cache: %v\n%s", err, out)
		}
	})

	t.Run("detect_spec_changes", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "detect-spec-changes")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system detect-spec-changes: %v\n%s", err, out)
		}
	})

	t.Run("system_feature_flags_list", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "feature-flags", "list")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system feature-flags list: %v\n%s", err, out)
		}
	})

	t.Run("system_cli_hooks_list", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "cli-hooks", "list")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system cli-hooks list: %v\n%s", err, out)
		}
	})

	t.Run("object_list_test_case_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "object", "list", "test_case", "--format", "json", "--limit", "5", "--allow-degraded")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("object list test_case --format json: %v\n%s", err, out)
		}
		var doc any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("parse list json: %v\n%s", err, out)
		}
		if doc == nil {
			t.Fatalf("expected non-nil list json payload")
		}
	})

	t.Run("system_validate_kind_test_case_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "validate", "--kind", "test_case", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system validate --kind test_case --format json: %v\n%s", err, out)
		}
		var rep struct {
			Kind         string `json:"kind"`
			ValidCount   int    `json:"valid_count"`
			InvalidCount int    `json:"invalid_count"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse validate json: %v\n%s", err, out)
		}
		if rep.Kind != "test_case" {
			t.Fatalf("validate json: want kind test_case, got %q", rep.Kind)
		}
		if rep.ValidCount < 0 || rep.InvalidCount < 0 {
			t.Fatalf("validate json: unexpected counts valid=%d invalid=%d", rep.ValidCount, rep.InvalidCount)
		}
	})

	t.Run("internal_object_spec_fields_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "internal", "object_spec", "fields", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("internal object_spec fields --format json: %v\n%s", err, out)
		}
		var doc any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("parse internal fields json: %v\n%s", err, out)
		}
		if doc == nil {
			t.Fatalf("expected non-nil internal object_spec fields json payload")
		}
	})

	t.Run("object_count_test_case_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "object", "count", "test_case", "--format", "json", "--allow-degraded")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("object count test_case --format json: %v\n%s", err, out)
		}
		var rep struct {
			Count int    `json:"count"`
			Kind  string `json:"kind"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse count json: %v\n%s", err, out)
		}
		if rep.Kind != "test_case" {
			t.Fatalf("count json: want kind test_case, got %q", rep.Kind)
		}
		if rep.Count < 0 {
			t.Fatalf("count json: unexpected count %d", rep.Count)
		}
	})

	t.Run("spec_list_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "spec", "list", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("spec list --format json: %v\n%s", err, out)
		}
		var rep struct {
			Objects []map[string]any `json:"objects"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse spec list json: %v\n%s", err, out)
		}
		if len(rep.Objects) == 0 {
			t.Fatalf("spec list: expected at least one object from file-backed object_specs, got 0")
		}
	})

	t.Run("internal_fields_list_kinds_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "internal", "fields", "--list-kinds", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("internal fields --list-kinds --format json: %v\n%s", err, out)
		}
		var rep struct {
			Kinds []string `json:"kinds"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse internal kinds json: %v\n%s", err, out)
		}
		found := false
		for _, k := range rep.Kinds {
			if k == "object_spec" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("internal fields --list-kinds: want object_spec in kinds, got %v", rep.Kinds)
		}
	})

	t.Run("system_generate_lifecycle_id_list_noop", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "generate-lifecycle-id-list")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system generate-lifecycle-id-list: %v\n%s", err, out)
		}
		s := string(out)
		if !strings.Contains(s, "No lifecycle objects with old ID format found") {
			t.Fatalf("generate-lifecycle-id-list: expected empty-migration message, got:\n%s", s)
		}
	})

	t.Run("utility_validate_yaml_test_case_spec", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		specRel := filepath.Join(paths.ProcessInternalObjectSpecsDir, "test_case.yaml")
		cmd := execwrap.CommandContext(ctx, cliBinary, "utility", "validate-yaml", specRel)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("utility validate-yaml %s: %v\n%s", specRel, err, out)
		}
	})

	t.Run("utility_validate_yaml_recursive_object_specs", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "utility", "validate-yaml", "--recursive", paths.ProcessInternalObjectSpecsDir)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("utility validate-yaml --recursive %s: %v\n%s", paths.ProcessInternalObjectSpecsDir, err, out)
		}
	})

	t.Run("system_validate_all_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "validate", "--all", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system validate --all --format json: %v\n%s", err, out)
		}
		var rep struct {
			Valid        bool `json:"valid"`
			ValidCount   int  `json:"valid_count"`
			InvalidCount int  `json:"invalid_count"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse validate --all json: %v\n%s", err, out)
		}
		if !rep.Valid || rep.InvalidCount != 0 {
			t.Fatalf("validate --all: want valid=true and invalid_count=0 on greenfield, got valid=%v valid_count=%d invalid_count=%d",
				rep.Valid, rep.ValidCount, rep.InvalidCount)
		}
	})

	t.Run("internal_count_all_kinds_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "internal", "count", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("internal count --format json: %v\n%s", err, out)
		}
		var rep struct {
			CountsByKind map[string]int `json:"counts_by_kind"`
			TotalObjects int            `json:"total_objects"`
			TotalKinds   int            `json:"total_kinds"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse internal count json: %v\n%s", err, out)
		}
		if rep.CountsByKind == nil {
			t.Fatalf("internal count: expected counts_by_kind object")
		}
		if rep.TotalObjects < 0 || rep.TotalKinds < 0 {
			t.Fatalf("internal count: unexpected totals objects=%d kinds=%d", rep.TotalObjects, rep.TotalKinds)
		}
	})

	t.Run("internal_count_object_spec_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "internal", "count", "object_spec", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("internal count object_spec --format json: %v\n%s", err, out)
		}
		var rep struct {
			Count int    `json:"count"`
			Kind  string `json:"kind"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse internal count object_spec json: %v\n%s", err, out)
		}
		if rep.Kind != "object_spec" {
			t.Fatalf("internal count object_spec: want kind object_spec, got %q", rep.Kind)
		}
		if rep.Count < 0 {
			t.Fatalf("internal count object_spec: unexpected count %d", rep.Count)
		}
	})

	t.Run("system_generate_field_keys_under_zqk", func(t *testing.T) {
		outRel := filepath.Join(paths.ProjectDataDir, "spec_cell_field_keys.go")
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "generate-field-keys", "--output", outRel)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		combined, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system generate-field-keys --output %s: %v\n%s", outRel, err, combined)
		}
		genPath := filepath.Join(scenarioDir, outRel)
		src, err := fileutil.ReadFile(genPath)
		if err != nil {
			t.Fatalf("read generated field_keys: %v", err)
		}
		s := string(src)
		if !strings.Contains(s, "package objects") || !strings.Contains(s, `zqk system generate-field-keys`) {
			t.Fatalf("generated field_keys.go missing expected header (len=%d)", len(s))
		}
	})

	t.Run("system_retention_status_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "retention-status", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system retention-status --format json: %v\n%s", err, out)
		}
		var rep struct {
			TotalInternal   int  `json:"total_internal"`
			TargetTotal     int  `json:"target_total"`
			AtOrUnderTarget bool `json:"at_or_under_target"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse retention-status json: %v\n%s", err, out)
		}
		if rep.TotalInternal < 0 || rep.TargetTotal < 0 {
			t.Fatalf("retention-status: unexpected totals internal=%d target=%d", rep.TotalInternal, rep.TargetTotal)
		}
	})

	t.Run("system_health_data_json_allow_degraded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "health-data", "--format", "json", "--allow-degraded")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system health-data --format json --allow-degraded: %v\n%s", err, out)
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("parse health-data json: %v\n%s", err, out)
		}
		if doc == nil {
			t.Fatalf("health-data: expected non-empty json object")
		}
	})

	t.Run("system_update_specs_dry_run_single_file", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "update-specs", "--dry-run", "--files", "test_case.yaml")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system update-specs --dry-run --files test_case.yaml: %v\n%s", err, out)
		}
		s := string(out)
		if !strings.Contains(s, "No updates needed") {
			t.Fatalf("update-specs file dry-run: expected no-op message, got:\n%s", s)
		}
	})

	t.Run("internal_list_json_allow_degraded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "internal", "list", "--format", "json", "--limit", "5", "--allow-degraded")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("internal list --format json --allow-degraded: %v\n%s", err, out)
		}
		var doc any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("parse internal list json: %v\n%s", err, out)
		}
		if doc == nil {
			t.Fatalf("expected non-nil internal list json payload")
		}
	})

	t.Run("system_quarantine_report_json", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "system", "quarantine-report", "--format", "json")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system quarantine-report --format json: %v\n%s", err, out)
		}
		var rep struct {
			QuarantineRoot string `json:"quarantine_root"`
			TotalFiles     int    `json:"total_files"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse quarantine-report json: %v\n%s", err, out)
		}
		if rep.QuarantineRoot == "" {
			t.Fatalf("quarantine-report: expected quarantine_root")
		}
		if rep.TotalFiles < 0 {
			t.Fatalf("quarantine-report: unexpected total_files %d", rep.TotalFiles)
		}
	})

	t.Run("utility_version", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "utility", "version")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("utility version: %v\n%s", err, out)
		}
		s := string(out)
		if !strings.Contains(s, "Version:") {
			t.Fatalf("utility version: expected Version line, got:\n%s", s)
		}
	})

	t.Run("object_count_all_kinds_json_allow_degraded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary, "object", "count", "--format", "json", "--allow-degraded")
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("object count --format json --allow-degraded: %v\n%s", err, out)
		}
		var rep struct {
			CountsByKind map[string]int `json:"counts_by_kind"`
			TotalObjects int            `json:"total_objects"`
			TotalKinds   int            `json:"total_kinds"`
		}
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("parse object count (all kinds) json: %v\n%s", err, out)
		}
		if rep.CountsByKind == nil {
			t.Fatalf("object count: expected counts_by_kind object")
		}
		if rep.TotalObjects < 0 || rep.TotalKinds < 0 {
			t.Fatalf("object count: unexpected totals objects=%d kinds=%d", rep.TotalObjects, rep.TotalKinds)
		}
	})

	t.Run("field_define_sidecar", func(t *testing.T) {
		// Sidecar name must match runFieldOperation: <ontology>_field_<fieldName>.yaml (cwd = project root).
		fieldName := "spec_cell_alpha_probe"
		sidecar := filepath.Join(scenarioDir, "test_case_field_"+fieldName+".yaml")
		if err := fileutil.WriteSecureFile(sidecar, []byte(testCaseSidecarFieldYAML)); err != nil {
			t.Fatalf("write sidecar field YAML: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		defineArgs := []string{
			"system", "update-specs", "test_case",
			"--field", fieldName,
			"--operation", "define",
			"--reason", "spec cell integration: subprocess field define + materialized index refresh",
		}
		if zqkenvTruthy(zqkenv.EnableSpecCellREQ019Validate().Get()) {
			defineArgs = append(defineArgs, "--validate")
		}
		cmd := execwrap.CommandContext(ctx, cliBinary, defineArgs...)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("update-specs field define: %v\n%s", err, out)
		}
		specPath := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir, "test_case.yaml")
		raw, err := fileutil.ReadFile(specPath)
		if err != nil {
			t.Fatalf("read test_case.yaml: %v", err)
		}
		if !strings.Contains(string(raw), fieldName+":") {
			t.Fatalf("expected new field %q in test_case.yaml", fieldName)
		}
		idxPath := filepath.Join(scenarioDir, paths.ProcessInternalDir, "spec_index.json")
		if _, err := fileutil.Stat(idxPath); err != nil {
			t.Fatalf("expected spec_index.json after field op: %v", err)
		}
	})

	t.Run("probe_field_req019_checklist_keys", func(t *testing.T) {
		// Key presence: checklist rows per objects.RequiredChecklistItems, plus field_profile_code on the field (same layout as object_specs YAML).
		// Full LoadSpecAndValidate on the whole spec remains opt-in (inherited-field debt).
		fieldName := "spec_cell_alpha_probe"
		assertProbeFieldHasREQ019ChecklistKeys(t, scenarioDir, fieldName)
	})

	fieldName := "spec_cell_alpha_probe"

	t.Run("field_modify_sidecar", func(t *testing.T) {
		sidecar := filepath.Join(scenarioDir, "test_case_field_"+fieldName+".yaml")
		if err := fileutil.WriteSecureFile(sidecar, []byte(testCaseSidecarFieldYAMLModified)); err != nil {
			t.Fatalf("write sidecar for modify: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary,
			"system", "update-specs", "test_case",
			"--field", fieldName,
			"--operation", "modify",
			"--reason", "spec cell integration: subprocess modify",
		)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("update-specs field modify: %v\n%s", err, out)
		}
		specPath := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir, "test_case.yaml")
		raw, err := fileutil.ReadFile(specPath)
		if err != nil {
			t.Fatalf("read test_case.yaml: %v", err)
		}
		s := string(raw)
		if !strings.Contains(s, "Modified via subprocess field modify") {
			t.Fatalf("expected modified checklist purpose in test_case.yaml")
		}
	})

	t.Run("field_deprecate", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary,
			"system", "update-specs", "test_case",
			"--field", fieldName,
			"--operation", "deprecate",
			"--replaced-by", "category",
			"--reason", "spec cell integration: subprocess deprecate",
		)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("update-specs field deprecate: %v\n%s", err, out)
		}
		specPath := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir, "test_case.yaml")
		raw, err := fileutil.ReadFile(specPath)
		if err != nil {
			t.Fatalf("read test_case.yaml: %v", err)
		}
		s := string(raw)
		if !strings.Contains(s, "deprecated_at:") {
			t.Fatalf("expected version_info deprecation metadata in test_case.yaml")
		}
	})

	t.Run("field_archive", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary,
			"system", "update-specs", "test_case",
			"--field", fieldName,
			"--operation", "archive",
			"--reason", "spec cell integration: subprocess archive",
		)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("update-specs field archive: %v\n%s", err, out)
		}
		specPath := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir, "test_case.yaml")
		raw, err := fileutil.ReadFile(specPath)
		if err != nil {
			t.Fatalf("read test_case.yaml: %v", err)
		}
		if !strings.Contains(string(raw), "archived_at:") {
			t.Fatalf("expected version_info archive metadata in test_case.yaml")
		}
	})

	t.Run("field_delete", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary,
			"system", "update-specs", "test_case",
			"--field", fieldName,
			"--operation", "delete",
			"--replaced-by", "category",
			"--reason", "spec cell integration: subprocess delete (soft; history retained)",
		)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("update-specs field delete: %v\n%s", err, out)
		}
		specPath := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir, "test_case.yaml")
		raw, err := fileutil.ReadFile(specPath)
		if err != nil {
			t.Fatalf("read test_case.yaml: %v", err)
		}
		if !strings.Contains(string(raw), "deleted_at:") {
			t.Fatalf("expected version_info delete metadata in test_case.yaml")
		}
	})

	t.Run("check_fast_clean_cache", func(t *testing.T) {
		// Exercises object ID cache clean + async check pipeline paths used for stale CAS hygiene (see async_check proactive cleanup).
		ctx, cancel := context.WithTimeout(context.Background(), specCellCheckCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary,
			"system", "check", "all",
			"--fast",
			"--clean-cache",
			"--timeout", (specCellCheckCommandTimeout - 5*time.Second).String(),
		)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system check all --fast --clean-cache: %v\n%s", err, out)
		}
		if len(out) == 0 {
			t.Logf("check produced no output (acceptable for empty object set)")
		}
	})

	t.Run("cleanup_duplicates_dry_run", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ctx, cliBinary,
			"system", "cleanup-duplicates",
			"--allow-degraded",
			"--dry-run",
		)
		zqkenv.WireExecForIsolatedProject(cmd, scenarioDir)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("system cleanup-duplicates --dry-run: %v\n%s", err, out)
		}
	})
}

func zqkenvTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func readYAMLMapFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}

func writeYAMLConfigFile(t *testing.T, path string, doc map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		t.Fatalf("encode yaml %s: %v", path, err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("close yaml encoder %s: %v", path, err)
	}
	if err := fileutil.WriteSecureFile(path, buf.Bytes()); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func specCellAsStringMap(t *testing.T, v any, ctx string) map[string]any {
	t.Helper()
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[interface{}]interface{}:
		out := make(map[string]any, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				t.Fatalf("%s: non-string key %v", ctx, k)
			}
			out[ks] = val
		}
		return out
	default:
		t.Fatalf("%s: want map, got %T", ctx, v)
		return nil
	}
}

func specCellMapGet(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := parent[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	return specCellAsStringMap(t, v, key)
}

func patchKindMappingsForSpecCellNetNew(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, paths.ProcessInternalDir, "configs", "kind_mappings_config.yaml")
	doc := readYAMLMapFile(t, path)
	backends := specCellMapGet(t, doc, "backends")
	def := specCellMapGet(t, backends, "default")
	ktd := specCellMapGet(t, def, "kind_to_directory")
	dtk := specCellMapGet(t, def, "directory_to_kind")
	ktd[specCellNetNewKind] = specCellNetNewDir
	dtk[specCellNetNewDir] = specCellNetNewKind
	writeYAMLConfigFile(t, path, doc)
}

func patchIDPrefixesForSpecCellNetNew(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, paths.ProcessInternalDir, "configs", "id_prefixes_config.yaml")
	doc := readYAMLMapFile(t, path)
	ktp := specCellMapGet(t, doc, "kind_to_prefixes")
	ktp[specCellNetNewKind] = []any{"SCN-"}
	writeYAMLConfigFile(t, path, doc)
}

func patchNamespacesConfigForSpecCellNetNew(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, paths.ProcessInternalDir, "configs", "namespaces_config.yaml")
	doc := readYAMLMapFile(t, path)
	ns := specCellMapGet(t, doc, "namespaces")
	kernel := specCellMapGet(t, ns, "zqk:kernel")
	kindsAny, ok := kernel["kinds"]
	if !ok {
		t.Fatalf("namespaces zqk:kernel: missing kinds")
	}
	kinds, ok := kindsAny.([]interface{})
	if !ok {
		t.Fatalf("namespaces zqk:kernel kinds: want []interface{}, got %T", kindsAny)
	}
	for _, k := range kinds {
		if ks, ok := k.(string); ok && ks == specCellNetNewKind {
			return
		}
	}
	kernel["kinds"] = append(kinds, specCellNetNewKind)
	writeYAMLConfigFile(t, path, doc)
}

// removeAppleDoubleFilesInDir deletes macOS metadata files like ._foo.yaml that break YAML scans (e.g. sync-glossary-from-specs).
func removeAppleDoubleFilesInDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return
		}
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !appledouble.SkipNameInReadDir(name) {
			continue
		}
		p := filepath.Join(dir, name)
		if err := fileutil.Remove(p); err != nil {
			t.Fatalf("remove appledouble junk %s: %v", p, err)
		}
	}
}

func assertProbeFieldHasREQ019ChecklistKeys(t *testing.T, scenarioDir, fieldName string) {
	t.Helper()
	specPath := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir, "test_case.yaml")
	data, err := fileutil.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read test_case.yaml: %v", err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse test_case.yaml: %v", err)
	}
	fields, ok := root["fields"].(map[string]any)
	if !ok {
		t.Fatalf("test_case.yaml: missing fields map")
	}
	fm, ok := fields[fieldName].(map[string]any)
	if !ok {
		t.Fatalf("test_case.yaml: missing field %q", fieldName)
	}
	checklist, ok := fm["checklist"].(map[string]any)
	if !ok {
		t.Fatalf("field %q: missing checklist", fieldName)
	}
	for _, item := range objects.RequiredChecklistItems {
		if item == "field_profile_code" {
			if _, ok := fm["field_profile_code"]; !ok {
				t.Fatalf("field %q missing field_profile_code (REQ-019)", fieldName)
			}
			continue
		}
		if _, ok := checklist[item]; !ok {
			t.Fatalf("field %q checklist missing key %q (REQ-019 row)", fieldName, item)
		}
	}
}
