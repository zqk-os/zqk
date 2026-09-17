package zqkenv_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/stretchr/testify/assert"
)

func TestMaskSensitiveValue(t *testing.T) {
	assert.Equal(t, "******", zqkenv.MaskSensitiveValue("ZQK_API_KEY", "secret-token-123"))
	assert.Equal(t, "******", zqkenv.MaskSensitiveValue("DB_PASSWORD", "supersecret"))
	assert.Equal(t, "Bearer ******", zqkenv.MaskSensitiveValue("CUSTOM_HEADER", "Bearer abcdef123456"))
	assert.Equal(t, "plain_val", zqkenv.MaskSensitiveValue("PUBLIC_PATH", "plain_val"))
}

func TestSanitizeEnvironment(t *testing.T) {
	input := []string{
		"ZQK_API_KEY=secret_key",
		"PUBLIC_VAR=hello",
		"AUTH_TOKEN=Bearer 12345",
	}
	sanitized := zqkenv.SanitizeEnvironment(input)
	assert.Equal(t, "ZQK_API_KEY=******", sanitized[0])
	assert.Equal(t, "PUBLIC_VAR=hello", sanitized[1])
	assert.Equal(t, "AUTH_TOKEN=******", sanitized[2])
}
