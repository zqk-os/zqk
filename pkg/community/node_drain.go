package community

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	errDrainBlankNodeIDMsg = "node ID cannot be blank"
	errDrainBlankTaskIDMsg = "task ID cannot be blank"
	errDrainBlankReasonMsg = "drain reason cannot be blank"
)

var (
	// ErrBlankDrainNodeID indicates that the provided node identifier is blank.
	ErrBlankDrainNodeID = errors.New(errDrainBlankNodeIDMsg)

	// ErrBlankDrainReason indicates that the cordon/drain reason is blank.
	ErrBlankDrainReason = errors.New(errDrainBlankReasonMsg)

	// ErrBlankTaskID indicates that the provided task identifier is blank.
	ErrBlankTaskID = errors.New(errDrainBlankTaskIDMsg)

	// ErrNodeAlreadyRegistered indicates that the node is already known to the controller.
	ErrNodeAlreadyRegistered = errors.New("node already registered in controller")

	// ErrNodeNotRegistered indicates that the target node is not registered.
	ErrNodeNotRegistered = errors.New("node not registered in controller")

	// ErrInvalidTransition indicates an illegal lifecycle transition attempt.
	ErrInvalidTransition = errors.New("invalid node lifecycle state transition")

	// ErrNodeUnavailable indicates the node is not accepting new tasks (cordoned or draining).
	ErrNodeUnavailable = errors.New("node is not accepting tasks in current state")

	// ErrTaskAlreadyAllocated indicates the task ID is already in flight on the node.
	ErrTaskAlreadyAllocated = errors.New("task is already allocated on node")

	// ErrTaskNotFound indicates the task ID is not tracked on the target node.
	ErrTaskNotFound = errors.New("task not found on node")

	// ErrNodeDrainTimeout indicates that the node did not complete in-flight tasks before timeout.
	ErrNodeDrainTimeout = errors.New("node drain operation timed out")
)

// NodeDrainStatus represents the operational lifecycle state of an agent node.
type NodeDrainStatus string

const (
	// NodeDrainStatusActive represents a normal healthy node accepting tasks.
	NodeDrainStatusActive NodeDrainStatus = "active"

	// NodeDrainStatusCordoned prevents new task dispatch while allowing running tasks to finish.
	NodeDrainStatusCordoned NodeDrainStatus = "cordoned"

	// NodeDrainStatusDraining indicates active eviction of in-flight tasks.
	NodeDrainStatusDraining NodeDrainStatus = "draining"

	// NodeDrainStatusDrained indicates all in-flight tasks have completed.
	NodeDrainStatusDrained NodeDrainStatus = "drained"

	// NodeDrainStatusRecovered indicates a node restored to active service.
	NodeDrainStatusRecovered NodeDrainStatus = "recovered"
)

type managedNode struct {
	id          string
	status      NodeDrainStatus
	cordonCause string
	tasks       map[string]time.Time
	drainedChan chan struct{}
}

// NodeDrainConfig supplies tuning parameters for the drain controller.
type NodeDrainConfig struct {
	DefaultDrainTimeout time.Duration
}

// NodeDrainController manages the graceful drain and recovery lifecycles of agent nodes.
type NodeDrainController struct {
	mu    sync.RWMutex
	cfg   NodeDrainConfig
	nodes map[string]*managedNode
}

// NewNodeDrainController creates a new node drain supervisor.
func NewNodeDrainController(cfg NodeDrainConfig) (*NodeDrainController, error) {
	if cfg.DefaultDrainTimeout <= 0 {
		cfg.DefaultDrainTimeout = 30 * time.Second
	}
	return &NodeDrainController{
		cfg:   cfg,
		nodes: make(map[string]*managedNode),
	}, nil
}

func (c *NodeDrainController) executeOnNode(nodeID string, write bool, fn func(*managedNode) error) error {
	if err := strutil.ErrorOnBlank(nodeID, errDrainBlankNodeIDMsg); err != nil {
		return ErrBlankDrainNodeID
	}
	var unlock func()
	if write {
		c.mu.Lock()
		unlock = c.mu.Unlock
	} else {
		c.mu.RLock()
		unlock = c.mu.RUnlock
	}
	defer unlock()

	node, exists := c.nodes[nodeID]
	if !exists {
		return ErrNodeNotRegistered
	}
	return fn(node)
}

// RegisterNode adds a new active node to the controller.
func (c *NodeDrainController) RegisterNode(nodeID string) error {
	if err := strutil.ErrorOnBlank(nodeID, errDrainBlankNodeIDMsg); err != nil {
		return ErrBlankDrainNodeID
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.nodes[nodeID]; exists {
		return ErrNodeAlreadyRegistered
	}

	c.nodes[nodeID] = &managedNode{
		id:          nodeID,
		status:      NodeDrainStatusActive,
		tasks:       make(map[string]time.Time),
		drainedChan: make(chan struct{}),
	}
	return nil
}

// GetNodeStatus returns the current lifecycle status of the specified node.
func (c *NodeDrainController) GetNodeStatus(nodeID string) (NodeDrainStatus, error) {
	var currentStatus NodeDrainStatus
	err := c.executeOnNode(nodeID, false, func(n *managedNode) error {
		currentStatus = n.status
		return nil
	})
	return currentStatus, err
}

// CordonNode marks a node cordoned, preventing new task dispatches while leaving existing tasks intact.
func (c *NodeDrainController) CordonNode(nodeID string, reason string) error {
	if err := strutil.ErrorOnBlank(reason, errDrainBlankReasonMsg); err != nil {
		return ErrBlankDrainReason
	}
	return c.executeOnNode(nodeID, true, func(n *managedNode) error {
		if n.status == NodeDrainStatusDraining || n.status == NodeDrainStatusDrained {
			return ErrInvalidTransition
		}
		n.status = NodeDrainStatusCordoned
		n.cordonCause = reason
		return nil
	})
}

// AllocateTask assigns an active task to the node if it is in an accepting state.
func (c *NodeDrainController) AllocateTask(nodeID string, taskID string) error {
	if err := strutil.ErrorOnBlank(taskID, errDrainBlankTaskIDMsg); err != nil {
		return ErrBlankTaskID
	}
	return c.executeOnNode(nodeID, true, func(n *managedNode) error {
		if n.status != NodeDrainStatusActive && n.status != NodeDrainStatusRecovered {
			return ErrNodeUnavailable
		}
		if _, exists := n.tasks[taskID]; exists {
			return ErrTaskAlreadyAllocated
		}
		n.tasks[taskID] = time.Now()
		return nil
	})
}

// CompleteTask marks an allocated task as finished, notifying any waiting drain barrier.
func (c *NodeDrainController) CompleteTask(nodeID string, taskID string) error {
	if err := strutil.ErrorOnBlank(taskID, errDrainBlankTaskIDMsg); err != nil {
		return ErrBlankTaskID
	}
	return c.executeOnNode(nodeID, true, func(n *managedNode) error {
		if _, exists := n.tasks[taskID]; !exists {
			return ErrTaskNotFound
		}
		delete(n.tasks, taskID)
		if len(n.tasks) == 0 && n.status == NodeDrainStatusDraining {
			n.status = NodeDrainStatusDrained
			select {
			case <-n.drainedChan:
			default:
				close(n.drainedChan)
			}
		}
		return nil
	})
}

// ActiveTaskCount returns the number of in-flight tasks running on the node.
func (c *NodeDrainController) ActiveTaskCount(nodeID string) (int, error) {
	var activeCount int
	err := c.executeOnNode(nodeID, false, func(n *managedNode) error {
		activeCount = len(n.tasks)
		return nil
	})
	return activeCount, err
}

// DrainNode initiates draining and blocks until all in-flight tasks complete or timeout expires.
func (c *NodeDrainController) DrainNode(ctx context.Context, nodeID string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = c.cfg.DefaultDrainTimeout
	}

	var drainedCh chan struct{}
	err := c.executeOnNode(nodeID, true, func(n *managedNode) error {
		if n.status == NodeDrainStatusDrained {
			return nil
		}
		n.status = NodeDrainStatusDraining
		if len(n.tasks) == 0 {
			n.status = NodeDrainStatusDrained
			select {
			case <-n.drainedChan:
			default:
				close(n.drainedChan)
			}
			return nil
		}
		drainedCh = n.drainedChan
		return nil
	})
	if err != nil {
		return err
	}
	if drainedCh == nil {
		return nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-drainedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrNodeDrainTimeout
	}
}

// RecoverNode resets a cordoned or drained node back to active status.
func (c *NodeDrainController) RecoverNode(nodeID string) error {
	return c.executeOnNode(nodeID, true, func(n *managedNode) error {
		if n.status == NodeDrainStatusDraining {
			return ErrInvalidTransition
		}
		n.status = NodeDrainStatusActive
		n.cordonCause = ""
		n.drainedChan = make(chan struct{})
		return nil
	})
}
