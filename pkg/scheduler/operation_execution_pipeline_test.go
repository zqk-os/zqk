package scheduler

import (
	"context"
	"testing"
)

func TestRunOperationExecutionViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeOperationExecution}
	err := RunOperationExecutionViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunOperationExecutionViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &OperationExecutionHandler{logger: nil}
	err := RunOperationExecutionViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
