package ux

import (
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"golang.org/x/term"
)

var globalSpinner *spinner.Spinner
var isTTY bool

func init() {
	isTTY = term.IsTerminal(int(os.Stdout.Fd())) // #nosec G115 - os.Stdout.Fd() is a valid file descriptor
	if isTTY {
		globalSpinner = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	}
}

// StartSpinner starts a terminal spinner with the given message.
// It falls back to printing the message if not a TTY.
func StartSpinner(message string) {
	if !isTTY {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Info(message).Log()
		return
	}
	globalSpinner.Suffix = " " + message
	globalSpinner.Start()
}

// StopSpinner stops the terminal spinner, indicating success or failure,
// and optionally replacing the message.
func StopSpinner(success bool, finalMessage string) {
	if !isTTY {
		if finalMessage != "" {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Info(finalMessage).Log()
		}
		return
	}

	if success {
		globalSpinner.FinalMSG = color.GreenString("✓ ") + finalMessage + "\n"
	} else {
		globalSpinner.FinalMSG = color.RedString("✗ ") + finalMessage + "\n"
	}
	globalSpinner.Stop()
}
