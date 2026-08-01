package diagnostics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// captureMu serializes CaptureDiagnostics so only one run executes at a time (signal + scheduler timer).
// Prevents overlapping captures, file contention, and blocking signal handler on slow subprocesses.
var captureMu sync.Mutex

// CaptureDiagnostics writes goroutine stacks (text + gzip pprof proto), heap profile,
// bounded process snapshots (ps/top/lsof), and an optional runtime.Stack companion file.
//
// Operational notes (artifact names, SIGUSR1 behavior, tuning env): scripts/README.md
// subsection "Scheduler diagnostics dumps".
//
// outputDir is the directory for diagnostic files; prefix names each artifact group (e.g. "scheduler_daemon").
// Only one CaptureDiagnostics runs at a time; concurrent callers block until the active run completes.
func CaptureDiagnostics(outputDir, prefix string) error {
	captureMu.Lock()
	defer captureMu.Unlock()

	// Create diagnostics directory
	if err := os.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create diagnostics directory").Wrap(err)
	}

	timestamp := time.Now().Format("20060102-150405")
	basePath := filepath.Join(outputDir, fmt.Sprintf("%s_%s", prefix, timestamp))

	// Capture goroutine dump (text for grepping, proto for pprof with labels)
	goroutineFile := basePath + "_goroutines.txt"
	if err := captureGoroutineDump(goroutineFile); err != nil {
		return errfmt.Newf("failed to capture goroutine dump").Wrap(err)
	}
	goroutineProtoFile := basePath + "_goroutines.pb.gz"
	if err := captureGoroutineProfileProto(goroutineProtoFile); err != nil {
		return errfmt.Newf("failed to capture goroutine profile (proto)").Wrap(err)
	}

	// Capture heap profile
	heapFile := basePath + "_heap.prof"
	if err := captureHeapProfile(heapFile); err != nil {
		return errfmt.Newf("failed to capture heap profile").Wrap(err)
	}

	// Capture process information
	processFile := basePath + "_processes.txt"
	if err := captureProcessInfo(processFile); err != nil {
		return errfmt.Newf("failed to capture process info").Wrap(err)
	}

	// Capture thread dump (all goroutines with stack traces)
	threadFile := basePath + "_threads.txt"
	if err := captureThreadDump(threadFile); err != nil {
		return errfmt.Newf("failed to capture thread dump").Wrap(err)
	}

	// Best-effort: cap flat-file growth under outputDir; failures must not fail capture.
	pruneDiagnosticsCaptures(outputDir, prefix)

	return nil
}

// captureGoroutineDump writes all goroutine stack traces to file in panic-style text.
// Goroutine labels (name/purpose from goroutinelabels) do not appear in this format;
// use the .pb.gz profile and "go tool pprof -http=:8080 <base>_goroutines.pb.gz" to see them.
func captureGoroutineDump(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	profile := pprof.Lookup("goroutine")
	if profile == nil {
		return errfmt.Errorf("goroutine profile not available")
	}

	// Hint: labels (name/purpose) appear in the .pb.gz file; open with: go tool pprof -http=:8080 <base>_goroutines.pb.gz
	if _, err := fmt.Fprintln(file, "Goroutine stacks (panic-style). For labels use the matching _goroutines.pb.gz with: go tool pprof -http=:8080 <base>_goroutines.pb.gz"); err != nil {
		return err
	}
	// debug=2: same format as unrecovered panic (no labels in output)
	if err := profile.WriteTo(file, 2); err != nil {
		return err
	}

	return nil
}

// captureGoroutineProfileProto writes the goroutine profile in pprof proto format (gzip).
// Opening this with "go tool pprof -http=:8080 <file>" shows goroutine labels (name, purpose).
func captureGoroutineProfileProto(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	profile := pprof.Lookup("goroutine")
	if profile == nil {
		return errfmt.Errorf("goroutine profile not available")
	}

	// debug=0: gzip-compressed proto; pprof UI displays labels
	if err := profile.WriteTo(file, 0); err != nil {
		return err
	}

	return nil
}

// captureHeapProfile writes heap memory profile to file
func captureHeapProfile(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return pprof.WriteHeapProfile(file)
}

// threadDumpTimeout limits how long we wait for runtime.Stack when goroutine count is very high.
// If exceeded, we write a note and complete the dump so the process stays responsive (e.g. to SIGKILL).
const threadDumpTimeout = 90 * time.Second

// threadDumpSkipRuntimeStackMinGoroutines skips runtime.Stack when the process already has this many
// goroutines. CaptureDiagnostics always writes *_goroutines.txt and *_goroutines.pb.gz first; those
// contain the same all-goroutine stacks (pb.gz adds labels). Skipping avoids duplicating 10MB+ text
// and long runtime.Stack calls under heavy scheduler load.
const threadDumpSkipRuntimeStackMinGoroutines = 5000

// threadDumpStackBufSize is the buffer passed to runtime.Stack. If the written length equals this
// size, stacks may be truncated (Go does not grow the buffer).
const threadDumpStackBufSize = 32 * 1024 * 1024

// threadStackSkipThresholdFromEnv returns the minimum goroutine count at which we skip runtime.Stack.
// Default when unset: threadDumpSkipRuntimeStackMinGoroutines. Parse errors fall back to default.
// Zero or negative values mean never skip on threshold (always attempt runtime.Stack).
func threadStackSkipThresholdFromEnv() int {
	def := threadDumpSkipRuntimeStackMinGoroutines
	v := strings.TrimSpace(os.Getenv(zqkenv.DiagnosticsThreadStackSkipMinGoroutines()))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n <= 0 {
		return 0
	}
	return n
}

// captureThreadDump captures detailed thread/goroutine information.
// If runtime.Stack does not complete within threadDumpTimeout (e.g. 4k+ goroutines), we write
// the header and a timeout note so the dump handler can finish and the process remains killable.
func captureThreadDump(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	goroutineCount := runtime.NumGoroutine()
	fmt.Fprintf(file, "Thread Dump captured at: %s\n", zqktime.NowRFC3339UTC())
	fmt.Fprintf(file, "Goroutine count: %d\n", goroutineCount)
	// runtime has no portable API for OS-thread count; do not confuse with goroutines or CPUs.
	fmt.Fprintf(file, "runtime.NumCPU (hardware CPUs): %d\n", runtime.NumCPU())
	fmt.Fprintf(file, "runtime.GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	fmt.Fprintf(file, "Note: stacks below overlap *_goroutines.txt from this capture; use *_goroutines.pb.gz with go tool pprof for labeled stacks.\n")
	fmt.Fprintf(file, "========================================\n\n")

	skipMin := threadStackSkipThresholdFromEnv()
	if skipMin > 0 && goroutineCount >= skipMin {
		fmt.Fprintf(file, "[runtime.Stack body skipped: %d goroutines >= %d threshold; same stacks are in matching *_goroutines.txt and *_goroutines.pb.gz from this capture.]\n",
			goroutineCount, skipMin)
		return nil
	}

	type result struct {
		n   int
		buf []byte
	}
	done := make(chan result, 1)
	buf := make([]byte, threadDumpStackBufSize)
	goroutinelabels.NewGoroutine("diagnostics", "capture stack trace").
		StartSimple(func() {
			n := runtime.Stack(buf, true)
			done <- result{n: n, buf: buf}
		})

	select {
	case res := <-done:
		if _, err := file.Write(res.buf[:res.n]); err != nil {
			return err
		}
		if res.n >= len(res.buf) && len(res.buf) > 0 {
			fmt.Fprintf(file, "\n[warning: runtime.Stack filled %d-byte buffer; output may be truncated. Prefer *_goroutines.pb.gz or increase threadDumpStackBufSize.]\n", len(res.buf))
		}
	case <-time.After(threadDumpTimeout):
		fmt.Fprintf(file, "[thread dump skipped: timeout after %s with %d goroutines; runtime.Stack did not complete. Use _goroutines.txt or _goroutines.pb.gz from this run, or reduce load before dumping.]\n",
			threadDumpTimeout, goroutineCount)
	}

	return nil
}

// processInfoTimeout limits each external command (ps, top, lsof) so the signal handler
// and capture pipeline do not block indefinitely on a hung or slow subprocess.
const processInfoTimeout = 8 * time.Second

// captureProcessInfo captures system process information using exec.CommandContext with timeout.
// Each command (ps, top, lsof) is bounded so diagnostics capture remains responsive.
func captureProcessInfo(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write header
	fmt.Fprintf(file, "Process Information captured at: %s\n", zqktime.NowRFC3339UTC())
	fmt.Fprintf(file, "PID: %d\n", os.Getpid())
	fmt.Fprintf(file, "========================================\n\n")

	runWithTimeout := func(name string, args []string, section string) {
		ctx, cancel := context.WithTimeout(context.Background(), processInfoTimeout)
		defer cancel()
		//nolint:gosec // G204: command and args are fixed or derived from PID, not user input
		cmd := exec.CommandContext(ctx, name, args...)
		output, runErr := cmd.Output()
		if runErr != nil {
			if ctx.Err() == context.DeadlineExceeded {
				fmt.Fprintf(file, "=== %s (timed out after %s) ===\n%s\n\n", section, processInfoTimeout, runErr.Error())
			} else {
				fmt.Fprintf(file, "=== %s (error) ===\n%s\n\n", section, runErr.Error())
			}
			return
		}
		fmt.Fprintf(file, "=== %s ===\n", section)
		if _, writeErr := file.Write(output); writeErr != nil {
			_, _ = file.WriteString(writeErr.Error() + "\n")
		}
		fmt.Fprintf(file, "\n")
	}

	// Capture ps output (Unix/Linux/macOS)
	runWithTimeout("ps", []string{"aux"}, "All Processes (ps aux)")

	// Capture top output snapshot (if available, macOS)
	runWithTimeout("top", []string{"-l", "1", "-n", "10"}, "Top Processes (top -l 1 -n 10)")

	// Capture lsof for this process (what files/sockets are open)
	runWithTimeout("lsof", []string{"-p", fmt.Sprintf("%d", os.Getpid())}, fmt.Sprintf("Open Files/Sockets for PID %d (lsof -p)", os.Getpid()))

	return nil
}
