package orchestration

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/hive/capability"
	"github.com/lanceman/zqk/pkg/hivemind"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
)

// RawIntent represents the initial signal for a potential capability.
type RawIntent struct {
	Signature string
	Payload   map[string]any
	Priority  int
}

// CapabilityRef identifies the resulting synthesized capability.
type CapabilityRef struct {
	ID   string
	Kind string
}

// OrchestrationManager defines the orchestration logic for capability synthesis.
type OrchestrationManager interface {
	ProcessIntent(ctx context.Context, intent RawIntent) (CapabilityRef, error)
}

// Manager is the concrete implementation of OrchestrationManager.
type Manager struct {
	negotiator scheduler.PolicyNegotiator
	memory     hivemind.MemoryStore
	storage    storage.ObjectStorageProvider
}

// NewManager initializes a new OrchestrationManager.
func NewManager(n scheduler.PolicyNegotiator, m hivemind.MemoryStore, s storage.ObjectStorageProvider) *Manager {
	return &Manager{
		negotiator: n,
		memory:     m,
		storage:    s,
	}
}

type dynamicCapability struct {
	id    string
	title string
}

func (d *dynamicCapability) Name() string        { return d.id }
func (d *dynamicCapability) Description() string { return d.title }
func (d *dynamicCapability) EvaluatePolicy(ctx context.Context, evalCtx capability.EvalContext) (bool, error) {
	return true, nil
}

// ProcessIntent synthesizes a new capability based on input intent.
func (m *Manager) ProcessIntent(ctx context.Context, intent RawIntent) (ref CapabilityRef, err error) {
	// 1. Policy Validation
	// Wrap intent as Proposal.
	prop := scheduler.Proposal{
		AgentID:     "orchestrator",
		TargetJobID: intent.Signature,
		Type:        scheduler.ProposalOverride, // Use a placeholder type
		Metadata:    map[string]any{"raw": intent},
	}
	res, err := m.negotiator.Propose(ctx, prop)
	if err != nil {
		return CapabilityRef{}, errfmt.Newf(ErrMsgPolicyNegotiationFailed).Wrap(err)
	}
	if !res.Accepted {
		return CapabilityRef{}, fmt.Errorf(ErrMsgPolicyRejectedIntent, res.Reason)
	}

	// 2. Semantic Context Retrieval
	_, err = m.memory.RetrieveSemantically(ctx, intent.Signature, 5)
	if err != nil {
		return CapabilityRef{}, errfmt.Newf(ErrMsgMemoryRetrievalFailed).Wrap(err)
	}

	// 3. Synthesis
	var testCaseRefs []string
	if tc, ok := intent.Payload["test_cases"].([]string); ok {
		testCaseRefs = tc
	} else if tc, ok := intent.Payload["test_cases"].([]any); ok {
		for _, t := range tc {
			if ts, ok := t.(string); ok {
				testCaseRefs = append(testCaseRefs, ts)
			}
		}
	} else if tc, ok := intent.Payload["test_case"].(string); ok {
		testCaseRefs = append(testCaseRefs, tc)
	}

	capID := fmt.Sprintf("CAP-%s", strings.ToUpper(intent.Signature))
	capTitle := fmt.Sprintf(LogFmtSynthesized, intent.Signature)

	dynCap := &dynamicCapability{
		id:    capID,
		title: capTitle,
	}

	pipeline := capability.NewSynthesisPipeline(capability.NewRegistry())
	if err := pipeline.Synthesize(dynCap, testCaseRefs); err != nil {
		return CapabilityRef{}, errfmt.Newf("capability synthesis validation failed").Wrap(err)
	}

	newCap := map[string]any{
		objects.FieldKeyID:     capID,
		objects.FieldKeyKind:   "capability",
		objects.FieldKeyTitle:  capTitle,
		objects.FieldKeyStatus: "proposed",
	}

	// 4. Commitment
	secCtx := pkgctx.NewSystemSecurityContext()
	err = m.storage.Create(ctx, secCtx, newCap)
	if err != nil {
		return CapabilityRef{}, errfmt.Newf(ErrMsgCommitmentFailed).Wrap(err)
	}

	return CapabilityRef{
		ID:   newCap[objects.FieldKeyID].(string),
		Kind: newCap[objects.FieldKeyKind].(string),
	}, nil
}
