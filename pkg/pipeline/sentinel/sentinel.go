package sentinel

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/multimodal"
	"github.com/lanceman/zqk/pkg/pipeline"
)

// SentinelPayload represents the input for Sentinel QC pipeline.
type SentinelPayload struct {
	VideoFrames [][]byte
	Script      string
	Analyzer    multimodal.MultimodalAnalyzer
}

// BuildQCPipeline creates a new Sentinel Quality Control pipeline.
func BuildQCPipeline(logger logging.Logger) *pipeline.Pipeline {
	b := pipeline.NewInstrumentedBuilder("sentinel_qc", logger)

	b.AddStage("analyze_video", func(ctx *pipeline.Context, payload any) (any, error) {
		sp, ok := payload.(*SentinelPayload)
		if !ok {
			return nil, fmt.Errorf("invalid payload type, expected *SentinelPayload")
		}

		res, err := sp.Analyzer.AnalyzeVideoFrames(ctx.Ctx, sp.VideoFrames, sp.Script)
		if err != nil {
			return nil, err
		}

		if res.TruthScore < 0.8 {
			return nil, fmt.Errorf("quality control failed: truth score %f is below threshold", res.TruthScore)
		}

		return res, nil
	})

	return b.Build()
}
