# Screen Capture & Terminal Automation Guide

This guide explains how to use ZQK's built-in terminal automator and visual capture engine (`pkg/screencap`) to record automated terminal sessions, generate visual verification artifacts, and drive interactive CLI testing.

---

## 1. When to Use

1. **Automated Documentation & Walkthroughs**: Capture crisp screenshots of terminal commands, dashboards, and TUI outputs directly during CI or script runs.
2. **Visual QA Verification**: Fulfill visual evidence criteria (`verification_matrix`) by automating a sequence of user inputs and capturing window or screen state.
3. **Animated Terminal Transcripts**: Record millisecond-accurate timeline event logs for replay or SVG export.

---

## 2. Terminal Automation Engine (`TerminalAutomator`)

The `TerminalAutomator` wraps standard OS processes with interactive pseudo-terminal controls:

```go
package main

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/screencap"
)

func main() {
	capturer := screencap.NewMacCapturer()
	automator, err := screencap.NewTerminalAutomator(capturer, "zqk", "tui")
	if err != nil {
		panic(err)
	}

	if err := automator.Start(); err != nil {
		panic(err)
	}

	// Wait for TUI initialization
	ctx := context.Background()
	_ = automator.WaitForOutput("Dashboard", 5*time.Second)

	// Simulate user typing a key
	_ = automator.TypeCommand("q")

	// Snapshot
	_ = capturer.CaptureScreen(ctx, "dashboard.png")
}
```

---

## 3. Screen, Window, and Region Capture

The `Capturer` interface provides three granularity levels:

| Method | Target | Parameters |
|---|---|---|
| `CaptureScreen(ctx, path)` | Entire display | `path` destination image |
| `CaptureWindow(ctx, windowID, path)` | Targeted window | OS Window ID string, `path` |
| `CaptureRegion(ctx, x, y, w, h, path)` | Rectangular bounding box | Pixel coordinates $(x, y)$, dimensions $(w, h)$ |

---

## 4. Platform Considerations

- **macOS**: Native implementation uses `/usr/sbin/screencapture` with `-x` flag (suppressing audio shutter clicks).
- **Linux/X11/Wayland**: Extensible via `screencap.CommandRunner` interface to wrap `import` (ImageMagick) or `grim`.
- **Headless CI**: When running in headless virtualized containers, ensure a virtual framebuffer (like `xvfb`) is configured if capturing GUI windows.
