package community

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestNodeDrain_StaticFloorInvariant verifies zero-value defaults, configuration validation,
// and state immutability baseline.
func TestNodeDrain_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	ctrl, err := NewNodeDrainController(NodeDrainConfig{})
	require.NoError(t, err)
	require.NotNil(t, ctrl)
	assert.Equal(t, 30*time.Second, ctrl.cfg.DefaultDrainTimeout)

	customTimeout := 10 * time.Second
	ctrlCustom, err := NewNodeDrainController(NodeDrainConfig{
		DefaultDrainTimeout: customTimeout,
	})
	require.NoError(t, err)
	assert.Equal(t, customTimeout, ctrlCustom.cfg.DefaultDrainTimeout)
}

// TestNodeDrain_OperationalDynamicProof verifies concurrent task allocation, graceful draining
// with zero dropped tasks, task completion signaling, and auto-recovery.
func TestNodeDrain_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	ctrl, err := NewNodeDrainController(NodeDrainConfig{
		DefaultDrainTimeout: 2 * time.Second,
	})
	require.NoError(t, err)

	nodeID := "node-dynamic-001"
	require.NoError(t, ctrl.RegisterNode(nodeID))

	status, err := ctrl.GetNodeStatus(nodeID)
	require.NoError(t, err)
	assert.Equal(t, NodeDrainStatusActive, status)

	// Allocate 3 tasks
	require.NoError(t, ctrl.AllocateTask(nodeID, "task-1"))
	require.NoError(t, ctrl.AllocateTask(nodeID, "task-2"))
	require.NoError(t, ctrl.AllocateTask(nodeID, "task-3"))

	count, err := ctrl.ActiveTaskCount(nodeID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Cordon node
	require.NoError(t, ctrl.CordonNode(nodeID, "routine rolling maintenance"))
	status, err = ctrl.GetNodeStatus(nodeID)
	require.NoError(t, err)
	assert.Equal(t, NodeDrainStatusCordoned, status)

	// Inflight drain barrier in goroutine
	drainErrCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_drain_worker", "verify graceful task completion during drain").
		StartSimple(func() {
			drainErrCh <- ctrl.DrainNode(context.Background(), nodeID, 3*time.Second)
		})

	// Give worker time to acquire drain lock
	time.Sleep(20 * time.Millisecond)

	// Check status is draining
	status, err = ctrl.GetNodeStatus(nodeID)
	require.NoError(t, err)
	assert.Equal(t, NodeDrainStatusDraining, status)

	// Complete tasks one by one
	require.NoError(t, ctrl.CompleteTask(nodeID, "task-1"))
	require.NoError(t, ctrl.CompleteTask(nodeID, "task-2"))
	require.NoError(t, ctrl.CompleteTask(nodeID, "task-3"))

	// Wait for drain barrier to return
	select {
	case drainErr := <-drainErrCh:
		require.NoError(t, drainErr)
	case <-time.After(2 * time.Second):
		t.Fatal("drain timed out unexpectedly")
	}

	// Verify node status is drained
	status, err = ctrl.GetNodeStatus(nodeID)
	require.NoError(t, err)
	assert.Equal(t, NodeDrainStatusDrained, status)

	count, err = ctrl.ActiveTaskCount(nodeID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Auto-recover node
	require.NoError(t, ctrl.RecoverNode(nodeID))
	status, err = ctrl.GetNodeStatus(nodeID)
	require.NoError(t, err)
	assert.Equal(t, NodeDrainStatusActive, status)

	// Node can accept tasks again
	require.NoError(t, ctrl.AllocateTask(nodeID, "task-post-recover"))
	count, err = ctrl.ActiveTaskCount(nodeID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// TestNodeDrain_NegativeAdversarialBoundary validates error handling, rejection of dispatch
// to cordoned/draining nodes, timeout expiry, and blank input rejections.
func TestNodeDrain_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	ctrl, err := NewNodeDrainController(NodeDrainConfig{
		DefaultDrainTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	// Blank input validation
	assert.ErrorIs(t, ctrl.RegisterNode(""), ErrBlankDrainNodeID)
	assert.ErrorIs(t, ctrl.RegisterNode("   "), ErrBlankDrainNodeID)

	_, err = ctrl.GetNodeStatus("")
	assert.ErrorIs(t, err, ErrBlankDrainNodeID)

	assert.ErrorIs(t, ctrl.CordonNode("", "maintenance"), ErrBlankDrainNodeID)
	assert.ErrorIs(t, ctrl.CordonNode("node-1", ""), ErrBlankDrainReason)
	assert.ErrorIs(t, ctrl.CordonNode("node-1", "   "), ErrBlankDrainReason)

	assert.ErrorIs(t, ctrl.AllocateTask("", "task-1"), ErrBlankDrainNodeID)
	assert.ErrorIs(t, ctrl.AllocateTask("node-1", ""), ErrBlankTaskID)
	assert.ErrorIs(t, ctrl.CompleteTask("", "task-1"), ErrBlankDrainNodeID)
	assert.ErrorIs(t, ctrl.CompleteTask("node-1", ""), ErrBlankTaskID)
	assert.ErrorIs(t, ctrl.RecoverNode(""), ErrBlankDrainNodeID)
	assert.ErrorIs(t, ctrl.DrainNode(context.Background(), "", 100*time.Millisecond), ErrBlankDrainNodeID)

	// Unregistered node operations
	assert.ErrorIs(t, ctrl.CordonNode("unknown", "maint"), ErrNodeNotRegistered)
	assert.ErrorIs(t, ctrl.AllocateTask("unknown", "task"), ErrNodeNotRegistered)
	assert.ErrorIs(t, ctrl.CompleteTask("unknown", "task"), ErrNodeNotRegistered)
	assert.ErrorIs(t, ctrl.RecoverNode("unknown"), ErrNodeNotRegistered)
	assert.ErrorIs(t, ctrl.DrainNode(context.Background(), "unknown", 100*time.Millisecond), ErrNodeNotRegistered)

	// Duplicate registration
	nodeID := "node-neg-001"
	require.NoError(t, ctrl.RegisterNode(nodeID))
	assert.ErrorIs(t, ctrl.RegisterNode(nodeID), ErrNodeAlreadyRegistered)

	// Cordoned node disallows new task dispatches
	require.NoError(t, ctrl.CordonNode(nodeID, "quarantined"))
	assert.ErrorIs(t, ctrl.AllocateTask(nodeID, "new-task"), ErrNodeUnavailable)

	// Drain timeout on stuck task
	require.NoError(t, ctrl.RecoverNode(nodeID))
	require.NoError(t, ctrl.AllocateTask(nodeID, "stuck-task"))

	// Draining with stuck task should time out
	ctx := context.Background()
	drainErr := ctrl.DrainNode(ctx, nodeID, 50*time.Millisecond)
	assert.ErrorIs(t, drainErr, ErrNodeDrainTimeout)

	// Cannot recover node while draining
	assert.ErrorIs(t, ctrl.RecoverNode(nodeID), ErrInvalidTransition)

	// Completing missing task
	assert.ErrorIs(t, ctrl.CompleteTask(nodeID, "nonexistent-task"), ErrTaskNotFound)

	// Complete stuck task to allow drain to finish
	require.NoError(t, ctrl.CompleteTask(nodeID, "stuck-task"))
	status, err := ctrl.GetNodeStatus(nodeID)
	require.NoError(t, err)
	assert.Equal(t, NodeDrainStatusDrained, status)

	// Now recovery succeeds
	require.NoError(t, ctrl.RecoverNode(nodeID))
	require.NoError(t, ctrl.AllocateTask(nodeID, "task-dup"))
	assert.ErrorIs(t, ctrl.AllocateTask(nodeID, "task-dup"), ErrTaskAlreadyAllocated)
}
