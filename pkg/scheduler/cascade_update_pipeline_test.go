package scheduler

import (
	"context"
	"testing"
)

func TestRunCascadeUpdateViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeCascadeUpdate}
	err := RunCascadeUpdateViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunCascadeUpdateViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &CascadeUpdateHandler{logger: nil}
	err := RunCascadeUpdateViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
