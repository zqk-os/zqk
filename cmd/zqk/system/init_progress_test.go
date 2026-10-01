package system

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestDetectLegacyCodebase(t *testing.T) {
	// 1. Pure empty dir
	dir1 := t.TempDir()
	if detectLegacyCodebase(dir1) {
		t.Errorf("expected empty dir to be greenfield (not legacy)")
	}

	// 2. Dir with only bin/zqk
	dir2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir2, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "bin", "zqk"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if detectLegacyCodebase(dir2) {
		t.Errorf("expected dir with only bin/zqk to be greenfield (not legacy)")
	}

	// 3. Dir with bin/zqk, .gitignore, and ANTIGRAVITY.md
	if err := os.WriteFile(filepath.Join(dir2, ".gitignore"), []byte(".zqk/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "ANTIGRAVITY.md"), []byte("# Rules\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if detectLegacyCodebase(dir2) {
		t.Errorf("expected dir with bin/zqk, .gitignore, ANTIGRAVITY.md to be greenfield (not legacy)")
	}

	// 4. Dir with actual source code (e.g. main.go)
	dir3 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir3, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !detectLegacyCodebase(dir3) {
		t.Errorf("expected dir with main.go to be detected as legacy codebase")
	}

	// 5. Dir with bin/ containing non-zqk binary/script
	dir4 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir4, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir4, "bin", "custom.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if !detectLegacyCodebase(dir4) {
		t.Errorf("expected dir with custom script in bin/ to be detected as legacy codebase")
	}
}

func TestInit_AllStepsProgressSequencing(t *testing.T) {
	var buf bytes.Buffer
	var recordedSteps []int
	ctx := pkgctx.WithValidationProgress(context.Background(), func(stage, msg string) {
		for i := 1; i <= 7; i++ {
			prefix := fmt.Sprintf("[%d/7]", i)
			if strings.Contains(msg, prefix) {
				recordedSteps = append(recordedSteps, i)
			}
		}
	})

	p := newInitProgress(ctx, &buf, true)
	p.Header("step-test", InitModeLegacy)
	for i := 1; i <= 7; i++ {
		p.Step(i, 7, fmt.Sprintf("Stage %d", i))
	}
	p.Done()

	out := buf.String()
	for i := 1; i <= 7; i++ {
		expectedStep := fmt.Sprintf("[%d/7] Stage %d", i, i)
		if !strings.Contains(out, expectedStep) {
			t.Errorf("expected output to contain %q, but got:\n%s", expectedStep, out)
		}
	}

	if len(recordedSteps) != 7 {
		t.Errorf("expected 7 progress callbacks, got %d: %v", len(recordedSteps), recordedSteps)
	}
}
