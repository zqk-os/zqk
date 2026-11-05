package api_builders

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errReader struct{}

func (e *errReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func (e *errReader) Close() error {
	return nil
}

func TestRoundTripFunc(t *testing.T) {
	called := false
	fn := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	req, _ := http.NewRequest(http.MethodGet, "http://localhost", nil)
	resp, err := fn.RoundTrip(req)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestTelemetryInterceptor(t *testing.T) {
	spec := APISpec{Name: "test_telemetry"}
	interceptor := TelemetryInterceptor(spec)

	// Success case
	nextSuccess := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	pipeline := interceptor(nextSuccess)

	req, _ := http.NewRequest(http.MethodGet, "http://localhost/path", nil)
	resp, err := pipeline.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Failure case
	nextFailure := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("roundtrip error")
	})
	pipelineFail := interceptor(nextFailure)
	_, err = pipelineFail.RoundTrip(req)
	require.Error(t, err)
}

func TestResiliencyInterceptor_Retries(t *testing.T) {
	spec := APISpec{
		Name:       "retry_spec",
		MaxRetries: 3,
	}
	interceptor := ResiliencyInterceptor(spec)

	// Case 1: Succeeds on 3rd attempt after 500 then 429
	attempts := 0
	next := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		b, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.Equal(t, "payload", string(b))

		if attempts == 1 {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewReader([]byte("server error"))),
			}, nil
		}
		if attempts == 2 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(bytes.NewReader([]byte("rate limited"))),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte("success"))),
		}, nil
	})

	pipeline := interceptor(next)
	req, _ := http.NewRequest(http.MethodPost, "http://localhost", bytes.NewReader([]byte("payload")))
	resp, err := pipeline.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, attempts)

	// Case 2: Body read failure
	badReq, _ := http.NewRequest(http.MethodPost, "http://localhost", &errReader{})
	_, err = pipeline.RoundTrip(badReq)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read request body for resiliency")

	// Case 3: MaxRetries <= 0 defaults to 1 attempt
	singleSpec := APISpec{MaxRetries: 0}
	singleInterceptor := ResiliencyInterceptor(singleSpec)
	singleAttempts := 0
	singleNext := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		singleAttempts++
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(bytes.NewReader([]byte("down"))),
		}, nil
	})
	singlePipeline := singleInterceptor(singleNext)
	req2, _ := http.NewRequest(http.MethodGet, "http://localhost", nil)
	resp2, err := singlePipeline.RoundTrip(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp2.StatusCode)
	assert.Equal(t, 1, singleAttempts)
}

func TestBuildPipeline(t *testing.T) {
	spec := APISpec{
		Name:    "pipeline_spec",
		Timeout: 2 * time.Second,
	}

	order := []string{}
	int1 := func(next http.RoundTripper) http.RoundTripper {
		return RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			order = append(order, "int1_pre")
			resp, err := next.RoundTrip(req)
			order = append(order, "int1_post")
			return resp, err
		})
	}
	int2 := func(next http.RoundTripper) http.RoundTripper {
		return RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			order = append(order, "int2_pre")
			resp, err := next.RoundTrip(req)
			order = append(order, "int2_post")
			return resp, err
		})
	}

	client := BuildPipeline(nil, spec, int1, int2)
	require.NotNil(t, client)
	assert.Equal(t, 2*time.Second, client.Timeout)

	// With mock base transport
	base := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		order = append(order, "base")
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	clientWithBase := BuildPipeline(base, spec, int1, int2)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost", nil)
	resp, err := clientWithBase.Transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Interceptor 1 should execute before interceptor 2
	expectedOrder := []string{"int1_pre", "int2_pre", "base", "int2_post", "int1_post"}
	assert.Equal(t, expectedOrder, order)
}
