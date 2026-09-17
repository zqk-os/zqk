package scheduler

import (
	"context"
	"testing"
)

func TestRunMetricsCollectionViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeMetricsCollection}
	err := RunMetricsCollectionViaPipeline(ctx, nil, job)
	if err == nil {
		t.Fatal("expected error when handler is nil")
	}
}

func TestRunMetricsCollectionViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &FileLockMetricsCollectionHandler{logger: nil}
	err := RunMetricsCollectionViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Fatal("expected error when job is nil")
	}
}
