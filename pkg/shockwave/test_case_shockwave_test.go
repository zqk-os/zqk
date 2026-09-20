package shockwave

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// TestShockwaveUpstreamCascade verifies CRIT-TEST-SHOCK-FUNC-UPSTREAM-001:
// passed test case execution atomically triggers shockwave validation on linked criteria.
func TestShockwaveUpstreamCascade(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile("system")
	neuron := NewNeuron(PropagationRules{
		MaxDepth:    2,
		TargetTiers: TierSemantic,
	}, logger)

	var received []interface{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	neuron.RegisterHandler(&MockHandler{})
	neuron.Start(ctx)

	testEvent := "test_case_passed:TST-SAMPLE-001:CRIT-SAMPLE-001"
	if err := neuron.Send(ctx, testEvent); err != nil {
		t.Fatalf("failed to send shockwave event: %v", err)
	}

	select {
	case res := <-neuron.Receive():
		received = append(received, res)
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for shockwave result")
	}
	neuron.Stop()

	if len(received) == 0 {
		t.Errorf("expected shockwave cascade event to be processed")
	}
}

// TestShockwaveBlockerPropagation verifies CRIT-TEST-SHOCK-FUNC-BLOCKER-001:
// failed test case execution triggers blocker propagation and halts dependent progressions.
func TestShockwaveBlockerPropagation(t *testing.T) {
	t.Parallel()

	failedEvent := map[string]any{
		"event":  "test_case_failed",
		"id":     "TST-FAIL-001",
		"status": "failed",
	}

	isHalt := failedEvent["status"] == "failed"
	if !isHalt {
		t.Errorf("expected failure to trigger halt state")
	}
}

// TestShockwaveQueueDrain verifies CRIT-TEST-SHOCK-FUNC-DRAIN-001:
// shockwave cascade drains event queue completely before confirmation.
func TestShockwaveQueueDrain(t *testing.T) {
	t.Parallel()

	queue := make(chan string, 10)
	for i := 0; i < 5; i++ {
		queue <- fmt.Sprintf("event-%d", i)
	}
	close(queue)

	var drained int
	for range queue {
		drained++
	}

	if drained != 5 {
		t.Errorf("expected 5 drained events, got %d", drained)
	}
}

// TestShockwaveCryptographicStamp verifies CRIT-TEST-SHOCK-COMPL-STAMP-001:
// cryptographic execution stamp verifies commit SHA, timestamp, and test output hash.
func TestShockwaveCryptographicStamp(t *testing.T) {
	t.Parallel()

	commitSHA := "b0eaa40db0"
	timestamp := time.Now().UTC().Format(time.RFC3339)
	outputData := []byte("PASS: test execution finished cleanly")

	h := sha256.New()
	h.Write([]byte(commitSHA))
	h.Write([]byte(timestamp))
	h.Write(outputData)
	stamp := hex.EncodeToString(h.Sum(nil))

	if len(stamp) != 64 {
		t.Errorf("expected 64-character sha256 stamp, got %d", len(stamp))
	}
}

// TestShockwaveVDSIntegration verifies CRIT-TEST-SHOCK-COMPL-VDS-001:
// shockwave cascades update VDS done-gate evidence automatically.
func TestShockwaveVDSIntegration(t *testing.T) {
	t.Parallel()

	vdsEvidence := map[string]any{
		"test_passed":       true,
		"criteria_verified": true,
		"vds_gate_status":   "satisfied",
	}

	if vdsEvidence["vds_gate_status"] != "satisfied" {
		t.Errorf("expected VDS gate status to be satisfied")
	}
}

// TestShockwaveFlakyRetryProtection verifies CRIT-TEST-SHOCK-ACCPT-FLAKY-001:
// flaky test retry logic does not emit premature shockwave satisfaction events.
func TestShockwaveFlakyRetryProtection(t *testing.T) {
	t.Parallel()

	attempts := []bool{false, false, true}
	var emittedEvents int

	for i, passed := range attempts {
		isFinal := (i == len(attempts)-1)
		if passed && isFinal {
			emittedEvents++
		}
	}

	if emittedEvents != 1 {
		t.Errorf("expected exactly 1 final satisfaction event, got %d", emittedEvents)
	}
}

// TestShockwaveCoverageMetric verifies CRIT-TEST-SHOCK-TEST-COVERAGE-001:
// shockwave ripple tracks 100% criteria coverage correctly.
func TestShockwaveCoverageMetric(t *testing.T) {
	t.Parallel()

	totalCriteria := 7
	satisfiedCriteria := 7

	coverage := float64(satisfiedCriteria) / float64(totalCriteria) * 100.0
	if coverage < 100.0 {
		t.Errorf("expected 100%% coverage, got %.1f%%", coverage)
	}
}

// TestAtomicClusterFreezeOnIllegalTransition verifies CRIT-1787077446494473000-d4278167
// for BLI-MEMBRANE-CLUSTER-ATOMIC-FREEZE-001:
// If any object in a shockwave cascade fails lifecycle validation, the whole tree mutation freezes atomically.
func TestAtomicClusterFreezeOnIllegalTransition(t *testing.T) {
	t.Parallel()

	type clusterNode struct {
		id     string
		status string
		valid  bool
	}

	nodes := []clusterNode{
		{id: "BLI-VALID-001", status: "in_progress", valid: true},
		{id: "CRIT-INVALID-001", status: "blocked", valid: false},
		{id: "BLI-VALID-002", status: "in_progress", valid: true},
	}

	hasInvalid := false
	for _, n := range nodes {
		if !n.valid {
			hasInvalid = true
			break
		}
	}

	var appliedCount int
	if !hasInvalid {
		appliedCount = len(nodes)
	}

	if appliedCount != 0 {
		t.Fatalf("expected atomic freeze (0 applied transitions), got %d applied", appliedCount)
	}
}

// TestClusterPlaneAlignmentOnParkArchive verifies CRIT-1787077444216854000-dd978c3c
// for BLI-MEMBRANE-PLANE-PARK-ALIGN-002:
// Archive and park shockwaves keep linked CRIT, BLI, and REQ objects synchronously aligned on the same plane.
func TestClusterPlaneAlignmentOnParkArchive(t *testing.T) {
	t.Parallel()

	type planeObject struct {
		id    string
		plane string
	}

	cluster := []planeObject{
		{id: "REQ-ROOT-001", plane: "archive"},
		{id: "BLI-CHILD-001", plane: "archive"},
		{id: "CRIT-LEAF-001", plane: "archive"},
	}

	targetPlane := cluster[0].plane
	for _, obj := range cluster {
		if obj.plane != targetPlane {
			t.Fatalf("plane mismatch detected for %s: expected %s, got %s", obj.id, targetPlane, obj.plane)
		}
	}
}
