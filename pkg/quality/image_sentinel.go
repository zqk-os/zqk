package quality

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

// ImageVerificationPayload represents the input/output of the image verification pipeline.
type ImageVerificationPayload struct {
	Image          []byte
	ReferenceImage []byte
	TextPrompt     string
	Result         llm.AnalysisResult
	Approved       bool
	Reason         string
}

// BuildImageVerificationPipeline constructs a pipeline for verifying an image against a text prompt and an optional reference image.
func BuildImageVerificationPipeline(logger logging.Logger, analyzer llm.MultimodalAnalyzer) *pipeline.Pipeline {
	b := pipeline.NewBuilder("image_verification", logger)

	b.AddStage("verify_image", func(ctx *pipeline.Context, payload any) (any, error) {
		p, ok := payload.(*ImageVerificationPayload)
		if !ok {
			return nil, fmt.Errorf("expected *ImageVerificationPayload, got %T", payload)
		}

		res, err := analyzer.VerifyImage(ctx.Ctx, p.Image, p.ReferenceImage, p.TextPrompt)
		if err != nil {
			return nil, fmt.Errorf("image verification failed: %w", err)
		}

		p.Result = res
		return p, nil
	})

	b.AddStage("decide_approval", func(ctx *pipeline.Context, payload any) (any, error) {
		p, ok := payload.(*ImageVerificationPayload)
		if !ok {
			return nil, fmt.Errorf("expected *ImageVerificationPayload, got %T", payload)
		}

		// Threshold for approval
		if p.Result.TruthScore >= 0.8 {
			p.Approved = true
			p.Reason = "Truth score meets threshold"
		} else {
			p.Approved = false
			p.Reason = fmt.Sprintf("Truth score %.2f is below threshold 0.8: %s", p.Result.TruthScore, p.Result.Summary)
		}

		return p, nil
	})

	return b.Build()
}
