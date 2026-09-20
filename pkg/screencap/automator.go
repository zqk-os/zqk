// Traceability: [REDACTED-ID]

package screencap

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TimelineEvent represents a chunk of terminal output and when it occurred.
type TimelineEvent struct {
	DelayMs int64
	Text    string
}

// TerminalAutomator simulates typing commands into a process and waiting for specific output.
// It can trigger a Capturer at specific points.
type TerminalAutomator struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.Reader
	capturer  Capturer
	outputLog strings.Builder
	timeline  []TimelineEvent
	startTime time.Time
	mu        sync.Mutex
}

// NewTerminalAutomator creates a new TerminalAutomator running the specified command.
func NewTerminalAutomator(capturer Capturer, command string, args ...string) (*TerminalAutomator, error) {
	cmd := execwrap.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	cmd.Stderr = cmd.Stdout

	return &TerminalAutomator{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		capturer:  capturer,
		startTime: time.Now(),
		timeline:  make([]TimelineEvent, 0),
	}, nil
}

// GetTimeline returns the sequence of terminal output events for animation.
func (a *TerminalAutomator) GetTimeline() []TimelineEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	events := make([]TimelineEvent, len(a.timeline))
	copy(events, a.timeline)
	return events
}

// AppendTimelineEvent manually adds an event (like user input) to the timeline.
func (a *TerminalAutomator) AppendTimelineEvent(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	elapsed := time.Since(a.startTime).Milliseconds()
	a.timeline = append(a.timeline, TimelineEvent{DelayMs: elapsed, Text: text})
}

// Start begins the process.
func (a *TerminalAutomator) Start() error {
	a.startTime = time.Now()
	return a.cmd.Start()
}

// TypeCommand simulates typing a command by writing it to stdin followed by a newline.
func (a *TerminalAutomator) TypeCommand(command string) error {
	a.AppendTimelineEvent(command + "\n")
	_, err := io.WriteString(a.stdin, command+"\n")
	return err
}

// TypeCommandAndCapture types a command and immediately triggers a screen capture.
func (a *TerminalAutomator) TypeCommandAndCapture(ctx context.Context, command string, outputPath string) error {
	if err := a.TypeCommand(command); err != nil {
		return err
	}
	return a.capturer.CaptureScreen(ctx, outputPath)
}

// WaitForOutput reads from the process output until it sees the expected string.
func (a *TerminalAutomator) WaitForOutput(ctx context.Context, expected string) error {
	ch := make(chan error, 1)

	goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
		StartSimple(func() {
			func() {
				buf := make([]byte, 256)
				for {
					n, err := a.stdout.Read(buf)
					if n > 0 {
						chunk := string(buf[:n])
						a.mu.Lock()

						// Record timeline event
						elapsed := time.Since(a.startTime).Milliseconds()
						a.timeline = append(a.timeline, TimelineEvent{DelayMs: elapsed, Text: chunk})

						a.outputLog.WriteString(chunk)
						currentOutput := a.outputLog.String()
						a.mu.Unlock()

						if strings.Contains(currentOutput, expected) {
							ch <- nil
							return
						}
					}
					if err != nil {
						if err == io.EOF {
							ch <- fmt.Errorf("reached EOF before finding expected output %q", expected)
						} else {
							ch <- fmt.Errorf("error reading output: %w", err)
						}
						return
					}
				}
			}()
		})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-ch:
		return err
	}
}

// WaitForOutputAndCapture waits for the expected string, then triggers a screen capture.
func (a *TerminalAutomator) WaitForOutputAndCapture(ctx context.Context, expected string, outputPath string) error {
	if err := a.WaitForOutput(ctx, expected); err != nil {
		return err
	}
	if a.capturer != nil {
		return a.capturer.CaptureScreen(ctx, outputPath)
	}
	return nil
}

// Output returns the accumulated output so far.
func (a *TerminalAutomator) Output() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.outputLog.String()
}

// Close closes stdin and waits for the command to finish.
func (a *TerminalAutomator) Close() error {
	if a.stdin != nil {
		a.stdin.Close() //nolint:gosec
	}
	return a.cmd.Wait()
}
