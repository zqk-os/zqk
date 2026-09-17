package scheduler

import (
	"context"
	"testing"
)

func TestRunChangeJournalAggregationViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeChangeJournalAggregation}

	result, err := RunChangeJournalAggregationViaPipeline(ctx, nil, job)
	if err == nil {
		t.Error("expected error when handler is nil")
	}
	if result != nil {
		t.Errorf("expected nil result when handler is nil, got %v", result)
	}
}

func TestRunChangeJournalAggregationViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &ChangeJournalAggregationHandler{logger: nil}

	result, err := RunChangeJournalAggregationViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Error("expected error when job is nil")
	}
	if result != nil {
		t.Errorf("expected nil result when job is nil, got %v", result)
	}
}
