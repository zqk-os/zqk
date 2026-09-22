package specorigination

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

func setupSpecOriginationTestRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()

	// Create directory structure
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	configsDir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	_ = os.MkdirAll(specsDir, paths.DirPerm755)
	_ = os.MkdirAll(configsDir, paths.DirPerm755)

	// Create a minimal valid object spec
	specContent := `schema_version: "2.0.0"
ontology: "sample_item"
visibility: "public"
namespace: "zqk:sample"
id_prefixes:
  - "SMP-"
description: "Sample item spec"
traits: []
fields:
  title:
    type: string
    required: true
`
	specPath := filepath.Join(specsDir, "sample_item.yaml")
	_ = os.WriteFile(specPath, []byte(specContent), paths.FilePerm644)

	// Create kind_mappings_config.yaml
	kmContent := `$schema: kind_mappings_schema.json

backends:
  default:
    kind_to_directory:
      existing_kind: "existing_items"
    directory_to_kind:
      existing_items: "existing_kind"
`
	_ = os.WriteFile(filepath.Join(configsDir, paths.KindMappingsConfigFile), []byte(kmContent), paths.FilePerm644)

	// Create id_prefixes_config.yaml
	idpContent := `$schema: id_prefixes_schema.json

kind_to_prefixes:
  existing_kind:
    - "EXI-"
`
	_ = os.WriteFile(filepath.Join(configsDir, paths.IdPrefixesConfigFile), []byte(idpContent), paths.FilePerm644)

	// Create namespaces_config.yaml
	nsContent := `$schema: namespaces_schema.json

namespaces:
  "zqk:sample":
    description: "Sample namespace"
    kinds:
      - "existing_kind"
`
	_ = os.WriteFile(filepath.Join(configsDir, paths.NamespacesConfigFile), []byte(nsContent), paths.FilePerm644)

	return root, specPath
}

func TestRun_EndToEnd_DryRun(t *testing.T) {
	root, _ := setupSpecOriginationTestRepo(t)
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	opts := Options{
		ProjectRoot:              root,
		Ontology:                 "sample_item",
		DryRun:                   true,
		SkipFinalizeValidation:   true,
		SkipMaterializeSpecIndex: true,
	}

	state, err := Run(pctx, logger, opts)
	if err != nil {
		t.Fatalf("Run dry-run failed: %v", err)
	}

	if state == nil {
		t.Fatal("expected non-nil final state")
	}
	if pctx.Outcome[pipeline.OutcomeKeyPlanDryRun] != true {
		t.Errorf("expected plan_dry_run=true, got %v", pctx.Outcome[pipeline.OutcomeKeyPlanDryRun])
	}
	if pctx.Outcome[pipeline.OutcomeKeySpecFileExists] != true {
		t.Errorf("expected spec_file_exists=true, got %v", pctx.Outcome[pipeline.OutcomeKeySpecFileExists])
	}
}

func TestRun_EndToEnd_NonDryRun(t *testing.T) {
	root, _ := setupSpecOriginationTestRepo(t)
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	opts := Options{
		ProjectRoot:              root,
		Ontology:                 "sample_item",
		DryRun:                   false,
		SkipFinalizeValidation:   true,
		SkipMaterializeSpecIndex: true,
		ApplyTrigger:             false,
	}

	state, err := Run(pctx, logger, opts)
	if err != nil {
		t.Fatalf("Run non-dry-run failed: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil final state")
	}
	if pctx.Outcome["membrane_crossed"] != true {
		t.Errorf("expected membrane_crossed=true, got %v", pctx.Outcome["membrane_crossed"])
	}
}

func TestPipeline_StageErrors(t *testing.T) {
	t.Run("stageIngest invalid payload", func(t *testing.T) {
		_, err := stageIngest(nil, "invalid payload type")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("stageNormalize invalid payload", func(t *testing.T) {
		_, err := stageNormalize(nil, "invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("stageDecide invalid payload", func(t *testing.T) {
		_, err := stageDecide(nil, "invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("stageCommit invalid payload", func(t *testing.T) {
		_, err := stageCommit(nil, "invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("stageCommit file missing", func(t *testing.T) {
		st := &State{
			SpecPath: "/nonexistent/spec.yaml",
		}
		_, err := stageCommit(nil, st)
		if err == nil {
			t.Fatal("expected error for missing spec file")
		}
	})

	t.Run("stageCrossMembrane invalid payload", func(t *testing.T) {
		_, err := stageCrossMembrane(nil, "invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("stageMaterializeIndexes invalid payload", func(t *testing.T) {
		_, err := stageMaterializeIndexes(nil, "invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("stageTrigger invalid payload", func(t *testing.T) {
		_, err := stageTrigger(nil, "invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("stageFinalize invalid payload", func(t *testing.T) {
		_, err := stageFinalize(nil, "invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestMutationHelpers(t *testing.T) {
	t.Run("mutateKindMappings", func(t *testing.T) {
		doc := make(map[string]any)
		mutateKindMappings(doc, "book", "books")
		backends := doc["backends"].(map[string]any)
		def := backends["default"].(map[string]any)
		ktd := def["kind_to_directory"].(map[string]any)
		dtk := def["directory_to_kind"].(map[string]any)
		if ktd["book"] != "books" || dtk["books"] != "book" {
			t.Fatalf("mutateKindMappings did not map correctly: %v", doc)
		}
	})

	t.Run("mutateIDPrefixes", func(t *testing.T) {
		doc := make(map[string]any)
		mutateIDPrefixes(doc, "book", []string{"BK-"})
		ktp := doc["kind_to_prefixes"].(map[string]any)
		prefixes := ktp["book"].([]string)
		if len(prefixes) != 1 || prefixes[0] != "BK-" {
			t.Fatalf("mutateIDPrefixes failed: %v", doc)
		}
	})

	t.Run("mutateNamespaces", func(t *testing.T) {
		doc := map[string]any{
			objects.FieldKeyNamespaces: map[string]any{
				"ns1": map[string]any{
					"kinds": []any{"existing"},
				},
			},
		}
		if err := mutateNamespaces(doc, "book", "ns1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// idempotency test
		if err := mutateNamespaces(doc, "book", "ns1"); err != nil {
			t.Fatalf("unexpected error on second add: %v", err)
		}
		// missing namespace test
		if err := mutateNamespaces(doc, "book", "nonexistent_ns"); err == nil {
			t.Fatal("expected error for missing namespace")
		}
		// missing namespaces key
		if err := mutateNamespaces(map[string]any{}, "book", "ns1"); err == nil {
			t.Fatal("expected error for missing namespaces key")
		}
	})
}

func TestPipeline_StageMaterializeIndexes_FieldKeys(t *testing.T) {
	root, _ := setupSpecOriginationTestRepo(t)
	// Create pkg/objects directory so FieldKeys can be written
	_ = os.MkdirAll(filepath.Join(root, "pkg", "objects"), paths.DirPerm755)

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	st := &State{
		Opts: Options{
			ProjectRoot:              root,
			Ontology:                 "sample_item",
			DryRun:                   false,
			SkipMaterializeSpecIndex: true,
			MaterializeFieldKeys:     true,
		},
	}

	_, err := stageMaterializeIndexes(pctx, st)
	if err != nil {
		t.Fatalf("stageMaterializeIndexes failed: %v", err)
	}
	if pctx.Outcome[pipeline.OutcomeKeyFieldKeysWritten] != true {
		t.Errorf("expected field_keys_written=true, got %v", pctx.Outcome[pipeline.OutcomeKeyFieldKeysWritten])
	}
}

func TestPipeline_StageFinalize_ValidationHandling(t *testing.T) {
	root, _ := setupSpecOriginationTestRepo(t)
	loader := objects.NewSpecLoader(root)

	// Create an invalid spec (missing required type on field)
	badSpec := &objects.Spec{
		Ontology: "invalid_spec",
		Fields: map[string]any{
			"bad_field": map[string]any{},
		},
	}

	st := &State{
		Opts: Options{
			ProjectRoot:            root,
			Ontology:               "invalid_spec",
			SkipFinalizeValidation: false,
		},
		Loader: loader,
		Spec:   badSpec,
	}

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	// Should fail with validation error when SkipFinalizeValidation is false
	_, err := stageFinalize(pctx, st)
	if err == nil {
		t.Fatal("expected error when validating bad spec")
	}

	// Should succeed (soft finalize) when SkipFinalizeValidation is true
	st.Opts.SkipFinalizeValidation = true
	pctx.Outcome = make(map[string]any)
	_, err = stageFinalize(pctx, st)
	if err != nil {
		t.Fatalf("expected soft finalize to succeed, got: %v", err)
	}
	if pctx.Outcome[pipeline.OutcomeKeyFinalizeValidationSoft] != true {
		t.Errorf("expected finalize_validation_soft=true")
	}
}

func TestResolveAdminTriggerBinary(t *testing.T) {
	// Calling resolveAdminTriggerBinary in test environment
	_, _ = resolveAdminTriggerBinary()
}

func TestRun_NilContextHandling(t *testing.T) {
	root, _ := setupSpecOriginationTestRepo(t)
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	opts := Options{
		ProjectRoot:              root,
		Ontology:                 "sample_item",
		DryRun:                   true,
		SkipFinalizeValidation:   true,
		SkipMaterializeSpecIndex: true,
	}

	// Test with nil pctx
	st, err := Run(nil, logger, opts)
	if err != nil {
		t.Fatalf("Run with nil pctx failed: %v", err)
	}
	if st == nil {
		t.Fatal("expected non-nil state")
	}

	// Test with pctx having nil Ctx
	pctxEmpty := &pipeline.Context{}
	st, err = Run(pctxEmpty, logger, opts)
	if err != nil {
		t.Fatalf("Run with empty pctx failed: %v", err)
	}
	if st == nil {
		t.Fatal("expected non-nil state")
	}
}
