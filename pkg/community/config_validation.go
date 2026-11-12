package community

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrInvalidConfigSchema is returned when configuration fails schema validation.
	ErrInvalidConfigSchema = errors.New("configuration does not conform to required provider schema")
	// ErrMissingRequiredConfig is returned when a mandatory field is missing.
	ErrMissingRequiredConfig = errors.New("missing mandatory provider configuration field")
	// ErrSecretNotFound is returned when a requested secret key is not present in the store.
	ErrSecretNotFound = errors.New("requested secret key not found in provider secret store")
)

// ProviderConfigProperty defines constraints for a single configuration field.
type ProviderConfigProperty struct {
	Type     string // "string", "int", "bool"
	Required bool
	MinVal   int
	MaxVal   int
}

// ProviderConfigSchema defines expected configuration fields and constraints for a provider.
type ProviderConfigSchema struct {
	ProviderType string
	Properties   map[string]ProviderConfigProperty
}

// ProviderConfigValidator validates raw provider configurations against registered schemas.
type ProviderConfigValidator struct {
	mu      sync.RWMutex
	schemas map[string]ProviderConfigSchema
}

// NewProviderConfigValidator initializes a configuration validator.
func NewProviderConfigValidator() *ProviderConfigValidator {
	return &ProviderConfigValidator{
		schemas: make(map[string]ProviderConfigSchema),
	}
}

// RegisterSchema registers a configuration schema for a specific provider type.
func (v *ProviderConfigValidator) RegisterSchema(schema ProviderConfigSchema) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.schemas[schema.ProviderType] = schema
}

// Validate checks whether the provided configuration map complies with the registered schema.
func (v *ProviderConfigValidator) Validate(providerType string, config map[string]any) error {
	v.mu.RLock()
	schema, ok := v.schemas[providerType]
	v.mu.RUnlock()

	if !ok {
		return fmt.Errorf("%w: unknown provider type %q", ErrInvalidConfigSchema, providerType)
	}

	for key, prop := range schema.Properties {
		val, exists := config[key]
		if !exists || val == nil {
			if prop.Required {
				return fmt.Errorf("%w: field %q is required", ErrMissingRequiredConfig, key)
			}
			continue
		}

		switch prop.Type {
		case "string":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("%w: field %q expected string, got %T", ErrInvalidConfigSchema, key, val)
			}
			if prop.MinVal > 0 && len(s) < prop.MinVal {
				return fmt.Errorf("%w: field %q length %d below minimum %d", ErrInvalidConfigSchema, key, len(s), prop.MinVal)
			}
		case "int":
			var intVal int
			switch v := val.(type) {
			case int:
				intVal = v
			case int64:
				intVal = int(v)
			case float64:
				intVal = int(v)
			default:
				return fmt.Errorf("%w: field %q expected integer, got %T", ErrInvalidConfigSchema, key, val)
			}

			if prop.MinVal != 0 && intVal < prop.MinVal {
				return fmt.Errorf("%w: field %q value %d below minimum %d", ErrInvalidConfigSchema, key, intVal, prop.MinVal)
			}
			if prop.MaxVal != 0 && intVal > prop.MaxVal {
				return fmt.Errorf("%w: field %q value %d exceeds maximum %d", ErrInvalidConfigSchema, key, intVal, prop.MaxVal)
			}
		case "bool":
			if _, ok := val.(bool); !ok {
				return fmt.Errorf("%w: field %q expected boolean, got %T", ErrInvalidConfigSchema, key, val)
			}
		}
	}

	return nil
}

// ProviderSecretStore manages scoped runtime credentials for providers.
type ProviderSecretStore struct {
	mu      sync.RWMutex
	secrets map[string]map[string]string
}

// NewProviderSecretStore initializes an in-memory secret store.
func NewProviderSecretStore() *ProviderSecretStore {
	return &ProviderSecretStore{
		secrets: make(map[string]map[string]string),
	}
}

// SetSecret stores a scoped secret for a provider.
func (ss *ProviderSecretStore) SetSecret(providerID, key, value string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if _, ok := ss.secrets[providerID]; !ok {
		ss.secrets[providerID] = make(map[string]string)
	}
	ss.secrets[providerID][key] = value
}

// GetSecret resolves a scoped secret for a provider.
func (ss *ProviderSecretStore) GetSecret(ctx context.Context, providerID, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	ss.mu.RLock()
	defer ss.mu.RUnlock()

	provSecrets, ok := ss.secrets[providerID]
	if !ok {
		return "", fmt.Errorf("%w: provider %q has no secrets configured", ErrSecretNotFound, providerID)
	}

	val, ok := provSecrets[key]
	if !ok {
		return "", fmt.Errorf("%w: key %q not found for provider %q", ErrSecretNotFound, key, providerID)
	}

	return val, nil
}

// MaskSecret returns a masked representation of a secret value for safe logging.
func MaskSecret(secret string) string {
	if len(secret) <= 4 {
		return "****"
	}
	return secret[:2] + "****" + secret[len(secret)-2:]
}
