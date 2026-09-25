package kernelcas_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/kernelcas"
)

// FuzzKernelCASMutation fuzzes the KernelCAS mutation pipeline stages
// to assert that arbitrary random inputs never panic or trigger unhandled faults.
func FuzzKernelCASMutation(f *testing.F) {
	// Seed corpus with representative mutation inputs
	seeds := []struct {
		kind       string
		id         string
		intent     string
		status     string
		reason     string
		cascade    bool
		unlinkRefs bool
	}{
		{"criteria", "CRIT-001", kernelcas.IntentReconcileIndex, "planned", "initial sync", false, false},
		{"requirement", "REQ-100", kernelcas.IntentEraseLogical, "complete", "deprecated", true, true},
		{"milestone", "MIL-ABC", "custom_intent", "originated", "", false, true},
		{"", "", "", "", "", false, false},
		{"unknown_kind", "ID-999", "invalid", "unknown_status", "fuzz", true, false},
	}

	for _, s := range seeds {
		f.Add(s.kind, s.id, s.intent, s.status, s.reason, s.cascade, s.unlinkRefs)
	}

	f.Fuzz(func(t *testing.T, kind, id, intent, status, reason string, cascade, unlinkRefs bool) {
		m := &kernelcas.Mutation{
			Kind:       kind,
			ID:         id,
			Intent:     intent,
			Status:     status,
			Reason:     reason,
			Cascade:    cascade,
			UnlinkRefs: unlinkRefs,
			CommitFn: func(ctx context.Context) error {
				return nil
			},
		}

		// Execute against reconcile index pipeline
		_ = kernelcas.RunReconcileIndex(context.Background(), nil, m)

		// Execute against erase pipeline
		_ = kernelcas.RunErase(context.Background(), nil, m)
	})
}
