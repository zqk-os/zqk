// Traceability: ITEM-EXAMPLE

package screencap

import (
	"context"
	"testing"
	"time"
)

// mockCapturer implements Capturer for testing.
type mockCapturer struct {
	capturedScreens []string
}

func (m *mockCapturer) CaptureWindow(ctx context.Context, windowID string, outputPath string) error {
	return nil
}

func (m *mockCapturer) CaptureRegion(ctx context.Context, x, y, width, height int, outputPath string) error {
	return nil
}

func (m *mockCapturer) CaptureScreen(ctx context.Context, outputPath string) error {
	m.capturedScreens = append(m.capturedScreens, outputPath)
	return nil
}

func TestTerminalAutomator(t *testing.T) {
	capturer := &mockCapturer{}

	// We'll run a simple bash script that echoes a prompt, waits for input,
	// and echoes a response.
	// bash -c "echo 'ready> '; read input; echo \"got: $input\""

	// Better yet, just start a bash process, send commands, and read output.
	automator, err := NewTerminalAutomator(capturer, "bash")
	if err != nil {
		t.Fatalf("failed to create automator: %v", err)
	}

	if err := automator.Start(); err != nil {
		t.Fatalf("failed to start automator: %v", err)
	}
	defer automator.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Type a command
	if err := automator.TypeCommand("echo 'hello from test'"); err != nil {
		t.Fatalf("failed to type command: %v", err)
	}

	// Wait for the output
	if err := automator.WaitForOutput(ctx, "hello from test"); err != nil {
		t.Fatalf("failed to wait for output: %v", err)
	}

	// Test TypeCommandAndCapture
	if err := automator.TypeCommandAndCapture(ctx, "echo 'capture this'", "test1.png"); err != nil {
		t.Fatalf("failed to type and capture: %v", err)
	}

	// Test WaitForOutputAndCapture
	if err := automator.WaitForOutputAndCapture(ctx, "capture this", "test2.png"); err != nil {
		t.Fatalf("failed to wait and capture: %v", err)
	}

	// Exit bash
	if err := automator.TypeCommand("exit"); err != nil {
		t.Fatalf("failed to type exit: %v", err)
	}

	// Verify captures
	if len(capturer.capturedScreens) != 2 {
		t.Fatalf("expected 2 screen captures, got %d", len(capturer.capturedScreens))
	}

	if capturer.capturedScreens[0] != "test1.png" {
		t.Errorf("expected test1.png, got %s", capturer.capturedScreens[0])
	}
	if capturer.capturedScreens[1] != "test2.png" {
		t.Errorf("expected test2.png, got %s", capturer.capturedScreens[1])
	}
}
