package scheduler

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

func TestRunIntegrityCheckViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeIntegrityCheck}
	err := RunIntegrityCheckViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunIntegrityCheckViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &IntegrityCheckHandler{
		logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
	err := RunIntegrityCheckViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
