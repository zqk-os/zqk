package mesh_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/mesh"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestInMemoryMesh(t *testing.T) {
	m := mesh.NewInMemoryMesh()
	ctx := context.Background()

	cap1 := mesh.CapacityAdvertisement{
		KernelID: "kernel-1",
		Compute:  10.5,
	}

	err := m.BroadcastCapacity(ctx, cap1)
	if err != nil {
		t.Fatalf("unexpected error broadcasting capacity: %v", err)
	}

	caps := m.GetCapacities()
	if len(caps) != 1 {
		t.Fatalf("expected 1 capacity, got %d", len(caps))
	}
	if caps[0].KernelID != "kernel-1" {
		t.Errorf("expected kernel-1, got %s", caps[0].KernelID)
	}
}

func TestInMemoryMesh_ToolPods(t *testing.T) {
	m := mesh.NewInMemoryMesh()
	ctx := context.Background()

	tp := mesh.ToolPodRegistration{
		ID:          "tp-1",
		Type:        "ffmpeg",
		Endpoint:    "http://localhost:8081",
		Description: "FFmpeg worker",
		Metadata:    map[string]string{objects.FieldKeyVersion: "1.0"},
	}

	err := m.RegisterToolPod(ctx, tp)
	if err != nil {
		t.Fatalf("unexpected error registering tool pod: %v", err)
	}

	pods, err := m.DiscoverToolPods(ctx)
	if err != nil {
		t.Fatalf("unexpected error discovering tool pods: %v", err)
	}

	if len(pods) != 1 {
		t.Fatalf("expected 1 tool pod, got %d", len(pods))
	}

	if pods[0].ID != "tp-1" {
		t.Errorf("expected tp-1, got %s", pods[0].ID)
	}
}
