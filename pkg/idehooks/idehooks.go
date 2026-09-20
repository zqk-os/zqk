package idehooks

import (
	"context"
	"sync"
)

// HookType represents the type of IDE hook.
type HookType string

const (
	PreToolUse     HookType = "PreToolUse"
	PostToolUse    HookType = "PostToolUse"
	PreInvocation  HookType = "PreInvocation"
	PostInvocation HookType = "PostInvocation"
	Stop           HookType = "Stop"
)

// Event encapsulates the information passed to interceptors.
type Event struct {
	Type    HookType
	Payload any
}

// Interceptor defines a standardized event interceptor for IDE hooks.
type Interceptor interface {
	OnEvent(ctx context.Context, event Event) error
}

// InterceptorFunc is a convenience type to allow functions to act as interceptors.
type InterceptorFunc func(ctx context.Context, event Event) error

// OnEvent calls the underlying function.
func (f InterceptorFunc) OnEvent(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Manager handles registration and dispatch of IDE hook events.
type Manager struct {
	mu           sync.RWMutex
	interceptors map[HookType][]Interceptor
}

// NewManager creates a new initialized Manager.
func NewManager() *Manager {
	return &Manager{
		interceptors: make(map[HookType][]Interceptor),
	}
}

// Register adds an interceptor for a specific hook type.
func (m *Manager) Register(hookType HookType, interceptor Interceptor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interceptors[hookType] = append(m.interceptors[hookType], interceptor)
}

// Dispatch fires an event to all interceptors registered for its type.
// If any interceptor returns an error, dispatch is aborted and the error is returned.
func (m *Manager) Dispatch(ctx context.Context, event Event) error {
	m.mu.RLock()
	interceptors := m.interceptors[event.Type]
	m.mu.RUnlock()

	for _, interceptor := range interceptors {
		if err := interceptor.OnEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
