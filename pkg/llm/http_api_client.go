package llm

import (
	"time"

	"github.com/zqk-os/zqk/pkg/specbuilder"
)

// TRACK: BLI-1783761336286408000-ca1625db — shared APISpec-backed HTTP client for LLM providers.
func newLLMAPIClient(name, baseURL string) specbuilder.APIClient {
	spec := specbuilder.APISpec{
		Name:    name,
		BaseURL: baseURL,
		Timeout: 3 * time.Minute,
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
