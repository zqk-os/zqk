// Traceability: [REDACTED-ID]

package screencap

import (
	"context"
	"strings"
	"testing"
)

// mockRunner captures the commands executed for testing.
type mockRunner struct {
	commandsExecuted [][]string
	errToReturn      error
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) error {
	fullCmd := append([]string{name}, args...)
	m.commandsExecuted = append(m.commandsExecuted, fullCmd)
	return m.errToReturn
}

func TestMacCapturer(t *testing.T) {
	ctx := context.Background()

	t.Run("CaptureWindow", func(t *testing.T) {
		runner := &mockRunner{}
		capturer := NewMacCapturerWithRunner(runner)

		err := capturer.CaptureWindow(ctx, "12345", "output.png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(runner.commandsExecuted) != 1 {
			t.Fatalf("expected 1 command, got %d", len(runner.commandsExecuted))
		}

		cmd := strings.Join(runner.commandsExecuted[0], " ")
		expected := "screencapture -x -l12345 output.png"
		if cmd != expected {
			t.Errorf("expected command %q, got %q", expected, cmd)
		}
	})

	t.Run("CaptureRegion", func(t *testing.T) {
		runner := &mockRunner{}
		capturer := NewMacCapturerWithRunner(runner)

		err := capturer.CaptureRegion(ctx, 10, 20, 100, 200, "region.png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(runner.commandsExecuted) != 1 {
			t.Fatalf("expected 1 command, got %d", len(runner.commandsExecuted))
		}

		cmd := strings.Join(runner.commandsExecuted[0], " ")
		expected := "screencapture -x -R10,20,100,200 region.png"
		if cmd != expected {
			t.Errorf("expected command %q, got %q", expected, cmd)
		}
	})

	t.Run("CaptureScreen", func(t *testing.T) {
		runner := &mockRunner{}
		capturer := NewMacCapturerWithRunner(runner)

		err := capturer.CaptureScreen(ctx, "screen.png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(runner.commandsExecuted) != 1 {
			t.Fatalf("expected 1 command, got %d", len(runner.commandsExecuted))
		}

		cmd := strings.Join(runner.commandsExecuted[0], " ")
		expected := "screencapture -x screen.png"
		if cmd != expected {
			t.Errorf("expected command %q, got %q", expected, cmd)
		}
	})
}
