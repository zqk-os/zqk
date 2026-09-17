package scheduler

import (
	"context"
	"testing"
)

func TestRunTestIOViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeTestIO}
	err := RunTestIOViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunTestIOViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &TestIOHandler{logger: nil}
	err := RunTestIOViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
