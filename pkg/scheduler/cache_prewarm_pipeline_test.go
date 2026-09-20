package scheduler

import (
	"context"
	"testing"
)

func TestRunCachePrewarmViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeCachePrewarm}
	err := RunCachePrewarmViaPipeline(ctx, nil, job)
	if err == nil {
		t.Fatal("expected error when handler is nil")
	}
}

func TestRunCachePrewarmViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &CachePrewarmHandler{logger: nil}
	err := RunCachePrewarmViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Fatal("expected error when job is nil")
	}
}
