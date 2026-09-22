// BLI-STARTER-COMMUNITY-057 / PRI-STARTER-COMMUNITY-057 coverage elevation
package ux

import (
	"testing"
	"time"

	"github.com/briandowns/spinner"
)

func TestExtraSpinnerTTYAndFallback(t *testing.T) {
	origTTY, origSpin := isTTY, globalSpinner
	t.Cleanup(func() {
		isTTY = origTTY
		globalSpinner = origSpin
	})

	isTTY = false
	globalSpinner = nil
	StartSpinner("offline")
	StopSpinner(true, "ok")
	StopSpinner(false, "")
	StopSpinner(true, "")

	isTTY = true
	globalSpinner = spinner.New(spinner.CharSets[14], time.Millisecond)
	StartSpinner("working")
	StopSpinner(true, "done")
	StartSpinner("again")
	StopSpinner(false, "fail")
}
