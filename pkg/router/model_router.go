package router

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// ModelRequest encapsulates input parameters for model execution and routing.
type ModelRequest struct {
	Model         string
	Prompt        string
	PreferredTier ModelTier
	MaxTokens     int
	FallbackChain []ModelTier
	Metadata      map[string]string
}

// ModelResponse captures execution results from a resolved model endpoint.
type ModelResponse struct {
	Model        string
	Tier         ModelTier
	Content      string
	Latency      time.Duration
	TokensUsed   int
	AttemptCount int
}

// ModelHandler represents an execution function for a specific model or tier.
type ModelHandler func(ctx context.Context, req *ModelRequest) (*ModelResponse, error)

// ModelRouter manages gradient dispatch, timeout budgeting, and fallback failovers across model tiers.
type ModelRouter struct {
	mu             sync.RWMutex
	envelope       *TimeoutEnvelope
	modelHandlers  map[string]ModelHandler
	tierHandlers   map[ModelTier]ModelHandler
	defaultHandler ModelHandler
}

// NewModelRouter constructs a ModelRouter with the specified or default timeout envelope.
func NewModelRouter(env *TimeoutEnvelope) *ModelRouter {
	if env == nil {
		env = NewTimeoutEnvelope()
	}
	return &ModelRouter{
		envelope:      env,
		modelHandlers: make(map[string]ModelHandler),
		tierHandlers:  make(map[ModelTier]ModelHandler),
	}
}

// RegisterModel associates a specific model identifier with a handler.
func (r *ModelRouter) RegisterModel(model string, handler ModelHandler) {
	bindHandler(&r.mu, r.modelHandlers, normalizeKey(model), handler)
}

// RegisterTier associates a default handler for an entire model tier.
func (r *ModelRouter) RegisterTier(tier ModelTier, handler ModelHandler) {
	bindHandler(&r.mu, r.tierHandlers, tier, handler)
}

// SetDefaultHandler specifies a universal catch-all handler for unmapped models and tiers.
func (r *ModelRouter) SetDefaultHandler(handler ModelHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultHandler = handler
}

// RouteModel resolves and dispatches a model request across the gradient with adaptive timeout envelopes.
func (r *ModelRouter) RouteModel(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	trimmedModel := strings.TrimSpace(req.Model)
	if trimmedModel == "" {
		return nil, ErrEmptyModel
	}

	primaryTier := req.PreferredTier
	if !primaryTier.Valid() {
		primaryTier = ClassifyModelTier(trimmedModel)
	}

	// Construct gradient execution plan: primary tier followed by fallback tiers.
	gradient := make([]ModelTier, 0, len(req.FallbackChain)+1)
	gradient = append(gradient, primaryTier)
	seen := map[ModelTier]bool{primaryTier: true}

	for _, tier := range req.FallbackChain {
		if tier.Valid() && !seen[tier] {
			gradient = append(gradient, tier)
			seen[tier] = true
		}
	}

	start := time.Now()
	var lastErr error
	attempts := 0

	for _, tier := range gradient {
		attempts++
		handler := r.resolveHandler(trimmedModel, tier)
		if handler == nil {
			lastErr = errfmt.Errorf("%w for tier %q", ErrModelNotFound, tier)
			continue
		}

		subCtx, cancel, _ := r.envelope.WithTimeoutContext(ctx, tier, req)
		resp, err := handler(subCtx, req)
		cancel()

		if err == nil && resp != nil {
			resp.Model = trimmedModel
			resp.Tier = tier
			resp.Latency = time.Since(start)
			resp.AttemptCount = attempts
			return resp, nil
		}

		if errors.Is(subCtx.Err(), context.DeadlineExceeded) || (err != nil && errors.Is(err, context.DeadlineExceeded)) {
			lastErr = errfmt.Errorf("%w on tier %q: %v", ErrModelTimeout, tier, err)
		} else if err != nil {
			lastErr = err
		} else {
			lastErr = errfmt.Errorf("empty response on tier %q", tier)
		}
	}

	if lastErr != nil {
		return nil, errfmt.Errorf("%w (attempts: %d, last error: %v)", ErrAllTiersFailed, attempts, lastErr)
	}
	return nil, ErrAllTiersFailed
}

func (r *ModelRouter) resolveHandler(model string, tier ModelTier) ModelHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if h, ok := r.modelHandlers[strings.ToLower(model)]; ok && h != nil {
		return h
	}
	if h, ok := r.tierHandlers[tier]; ok && h != nil {
		return h
	}
	return r.defaultHandler
}
