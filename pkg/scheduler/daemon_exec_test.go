package scheduler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestDaemonizationWithExec verifies the os/exec-based daemonization strategy.
func TestDaemonizationWithExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Daemonization using Setsid is not applicable on Windows.")
	}

	// Create a temporary directory for test artifacts
	testDir, err := ioutil.TempDir("", "zqk-daemon-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(testDir)

	pidFile := filepath.Join(testDir, "daemon.pid")
	logFile := filepath.Join(testDir, "daemon.log")

	// --- Compile a simple child executable for the test ---
	// This executable will simulate the `zqk-scheduler` binary that gets exec'd.
	// It writes its PID and PGID, then sleeps briefly to allow observation before exiting.
	childSource := `package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"syscall"
	"time"
)

func main() {
	pidFile := os.Args[1]
	logFile := os.Args[2]

	// Write PID and PGID to pidFile
	err := ioutil.WriteFile(pidFile, []byte(fmt.Sprintf("%d:%d", os.Getpid(), syscall.Getpgrp())), 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Child: Failed to write PID/PGID to file: %v\n", err)
		os.Exit(1)
	}

	// Write a log message to logFile
	logMsg := fmt.Sprintf("Child: Daemon started successfully. PID=%d, PGID=%d\n", os.Getpid(), syscall.Getpgrp())
	err = ioutil.WriteFile(logFile, []byte(logMsg), 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Child: Failed to write log message: %v\n", err)
		os.Exit(1)
	}

	// Keep alive for a moment to be observable before exiting.
	time.Sleep(1 * time.Second)
	os.Exit(0)
}
`
	childSrcPath := filepath.Join(testDir, "child_daemon.go")
	if err := ioutil.WriteFile(childSrcPath, []byte(childSource), 0o600); err != nil {
		t.Fatalf("Failed to write child source: %v", err)
	}

	childExePath := filepath.Join(testDir, "child_daemon_exe")
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer buildCancel()
	compileCmd := testkit.ManagedCommand(t, buildCtx, "go", "build", "-o", childExePath, childSrcPath) //nolint:gosec
	output, err := compileCmd.CombinedOutput()
	if err != nil {
		t.Fatalf(`Failed to compile child daemon: %v
Output:
%s`, err, output)
	}

	// --- Parent process launches the child using os/exec daemonization ---
	cmd := exec.Command(childExePath, pidFile, logFile, "--test-id=daemon-exec-test") //nolint:gosec
	defer func() {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	cmd.Dir = testDir // Run from test directory

	// Crucial part: SysProcAttr for daemonization
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Detach from controlling terminal, make it a session leader
	}

	// Redirect stdin, stdout, stderr to /dev/null
	nullDev, err := fileutil.OpenFile(fileutil.DevNull, fileutil.O_RDWR, 0)
	if err != nil {
		t.Fatalf("Failed to open /dev/null: %v", err)
	}
	defer nullDev.Close()
	cmd.Stdin = nullDev
	cmd.Stdout = nullDev // Will be redirected to /dev/null
	cmd.Stderr = nullDev // Will be redirected to /dev/null

	// Start the child process detached
	err = cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start daemonized child: %v", err)
	}

	// The parent process should not wait for the child in a real daemon scenario.
	// For testing, we verify its state after a short delay.
	// In a real CLI, the parent would simply exit here.
	t.Logf("Parent: Daemonized child started with PID %d", cmd.Process.Pid)

	// Give the child a moment to write its PID/PGID file
	var pidPGIDContent []byte
	var readErr error
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		pidPGIDContent, readErr = ioutil.ReadFile(pidFile) //nolint:gosec
		if readErr == nil {
			break
		}
	}
	if readErr != nil {
		t.Fatalf("Failed to read PID/PGID file after retries: %v", readErr)
	}

	parts := bytes.SplitN(pidPGIDContent, []byte(":"), 2)
	if len(parts) != 2 {
		t.Fatalf("Invalid PID/PGID file content: %s", pidPGIDContent)
	}

	childPid, err := strconv.Atoi(string(parts[0]))
	if err != nil {
		t.Fatalf("Failed to parse child PID: %v", err)
	}
	childPgid, err := strconv.Atoi(string(parts[1]))
	if err != nil {
		t.Fatalf("Failed to parse child PGID: %v", err)
	}

	t.Logf("Parent: Child reported PID=%d, PGID=%d", childPid, childPgid)

	// Check if the child process is still running (it should be, briefly)
	process, err := os.FindProcess(childPid)
	if err != nil {
		t.Fatalf("Failed to find child process (PID %d): %v", childPid, err)
	}
	// On Unix, signal 0 can be used to check for existence without sending a signal.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("Child process (PID %d) is not running: %v", childPid, err)
	}

	// Verify session detachment: child's PGID should be different from parent's
	parentPgid := syscall.Getpgrp()
	t.Logf("Parent: Parent PGID=%d", parentPgid)
	if childPgid == parentPgid {
		t.Errorf("Child PGID (%d) is unexpectedly same as parent PGID (%d). Setsid failed.", childPgid, parentPgid)
	}

	// Verify log file content (optional, but good for confidence)
	logContent, err := ioutil.ReadFile(logFile) //nolint:gosec
	if err != nil {
		t.Errorf("Failed to read child log file: %v", err)
	}
	expectedLogPrefix := fmt.Sprintf("Child: Daemon started successfully. PID=%d, PGID=%d", childPid, childPgid)
	if !bytes.Contains(logContent, []byte(expectedLogPrefix)) {
		t.Errorf("Child log content mismatch. Expected prefix: '%s', Got: '%s'", expectedLogPrefix, logContent)
	}

	// Clean up the child process forcefully for test completion, if it's still alive.
	// In a real scenario, the daemon would shut down gracefully.
	if err := process.Kill(); err != nil && !errors.Is(err, syscall.ESRCH) { // ESRCH means no such process, i.e., already exited
		t.Errorf("Failed to kill child process (PID %d): %v", childPid, err)
	}

	t.Log("Daemonization test completed successfully.")
}
