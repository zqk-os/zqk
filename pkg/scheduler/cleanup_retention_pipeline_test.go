package scheduler

import (
	"context"
	"testing"
)

func TestRunAggregationMetricsCleanupViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeAggregationMetricsCleanup}
	err := RunAggregationMetricsCleanupViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunAggregationMetricsCleanupViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &AggregationMetricsCleanupHandler{logger: nil}
	err := RunAggregationMetricsCleanupViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}

func TestRunGenericMetricsCleanupViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeGenericMetricsCleanup}
	err := RunGenericMetricsCleanupViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunGenericMetricsCleanupViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &GenericMetricsCleanupHandler{logger: nil}
	err := RunGenericMetricsCleanupViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}

func TestRunRetentionToleranceViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeRetentionTolerance}
	err := RunRetentionToleranceViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunRetentionToleranceViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &RetentionToleranceHandler{logger: nil}
	err := RunRetentionToleranceViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}

func TestRunSchedulerJobRetentionViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeSchedulerJobRetention}
	err := RunSchedulerJobRetentionViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunSchedulerJobRetentionViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &SchedulerJobRetentionHandler{logger: nil}
	err := RunSchedulerJobRetentionViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}

func TestRunAutofixBatchCleanupViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeAutofixBatchCleanup}
	err := RunAutofixBatchCleanupViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
}

func TestRunAutofixBatchCleanupViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &AutofixBatchCleanupHandler{logger: nil}
	err := RunAutofixBatchCleanupViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
}
