// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package execwrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunWithStandardStreams_Execution(t *testing.T) {
	t.Parallel()

	t.Run("success command", func(t *testing.T) {
		cmd := Command("true")
		if err := RunWithStandardStreams(cmd); err != nil {
			t.Fatalf("expected nil error running true with standard streams, got: %v", err)
		}
	})

	t.Run("failing command returns exit error", func(t *testing.T) {
		cmd := Command("false")
		err := RunWithStandardStreams(cmd)
		if err == nil {
			t.Fatal("expected non-nil error running false with standard streams")
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected exec.ExitError, got %T: %v", err, err)
		}
		if exitErr.ExitCode() == 0 {
			t.Fatalf("expected non-zero exit code, got %d", exitErr.ExitCode())
		}
	})

	t.Run("nonexistent binary returns error", func(t *testing.T) {
		cmd := Command("nonexistent_binary_pkg_execwrap_test")
		if err := RunWithStandardStreams(cmd); err == nil {
			t.Fatal("expected error executing nonexistent binary")
		}
	})
}

func TestRunWithTimeout_ZeroAndNegativeBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("zero timeout delegates to RunWithBuffers directly", func(t *testing.T) {
		stdout, stderr, timedOut, err := RunWithTimeout(context.Background(), 0, "echo", "zero-timeout-test")
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if timedOut {
			t.Fatal("expected timedOut to be false")
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got: %q", stderr)
		}
		if strings.TrimSpace(stdout) != "zero-timeout-test" {
			t.Fatalf("expected 'zero-timeout-test', got: %q", stdout)
		}
	})

	t.Run("negative timeout behaves as zero timeout", func(t *testing.T) {
		stdout, stderr, timedOut, err := RunWithTimeout(context.Background(), -1*time.Second, "echo", "negative-timeout-test")
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if timedOut {
			t.Fatal("expected timedOut to be false")
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got: %q", stderr)
		}
		if strings.TrimSpace(stdout) != "negative-timeout-test" {
			t.Fatalf("expected 'negative-timeout-test', got: %q", stdout)
		}
	})

	t.Run("zero timeout command error", func(t *testing.T) {
		_, _, timedOut, err := RunWithTimeout(context.Background(), 0, "false")
		if timedOut {
			t.Fatal("expected timedOut to be false")
		}
		if err == nil {
			t.Fatal("expected error executing false with zero timeout")
		}
	})

	t.Run("positive timeout command error", func(t *testing.T) {
		_, _, timedOut, err := RunWithTimeout(context.Background(), 5*time.Second, "false")
		if timedOut {
			t.Fatal("expected timedOut to be false")
		}
		if err == nil {
			t.Fatal("expected error executing false with positive timeout")
		}
	})
}

func TestRunWithTimeoutGroup_NegativeAndNonExistent(t *testing.T) {
	t.Parallel()

	t.Run("negative timeout behaves as zero timeout group", func(t *testing.T) {
		stdout, stderr, timedOut, err := RunWithTimeoutGroup(context.Background(), -500*time.Millisecond, "echo", "neg-group")
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if timedOut {
			t.Fatal("expected timedOut to be false")
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got: %q", stderr)
		}
		if strings.TrimSpace(stdout) != "neg-group" {
			t.Fatalf("expected 'neg-group', got: %q", stdout)
		}
	})

	t.Run("nonexistent binary returns error without timing out", func(t *testing.T) {
		_, _, timedOut, err := RunWithTimeoutGroup(context.Background(), 5*time.Second, "nonexistent_bin_12345")
		if timedOut {
			t.Fatal("expected timedOut to be false")
		}
		if err == nil {
			t.Fatal("expected error executing nonexistent binary in group")
		}
	})
}

func TestRunCombinedOutputWithRetry_DefaultParametersAndCap(t *testing.T) {
	t.Parallel()

	t.Run("defaults when maxRetries and initialBackoff are zero", func(t *testing.T) {
		out, err := RunCombinedOutputWithRetry(context.Background(), func() *exec.Cmd {
			return Command("echo", "defaults-applied")
		}, 0, 0)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if strings.TrimSpace(string(out)) != "defaults-applied" {
			t.Fatalf("expected 'defaults-applied', got: %q", string(out))
		}
	})

	t.Run("backoff cap triggers when initial backoff doubles beyond 200ms", func(t *testing.T) {
		attempts := 0
		out, err := RunCombinedOutputWithRetry(context.Background(), func() *exec.Cmd {
			attempts++
			cmd := Command("echo", "capped-backoff-success")
			if attempts == 1 {
				cmd.Err = syscall.ETXTBSY
			}
			return cmd
		}, 3, 150*time.Millisecond)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if attempts != 2 {
			t.Fatalf("expected exactly 2 attempts, got: %d", attempts)
		}
		if strings.TrimSpace(string(out)) != "capped-backoff-success" {
			t.Fatalf("expected 'capped-backoff-success', got: %q", string(out))
		}
	})
}

func TestRunCombinedOutputWithRetry_ETXTBSY_Lifecycle(t *testing.T) {
	t.Parallel()

	t.Run("retry recovers successfully after transient ETXTBSY errors", func(t *testing.T) {
		attempts := 0
		out, err := RunCombinedOutputWithRetry(context.Background(), func() *exec.Cmd {
			attempts++
			cmd := Command("echo", "recovered-after-etxtbsy")
			if attempts <= 2 {
				cmd.Err = syscall.ETXTBSY
			}
			return cmd
		}, 5, 2*time.Millisecond)
		if err != nil {
			t.Fatalf("expected successful execution after retry, got: %v", err)
		}
		if attempts != 3 {
			t.Fatalf("expected exactly 3 attempts, got: %d", attempts)
		}
		if strings.TrimSpace(string(out)) != "recovered-after-etxtbsy" {
			t.Fatalf("expected 'recovered-after-etxtbsy', got: %q", string(out))
		}
	})

	t.Run("retry exhausts maxRetries and returns ETXTBSY error", func(t *testing.T) {
		attempts := 0
		_, err := RunCombinedOutputWithRetry(context.Background(), func() *exec.Cmd {
			attempts++
			cmd := Command("echo", "will-fail")
			cmd.Err = syscall.ETXTBSY
			return cmd
		}, 2, 2*time.Millisecond)
		if err == nil {
			t.Fatal("expected error after exhausting retries, got nil")
		}
		if !IsTextFileBusy(err) {
			t.Fatalf("expected IsTextFileBusy to be true for returned error: %v", err)
		}
		if attempts != 3 {
			t.Fatalf("expected 3 total attempts (attempt 0, 1, 2), got: %d", attempts)
		}
	})

	t.Run("context cancelled during backoff select loop", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		_, err := RunCombinedOutputWithRetry(ctx, func() *exec.Cmd {
			attempts++
			cancel()
			cmd := Command("echo", "cancelled-during-backoff")
			cmd.Err = syscall.ETXTBSY
			return cmd
		}, 3, 50*time.Millisecond)
		if err == nil {
			t.Fatal("expected error due to cancelled context")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
	})
}

func TestIsTextFileBusy_ComprehensiveVariants(t *testing.T) {
	t.Parallel()

	if IsTextFileBusy(nil) {
		t.Fatal("expected nil error to report false")
	}
	if !IsTextFileBusy(syscall.ETXTBSY) {
		t.Fatal("expected syscall.ETXTBSY to report true")
	}
	wrappedETXTBSY := fmt.Errorf("wrapped os error: %w", syscall.ETXTBSY)
	if !IsTextFileBusy(wrappedETXTBSY) {
		t.Fatal("expected wrapped syscall.ETXTBSY to report true")
	}
	if !IsTextFileBusy(errors.New("fork/exec /bin/zqk: text file busy")) {
		t.Fatal("expected text file busy string to report true")
	}
	if !IsTextFileBusy(errors.New("CRITICAL: TEXT FILE BUSY IN SUBPROCESS")) {
		t.Fatal("expected uppercase text file busy string to report true")
	}
	if IsTextFileBusy(errors.New("executable file not found in $PATH")) {
		t.Fatal("expected unrelated error to report false")
	}
	if !IsETXTBSY(syscall.ETXTBSY) {
		t.Fatal("expected IsETXTBSY alias to report true for syscall.ETXTBSY")
	}
	if IsETXTBSY(errors.New("permission denied")) {
		t.Fatal("expected IsETXTBSY alias to report false for permission denied")
	}
}

func TestKillProcessGroup_ActiveProcess(t *testing.T) {
	t.Parallel()

	cmd := Command("sleep", "10")
	SetProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start background sleep command: %v", err)
	}

	if err := KillProcessGroup(cmd); err != nil {
		t.Fatalf("expected nil error killing active process group, got: %v", err)
	}

	waitErr := cmd.Wait()
	if waitErr == nil {
		t.Fatal("expected non-nil wait error after killing process group")
	}

	secondKillErr := KillProcessGroup(cmd)
	if secondKillErr != nil && !errors.Is(secondKillErr, syscall.ESRCH) {
		t.Logf("secondary kill returned: %v", secondKillErr)
	}
}

func TestCommandContext_LifecycleVariations(t *testing.T) {
	t.Parallel()

	t.Run("pre-canceled context fails immediately", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cmd := CommandContext(ctx, "echo", "pre-canceled")
		if err := cmd.Run(); err == nil {
			t.Fatal("expected error executing command with pre-canceled context")
		}
	})

	t.Run("pre-expired deadline context fails immediately", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-10*time.Minute))
		defer cancel()
		cmd := CommandContext(ctx, "echo", "pre-expired")
		if err := cmd.Run(); err == nil {
			t.Fatal("expected error executing command with pre-expired deadline")
		}
	})

	t.Run("context carrying telemetry values executes cleanly", func(t *testing.T) {
		type customKey struct{}
		ctx := context.WithValue(context.Background(), customKey{}, "context-metadata")
		stdout, stderr, err := RunWithBuffers(CommandContext(ctx, "echo", "context-value-test"))
		if err != nil {
			t.Fatalf("expected clean execution with value context, got: %v", err)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got: %q", stderr)
		}
		if strings.TrimSpace(stdout) != "context-value-test" {
			t.Fatalf("expected 'context-value-test', got: %q", stdout)
		}
	})
}

func TestRunWithBuffers_InterleavedAndBulkOutput(t *testing.T) {
	t.Parallel()

	t.Run("separate stdout and stderr streams", func(t *testing.T) {
		cmd := Command("sh", "-c", "echo 'stream-out' && echo 'stream-err' >&2")
		stdout, stderr, err := RunWithBuffers(cmd)
		if err != nil {
			t.Fatalf("expected clean run, got: %v", err)
		}
		if strings.TrimSpace(stdout) != "stream-out" {
			t.Fatalf("expected stdout 'stream-out', got: %q", stdout)
		}
		if strings.TrimSpace(stderr) != "stream-err" {
			t.Fatalf("expected stderr 'stream-err', got: %q", stderr)
		}
	})

	t.Run("large buffer throughput", func(t *testing.T) {
		cmd := Command("sh", "-c", "for i in $(seq 1 500); do echo \"line-$i\"; done")
		stdout, stderr, err := RunWithBuffers(cmd)
		if err != nil {
			t.Fatalf("expected clean bulk run, got: %v", err)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got: %q", stderr)
		}
		if !strings.Contains(stdout, "line-1\n") || !strings.Contains(stdout, "line-500\n") {
			t.Fatalf("expected bulk output containing first and last lines, got len: %d", len(stdout))
		}
	})
}

func TestExecutionEnvironment_DirectoryAndVariables(t *testing.T) {
	t.Parallel()

	t.Run("custom working directory resolution", func(t *testing.T) {
		tmpDir := t.TempDir()
		evalDir, err := filepath.EvalSymlinks(tmpDir)
		if err != nil {
			evalDir = tmpDir
		}
		cmd := Command("pwd")
		cmd.Dir = tmpDir
		stdout, stderr, runErr := RunWithBuffers(cmd)
		if runErr != nil {
			t.Fatalf("expected pwd to succeed, got: %v", runErr)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got: %q", stderr)
		}
		trimmed := strings.TrimSpace(stdout)
		evalTrimmed, evalErr := filepath.EvalSymlinks(trimmed)
		if evalErr != nil {
			evalTrimmed = trimmed
		}
		if evalTrimmed != evalDir {
			t.Fatalf("expected working dir %q, got: %q", evalDir, evalTrimmed)
		}
	})

	t.Run("nonexistent working directory aborts execution", func(t *testing.T) {
		nonexistentDir := filepath.Join(t.TempDir(), "deep", "nonexistent", "path")
		cmd := Command("echo", "fail-dir")
		cmd.Dir = nonexistentDir
		_, _, err := RunWithBuffers(cmd)
		if err == nil {
			t.Fatal("expected error executing command in nonexistent working directory")
		}
	})

	t.Run("custom environment variable inheritance", func(t *testing.T) {
		cmd := Command("sh", "-c", "echo $ZQK_EXECWRAP_TEST_VAR")
		cmd.Env = append(os.Environ(), "ZQK_EXECWRAP_TEST_VAR=kernel_craftsman_verified")
		stdout, stderr, err := RunWithBuffers(cmd)
		if err != nil {
			t.Fatalf("expected environment echo to succeed, got: %v", err)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got: %q", stderr)
		}
		if strings.TrimSpace(stdout) != "kernel_craftsman_verified" {
			t.Fatalf("expected 'kernel_craftsman_verified', got: %q", stdout)
		}
	})
}

func TestExitStatus_ExtractionAndMapping(t *testing.T) {
	t.Parallel()

	testCodes := []int{1, 2, 7, 42}
	for _, expectedCode := range testCodes {
		ec := expectedCode
		t.Run(fmt.Sprintf("exit status %d", ec), func(t *testing.T) {
			t.Parallel()
			cmd := Command("sh", "-c", fmt.Sprintf("exit %d", ec))
			_, _, err := RunWithBuffers(cmd)
			if err == nil {
				t.Fatalf("expected non-zero exit error for code %d", ec)
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
			}
			if exitErr.ExitCode() != ec {
				t.Fatalf("expected exit code %d, got %d", ec, exitErr.ExitCode())
			}
		})
	}
}

func TestProcessGroupIsolation_TreeTermination(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := CommandContextWithGroup(ctx, "sh", "-c", "sleep 30 & wait")
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("failed to start grouped command: %v", err)
	}

	cancel()
	err := cmd.Wait()
	if err == nil {
		t.Fatal("expected non-nil error when grouped command canceled by context")
	}
}

func TestEdgeCases_SyntacticAndPathBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("empty executable name fails to run", func(t *testing.T) {
		cmd := Command("")
		if err := cmd.Run(); err == nil {
			t.Fatal("expected error executing empty command string")
		}
	})

	t.Run("nonexistent absolute path fails to run", func(t *testing.T) {
		cmd := Command("/tmp/zqk_nonexistent_binary_xyz_12345")
		if err := cmd.Run(); err == nil {
			t.Fatal("expected error executing nonexistent binary path")
		}
	})

	t.Run("arguments with quotes whitespace and metacharacters preserve integrity", func(t *testing.T) {
		cmd := Command("echo", "alpha beta", "quotes: 'single' and \"double\"", "$VAR_LITERAL", "\\escaped")
		stdout, stderr, err := RunWithBuffers(cmd)
		if err != nil {
			t.Fatalf("expected clean argument echoing, got: %v", err)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got: %q", stderr)
		}
		expected := "alpha beta quotes: 'single' and \"double\" $VAR_LITERAL \\escaped"
		if strings.TrimSpace(stdout) != expected {
			t.Fatalf("expected %q, got %q", expected, strings.TrimSpace(stdout))
		}
	})
}
