package api_builders

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metrics"
)

var (
	apiRequestLatency   = metrics.NewPrometheusHistogram([]float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0})
	apiRequestSuccesses uint64
	apiRequestFailures  uint64
)

// RecordAPIMetrics records the success/failure and latency of an API request.
func RecordAPIMetrics(duration time.Duration, success bool) {
	apiRequestLatency.Observe(duration.Seconds())
	if success {
		atomic.AddUint64(&apiRequestSuccesses, 1)
	} else {
		atomic.AddUint64(&apiRequestFailures, 1)
	}
}

// GetAPIMetrics returns the collected metrics for API requests.
func GetAPIMetrics() (latency *metrics.PrometheusHistogram, successes, failures uint64) {
	return apiRequestLatency, atomic.LoadUint64(&apiRequestSuccesses), atomic.LoadUint64(&apiRequestFailures)
}

// DefaultBuilder is the standard implementation of APIBuilder.
type DefaultBuilder struct {
	url     string
	method  string
	headers map[string]string
	body    []byte
	timeout time.Duration
	client  HTTPClient
	logger  *logging.EventLogger
}

// NewBuilder creates a new DefaultBuilder.
func NewBuilder() APIBuilder {
	return &DefaultBuilder{
		method:  http.MethodGet,
		headers: make(map[string]string),
		client:  &http.Client{Timeout: 30 * time.Second},
		logger:  logging.GetLogger(), // Fallback to default logger
	}
}

func (b *DefaultBuilder) SetURL(url string) APIBuilder {
	b.url = url
	return b
}

func (b *DefaultBuilder) SetMethod(method string) APIBuilder {
	b.method = method
	return b
}

func (b *DefaultBuilder) AddHeader(key, value string) APIBuilder {
	b.headers[key] = value
	return b
}

func (b *DefaultBuilder) SetBody(body []byte) APIBuilder {
	b.body = body
	return b
}

func (b *DefaultBuilder) SetTimeout(timeout time.Duration) APIBuilder {
	b.timeout = timeout
	return b
}

func (b *DefaultBuilder) SetClient(client HTTPClient) APIBuilder {
	if client != nil {
		b.client = client
	}
	return b
}

func (b *DefaultBuilder) SetRoundTripper(rt http.RoundTripper) APIBuilder {
	if rt != nil {
		if b.client == nil {
			b.client = &http.Client{Transport: rt, Timeout: 30 * time.Second}
		} else if hc, ok := b.client.(*http.Client); ok {
			// Create a completely new client rather than copying to prevent shared transport mutation
			b.client = &http.Client{
				Transport:     rt,
				CheckRedirect: hc.CheckRedirect,
				Jar:           hc.Jar,
				Timeout:       hc.Timeout,
			}
		}
	}
	return b
}

func (b *DefaultBuilder) SetLogger(logger *logging.EventLogger) APIBuilder {
	if logger != nil {
		b.logger = logger
	}
	return b
}

func (b *DefaultBuilder) Build(ctx context.Context) (*http.Response, error) {
	if b.url == "" {
		return nil, errors.New("url is required")
	}

	if b.logger != nil {
		logging.FluentEvent(b.logger).Info("APIBuilder executing request").
			String("method", b.method).
			String("url", b.url).
			Log()
	}

	// Apply timeout if specified, or enforce a strict default fallback
	timeoutToUse := b.timeout
	if timeoutToUse == 0 {
		timeoutToUse = 30 * time.Second // Fallback to prevent indefinite hangs on unbounded clients
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeoutToUse)
	defer cancel()

	var req *http.Request
	var err error

	if len(b.body) > 0 {
		req, err = http.NewRequestWithContext(ctx, b.method, b.url, bytes.NewReader(b.body))
	} else {
		req, err = http.NewRequestWithContext(ctx, b.method, b.url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range b.headers {
		req.Header.Add(k, v)
	}

	start := time.Now()
	resp, err := b.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		RecordAPIMetrics(duration, false)
		if b.logger != nil {
			logging.FluentEvent(b.logger).Error("APIBuilder request failed", err).
				String("method", b.method).
				String("url", b.url).
				Int("latency_ms", int(duration.Milliseconds())).
				Log()
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		RecordAPIMetrics(duration, false)
		bodyBytes, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr == nil {
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
		if b.logger != nil {
			logging.FluentEvent(b.logger).Error("APIBuilder request returned error status", fmt.Errorf("status %d", resp.StatusCode)).
				String("method", b.method).
				String("url", b.url).
				String("payload", string(bodyBytes)).
				Int("status_code", resp.StatusCode).
				Int("latency_ms", int(duration.Milliseconds())).
				Log()
		}
	} else {
		RecordAPIMetrics(duration, true)
		if b.logger != nil {
			logging.FluentEvent(b.logger).Info("APIBuilder request success").
				String("method", b.method).
				String("url", b.url).
				Int("status_code", resp.StatusCode).
				Int("latency_ms", int(duration.Milliseconds())).
				Log()
		}
	}

	return resp, nil
}
