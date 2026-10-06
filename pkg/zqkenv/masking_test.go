package zqkenv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestMaskSensitiveValue(t *testing.T) {
	assert.Equal(t, "******", zqkenv.MaskSensitiveValue(zqkenv.APIKey().Name(), "secret-token-123"))
	assert.Equal(t, "******", zqkenv.MaskSensitiveValue("DB_PASSWORD", "supersecret"))
	assert.Equal(t, "Bearer ******", zqkenv.MaskSensitiveValue("CUSTOM_HEADER", "Bearer abcdef123456"))
	assert.Equal(t, "plain_val", zqkenv.MaskSensitiveValue("PUBLIC_PATH", "plain_val"))
}

func TestSanitizeEnvironment(t *testing.T) {
	input := []string{
		zqkenv.APIKey().Name() + "=secret_key",
		"PUBLIC_VAR=hello",
		"AUTH_TOKEN=Bearer 12345",
	}
	sanitized := zqkenv.SanitizeEnvironment(input)
	assert.Equal(t, zqkenv.APIKey().Name()+"=******", sanitized[0])
	assert.Equal(t, "PUBLIC_VAR=hello", sanitized[1])
	assert.Equal(t, "AUTH_TOKEN=******", sanitized[2])
}

func TestSanitizeFields(t *testing.T) {
	fields := map[string]any{
		"token":      "sensitive-abc-123",
		"password":   "super-secret",
		"account_id": "ACC-12345678",
		"details": map[string]any{
			"api_key": "nested-secret",
			"status":  "ok",
		},
	}
	sanitized := zqkenv.SanitizeFields(fields)
	assert.Equal(t, "******", sanitized["token"])
	assert.Equal(t, "******", sanitized["password"])
	assert.Equal(t, "ACC-12345678", sanitized["account_id"])
	nested, ok := sanitized["details"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "******", nested["api_key"])
	assert.Equal(t, "ok", nested["status"])
}
