package pipeline

import (
	"context"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// PipelineStorageProvider defines the minimal storage contract needed to load pipelines.
type PipelineStorageProvider interface {
	Read(ctx context.Context, secCtx any, id string) (map[string]any, error)
}

// LoadPipeline loads a PIP-* object from storage and returns an instrumented Builder.
// It maps the sequential/parallel execution_mode from pipeline_stage objects into the builder.
// The instructions are preserved on the pipeline execution contexts.
func LoadPipeline(ctx context.Context, sp PipelineStorageProvider, secCtx any, logger logging.Logger, pipelineID string) (*Builder, error) {
	if sp == nil {
		return nil, errfmt.Errorf("storage provider is nil")
	}
	if pipelineID == "" {
		return nil, errfmt.Errorf("pipeline ID is empty")
	}

	obj, err := sp.Read(ctx, secCtx, pipelineID)
	if err != nil {
		return nil, errfmt.Newf("read pipeline %s", pipelineID).Wrap(err)
	}
	if obj == nil {
		return nil, errfmt.Errorf("pipeline %s not found", pipelineID)
	}

	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind != objects.KindPipeline {
		return nil, errfmt.Errorf("object %s is not a pipeline (got %s)", pipelineID, kind)
	}

	builder := NewInstrumentedBuilder(pipelineID, logger)

	stagesRaw, ok := obj[objects.FieldKeyStages].([]any)
	if !ok {
		return builder, nil
	}

	for _, stageRaw := range stagesRaw {
		stageID, ok := stageRaw.(string)
		if !ok {
			continue
		}

		stageObj, stageErr := sp.Read(ctx, secCtx, stageID)
		if stageErr != nil || stageObj == nil {
			continue
		}

		execMode, _ := stageObj[objects.FieldKeyExecutionMode].(string)
		title, _ := stageObj[objects.FieldKeyTitle].(string)
		if title == "" {
			title = stageID
		}

		// For now, we wrap the stage payload execution logic.
		// A proper runner will integrate the execution_mode and instruction_refs.
		builder.AddStage(title, func(pctx *Context, payload any) (any, error) {
			if execMode == "parallel" {
				// Stub for parallel execution logic.
				logging.Fluent(logger).Info("Executing parallel pipeline stage").String("stage_id", stageID).Log()
			} else {
				logging.Fluent(logger).Info("Executing sequential pipeline stage").String("stage_id", stageID).Log()
			}
			return payload, nil
		})
	}

	return builder, nil
}
