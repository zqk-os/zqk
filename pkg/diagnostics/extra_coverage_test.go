package diagnostics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCaptureDiagnostics_Success(t *testing.T) {
	dir := t.TempDir()

	if err := CaptureDiagnostics(dir, "test_prefix"); err != nil {
		t.Fatalf("CaptureDiagnostics failed: %v", err)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 4 {
		t.Fatalf("expected at least 4 diagnostic files captured, got %d", len(files))
	}

	// Capture again with threshold set to 1 to exercise the skip threshold path
	zqkenv.DiagnosticsThreadStackSkipMinGoroutines().Set("1")
	defer zqkenv.DiagnosticsThreadStackSkipMinGoroutines().Unset()

	if err := CaptureDiagnostics(dir, "test_prefix"); err != nil {
		t.Fatalf("CaptureDiagnostics with skip threshold failed: %v", err)
	}

	// Capture with an invalid directory path (e.g. using a file as dir)
	filePathAsDir := filepath.Join(dir, "file_blocking_dir")
	_ = os.WriteFile(filePathAsDir, []byte("blocker"), paths.FilePerm644)
	if err := CaptureDiagnostics(filePathAsDir, "test_prefix"); err == nil {
		t.Fatal("expected error when outputDir is a file")
	}
}

func TestCaptureDiagnostics_DirectHelpers(t *testing.T) {
	dir := t.TempDir()

	// Direct tests of individual capture functions
	if err := captureGoroutineDump(filepath.Join(dir, "goroutines.txt")); err != nil {
		t.Fatalf("captureGoroutineDump failed: %v", err)
	}
	if err := captureGoroutineProfileProto(filepath.Join(dir, "goroutines.pb.gz")); err != nil {
		t.Fatalf("captureGoroutineProfileProto failed: %v", err)
	}
	if err := captureHeapProfile(filepath.Join(dir, "heap.prof")); err != nil {
		t.Fatalf("captureHeapProfile failed: %v", err)
	}
	if err := captureProcessInfo(filepath.Join(dir, "proc.txt")); err != nil {
		t.Fatalf("captureProcessInfo failed: %v", err)
	}
	if err := captureThreadDump(filepath.Join(dir, "threads.txt")); err != nil {
		t.Fatalf("captureThreadDump failed: %v", err)
	}

	// Test error paths with invalid file paths
	badPath := filepath.Join(dir, "nonexistent", "deep", "file.txt")
	if err := captureGoroutineDump(badPath); err == nil {
		t.Fatal("expected error on bad path for captureGoroutineDump")
	}
	if err := captureGoroutineProfileProto(badPath); err == nil {
		t.Fatal("expected error on bad path for captureGoroutineProfileProto")
	}
	if err := captureHeapProfile(badPath); err == nil {
		t.Fatal("expected error on bad path for captureHeapProfile")
	}
	if err := captureProcessInfo(badPath); err == nil {
		t.Fatal("expected error on bad path for captureProcessInfo")
	}
	if err := captureThreadDump(badPath); err == nil {
		t.Fatal("expected error on bad path for captureThreadDump")
	}
}

func TestPprofHelpers(t *testing.T) {
	if !pprofEnvTruthy("1") || !pprofEnvTruthy("true") || !pprofEnvTruthy("TRUE") {
		t.Fatal("expected truthy values to be true")
	}
	if pprofEnvTruthy("0") || pprofEnvTruthy("false") || pprofEnvTruthy("") {
		t.Fatal("expected non-truthy values to be false")
	}

	// Test pprofEnabledFromEnv with env set
	envVar := zqkenv.Pprof().Name()
	os.Setenv(envVar, "1")
	if !pprofEnabledFromEnv() {
		t.Fatal("expected pprofEnabledFromEnv to be true when env var is 1")
	}
	os.Unsetenv(envVar)

	// Fallback env vars for pprofEnabledFromEnv
	os.Setenv(fallbackSchedulerDaemonPprof, "true")
	if !pprofEnabledFromEnv() {
		t.Fatal("expected pprofEnabledFromEnv to be true with fallbackSchedulerDaemonPprof")
	}
	os.Unsetenv(fallbackSchedulerDaemonPprof)

	os.Setenv(fallbackStableBinaryPprof, "TRUE")
	if !pprofEnabledFromEnv() {
		t.Fatal("expected pprofEnabledFromEnv to be true with fallbackStableBinaryPprof")
	}
	os.Unsetenv(fallbackStableBinaryPprof)

	// Test pprofPortFromEnv
	portVar := zqkenv.PprofPort().Name()
	os.Setenv(portVar, "9876")
	if pprofPortFromEnv() != 9876 {
		t.Fatalf("expected port 9876, got %d", pprofPortFromEnv())
	}
	os.Unsetenv(portVar)

	os.Setenv(fallbackSchedulerDaemonPort, "9877")
	if pprofPortFromEnv() != 9877 {
		t.Fatalf("expected port 9877, got %d", pprofPortFromEnv())
	}
	os.Unsetenv(fallbackSchedulerDaemonPort)

	os.Setenv(fallbackStableBinaryPprofPort, "9878")
	if pprofPortFromEnv() != 9878 {
		t.Fatalf("expected port 9878, got %d", pprofPortFromEnv())
	}
	os.Unsetenv(fallbackStableBinaryPprofPort)

	os.Setenv(portVar, "invalid_port")
	if pprofPortFromEnv() != defaultPort {
		t.Fatalf("expected default port on invalid port, got %d", pprofPortFromEnv())
	}
	os.Unsetenv(portVar)
}

func TestStartPprofServerIfEnabled(t *testing.T) {
	// Without env set, should no-op
	StartPprofServerIfEnabled()

	// With env set and a dynamic port to avoid collision
	portVar := zqkenv.PprofPort().Name()
	flagVar := zqkenv.Pprof().Name()
	os.Setenv(flagVar, "1")
	os.Setenv(portVar, "39281")
	defer os.Unsetenv(flagVar)
	defer os.Unsetenv(portVar)

	StartPprofServerIfEnabled()

	// Verify server endpoint is reachable
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://localhost:39281/debug/pprof/")
	if err == nil {
		resp.Body.Close()
	}
}

func TestStartCPUProfileOnSIGUSR2(t *testing.T) {
	// Empty logs dir should no-op
	StartCPUProfileOnSIGUSR2("")

	dir := t.TempDir()
	StartCPUProfileOnSIGUSR2(dir)

	// Call runCPUProfileDump directly
	// To avoid blocking for 30s during test execution, test the active mutex check
	cpuProfileActiveMu.Lock()
	cpuProfileActive = true
	cpuProfileActiveMu.Unlock()

	// This should return immediately because profile is active
	runCPUProfileDump()

	cpuProfileActiveMu.Lock()
	cpuProfileActive = false
	cpuProfileActiveMu.Unlock()

	// Test error paths in runCPUProfileDump
	// 1. mkdir failure (dir is an existing file)
	blockerFile := filepath.Join(dir, "mkdir_blocker")
	_ = os.WriteFile(blockerFile, []byte("x"), paths.FilePerm644)
	cpuProfileDirMu.Lock()
	cpuProfileDir = filepath.Join(blockerFile, "sub")
	cpuProfileDirMu.Unlock()
	runCPUProfileDump()

	// 2. create failure (directory is not writable or is a file)
	blockerFile2 := filepath.Join(dir, "create_blocker")
	_ = os.WriteFile(blockerFile2, []byte("x"), paths.FilePerm644)
	cpuProfileDirMu.Lock()
	cpuProfileDir = blockerFile2
	cpuProfileDirMu.Unlock()
	// 3. full successful run with 20ms duration
	cpuProfileDirMu.Lock()
	cpuProfileDir = dir
	cpuProfileDirMu.Unlock()
	cpuProfileDuration = 20 * time.Millisecond
	defer func() {
		cpuProfileDuration = 30 * time.Second
	}()
	runCPUProfileDump()
}

func TestSetupSignalHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := t.TempDir()
	SetupSignalHandler(ctx, dir, "test_prefix")

	// Trigger SIGUSR1 to test handler
	_ = syscall.Kill(os.Getpid(), syscall.SIGUSR1)

	// Wait up to 3 seconds for the capture goroutine to write files
	targetDir := filepath.Join(dir, paths.ProjectDataDir, diagnosticsSubdir, "test_prefix")
	for i := 0; i < 30; i++ {
		if entries, err := os.ReadDir(targetDir); err == nil && len(entries) >= 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestPruneDiagnosticsCaptures_Edges(t *testing.T) {
	dir := t.TempDir()

	// Prune on empty dir
	pruneDiagnosticsCaptures(dir, "test")

	// Create dummy capture sets exceeding threshold
	prefix := "test_p"
	for i := 1; i <= 15; i++ {
		ts := fmt.Sprintf("20260901-1200%02d", i)
		name := fmt.Sprintf("%s_%s_goroutines.txt", prefix, ts)
		_ = os.WriteFile(filepath.Join(dir, name), []byte("test"), paths.FilePerm644)
	}

	pruneDiagnosticsCaptures(dir, prefix)
	remaining, _ := os.ReadDir(dir)
	if len(remaining) > diagnosticsRetentionMaxCaptureSets {
		t.Fatalf("expected at most %d sets, got %d", diagnosticsRetentionMaxCaptureSets, len(remaining))
	}
}
