package mcp

import (
	"context"
	"encoding/json"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageProcessor_ActorContextExtraction(t *testing.T) {
	t.Parallel()
	req := &JSONRPCRequest{
		Method: "tools/call",
		Params: json.RawMessage(`{
			"name": "example_tool",
			"arguments": {},
			"_meta": {
				"account_id": "account:test-actor",
				"actor_type": "user"
			}
		}`),
	}

	ctx := context.Background()
	extractedCtx := ExtractActorContext(ctx, req.Params)

	require.NotNil(t, extractedCtx)
	retrievedSecCtx := pkgctx.GetSecurityContext(extractedCtx)
	require.NotNil(t, retrievedSecCtx)
	assert.Equal(t, "account:test-actor", retrievedSecCtx.AccountID)
}

func TestExtractActorContextFromArgs(t *testing.T) {
	t.Parallel()
	args := map[string]any{
		objects.FieldKeyName: "example_tool",
		"_meta": map[string]any{
			objects.FieldKeyAccountID: "account:args-actor",
			"actor_type":              "user",
		},
	}

	ctx := context.Background()
	extractedCtx := ExtractActorContextFromArgs(ctx, args)

	require.NotNil(t, extractedCtx)
	retrievedSecCtx := pkgctx.GetSecurityContext(extractedCtx)
	require.NotNil(t, retrievedSecCtx)
	assert.Equal(t, "account:args-actor", retrievedSecCtx.AccountID)
}

func TestClientEventContext_LifetimeCounters(t *testing.T) {
	t.Parallel()
	var cecNil *ClientEventContext
	if cecNil.GetClientEventContextStats() != 0 {
		t.Fatalf("expected nil stats 0, got %d", cecNil.GetClientEventContextStats())
	}

	server := NewServer()
	cec := NewClientEventContext(server)

	if cec.GetClientEventContextStats() != 0 {
		t.Fatalf("expected initial stats 0, got %d", cec.GetClientEventContextStats())
	}

	cec.sequenceID = "seq-123"
	cec.clientID = "client-456"
	cec.canRecord = true
	cec.computed = true

	cec.RecordEvent("test.event", map[string]any{"key": "val"})

	if cec.GetClientEventContextStats() != 1 {
		t.Fatalf("expected stats 1, got %d", cec.GetClientEventContextStats())
	}
}
