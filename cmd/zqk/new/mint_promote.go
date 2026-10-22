package newcmd

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	mintPromoteJobPrefix     = "SCH-mint-promote-"
	mintPromoteJobCategory   = "lifecycle"
	mintPromoteJobType       = "run_wrapper"
	mintPromoteTriggerManual = "manual"
	mintPromoteExecutionOnce = "one_time"
	mintPromoteMaxRuntimeSec = 600
	mintPromoteRetryCount    = 1
	mintPromoteRetryDelaySec = 15
	mintPromoteStatusActive  = "active"
)

// enqueueMintPromoteJob creates a one-time run_wrapper job: zqk object promote <id>.
// Returns job id. Fail-closed: Create/Update errors are returned (trigger enqueue warn-only).
func enqueueMintPromoteJob(projectRoot, objectID string) (string, error) {
	if projectRoot == emptyValue || objectID == emptyValue {
		return "", errfmt.Errorf("mint promote: project root and object id required")
	}
	exe, err := fileutil.Executable()
	if err != nil || exe == emptyValue {
		exe = "zqk"
	}
	commandArgs := []string{"object", "promote", objectID}
	jobID, jobData := buildMintPromoteJobData(projectRoot, objectID, exe, commandArgs)

	stdctx := pkgctx.NewSystemContext()
	stdctx = pkgctx.WithPromoteOnCreate(stdctx)
	stdctx = storage.WithSyncCreateForSchedulerJob(stdctx)
	stdctx = storage.WithCLIOperation(storage.WithSkipWriteBehind(stdctx))
	factory, err := storage.NewStorageFactory(stdctx, projectRoot)
	if err != nil {
		return "", errfmt.Newf("mint promote: storage factory").Wrap(err)
	}
	provider := factory.GetStorage()
	if provider == nil {
		return "", errfmt.Errorf("mint promote: nil storage")
	}
	defer func() {
		if shutdownErr := provider.Shutdown(context.Background()); shutdownErr != nil {
			logging.LogSwallowedError(shutdownErr)
		}
	}()
	secCtx := pkgctx.NewSystemSecurityContext()

	existing, listErr := provider.List(stdctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind:    objects.KindSchedulerJob,
		Filters: map[string]any{objects.FieldKeyID: jobID},
		Limit:   1,
	})
	if listErr == nil && existing != nil && len(existing.Objects) > 0 {
		updates := map[string]any{
			objects.FieldKeyEnabled:          true,
			objects.FieldKeyStatus:           mintPromoteStatusActive,
			objects.FieldKeyCommand:          exe,
			objects.FieldKeyCommandArgs:      commandArgs,
			objects.FieldKeyWorkingDirectory: projectRoot,
			objects.FieldKeyTitle:            jobData[objects.FieldKeyTitle],
			objects.FieldKeyDescription:      jobData[objects.FieldKeyDescription],
		}
		if err := provider.Update(stdctx, secCtx, jobID, updates); err != nil {
			return "", errfmt.Newf("mint promote: update job %s", jobID).Wrap(err)
		}
	} else {
		if err := provider.Create(stdctx, secCtx, jobData); err != nil {
			return "", errfmt.Newf("mint promote: create job %s", jobID).Wrap(err)
		}
	}

	triggerQueue := scheduler.NewJobTriggerQueue(projectRoot)
	if err := triggerQueue.EnqueueTriggerRequest(jobID); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn("mint promote: job created but trigger enqueue failed").
			JobID(jobID).
			ObjectID(objectID).
			WithError(err).
			Log()
	}
	return jobID, nil
}

func buildMintPromoteJobData(projectRoot, objectID, exe string, commandArgs []string) (string, map[string]any) {
	jobID := fmt.Sprintf("%s%s", mintPromoteJobPrefix, objectID)
	now := time.Now().UTC()
	nowStr := zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ)

	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:            mintPromoteStatusActive,
		objects.FieldKeyJobType:           mintPromoteJobType,
		objects.FieldKeyTriggerType:       mintPromoteTriggerManual,
		objects.FieldKeyCategory:          mintPromoteJobCategory,
		objects.FieldKeyExecutionMode:     mintPromoteExecutionOnce,
		objects.FieldKeyMaxRuntimeSeconds: mintPromoteMaxRuntimeSec,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyTitle:             fmt.Sprintf("Mint promote: %s", objectID),
		objects.FieldKeyDescription:       fmt.Sprintf("Background promote for newly minted draft-plane object %s", objectID),
		objects.FieldKeyCommand:           exe,
		objects.FieldKeyCommandArgs:       commandArgs,
		objects.FieldKeyRetryCount:        mintPromoteRetryCount,
		objects.FieldKeyRetryDelaySeconds: mintPromoteRetryDelaySec,
		objects.FieldKeyCreatedAt:         nowStr,
		objects.FieldKeyCreatedBy:         pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:         nowStr,
		objects.FieldKeyUpdatedBy:         pkgctx.SystemAccountID,
		objects.FieldKeyWorkingDirectory:  projectRoot,
	}
	return jobID, jobData
}
