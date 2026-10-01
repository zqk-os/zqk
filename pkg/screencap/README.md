# ZQK Screen Capture & Terminal Automation Engine (`pkg/screencap`)

`pkg/screencap` provides headless and interactive terminal automation coupled with native screen, window, and rectangular region capture capabilities.

---

## 1. Capabilities

- **Terminal Automation (`TerminalAutomator`)**:
  - Spawns background CLI sessions with bidirectional pipe control (`stdin`, `stdout`, `stderr`).
  - Simulates interactive human typing with millisecond timestamps (`TypeCommand`, `AppendTimelineEvent`).
  - Records real-time timeline event traces (`TimelineEvent`) suitable for rendering animated terminal casts and SVG/GIF demos.
  - Matches live stdout patterns (`WaitForOutput`, `WaitForOutputWithTimeout`) to synchronize execution phases.
- **Visual Capture Engine (`Capturer`)**:
  - `CaptureWindow(ctx, windowID, outputPath)`: Captures a specific application window by native OS window identifier.
  - `CaptureRegion(ctx, x, y, width, height, outputPath)`: Crops and records a designated screen bounding box without capturing full display real-estate.
  - `CaptureScreen(ctx, outputPath)`: Full display snapshot.
  - `MacCapturer`: macOS native implementation utilizing headless `screencapture -x`.

---

## 2. Architecture & Interfaces

```
 ┌──────────────────────┐        TypeCommand()         ┌────────────────────┐
 │  TerminalAutomator   ├─────────────────────────────►│    exec.Cmd Pipe   │
 │                      │                              │ (zqk CLI / shell)  │
 │  - Timeline Tracer   │◄─────────────────────────────┤                    │
 │  - Pattern Matcher   │        WaitForOutput()       └────────────────────┘
 └──────────┬───────────┘
            │
            │ Triggers visual snapshot
            ▼
 ┌──────────────────────┐      screencapture -x        ┌────────────────────┐
 │       Capturer       ├─────────────────────────────►│ Artifacts / Images │
 │   (e.g. MacCapturer) │                              │ (.png, timeline)   │
 └──────────────────────┘                              └────────────────────┘
```

---

## 3. Usage Example

```go
capturer := screencap.NewMacCapturer()
automator, err := screencap.NewTerminalAutomator(capturer, "bash")
if err != nil {
    log.Fatal(err)
}

if err := automator.Start(); err != nil {
    log.Fatal(err)
}

// Type command and snapshot the terminal window
ctx := context.Background()
_ = automator.TypeCommand("zqk workflow whats-next")
_ = automator.WaitForOutput("Priority Plan", 3*time.Second)
_ = capturer.CaptureScreen(ctx, "artifacts/whats_next_output.png")
```

---

## 4. Operational Role in ZQK

`pkg/screencap` is used to:
1. Generate deterministic visual evidence for Quality Assurance (VDS Done-Gates and CRIT verification).
2. Automate walkthrough documentation and animated terminal tutorials for community releases.
3. Validate TUI dashboard rendering without manual human intervention.
