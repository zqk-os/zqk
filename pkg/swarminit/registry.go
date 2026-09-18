package swarminit

import (
	"context"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Result is the outcome of one executor invocation.
type Result struct {
	OK       bool
	Skipped  bool
	SkipWhy  string
	Evidence map[string]any
	Human    string // set when on_fail=human_gate or chat requires opt-in
}

// Executor runs one stage.
type Executor func(ctx context.Context, env *Env, stage Stage) (Result, error)

// Registry maps executor id → implementation. Unknown ids fail closed.
type Registry struct {
	exec map[string]Executor
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{exec: map[string]Executor{}}
}

// Register adds or replaces an executor. Empty id is ignored.
func (r *Registry) Register(id string, fn Executor) {
	if r == nil || fn == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	r.exec[id] = fn
}

// Lookup returns the executor or a fail-closed error.
func (r *Registry) Lookup(id string) (Executor, error) {
	id = strings.TrimSpace(id)
	if r == nil || r.exec == nil {
		return nil, errfmt.Errorf("unknown swarm-init executor %q", id)
	}
	fn, ok := r.exec[id]
	if !ok || fn == nil {
		return nil, errfmt.Errorf("unknown swarm-init executor %q", id)
	}
	return fn, nil
}

// DefaultRegistry wires the v1 executors against env hooks.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(ExecutorControlPlane, execControlPlane)
	r.Register(ExecutorBindSeats, execBindSeats)
	r.Register(ExecutorSeatWorkers, execSeatWorkers)
	r.Register(ExecutorCommsCheck, execCommsCheck)
	r.Register(ExecutorChatBootstrap, execChatBootstrap)
	r.Register(ExecutorOrchestratePlan, execOrchestratePlan)
	return r
}
