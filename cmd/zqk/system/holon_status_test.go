package system

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/ambient"
)

func TestHolonStatusCommand_HumanOutput(t *testing.T) {
	var buf bytes.Buffer
	err := runHolonStatus(&buf, false)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Middle-Brain Holon Vitality & Telemetry Dashboard") {
		t.Fatalf("expected dashboard banner in output, got: %s", out)
	}
	if !strings.Contains(out, "ambient-stream-hydrator") {
		t.Fatalf("expected hydrator holon in output, got: %s", out)
	}
	if !strings.Contains(out, "ambient-river-convergence") {
		t.Fatalf("expected river engine in output, got: %s", out)
	}
}

func TestHolonStatusCommand_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	err := runHolonStatus(&buf, true)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	var snap ambient.MiddleBrainTelemetrySnapshot
	if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
		t.Fatalf("failed to decode JSON output: %v", err)
	}

	if snap.ActiveHolonCount < 2 {
		t.Fatalf("expected at least 2 active holons, got %d", snap.ActiveHolonCount)
	}
	if snap.HealthStatus == "" {
		t.Fatal("expected non-empty health status")
	}
}

func TestNewHolonStatusCmd(t *testing.T) {
	cmd := NewHolonStatusCmd()
	if cmd.Use != "holon-status" {
		t.Fatalf("expected use 'holon-status', got %s", cmd.Use)
	}
	flag := cmd.Flags().Lookup("json")
	if flag == nil {
		t.Fatal("expected 'json' flag on command")
	}
}
