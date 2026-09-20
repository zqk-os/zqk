package mesh

import (
	"context"

	"github.com/zqk-os/zqk/pkg/llm"
)

// AnalysisResult holds the result of analyzing video frames against a script.
type AnalysisResult = llm.AnalysisResult

// MultimodalAnalyzer defines the interface for analyzing video frames and verifying images.
type MultimodalAnalyzer = llm.MultimodalAnalyzer

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
