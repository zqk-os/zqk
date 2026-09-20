package scheduler

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ShouldSkipBackgroundValidationBatchForKind returns true for kinds that must not enqueue
// work onto the reusable SCH-val object_validation batch on every create/update/delete.
//
//   - scheduler_job: validating via `system check scheduler_job <id>` from the batch was part of
//     legacy churn that created additional one-off run_wrapper jobs and recursive SCH-...-scheduler-job-... ids.
//   - Stream-backed high-volume kinds: object-id-cache excludes them; batching them adds overhead without benefit.
func ShouldSkipBackgroundValidationBatchForKind(kind string) bool {
	if kind == emptyValue {
		return true
	}
	if kind == objects.KindSchedulerJob {
		return true
	}
	return storage.IsHighVolumeKindForCache(kind)
}
