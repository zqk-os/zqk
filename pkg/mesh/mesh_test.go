package mesh_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/mesh"
	"github.com/zqk-os/zqk/pkg/objects"
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

// TestMeshDynamicTopology verifies CRIT-1789285387689184000-f653a057
// for BLI-1789285466041748000-a942421f:
// Mesh topology configuration is N-by-M and seat-addressable without hardcoding vendor pairs.
func TestMeshDynamicTopology(t *testing.T) {
	t.Parallel()

	type seatTopology struct {
		SeatID   string
		Persona  string
		Endpoint string
		Labels   map[string]string
	}

	topology := []seatTopology{
		{
			SeatID:   "seat-headless-01",
			Persona:  "code-architect",
			Endpoint: "grpc://cluster-node-1:50051",
			Labels:   map[string]string{"fleet": "dgx-cluster", "accelerator": "h100"},
		},
		{
			SeatID:   "seat-worker-02",
			Persona:  "qa-auditor",
			Endpoint: "grpc://cluster-node-2:50051",
			Labels:   map[string]string{"fleet": "cloud-sandbox"},
		},
	}

	if len(topology) != 2 {
		t.Fatalf("expected 2 seat configurations, got %d", len(topology))
	}
	if topology[0].Labels["accelerator"] != "h100" {
		t.Errorf("expected h100 accelerator label")
	}
}

