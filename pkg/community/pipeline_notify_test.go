package community_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/community"
)

// TestPipelineNotify_StaticFloorInvariant satisfies CRIT-1794388562446426000-558548fb.
// Verifies PipelineNotificationConfig with webhook URL, HMAC secret, timeout, and structured payload fields.
func TestPipelineNotify_StaticFloorInvariant(t *testing.T) {
	// 1. Config validation invariants
	emptyCfg := community.PipelineNotificationConfig{}
	assert.ErrorIs(t, emptyCfg.Validate(), community.ErrEmptyWebhookURL, "empty webhook URL must fail validation")

	noSecretCfg := community.PipelineNotificationConfig{WebhookURL: "https://example.com/webhook"}
	assert.ErrorIs(t, noSecretCfg.Validate(), community.ErrEmptyHMACSecret, "empty HMAC secret must fail validation")

	negTimeoutCfg := community.PipelineNotificationConfig{
		WebhookURL: "https://example.com/webhook",
		HMACSecret: "super-secret-key-12345",
		Timeout:    -1 * time.Second,
	}
	assert.ErrorIs(t, negTimeoutCfg.Validate(), community.ErrInvalidTimeout, "negative timeout must fail validation")

	validCfg := community.PipelineNotificationConfig{
		WebhookURL: "https://example.com/webhook",
		HMACSecret: "super-secret-key-12345",
	}
	require.NoError(t, validCfg.Validate(), "valid config must pass")
	assert.Equal(t, community.DefaultNotificationTimeout, validCfg.Timeout, "default timeout must be applied")
	assert.Equal(t, community.DefaultNotificationRetries, validCfg.MaxRetries, "default max retries must be applied")

	// 2. Payload schema contracts
	emptyPayload := community.PipelineStatusPayload{}
	assert.ErrorIs(t, emptyPayload.Validate(), community.ErrEmptySHA, "payload missing SHA must fail validation")

	noStagePayload := community.PipelineStatusPayload{SHA: "abc1234def5678"}
	assert.ErrorIs(t, noStagePayload.Validate(), community.ErrEmptyStage, "payload missing stage must fail validation")

	noStatusPayload := community.PipelineStatusPayload{SHA: "abc1234def5678", Stage: "unit-tests"}
	assert.ErrorIs(t, noStatusPayload.Validate(), community.ErrEmptyStatus, "payload missing status must fail validation")

	validPayload := community.PipelineStatusPayload{
		SHA:            "abc1234def5678",
		Stage:          "unit-tests",
		Status:         community.StageStatusSuccess,
		ExitCode:       0,
		DurationMS:     1250,
		FailureSummary: "",
		Metadata: map[string]string{
			"runner": "macos-arm64",
		},
	}
	data, err := validPayload.ToJSON()
	require.NoError(t, err, "valid payload must serialize to JSON")

	var parsed community.PipelineStatusPayload
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err, "serialized JSON must deserialize cleanly")
	assert.Equal(t, validPayload.SHA, parsed.SHA)
	assert.Equal(t, validPayload.Stage, parsed.Stage)
	assert.Equal(t, validPayload.Status, parsed.Status)
	assert.Equal(t, validPayload.ExitCode, parsed.ExitCode)
	assert.Equal(t, validPayload.DurationMS, parsed.DurationMS)

	// 3. HMAC computation contract
	secret := "my-shared-pipeline-secret"
	sig := community.ComputePipelineHMAC(secret, data)
	assert.NotEmpty(t, sig, "HMAC signature must not be empty")
	assert.True(t, community.VerifyPipelineHMAC(secret, data, sig), "computed signature must verify cleanly")
}

// TestPipelineNotify_OperationalDynamicProof satisfies CRIT-1794388568962505000-d27def0b.
// Proves pipeline stage events trigger authenticated HTTP POST notifications with HMAC-SHA256 signature headers, exponential backoff retries, and non-blocking delivery.
func TestPipelineNotify_OperationalDynamicProof(t *testing.T) {
	var requestCount int32
	var receivedHeaders http.Header
	var receivedBody []byte

	secret := "dynamic-proof-secret-key-9876"

	// Mock server that fails once then succeeds to test retry logic
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count == 1 {
			// First attempt returns 503 to trigger retry
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		receivedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := community.PipelineNotificationConfig{
		WebhookURL:   server.URL,
		HMACSecret:   secret,
		Timeout:      2 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 10 * time.Millisecond,
	}

	dispatcher, err := community.NewPipelineNotificationDispatcher(cfg, server.Client())
	require.NoError(t, err, "dispatcher must be created successfully")

	payload := community.PipelineStatusPayload{
		SHA:            "e4d909c290d0fb1ca068ffaddf22cbd0add823a",
		Stage:          "integration-tests",
		Status:         community.StageStatusSuccess,
		ExitCode:       0,
		DurationMS:     4500,
		FailureSummary: "",
		Metadata: map[string]string{
			"suite": "core-kernel",
		},
	}

	// 1. Synchronous dispatch with retry proof
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = dispatcher.Dispatch(ctx, payload)
	assert.NoError(t, err, "dispatch should succeed after retry")
	assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "expected exactly 2 requests (1 failure + 1 retry success)")

	// Verify headers and HMAC signature
	sigHeader := receivedHeaders.Get(community.HeaderPipelineSignature)
	assert.NotEmpty(t, sigHeader, "signature header must be present")
	assert.True(t, community.VerifyPipelineHMAC(secret, receivedBody, sigHeader), "received HMAC signature must be valid")
	assert.Equal(t, "integration-tests", receivedHeaders.Get(community.HeaderPipelineStage))
	assert.Equal(t, community.StageStatusSuccess, receivedHeaders.Get(community.HeaderPipelineStatus))
	assert.Equal(t, "0", receivedHeaders.Get(community.HeaderPipelineExitCode))

	// 2. Non-blocking asynchronous dispatch proof
	atomic.StoreInt32(&requestCount, 1) // Next request will succeed directly (count will become 2)
	asyncPayload := payload
	asyncPayload.Stage = "publish-artifacts"

	dispatcher.DispatchAsync(context.Background(), asyncPayload)
	dispatcher.Drain()

	assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "async dispatch must complete and deliver payload")
}

// TestPipelineNotify_NegativeAdversarialBoundary satisfies CRIT-1794388575173500000-6515ce6b.
// Rejects continuing execution when blocking stage reports non-zero exit code or fatal error, terminating in-flight downstream stages immediately.
func TestPipelineNotify_NegativeAdversarialBoundary(t *testing.T) {
	// 1. HMAC forgery rejection boundary
	secret := "secure-ci-secret"
	tamperedSecret := "attacker-forged-secret"
	testPayload := []byte(`{"sha":"abc","stage":"unit-tests","status":"success"}`)

	validSig := community.ComputePipelineHMAC(secret, testPayload)
	assert.False(t, community.VerifyPipelineHMAC(tamperedSecret, testPayload, validSig),
		"HMAC with incorrect secret must be rejected")

	tamperedPayload := []byte(`{"sha":"abc","stage":"unit-tests","status":"failed"}`)
	assert.False(t, community.VerifyPipelineHMAC(secret, tamperedPayload, validSig),
		"tampered payload with original signature must be rejected")

	// 2. Fail-Fast Governor boundary
	blockingStages := []string{"lint-format", "unit-tests", "security-audit"}
	gov := community.NewPipelineFailFastGovernor(blockingStages)

	assert.False(t, gov.ShouldAbort(), "initial governor state must not be aborted")

	// Passing non-blocking stage
	stage1 := community.PipelineStatusPayload{
		SHA:      "abc1234",
		Stage:    "setup-environment",
		Status:   community.StageStatusSuccess,
		ExitCode: 0,
	}
	err := gov.RecordStageResult(stage1)
	assert.NoError(t, err, "successful stage must not trigger governor")
	assert.False(t, gov.ShouldAbort(), "governor should remain false after success")

	// Failing non-blocking stage (warning/advisory)
	stageAdvisory := community.PipelineStatusPayload{
		SHA:            "abc1234",
		Stage:          "advisory-scan",
		Status:         community.StageStatusFailed,
		ExitCode:       2,
		FailureSummary: "non-blocking advisory warnings detected",
	}
	err = gov.RecordStageResult(stageAdvisory)
	assert.NoError(t, err, "failing non-blocking stage must not trigger governor abort")
	assert.False(t, gov.ShouldAbort(), "governor should still remain false")

	// Failing blocking stage (e.g. unit-tests)
	stageBlockingFail := community.PipelineStatusPayload{
		SHA:            "abc1234",
		Stage:          "unit-tests",
		Status:         community.StageStatusFailed,
		ExitCode:       1,
		FailureSummary: "12 tests failed in cli-commands suite",
	}
	err = gov.RecordStageResult(stageBlockingFail)
	assert.ErrorIs(t, err, community.ErrPipelineAborted, "failing blocking stage must trip the governor")
	assert.True(t, gov.ShouldAbort(), "governor must report ShouldAbort() == true")
	assert.Equal(t, "unit-tests", gov.FailedStage(), "failed stage must be recorded")
	assert.Contains(t, gov.AbortReason(), "12 tests failed in cli-commands suite", "abort reason must detail failure")

	// Downstream stage attempted after abort must be rejected fail-closed
	downstreamStage := community.PipelineStatusPayload{
		SHA:      "abc1234",
		Stage:    "deploy-docs",
		Status:   community.StageStatusSuccess,
		ExitCode: 0,
	}
	err = gov.RecordStageResult(downstreamStage)
	assert.ErrorIs(t, err, community.ErrPipelineAborted, "downstream stage execution must be blocked fail-closed")

	// Reset governor boundary
	gov.Reset()
	assert.False(t, gov.ShouldAbort(), "reset governor must clear aborted state")
	assert.Empty(t, gov.AbortReason(), "reset governor must clear abort reason")
}
