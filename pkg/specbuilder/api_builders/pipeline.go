package api_builders

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// APISpec defines the configuration for an external API integration.
type APISpec struct {
	Name       string
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
}

// RoundTripFunc is a signature that matches http.RoundTripper.RoundTrip.
type RoundTripFunc func(req *http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Interceptor defines a middleware for the HTTP pipeline.
type Interceptor func(next http.RoundTripper) http.RoundTripper

// TelemetryInterceptor injects standard logging.Fluent metrics for observability.
func TelemetryInterceptor(spec APISpec) Interceptor {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			start := time.Now()
			logger := logging.GetLoggerFromContext(req.Context())

			logging.FluentEvent(logger).
				Info(fmt.Sprintf("%s_request_start", spec.Name)).
				URL(req.URL.String()).
				Log()

			resp, err := next.RoundTrip(req)

			duration := time.Since(start)

			if err != nil {
				logging.FluentEvent(logger).
					Error(fmt.Sprintf("%s_request_failure", spec.Name), err).
					Int("latency_ms", int(duration.Milliseconds())).
					Log()
			} else {
				logging.FluentEvent(logger).
					Info(fmt.Sprintf("%s_request_success", spec.Name)).
					Int("latency_ms", int(duration.Milliseconds())).
					Int("status_code", resp.StatusCode).
					Log()
			}

			return resp, err
		})
	}
}

// ResiliencyInterceptor handles basic retries for 429s or 5xxs.
func ResiliencyInterceptor(spec APISpec) Interceptor {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			var resp *http.Response
			var err error

			maxRetries := spec.MaxRetries
			if maxRetries <= 0 {
				maxRetries = 1
			}

			// Read body into memory to allow retries
			var bodyBytes []byte
			if req.Body != nil {
				bodyBytes, err = io.ReadAll(req.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read request body for resiliency: %w", err)
				}
				_ = req.Body.Close()
			}

			for i := 0; i < maxRetries; i++ {
				// Clone request for retry
				clonedReq := req.Clone(req.Context())
				if len(bodyBytes) > 0 {
					clonedReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
					clonedReq.GetBody = func() (io.ReadCloser, error) {
						return io.NopCloser(bytes.NewReader(bodyBytes)), nil
					}
				}

				resp, err = next.RoundTrip(clonedReq)
				if err == nil && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
					break
				}

				if i < maxRetries-1 {
					if resp != nil && resp.Body != nil {
						_ = resp.Body.Close()
					}
					time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond) // Exponential backoff
				}
			}

			return resp, err
		})
	}
}

// BuildPipeline creates an http.Client with the provided interceptors wrapped around the base transport.
func BuildPipeline(baseTransport http.RoundTripper, spec APISpec, interceptors ...Interceptor) *http.Client {
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	transport := baseTransport
	// Apply in reverse order so the first interceptor is the outermost
	for i := len(interceptors) - 1; i >= 0; i-- {
		transport = interceptors[i](transport)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   spec.Timeout,
	}
}
