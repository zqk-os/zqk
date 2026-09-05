package llm

import (
	"time"

	"github.com/lanceman/zqk/pkg/specbuilder"
)

// TRACK: REDACTED — shared APISpec-backed HTTP client for LLM providers.
func newLLMAPIClient(name, baseURL string) specbuilder.APIClient {
	spec := specbuilder.APISpec{
		Name:    name,
		BaseURL: baseURL,
		Timeout: 15 * time.Minute,
		RetryPolicy: specbuilder.RetryPolicy{
			MaxRetries: 3,
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
