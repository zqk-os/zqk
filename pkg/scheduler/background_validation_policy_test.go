package scheduler

import (
	"testing"
)

func TestShouldSkipBackgroundValidationBatchForKind(t *testing.T) {
	t.Parallel()
	if !ShouldSkipBackgroundValidationBatchForKind("") {
		t.Fatal("empty kind should skip")
	}
	if !ShouldSkipBackgroundValidationBatchForKind("scheduler_job") {
		t.Fatal("scheduler_job should skip")
	}
	if ShouldSkipBackgroundValidationBatchForKind("backlog_item") {
		t.Fatal("backlog_item should not skip")
	}
}
