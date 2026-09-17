// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.
//
// remaining_open_count lives on the remaining_open mixin (open_countable trait).
// Seed at execution lock (cold List of members). Hops CAS this field. Non-zero: stop, no status
// catalyst. Zero: YAML auto complete (new catalyst). Do not List members on the hop.
// Unset field fail-closes.
// TRACK: BLI-CEF-CONTAINER-REMAINING-OPEN-001

package lifecycle

import (
	"context"
	"errors"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

const remainingOpenCASRetries = 8

var errRemainingOpenCountUnset = errors.New("remaining_open_count unset")

func remainingOpenCountFrom(obj map[string]any) (int, bool) {
	if obj == nil {
		return 0, false
	}
	v, ok := obj[objects.FieldKeyRemainingOpenCount]
	if !ok || v == nil {
		return 0, false
	}
	n, ok := coerceNonNegInt(v)
	return n, ok
}

func coerceNonNegInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		if n < 0 {
			return 0, false
		}
		return n, true
	case int32:
		if n < 0 {
			return 0, false
		}
		return int(n), true
	case int64:
		if n < 0 {
			return 0, false
		}
		return int(n), true
	case float64:
		if n < 0 || n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case float32:
		if n < 0 || n != float32(int(n)) {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

func ensureRemainingOpenCount(ctx context.Context, provider storage.ObjectStorageProvider, containerID string, n int) error {
	if provider == nil || containerID == emptyValue {
		return nil
	}
	if n < 0 {
		n = 0
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	obj, err := provider.Read(ctx, secCtx, containerID)
	if err != nil || obj == nil {
		return err
	}
	if cur, ok := remainingOpenCountFrom(obj); ok && cur == n {
		return nil
	}
	_, err = casWriteRemainingOpenCount(ctx, provider, secCtx, containerID, n)
	return err
}

func casIncrementRemainingOpenCount(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, planID string) (int, error) {
	return casAdjustRemainingOpenCount(ctx, provider, secCtx, planID, 1)
}

func casDecrementRemainingOpenCount(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, planID string) (int, error) {
	return casAdjustRemainingOpenCount(ctx, provider, secCtx, planID, -1)
}

func casAdjustRemainingOpenCount(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, planID string, delta int) (int, error) {
	if provider == nil || planID == emptyValue {
		return 0, errRemainingOpenCountUnset
	}
	var last error
	for i := 0; i < remainingOpenCASRetries; i++ {
		plan, err := provider.Read(ctx, secCtx, planID)
		if err != nil {
			return 0, err
		}
		n, ok := remainingOpenCountFrom(plan)
		if !ok {
			return 0, errRemainingOpenCountUnset
		}
		next := n + delta
		if next < 0 {
			next = 0
		}
		if next == n {
			return n, nil
		}
		written, err := casWriteRemainingOpenCountAt(ctx, provider, secCtx, plan, planID, next)
		if err == nil {
			return written, nil
		}
		if !errors.Is(err, storage.ErrVersionConflict) {
			return 0, err
		}
		last = err
	}
	if last == nil {
		last = errfmt.Errorf("remaining_open_count CAS retries exhausted")
	}
	return 0, last
}

func casWriteRemainingOpenCount(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, planID string, n int) (int, error) {
	plan, err := provider.Read(ctx, secCtx, planID)
	if err != nil {
		return 0, err
	}
	return casWriteRemainingOpenCountAt(ctx, provider, secCtx, plan, planID, n)
}

func casWriteRemainingOpenCountAt(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, plan map[string]any, planID string, n int) (int, error) {
	updates := map[string]any{objects.FieldKeyRemainingOpenCount: n}
	if expected := objects.GetString(plan, objects.FieldKeyUpdatedAt); expected != emptyValue {
		updates[storage.FieldKeyExpectedUpdatedAt] = expected
	}
	if err := provider.Update(ctx, secCtx, planID, updates); err != nil {
		return 0, err
	}
	return n, nil
}

// consumeOpenChild shrinks remaining_open_count by one for this trigger.
// Returns (remaining, handled). handled=false means the hop should not claim the event
// (field unset — fail closed, no List).
func consumeOpenChild(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, ev DependencyRefEvent) (remaining int, handled bool, err error) {
	if provider == nil || ev.TargetID == emptyValue {
		return 0, false, nil
	}
	remaining, err = casDecrementRemainingOpenCount(ctx, provider, secCtx, ev.TargetID)
	if err == nil {
		return remaining, true, nil
	}
	if errors.Is(err, errRemainingOpenCountUnset) {
		return 0, false, nil
	}
	return 0, false, err
}

func noteOpenCountableReentered(ctx context.Context, provider storage.ObjectStorageProvider, targetID string) {
	if provider == nil || targetID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	_, _ = casIncrementRemainingOpenCount(ctx, provider, secCtx, targetID)
}

// isBacklogItemTerminalStatus resolves the status's lifecycle role rather than a
// hand-written list. Role means a new terminal status is honored the day the lifecycle gains it.
func isBacklogItemTerminalStatus(status string) bool {
	trimmed := strings.TrimSpace(status)
	if trimmed == emptyValue {
		return false
	}
	return objects.GetGlobalStatusChecker().Role(objects.KindBacklogItem, trimmed) == objects.LifecycleRoleTerminal
}

// SeedRemainingOpenCountFromMembers lists child→parent members once (cold path at
// execution lock) and writes remaining_open_count. Does not write a sidecar.
// Membership filter today is backlog_item.priority_plan_ref (only PRI composes remaining_open).
// TRACK: BLI-CEF-CONTAINER-REMAINING-OPEN-001
func SeedRemainingOpenCountFromMembers(ctx context.Context, provider storage.ObjectStorageProvider, containerID string, force bool) error {
	if provider == nil || containerID == emptyValue {
		return nil
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	obj, err := provider.Read(ctx, secCtx, containerID)
	if err != nil || obj == nil {
		return err
	}
	if !force {
		if _, ok := remainingOpenCountFrom(obj); ok {
			return nil
		}
	}
	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind == objects.KindTestCase {
		critIDs := StringRefsFromAny(obj[objects.FieldKeyCriteriaRefs])
		n := 0
		for _, cid := range critIDs {
			co, err := provider.Read(ctx, secCtx, cid)
			if err != nil || co == nil {
				n++
				continue
			}
			st, _ := co[objects.FieldKeyStatus].(string)
			if st != "complete" && st != "archived" {
				n++
			}
		}
		return ensureRemainingOpenCount(ctx, provider, containerID, n)
	}
	if kind == objects.KindMilestone {
		storageCtx := pkgctx.NewStorageContext()
		seen := make(map[string]map[string]any)
		filter1 := storage.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyMilestoneRef: containerID,
			},
		}
		if res1, err := provider.List(ctx, secCtx, storageCtx, filter1); err == nil && res1 != nil {
			for _, obj := range res1.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id != "" {
					seen[id] = obj
				}
			}
		}
		filter2 := storage.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyMilestoneRefs: map[string]any{"$has": containerID},
			},
		}
		if res2, err := provider.List(ctx, secCtx, storageCtx, filter2); err == nil && res2 != nil {
			for _, obj := range res2.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id != "" {
					seen[id] = obj
				}
			}
		}
		n := 0
		for _, bli := range seen {
			id, _ := bli[objects.FieldKeyID].(string)
			st, _ := bli[objects.FieldKeyStatus].(string)
			if id == emptyValue || isBacklogItemTerminalStatus(st) {
				continue
			}
			n++
		}
		return ensureRemainingOpenCount(ctx, provider, containerID, n)
	}
	storageCtx := pkgctx.NewStorageContext()
	result, err := provider.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyPriorityPlanRef: containerID},
	})
	if err != nil {
		return err
	}
	n := 0
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		st, _ := obj[objects.FieldKeyStatus].(string)
		if id == emptyValue || isBacklogItemTerminalStatus(st) {
			continue
		}
		n++
	}
	return ensureRemainingOpenCount(ctx, provider, containerID, n)
}

// NoteOpenCountableMemberEntered increments remaining_open_count on override add.
// If the field is unset, cold-seeds from a member List (same as execution lock).
func NoteOpenCountableMemberEntered(ctx context.Context, provider storage.ObjectStorageProvider, containerID string) {
	if provider == nil || containerID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	if _, err := casIncrementRemainingOpenCount(ctx, provider, secCtx, containerID); err == nil {
		return
	}
	_ = SeedRemainingOpenCountFromMembers(ctx, provider, containerID, false)
}

// EmitTrustedRemainingOpenDrained appends criterion_satisfied without listing members.
// Call only when remaining_open_count has hit zero on a trusted hop.
func EmitTrustedRemainingOpenDrained(projectRoot, containerID string) {
	if projectRoot == emptyValue || containerID == emptyValue {
		return
	}
	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		return
	}
	scope := map[string]string{scopePlanID: containerID}
	crit := criterionAllBacklogComplete
	if strings.HasPrefix(containerID, "MIL-") || strings.HasPrefix(containerID, "milestone-") {
		scope = map[string]string{scopeMilestoneID: containerID}
		crit = criterionAllBacklogCompleteForMilestone
	}
	_ = AppendCriterionSatisfied(wal, crit, scope)
	if syncErr := wal.Sync(); syncErr != nil {
		logging.LogSwallowedError(syncErr)
	}
}
