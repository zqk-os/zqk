package scheduler

import (
	"context"
	"testing"
)

func TestRunCacheInvalidationViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeCacheInvalidation}
	err := RunCacheInvalidationViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunCacheInvalidationViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &CacheInvalidationHandler{logger: nil}
	err := RunCacheInvalidationViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
