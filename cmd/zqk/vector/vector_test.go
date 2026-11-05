package vector

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/events"
)

func TestVectorCmd_List(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewVectorCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"list"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("vector list failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "VEC-SECURITY") {
		t.Fatalf("expected output to contain VEC-SECURITY, got:\n%s", out)
	}
	if !strings.Contains(out, "VEC-RELIABILITY") {
		t.Fatalf("expected output to contain VEC-RELIABILITY, got:\n%s", out)
	}
}

func TestVectorCmd_ListPrioritized(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewVectorCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"list", "--prioritized"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("vector list --prioritized failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "VECTOR ID") {
		t.Fatalf("expected table header, got:\n%s", out)
	}
}

func TestVectorCmd_ListJSON(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewVectorCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"list", "-f", "json"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("vector list -f json failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"id": "VEC-SECURITY"`) {
		t.Fatalf("expected JSON array with VEC-SECURITY, got:\n%s", out)
	}
}

func TestVectorCmd_Evaluate(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	bus := events.GetDefaultBus()
	var evaluatedReceived bool
	bus.Subscribe("vector.evaluated", func(ctx context.Context, topic string, payload any) error {
		evaluatedReceived = true
		return nil
	})

	cmd := NewVectorCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"evaluate", "--all"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("vector evaluate failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Vector Evaluation Results") {
		t.Fatalf("expected evaluation header, got:\n%s", out)
	}
	if !strings.Contains(out, "Overall Weighted Health Score") {
		t.Fatalf("expected overall score, got:\n%s", out)
	}
	if !evaluatedReceived {
		t.Fatalf("expected vector.evaluated event to be dispatched")
	}
}

func TestVectorCmd_EvaluateJSON(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewVectorCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"evaluate", "-f", "json"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("vector evaluate json failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"overall_health_score"`) {
		t.Fatalf("expected overall_health_score in JSON, got:\n%s", out)
	}
}

func TestVectorCmd_SteerDelta(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	bus := events.GetDefaultBus()
	var steeredReceived bool
	bus.Subscribe("vector.steered", func(ctx context.Context, topic string, payload any) error {
		steeredReceived = true
		return nil
	})

	cmd := NewVectorCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"steer", "VEC-SECURITY=+0.05"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("vector steer failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Vector Steered Successfully") {
		t.Fatalf("expected success message, got:\n%s", out)
	}
	if !steeredReceived {
		t.Fatalf("expected vector.steered event to be dispatched")
	}
}

func TestVectorCmd_SteerNaturalLanguage(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewVectorCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"steer", "prioritize reliability strictly"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("vector steer natural language failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "VEC-RELIABILITY") {
		t.Fatalf("expected VEC-RELIABILITY to be steered, got:\n%s", out)
	}
}
