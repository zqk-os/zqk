// Traceability: [REDACTED-ID]

package screencap

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/execwrap"
)

// Capturer defines the interface for taking screenshots.
type Capturer interface {
	CaptureWindow(ctx context.Context, windowID string, outputPath string) error
	CaptureRegion(ctx context.Context, x, y, width, height int, outputPath string) error
	CaptureScreen(ctx context.Context, outputPath string) error
}

// commandRunner abstracts os/exec for testing.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type defaultRunner struct{}

func (d *defaultRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := execwrap.CommandContext(ctx, name, args...)
	return cmd.Run()
}

// MacCapturer implements Capturer for macOS using the native screencapture utility.
type MacCapturer struct {
	runner commandRunner
}

// NewMacCapturer creates a new MacCapturer using the system command runner.
func NewMacCapturer() *MacCapturer {
	return &MacCapturer{runner: &defaultRunner{}}
}

// CaptureWindow captures a specific window by ID.
func (m *MacCapturer) CaptureWindow(ctx context.Context, windowID string, outputPath string) error {
	// -x: do not play sounds
	// -l<windowid>: capture this window id
	return m.runner.Run(ctx, "screencapture", "-x", "-l"+windowID, outputPath)
}

// CaptureRegion captures a specific screen rect.
func (m *MacCapturer) CaptureRegion(ctx context.Context, x, y, width, height int, outputPath string) error {
	// -x: do not play sounds
	// -R<x,y,w,h>: capture screen rect
	rect := fmt.Sprintf("%d,%d,%d,%d", x, y, width, height)
	return m.runner.Run(ctx, "screencapture", "-x", "-R"+rect, outputPath)
}

// CaptureScreen captures the entire screen.
func (m *MacCapturer) CaptureScreen(ctx context.Context, outputPath string) error {
	// -x: do not play sounds
	return m.runner.Run(ctx, "screencapture", "-x", outputPath)
}
