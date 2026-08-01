package specbuilder

import (
	"net/http"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

// telemetryTransport injects logging and telemetry.
type telemetryTransport struct {
	base http.RoundTripper
	name string
}

func newTelemetryTransport(base http.RoundTripper, name string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &telemetryTransport{base: base, name: name}
}

func (t *telemetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	logger := logging.GetLoggerFromProfile("system")

	resp, err := t.base.RoundTrip(req)

	duration := time.Since(start)

	if err != nil {
		logging.Fluent(logger).Error("API Request Failed", err).
			String("api", t.name).
			String("method", req.Method).
			String("url", req.URL.String()).
			String("duration", duration.String()).Log()
		return resp, err
	}

	logging.Fluent(logger).Info("API Request Completed").
		String("api", t.name).
		String("method", req.Method).
		String("url", req.URL.String()).
		Int("status", resp.StatusCode).
		String("duration", duration.String()).Log()

	return resp, nil
}

// resiliencyTransport injects retries and backoffs.
type resiliencyTransport struct {
	base        http.RoundTripper
	retryPolicy RetryPolicy
}

func newResiliencyTransport(base http.RoundTripper, policy RetryPolicy) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &resiliencyTransport{base: base, retryPolicy: policy}
}

func (r *resiliencyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	maxRetries := r.retryPolicy.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1 // Default to 1 attempt (no retries)
	}

	for i := 0; i < maxRetries; i++ {
		resp, err = r.base.RoundTrip(req)
		if err == nil && resp.StatusCode < 500 {
			break
		}

		if i < maxRetries-1 && r.retryPolicy.Backoff > 0 {
			time.Sleep(r.retryPolicy.Backoff)
		}
	}

	return resp, err
}

// authTransport applies authentication strategies to the request.
type authTransport struct {
	base       http.RoundTripper
	strategies []AuthStrategy
}

func newAuthTransport(base http.RoundTripper, strategies []AuthStrategy) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &authTransport{base: base, strategies: strategies}
}

func (a *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())

	for _, strategy := range a.strategies {
		if err := strategy.Apply(clone); err != nil {
			return nil, err
		}
	}

	return a.base.RoundTrip(clone)
}
