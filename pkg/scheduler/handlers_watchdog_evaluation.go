package scheduler

import (
	"context"
	"encoding/json"

	"github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

type WatchdogEvaluationHandler struct {
	storage     storagepkg.ObjectStorageProvider
	logger      logging.Logger
	projectRoot string
}

func NewWatchdogEvaluationHandler(storage storagepkg.ObjectStorageProvider, logger logging.Logger, projectRoot string) JobHandler {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &WatchdogEvaluationHandler{
		storage:     storage,
		logger:      logger,
		projectRoot: projectRoot,
	}
}

func (h *WatchdogEvaluationHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunWatchdogEvaluationViaPipeline(ctx, h, job)
}

func (h *WatchdogEvaluationHandler) executeWatchdogEvaluationCore(ctx context.Context, job *ScheduledJob) error {
	secCtx := pkgctx.NewSystemSecurityContext()

	filter := storagepkg.ListFilter{
		Kind: "watchdog_registration",
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusApproved,
		},
	}
	res, err := h.storage.List(ctx, secCtx, nil, filter)
	if err != nil {
		return errfmt.Errorf("failed to list watchdog_registrations: %w", err)
	}

	for _, reg := range res.Objects {
		targetKind, _ := reg[objects.FieldKeyTargetKind].(string)
		conditionQuery, _ := reg[objects.FieldKeyConditionQuery].(string)
		notifyTargetRef, _ := reg[objects.FieldKeyNotifyTargetRef].(string)

		if targetKind == "" || conditionQuery == "" {
			continue
		}

		field, value, err := cli.ParseFilterString(conditionQuery)
		if err != nil {
			SLog(h.logger).Error("watchdog_query_parse_failed", err).
				String("target_kind", targetKind).
				String("condition_query", conditionQuery).
				Log()
			continue
		}

		listFilter := storagepkg.ListFilter{
			Kind: targetKind,
			Filters: map[string]any{
				field: value,
			},
		}

		resList, err := h.storage.List(ctx, secCtx, nil, listFilter)
		if err != nil {
			SLog(h.logger).Error("watchdog_query_failed", err).
				String("target_kind", targetKind).
				Log()
			continue
		}

		if len(resList.Objects) > 0 {
			// Found matches! Emit event or log it.
			SLog(h.logger).Info("watchdog_condition_met").
				String("target_kind", targetKind).
				String("condition_query", conditionQuery).
				Int("matched_count", len(resList.Objects)).
				Log()

			out, _ := json.Marshal(resList.Objects)

			if notifyTargetRef != "" {
				auditEvt := map[string]any{
					objects.FieldKeyKind:      "audit_event",
					objects.FieldKeyEventType: "watchdog_trigger",
					"event_source":            "scheduler_daemon",
					"target_ref":              notifyTargetRef,
					objects.FieldKeyPayload:   string(out),
					objects.FieldKeyStatus:    objects.ObjectStatusCompleted,
				}
				if err := h.storage.Create(ctx, secCtx, auditEvt); err != nil {
					SLog(h.logger).Error("watchdog_audit_emit_failed", err).Log()
				}
			}
		}
	}
	return nil
}
