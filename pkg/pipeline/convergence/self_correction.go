package convergence

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/pipeline"
)

// SelfCorrectionPayload represents the input for the Self-Correction pipeline.
type SelfCorrectionPayload struct {
	PlanID          string
	DriftDetected   bool
	FailedTaskIDs   []string
	TargetMilestone string
}

// BuildSelfCorrectionPipeline creates a new self-correction pipeline to remediate execution drift.
func BuildSelfCorrectionPipeline(logger logging.Logger) *pipeline.Pipeline {
	b := pipeline.NewInstrumentedBuilder("self_correction", logger)

	b.AddStage("detect_drift", func(ctx *pipeline.Context, payload any) (any, error) {
		p, ok := payload.(*SelfCorrectionPayload)
		if !ok {
			return nil, fmt.Errorf("invalid payload type, expected *SelfCorrectionPayload")
		}

		// In a real implementation, this would query the ConvergenceController
		if !p.DriftDetected {
			// No drift, short-circuit or proceed with no ops
			return p, nil
		}

		return p, nil
	})

	b.AddStage("reallocate_resources", func(ctx *pipeline.Context, payload any) (any, error) {
		p, ok := payload.(*SelfCorrectionPayload)
		if !ok {
			return nil, fmt.Errorf("invalid payload type, expected *SelfCorrectionPayload")
		}

		if p.DriftDetected {
			// Mock reallocation
			ctx.Outcome = map[string]any{
				"resources_reallocated": true,
			}
		}

		return p, nil
	})

	b.AddStage("update_priority_plan", func(ctx *pipeline.Context, payload any) (any, error) {
		p, ok := payload.(*SelfCorrectionPayload)
		if !ok {
			return nil, fmt.Errorf("invalid payload type, expected *SelfCorrectionPayload")
		}

		if p.DriftDetected {
			// Mock priority plan update
			if ctx.Outcome == nil {
				ctx.Outcome = make(map[string]any)
			}
			ctx.Outcome["plan_updated"] = true
		}

		return p, nil
	})

	return b.Build()
}
