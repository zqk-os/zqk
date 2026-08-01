package llm

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/multimodal"
)

// ResilientClient wraps primary and secondary clients to handle request timeout and fallback.
type ResilientClient struct {
	primary    Client
	secondary  Client
	timeout    time.Duration
	maxRetries int
	retryDelay time.Duration
}

// NewResilientClient creates a new ResilientClient.
func NewResilientClient(primary Client, secondary Client, timeout time.Duration) *ResilientClient {
	return &ResilientClient{
		primary:    primary,
		secondary:  secondary,
		timeout:    timeout,
		maxRetries: 3,
		retryDelay: 1 * time.Second,
	}
}

func (r *ResilientClient) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.timeout > 0 {
		return context.WithTimeout(ctx, r.timeout)
	}
	return ctx, func() {}
}

func executeWithRetry[T any](ctx context.Context, r *ResilientClient, action func(context.Context) (T, error), fallbackAction func(context.Context) (T, error)) (T, error) {
	var err error
	var res T

	retries := r.maxRetries
	if retries == 0 {
		retries = 3
	}
	delay := r.retryDelay
	if delay == 0 {
		delay = 1 * time.Second
	}

	for i := 0; i < retries; i++ {
		tCtx, cancel := r.withTimeout(ctx)
		res, err = action(tCtx)
		cancel()

		if err == nil {
			return res, nil
		}

		if i < retries-1 {
			select {
			case <-ctx.Done():
				return res, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	if err != nil && r.secondary != nil {
		logger := logging.GetLogger()
		logging.FluentEvent(logger).Warn("Primary LLM client failed, falling back to secondary").
			WithError(err).Log()
		if fallbackAction != nil {
			return fallbackAction(ctx)
		}
	}

	return res, err
}

func (r *ResilientClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (string, error) { return r.primary.GenerateIntent(c, code) },
		func(c context.Context) (string, error) { return r.secondary.GenerateIntent(c, code) },
	)
}

func (r *ResilientClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) ([]float32, error) { return r.primary.GenerateEmbedding(c, text) },
		func(c context.Context) ([]float32, error) { return r.secondary.GenerateEmbedding(c, text) },
	)
}

func (r *ResilientClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (multimodal.AnalysisResult, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (multimodal.AnalysisResult, error) {
			return r.primary.AnalyzeVideoFrames(c, frames, script)
		},
		func(c context.Context) (multimodal.AnalysisResult, error) {
			return r.secondary.AnalyzeVideoFrames(c, frames, script)
		},
	)
}

func (r *ResilientClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (string, error) { return r.primary.GenerateCompletion(c, prompt, system) },
		func(c context.Context) (string, error) { return r.secondary.GenerateCompletion(c, prompt, system) },
	)
}

func (r *ResilientClient) GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (StructuredCompletionResponse, error) {
			return r.primary.GenerateStructuredCompletion(c, messages, tools)
		},
		func(c context.Context) (StructuredCompletionResponse, error) {
			return r.secondary.GenerateStructuredCompletion(c, messages, tools)
		},
	)
}

func (r *ResilientClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (multimodal.AnalysisResult, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (multimodal.AnalysisResult, error) {
			return r.primary.VerifyImage(c, image, referenceImage, textPrompt)
		},
		func(c context.Context) (multimodal.AnalysisResult, error) {
			return r.secondary.VerifyImage(c, image, referenceImage, textPrompt)
		},
	)
}

func (r *ResilientClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (string, error) { return r.primary.DescribeImage(c, image) },
		func(c context.Context) (string, error) { return r.secondary.DescribeImage(c, image) },
	)
}

func (r *ResilientClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (string, error) { return r.primary.DescribeScene(c, frames) },
		func(c context.Context) (string, error) { return r.secondary.DescribeScene(c, frames) },
	)
}

func (r *ResilientClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (float64, error) {
			return r.primary.SemanticCompare(c, observedDescription, expectedNarrative)
		},
		func(c context.Context) (float64, error) {
			return r.secondary.SemanticCompare(c, observedDescription, expectedNarrative)
		},
	)
}
