package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
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

func createJSONPostRequest(ctx context.Context, url string, payload any) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func executeHTTPRequest(client specbuilder.APIClient, req *http.Request) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, errfmt.Newf("request failed").Wrap(err)
	}
	return resp, nil
}
