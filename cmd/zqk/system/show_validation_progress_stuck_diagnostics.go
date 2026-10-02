package system

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// dumpGoroutineStacksOnStuck dumps goroutine stacks when stuck condition is detected
// This helps identify which labeled goroutines are blocking
func dumpGoroutineStacksOnStuck(vpc *ValidationProgressContext, stuckDuration time.Duration) {
	goroutineProfile := ""
	if vpc.Cmd != nil {
		var err error
		goroutineProfile, err = vpc.Cmd.Flags().GetString("goroutine-profile")
		if err != nil {
			goroutineProfile = ""
		}
	}

	// Always write to stderr for immediate visibility
	_, _, _, queueSize := vpc.Validator.GetValidationStats()
	logging.Fluent(vpc.Logger).Warn("=== GOROUTINE STACK DUMP (STUCK DETECTED) ===").
		String("stuck_duration", stuckDuration.String()).
		Int("completed", vpc.Completed).
		Int("failed", vpc.Failed).
		Int("queue_size", queueSize).
		Log()

	// Write goroutine profile to file if requested
	if goroutineProfile != emptyValue {
		// Append timestamp to filename for multiple dumps
		timestamp := time.Now().Format("20060102-150405")
		ext := filepath.Ext(goroutineProfile)
		base := goroutineProfile[:len(goroutineProfile)-len(ext)]
		stuckProfile := fmt.Sprintf("%s-stuck-%s%s", base, timestamp, ext)

		if err := writeGoroutineProfileToFile(stuckProfile); err != nil {
			logging.Fluent(vpc.Logger).Warn("Failed to write stuck goroutine profile").
				File(stuckProfile).
				WithError(err).
				Log()
		} else {
			logging.Fluent(vpc.Logger).Info("Goroutine profile written (stuck condition)").File(stuckProfile).Log()
		}
	}

	// Also dump to stderr for immediate visibility (stack bytes via profile.WriteTo; header/footer via logger)
	profile := pprof.Lookup("goroutine")
	if profile != nil {
		logging.Fluent(vpc.Logger).Warn("GOROUTINE STACK DUMP (stuck)").
			String("stuck_duration", stuckDuration.String()).
			Log()
		if err := profile.WriteTo(os.Stderr, 2); err != nil { // Debug level 2 shows more detail
			logging.Fluent(vpc.Logger).Warn("Failed to write goroutine stack dump to stderr").WithError(err).Log()
		}
		logging.Fluent(vpc.Logger).Warn("END GOROUTINE STACK DUMP").Log()
	}
}

// writeGoroutineProfileToFile writes a goroutine profile to file
func writeGoroutineProfileToFile(filename string) error {
	return writeGoroutineProfile(filename, 2)
}
