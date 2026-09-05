package llm

import (
	"context"
	"strings"

	"github.com/lanceman/zqk/pkg/mesh"
)

// StaticMockClient returns deterministic offline responses when the local/primary
// LLM is unreachable and no secondary endpoint is configured.
// TRACK: REDACTED
type StaticMockClient struct{}

const mockUnavailablePrefix = "[mock-fallback] "

// IsMockFallback reports whether content came from the offline static client.
// Execution paths must not treat this diagnostic response as completed work.
func IsMockFallback(content string) bool {
	return strings.HasPrefix(content, mockUnavailablePrefix)
}

func (StaticMockClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	_ = ctx
	return mockUnavailablePrefix + "semantic intent unavailable (offline)", nil
}

func (StaticMockClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	_ = text
	return []float32{0, 0, 0, 0}, nil
}

func (StaticMockClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (mesh.AnalysisResult, error) {
	_ = ctx
	_ = frames
	_ = script
	return mesh.AnalysisResult{TruthScore: 0, Summary: mockUnavailablePrefix + "video analysis offline"}, nil
}

func (StaticMockClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (mesh.AnalysisResult, error) {
	_ = ctx
	_ = image
	_ = referenceImage
	_ = textPrompt
	return mesh.AnalysisResult{TruthScore: 0, Summary: mockUnavailablePrefix + "image verify offline"}, nil
}

func (StaticMockClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	_ = ctx
	_ = prompt
	_ = system
	return mockUnavailablePrefix + "completion unavailable; local LLM timed out or failed", nil
}

func (StaticMockClient) GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
	_ = ctx
	_ = messages
	_ = tools
	return StructuredCompletionResponse{Content: mockUnavailablePrefix + "structured completion unavailable"}, nil
}

func (StaticMockClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	_ = ctx
	_ = image
	return mockUnavailablePrefix + "image description unavailable", nil
}

func (StaticMockClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	_ = ctx
	_ = frames
	return mockUnavailablePrefix + "scene description unavailable", nil
}

func (StaticMockClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	_ = ctx
	_ = observedDescription
	_ = expectedNarrative
	return 0, nil
}
