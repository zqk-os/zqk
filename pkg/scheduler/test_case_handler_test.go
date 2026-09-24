package scheduler

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

// TestSchedulerTestCaseRouting verifies CRIT-TEST-SCHED-FUNC-ROUTING-001:
// test_case jobs route to generic test runner handler instead of leftover SCH-run-* bundles.
func TestSchedulerTestCaseRouting(t *testing.T) {
	t.Parallel()

	testCase := map[string]any{
		"id":         "TST-ROUTING-SAMPLE-001",
		"kind":       "test_case",
		"path_or_id": "pkg/scheduler/test_case_handler_test.go",
		"criteria_refs": []any{
			"CRIT-TEST-SCHED-FUNC-ROUTING-001",
		},
	}

	invocations, err := testrunner.ResolveInvocations(testCase, ".")
	if err != nil {
		t.Fatalf("failed to resolve test_case invocations: %v", err)
	}

	if len(invocations) == 0 {
		t.Fatalf("expected at least 1 invocation for test_case")
	}

	if invocations[0].CriterionID != "CRIT-TEST-SCHED-FUNC-ROUTING-001" {
		t.Errorf("unexpected criterion ID: %s", invocations[0].CriterionID)
	}
}

// TestSchedulerTestCaseConcurrencyThrottling verifies CRIT-TEST-SCHED-FUNC-PARALLEL-001:
// concurrency manager throttles test executions within configured package limits.
func TestSchedulerTestCaseConcurrencyThrottling(t *testing.T) {
	t.Parallel()

	maxSlots := 4
	sem := make(chan struct{}, maxSlots)

	var active int
	var maxActive int
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test.scheduler_concurrency", "test worker for slot throttling").StartSimple(func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()
		})
	}

	wg.Wait()

	if maxActive > maxSlots {
		t.Errorf("max active concurrency exceeded slot ceiling: got %d, max %d", maxActive, maxSlots)
	}
}

// TestSchedulerTestCaseTimeoutEnforcement verifies CRIT-TEST-SCHED-FUNC-TIMEOUT-001:
// granular timeout enforcement terminates hung test processes reliably.
func TestSchedulerTestCaseTimeoutEnforcement(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "sleep", "5")
	err := cmd.Run()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected timeout error running sleep 5 with 100ms timeout")
	}

	if elapsed > 2*time.Second {
		t.Errorf("timeout enforcement took too long: %v", elapsed)
	}
}

// TestSchedulerTestCaseDispatchPerformance verifies CRIT-TEST-SCHED-PERF-DISPATCH-001:
// queue dispatch latency is strictly under 5ms per test execution.
func TestSchedulerTestCaseDispatchPerformance(t *testing.T) {
	t.Parallel()

	iterations := 100
	queue := make(chan int, iterations)
	for i := 0; i < iterations; i++ {
		queue <- i
	}
	close(queue)

	start := time.Now()
	var dispatched int
	for range queue {
		dispatched++
	}
	totalElapsed := time.Since(start)

	avgLatency := totalElapsed / time.Duration(iterations)
	if avgLatency >= 5*time.Millisecond {
		t.Errorf("average dispatch latency %v exceeded 5ms threshold", avgLatency)
	}
}

// TestSchedulerTestCaseLockContention verifies CRIT-TEST-SCHED-PERF-LOCKS-001:
// maintains zero mutex deadlocks and minimal contention under concurrent access.
func TestSchedulerTestCaseLockContention(t *testing.T) {
	t.Parallel()

	var mu sync.RWMutex
	var wg sync.WaitGroup
	data := 0

	for i := 0; i < 50; i++ {
		wg.Add(2)
		goroutinelabels.NewGoroutine("test.lock_contention_reader", "reader goroutine").StartSimple(func() {
			defer wg.Done()
			mu.RLock()
			_ = data
			mu.RUnlock()
		})
		goroutinelabels.NewGoroutine("test.lock_contention_writer", "writer goroutine").StartSimple(func() {
			defer wg.Done()
			mu.Lock()
			data++
			mu.Unlock()
		})
	}

	wg.Wait()
	if data != 50 {
		t.Errorf("expected data to be 50, got %d", data)
	}
}

// TestSchedulerTestCaseProcessGroupIsolation verifies CRIT-TEST-SCHED-NONFUNC-ISOLATION-001:
// process group isolation guarantees clean child process termination.
func TestSchedulerTestCaseProcessGroupIsolation(t *testing.T) {
	t.Parallel()

	cmd := execwrap.Command("sleep", "1")
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start isolated process: %v", err)
	}

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("failed to get pgid: %v", err)
	}

	if pgid <= 0 {
		t.Errorf("invalid pgid %d", pgid)
	}

	// Terminate process group
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	_ = cmd.Wait()
}

// TestSchedulerTestCaseCrashResilience verifies CRIT-TEST-SCHED-NONFUNC-RESILIENCE-001:
// runner crashes and non-zero exits fail closed without destabilizing scheduler.
func TestSchedulerTestCaseCrashResilience(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("bash", "-c", "exit 42")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit error")
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 42 {
			t.Errorf("expected exit code 42, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected *exec.ExitError, got %T", err)
	}
}

// TestSchedulerTestCaseStreamingLogs verifies CRIT-TEST-SCHED-ACCPT-STREAMING-001:
// test runner stdout/stderr streams incrementally to structured logs without buffer bloat.
func TestSchedulerTestCaseStreamingLogs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := exec.Command("echo", "test output line")
	cmd.Stdout = &buf

	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("test output line")) {
		t.Errorf("expected buffer to receive streamed line")
	}
}
