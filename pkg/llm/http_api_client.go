package llm

import (
	"time"

	"github.com/zqk-os/zqk/pkg/specbuilder"
)

// defaultLLMHTTPTimeout is used only when Config.Timeout is unset.
// Local Ollama DefaultConfig is 900s; the old 3m floor caused
// "Client.Timeout exceeded while awaiting headers" then static_mock.
// TRACK: follow-up in kernel backlog
const defaultLLMHTTPTimeout = 3 * time.Minute

func llmHTTPTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultLLMHTTPTimeout
	}
	return timeout
}

func newLLMAPIClient(name, baseURL string, timeout time.Duration) specbuilder.APIClient {
	spec := specbuilder.APISpec{
		Name:    name,
		BaseURL: baseURL,
		Timeout: llmHTTPTimeout(timeout),
		RetryPolicy: specbuilder.RetryPolicy{
			MaxRetries: 2,
			Backoff:    2 * time.Second,
		},
	}
	apiClient, err := specbuilder.NewBuilder().
		WithSpec(spec).
		WithTelemetry(true).
		WithResiliency(true).
		Build()
	if err != nil || apiClient == nil {
		apiClient, _ = specbuilder.NewBuilder().WithSpec(spec).Build()
	}
	return apiClient
}
