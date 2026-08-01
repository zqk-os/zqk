package primaryorch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestLoadBinding_DefaultWhenMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv(zqkenv.AgentID(), "")
	b, err := LoadBinding(root)
	if err != nil {
		t.Fatal(err)
	}
	if b.Adapter != AdapterAgentChat || b.AgentID != DefaultAgentIDFallback {
		t.Fatalf("unexpected default: %+v", b)
	}
}

func TestDefaultBinding_UsesEnvAgentID(t *testing.T) {
	t.Setenv(zqkenv.AgentID(), "my-host-tpm")
	b := DefaultBinding()
	if b.AgentID != "my-host-tpm" {
		t.Fatalf("AgentID=%q want my-host-tpm", b.AgentID)
	}
}

func TestWakePrimary_AgentChat(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := WriteBinding(root, Binding{
		SchemaVersion: SchemaVersion,
		AgentID:       "test-orchestrator",
		Adapter:       AdapterAgentChat,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := WakePrimary(context.Background(), root, WakeRequest{
		TaskID:  "ATK-TEST",
		Persona: PersonaTPM,
		Message: "investigate plan error",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Adapter != AdapterAgentChat || res.AgentID != "test-orchestrator" {
		t.Fatalf("result: %+v", res)
	}
	if !strings.Contains(res.DeliveredTo, "agent_chat:") {
		t.Fatalf("delivered_to: %s", res.DeliveredTo)
	}
}

func TestWakePrimary_ScriptAdapter(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scripts := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scripts, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "wake.log")
	script := filepath.Join(scripts, "wake-vendor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\necho \"$@\" >> \""+logPath+"\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := WriteBinding(root, Binding{
		AgentID: "antigravity",
		Adapter: AdapterScript,
		Script:  "scripts/wake-vendor.sh",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := WakePrimary(context.Background(), root, WakeRequest{TaskID: "PLN-1", Message: "hello PLN-1"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "PLN-1") {
		t.Fatalf("log: %s", raw)
	}
}

func TestWakePrimary_UnknownAdapter(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := ConfigPath(root)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]string{"agent_id": "x", "adapter": "telepathy"}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := WakePrimary(context.Background(), root, WakeRequest{Message: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}
