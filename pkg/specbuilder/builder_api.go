package specbuilder

import (
	"net/http"
)

// Builder constructs API clients safely.
type Builder interface {
	WithSpec(spec APISpec) Builder
	WithTelemetry(enabled bool) Builder
	WithResiliency(enabled bool) Builder
	Build() (APIClient, error)
}

type apiBuilder struct {
	spec             APISpec
	enableTelemetry  bool
	enableResiliency bool
}

// NewBuilder creates a new APISpec Builder.
func NewBuilder() Builder {
	return &apiBuilder{}
}

func (b *apiBuilder) WithSpec(spec APISpec) Builder {
	b.spec = spec
	return b
}

func (b *apiBuilder) WithTelemetry(enabled bool) Builder {
	b.enableTelemetry = enabled
	return b
}

func (b *apiBuilder) WithResiliency(enabled bool) Builder {
	b.enableResiliency = enabled
	return b
}

func (b *apiBuilder) Build() (APIClient, error) {
	// Construct the standard http client
	client := &http.Client{
		Timeout: b.spec.Timeout,
	}

	// Wrap the transport to inject telemetry, auth, and resiliency.
	var transport http.RoundTripper = http.DefaultTransport

	if b.enableResiliency {
		transport = newResiliencyTransport(transport, b.spec.RetryPolicy)
	}

	if b.enableTelemetry {
		transport = newTelemetryTransport(transport, b.spec.Name)
	}

	if len(b.spec.AuthStrategies) > 0 {
		transport = newAuthTransport(transport, b.spec.AuthStrategies)
	}

	client.Transport = transport

	return &defaultAPIClient{client: client}, nil
}

type defaultAPIClient struct {
	client *http.Client
}

func (c *defaultAPIClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}
