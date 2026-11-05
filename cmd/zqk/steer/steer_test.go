package steer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/events"
)

func TestSteerCmd_Execution(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewSteerCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"prioritize security urgently"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("steer execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Steering Applied") {
		t.Fatalf("expected output to contain 'Steering Applied', got:\n%s", out)
	}
	if !strings.Contains(out, "VEC-SECURITY") {
		t.Fatalf("expected output to mention VEC-SECURITY, got:\n%s", out)
	}
}

func TestSteerCmd_DryRun(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewSteerCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"slightly relax docstrings", "--dry-run"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("steer dry-run execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[Dry Run]") {
		t.Fatalf("expected output to contain '[Dry Run]', got:\n%s", out)
	}
}

func TestSteerCmd_Reset(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewSteerCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"reset"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("steer reset execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Successfully reset") {
		t.Fatalf("expected output to confirm reset, got:\n%s", out)
	}
}

func TestSteerCmd_JsonFormat(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewSteerCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"boost speed and efficiency", "-f", "json"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("steer json format failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"raw_text"`) {
		t.Fatalf("expected valid JSON output, got:\n%s", out)
	}
}

func TestSteerCmd_DispatchesEvent(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	bus := events.GetDefaultBus()
	var received bool
	bus.Subscribe("steering.applied", func(ctx context.Context, topic string, payload any) error {
		received = true
		return nil
	})

	cmd := NewSteerCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"strictly strengthen reliability"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	if !received {
		t.Fatalf("expected steering.applied event to be dispatched")
	}
}

func TestSteerCmd_NoArgsShowsState(t *testing.T) {
	t.Setenv("ZQK_TEST_ROOT", t.TempDir())
	cmd := NewSteerCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("steer no args failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No active steering offsets") {
		t.Fatalf("expected initial message for no active offsets, got:\n%s", out)
	}
}
