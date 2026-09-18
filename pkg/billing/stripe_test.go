package billing

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zqk-os/zqk/pkg/license"
)

type mockBlocklist struct {
	blocked map[string]bool
}

func (m *mockBlocklist) AddToBlocklist(subID string) {
	if m.blocked == nil {
		m.blocked = make(map[string]bool)
	}
	m.blocked[subID] = true
}

func TestStripeHandler_SubscriptionCreated(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	blocklist := &mockBlocklist{blocked: make(map[string]bool)}
	handler := NewStripeHandler(priv, blocklist)

	payload := StripePayload{
		Type:           "customer.subscription.created",
		CustomerID:     "cust_123",
		SubscriptionID: "sub_456",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	tokenStr := resp["token"]
	if tokenStr == "" {
		t.Fatalf("expected token in response")
	}

	// Verify the token using the license package
	validator := license.NewValidator(pub)
	claims, err := validator.Verify(tokenStr)
	if err != nil {
		t.Fatalf("failed to verify generated token: %v", err)
	}

	if claims.Subject != "cust_123" {
		t.Errorf("expected subject cust_123, got %s", claims.Subject)
	}
	if !claims.HasFeature("relay_access") {
		t.Errorf("expected relay_access feature")
	}
}

func TestStripeHandler_SubscriptionDeleted(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	blocklist := &mockBlocklist{blocked: make(map[string]bool)}
	handler := NewStripeHandler(priv, blocklist)

	payload := StripePayload{
		Type:           "customer.subscription.deleted",
		CustomerID:     "cust_123",
		SubscriptionID: "sub_456",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if !blocklist.blocked["cust_123"] {
		t.Errorf("expected cust_123 to be blocklisted")
	}
}

func TestStripeHandler_SubscriptionDeleted_MissingCustomerID(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	blocklist := &mockBlocklist{blocked: make(map[string]bool)}
	handler := NewStripeHandler(priv, blocklist)

	payload := StripePayload{
		Type:           "customer.subscription.deleted",
		SubscriptionID: "sub_456",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestStripeHandler_InvalidMethod(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	handler := NewStripeHandler(priv, &mockBlocklist{})

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestStripeHandler_InvalidBody(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	handler := NewStripeHandler(priv, &mockBlocklist{})

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestStripeHandler_MissingCustomerID(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	handler := NewStripeHandler(priv, &mockBlocklist{})

	payload := StripePayload{
		Type: "customer.subscription.created",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestStripeHandler_OtherEvents(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	handler := NewStripeHandler(priv, &mockBlocklist{})

	payload := StripePayload{
		Type: "invoice.paid",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
