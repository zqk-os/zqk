package router

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// RouterStats aggregates ambient operational metrics and utilization across model tiers and tools.
type RouterStats struct {
	ModelRequestsTotal   uint64               `json:"model_requests_total"`
	ModelRequestsByTier  map[ModelTier]uint64 `json:"model_requests_by_tier"`
	ToolCallsTotal       uint64               `json:"tool_calls_total"`
	ToolCallsIntercepted uint64               `json:"tool_calls_intercepted"`
	ToolCallsPassthrough uint64               `json:"tool_calls_passthrough"`
	FallbackEventsTotal  uint64               `json:"fallback_events_total"`
}

// GradientRouter combines model gradient dispatch with scope-aware tool routing.
type GradientRouter struct {
	modelRouter *ModelRouter
	toolRouter  *ToolRouter
	envelope    *TimeoutEnvelope
	scope       *ToolScope

	statsMu             sync.RWMutex
	modelRequestsTotal  atomic.Uint64
	modelRequestsByTier map[ModelTier]*atomic.Uint64
	toolCallsTotal      atomic.Uint64
	toolIntercepted     atomic.Uint64
	toolPassthrough     atomic.Uint64
	fallbackEvents      atomic.Uint64
}

// NewGradientRouter initializes a router with the provided envelope and tool scope.
func NewGradientRouter(env *TimeoutEnvelope, scope *ToolScope) *GradientRouter {
	if env == nil {
		env = NewTimeoutEnvelope()
	}
	gr := &GradientRouter{
		modelRouter: NewModelRouter(env),
		toolRouter:  NewToolRouter(scope),
		envelope:    env,
		scope:       scope,
		modelRequestsByTier: map[ModelTier]*atomic.Uint64{
			TierFast:     new(atomic.Uint64),
			TierStandard: new(atomic.Uint64),
			TierHeavy:    new(atomic.Uint64),
			TierLocal:    new(atomic.Uint64),
		},
	}
	return gr
}

// RouteModel routes an LLM request across the model gradient with adaptive timeouts.
func (r *GradientRouter) RouteModel(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
	resp, err := r.modelRouter.RouteModel(ctx, req)
	r.modelRequestsTotal.Add(1)

	if resp != nil {
		r.statsMu.RLock()
		counter, exists := r.modelRequestsByTier[resp.Tier]
		r.statsMu.RUnlock()
		if exists && counter != nil {
			counter.Add(1)
		}
		if resp.AttemptCount > 1 {
			r.fallbackEvents.Add(uint64(resp.AttemptCount - 1))
		}
	}
	return resp, err
}

// RouteTool dispatches a tool call with scope inspection and path passthrough.
func (r *GradientRouter) RouteTool(ctx context.Context, call *ToolCall) (*ToolResult, error) {
	r.toolCallsTotal.Add(1)
	res, err := r.toolRouter.RouteTool(ctx, call)
	if res != nil {
		if res.Intercepted {
			r.toolIntercepted.Add(1)
		}
		if res.Passthrough {
			r.toolPassthrough.Add(1)
		}
	} else if err != nil {
		r.toolIntercepted.Add(1)
	}
	return res, err
}

// RegisterModel binds a handler to a specific model identifier.
func (r *GradientRouter) RegisterModel(model string, handler ModelHandler) *GradientRouter {
	r.modelRouter.RegisterModel(model, handler)
	return r
}

// RegisterTier binds a handler to an entire model tier.
func (r *GradientRouter) RegisterTier(tier ModelTier, handler ModelHandler) *GradientRouter {
	r.modelRouter.RegisterTier(tier, handler)
	return r
}

// SetDefaultModelHandler sets the fallback handler for unmapped models.
func (r *GradientRouter) SetDefaultModelHandler(handler ModelHandler) *GradientRouter {
	r.modelRouter.SetDefaultHandler(handler)
	return r
}

// RegisterTool binds a handler to a tool name.
func (r *GradientRouter) RegisterTool(toolName string, handler ToolHandler) *GradientRouter {
	r.toolRouter.RegisterTool(toolName, handler)
	return r
}

// AddToolInterceptor appends an interceptor to the tool pipeline.
func (r *GradientRouter) AddToolInterceptor(interceptor ToolInterceptor) *GradientRouter {
	r.toolRouter.AddInterceptor(interceptor)
	return r
}

// SetDefaultToolHandler sets the passthrough or fallback handler for tools.
func (r *GradientRouter) SetDefaultToolHandler(handler ToolHandler) *GradientRouter {
	r.toolRouter.SetDefaultHandler(handler)
	return r
}

// Stats returns a point-in-time snapshot of router telemetry and ambient utilization.
func (r *GradientRouter) Stats() RouterStats {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	tierStats := make(map[ModelTier]uint64)
	for tier, counter := range r.modelRequestsByTier {
		if counter != nil {
			tierStats[tier] = counter.Load()
		}
	}

	return RouterStats{
		ModelRequestsTotal:   r.modelRequestsTotal.Load(),
		ModelRequestsByTier:  tierStats,
		ToolCallsTotal:       r.toolCallsTotal.Load(),
		ToolCallsIntercepted: r.toolIntercepted.Load(),
		ToolCallsPassthrough: r.toolPassthrough.Load(),
		FallbackEventsTotal:  r.fallbackEvents.Load(),
	}
}

// GradientRouterBuilder provides a fluent builder interface for constructing a GradientRouter.
type GradientRouterBuilder struct {
	envelope            *TimeoutEnvelope
	scope               *ToolScope
	defaultModelHandler ModelHandler
	defaultToolHandler  ToolHandler
}

// NewGradientRouterBuilder starts a fluent configuration sequence.
func NewGradientRouterBuilder() *GradientRouterBuilder {
	return &GradientRouterBuilder{
		envelope: NewTimeoutEnvelope(),
	}
}

// WithFastTimeout configures the baseline timeout for fast-tier models.
func (b *GradientRouterBuilder) WithFastTimeout(d time.Duration) *GradientRouterBuilder {
	b.envelope.WithBaseTimeout(TierFast, d)
	return b
}

// WithStandardTimeout configures the baseline timeout for standard-tier models.
func (b *GradientRouterBuilder) WithStandardTimeout(d time.Duration) *GradientRouterBuilder {
	b.envelope.WithBaseTimeout(TierStandard, d)
	return b
}

// WithHeavyTimeout configures the baseline timeout for heavy-tier models.
func (b *GradientRouterBuilder) WithHeavyTimeout(d time.Duration) *GradientRouterBuilder {
	b.envelope.WithBaseTimeout(TierHeavy, d)
	return b
}

// WithLocalTimeout configures the baseline timeout for local-tier models.
func (b *GradientRouterBuilder) WithLocalTimeout(d time.Duration) *GradientRouterBuilder {
	b.envelope.WithBaseTimeout(TierLocal, d)
	return b
}

// WithWorkspaceRoot initializes a strict tool scope rooted at the specified workspace.
func (b *GradientRouterBuilder) WithWorkspaceRoot(root string) *GradientRouterBuilder {
	if b.scope == nil {
		b.scope = NewToolScope(root)
	} else {
		b.scope.WorkspaceRoot = root
		b.scope.AllowedRoots = append(b.scope.AllowedRoots, root)
	}
	return b
}

// WithAllowedRoots appends additional permitted filesystem roots.
func (b *GradientRouterBuilder) WithAllowedRoots(roots ...string) *GradientRouterBuilder {
	if b.scope == nil {
		b.scope = NewToolScope("")
	}
	b.scope.AllowedRoots = append(b.scope.AllowedRoots, roots...)
	return b
}

// WithScopeMode configures the enforcement mode (Strict, Permissive, ReadOnly).
func (b *GradientRouterBuilder) WithScopeMode(mode ScopeMode) *GradientRouterBuilder {
	if b.scope == nil {
		b.scope = NewToolScope("")
	}
	b.scope.Mode = mode
	return b
}

// WithDefaultModelHandler configures the universal fallback model execution handler.
func (b *GradientRouterBuilder) WithDefaultModelHandler(handler ModelHandler) *GradientRouterBuilder {
	b.defaultModelHandler = handler
	return b
}

// WithDefaultToolHandler configures the fallback or passthrough tool execution handler.
func (b *GradientRouterBuilder) WithDefaultToolHandler(handler ToolHandler) *GradientRouterBuilder {
	b.defaultToolHandler = handler
	return b
}

// Build creates and returns the fully configured GradientRouter.
func (b *GradientRouterBuilder) Build() *GradientRouter {
	gr := NewGradientRouter(b.envelope, b.scope)
	if b.defaultModelHandler != nil {
		gr.SetDefaultModelHandler(b.defaultModelHandler)
	}
	if b.defaultToolHandler != nil {
		gr.SetDefaultToolHandler(b.defaultToolHandler)
	}
	return gr
}
