package envelope_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/envelope"
	"github.com/stretchr/testify/assert"
)

func TestParseTDEEnvelope(t *testing.T) {
	validJSON := []byte(`{
		"id": "env-123",
		"kind": "tde_envelope",
		"status": "pending",
		"agent_id": "agent-xyz",
		"intent": "mutate filesystem",
		"payload": "base64-payload"
	}`)

	env, err := envelope.ParseTDEEnvelope(validJSON)
	assert.NoError(t, err)
	assert.NotNil(t, env)
	assert.Equal(t, "env-123", env.ID)
	assert.Equal(t, envelope.StatusPending, env.Status)

	invalidJSON := []byte(`{
		"id": "env-123",
		"kind": "tde_envelope"
	}`)
	_, err = envelope.ParseTDEEnvelope(invalidJSON)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field: status")
}
