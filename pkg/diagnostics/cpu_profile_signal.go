package diagnostics

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/logging"
)

const (
	cpuProfileDuration = 30 * time.Second
	emptyValue         = ""
)

var (
	cpuProfileOnce     sync.Once
	cpuProfileDir      string
	cpuProfileDirMu    sync.Mutex
	cpuProfileActive   bool
	cpuProfileActiveMu sync.Mutex
)

// StartCPUProfileOnSIGUSR2 starts a goroutine that listens for SIGUSR2 and writes a 30s
// CPU profile to the given directory (e.g. projectRoot/.zqk/logs). Call once from root
// PreRunE after resolving project root. No-op on Windows. File name: cpu-<timestamp>.prof.
// To capture: kill -USR2 <pid>, wait 30s, then inspect the new .prof file and run
// go tool pprof -http=:8080 <file>.
func StartCPUProfileOnSIGUSR2(logsDir string) {
	if logsDir == emptyValue {
		return
	}
	cpuProfileDirMu.Lock()
	cpuProfileDir = logsDir
	cpuProfileDirMu.Unlock()

	cpuProfileOnce.Do(func() {
		if isWindows() {
			return
		}
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGUSR2)
		goroutinelabels.NewGoroutine("diagnostics", "cpu profile dump").
			StartSimple(func() {
				for range ch {
					runCPUProfileDump()
				}
			})
	})
}

func isWindows() bool {
	return runtime.GOOS == "windows"
}

func runCPUProfileDump() {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	cpuProfileActiveMu.Lock()
	if cpuProfileActive {
		cpuProfileActiveMu.Unlock()
		logging.Fluent(logger).Warn("[pprof] CPU profile already in progress, skipped").Log()
		return
	}
	cpuProfileActive = true
	cpuProfileActiveMu.Unlock()

	defer func() {
		cpuProfileActiveMu.Lock()
		cpuProfileActive = false
		cpuProfileActiveMu.Unlock()
	}()

	cpuProfileDirMu.Lock()
	dir := cpuProfileDir
	cpuProfileDirMu.Unlock()
	if dir == emptyValue {
		dir = fileutil.TempDir()
	}

	ts := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("cpu-%s.prof", ts)
	path := filepath.Join(dir, name)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		logging.Fluent(logger).Warn("[pprof] CPU profile failed (mkdir)").
			WithError(err).
			Log()
		return
	}
	f, err := fileutil.Create(path)
	if err != nil {
		logging.Fluent(logger).Warn("[pprof] CPU profile failed (create)").
			WithError(err).
			Log()
		return
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close() //nolint:gosec
		logging.Fluent(logger).Warn("[pprof] CPU profile failed (start)").
			WithError(err).
			Log()
		return
	}
	logging.Fluent(logger).Info("[pprof] CPU profile started (30s)").
		Path(path).
		Log()
	time.Sleep(cpuProfileDuration)
	pprof.StopCPUProfile()
	if err := f.Close(); err != nil { //nolint:gosec
		logging.Fluent(logger).Warn("[pprof] CPU profile close warning").
			WithError(err).
			Log()
	}
	logging.Fluent(logger).Info("[pprof] CPU profile written").
		Path(path).
		Log()
}
