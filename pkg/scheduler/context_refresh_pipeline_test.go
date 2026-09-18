package scheduler

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestRunContextRefreshViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeContextRefresh}
	err := RunContextRefreshViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunContextRefreshViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &ContextRefreshHandler{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))}
	err := RunContextRefreshViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
