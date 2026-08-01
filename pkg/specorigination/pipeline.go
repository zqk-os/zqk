package specorigination

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacellregistry"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
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
	st, err := os.Stat(s.SpecPath)
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
		if err := os.WriteFile(outPath, src, paths.FilePerm644); err != nil {
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
		if err := applyTriggerCommands(ctx, s.Opts.ProjectRoot); err != nil {
			return nil, err
		}
		if pctx != nil && pctx.Outcome != nil {
			pctx.Outcome[pipeline.OutcomeKeyTriggerExecuted] = true
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
	errs := objects.ValidateLoadedSpec(ctx, s.Loader, s.Spec, nil)
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
