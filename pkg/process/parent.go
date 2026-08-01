// Package process provides process-environment helpers (parent detection, idle activity) for CLI and scheduler.
package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

var (
	lastMeaningfulActivity   time.Time
	lastMeaningfulActivityMu sync.RWMutex
)

const emptyValue = ""

// TouchMeaningfulActivity records that the current command did real work (e.g. scan-tests loaded bundles).
// Used by the idle watchdog when parent is not zqk to cancel context after prolonged inactivity.
func TouchMeaningfulActivity() {
	lastMeaningfulActivityMu.Lock()
	defer lastMeaningfulActivityMu.Unlock()
	lastMeaningfulActivity = time.Now()
}

// GetLastMeaningfulActivity returns the time of last meaningful activity for the idle watchdog.
func GetLastMeaningfulActivity() time.Time {
	lastMeaningfulActivityMu.RLock()
	defer lastMeaningfulActivityMu.RUnlock()
	return lastMeaningfulActivity
}

// ParentProcessName returns the executable/command name of the process with the given PID, or empty string on error.
func ParentProcessName(pid int) string {
	if pid <= 0 {
		return emptyValue
	}
	if runtime.GOOS == "linux" {
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
		if err != nil {
			return emptyValue
		}
		return strings.TrimSpace(string(b))
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return emptyValue
	}
	return strings.TrimSpace(string(out))
}

// IsParentZqk returns true if the parent process is the main zqk executable (or our executable name).
// When false, we are likely a child of a script, IDE, or test runner; idle shutdown may apply.
func IsParentZqk() bool {
	if os.Getenv(zqkenv.IsParentZqk()) == "1" {
		return true
	}
	ppid := os.Getppid()
	if ppid <= 0 {
		return false
	}
	parentName := ParentProcessName(ppid)
	if parentName == emptyValue {
		return false
	}
	ourName := ourExecutableBaseName()
	return strings.TrimSpace(strings.ToLower(parentName)) == strings.TrimSpace(strings.ToLower(ourName))
}

func ourExecutableBaseName() string {
	exe, err := os.Executable()
	if err != nil {
		return "zqk"
	}
	base := filepath.Base(exe)
	if len(base) > 4 && strings.EqualFold(base[len(base)-4:], ".exe") {
		base = base[:len(base)-4]
	}
	return base
}
