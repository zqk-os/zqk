package specorigination

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/contractchange"
	"github.com/zqk-os/zqk/pkg/datacellregistry"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// BuildSpecOriginationPipeline constructs the spec origination pipeline (see DATA_ORIGINATION_PIPELINE_VISION.md).
// Callers run it with [pipeline.Pipeline.Run], passing [Options] as the initial payload.
func BuildSpecOriginationPipeline(logger logging.Logger, opts Options) (*pipeline.Pipeline, error) {
	if err := ValidateOptions(opts); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(context.Background()))
	}

	mc := pipeline.DefaultMetricsConfig(logger)

	b := pipeline.NewBuilder(PipelineKind, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(mc).
		AddStage(StageIngest, stageIngest).
		AddStage(StageNormalize, stageNormalize).
		AddStage(StageDecide, stageDecide).
		AddStage(StageCommit, stageCommit).
		AddStage(StageCrossMembrane, stageCrossMembrane).
		AddStage(StageMaterializeIndexes, stageMaterializeIndexes).
		AddStage(StageTriggerSideEffects, stageTrigger).
		AddStage(StageFinalize, stageFinalize)

	return b.Build(), nil
}

// Run executes the pipeline with a fresh [pipeline.Context] when pctx is nil.
// On success, returns the final [State]; pctx.Outcome holds observability keys.
func Run(pctx *pipeline.Context, logger logging.Logger, opts Options) (*State, error) {
	pl, err := BuildSpecOriginationPipeline(logger, opts)
	if err != nil {
		return nil, err
	}
	if pctx == nil {
		pctx = &pipeline.Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	} else if pctx.Ctx == nil {
		pctx.Ctx = pkgctx.NewSystemContext()
	}
	if pctx.Outcome == nil {
		pctx.Outcome = make(map[string]any)
	}
	out, err := pl.Run(pctx, opts)
	if err != nil {
		return nil, err
	}
	s, ok := out.(*State)
	if !ok {
		return nil, errfmt.Errorf("spec origination: unexpected final payload type %T", out)
	}
	return s, nil
}

func stageIngest(_ *pipeline.Context, payload any) (any, error) {
	opts, ok := payload.(Options)
	if !ok {
		return nil, errfmt.Errorf("SPEC_ORIGIN_INGEST: expected Options, got %T", payload)
	}
	if err := ValidateOptions(opts); err != nil {
		return nil, err
	}
	specPath := filepath.Join(opts.ProjectRoot, paths.ProcessInternalObjectSpecsDir, opts.Ontology+".yaml")
	s := &State{
		Opts:     opts,
		Loader:   objects.NewSpecLoader(opts.ProjectRoot),
		SpecPath: specPath,
	}
	return s, nil
}

func stageNormalize(pctx *pipeline.Context, pl any) (any, error) {
	s, ok := pl.(*State)
	if !ok {
		return nil, errfmt.Errorf("SPEC_ORIGIN_NORMALIZE: expected *State, got %T", pl)
	}
	if pctx != nil && pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeySpecPath] = s.SpecPath
	}
	spec, err := s.Loader.LoadSpecWithInheritance(s.Opts.Ontology + ".yaml")
	if err != nil {
		return nil, errfmt.Errorf("load spec: %w", err)
	}
	s.Spec = spec
	return s, nil
}

func stageDecide(pctx *pipeline.Context, pl any) (any, error) {
	s, ok := pl.(*State)
	if !ok {
		return nil, errfmt.Errorf("SPEC_ORIGIN_DECIDE: expected *State, got %T", pl)
	}
	if pctx != nil && pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyPlanDryRun] = s.Opts.DryRun
		pctx.Outcome[pipeline.OutcomeKeyPlanSkipSpecIndex] = s.Opts.SkipMaterializeSpecIndex
		pctx.Outcome[pipeline.OutcomeKeyPlanFieldKeys] = s.Opts.MaterializeFieldKeys
		pctx.Outcome[pipeline.OutcomeKeyPlanApplyTrigger] = s.Opts.ApplyTrigger
	}
	return s, nil
}

func stageCommit(pctx *pipeline.Context, pl any) (any, error) {
	s, ok := pl.(*State)
	if !ok {
		return nil, errfmt.Errorf("SPEC_ORIGIN_COMMIT: expected *State, got %T", pl)
	}
	st, err := fileutil.Stat(s.SpecPath)
	if err != nil {
		return nil, errfmt.Errorf("spec file required at %s: %w", s.SpecPath, err)
	}
	if st.IsDir() {
		return nil, errfmt.Errorf("spec path is a directory: %s", s.SpecPath)
	}
	if pctx != nil && pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeySpecFileExists] = true
	}
	return s, nil
}

func stageCrossMembrane(pctx *pipeline.Context, pl any) (any, error) {
	s, ok := pl.(*State)
	if !ok {
		return nil, errfmt.Errorf("SPEC_ORIGIN_CROSS_MEMBRANE: expected *State, got %T", pl)
	}

	if s.Opts.DryRun {
		return s, nil
	}

	spec := s.Spec
	if spec == nil {
		return nil, errfmt.Errorf("SPEC_ORIGIN_CROSS_MEMBRANE: spec is nil")
	}
	kind := spec.Ontology
	if kind == "" {
		return s, nil // No ontology, skip
	}

	kmPath := filepath.Join(s.Opts.ProjectRoot, paths.ProcessInternalConfigsDir, paths.KindMappingsConfigFile)
	idpPath := filepath.Join(s.Opts.ProjectRoot, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
	nsPath := filepath.Join(s.Opts.ProjectRoot, paths.ProcessInternalConfigsDir, paths.NamespacesConfigFile)

	// Determine directory using inference rules for irregular plurals (S04)
	dirName := kind + "s"
	if kmCfg, err := objects.LoadKindMappingsConfig(kmPath); err == nil {
		if inferredDir := kmCfg.GetDirectoryFromKind(kind); inferredDir != "" {
			dirName = inferredDir
		}
	}

	// 1. Prepare Kind Mappings mutation
	mutations := make([]func() error, 0, 3)

	kmDoc, err := loadYAMLConfigDoc(kmPath)
	if err != nil {
		return nil, errfmt.Errorf("fail-closed: load kind_mappings: %w", err)
	}
	mutateKindMappings(kmDoc.doc, kind, dirName)
	mutations = append(mutations, func() error { return writeYAMLConfigDoc(kmPath, kmDoc) })

	// 2. Prepare ID Prefixes mutation (only if defined in spec, but fail-closed if file missing and we need to write)
	if len(spec.IDPrefixes) > 0 {
		idpDoc, err := loadYAMLConfigDoc(idpPath)
		if err != nil {
			return nil, errfmt.Errorf("fail-closed: load id_prefixes: %w", err)
		}
		mutateIDPrefixes(idpDoc.doc, kind, spec.IDPrefixes)
		mutations = append(mutations, func() error { return writeYAMLConfigDoc(idpPath, idpDoc) })
	}

	// 3. Prepare Namespaces mutation (only if defined in spec)
	if spec.Namespace != "" {
		nsDoc, err := loadYAMLConfigDoc(nsPath)
		if err != nil {
			return nil, errfmt.Errorf("fail-closed: load namespaces: %w", err)
		}
		err = mutateNamespaces(nsDoc.doc, kind, spec.Namespace)
		if err != nil {
			return nil, errfmt.Errorf("fail-closed: mutate namespaces: %w", err)
		}
		mutations = append(mutations, func() error { return writeYAMLConfigDoc(nsPath, nsDoc) })
	}

	// Apply all mutations atomically
	for _, mut := range mutations {
		if err := mut(); err != nil {
			return nil, errfmt.Errorf("atomic commit failed: %w", err)
		}
	}

	if pctx != nil && pctx.Outcome != nil {
		pctx.Outcome["membrane_crossed"] = true
	}
	return s, nil
}

type yamlDoc struct {
	schema string
	doc    map[string]any
}

func loadYAMLConfigDoc(path string) (*yamlDoc, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var schema string
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "$schema:") {
		schema = lines[0] + "\n\n"
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if schema != "" {
		delete(doc, "$schema")
	}
	return &yamlDoc{schema: schema, doc: doc}, nil
}

func writeYAMLConfigDoc(path string, doc *yamlDoc) error {
	outData, err := yaml.Marshal(doc.doc)
	if err != nil {
		return err
	}
	finalOutput := []byte(doc.schema + string(outData))
	return fileutil.WriteFile(path, finalOutput, 0644)
}

func mutateKindMappings(doc map[string]any, kind, dirName string) {
	backends, ok := doc["backends"].(map[string]any)
	if !ok {
		backends = make(map[string]any)
		doc["backends"] = backends
	}
	def, ok := backends["default"].(map[string]any)
	if !ok {
		def = make(map[string]any)
		backends["default"] = def
	}
	ktd, ok := def["kind_to_directory"].(map[string]any)
	if !ok {
		ktd = make(map[string]any)
		def["kind_to_directory"] = ktd
	}
	dtk, ok := def["directory_to_kind"].(map[string]any)
	if !ok {
		dtk = make(map[string]any)
		def["directory_to_kind"] = dtk
	}
	ktd[kind] = dirName
	dtk[dirName] = kind
}

func mutateIDPrefixes(doc map[string]any, kind string, prefixes []string) {
	ktp, ok := doc["kind_to_prefixes"].(map[string]any)
	if !ok {
		ktp = make(map[string]any)
		doc["kind_to_prefixes"] = ktp
	}
	ktp[kind] = prefixes
}

func mutateNamespaces(doc map[string]any, kind, namespace string) error {
	nsAny, ok := doc[objects.FieldKeyNamespaces].(map[string]any)
	if !ok {
		return errfmt.Errorf("namespaces key missing")
	}
	nsMap, ok := nsAny[namespace].(map[string]any)
	if !ok {
		return errfmt.Errorf("namespace %q not found in config", namespace)
	}
	kindsAny, ok := nsMap["kinds"].([]any)
	if !ok {
		kindsAny = []any{}
	}
	for _, k := range kindsAny {
		if ks, ok := k.(string); ok && ks == kind {
			return nil
		}
	}
	nsMap["kinds"] = append(kindsAny, kind)
	return nil
}

func stageMaterializeIndexes(pctx *pipeline.Context, pl any) (any, error) {
	s, ok := pl.(*State)
	if !ok {
		return nil, errfmt.Errorf("SPEC_ORIGIN_MATERIALIZE_INDEXES: expected *State, got %T", pl)
	}
	if s.Opts.DryRun {
		if pctx != nil && pctx.Outcome != nil {
			pctx.Outcome[pipeline.OutcomeKeySpecIndexRefreshed] = false
			pctx.Outcome[pipeline.OutcomeKeyFieldKeysWritten] = false
		}
		return s, nil
	}
	if !s.Opts.SkipMaterializeSpecIndex {
		if _, err := objects.RefreshMaterializedSpecIndex(s.Opts.ProjectRoot); err != nil {
			return nil, errfmt.Errorf("refresh spec index: %w", err)
		}
		datacellregistry.InvalidateDescriptorReadModelCache(s.Opts.ProjectRoot)
		if pctx != nil && pctx.Outcome != nil {
			pctx.Outcome[pipeline.OutcomeKeySpecIndexRefreshed] = true
		}
	}
	if s.Opts.MaterializeFieldKeys {
		specsDir := filepath.Join(s.Opts.ProjectRoot, paths.ProcessInternalObjectSpecsDir)
		src, err := objects.GenerateFieldKeysGoSource(specsDir)
		if err != nil {
			return nil, errfmt.Errorf("generate field_keys.go: %w", err)
		}
		outPath := filepath.Join(s.Opts.ProjectRoot, "pkg", "objects", "field_keys.go")
		if err := fileutil.WriteFile(outPath, src, paths.FilePerm644); err != nil {
			return nil, errfmt.Errorf("write field_keys.go: %w", err)
		}
		if pctx != nil && pctx.Outcome != nil {
			pctx.Outcome[pipeline.OutcomeKeyFieldKeysWritten] = true
		}
	}
	return s, nil
}

func stageTrigger(pctx *pipeline.Context, pl any) (any, error) {
	s, ok := pl.(*State)
	if !ok {
		return nil, errfmt.Errorf("SPEC_ORIGIN_TRIGGER: expected *State, got %T", pl)
	}
	note := strings.Join([]string{
		"After spec changes, run when applicable:",
		"  zqk system sync-glossary-from-specs [--apply]",
		"  zqk system generate-instance-builders --overwrite",
		"  zqk system path-cache  (optional)",
		"Or pass --apply-trigger (not with --dry-run) to run these steps automatically.",
	}, "\n")
	if pctx != nil && pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyTriggerManualCommands] = note
	}

	if !s.Opts.DryRun && s.Opts.ApplyTrigger {
		ctx := context.Background()
		if pctx != nil && pctx.Ctx != nil {
			ctx = pctx.Ctx
		}
		if err := applyTriggerCommands(ctx, s.Opts.ProjectRoot, s.Opts.Ontology); err != nil {
			return nil, err
		}
		if pctx != nil && pctx.Outcome != nil {
			pctx.Outcome[pipeline.OutcomeKeyTriggerExecuted] = true
		}
	}
	// Durable contract-change outbox (async demote on kernel/scheduler start).
	// TRACK: BLI-1785918841712163000-f128dc79
	if !s.Opts.DryRun {
		if err := contractchange.EmitForKind(s.Opts.ProjectRoot, s.Opts.Ontology, "spec_origin_trigger"); err != nil {
			return nil, errfmt.Errorf("SPEC_ORIGIN_TRIGGER: contract-change emit: %w", err)
		}
	}
	return s, nil
}

func stageFinalize(pctx *pipeline.Context, pl any) (any, error) {
	s, ok := pl.(*State)
	if !ok {
		return nil, errfmt.Errorf("SPEC_ORIGIN_FINALIZE: expected *State, got %T", pl)
	}
	ctx := context.Background()
	if pctx != nil && pctx.Ctx != nil {
		ctx = pctx.Ctx
	}
	errs := objects.ValidateLoadedSpec(ctx, s.Loader, authoredFieldValidationSpec(s.Spec), nil)
	if pctx != nil && pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyValidationErrorCount] = len(errs)
	}
	if len(errs) > 0 && s.Opts.SkipFinalizeValidation {
		if pctx != nil && pctx.Outcome != nil {
			pctx.Outcome[pipeline.OutcomeKeyFinalizeValidationSoft] = true
		}
		return s, nil
	}
	if len(errs) > 0 {
		var b strings.Builder
		for i, e := range errs {
			if i > 0 {
				b.WriteString("; ")
			}
			if e.Field != "" {
				b.WriteString(e.Field)
				b.WriteString(": ")
			}
			b.WriteString(e.Message)
		}
		return nil, errfmt.Errorf("spec validation failed (%d): %s", len(errs), b.String())
	}
	return s, nil
}
