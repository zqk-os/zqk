package mesh

import (
	"context"
	"testing"
)

func TestAnalyzeVideoFrames(t *testing.T) {
	ctx := context.Background()

	// Create a mock analyzer
	analyzer := &MockMultimodalAnalyzer{
		MockResult: AnalysisResult{
			TruthScore: 0.95,
			Summary:    "The frames match the script perfectly.",
		},
	}

	frames := [][]byte{
		[]byte("frame1"),
		[]byte("frame2"),
	}
	script := "A person walks into the room."

	result, err := analyzer.AnalyzeVideoFrames(ctx, frames, script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TruthScore != 0.95 {
		t.Errorf("expected TruthScore 0.95, got %v", result.TruthScore)
	}

	if result.Summary != "The frames match the script perfectly." {
		t.Errorf("expected Summary 'The frames match the script perfectly.', got '%s'", result.Summary)
	}
}

type MockMultimodalAnalyzer struct {
	MockResult AnalysisResult
	MockError  error
}

func (m *MockMultimodalAnalyzer) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (AnalysisResult, error) {
	return m.MockResult, m.MockError
}

func (m *MockMultimodalAnalyzer) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (AnalysisResult, error) {
	return m.MockResult, m.MockError
}

func (m *MockMultimodalAnalyzer) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", m.MockError
}

func (m *MockMultimodalAnalyzer) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "", m.MockError
}

func (m *MockMultimodalAnalyzer) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return 0.0, m.MockError
}

func TestMultimodalPromptAndHook(t *testing.T) {
	prompt := MultimodalPrompt{
		TextPrompt: "Test prompt",
		Images:     [][]byte{[]byte("image1")},
	}

	if prompt.TextPrompt != "Test prompt" {
		t.Errorf("Expected Test prompt, got %s", prompt.TextPrompt)
	}

	mockHook := &MockMultimodalHook{}
	ctx := context.Background()
	_, err := mockHook.BeforeAnalysis(ctx, prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type MockMultimodalHook struct{}

func (m *MockMultimodalHook) BeforeAnalysis(ctx context.Context, prompt MultimodalPrompt) (context.Context, error) {
	return ctx, nil
}

func (m *MockMultimodalHook) AfterAnalysis(ctx context.Context, result AnalysisResult) error {
	return nil
}
