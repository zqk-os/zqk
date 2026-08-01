package multimodal

import "context"

// AnalysisResult holds the result of analyzing video frames against a script.
type AnalysisResult struct {
	TruthScore float64 `json:"truth_score"`
	Summary    string  `json:"summary"`
}

// MultimodalAnalyzer defines the interface for analyzing video frames and verifying images.
type MultimodalAnalyzer interface {
	AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (AnalysisResult, error)
	// VerifyImage checks an image against a text prompt and an optional reference image.
	VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (AnalysisResult, error)
	// DescribeImage returns a literal, objective text description of the provided image.
	DescribeImage(ctx context.Context, image []byte) (string, error)
	// DescribeScene returns a comprehensive semantic description of a scene across multiple frames (setting, mood, action).
	DescribeScene(ctx context.Context, frames [][]byte) (string, error)
	// SemanticCompare calculates the cosine similarity or semantic alignment between an observed description and an expected narrative.
	SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error)
}

// MultimodalPrompt defines the structure for a prompt sent to a multimodal LLM.
type MultimodalPrompt struct {
	TextPrompt string   `json:"text_prompt"`
	Images     [][]byte `json:"images,omitempty"`
	Video      [][]byte `json:"video,omitempty"`
}

// MultimodalHook defines API hooks for multimodal LLM operations.
type MultimodalHook interface {
	BeforeAnalysis(ctx context.Context, prompt MultimodalPrompt) (context.Context, error)
	AfterAnalysis(ctx context.Context, result AnalysisResult) error
}
