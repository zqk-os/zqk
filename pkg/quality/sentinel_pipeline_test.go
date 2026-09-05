package quality

import (
	"context"
	"errors"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mesh"
	"github.com/lanceman/zqk/pkg/pipeline"
)

type mockAnalyzer struct {
	result mesh.AnalysisResult
	err    error
}

func (m *mockAnalyzer) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (mesh.AnalysisResult, error) {
	return m.result, m.err
}

func (m *mockAnalyzer) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (mesh.AnalysisResult, error) {
	return m.result, m.err
}

func (m *mockAnalyzer) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return m.result.Summary, m.err
}

func (m *mockAnalyzer) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return m.result.Summary, m.err
}

func (m *mockAnalyzer) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return m.result.TruthScore, m.err
}

func TestSentinelPipeline(t *testing.T) {
	var logger logging.Logger = nil

	tests := []struct {
		name          string
		payload       *SentinelPayload
		mockResult    mesh.AnalysisResult
		mockErr       error
		expectApprove bool
		expectErr     bool
	}{
		{
			name: "high truth score approves video",
			payload: &SentinelPayload{
				VideoURL: "http://example.com/video.mp4",
				Script:   "Two Neanderthals",
				Frames:   [][]byte{[]byte("dummy")},
			},
			mockResult: mesh.AnalysisResult{
				TruthScore: 0.9,
				Summary:    "Matches well",
			},
			expectApprove: true,
		},
		{
			name: "low truth score rejects video",
			payload: &SentinelPayload{
				VideoURL: "http://example.com/video.mp4",
				Script:   "Two Neanderthals",
				Frames:   [][]byte{[]byte("dummy")},
			},
			mockResult: mesh.AnalysisResult{
				TruthScore: 0.5,
				Summary:    "Too modern",
			},
			expectApprove: false,
		},
		{
			name: "analyzer error propagates",
			payload: &SentinelPayload{
				VideoURL: "http://example.com/video.mp4",
				Script:   "Two Neanderthals",
				Frames:   [][]byte{[]byte("dummy")},
			},
			mockErr:   errors.New("analyzer failed"),
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := &mockAnalyzer{result: tc.mockResult, err: tc.mockErr}
			p := BuildSentinelPipeline(logger, analyzer)

			ctx := &pipeline.Context{}
			result, err := p.Run(ctx, tc.payload)

			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			outPayload, ok := result.(*SentinelPayload)
			if !ok {
				t.Fatalf("expected *SentinelPayload, got %T", result)
			}

			if outPayload.Approved != tc.expectApprove {
				t.Errorf("expected approved=%v, got %v", tc.expectApprove, outPayload.Approved)
			}
		})
	}
}
