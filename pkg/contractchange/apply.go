package contractchange

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ApplyResult summarizes demotions performed while draining the outbox.
type ApplyResult struct {
	EventsConsumed int
	Demoted        []string
}

// ApplyPending drains unconsumed contract-change events and demotes ineligible
// shovel_ready|execution_locked instances (priority_plan → grooming when missing
// team_configuration_ref and persona_refs).
func ApplyPending(ctx context.Context, projectRoot string, provider storage.ObjectStorageProvider) (ApplyResult, error) {
	var res ApplyResult
	if projectRoot == "" || provider == nil {
		return res, nil
	}
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	pending, err := ListPending(projectRoot)
	if err != nil {
		return res, err
	}
	if len(pending) == 0 {
		return res, nil
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sec := pkgctx.NewSystemSecurityContext()
	var consumed []string
	seenKinds := map[string]struct{}{}

	for _, ev := range pending {
		consumed = append(consumed, ev.ID)
		if _, done := seenKinds[ev.Kind]; done {
			continue
		}
		seenKinds[ev.Kind] = struct{}{}
		if ev.Kind != objects.KindPriorityPlan {
			// Other kinds: fingerprint recorded; demote rules expand later.
			continue
		}
		ids, listErr := listPriorityPlansNeedingDemote(ctx, provider)
		if listErr != nil {
			logging.Fluent(logger).Warn("contractchange: list priority_plan failed").
				WithError(listErr).
				Log()
			continue
		}
		for _, id := range ids {
			obj, rerr := provider.Read(ctx, sec, id)
			if rerr != nil || obj == nil {
				continue
			}
			st, _ := obj[objects.FieldKeyStatus].(string)
			if !IsShovelOrLockedPlanStatus(st) {
				continue
			}
			if HasDispatchIdentity(obj) {
				continue
			}
			updates := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusGrooming}
			if uerr := provider.Update(ctx, sec, id, updates); uerr != nil {
				logging.Fluent(logger).Warn("contractchange: demote failed").
					String("object_id", id).
					WithError(uerr).
					Log()
				continue
			}
			res.Demoted = append(res.Demoted, id)
			logging.Fluent(logger).Info("contractchange: demoted priority_plan to grooming (missing dispatch identity)").
				String("object_id", id).
				String("fingerprint", ev.Fingerprint).
				Log()
		}
	}

	if err := MarkConsumed(projectRoot, consumed); err != nil {
		return res, err
	}
	res.EventsConsumed = len(consumed)
	if projectRoot != "" && len(res.Demoted) > 0 {
		_ = storage.FlushListingIndexForProjectRoot(projectRoot, objects.KindPriorityPlan)
	}
	return res, nil
}

func listPriorityPlansNeedingDemote(ctx context.Context, provider storage.ObjectStorageProvider) ([]string, error) {
	sec := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{Kind: objects.KindPriorityPlan}
	result, err := provider.List(ctx, sec, storageCtx, filter)
	if err != nil {
		return nil, err
	}
	var ids []string
	if result == nil {
		return ids, nil
	}
	for _, obj := range result.Objects {
		if obj == nil {
			continue
		}
		id, _ := obj[objects.FieldKeyID].(string)
		st, _ := obj[objects.FieldKeyStatus].(string)
		if id == "" {
			continue
		}
		if !IsShovelOrLockedPlanStatus(st) {
			continue
		}
		if HasDispatchIdentity(obj) {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}
