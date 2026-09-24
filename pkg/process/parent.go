// Package process provides process-environment helpers (parent detection, idle activity) for CLI and scheduler.
package process

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	lastMeaningfulActivity     time.Time
	lastMeaningfulActivityMu   sync.RWMutex
	timeoutMonitorDisconnected atomic.Bool
	pagerExitCallbacksMu       sync.Mutex
	pagerExitCallbacks         []func()
)

const emptyValue = ""

// DisconnectTimeoutMonitor temporarily disconnects the command timeout monitor and idle watchdog
// for interactive sessions (such as pagers). It registers optional onExit callbacks and returns
// a reconnect function that must be called when the interactive session ends.
func DisconnectTimeoutMonitor(onExit ...func()) func() {
	timeoutMonitorDisconnected.Store(true)
	TouchMeaningfulActivity()

	if len(onExit) > 0 {
		pagerExitCallbacksMu.Lock()
		pagerExitCallbacks = append(pagerExitCallbacks, onExit...)
		pagerExitCallbacksMu.Unlock()
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			timeoutMonitorDisconnected.Store(false)
			TouchMeaningfulActivity()

			pagerExitCallbacksMu.Lock()
			callbacks := pagerExitCallbacks
			pagerExitCallbacks = nil
			pagerExitCallbacksMu.Unlock()

			for _, cb := range callbacks {
				if cb != nil {
					cb()
				}
			}
		})
	}
}

// IsTimeoutMonitorDisconnected reports whether the timeout monitor is currently disconnected.
func IsTimeoutMonitorDisconnected() bool {
	return timeoutMonitorDisconnected.Load()
}

// RegisterPagerExitCallback registers a callback to be invoked when the pager/interactive session exits.
func RegisterPagerExitCallback(cb func()) {
	if cb == nil {
		return
	}
	pagerExitCallbacksMu.Lock()
	defer pagerExitCallbacksMu.Unlock()
	pagerExitCallbacks = append(pagerExitCallbacks, cb)
}

// TouchMeaningfulActivity records that the current command did real work (e.g. zqk test run executed test_case objects).
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
		b, err := fileutil.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
		if err != nil {
			return emptyValue
		}
		return strings.TrimSpace(string(b))
	}
	out, err := execwrap.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return emptyValue
	}
	return strings.TrimSpace(string(out))
}

// IsParentZqk returns true if the parent process is the main zqk executable (or our executable name).
// When false, we are likely a child of a script, IDE, or test runner; idle shutdown may apply.
func IsParentZqk() bool {
	if zqkenv.IsParentZqk().Get() == "1" {
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
	parentBase := strings.TrimSpace(strings.ToLower(parentName))
	ourName := strings.TrimSpace(strings.ToLower(ourExecutableBaseName()))
	if parentBase == ourName {
		return true
	}
	// Role symlinks (zqk-mcp-daemon, zqk-scheduler, …) are still the brand binary.
	brand := ourName
	if brand == emptyValue {
		brand = "zqk"
	}
	return strings.HasPrefix(parentBase, brand+"-")
}

func ourExecutableBaseName() string {
	exe, err := fileutil.Executable()
	if err != nil {
		return "zqk"
	}
	base := filepath.Base(exe)
	if len(base) > 4 && strings.EqualFold(base[len(base)-4:], ".exe") {
		base = base[:len(base)-4]
	}
	return base
}
