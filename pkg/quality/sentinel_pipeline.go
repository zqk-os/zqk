package quality

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/multimodal"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/specbuilder/cli_builders"
)

// SentinelPayload represents the input/output of the sentinel quality control pipeline.
type SentinelPayload struct {
	VideoURL            string
	Script              string
	Frames              [][]byte // Extracted frames
	ObservedDescription string
	TruthScore          float64
	Approved            bool
	Reason              string
}

func toSentinelPayload(payload any) (*SentinelPayload, error) {
	p, ok := payload.(*SentinelPayload)
	if !ok {
		return nil, fmt.Errorf("expected *SentinelPayload, got %T", payload)
	}
	return p, nil
}

// BuildSentinelPipeline constructs the pipeline for validating generated videos using decoupled semantic verification.
func BuildSentinelPipeline(logger logging.Logger, analyzer multimodal.MultimodalAnalyzer) *pipeline.Pipeline {
	b := pipeline.NewBuilder("sentinel_quality", logger)

	b.AddStage("extract_frames", func(ctx *pipeline.Context, payload any) (any, error) {
		p, err := toSentinelPayload(payload)
		if err != nil {
			return nil, err
		}

		if len(p.Frames) > 0 {
			return p, nil
		}

		if p.VideoURL == "" {
			return nil, fmt.Errorf("video URL is empty")
		}

		// Use CLIBuilder to extract 3 frames evenly spaced
		spec := cli_builders.CLISpec{
			Name:    "ffmpeg_extract",
			Command: "ffmpeg",
			Timeout: 60 * time.Second,
		}

		// Create a temporary directory for frames
		tmpDir, err := os.MkdirTemp("", "frames-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		// ffmpeg -i url -vf fps=1/2 -vframes 3 tmpDir/frame_%d.jpg
		builder := cli_builders.NewCLIBuilder(spec, nil).
			WithArgs("-i", p.VideoURL, "-vf", "fps=1/2", "-vframes", "3", filepath.Join(tmpDir, "frame_%d.jpg"))

		if _, err := builder.Execute(ctx.Ctx); err != nil {
			return nil, fmt.Errorf("failed to extract frames: %w", err)
		}

		for i := 1; i <= 3; i++ {
			framePath := filepath.Join(tmpDir, fmt.Sprintf("frame_%d.jpg", i))
			data, err := os.ReadFile(framePath)
			if err != nil {
				// If the video is short, we might not get all 3 frames
				continue
			}
			p.Frames = append(p.Frames, data)
		}

		if len(p.Frames) == 0 {
			return nil, fmt.Errorf("no frames extracted from video")
		}

		return p, nil
	})

	b.AddStage("describe_scene", func(ctx *pipeline.Context, payload any) (any, error) {
		p, err := toSentinelPayload(payload)
		if err != nil {
			return nil, err
		}

		// Extract a purely literal, objective description of the frames
		desc, err := analyzer.DescribeScene(ctx.Ctx, p.Frames)
		if err != nil {
			return nil, fmt.Errorf("scene description failed: %w", err)
		}

		p.ObservedDescription = desc
		return p, nil
	})

	b.AddStage("semantic_compare", func(ctx *pipeline.Context, payload any) (any, error) {
		p, err := toSentinelPayload(payload)
		if err != nil {
			return nil, err
		}

		// Calculate cosine similarity between the observed description and the expected script
		score, err := analyzer.SemanticCompare(ctx.Ctx, p.ObservedDescription, p.Script)
		if err != nil {
			return nil, fmt.Errorf("semantic comparison failed: %w", err)
		}

		p.TruthScore = score
		return p, nil
	})

	b.AddStage("decide_approval", func(ctx *pipeline.Context, payload any) (any, error) {
		p, err := toSentinelPayload(payload)
		if err != nil {
			return nil, err
		}

		// Threshold for approval
		if p.TruthScore >= 0.8 {
			p.Approved = true
			p.Reason = "Semantic Truth Score meets threshold"
		} else {
			p.Approved = false
			p.Reason = fmt.Sprintf("Semantic Truth Score %.2f is below threshold 0.8. Observed: %s", p.TruthScore, p.ObservedDescription)
		}

		return p, nil
	})

	return b.Build()
}
