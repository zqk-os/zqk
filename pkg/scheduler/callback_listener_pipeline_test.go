package scheduler

import (
	"context"
	"testing"
)

func TestRunCallbackListenerViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeCallbackListener}
	err := RunCallbackListenerViaPipeline(ctx, nil, job)
	if err == nil {
		t.Fatal("expected error when handler is nil")
	}
}

func TestRunCallbackListenerViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &CallbackListenerHandler{logger: nil}
	err := RunCallbackListenerViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Fatal("expected error when job is nil")
	}
}
