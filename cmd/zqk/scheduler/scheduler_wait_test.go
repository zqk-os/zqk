package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
)

func TestSchedulerWait_DispatchImmediate(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	jobID := "JOB-DISPATCH-001"
	cmd := NewWaitCmd()
	var out bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &out)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{jobID, "--poll-interval", "20ms", "--timeout", "5s"})

	goroutinelabels.NewGoroutine("test_wait_dispatch", "dispatch event for wait test").StartSimple(func() {
		time.Sleep(30 * time.Millisecond)
		lifecycle.GetGlobalJobWakerRegistry().Dispatch(&lifecycle.LifecycleEvent{
			EventType: lifecycle.EventTypeSchedulerCallback,
			ID:        jobID,
			Kind:      "scheduler_job",
			ToStatus:  "completed",
			Ts:        time.Now(),
			Scope: map[string]string{
				"origin": "test",
			},
		})
	})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("unexpected error executing wait command: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, jobID) {
		t.Errorf("expected output to contain job ID %s, got: %s", jobID, output)
	}
	if !strings.Contains(output, "completed") {
		t.Errorf("expected output to contain status completed, got: %s", output)
	}
	if !strings.Contains(output, "origin: test") {
		t.Errorf("expected output to contain scope metadata, got: %s", output)
	}
}

func TestSchedulerWait_FormatJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	jobID := "JOB-JSON-001"
	cmd := NewWaitCmd()
	var out bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &out)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{jobID, "--format", "json", "--poll-interval", "20ms", "--timeout", "5s"})

	goroutinelabels.NewGoroutine("test_wait_json_dispatch", "dispatch event for json test").StartSimple(func() {
		time.Sleep(30 * time.Millisecond)
		lifecycle.GetGlobalJobWakerRegistry().Dispatch(&lifecycle.LifecycleEvent{
			EventType: lifecycle.EventTypeSchedulerCallback,
			ID:        jobID,
			Kind:      "scheduler_job",
			ToStatus:  "completed",
			Ts:        time.Now(),
			Scope: map[string]string{
				"source": "ci_pipeline",
			},
		})
	})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("unexpected error executing wait command: %v", err)
	}

	var res WaitResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v, raw: %s", err, out.String())
	}

	if res.JobID != jobID {
		t.Errorf("expected JobID %s, got %s", jobID, res.JobID)
	}
	if res.Status != "completed" {
		t.Errorf("expected Status completed, got %s", res.Status)
	}
	if res.Scope["source"] != "ci_pipeline" {
		t.Errorf("expected Scope source=ci_pipeline, got %v", res.Scope)
	}
	if res.Duration == "" {
		t.Errorf("expected non-empty duration")
	}
}

func TestSchedulerWait_WALReplay(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	wal, err := lifecycle.NewLifecycleEventWAL(tmpDir)
	if err != nil {
		t.Fatalf("failed to create WAL: %v", err)
	}

	jobID := "JOB-WAL-REPLAY-001"
	walEvent := &lifecycle.LifecycleEvent{
		EventType: lifecycle.EventTypeSchedulerCallback,
		ID:        jobID,
		Kind:      "scheduler_job",
		ToStatus:  "completed",
		Ts:        time.Now(),
		Scope: map[string]string{
			"replayed": "true",
		},
	}
	if err := wal.Append(walEvent); err != nil {
		t.Fatalf("failed to append WAL event: %v", err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("failed to sync WAL: %v", err)
	}

	cmd := NewWaitCmd()
	var out bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &out)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{jobID, "--poll-interval", "20ms", "--timeout", "5s"})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("unexpected error executing wait command with WAL replay: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, jobID) {
		t.Errorf("expected output to contain job ID %s, got: %s", jobID, output)
	}
	if !strings.Contains(output, "replayed: true") {
		t.Errorf("expected output to contain replayed scope, got: %s", output)
	}
}

func TestSchedulerWait_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	jobID := "JOB-TIMEOUT-001"
	cmd := NewWaitCmd()
	var out bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &out)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{jobID, "--poll-interval", "10ms", "--timeout", "50ms"})

	err := cmd.ExecuteContext(ctx)
	if err == nil {
		t.Fatalf("expected error on timeout, got nil")
	}

	if !strings.Contains(err.Error(), "timed out waiting for job completion") {
		t.Errorf("expected timeout error message, got: %v", err)
	}
}

func TestSchedulerWait_ContextCanceled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	jobID := "JOB-CANCEL-001"
	cmd := NewWaitCmd()
	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before execution

	cmdCtx := pkgctx.WithCommandOutputWriter(ctx, &out)
	cmd.SetContext(cmdCtx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{jobID, "--poll-interval", "10ms", "--timeout", "5s"})

	err := cmd.ExecuteContext(cmdCtx)
	if err == nil {
		t.Fatalf("expected error on canceled context, got nil")
	}

	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("expected cancellation error message, got: %v", err)
	}
}

func TestSchedulerWait_FailedJobStatus(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	jobID := "JOB-FAILED-001"
	cmd := NewWaitCmd()
	var out bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &out)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{jobID, "--poll-interval", "20ms", "--timeout", "5s"})

	goroutinelabels.NewGoroutine("test_wait_failed_dispatch", "dispatch failed event for wait test").StartSimple(func() {
		time.Sleep(30 * time.Millisecond)
		lifecycle.GetGlobalJobWakerRegistry().Dispatch(&lifecycle.LifecycleEvent{
			EventType: lifecycle.EventTypeSchedulerCallback,
			ID:        jobID,
			Kind:      "scheduler_job",
			ToStatus:  "failed",
			Ts:        time.Now(),
		})
	})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("unexpected error executing wait command: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "failed") {
		t.Errorf("expected output to contain status failed, got: %s", output)
	}
}
