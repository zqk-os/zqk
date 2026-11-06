package router

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/search/interceptor"
)

// Pipeline and stage identifiers for L7 gradient command and tool routing.
const (
	PipelineKindGradientRouter = "gradient_router"

	StageFunnelFilter            = "funnel_filter"
	StageInRepoFilter            = "in_repo_filter"
	StageTokenBudgetGuard        = "token_budget_guard"
	StageSpecializedToolSelector = "specialized_tool_selector"
	StageExternalFallback        = "external_fallback"

	ActionIntercept   = "intercept"
	ActionPassthrough = "passthrough"
	ActionFallback    = "fallback"

	ReasonEmptyRequest            = "empty_request"
	ReasonUnsupportedFlags        = "unsupported_flags"
	ReasonExternalPath            = "external_path"
	ReasonStreamPipeline          = "stream_pipeline"
	ReasonSpecializedToolSelected = "specialized_tool_selected"
	ReasonPassthroughFallback     = "passthrough_system_fallback"
)

// RouteRequest defines the input payload for routing an execution or tool request through the gradient router.
type RouteRequest struct {
	CommandLine string            `json:"command_line,omitempty"`
	ToolName    string            `json:"tool_name,omitempty"`
	Arguments   map[string]any    `json:"arguments,omitempty"`
	Cwd         string            `json:"cwd,omitempty"`
	ProjectRoot string            `json:"project_root,omitempty"`
	TokenBudget int               `json:"token_budget,omitempty"`
	ModelTier   ModelTier         `json:"model_tier,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// RouteDecision captures the decision, rewrites, and metrics resulting from gradient router pipeline evaluation.
type RouteDecision struct {
	OriginalCommand  string `json:"original_command,omitempty"`
	RewrittenCommand string `json:"rewritten_command,omitempty"`
	TargetTool       string `json:"target_tool,omitempty"`
	Action           string `json:"action"` // "intercept", "passthrough", "fallback"
	Intercepted      bool   `json:"intercepted"`
	Bypassed         bool   `json:"bypassed"`
	Fallback         bool   `json:"fallback"`
	Reason           string `json:"reason,omitempty"`
	Stage            string `json:"stage,omitempty"`
	TokenBudget      int    `json:"token_budget,omitempty"`
	EstimatedTokens  int    `json:"estimated_tokens,omitempty"`
}

// routingState tracks mutable context as a RouteRequest progresses through the routing pipeline.
type routingState struct {
	req             RouteRequest
	decision        RouteDecision
	halt            bool
	searchIntercept *interceptor.InterceptResult
	projectRoot     string
}

// RouterStats aggregates ambient operational metrics and utilization across model tiers and tools.
type RouterStats struct {
	ModelRequestsTotal   uint64               `json:"model_requests_total"`
	ModelRequestsByTier  map[ModelTier]uint64 `json:"model_requests_by_tier"`
	ToolCallsTotal       uint64               `json:"tool_calls_total"`
	ToolCallsIntercepted uint64               `json:"tool_calls_intercepted"`
	ToolCallsPassthrough uint64               `json:"tool_calls_passthrough"`
	FallbackEventsTotal  uint64               `json:"fallback_events_total"`
	RouteRequestsTotal   uint64               `json:"route_requests_total"`
	RoutesChosen         map[string]uint64    `json:"routes_chosen"`
	BypassReasons        map[string]uint64    `json:"bypass_reasons"`
}

// GradientRouter combines model gradient dispatch with scope-aware tool routing.
type GradientRouter struct {
	modelRouter     *ModelRouter
	toolRouter      *ToolRouter
	envelope        *TimeoutEnvelope
	scope           *ToolScope
	routingPipeline *pipeline.Pipeline

	statsMu             sync.RWMutex
	modelRequestsTotal  atomic.Uint64
	modelRequestsByTier map[ModelTier]*atomic.Uint64
	toolCallsTotal      atomic.Uint64
	toolIntercepted     atomic.Uint64
	toolPassthrough     atomic.Uint64
	fallbackEvents      atomic.Uint64

	routeRequestsTotal atomic.Uint64
	routesChosen       map[string]*atomic.Uint64
	bypassReasons      map[string]*atomic.Uint64
}

// NewGradientRouter initializes a router with the provided envelope and tool scope.
func NewGradientRouter(env *TimeoutEnvelope, scope *ToolScope) *GradientRouter {
	if env == nil {
		env = NewTimeoutEnvelope()
	}
	pipe, _ := buildRoutingPipeline(nil)
	gr := &GradientRouter{
		modelRouter:     NewModelRouter(env),
		toolRouter:      NewToolRouter(scope),
		envelope:        env,
		scope:           scope,
		routingPipeline: pipe,
		modelRequestsByTier: map[ModelTier]*atomic.Uint64{
			TierFast:     new(atomic.Uint64),
			TierStandard: new(atomic.Uint64),
			TierHeavy:    new(atomic.Uint64),
			TierLocal:    new(atomic.Uint64),
		},
		routesChosen:  make(map[string]*atomic.Uint64),
		bypassReasons: make(map[string]*atomic.Uint64),
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

func (r *GradientRouter) incCounter(m map[string]*atomic.Uint64, key string) {
	if key == "" {
		return
	}
	r.statsMu.RLock()
	c, ok := m[key]
	r.statsMu.RUnlock()
	if ok && c != nil {
		c.Add(1)
		return
	}
	r.statsMu.Lock()
	defer r.statsMu.Unlock()
	c, ok = m[key]
	if !ok {
		c = new(atomic.Uint64)
		m[key] = c
	}
	c.Add(1)
}

func (r *GradientRouter) incRouteChosen(route string) {
	r.incCounter(r.routesChosen, route)
}

func (r *GradientRouter) incBypassReason(reason string) {
	r.incCounter(r.bypassReasons, reason)
}

// Route routes a command or tool request through the L7 gradient router pipeline.
func (r *GradientRouter) Route(ctx context.Context, req *RouteRequest) (*RouteDecision, error) {
	if req == nil {
		return nil, ErrNilRouteRequest
	}
	r.routeRequestsTotal.Add(1)

	pctx := pipeline.NewContext(ctx)
	projRoot := req.ProjectRoot
	if projRoot == "" {
		if r.scope != nil && r.scope.WorkspaceRoot != "" {
			projRoot = r.scope.WorkspaceRoot
		} else {
			projRoot = paths.ResolveProjectRoot(req.Cwd)
		}
	}

	state := &routingState{
		req:         *req,
		projectRoot: projRoot,
	}

	out, err := r.routingPipeline.Run(pctx, state)
	if err != nil {
		return nil, err
	}
	resState, ok := out.(*routingState)
	if !ok {
		return nil, errfmt.Errorf("gradient router: unexpected pipeline output %T", out)
	}

	decision := &resState.decision
	if decision.TargetTool != "" {
		r.incRouteChosen(decision.TargetTool)
	}
	if decision.Bypassed && decision.Reason != "" {
		r.incBypassReason(decision.Reason)
	}

	return decision, nil
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

	routesMap := make(map[string]uint64)
	for k, counter := range r.routesChosen {
		if counter != nil {
			routesMap[k] = counter.Load()
		}
	}

	bypassMap := make(map[string]uint64)
	for k, counter := range r.bypassReasons {
		if counter != nil {
			bypassMap[k] = counter.Load()
		}
	}

	return RouterStats{
		ModelRequestsTotal:   r.modelRequestsTotal.Load(),
		ModelRequestsByTier:  tierStats,
		ToolCallsTotal:       r.toolCallsTotal.Load(),
		ToolCallsIntercepted: r.toolIntercepted.Load(),
		ToolCallsPassthrough: r.toolPassthrough.Load(),
		FallbackEventsTotal:  r.fallbackEvents.Load(),
		RouteRequestsTotal:   r.routeRequestsTotal.Load(),
		RoutesChosen:         routesMap,
		BypassReasons:        bypassMap,
	}
}

// buildRoutingPipeline constructs the stage pipeline for L7 command & tool routing.
func buildRoutingPipeline(logger logging.Logger) (*pipeline.Pipeline, error) {
	b := pipeline.NewBuilder(PipelineKindGradientRouter, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(StageFunnelFilter, stageFunnelFilter).
		AddStage(StageInRepoFilter, stageInRepoFilter).
		AddStage(StageTokenBudgetGuard, stageTokenBudgetGuard).
		AddStage(StageSpecializedToolSelector, stageSpecializedToolSelector).
		AddStage(StageExternalFallback, stageExternalFallback)

	return b.Build(), nil
}

// stageFunnelFilter validates command syntax, extracts executables, and handles unsupported flags or stream pipes.
func stageFunnelFilter(pctx *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*routingState)
	if !ok {
		return nil, errfmt.Errorf("funnel_filter: invalid payload type %T", payload)
	}
	if state.halt {
		return state, nil
	}

	cmdLine := strings.TrimSpace(state.req.CommandLine)
	toolName := strings.TrimSpace(state.req.ToolName)

	if cmdLine == "" && toolName == "" {
		state.decision.Bypassed = true
		state.decision.Fallback = true
		state.decision.Action = ActionFallback
		state.decision.Reason = ReasonEmptyRequest
		state.decision.Stage = StageFunnelFilter
		state.halt = true
		return state, nil
	}

	if cmdLine != "" {
		state.decision.OriginalCommand = state.req.CommandLine
		var opts []interceptor.Option
		if state.req.TokenBudget > 0 {
			opts = append(opts, interceptor.WithMaxTokens(state.req.TokenBudget))
		}
		ctx := context.Background()
		if pctx != nil && pctx.Ctx != nil {
			ctx = pctx.Ctx
		}
		res, err := interceptor.Intercept(ctx, interceptor.InterceptRequest{
			CommandLine: cmdLine,
			Cwd:         state.req.Cwd,
			ProjectRoot: state.projectRoot,
			Env:         state.req.Environment,
		}, opts...)
		if err == nil && res != nil {
			state.searchIntercept = res
			if res.Reason == interceptor.ReasonUnsupportedFlags {
				state.decision.Intercepted = false
				state.decision.Bypassed = true
				state.decision.Fallback = true
				state.decision.Action = ActionFallback
				state.decision.TargetTool = "system"
				state.decision.Reason = ReasonUnsupportedFlags
				state.decision.RewrittenCommand = state.req.CommandLine
				state.decision.Stage = StageFunnelFilter
				state.halt = true
				return state, nil
			}
			if res.Reason == interceptor.ReasonStreamFilter {
				state.decision.Intercepted = false
				state.decision.Bypassed = true
				state.decision.Fallback = true
				state.decision.Action = ActionFallback
				state.decision.TargetTool = "system"
				state.decision.Reason = ReasonStreamPipeline
				state.decision.RewrittenCommand = state.req.CommandLine
				state.decision.Stage = StageFunnelFilter
				state.halt = true
				return state, nil
			}
		}
	}

	return state, nil
}

// stageInRepoFilter verifies path boundaries against repository and scope roots.
func stageInRepoFilter(_ *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*routingState)
	if !ok {
		return nil, errfmt.Errorf("in_repo_filter: invalid payload type %T", payload)
	}
	if state.halt {
		return state, nil
	}

	if state.searchIntercept != nil && state.searchIntercept.Reason == interceptor.ReasonExternalPath {
		state.decision.Intercepted = false
		state.decision.Bypassed = true
		state.decision.Fallback = true
		state.decision.Action = ActionFallback
		state.decision.TargetTool = "system"
		state.decision.Reason = ReasonExternalPath
		state.decision.RewrittenCommand = state.req.CommandLine
		state.decision.Stage = StageInRepoFilter
		state.halt = true
		return state, nil
	}

	return state, nil
}

// stageTokenBudgetGuard enforces token ceilings and ensures token budgets are propagated.
func stageTokenBudgetGuard(_ *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*routingState)
	if !ok {
		return nil, errfmt.Errorf("token_budget_guard: invalid payload type %T", payload)
	}
	if state.halt {
		return state, nil
	}

	budget := state.req.TokenBudget
	if budget <= 0 {
		budget = 2000
	}
	state.decision.TokenBudget = budget
	state.decision.EstimatedTokens = budget

	if state.searchIntercept != nil && state.searchIntercept.Intercepted {
		rewritten := state.searchIntercept.RewrittenCommand
		if !strings.Contains(rewritten, "--max-tokens") {
			rewritten = rewritten + fmt.Sprintf(" --max-tokens %d", budget)
			state.searchIntercept.RewrittenCommand = rewritten
		}
	}

	return state, nil
}

// stageSpecializedToolSelector binds verified in-repo commands to specialized kernel tools.
func stageSpecializedToolSelector(_ *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*routingState)
	if !ok {
		return nil, errfmt.Errorf("specialized_tool_selector: invalid payload type %T", payload)
	}
	if state.halt {
		return state, nil
	}

	if state.searchIntercept != nil && state.searchIntercept.Intercepted {
		state.decision.Intercepted = true
		state.decision.Bypassed = false
		state.decision.Fallback = false
		state.decision.Action = ActionIntercept
		state.decision.TargetTool = paths.CLIUsage("grep")
		state.decision.OriginalCommand = state.req.CommandLine
		state.decision.RewrittenCommand = state.searchIntercept.RewrittenCommand
		state.decision.Reason = ReasonSpecializedToolSelected
		state.decision.Stage = StageSpecializedToolSelector
		state.halt = true
		return state, nil
	}

	return state, nil
}

// stageExternalFallback routes unmatched or bypassed commands to native system utilities.
func stageExternalFallback(_ *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*routingState)
	if !ok {
		return nil, errfmt.Errorf("external_fallback: invalid payload type %T", payload)
	}
	if state.halt {
		return state, nil
	}

	state.decision.Intercepted = false
	state.decision.Bypassed = true
	state.decision.Fallback = true
	state.decision.Action = ActionFallback
	state.decision.TargetTool = "system"
	state.decision.OriginalCommand = state.req.CommandLine
	state.decision.RewrittenCommand = state.req.CommandLine
	if state.decision.Reason == "" {
		state.decision.Reason = ReasonPassthroughFallback
	}
	state.decision.Stage = StageExternalFallback
	state.halt = true

	return state, nil
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
