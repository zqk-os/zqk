package specbuilder

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dummyAuthStrategy struct {
	headerKey string
	headerVal string
	err       error
}

func (s *dummyAuthStrategy) Apply(req *http.Request) error {
	if s.err != nil {
		return s.err
	}
	req.Header.Set(s.headerKey, s.headerVal)
	return nil
}

type errTransport struct {
	err error
}

func (e *errTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, e.err
}

// TestSpecBuilder_ThreeFoldProof verifies Static Floor, Operational Proof, and Negative Boundary invariants
func TestSpecBuilder_ThreeFoldProof(t *testing.T) {
	t.Run("Stage1_StaticFloor_BuilderConfigurationAndDefaults", func(t *testing.T) {
		builder := NewBuilder()
		require.NotNil(t, builder)

		// Chain all configuration methods
		b := builder.
			WithSpec(APISpec{
				Name:    "static-spec",
				BaseURL: "http://localhost:8080",
				Timeout: 5 * time.Second,
				RetryPolicy: RetryPolicy{
					MaxRetries: 2,
					Backoff:    5 * time.Millisecond,
				},
			}).
			WithTelemetry(true).
			WithResiliency(true)

		require.NotNil(t, b)

		client, err := b.Build()
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("Stage2_OperationalProof_FullPipelineWithAuthAndTelemetry", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer test-token-123", r.Header.Get("Authorization"))
			require.Equal(t, "custom-id-xyz", r.Header.Get("X-Custom-Auth"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
		defer server.Close()

		auth1 := &dummyAuthStrategy{headerKey: "Authorization", headerVal: "Bearer test-token-123"}
		auth2 := &dummyAuthStrategy{headerKey: "X-Custom-Auth", headerVal: "custom-id-xyz"}

		spec := APISpec{
			Name:           "test-service",
			BaseURL:        server.URL,
			Timeout:        10 * time.Second,
			RetryPolicy:    RetryPolicy{MaxRetries: 2, Backoff: 5 * time.Millisecond},
			AuthStrategies: []AuthStrategy{auth1, auth2},
		}

		client, err := NewBuilder().
			WithSpec(spec).
			WithTelemetry(true).
			WithResiliency(true).
			Build()

		require.NoError(t, err)
		require.NotNil(t, client)

		req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/test", nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "ok")
	})

	t.Run("Stage3_NegativeBoundary_AuthFailureAndTelemetryErrors", func(t *testing.T) {
		// 1. Auth Strategy error propagation
		expectedAuthErr := errors.New("auth token expired or revoked")
		failingAuth := &dummyAuthStrategy{err: expectedAuthErr}

		authTx := newAuthTransport(nil, []AuthStrategy{failingAuth})
		req, _ := http.NewRequest(http.MethodGet, "http://example.com/protected", nil)
		resp, err := authTx.RoundTrip(req)
		require.ErrorIs(t, err, expectedAuthErr)
		require.Nil(t, resp)

		// 2. Telemetry transport error logging when base RoundTripper returns error
		netErr := errors.New("network connection reset by peer")
		telemetryTx := newTelemetryTransport(&errTransport{err: netErr}, "error-api")
		resp, err = telemetryTx.RoundTrip(req)
		require.ErrorIs(t, err, netErr)
		require.Nil(t, resp)

		// 3. Resiliency with nil base and MaxRetries <= 0 default fallback
		resTx := newResiliencyTransport(nil, RetryPolicy{MaxRetries: 0, Backoff: 0})
		require.NotNil(t, resTx)

		// 4. Telemetry with nil base transport
		telemetryDefault := newTelemetryTransport(nil, "default-transport-api")
		require.NotNil(t, telemetryDefault)

		// 5. Auth with nil base transport
		authDefault := newAuthTransport(nil, nil)
		require.NotNil(t, authDefault)
	})
}

func TestSpecBuilder_ResiliencyExhaustion(t *testing.T) {
	failCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		failCount++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("Service Unavailable"))
	}))
	defer server.Close()

	spec := APISpec{
		Name:    "failing-service",
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
		RetryPolicy: RetryPolicy{
			MaxRetries: 3,
			Backoff:    2 * time.Millisecond,
		},
	}

	client, err := NewBuilder().
		WithSpec(spec).
		WithResiliency(true).
		Build()

	require.NoError(t, err)

	req, _ := http.NewRequest(http.MethodGet, server.URL, bytes.NewBuffer(nil))
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.Equal(t, 3, failCount)
}
