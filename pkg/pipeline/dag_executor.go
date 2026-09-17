package pipeline

import (
	"context"
	"fmt"
	"sync"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TaskSession tracks specific progress for a subagent against a graph node.
type TaskSession struct {
	NodeID string
	State  sync.Map
}

// SetState updates session state.
func (c *TaskSession) SetState(key string, val any) {
	c.State.Store(key, val)
}

// GetState retrieves session state.
func (c *TaskSession) GetState(key string) (any, bool) {
	return c.State.Load(key)
}

// TaskExecutor represents a subagent pool or handler that executes a node via MCP.
type TaskExecutor interface {
	Execute(ctx context.Context, session *TaskSession, node *Node, inputs map[string]string) (hash string, err error)
}

// Node represents an isolated unit of work in the DAG.
type Node struct {
	ID           string
	Action       string
	Role         string
	Dependencies []string
	Payload      any
}

// DAG represents the Directed Acyclic Graph of tasks.
type DAG struct {
	mu         sync.RWMutex
	nodes      map[string]*Node
	completed  map[string]string // map node ID to its output hash (CAS)
	inProgress map[string]bool
}

// NewDAG creates a new empty DAG.
func NewDAG() *DAG {
	return &DAG{
		nodes:      make(map[string]*Node),
		completed:  make(map[string]string),
		inProgress: make(map[string]bool),
	}
}

// AddNode injects a new node into the DAG dynamically.
func (d *DAG) AddNode(n *Node) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nodes[n.ID] = n
}

// isReady checks if a node's dependencies are met.
func (d *DAG) isReady(nodeID string) bool {
	n, exists := d.nodes[nodeID]
	if !exists {
		return false
	}
	for _, dep := range n.Dependencies {
		if _, done := d.completed[dep]; !done {
			return false
		}
	}
	return true
}

// NextReady returns nodes that can be executed now.
func (d *DAG) NextReady() []*Node {
	d.mu.Lock()
	defer d.mu.Unlock()
	var ready []*Node
	for id, node := range d.nodes {
		_, completed := d.completed[id]
		if completed || d.inProgress[id] {
			continue
		}
		if d.isReady(id) {
			d.inProgress[id] = true
			ready = append(ready, node)
		}
	}
	return ready
}

// MarkComplete sets a node as finished with the resulting CAS hash.
func (d *DAG) MarkComplete(nodeID string, hash string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.completed[nodeID] = hash
	delete(d.inProgress, nodeID)
}

// MarkFailed resets an in-progress node so it can be retried (e.g. by Sentinel).
func (d *DAG) MarkFailed(nodeID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.inProgress, nodeID)
}

// IsDone returns true if all nodes are completed.
func (d *DAG) IsDone() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.nodes) == 0 {
		return false
	}
	return len(d.completed) == len(d.nodes)
}

// GetInputs returns the output hashes of the dependencies for a given node.
func (d *DAG) GetInputs(nodeID string) map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	inputs := make(map[string]string)
	n, ok := d.nodes[nodeID]
	if !ok {
		return inputs
	}
	for _, dep := range n.Dependencies {
		inputs[dep] = d.completed[dep]
	}
	return inputs
}

// DAGExecutor orchestrates the execution of a DAG.
type DAGExecutor struct {
	pool TaskExecutor
}

// NewDAGExecutor creates a new DAG executor.
func NewDAGExecutor(pool TaskExecutor) *DAGExecutor {
	return &DAGExecutor{pool: pool}
}

// Execute runs the DAG until completion or context cancellation.
func (e *DAGExecutor) Execute(ctx context.Context, dag *DAG) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1) // First error stops execution
	doneCh := make(chan struct{})

	// we need a waitgroup to track running nodes
	var wg sync.WaitGroup

	// signal to evaluate the DAG
	evalCh := make(chan struct{}, 1)
	evalCh <- struct{}{} // initial evaluation

	goroutinelabels.NewGoroutine("pipeline.dag_eval", "evaluate DAG and dispatch ready nodes").
		StartSimple(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-evalCh:
					if dag.IsDone() {
						close(doneCh)
						return
					}
					readyNodes := dag.NextReady()
					for _, n := range readyNodes {
						wg.Add(1)
						node := n
						goroutinelabels.NewGoroutine("pipeline.dag_node", "execute DAG node "+node.ID).
							StartSimple(func() {
								defer wg.Done()

								session := &TaskSession{
									NodeID: node.ID,
								}

								inputs := dag.GetInputs(node.ID)
								hash, err := e.pool.Execute(ctx, session, node, inputs)
								if err != nil {
									dag.MarkFailed(node.ID)
									select {
									case errCh <- fmt.Errorf("node %s failed: %w", node.ID, err):
									default:
									}
									return
								}

								dag.MarkComplete(node.ID, hash)

								select {
								case evalCh <- struct{}{}:
								default:
								}
							})
					}
				}
			}
		})

	defer func() {
		cancel()
		wg.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	case <-doneCh:
		return nil
	}
}
