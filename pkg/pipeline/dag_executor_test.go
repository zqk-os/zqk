package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockExecutor struct {
	mu         sync.Mutex
	executed   []string
	shouldFail map[string]bool
}

func (m *mockExecutor) Execute(ctx context.Context, session *TaskSession, node *Node, inputs map[string]string) (string, error) {
	m.mu.Lock()
	if m.shouldFail[node.ID] {
		m.mu.Unlock()
		return "", errors.New("simulated error")
	}
	m.executed = append(m.executed, node.ID)
	m.mu.Unlock()

	// Simulate work
	time.Sleep(10 * time.Millisecond)

	return "hash-" + node.ID, nil
}

func TestDAGExecutor(t *testing.T) {
	dag := NewDAG()
	dag.AddNode(&Node{ID: "A"})
	dag.AddNode(&Node{ID: "B"})
	dag.AddNode(&Node{ID: "C", Dependencies: []string{"A", "B"}})
	dag.AddNode(&Node{ID: "D", Dependencies: []string{"C"}})

	pool := &mockExecutor{
		shouldFail: make(map[string]bool),
	}
	exec := NewDAGExecutor(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := exec.Execute(ctx, dag)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !dag.IsDone() {
		t.Fatal("expected dag to be done")
	}

	if len(pool.executed) != 4 {
		t.Fatalf("expected 4 nodes executed, got %d", len(pool.executed))
	}

	var order []string
	pool.mu.Lock()
	order = append(order, pool.executed...)
	pool.mu.Unlock()

	cIdx, dIdx := -1, -1
	for i, id := range order {
		if id == "C" {
			cIdx = i
		}
		if id == "D" {
			dIdx = i
		}
	}
	if cIdx == -1 || dIdx == -1 || cIdx > dIdx {
		t.Errorf("expected C before D, got order: %v", order)
	}
}

func TestDAGExecutorFailure(t *testing.T) {
	dag := NewDAG()
	dag.AddNode(&Node{ID: "A"})

	pool := &mockExecutor{
		shouldFail: map[string]bool{"A": true},
	}
	exec := NewDAGExecutor(pool)

	ctx := context.Background()
	err := exec.Execute(ctx, dag)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDAGExecutorParallelism(t *testing.T) {
	dag := NewDAG()
	// Add 5 independent nodes
	dag.AddNode(&Node{ID: "A"})
	dag.AddNode(&Node{ID: "B"})
	dag.AddNode(&Node{ID: "C"})
	dag.AddNode(&Node{ID: "D"})
	dag.AddNode(&Node{ID: "E"})

	pool := &mockExecutor{
		shouldFail: make(map[string]bool),
	}
	exec := NewDAGExecutor(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	err := exec.Execute(ctx, dag)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !dag.IsDone() {
		t.Fatal("expected dag to be done")
	}

	// 5 nodes sleeping 10ms each sequentially would take 50ms+.
	// In parallel, it should take ~10-20ms.
	if duration >= 40*time.Millisecond {
		t.Fatalf("execution took too long (%v), nodes likely not running in parallel", duration)
	}
}
