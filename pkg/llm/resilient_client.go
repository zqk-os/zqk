package llm

import (
	"context"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ResilientClient wraps primary and secondary clients to handle request timeout and fallback.
// When secondary is nil, StaticMockClient is used after retries (offline mock fallback).
// TRACK: follow-up in kernel backlog
type ResilientClient struct {
	primary           Client
	secondary         Client
	timeout           time.Duration
	maxRetries        int
	retryDelay        time.Duration
	allowMockFallback bool
}

// NewResilientClient creates a new ResilientClient with mock fallback when secondary is unset.
func NewResilientClient(primary Client, secondary Client, timeout time.Duration) *ResilientClient {
	return &ResilientClient{
		primary:           primary,
		secondary:         secondary,
		timeout:           timeout,
		maxRetries:        3,
		retryDelay:        1 * time.Second,
		allowMockFallback: true,
	}
}

// isNonRetryableLLMError is true for client payload faults (HTTP 400 /
// invalid_request). Retrying the same messages cannot help, and falling
// through to static_mock produces a no-tool-call "success" that parks the
// seat-worker for missing mutation evidence.
// reject omitted/nil content and mock fallback is scoped to transport only.
func isNonRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "status=400") ||
		strings.Contains(s, "invalid_request_error") ||
		strings.Contains(s, "invalid message content type")
}

func (r *ResilientClient) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.timeout > 0 {
		return context.WithTimeout(ctx, r.timeout)
	}
	return ctx, func() {}
}

func (r *ResilientClient) fallback() Client {
	if r.secondary != nil {
		return r.secondary
	}
	if r.allowMockFallback {
		return StaticMockClient{}
	}
	return nil
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
		if isNonRetryableLLMError(err) {
			break
		}

		if i < retries-1 {
			select {
			case <-ctx.Done():
				return res, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	if err != nil && fallbackAction != nil && r.fallback() != nil && !isNonRetryableLLMError(err) {
		logger := logging.GetLogger()
		kind := "secondary"
		if r.secondary == nil {
			kind = "static_mock"
		}
		logging.FluentEvent(logger).Error("Primary LLM client failed, falling back", err).
			String("fallback", kind).
			Log()
		return fallbackAction(ctx)
	}

	return res, err
}

func (r *ResilientClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (string, error) { return r.primary.GenerateIntent(c, code) },
		func(c context.Context) (string, error) {
			fb := r.fallback()
			if fb == nil {
				return "", errfmt.Errorf("no LLM fallback configured")
			}
			return fb.GenerateIntent(c, code)
		},
	)
}

func (r *ResilientClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) ([]float32, error) { return r.primary.GenerateEmbedding(c, text) },
		func(c context.Context) ([]float32, error) {
			fb := r.fallback()
			if fb == nil {
				return nil, errfmt.Errorf("no LLM fallback configured")
			}
			return fb.GenerateEmbedding(c, text)
		},
	)
}

func (r *ResilientClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (AnalysisResult, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (AnalysisResult, error) {
			return r.primary.AnalyzeVideoFrames(c, frames, script)
		},
		func(c context.Context) (AnalysisResult, error) {
			fb := r.fallback()
			if fb == nil {
				return AnalysisResult{}, errfmt.Errorf("no LLM fallback configured")
			}
			return fb.AnalyzeVideoFrames(c, frames, script)
		},
	)
}

func (r *ResilientClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (string, error) { return r.primary.GenerateCompletion(c, prompt, system) },
		func(c context.Context) (string, error) {
			fb := r.fallback()
			if fb == nil {
				return "", errfmt.Errorf("no LLM fallback configured")
			}
			return fb.GenerateCompletion(c, prompt, system)
		},
	)
}

func (r *ResilientClient) GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error) {
	return r.GenerateStructuredCompletionWithOptions(ctx, messages, tools, StructuredCompletionOptions{})
}

func (r *ResilientClient) GenerateStructuredCompletionWithOptions(ctx context.Context, messages []Message, tools []ToolDefinition, opts StructuredCompletionOptions) (StructuredCompletionResponse, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (StructuredCompletionResponse, error) {
			return StructuredCompletion(c, r.primary, messages, tools, opts)
		},
		func(c context.Context) (StructuredCompletionResponse, error) {
			fb := r.fallback()
			if fb == nil {
				return StructuredCompletionResponse{}, errfmt.Errorf("no LLM fallback configured")
			}
			return StructuredCompletion(c, fb, messages, tools, opts)
		},
	)
}

func (r *ResilientClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (AnalysisResult, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (AnalysisResult, error) {
			return r.primary.VerifyImage(c, image, referenceImage, textPrompt)
		},
		func(c context.Context) (AnalysisResult, error) {
			fb := r.fallback()
			if fb == nil {
				return AnalysisResult{}, errfmt.Errorf("no LLM fallback configured")
			}
			return fb.VerifyImage(c, image, referenceImage, textPrompt)
		},
	)
}

func (r *ResilientClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (string, error) { return r.primary.DescribeImage(c, image) },
		func(c context.Context) (string, error) {
			fb := r.fallback()
			if fb == nil {
				return "", errfmt.Errorf("no LLM fallback configured")
			}
			return fb.DescribeImage(c, image)
		},
	)
}

func (r *ResilientClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (string, error) { return r.primary.DescribeScene(c, frames) },
		func(c context.Context) (string, error) {
			fb := r.fallback()
			if fb == nil {
				return "", errfmt.Errorf("no LLM fallback configured")
			}
			return fb.DescribeScene(c, frames)
		},
	)
}

func (r *ResilientClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return executeWithRetry(ctx, r,
		func(c context.Context) (float64, error) {
			return r.primary.SemanticCompare(c, observedDescription, expectedNarrative)
		},
		func(c context.Context) (float64, error) {
			fb := r.fallback()
			if fb == nil {
				return 0, errfmt.Errorf("no LLM fallback configured")
			}
			return fb.SemanticCompare(c, observedDescription, expectedNarrative)
		},
	)
}
