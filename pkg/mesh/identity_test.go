package mesh_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/mesh"
)

func TestInMemoryRegistry(t *testing.T) {
	reg := mesh.NewInMemoryRegistry()
	ctx := context.Background()

	kernel := mesh.RemoteKernel{
		ID:        "kernel-1",
		PublicKey: "0123456789abcdef",
		Endpoint:  "https://kernel.local",
	}

	err := reg.Register(ctx, kernel)
	if err != nil {
		t.Fatalf("unexpected error registering kernel: %v", err)
	}

	found, err := reg.Verify(ctx, "kernel-1")
	if err != nil {
		t.Fatalf("unexpected error verifying kernel: %v", err)
	}
	if found.ID != kernel.ID {
		t.Errorf("expected %s, got %s", kernel.ID, found.ID)
	}

	_, err = reg.Verify(ctx, "unknown")
	if err == nil {
		t.Error("expected error verifying unknown kernel, got nil")
	}
	if !errors.Is(err, mesh.ErrKernelNotFound) {
		t.Errorf("expected ErrKernelNotFound, got %v", err)
	}
}
