package ux

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"golang.org/x/term"
)

var (
	uxMu          sync.RWMutex
	globalSpinner *spinner.Spinner
	isTTY         bool
	suppressed    bool
)

func init() {
	isTTY = determineIsTTY()
	if isTTY {
		globalSpinner = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	}
}

// determineIsTTY returns true only if stdout is an interactive terminal and non-interactive/CI flags are not set.
func determineIsTTY() bool {
	if os.Getenv("CI") != "" || os.Getenv("TERM") == "dumb" || os.Getenv("ZQK_NON_INTERACTIVE") == "1" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd())) // #nosec G115 - os.Stdout.Fd() is a valid file descriptor
}

// SetSuppressed toggles suppression of visual spinners and ANSI codes (e.g. for JSON formatting or quiet mode).
func SetSuppressed(s bool) {
	uxMu.Lock()
	defer uxMu.Unlock()
	suppressed = s
	if s && globalSpinner != nil && globalSpinner.Active() {
		globalSpinner.Stop()
	}
}

// IsSuppressed reports whether spinners and ANSI codes are currently suppressed.
func IsSuppressed() bool {
	uxMu.RLock()
	defer uxMu.RUnlock()
	return suppressed
}

// IsInteractive reports whether interactive ANSI spinner rendering is enabled.
func IsInteractive() bool {
	uxMu.RLock()
	defer uxMu.RUnlock()
	return isTTY && !suppressed
}

// SetInteractive allows overriding the interactive TTY detection (primarily for testing).
func SetInteractive(interactive bool) {
	uxMu.Lock()
	defer uxMu.Unlock()
	isTTY = interactive
	if interactive && globalSpinner == nil {
		globalSpinner = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	}
}

// StartSpinner starts a terminal spinner with the given message.
// It falls back to printing the message if not interactive.
func StartSpinner(message string) {
	uxMu.Lock()
	defer uxMu.Unlock()

	if !isTTY || suppressed {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Info(message).Log()
		return
	}
	if globalSpinner != nil {
		globalSpinner.Suffix = " " + message
		globalSpinner.Start()
	}
}

// ClearSpinner stops the active terminal spinner immediately, clears its final message,
// and clears the line in the terminal.
func ClearSpinner() {
	uxMu.Lock()
	defer uxMu.Unlock()

	if globalSpinner != nil && globalSpinner.Active() {
		globalSpinner.FinalMSG = ""
		globalSpinner.Stop()
		if isTTY && !suppressed {
			_, _ = os.Stdout.WriteString("\r\033[K")
		}
	}
}

// StopSpinner stops the terminal spinner, indicating success or failure,
// and optionally replacing the message.
func StopSpinner(success bool, finalMessage string) {
	uxMu.Lock()
	defer uxMu.Unlock()

	if !isTTY || suppressed {
		if finalMessage != "" {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Info(finalMessage).Log()
		}
		return
	}

	if globalSpinner != nil {
		if !globalSpinner.Active() {
			return
		}
		if success {
			globalSpinner.FinalMSG = color.GreenString("✓ ") + finalMessage + "\n"
		} else {
			globalSpinner.FinalMSG = color.RedString("✗ ") + finalMessage + "\n"
		}
		globalSpinner.Stop()
	}
}

// StepTracker tracks elapsed time and visual progress for multi-step operations.
type StepTracker struct {
	mu        sync.Mutex
	stepName  string
	startTime time.Time
	active    bool
}

// NewStepTracker initializes and immediately starts a new StepTracker for the given step.
func NewStepTracker(stepName string) *StepTracker {
	st := &StepTracker{
		stepName:  stepName,
		startTime: time.Now(),
		active:    true,
	}
	StartSpinner(stepName)
	return st
}

// Update updates the in-progress message for the active step.
func (st *StepTracker) Update(detail string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.active {
		return
	}
	msg := fmt.Sprintf("%s (%s)", st.stepName, detail)
	StartSpinner(msg)
}

// Elapsed returns the duration since this step was started.
func (st *StepTracker) Elapsed() time.Duration {
	st.mu.Lock()
	defer st.mu.Unlock()
	return time.Since(st.startTime)
}

// Complete stops tracking and prints a success indicator with elapsed execution duration.
func (st *StepTracker) Complete(summary string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.active {
		return
	}
	st.active = false
	elapsed := time.Since(st.startTime).Round(time.Millisecond)
	msg := fmt.Sprintf("%s [%s]", summary, elapsed)
	StopSpinner(true, msg)
}

// Fail stops tracking and prints a failure indicator with elapsed execution duration and error details.
func (st *StepTracker) Fail(summary string, err error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.active {
		return
	}
	st.active = false
	elapsed := time.Since(st.startTime).Round(time.Millisecond)
	var msg string
	if err != nil {
		msg = fmt.Sprintf("%s: %v [%s]", summary, err, elapsed)
	} else {
		msg = fmt.Sprintf("%s [%s]", summary, elapsed)
	}
	StopSpinner(false, msg)
}
