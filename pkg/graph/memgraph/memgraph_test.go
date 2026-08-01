package memgraph

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	ctx := context.Background()
	// Using a dummy URI. The driver creation will not fail immediately,
	// but the Ping should fail because no database is listening.
	client, err := NewClient("bolt://localhost:1234", "test", "test")
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	defer client.Close(ctx)

	err = client.Ping(ctx)
	if err == nil {
		t.Fatal("expected ping to fail with no db running, got nil")
	}
}
