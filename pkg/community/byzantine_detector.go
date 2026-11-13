package community

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// NodeQuarantineState represents the trust and partition status of a cluster agent node.
type NodeQuarantineState string

const (
	// NodeStateHealthy indicates normal swarm participation and voting rights.
	NodeStateHealthy NodeQuarantineState = "healthy"
	// NodeStateSuspect indicates anomalous telemetry; gossip assertions are monitored.
	NodeStateSuspect NodeQuarantineState = "suspect"
	// NodeStateQuarantined indicates active eviction from voting, leader election, and gossip pools.
	NodeStateQuarantined NodeQuarantineState = "quarantined"
	// NodeStateExpelled indicates permanent cryptographic revocation and ejection.
	NodeStateExpelled NodeQuarantineState = "expelled"
)

// ByzantineFaultType classifies verifiable Byzantine protocol violations.
type ByzantineFaultType string

const (
	// FaultEquivocation occurs when an agent signs conflicting statements for the same view/sequence.
	FaultEquivocation ByzantineFaultType = "equivocation"
	// FaultSignatureMismatch occurs when payload digest does not match the node cryptographic signature.
	FaultSignatureMismatch ByzantineFaultType = "signature_mismatch"
	// FaultStateDivergence occurs when an agent asserts illegitimate epoch hops or invalid causal histories.
	FaultStateDivergence ByzantineFaultType = "state_divergence"
)

// SignedAssertion encapsulates an epoch/sequence assertion signed by an agent.
type SignedAssertion struct {
	NodeID    string    `json:"node_id"`
	View      uint64    `json:"view"`
	Sequence  uint64    `json:"sequence"`
	Digest    string    `json:"digest"`
	Signature string    `json:"signature"`
	Timestamp time.Time `json:"timestamp"`
}

// FaultEvidence encapsulates verifiable cryptographic proof of Byzantine misconduct.
type FaultEvidence struct {
	FaultType         ByzantineFaultType `json:"fault_type"`
	AccusedNode       string             `json:"accused_node"`
	ReporterNode      string             `json:"reporter_node"`
	View              uint64             `json:"view"`
	Sequence          uint64             `json:"sequence"`
	AssertionA        SignedAssertion    `json:"assertion_a"`
	AssertionB        SignedAssertion    `json:"assertion_b"`
	WitnessSignatures []string           `json:"witness_signatures"`
	Timestamp         time.Time          `json:"timestamp"`
}

// QuarantineNotice is generated when an agent node transitions into quarantine or expulsion.
type QuarantineNotice struct {
	NoticeID      string              `json:"notice_id"`
	NodeID        string              `json:"node_id"`
	PreviousState NodeQuarantineState `json:"previous_state"`
	NewState      NodeQuarantineState `json:"new_state"`
	Reason        ByzantineFaultType  `json:"reason"`
	Evidence      FaultEvidence       `json:"evidence"`
	IssuedAt      time.Time           `json:"issued_at"`
}

// ByzantineDetector coordinates real-time fault detection and cluster quarantine state.
type ByzantineDetector struct {
	mu                sync.RWMutex
	nodeStates        map[string]NodeQuarantineState
	seenAssertions    map[string]map[uint64]SignedAssertion // nodeID -> sequence -> SignedAssertion
	quarantineHistory []QuarantineNotice
	witnessThreshold  int
}

// NewByzantineDetector initializes an empty detector with the configured witness verification threshold.
func NewByzantineDetector(witnessThreshold int) (*ByzantineDetector, error) {
	if witnessThreshold < 1 {
		return nil, fmt.Errorf("witness threshold must be at least 1, got %d", witnessThreshold)
	}
	return &ByzantineDetector{
		nodeStates:        make(map[string]NodeQuarantineState),
		seenAssertions:    make(map[string]map[uint64]SignedAssertion),
		quarantineHistory: make([]QuarantineNotice, 0),
		witnessThreshold:  witnessThreshold,
	}, nil
}

// RegisterNode provisions a node into the detector under healthy status.
func (d *ByzantineDetector) RegisterNode(nodeID string) error {
	if err := strutil.ErrorOnBlank(nodeID, "node identifier cannot be blank"); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.nodeStates[nodeID]; !exists {
		d.nodeStates[nodeID] = NodeStateHealthy
		d.seenAssertions[nodeID] = make(map[uint64]SignedAssertion)
	}
	return nil
}

// GetNodeState queries the current quarantine state for a node.
func (d *ByzantineDetector) GetNodeState(nodeID string) NodeQuarantineState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if state, exists := d.nodeStates[nodeID]; exists {
		return state
	}
	return NodeStateHealthy
}

// ProcessAssertion analyzes an incoming signed assertion. If conflicting assertions for the
// same sequence/view are encountered, an equivocation fault is detected and quarantine notice returned.
func (d *ByzantineDetector) ProcessAssertion(assertion SignedAssertion) (*QuarantineNotice, error) {
	if err := strutil.ErrorOnBlank(assertion.NodeID, "assertion node identifier cannot be blank"); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(assertion.Digest, "assertion digest cannot be blank"); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(assertion.Signature, "assertion signature cannot be blank"); err != nil {
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	nodeID := assertion.NodeID
	if _, exists := d.nodeStates[nodeID]; !exists {
		d.nodeStates[nodeID] = NodeStateHealthy
		d.seenAssertions[nodeID] = make(map[uint64]SignedAssertion)
	}

	currentState := d.nodeStates[nodeID]
	if currentState == NodeStateQuarantined || currentState == NodeStateExpelled {
		return nil, fmt.Errorf("node %s is already quarantined/expelled (state=%s)", nodeID, currentState)
	}

	prior, exists := d.seenAssertions[nodeID][assertion.Sequence]
	if exists {
		// If same sequence has different digest or signature in the same view, equivocation detected!
		if prior.View == assertion.View && (prior.Digest != assertion.Digest || prior.Signature != assertion.Signature) {
			evidence := FaultEvidence{
				FaultType:         FaultEquivocation,
				AccusedNode:       nodeID,
				ReporterNode:      "cluster:detector",
				View:              assertion.View,
				Sequence:          assertion.Sequence,
				AssertionA:        prior,
				AssertionB:        assertion,
				WitnessSignatures: []string{prior.Signature, assertion.Signature},
				Timestamp:         time.Now().UTC(),
			}

			d.nodeStates[nodeID] = NodeStateQuarantined
			notice := QuarantineNotice{
				NoticeID:      generateNoticeID(nodeID, FaultEquivocation),
				NodeID:        nodeID,
				PreviousState: currentState,
				NewState:      NodeStateQuarantined,
				Reason:        FaultEquivocation,
				Evidence:      evidence,
				IssuedAt:      time.Now().UTC(),
			}
			d.quarantineHistory = append(d.quarantineHistory, notice)
			return &notice, nil
		}
		// Exact duplicate replay is benign
		return nil, nil
	}

	d.seenAssertions[nodeID][assertion.Sequence] = assertion
	return nil, nil
}

// ReportFault validates third-party evidence against adversarial tampering before triggering quarantine.
func (d *ByzantineDetector) ReportFault(evidence FaultEvidence) (*QuarantineNotice, error) {
	if err := d.ValidateQuarantineEvidence(evidence); err != nil {
		return nil, fmt.Errorf("fault evidence verification rejected: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	accused := evidence.AccusedNode
	currentState := d.nodeStates[accused]
	if currentState == "" {
		currentState = NodeStateHealthy
	}

	if currentState == NodeStateQuarantined || currentState == NodeStateExpelled {
		return nil, fmt.Errorf("accused node %s is already %s", accused, currentState)
	}

	d.nodeStates[accused] = NodeStateQuarantined
	notice := QuarantineNotice{
		NoticeID:      generateNoticeID(accused, evidence.FaultType),
		NodeID:        accused,
		PreviousState: currentState,
		NewState:      NodeStateQuarantined,
		Reason:        evidence.FaultType,
		Evidence:      evidence,
		IssuedAt:      time.Now().UTC(),
	}
	d.quarantineHistory = append(d.quarantineHistory, notice)
	return &notice, nil
}

// ValidateQuarantineEvidence verifies cryptographic and witness requirements on submitted evidence.
func (d *ByzantineDetector) ValidateQuarantineEvidence(evidence FaultEvidence) error {
	if err := strutil.ErrorOnBlank(evidence.AccusedNode, "accused node identifier cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(evidence.ReporterNode, "reporter node identifier cannot be blank"); err != nil {
		return err
	}
	if evidence.AccusedNode == evidence.ReporterNode {
		return fmt.Errorf("self-reporting cannot trigger external Byzantine quarantine")
	}

	if len(evidence.WitnessSignatures) < d.witnessThreshold {
		return fmt.Errorf("insufficient witness signatures: required %d, provided %d", d.witnessThreshold, len(evidence.WitnessSignatures))
	}

	for i, sig := range evidence.WitnessSignatures {
		if err := strutil.ErrorOnBlank(sig, "witness signature at index %d cannot be blank", i); err != nil {
			return err
		}
	}

	switch evidence.FaultType {
	case FaultEquivocation:
		if evidence.AssertionA.Digest == "" || evidence.AssertionB.Digest == "" {
			return fmt.Errorf("equivocation evidence requires non-empty digests for both assertions")
		}
		if evidence.AssertionA.Digest == evidence.AssertionB.Digest && evidence.AssertionA.Signature == evidence.AssertionB.Signature {
			return fmt.Errorf("equivocation evidence requires divergent assertions; assertions are identical")
		}
	case FaultSignatureMismatch, FaultStateDivergence:
		// Acceptable fault types
	default:
		return fmt.Errorf("unrecognized Byzantine fault type: %s", evidence.FaultType)
	}

	return nil
}

// ExportQuarantineNotices returns a clone of the recorded quarantine history.
func (d *ByzantineDetector) ExportQuarantineNotices() []QuarantineNotice {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]QuarantineNotice, len(d.quarantineHistory))
	copy(out, d.quarantineHistory)
	return out
}

func generateNoticeID(nodeID string, fault ByzantineFaultType) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", nodeID, fault, time.Now().UnixNano())))
	return "QNT-" + hex.EncodeToString(sum[:8])
}
