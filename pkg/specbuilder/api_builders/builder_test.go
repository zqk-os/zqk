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
	"github.com/zqk-os/zqk/pkg/logging"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.doFunc != nil {
		return m.doFunc(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte("ok"))),
	}, nil
}

type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.roundTripFunc != nil {
		return m.roundTripFunc(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte("rt-ok"))),
	}, nil
}

func TestDefaultBuilder_ValidationAndBasicFlow(t *testing.T) {
	builder := NewBuilder()
	require.NotNil(t, builder)

	// Missing URL
	_, err := builder.Build(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url is required")

	// Successful build with custom mock client
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "POST", req.Method)
			assert.Equal(t, "https://example.com/api", req.URL.String())
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			b, _ := io.ReadAll(req.Body)
			assert.Equal(t, []byte(`{"ping":true}`), b)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"pong":true}`))),
			}, nil
		},
	}

	logger := logging.GetLogger()
	resp, err := builder.
		SetURL("https://example.com/api").
		SetMethod(http.MethodPost).
		AddHeader("Content-Type", "application/json").
		SetBody([]byte(`{"ping":true}`)).
		SetTimeout(5 * time.Second).
		SetClient(mockClient).
		SetLogger(logger).
		Build(context.Background())

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test 4xx/5xx error status code branch
	mockClient.doFunc = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewReader([]byte("not found"))),
		}, nil
	}

	resp, err = builder.
		SetMethod(http.MethodGet).
		SetBody(nil).
		Build(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "not found", string(body))

	// Test client Do network error
	mockClient.doFunc = func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network timeout")
	}

	_, err = builder.Build(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

func TestDefaultBuilder_RoundTripperAndClientConfig(t *testing.T) {
	b := NewBuilder().(*DefaultBuilder)

	// Set nil round tripper (noop)
	b.SetRoundTripper(nil)

	// Set round tripper on existing client
	rt := &mockRoundTripper{}
	b.SetRoundTripper(rt)
	hc, ok := b.client.(*http.Client)
	require.True(t, ok)
	assert.Equal(t, rt, hc.Transport)

	// Set round tripper when client is nil
	b.client = nil
	b.SetRoundTripper(rt)
	require.NotNil(t, b.client)

	// Set nil client and logger
	b.SetClient(nil)
	b.SetLogger(nil)
}

func TestAPIMetrics(t *testing.T) {
	RecordAPIMetrics(10*time.Millisecond, true)
	RecordAPIMetrics(20*time.Millisecond, false)

	hist, successes, failures := GetAPIMetrics()
	assert.NotNil(t, hist)
	assert.GreaterOrEqual(t, successes, uint64(1))
	assert.GreaterOrEqual(t, failures, uint64(1))
}

func TestBaseAPISpecGenBuilder(t *testing.T) {
	b := NewBaseAPISpecGenBuilder("test_api", "v1_0_0")
	require.NotNil(t, b)
	assert.Equal(t, "test_api", b.GetFileName())
	assert.Equal(t, "v1_0_0", b.GetVersion())

	rawMap := map[string]any{
		"version": "1.0.0",
		"title":   "Test API",
	}
	b.SetAPI(rawMap)

	yamlBytes, err := b.Build()
	require.NoError(t, err)
	assert.Contains(t, string(yamlBytes), "title: Test API")
	assert.Contains(t, string(yamlBytes), "version: 1.0.0")
}
