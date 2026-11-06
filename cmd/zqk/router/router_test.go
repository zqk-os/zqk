package router

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	routerpkg "github.com/zqk-os/zqk/pkg/router"
)

func TestNewRouterCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := NewRouterCmd()
	if cmd.Use != "router" {
		t.Errorf("expected use 'router', got %q", cmd.Use)
	}

	evalCmd, _, err := cmd.Find([]string{"eval"})
	if err != nil || evalCmd == nil {
		t.Fatalf("expected 'eval' subcommand to be present")
	}

	statsCmd, _, err := cmd.Find([]string{"stats"})
	if err != nil || statsCmd == nil {
		t.Fatalf("expected 'stats' subcommand to be present")
	}
}

func TestRouterCmd_Eval_JSON(t *testing.T) {
	t.Parallel()

	cmd := NewRouterCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"eval", "grep -rn 'func' pkg/", "--token-budget", "1200", "-f", "json"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("expected eval to succeed, got %v", err)
	}

	var decision routerpkg.RouteDecision
	if err := json.Unmarshal(buf.Bytes(), &decision); err != nil {
		t.Fatalf("failed to parse json output: %v, raw: %s", err, buf.String())
	}

	if !decision.Intercepted {
		t.Errorf("expected Intercepted=true, got %v", decision.Intercepted)
	}
	if decision.TargetTool != "zqk grep" {
		t.Errorf("expected TargetTool='zqk grep', got %q", decision.TargetTool)
	}
}

func TestRouterCmd_Eval_Text(t *testing.T) {
	t.Parallel()

	cmd := NewRouterCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"eval", "cat README.md", "-f", "text"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("expected eval to succeed, got %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Action:") || !strings.Contains(out, "Target:") {
		t.Errorf("expected text output with Action and Target, got %q", out)
	}
}

func TestRouterCmd_Stats(t *testing.T) {
	t.Parallel()

	// JSON format
	cmd := NewRouterCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"stats", "-f", "json"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("expected stats to succeed, got %v", err)
	}

	var stats routerpkg.RouterStats
	if err := json.Unmarshal(buf.Bytes(), &stats); err != nil {
		t.Fatalf("failed to parse stats json: %v", err)
	}

	// Text format
	buf.Reset()
	cmd.SetArgs([]string{"stats", "-f", "text"})
	err = cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("expected stats text to succeed, got %v", err)
	}
	if !strings.Contains(buf.String(), "Model Requests:") {
		t.Errorf("expected text output containing 'Model Requests:', got %q", buf.String())
	}
}
