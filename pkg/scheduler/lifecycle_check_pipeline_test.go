package scheduler

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestRunLifecycleCheckViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeLifecycleCheck}
	err := RunLifecycleCheckViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunLifecycleCheckViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &LifecycleCheckHandler{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))}
	err := RunLifecycleCheckViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}

func TestLifecycleCheckHandler_Execute_NoError(t *testing.T) {
	t.Parallel()
	handler := &LifecycleCheckHandler{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))}
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeLifecycleCheck}
	if err := handler.Execute(context.Background(), job); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
}
