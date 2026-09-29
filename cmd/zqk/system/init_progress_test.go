package system

import (
	"bytes"
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestInitProgress_Interactive(t *testing.T) {
	var buf bytes.Buffer
	var lastStage, lastMsg string
	progressCallback := func(stage, msg string) {
		lastStage = stage
		lastMsg = msg
	}

	ctx := pkgctx.WithValidationProgress(context.Background(), progressCallback)
	p := newInitProgress(ctx, &buf, true)

	p.Header("my-project", InitModeGreenfield)
	p.Step(1, 7, "Preparing workspace")
	p.SubStep("Creating directories")
	p.Done()
	p.Summary("/tmp/my-project")

	out := buf.String()
	if !strings.Contains(out, "my-project") {
		t.Errorf("expected output to contain project name, got: %s", out)
	}
	if !strings.Contains(out, "[1/7] Preparing workspace") {
		t.Errorf("expected step output, got: %s", out)
	}
	if !strings.Contains(out, "↳ Creating directories") {
		t.Errorf("expected substep output, got: %s", out)
	}
	if !strings.Contains(out, "initialized successfully") {
		t.Errorf("expected summary, got: %s", out)
	}

	// Verify callback notification into existing coordinator / async progress pattern
	if lastStage != "Preparing workspace" {
		t.Errorf("expected lastStage 'Preparing workspace', got: %s", lastStage)
	}
	if lastMsg != "Creating directories" {
		t.Errorf("expected lastMsg 'Creating directories', got: %s", lastMsg)
	}
}

func TestInitProgress_NonInteractive(t *testing.T) {
	var buf bytes.Buffer
	var called bool
	ctx := pkgctx.WithValidationProgress(context.Background(), func(stage, msg string) {
		called = true
	})

	p := newInitProgress(ctx, &buf, false)
	p.Header("silent-project", InitModeGreenfield)
	p.Step(1, 7, "Silent Stage")
	p.SubStep("Silent SubStep")
	p.Done()
	p.Summary("/tmp/silent")

	// Non-interactive should write nothing to buffer
	if buf.Len() != 0 {
		t.Errorf("expected empty buffer for non-interactive progress, got: %s", buf.String())
	}
	// But coordinator callback MUST still be invoked
	if !called {
		t.Error("expected validation progress callback to be called even in non-interactive mode")
	}
}

func TestInitProgress_NilSafety(t *testing.T) {
	var p *initProgress
	// None of these should panic
	p.Header("nil-test", InitModeGreenfield)
	p.Step(1, 7, "nil")
	p.SubStep("nil")
	p.Done()
	p.Summary("/tmp/nil")
}
