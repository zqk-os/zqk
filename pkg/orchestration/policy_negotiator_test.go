package orchestration

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/scheduler"
)

func TestPolicyNegotiator_Propose_Reconciliation(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	negotiator := NewPolicyNegotiator(logger)

	p := scheduler.Proposal{
		Metadata: map[string]any{
			"raw": map[string]any{
				"version_context": "v1",
			},
		},
	}

	result, err := negotiator.Propose(context.Background(), p)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Accepted {
		t.Errorf("expected accepted, got false")
	}
	if result.Reason != "schema reconciled" {
		t.Errorf("expected reason 'schema reconciled', got '%s'", result.Reason)
	}
}

func TestPolicyNegotiator_Propose_Standard(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	negotiator := NewPolicyNegotiator(logger)

	p := scheduler.Proposal{
		Metadata: map[string]any{
			"raw": map[string]any{
				"version_context": "current",
			},
		},
	}

	result, err := negotiator.Propose(context.Background(), p)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Accepted {
		t.Errorf("expected accepted, got false")
	}
	if result.Reason != "standard proposal accepted" {
		t.Errorf("expected reason 'standard proposal accepted', got '%s'", result.Reason)
	}
}
