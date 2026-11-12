package community

import (
	"context"
	"errors"
	"testing"
)

func TestConfigValidation_SchemaCompliance(t *testing.T) {
	validator := NewProviderConfigValidator()
	validator.RegisterSchema(ProviderConfigSchema{
		ProviderType: "openai-adapter",
		Properties: map[string]ProviderConfigProperty{
			"endpoint": {Type: "string", Required: true, MinVal: 8},
			"timeout":  {Type: "int", Required: true, MinVal: 1, MaxVal: 60},
			"enabled":  {Type: "bool", Required: false},
		},
	})

	validConfig := map[string]any{
		"endpoint": "https://api.openai.com/v1",
		"timeout":  30,
		"enabled":  true,
	}

	if err := validator.Validate("openai-adapter", validConfig); err != nil {
		t.Fatalf("expected valid config to pass validation, got: %v", err)
	}
}

func TestConfigValidation_SecretInjectionAndMasking(t *testing.T) {
	store := NewProviderSecretStore()
	store.SetSecret("provider-1", "api_key", "sk-proj-1234567890abcdef")

	ctx := context.Background()
	val, err := store.GetSecret(ctx, "provider-1", "api_key")
	if err != nil {
		t.Fatalf("expected to resolve secret, got: %v", err)
	}
	if val != "sk-proj-1234567890abcdef" {
		t.Fatalf("unexpected secret value: %s", val)
	}

	masked := MaskSecret(val)
	if masked != "sk****ef" {
		t.Fatalf("unexpected masked format: %s", masked)
	}

	shortMasked := MaskSecret("abc")
	if shortMasked != "****" {
		t.Fatalf("unexpected short masked format: %s", shortMasked)
	}
}

func TestConfigValidation_RejectionsAndBoundaries(t *testing.T) {
	validator := NewProviderConfigValidator()
	validator.RegisterSchema(ProviderConfigSchema{
		ProviderType: "local-llm",
		Properties: map[string]ProviderConfigProperty{
			"host":       {Type: "string", Required: true},
			"max_tokens": {Type: "int", Required: true, MinVal: 10, MaxVal: 4096},
		},
	})

	// Missing required field
	err := validator.Validate("local-llm", map[string]any{"max_tokens": 100})
	if !errors.Is(err, ErrMissingRequiredConfig) {
		t.Fatalf("expected ErrMissingRequiredConfig, got: %v", err)
	}

	// Below min integer bound
	err = validator.Validate("local-llm", map[string]any{"host": "localhost", "max_tokens": 5})
	if !errors.Is(err, ErrInvalidConfigSchema) {
		t.Fatalf("expected ErrInvalidConfigSchema for out of bounds int, got: %v", err)
	}

	// Unknown provider type
	err = validator.Validate("unknown-provider", map[string]any{})
	if !errors.Is(err, ErrInvalidConfigSchema) {
		t.Fatalf("expected ErrInvalidConfigSchema for unknown provider type, got: %v", err)
	}

	// Secret not found
	store := NewProviderSecretStore()
	_, err = store.GetSecret(context.Background(), "provider-unknown", "token")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got: %v", err)
	}
}

func TestConfigValidation_ContextCancellation(t *testing.T) {
	store := NewProviderSecretStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.GetSecret(ctx, "provider-1", "key")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
