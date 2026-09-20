package scheduler

import (
	"context"
	"testing"
)

func TestRunCleanupConfigViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeCleanup}
	err := RunCleanupConfigViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunCleanupConfigViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &CleanupConfigHandler{projectRoot: "/tmp", logger: nil}
	err := RunCleanupConfigViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
