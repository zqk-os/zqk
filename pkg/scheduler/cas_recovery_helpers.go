package scheduler

import (
	"context"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"github.com/lanceman/zqk/pkg/logging"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
)

// runCASRecoveryForKind runs CAS recovery for a single kind (stale index entries / missing hash files).
// Best-effort: logs warnings on failure but does not return error, so per-kind jobs (retention, aggregation)
// can run recovery automatically without failing the job.
// storageForKind is optional: when provided as *FileObjectStorage, recovery reuses its CAS cache (INDEX_FIRST_LOW_CPU_SCAN_DESIGN).
func runCASRecoveryForKind(ctx context.Context, projectRoot, kind string, logger logging.Logger, storageForKind storagepkg.ObjectStorageProvider) {
	if projectRoot == emptyValue || kind == emptyValue {
		return
	}
	opts := caspkg.DefaultCASRecoveryOptions(logger)
	results, err := caspkg.RecoverCASKind(ctx, projectRoot, kind, opts, storageForKind)
	if err != nil {
		CASRecoveryKindLog(logger).Warn(LogEventCASRecoveryKindFailed).
			Kind(kind).
			WithError(err).
			Log()
		return
	}
	var recovered, failed, skipped int
	for _, r := range results {
		when.When(func() bool { return r.Success }).Then(func() {
			when.When(func() bool { return r.Action == "recreated" || r.Action == "removed_from_index" }).Then(func() {
				recovered++
			}).OrElse(func() {
				skipped++
			}).Run()
		}).OrElse(func() {
			failed++
		}).Run()
	}
	if recovered > 0 || failed > 0 {
		CASRecoveryKindLog(logger).Info(LogEventCASRecoveryKindCompleted).
			Kind(kind).
			Recovered(recovered).
			FailedOps(failed).
			SkippedOps(skipped).
			Total(len(results)).
			Log()
	}
}
