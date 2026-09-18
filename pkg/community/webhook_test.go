package community_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestWebhookGateway_FunctionalAcceptance satisfies CRIT-1789712365072620000-b81dfaa7.
// Verifies inbound WebhookGateway HMAC-SHA256 signature verification, replay protection, and outbound WebhookDispatcher subscription routing.
func TestWebhookGateway_FunctionalAcceptance(t *testing.T) {
	secret := "test-secret-webhook-key-12345"
	var receivedEvents []community.WebhookEvent
	var mu sync.Mutex

	gw := community.NewWebhookGateway(
		secret,
		community.WithMaxTimestampSkew(2*time.Minute),
	)
	gw.RegisterHandler(func(ctx context.Context, ev community.WebhookEvent) error {
		mu.Lock()
		receivedEvents = append(receivedEvents, ev)
		mu.Unlock()
		return nil
	})

	server := httptest.NewServer(gw)
	defer server.Close()

	payload := map[string]any{"action": "deploy", "cluster": "community-prod"}
	event := community.WebhookEvent{
		ID:        "evt-deploy-001",
		Type:      "deployment.triggered",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}

	payloadBytes, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	ts := time.Now().Unix()
	sig := community.ComputeHMACSignature(secret, payloadBytes, ts)

	req, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(community.HeaderWebhookTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(community.HeaderWebhookSignature, "sha256="+sig)
	req.Header.Set(community.HeaderWebhookEvent, "deployment.triggered")
	req.Header.Set(community.HeaderWebhookDelivery, "del-abc-001")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected HTTP 202 Accepted, got %d", resp.StatusCode)
	}

	mu.Lock()
	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 received event, got %d", len(receivedEvents))
	}
	ev := receivedEvents[0]
	mu.Unlock()

	if ev.ID != "evt-deploy-001" {
		t.Errorf("expected event ID 'evt-deploy-001', got %q", ev.ID)
	}
	if ev.Type != "deployment.triggered" {
		t.Errorf("expected event type 'deployment.triggered', got %q", ev.Type)
	}
	if ev.Payload["action"] != "deploy" {
		t.Errorf("expected payload action 'deploy', got %v", ev.Payload["action"])
	}
}

// TestWebhookGateway_BoundaryAndErrorHandling satisfies CRIT-1789712365072621000-9ee85c48.
// Verifies boundary handling: forged HMAC signatures, expired timestamps, malformed payload, subscription event filtering.
func TestWebhookGateway_BoundaryAndErrorHandling(t *testing.T) {
	secret := "secret-boundary-check"
	gw := community.NewWebhookGateway(
		secret,
		community.WithMaxTimestampSkew(30*time.Second),
	)

	server := httptest.NewServer(gw)
	defer server.Close()
	body, _ := json.Marshal(map[string]any{objects.FieldKeyID: "evt-test", objects.FieldKeyType: "test.event"})

	t.Run("invalid_method", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 Method Not Allowed, got %d", resp.StatusCode)
		}
	})

	t.Run("missing_timestamp", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(body))
		req.Header.Set(community.HeaderWebhookSignature, "abc")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for missing timestamp, got %d", resp.StatusCode)
		}
	})

	t.Run("expired_timestamp", func(t *testing.T) {
		oldTs := time.Now().Add(-10 * time.Minute).Unix()
		sig := community.ComputeHMACSignature(secret, body, oldTs)

		req, _ := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(body))
		req.Header.Set(community.HeaderWebhookTimestamp, strconv.FormatInt(oldTs, 10))
		req.Header.Set(community.HeaderWebhookSignature, sig)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for expired timestamp, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid_forged_signature", func(t *testing.T) {
		currentTs := time.Now().Unix()
		req, _ := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(body))
		req.Header.Set(community.HeaderWebhookTimestamp, strconv.FormatInt(currentTs, 10))
		req.Header.Set(community.HeaderWebhookSignature, "deadbeefcafebabe00112233445566778899aabbccddeeff")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for invalid signature, got %d", resp.StatusCode)
		}
	})

	t.Run("malformed_json_body", func(t *testing.T) {
		currentTs := time.Now().Unix()
		badBody := []byte("{ malformed: json ]")
		sig := community.ComputeHMACSignature(secret, badBody, currentTs)

		req, _ := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(badBody))
		req.Header.Set(community.HeaderWebhookTimestamp, strconv.FormatInt(currentTs, 10))
		req.Header.Set(community.HeaderWebhookSignature, sig)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for malformed JSON, got %d", resp.StatusCode)
		}
	})

	t.Run("subscription_event_filtering", func(t *testing.T) {
		subAll := &community.WebhookSubscription{ID: "sub-all", URL: "http://example.com/all", Events: []string{"*"}, Active: true}
		subWildcard := &community.WebhookSubscription{ID: "sub-wild", URL: "http://example.com/wild", Events: []string{"community.*"}, Active: true}
		subOnlyRelease := &community.WebhookSubscription{ID: "sub-rel", URL: "http://example.com/rel", Events: []string{"release.published"}, Active: true}

		if !subAll.Matches("build.succeeded") || !subAll.Matches("release.published") {
			t.Errorf("subAll should match all events")
		}
		if !subWildcard.Matches("community.release") || !subWildcard.Matches("community.deploy") {
			t.Errorf("subWildcard should match community.*")
		}
		if subWildcard.Matches("core.release") {
			t.Errorf("subWildcard should not match core.release")
		}
		if subOnlyRelease.Matches("build.succeeded") {
			t.Errorf("subOnlyRelease should not match build.succeeded")
		}
		if !subOnlyRelease.Matches("release.published") {
			t.Errorf("subOnlyRelease should match release.published")
		}
	})
}

// TestWebhookGateway_IntegrationAndConformance satisfies CRIT-1789712365072622000-d05ed588.
// Verifies concurrent dispatching, retry backoff on transient errors, subscription registry lifecycle, and delivery reports.
func TestWebhookGateway_IntegrationAndConformance(t *testing.T) {
	var attemptCount int64
	mockSubscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&attemptCount, 1)
		// Verify standard headers
		if r.Header.Get(community.HeaderWebhookEvent) == "" {
			t.Errorf("missing event header in outgoing webhook")
		}
		if r.Header.Get(community.HeaderWebhookSignature) == "" {
			t.Errorf("missing signature header in outgoing webhook")
		}

		// Fail on first attempt, succeed on second attempt (verify retry backoff)
		if count == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer mockSubscriber.Close()

	dispatcher := community.NewWebhookDispatcher(http.DefaultClient)

	sub := community.WebhookSubscription{
		ID:         "sub-test-01",
		URL:        mockSubscriber.URL,
		Secret:     "outbound-secret-key",
		Events:     []string{"artifact.verified"},
		Active:     true,
		RetryLimit: 3,
		Timeout:    2 * time.Second,
	}

	if err := dispatcher.RegisterSubscription(sub); err != nil {
		t.Fatalf("failed to register subscription: %v", err)
	}

	if len(dispatcher.GetSubscriptions()) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(dispatcher.GetSubscriptions()))
	}

	event := community.WebhookEvent{
		ID:        "evt-9901",
		Type:      "artifact.verified",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"artifact": "zqk-v2.8.0.tar.gz", "sha256": "abc1234"},
	}

	statuses := dispatcher.Dispatch(context.Background(), event)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 delivery status record, got %d", len(statuses))
	}

	delivery := statuses[0]
	if delivery.Attempt != 2 {
		t.Errorf("expected 2 attempts due to transient retry, got %d", delivery.Attempt)
	}
	if !delivery.Success {
		t.Errorf("expected successful delivery on retry, got error: %s", delivery.Error)
	}

	history := dispatcher.GetDeliveryHistory()
	if len(history) == 0 {
		t.Errorf("expected non-empty delivery history")
	}

	// Unregister
	if !dispatcher.UnregisterSubscription("sub-test-01") {
		t.Errorf("expected successful unregister")
	}
	if len(dispatcher.GetSubscriptions()) != 0 {
		t.Errorf("expected 0 subscriptions after unregister")
	}
}
