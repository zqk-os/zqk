package sentinel

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/multimodal"
	"github.com/lanceman/zqk/pkg/pipeline"
)

type mockAnalyzer struct {
	result multimodal.AnalysisResult
	err    error
}

func (m *mockAnalyzer) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (multimodal.AnalysisResult, error) {
	return m.result, m.err
}

func (m *mockAnalyzer) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (multimodal.AnalysisResult, error) {
	return m.result, m.err
}

func (m *mockAnalyzer) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", m.err
}

func (m *mockAnalyzer) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "", m.err
}

func (m *mockAnalyzer) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return 0.0, m.err
}

func TestBuildQCPipeline_Pass(t *testing.T) {
	p := BuildQCPipeline(nil)

	analyzer := &mockAnalyzer{
		result: multimodal.AnalysisResult{
			TruthScore: 0.9,
			Summary:    "Looks good",
		},
	}

	payload := &SentinelPayload{
		VideoFrames: [][]byte{[]byte("frame")},
		Script:      "test script",
		Analyzer:    analyzer,
	}

	pctx := &pipeline.Context{Ctx: context.Background()}
	res, err := p.Run(pctx, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysisRes, ok := res.(multimodal.AnalysisResult); !ok || analysisRes.TruthScore != 0.9 {
		t.Errorf("unexpected result: %v", res)
	}
}

func TestBuildQCPipeline_Fail(t *testing.T) {
	p := BuildQCPipeline(nil)

	analyzer := &mockAnalyzer{
		result: multimodal.AnalysisResult{
			TruthScore: 0.5,
			Summary:    "Does not match",
		},
	}

	payload := &SentinelPayload{
		VideoFrames: [][]byte{[]byte("frame")},
		Script:      "test script",
		Analyzer:    analyzer,
	}

	pctx := &pipeline.Context{Ctx: context.Background()}
	_, err := p.Run(pctx, payload)
	if err == nil {
		t.Fatalf("expected error for low truth score, got nil")
	}
}
