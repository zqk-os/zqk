package storage

import (
	"math"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func (f *FileObjectStorage) mergeFileObjectUpdate(p *fileObjectUpdatePrep) error {
	existing, updates := p.existing, p.updates
	kind, id := p.kind, p.id
	spec, idUpdated := p.spec, p.idUpdated
	ctx, secCtx := p.ctx, p.secCtx
	previousStateForJournal := p.previousStateForJournal
	isBuiltIn, hasAdminRole := p.isBuiltIn, p.hasAdminRole
	// Merge updates into existing object
	for k, v := range updates {
		// CLI --unset-field: remove key from persisted object (not a value write)
		if IsFieldUnset(v) {
			delete(existing, k)
			continue
		}
		// Handle ID update separately (after validation)
		if k == objects.FieldKeyID {
			if idUpdated {
				existing[k] = v // Update ID in object
			}
			continue
		}

		// Load spec rules
		var lifecycleMode string
		if spec != nil && spec.ResolvedFields != nil {
			if fieldDefAny, ok := spec.ResolvedFields[k]; ok {
				if fieldDef, ok := fieldDefAny.(map[string]any); ok {
					if checklistAny, ok := fieldDef["checklist"]; ok {
						if checklist, ok := checklistAny.(map[string]any); ok {
							var lifecycleAny any
							if l, exists := checklist[objects.FieldKeyOriginLifecycle]; exists {
								lifecycleAny = l
							} else if l, exists := checklist["lifecycle"]; exists {
								lifecycleAny = l
							}
							if lifecycleAny != nil {
								if lifecycleStr, ok := lifecycleAny.(string); ok {
									lifecycleMode = strings.ToLower(lifecycleStr)
								}
							}
						}
					}
				}
			}
		}

		// Spec-Driven Read-Only/Immutable Fields
		isFieldImmutable := strings.HasPrefix(lifecycleMode, "immutable") || strings.HasPrefix(lifecycleMode, "read-only") ||
			(lifecycleMode == "" && (k == objects.FieldKeyCreatedAt || k == objects.FieldKeyCreatedBy || k == "cas_address" || k == "hash" || k == objects.FieldKeyUpdatedAt || k == objects.FieldKeyUpdatedBy)) // Fallback

		// Don't allow updating immutable fields, unless:
		// - Object is built-in AND user has admin role (for internal command)
		// - Even then, kind remains immutable (too fundamental)
		if k == objects.FieldKeyKind {
			continue
		}
		if isFieldImmutable {
			// Allow updating created_at/created_by for built-in objects with admin role, or when break_glass is active
			if (isBuiltIn && hasAdminRole) || pkgctx.IsLifecycleBreakGlass(ctx) {
				existing[k] = v
			} else {
				continue
			}
		}

		// Spec-Driven Append-Only Arrays
		if strings.HasPrefix(lifecycleMode, "append-only") || strings.HasPrefix(lifecycleMode, "append-mostly") || (k == objects.FieldKeyActivityLog && kind == objects.KindConvergenceSession) {
			// Ensure v is an array
			var newEntries []any
			if ne, ok := v.([]any); ok {
				newEntries = ne
			} else {
				// Single item append
				newEntries = []any{v}
			}

			if len(newEntries) > 0 {
				var merged []any
				if el, ok := existing[k].([]any); ok {
					allocCap := len(el)
					if allocCap <= math.MaxInt-len(newEntries) {
						allocCap += len(newEntries)
					}
					merged = make([]any, 0, allocCap)
					merged = append(merged, el...)
					merged = append(merged, newEntries...)
				} else {
					merged = newEntries
				}
				existing[k] = merged
				continue
			}
		}

		if mergeMapPatchIntoExisting(existing, k, v) {
			continue
		}
		existing[k] = v
	}
	// Ensure kind field is set correctly (preserve the kind, not ontology)
	existing[objects.FieldKeyKind] = kind

	// Ensure metadata: if object is materializing from draft plane into CAS or lacks created_at, stamp origination metadata
	newStateCandidate, _ := existing[objects.FieldKeyStatus].(string)
	isLeavingDraftPlane := f.objectDraftPlaneExists(kind, id) && !shouldUseObjectDraftPlane(kind, newStateCandidate)
	if isLeavingDraftPlane || existing[objects.FieldKeyCreatedAt] == nil || existing[objects.FieldKeyCreatedAt] == "" {
		if _, ok := existing[objects.FieldKeyCreatedAt]; !ok || existing[objects.FieldKeyCreatedAt] == "" {
			existing[objects.FieldKeyCreatedAt] = zqktime.NowRFC3339UTC()
		}
		if _, ok := existing[objects.FieldKeyCreatedBy]; !ok || existing[objects.FieldKeyCreatedBy] == "" {
			existing[objects.FieldKeyCreatedBy] = pkgctx.ActorIDForAttribution(secCtx.AccountID)
		}
	}
	f.ensureObjectMetadata(ctx, existing, secCtx, false)
	if pkgctx.HasLifecycleBreakGlass(ctx) {
		if explicitUpdatedAt, has := updates[objects.FieldKeyUpdatedAt]; has {
			existing[objects.FieldKeyUpdatedAt] = explicitUpdatedAt
		}
		if explicitUpdatedBy, has := updates[objects.FieldKeyUpdatedBy]; has {
			existing[objects.FieldKeyUpdatedBy] = explicitUpdatedBy
		}
	}

	// Priority & priority_tier pair synchronization on update:
	// If only one was explicitly updated, update the other to match.
	// If both were updated, both are kept and validated.
	if kind == objects.KindBacklogItem {
		_, updatedPri := updates[objects.FieldKeyPriority]
		_, updatedTier := updates[objects.FieldKeyPriorityTier]
		if updatedPri && !updatedTier {
			if tier, ok := objects.PriorityToTier(objects.GetString(existing, objects.FieldKeyPriority)); ok {
				existing[objects.FieldKeyPriorityTier] = tier
			}
		} else if updatedTier && !updatedPri {
			if pri, ok := objects.TierToPriority(objects.GetString(existing, objects.FieldKeyPriorityTier)); ok {
				existing[objects.FieldKeyPriority] = pri
			}
		}
	}

	// Normalize the merged object to ensure correct types (especially number fields)
	// This handles cases where existing objects have int values that should be float64
	crud.NormalizeObjectValues(existing, kind)

	// Get current state for lifecycle validation and hook triggering
	oldState, _ := previousStateForJournal[objects.FieldKeyStatus].(string)
	newState, _ := existing[objects.FieldKeyStatus].(string)
	if newState == emptyValue {
		newState, _ = updates[objects.FieldKeyStatus].(string)
	}
	// Work-envelope autofill: started_at on execution-locked; completed_at/actual on work_done.
	_ = applyCompleteTransitionDefaults(kind, existing, oldState, newState)

	// Validate workflow constraints for workstream operations
	if kind == objects.KindWorkstream {
		workflowValidator := NewWorkflowConstraintValidator(f)
		operation := OpUpdate
		if err := workflowValidator.ValidateWorkstreamOperation(ctx, secCtx, id, operation, kind); err != nil {
			return errfmt.Newf(ErrMsgWorkflowConstraintFail).Wrap(err)
		}
	}

	// Validate object (spec, lifecycle, references)
	if err := f.validateObject(ctx, existing, kind, oldState); err != nil {
		return errfmt.Newf(ErrMsgValidationFailed).Wrap(err)
	}

	// Route status transitions through the Shockwave API Gateway
	if err := f.dispatchStatusGateway(ctx, secCtx, existing, kind, oldState); err != nil {
		return err
	}
	// Classify updates using effective field changes after merge/normalization.
	// Some call sites pass full object payloads; unchanged structural keys should not
	// force structural CAS rewrites when only runtime-delta fields changed.
	effectiveUpdates := effectiveUpdateFieldsForClassification(previousStateForJournal, existing, updates)
	runtimeDeltaOnly := !idUpdated && updateIsRuntimeDeltaOnly(f.projectRoot, kind, effectiveUpdates)
	mutationClass := classifyUpdateMutation(idUpdated, runtimeDeltaOnly)
	RecordUpdateMutationClass(kind, mutationClass)
	StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
		Debug(LogEventStorageObjectUpdateMutationClassifiedDebug).
		ObjectID(id).
		Kind(kind).
		String(FieldKeyMutationClass, mutationClass).
		Int(FieldKeyRequestedFields, len(updates)).
		Int(FieldKeyEffectiveFields, len(effectiveUpdates)).
		Log()

	p.oldState = oldState
	p.newState = newState
	p.effectiveUpdates = effectiveUpdates
	p.runtimeDeltaOnly = runtimeDeltaOnly
	p.existing = existing
	p.updates = updates
	return nil
}
