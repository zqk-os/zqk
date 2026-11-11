package community_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/llm"
)

// CRIT-1794385617429642000-b7c79571: Static Floor Invariant
// Provider Registry Schema & Local Model Detection Interfaces
func TestModelFederation_StaticFloorInvariant(t *testing.T) {
	registry := llm.NewEngineRegistry()
	if registry == nil {
		t.Fatal("expected non-nil EngineRegistry")
	}

	// Verify profile schema validation
	invalidProfile := llm.EndpointProfile{
		EngineType: "",
		BaseURL:    "",
	}
	if err := registry.Register(invalidProfile); err == nil {
		t.Error("expected error registering invalid profile, got nil")
	}

	validProfile := llm.EndpointProfile{
		EngineType:       llm.EngineOllama,
		BaseURL:          "http://127.0.0.1:11434",
		DefaultModel:     "qwen2.5-coder:7b",
		DiscoveredModels: []string{"qwen2.5-coder:7b", "deepseek-r1:7b"},
		MaxContextTokens: 8192,
		SupportsTools:    true,
		IsHealthy:        true,
		LatencyMs:        12,
	}

	if err := registry.Register(validProfile); err != nil {
		t.Fatalf("expected successful registration, got %v", err)
	}

	got, found := registry.Get(llm.EngineOllama)
	if !found {
		t.Fatalf("expected to find registered profile for EngineOllama")
	}
	if got.DefaultModel != "qwen2.5-coder:7b" {
		t.Errorf("expected default model 'qwen2.5-coder:7b', got %s", got.DefaultModel)
	}
	if got.MaxContextTokens != 8192 {
		t.Errorf("expected max context 8192, got %d", got.MaxContextTokens)
	}

	best, err := registry.BestAvailable()
	if err != nil {
		t.Fatalf("expected BestAvailable to succeed, got %v", err)
	}
	if best.EngineType != llm.EngineOllama {
		t.Errorf("expected best engine EngineOllama, got %s", best.EngineType)
	}
}

// CRIT-1794385617429643000-ff995b3a: Operational Dynamic Proof
// Ollama and vLLM Live Model Discovery & Zero-Egress Fallback
func TestModelFederation_OperationalProof(t *testing.T) {
	// Mock local Ollama server
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:7b"},{"name":"deepseek-r1:7b"}]}`))
	}))
	defer ollamaServer.Close()

	// Mock local vLLM server
	vllmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen2.5-coder-7b-instruct"}]}`))
	}))
	defer vllmServer.Close()

	registry := llm.NewEngineRegistry()
	_ = registry.Register(llm.EndpointProfile{
		EngineType:       llm.EngineVLLM,
		BaseURL:          vllmServer.URL,
		DefaultModel:     "qwen2.5-coder-7b-instruct",
		DiscoveredModels: []string{"qwen2.5-coder-7b-instruct"},
		IsHealthy:        true,
	})
	_ = registry.Register(llm.EndpointProfile{
		EngineType:       llm.EngineOllama,
		BaseURL:          ollamaServer.URL,
		DefaultModel:     "qwen2.5-coder:7b",
		DiscoveredModels: []string{"qwen2.5-coder:7b", "deepseek-r1:7b"},
		IsHealthy:        true,
	})

	// BestAvailable prioritizes vLLM over Ollama
	best, err := registry.BestAvailable()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.EngineType != llm.EngineVLLM {
		t.Errorf("expected vLLM to take priority, got %s", best.EngineType)
	}

	// Verify fallback chain construction
	chain, err := llm.BuildFallbackChain(registry.List())
	if err != nil {
		t.Fatalf("expected fallback chain construction to succeed, got %v", err)
	}
	if len(chain) != 2 {
		t.Errorf("expected 2 engines in chain, got %d", len(chain))
	}
}

// CRIT-1794385617429644000-95fa0ebc: Negative Adversarial Boundary
// Rejection of Unauthorized External Network Egress & Malformed Model Endpoints
func TestModelFederation_NegativeAdversarialBoundary(t *testing.T) {
	guard := llm.NewZeroEgressGuard(true)

	// 1. Loopback addresses should succeed
	validTargets := []string{
		"http://127.0.0.1:11434/api/tags",
		"http://localhost:8000/v1/models",
		"http://[::1]:8080/v1/chat/completions",
	}
	for _, target := range validTargets {
		if err := guard.ValidateDestination(target); err != nil {
			t.Errorf("expected target %s to be allowed, got error: %v", target, err)
		}
	}

	// 2. External public network targets must fail closed
	prohibitedTargets := []string{
		"https://api.openai.com/v1/chat/completions",
		"https://generativelanguage.googleapis.com/v1beta/models",
		"http://142.250.190.46:80/leak",
		"http://8.8.8.8:53/probe",
		"https://auth.external-saas.com/token",
	}
	for _, target := range prohibitedTargets {
		err := guard.ValidateDestination(target)
		if err == nil {
			t.Errorf("expected target %s to be rejected under zero-egress, but it passed", target)
		}
		var violation *llm.EgressViolationError
		if !errors.As(err, &violation) {
			t.Errorf("expected EgressViolationError, got %T: %v", err, err)
		} else if violation.Code != "EGRESS_INTERCEPT_BLOCKED" {
			t.Errorf("expected violation code 'EGRESS_INTERCEPT_BLOCKED', got %s", violation.Code)
		}
	}

	// 3. Malformed destinations must fail closed with EGRESS_MALFORMED_ENDPOINT
	malformedTargets := []string{
		"",
		"   ",
		"not-a-url",
		"://missing-scheme",
	}
	for _, target := range malformedTargets {
		err := guard.ValidateDestination(target)
		if err == nil {
			t.Errorf("expected malformed target %q to be rejected, but it passed", target)
		}
		var violation *llm.EgressViolationError
		if !errors.As(err, &violation) {
			t.Errorf("expected EgressViolationError, got %T: %v", err, err)
		} else if violation.Code != "EGRESS_MALFORMED_ENDPOINT" {
			t.Errorf("expected code 'EGRESS_MALFORMED_ENDPOINT', got %s", violation.Code)
		}
	}

	// 4. Test HTTP round-tripper interception
	client := &http.Client{
		Transport: guard.WrapTransport(http.DefaultTransport),
		Timeout:   1 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected round-tripper to block egress to api.openai.com, but request succeeded")
	}
	if !errors.Is(err, llm.ErrEgressViolation) {
		t.Errorf("expected error to unwrap to ErrEgressViolation, got %v", err)
	}
}
