package community

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// DefaultMaxWebhookBodyBytes is the maximum allowed payload size for webhook ingestion (1 MB).
	DefaultMaxWebhookBodyBytes = 1024 * 1024

	// DefaultMaxTimestampSkew is the maximum allowed difference between webhook timestamp and server time (5 minutes).
	DefaultMaxTimestampSkew = 5 * time.Minute

	// HeaderWebhookSignature is the HTTP header containing the HMAC signature.
	HeaderWebhookSignature = "X-ZQK-Signature-256"

	// HeaderWebhookTimestamp is the HTTP header containing the webhook generation unix timestamp.
	HeaderWebhookTimestamp = "X-ZQK-Timestamp"

	// HeaderWebhookEvent is the HTTP header containing the event type.
	HeaderWebhookEvent = "X-ZQK-Event"

	// HeaderWebhookDelivery is the HTTP header containing the unique delivery ID.
	HeaderWebhookDelivery = "X-ZQK-Delivery"
)

var (
	ErrInvalidSignature = errors.New("webhook: invalid or forged HMAC signature")
	ErrMissingSignature = errors.New("webhook: missing signature header")
	ErrExpiredTimestamp = errors.New("webhook: timestamp skew exceeds tolerance")
	ErrMissingTimestamp = errors.New("webhook: missing timestamp header")
	ErrMalformedPayload = errors.New("webhook: malformed JSON payload")
	ErrPayloadTooLarge  = errors.New("webhook: payload exceeds maximum body limit")
	ErrDuplicateEvent   = errors.New("webhook: duplicate event id detected")
	ErrInvalidURL       = errors.New("webhook: subscription URL must be valid HTTP/HTTPS")
)

// WebhookEvent represents an ingested or outbound webhook event payload.
type WebhookEvent struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Source    string            `json:"source"`
	Timestamp time.Time         `json:"timestamp"`
	Payload   map[string]any    `json:"payload"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// WebhookSubscription represents a registered outbound webhook target.
type WebhookSubscription struct {
	ID         string        `json:"id"`
	URL        string        `json:"url"`
	Events     []string      `json:"events"`
	Secret     string        `json:"secret"`
	Active     bool          `json:"active"`
	RetryLimit int           `json:"retry_limit"`
	Timeout    time.Duration `json:"timeout"`
}

// Matches reports whether this subscription accepts the given event type.
// Supports exact match ("community.release") and wildcards ("*" or "community.*").
func (s *WebhookSubscription) Matches(eventType string) bool {
	if !s.Active {
		return false
	}
	for _, pattern := range s.Events {
		if pattern == "*" || pattern == eventType {
			return true
		}
		if strings.HasSuffix(pattern, ".*") {
			prefix := strings.TrimSuffix(pattern, ".*")
			if strings.HasPrefix(eventType, prefix+".") {
				return true
			}
		}
	}
	return false
}

// DeliveryStatus tracks the delivery result of a dispatched event to a subscription.
type DeliveryStatus struct {
	DeliveryID     string        `json:"delivery_id"`
	SubscriptionID string        `json:"subscription_id"`
	EventID        string        `json:"event_id"`
	Attempt        int           `json:"attempt"`
	StatusCode     int           `json:"status_code"`
	Success        bool          `json:"success"`
	Error          string        `json:"error,omitempty"`
	Duration       time.Duration `json:"duration"`
	Timestamp      time.Time     `json:"timestamp"`
}

// ComputeHMACSignature computes the HMAC-SHA256 signature for a payload with timestamp.
// Format: HMAC-SHA256(secret, "t=<timestamp>.<payload>")
func ComputeHMACSignature(secret string, payload []byte, timestamp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	prefix := fmt.Sprintf("t=%d.", timestamp)
	mac.Write([]byte(prefix))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyHMACSignature verifies the signature and timestamp headers against the payload and secret.
func VerifyHMACSignature(secret, signatureHeader, timestampHeader string, payload []byte, maxSkew time.Duration) error {
	if secret == "" {
		return errors.New("webhook: secret cannot be empty")
	}
	if signatureHeader == "" {
		return ErrMissingSignature
	}
	if timestampHeader == "" {
		return ErrMissingTimestamp
	}

	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: invalid timestamp format", ErrMalformedPayload)
	}

	eventTime := time.Unix(ts, 0)
	now := time.Now()
	diff := now.Sub(eventTime)
	if diff < 0 {
		diff = -diff
	}
	if maxSkew > 0 && diff > maxSkew {
		return ErrExpiredTimestamp
	}

	// Normalize signature (support "sha256=<hex>" or raw hex)
	rawSig := strings.TrimPrefix(signatureHeader, "sha256=")
	expectedSig := ComputeHMACSignature(secret, payload, ts)

	expectedBytes, err1 := hex.DecodeString(expectedSig)
	actualBytes, err2 := hex.DecodeString(rawSig)
	if err1 != nil || err2 != nil || !hmac.Equal(expectedBytes, actualBytes) {
		return ErrInvalidSignature
	}

	return nil
}

// GenerateRandomID creates a cryptographically secure random hexadecimal ID.
func GenerateRandomID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	if prefix == "" {
		return hex.EncodeToString(b)
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

// EventHandlerFunc is a callback invoked when an event is successfully ingested.
type EventHandlerFunc func(ctx context.Context, event WebhookEvent) error

// WebhookGateway handles inbound webhook ingestion with cryptographic verification.
type WebhookGateway struct {
	secret        string
	maxBodyBytes  int64
	maxSkew       time.Duration
	handlers      []EventHandlerFunc
	seenIDs       map[string]time.Time
	mu            sync.RWMutex
	cleanupTicker *time.Ticker
}

// GatewayOption configures the WebhookGateway.
type GatewayOption func(*WebhookGateway)

// WithMaxBodyBytes overrides the maximum allowed request body size.
func WithMaxBodyBytes(limit int64) GatewayOption {
	return func(g *WebhookGateway) {
		if limit > 0 {
			g.maxBodyBytes = limit
		}
	}
}

// WithMaxTimestampSkew overrides the allowable timestamp skew.
func WithMaxTimestampSkew(skew time.Duration) GatewayOption {
	return func(g *WebhookGateway) {
		g.maxSkew = skew
	}
}

// NewWebhookGateway creates a new WebhookGateway.
func NewWebhookGateway(secret string, opts ...GatewayOption) *WebhookGateway {
	gw := &WebhookGateway{
		secret:       secret,
		maxBodyBytes: DefaultMaxWebhookBodyBytes,
		maxSkew:      DefaultMaxTimestampSkew,
		seenIDs:      make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(gw)
	}
	return gw
}

// RegisterHandler registers a callback invoked upon successful ingestion.
func (g *WebhookGateway) RegisterHandler(h EventHandlerFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.handlers = append(g.handlers, h)
}

// ServeHTTP implements http.Handler for the webhook ingestion gateway.
func (g *WebhookGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Read body with limit
	r.Body = http.MaxBytesReader(w, r.Body, g.maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`{"error":"payload exceeds maximum allowed size"}`))
			return
		}
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	// Verify headers & signature
	sigHeader := r.Header.Get(HeaderWebhookSignature)
	if sigHeader == "" {
		sigHeader = r.Header.Get("X-Hub-Signature-256")
	}
	tsHeader := r.Header.Get(HeaderWebhookTimestamp)

	if err := VerifyHMACSignature(g.secret, sigHeader, tsHeader, body, g.maxSkew); err != nil {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case errors.Is(err, ErrExpiredTimestamp):
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error":%q}`, err.Error())))
		case errors.Is(err, ErrMissingSignature) || errors.Is(err, ErrMissingTimestamp) || errors.Is(err, ErrInvalidSignature):
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error":%q}`, err.Error())))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error":%q}`, err.Error())))
		}
		return
	}

	// Parse JSON event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid JSON body"}`))
		return
	}

	if event.ID == "" {
		event.ID = GenerateRandomID("evt")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Deduplication check
	g.mu.Lock()
	if _, exists := g.seenIDs[event.ID]; exists {
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		dupResp, _ := json.Marshal(map[string]any{
			"result":   "duplicate_ignored",
			"event_id": event.ID,
		})
		_, _ = w.Write(dupResp)
		return
	}
	g.seenIDs[event.ID] = time.Now()
	handlers := append([]EventHandlerFunc(nil), g.handlers...)
	g.mu.Unlock()

	// Notify registered handlers
	for _, handler := range handlers {
		_ = handler(r.Context(), event)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	okResp, _ := json.Marshal(map[string]any{
		"result":                  "accepted",
		"event_id":                event.ID,
		objects.FieldKeyEventType: event.Type,
	})
	_, _ = w.Write(okResp)
}

// WebhookDispatcher manages subscriptions and outbound event dispatches with retry semantics.
type WebhookDispatcher struct {
	client        *http.Client
	subscriptions map[string]WebhookSubscription
	history       []DeliveryStatus
	mu            sync.RWMutex
	maxHistory    int
}

// NewWebhookDispatcher creates a new outbound webhook dispatcher.
func NewWebhookDispatcher(client *http.Client) *WebhookDispatcher {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &WebhookDispatcher{
		client:        client,
		subscriptions: make(map[string]WebhookSubscription),
		maxHistory:    1000,
	}
}

// RegisterSubscription adds or updates an outbound webhook subscription.
func (d *WebhookDispatcher) RegisterSubscription(sub WebhookSubscription) error {
	if sub.ID == "" {
		sub.ID = GenerateRandomID("sub")
	}
	if sub.URL == "" || (!strings.HasPrefix(sub.URL, "http://") && !strings.HasPrefix(sub.URL, "https://")) {
		return ErrInvalidURL
	}
	if sub.RetryLimit <= 0 {
		sub.RetryLimit = 3
	}
	if sub.Timeout <= 0 {
		sub.Timeout = 5 * time.Second
	}
	if len(sub.Events) == 0 {
		sub.Events = []string{"*"}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.subscriptions[sub.ID] = sub
	return nil
}

// UnregisterSubscription removes a subscription by ID.
func (d *WebhookDispatcher) UnregisterSubscription(subID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.subscriptions[subID]; exists {
		delete(d.subscriptions, subID)
		return true
	}
	return false
}

// GetSubscriptions returns a copy of all registered subscriptions.
func (d *WebhookDispatcher) GetSubscriptions() []WebhookSubscription {
	d.mu.RLock()
	defer d.mu.RUnlock()
	res := make([]WebhookSubscription, 0, len(d.subscriptions))
	for _, sub := range d.subscriptions {
		res = append(res, sub)
	}
	return res
}

// Dispatch broadcasts an event to all matching active subscriptions concurrently.
func (d *WebhookDispatcher) Dispatch(ctx context.Context, event WebhookEvent) []DeliveryStatus {
	d.mu.RLock()
	var targets []WebhookSubscription
	for _, sub := range d.subscriptions {
		if sub.Matches(event.Type) {
			targets = append(targets, sub)
		}
	}
	d.mu.RUnlock()

	if len(targets) == 0 {
		return nil
	}

	var (
		wg       sync.WaitGroup
		statusMu sync.Mutex
		results  []DeliveryStatus
	)

	payloadBytes, err := json.Marshal(event)
	if err != nil {
		return []DeliveryStatus{{
			DeliveryID: GenerateRandomID("dlv"),
			EventID:    event.ID,
			Success:    false,
			Error:      fmt.Sprintf("json serialization error: %v", err),
			Timestamp:  time.Now().UTC(),
		}}
	}

	for _, sub := range targets {
		wg.Add(1)
		target := sub
		goroutinelabels.NewGoroutine("webhook_dispatcher_delivery", "dispatching outbound webhook event to subscriber").StartSimple(func() {
			defer wg.Done()
			status := d.deliverWithRetry(ctx, target, event.ID, event.Type, payloadBytes)
			statusMu.Lock()
			results = append(results, status)
			statusMu.Unlock()
		})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("webhook_dispatcher_wait", "waiting for deliveries").StartSimple(func() {
		wg.Wait()
		close(waitDone)
	})

	select {
	case <-waitDone:
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
	}

	d.mu.Lock()
	d.history = append(d.history, results...)
	if len(d.history) > d.maxHistory {
		d.history = d.history[len(d.history)-d.maxHistory:]
	}
	d.mu.Unlock()

	return results
}

// deliverWithRetry attempts HTTP delivery to a single subscription with retries.
func (d *WebhookDispatcher) deliverWithRetry(ctx context.Context, sub WebhookSubscription, eventID, eventType string, payload []byte) DeliveryStatus {
	deliveryID := GenerateRandomID("dlv")
	maxAttempts := sub.RetryLimit
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastStatus DeliveryStatus
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		start := time.Now()
		reqCtx, cancel := context.WithTimeout(ctx, sub.Timeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, sub.URL, bytes.NewReader(payload))
		if err != nil {
			cancel()
			return DeliveryStatus{
				DeliveryID:     deliveryID,
				SubscriptionID: sub.ID,
				EventID:        eventID,
				Attempt:        attempt,
				Success:        false,
				Error:          err.Error(),
				Duration:       time.Since(start),
				Timestamp:      time.Now().UTC(),
			}
		}

		nowUnix := time.Now().Unix()
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "ZQK-Webhook-Dispatcher/1.0")
		req.Header.Set(HeaderWebhookEvent, eventType)
		req.Header.Set(HeaderWebhookDelivery, deliveryID)
		req.Header.Set(HeaderWebhookTimestamp, strconv.FormatInt(nowUnix, 10))

		if sub.Secret != "" {
			sig := ComputeHMACSignature(sub.Secret, payload, nowUnix)
			req.Header.Set(HeaderWebhookSignature, "sha256="+sig)
		}

		resp, reqErr := d.client.Do(req)
		duration := time.Since(start)
		cancel()

		if reqErr != nil {
			lastStatus = DeliveryStatus{
				DeliveryID:     deliveryID,
				SubscriptionID: sub.ID,
				EventID:        eventID,
				Attempt:        attempt,
				Success:        false,
				Error:          reqErr.Error(),
				Duration:       duration,
				Timestamp:      time.Now().UTC(),
			}
		} else {
			_ = resp.Body.Close()
			success := resp.StatusCode >= 200 && resp.StatusCode < 300
			lastStatus = DeliveryStatus{
				DeliveryID:     deliveryID,
				SubscriptionID: sub.ID,
				EventID:        eventID,
				Attempt:        attempt,
				StatusCode:     resp.StatusCode,
				Success:        success,
				Duration:       duration,
				Timestamp:      time.Now().UTC(),
			}
			if success {
				return lastStatus
			}
			lastStatus.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}

		// Backoff briefly before retry if not on last attempt
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				lastStatus.Error = ctx.Err().Error()
				return lastStatus
			case <-time.After(time.Duration(attempt*20) * time.Millisecond):
			}
		}
	}

	return lastStatus
}

// GetDeliveryHistory returns recorded delivery statuses.
func (d *WebhookDispatcher) GetDeliveryHistory() []DeliveryStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	res := make([]DeliveryStatus, len(d.history))
	copy(res, d.history)
	return res
}
