package quality

import (
	"errors"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mesh"
	"github.com/lanceman/zqk/pkg/pipeline"
)

func TestImageVerificationPipeline(t *testing.T) {
	var logger logging.Logger = nil

	tests := []struct {
		name          string
		payload       *ImageVerificationPayload
		mockResult    mesh.AnalysisResult
		mockErr       error
		expectApprove bool
		expectErr     bool
	}{
		{
			name: "high truth score approves image",
			payload: &ImageVerificationPayload{
				Image:      []byte("image_data"),
				TextPrompt: "A red car",
			},
			mockResult: mesh.AnalysisResult{
				TruthScore: 0.85,
				Summary:    "Matches well",
			},
			expectApprove: true,
		},
		{
			name: "low truth score rejects image",
			payload: &ImageVerificationPayload{
				Image:      []byte("image_data"),
				TextPrompt: "A red car",
			},
			mockResult: mesh.AnalysisResult{
				TruthScore: 0.4,
				Summary:    "It's a blue truck",
			},
			expectApprove: false,
		},
		{
			name: "analyzer error propagates",
			payload: &ImageVerificationPayload{
				Image:      []byte("image_data"),
				TextPrompt: "A red car",
			},
			mockErr:   errors.New("analyzer failed"),
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := &mockAnalyzer{result: tc.mockResult, err: tc.mockErr}
			p := BuildImageVerificationPipeline(logger, analyzer)

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

			outPayload, ok := result.(*ImageVerificationPayload)
			if !ok {
				t.Fatalf("expected *ImageVerificationPayload, got %T", result)
			}

			if outPayload.Approved != tc.expectApprove {
				t.Errorf("expected approved=%v, got %v", tc.expectApprove, outPayload.Approved)
			}
		})
	}
}
