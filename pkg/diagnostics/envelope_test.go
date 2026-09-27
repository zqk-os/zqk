package diagnostics_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/diagnostics"
)

func TestExtractFailureEnvelope_AgnosticAttribution(t *testing.T) {
	errSample := errors.New("command terminated unexpectedly")
	actions := []string{"git checkout -b branch", "go build ./..."}
	constraints := []string{"must pass zero warnings", "exit code must be 0"}

	report := &diagnostics.DiagnosticReport{
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
		Results: []diagnostics.ResultRecord{
			{
				Extractor: "test_extractor",
				OK:        true,
				Artifact: &diagnostics.DiagnosticArtifact{
					Name:      "test_artifact",
					SizeBytes: 42,
					MIMEType:  "text/plain",
					Payload:   "diagnostic log payload",
				},
			},
		},
	}

	env := diagnostics.ExtractFailureEnvelope(1, 1, errSample, actions, constraints, report)

	if env.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", env.Attempt)
	}
	if env.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", env.ExitCode)
	}
	if env.ErrorMessage != "command terminated unexpectedly" {
		t.Errorf("expected error message to match, got %q", env.ErrorMessage)
	}
	if env.Category != diagnostics.CategoryConstraintViolation {
		t.Errorf("expected category %q, got %q", diagnostics.CategoryConstraintViolation, env.Category)
	}
	if len(env.FailingConstraints) != 2 {
		t.Errorf("expected 2 failing constraints, got %d", len(env.FailingConstraints))
	}
	if len(env.ExecutedActions) != 2 {
		t.Errorf("expected 2 executed actions, got %d", len(env.ExecutedActions))
	}
	if len(env.DiagnosticReports) != 1 {
		t.Errorf("expected 1 diagnostic artifact, got %d", len(env.DiagnosticReports))
	}
	if env.Signature == "" {
		t.Error("expected non-empty deterministic signature")
	}
	if env.IsSuccess() {
		t.Error("expected IsSuccess to be false for failure envelope")
	}
}

func TestExtractFailureEnvelope_ErrorCategorization(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		exitCode    int
		constraints []string
		expectedCat string
	}{
		{
			name:        "clean success",
			err:         nil,
			exitCode:    0,
			constraints: nil,
			expectedCat: diagnostics.CategorySuccess,
		},
		{
			name:        "constraint violation takes precedence",
			err:         errors.New("generic error"),
			exitCode:    1,
			constraints: []string{"constraint A"},
			expectedCat: diagnostics.CategoryConstraintViolation,
		},
		{
			name:        "timeout classification",
			err:         errors.New("operation timed out after 30s"),
			exitCode:    0,
			constraints: nil,
			expectedCat: diagnostics.CategoryTimeout,
		},
		{
			name:        "context cancelled classification",
			err:         context.Canceled,
			exitCode:    0,
			constraints: nil,
			expectedCat: diagnostics.CategoryContextCancelled,
		},
		{
			name:        "verification failure classification",
			err:         errors.New("verification failed: invariant broken"),
			exitCode:    0,
			constraints: nil,
			expectedCat: diagnostics.CategoryVerificationFailed,
		},
		{
			name:        "exit code failure",
			err:         errors.New("process error"),
			exitCode:    2,
			constraints: nil,
			expectedCat: diagnostics.CategoryExecutionFailure,
		},
		{
			name:        "unknown error without exit code",
			err:         errors.New("unrecognized phenomenon"),
			exitCode:    0,
			constraints: nil,
			expectedCat: diagnostics.CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := diagnostics.ClassifyError(tt.err, tt.exitCode, tt.constraints)
			if cat != tt.expectedCat {
				t.Errorf("got category %q, want %q", cat, tt.expectedCat)
			}
		})
	}
}

func TestExtractFailureEnvelope_BoundaryConditions(t *testing.T) {
	// Negative attempt normalized to 1
	env := diagnostics.ExtractFailureEnvelope(-5, 0, nil, nil, nil, nil)
	if env.Attempt != 1 {
		t.Errorf("expected attempt normalized to 1, got %d", env.Attempt)
	}
	if !env.IsSuccess() {
		t.Error("expected clean envelope with nil error and 0 exit to be success")
	}

	// Oversized error message truncated safely
	hugeError := strings.Repeat("x", diagnostics.DefaultMaxErrorLength+500)
	envHuge := diagnostics.ExtractFailureEnvelope(1, 1, errors.New(hugeError), nil, nil, nil)
	if len(envHuge.ErrorMessage) > diagnostics.DefaultMaxErrorLength {
		t.Errorf("expected error message bounded to %d, got %d", diagnostics.DefaultMaxErrorLength, len(envHuge.ErrorMessage))
	}

	// Oversized action list bounded safely
	manyActions := make([]string, diagnostics.DefaultMaxActionsLimit+100)
	for i := range manyActions {
		manyActions[i] = "action"
	}
	envActions := diagnostics.ExtractFailureEnvelope(1, 1, errors.New("err"), manyActions, nil, nil)
	if len(envActions.ExecutedActions) > diagnostics.DefaultMaxActionsLimit {
		t.Errorf("expected actions bounded to %d, got %d", diagnostics.DefaultMaxActionsLimit, len(envActions.ExecutedActions))
	}
}

func TestComputeSignature_Determinism(t *testing.T) {
	constraints1 := []string{"alpha", "beta", "gamma"}
	constraints2 := []string{"gamma", "alpha", "beta"}
	actions := []string{"run a", "run b"}

	sig1 := diagnostics.ComputeSignature(diagnostics.CategoryExecutionFailure, 1, "test err", constraints1, actions)
	sig2 := diagnostics.ComputeSignature(diagnostics.CategoryExecutionFailure, 1, "test err", constraints2, actions)

	if sig1 != sig2 {
		t.Errorf("signature should be invariant to constraint ordering: %q vs %q", sig1, sig2)
	}

	diffSig := diagnostics.ComputeSignature(diagnostics.CategoryExecutionFailure, 1, "other err", constraints1, actions)
	if sig1 == diffSig {
		t.Error("distinct error messages should produce distinct signatures")
	}
}
