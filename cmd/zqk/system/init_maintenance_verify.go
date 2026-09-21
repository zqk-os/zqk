package system

import (
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// verifyMaintenanceJobsPresent confirms at least one scheduler_job exists after
// --with-maintenance-jobs ([REDACTED-ID]).
func verifyMaintenanceJobsPresent(projectRoot string, logger logging.Logger) error {
	ctx := pkgctx.NewSystemContext()
	factory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return errfmt.Newf("verify maintenance jobs: storage factory").Wrap(err)
	}
	sp := factory.GetStorage()
	if sp == nil {
		return errfmt.Errorf("verify maintenance jobs: storage provider is nil")
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	res, lerr := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{Kind: objects.KindSchedulerJob})
	if lerr != nil {
		return errfmt.Newf("verify maintenance jobs: list scheduler_job").Wrap(lerr)
	}
	count := 0
	if res != nil {
		count = len(res.Objects)
	}
	if count == 0 {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("init --with-maintenance-jobs left zero scheduler_job objects; run 'zqk system ensure-retention-jobs' and re-check"))
	}
	if logger != nil {
		logging.Fluent(logger).Info("Verified maintenance scheduler jobs present").
			Int("scheduler_job_count", count).
			Log()
	}
	return nil
}
