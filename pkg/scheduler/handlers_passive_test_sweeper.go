package scheduler

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testrunner"
)

// JobTypePassiveTestSweeper defines the job_type for background test sweeping.
const JobTypePassiveTestSweeper = "passive_test_sweeper"

// PassiveTestSweeperHandler passively checks in_progress backlog items with linked test cases,
// runs their verification suites, and cascades Shockwave criteria status.
type PassiveTestSweeperHandler struct {
	storage     storagepkg.ObjectStorageProvider
	projectRoot string
	logger      logging.Logger
}

// NewPassiveTestSweeperHandler creates a new passive test sweeper handler.
func NewPassiveTestSweeperHandler(storage storagepkg.ObjectStorageProvider, projectRoot string, logger logging.Logger) JobHandler {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &PassiveTestSweeperHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logger,
	}
}

// Execute scans for in_progress BLIs, finds linked test cases, and executes them.
func (h *PassiveTestSweeperHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	SLog(h.logger).Info("passive_test_sweeper_started").JobID(job.ID).Log()

	if h.storage == nil {
		return nil
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// 1. List all backlog items
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindBacklogItem,
	})
	if err != nil || res == nil {
		SLog(h.logger).Warn("passive_test_sweeper_list_blis_failed").WithError(err).Log()
		return nil
	}

	var inProgressBLIs []map[string]any
	for _, obj := range res.Objects {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if status == objects.ObjectStatusInProgress {
			inProgressBLIs = append(inProgressBLIs, obj)
		}
	}

	if len(inProgressBLIs) == 0 {
		SLog(h.logger).Debug("passive_test_sweeper_no_in_progress_blis").Log()
		return nil
	}

	// 2. List all test cases to find matching ones
	tcRes, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindTestCase,
	})
	if err != nil || tcRes == nil {
		SLog(h.logger).Warn("passive_test_sweeper_list_test_cases_failed").WithError(err).Log()
		return nil
	}

	// Index test cases by ID and by linked BLI / Criteria
	testCasesToRun := make(map[string]map[string]any)

	for _, bli := range inProgressBLIs {
		bliID, _ := bli[objects.FieldKeyID].(string)
		bliCritRefs := toStringSlice(bli[objects.FieldKeyCriteriaRefs])

		for _, tc := range tcRes.Objects {
			tcStatus, _ := tc[objects.FieldKeyStatus].(string)
			if tcStatus == objects.ObjectStatusComplete {
				continue
			}
			tcID, _ := tc[objects.FieldKeyID].(string)
			if tcID == "" {
				continue
			}

			matched := false
			// Check direct backlog_item_refs
			for _, b := range toStringSlice(tc[objects.FieldKeyBacklogItemRefs]) {
				if b == bliID {
					matched = true
					break
				}
			}

			// Check criteria overlap
			if !matched && len(bliCritRefs) > 0 {
				tcCrits := toStringSlice(tc[objects.FieldKeyCriteriaRefs])
				for _, tcCritStr := range tcCrits {
					for _, bCrit := range bliCritRefs {
						if tcCritStr != "" && tcCritStr == bCrit {
							matched = true
							break
						}
					}
					if matched {
						break
					}
				}
			}

			if matched {
				testCasesToRun[tcID] = tc
			}
		}
	}

	if len(testCasesToRun) == 0 {
		SLog(h.logger).Debug("passive_test_sweeper_no_matching_test_cases").Log()
		return nil
	}

	// 3. Run test cases
	sweptCount := 0
	passedCount := 0
	for tcID := range testCasesToRun {
		sweptCount++
		opts := testrunner.RunOptions{
			Timeout: 45 * time.Second,
		}
		runRes, runErr := testrunner.RunTestCase(ctx, h.storage, h.projectRoot, tcID, opts)
		if runErr != nil {
			SLog(h.logger).Warn("passive_test_sweeper_tc_run_error").
				ObjectID(tcID).
				WithError(runErr).
				Log()
			continue
		}
		if runRes != nil && runRes.PassedCriteria > 0 && runRes.FailedCriteria == 0 {
			passedCount++
			SLog(h.logger).Info("passive_test_sweeper_tc_passed").
				ObjectID(tcID).
				Count(runRes.PassedCriteria).
				Log()
		}
	}

	SLog(h.logger).Info("passive_test_sweeper_completed").
		Count(sweptCount).
		Log()

	_ = fmt.Sprintf("Swept %d test cases, %d passed", sweptCount, passedCount)
	return nil
}

func toStringSlice(val any) []string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		var res []string
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				res = append(res, s)
			}
		}
		return res
	}
	return nil
}
