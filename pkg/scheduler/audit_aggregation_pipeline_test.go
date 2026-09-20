package scheduler

import (
	"context"
	"testing"
)

func TestRunAuditAggregationViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeAuditEventAggregation}

	err := RunAuditAggregationViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunAuditAggregationViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &AuditAggregationHandler{logger: nil}

	err := RunAuditAggregationViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
