package object

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// NewRefCmd creates the top-level 'object ref' command.
// TRACK: REQ-REDACTED / BLI-REDACTED
func NewRefCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectRefCommandBuilder()
	cmd.AddCommand(NewRefAddCmd())
	cmd.AddCommand(NewRefRemoveCmd())
	return cmd
}

// NewRefAddCmd creates the 'object ref add' subcommand.
func NewRefAddCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectRefAddCommandBuilder()
	cli.BindAsyncProgress(cmd, runRefAdd)
	return cmd
}

func runRefAdd(cmd *cobra.Command, args []string) error {
	sourceRawIDs := strings.Split(args[0], ",")
	targetRawIDs := args[1:]

	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		for _, sourceRawID := range sourceRawIDs {
			sourceRawID = strings.TrimSpace(sourceRawID)
			if sourceRawID == "" {
				continue
			}
			sourceID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", sourceRawID)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve source ID %s: %w", sourceRawID, err)).Return()
			}

			sourceObj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), sourceID)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("source object %s not found: %w", sourceID, err)).Return()
			}

			sourceKind, _ := sourceObj[objects.FieldKeyKind].(string)
			sourceSpec := loadSpecForKind(sourceKind)

			explicitField, _ := cmd.Flags().GetString("field")
			override, _ := cmd.Flags().GetBool("override")
			force, _ := cmd.Flags().GetBool("force")
			if force {
				override = true
			}
			reasonCode, _ := cmd.Flags().GetString("reason-code")

			updates := make(map[string]any)
			var associatedLivePlanID string

		for _, rawTarget := range targetRawIDs {
			targetID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", rawTarget)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve target ID %s: %w", rawTarget, err)).Return()
			}

			// Self-reference check
			if sourceID == targetID {
				return cli.Guard(cmd).Err(errfmt.Errorf("cannot reference self (%s)", sourceID)).Return()
			}

			// Read target object to verify existence and kind
			targetObj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), targetID)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("target object %s not found: %w", targetID, err)).Return()
			}
			targetKind, _ := targetObj[objects.FieldKeyKind].(string)

			// Check if this is an inverted relationship where target owns ref to source
			// e.g. source is priority_plan, target is backlog_item (which has priority_plan_ref)
			if explicitField == "" && !hasSpecificRefField(sourceSpec, sourceObj, targetKind) {
				targetSpec := loadSpecForKind(targetKind)
				if hasSpecificRefField(targetSpec, targetObj, sourceKind) {
					invField, invIsSlice := resolveSpecificRefField(targetSpec, targetObj, sourceKind)

					// Cross-plane validation: CAS target cannot reference draft-plane source
					targetIsDraftOnly := storage.IsDraftPlaneOnly(proc.ProjectRoot(), targetKind, targetID)
					sourceIsDraftOnly := storage.IsDraftPlaneOnly(proc.ProjectRoot(), sourceKind, sourceID)
					if !targetIsDraftOnly && sourceIsDraftOnly {
						return cli.Guard(cmd).Err(errfmt.Errorf("association denied: CAS object %s (%s) cannot reference draft-plane object %s (%s); cross-plane references prohibited", targetID, targetKind, sourceID, sourceKind)).Return()
					}

					// Scope-lock and terminal checks on source (plan)
					statusRole := objects.GetGlobalStatusChecker()
					targetStatus, _ := targetObj[objects.FieldKeyStatus].(string)
					if statusRole.Role(targetKind, targetStatus) == objects.LifecycleRoleTerminal {
						return cli.Guard(cmd).Err(errfmt.Errorf("association denied: %s %s is in terminal status %s", targetKind, targetID, targetStatus)).Return()
					}
					sourceStatus, _ := sourceObj[objects.FieldKeyStatus].(string)
					if statusRole.Role(sourceKind, sourceStatus) == objects.LifecycleRoleTerminal {
						return cli.Guard(cmd).Err(errfmt.Errorf("association denied: %s %s is in terminal status %s", sourceKind, sourceID, sourceStatus)).Return()
					}
					if sourceKind == objects.KindPriorityPlan && (sourceStatus == objects.ObjectStatusInProgress || sourceStatus == objects.ObjectStatusActive) {
						if !override {
							return cli.Guard(cmd).Err(errfmt.Errorf("association denied: priority plan %s is '%s' (sealed / execution-facing); open a grooming plan for new work instead of stuffing the locked column", sourceID, sourceStatus)).Return()
						}
						if reasonCode == "" {
							return cli.Guard(cmd).Require(false, "--reason-code is required when using --override on sealed priority plans").Return()
						}
						associatedLivePlanID = sourceID
					}

					targetUpdates := make(map[string]any)
					if invIsSlice {
						currentSlice := getRefSlice(targetObj, nil, invField)
						alreadyPresent := false
						for _, item := range currentSlice {
							if item == sourceID {
								alreadyPresent = true
								break
							}
						}
						if !alreadyPresent {
							currentSlice = append(currentSlice, sourceID)
							targetUpdates[invField] = stringSliceToAny(currentSlice)
						}
					} else {
						currentVal := getRefScalar(targetObj, nil, invField)
						if currentVal != "" && currentVal != sourceID {
							return cli.Guard(cmd).Err(errfmt.Errorf("cannot overwrite scalar reference '%s' on %s (currently pointing to %s); remove existing reference first", invField, targetID, currentVal)).Return()
						}
						targetUpdates[invField] = sourceID
					}

					if len(targetUpdates) > 0 {
						cleanLegacyKeys(targetObj, targetUpdates)
						updateCtx := proc.OperationContext()
						if err := proc.Storage().Update(updateCtx, proc.SecurityContext(), targetID, targetUpdates); err != nil {
							return cli.Guard(cmd).Err(errfmt.Errorf("failed to link %s to %s: %w", targetID, sourceID, err)).Return()
						}
						if sourceKind == objects.KindPriorityPlan {
							lifecycle.NoteOpenCountableMemberEntered(proc.OperationContext(), proc.Storage(), sourceID)
						}
						_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), targetKind)
						_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), sourceKind)
						proc.TriggerCacheFreshnessCheck("ref_add", []string{targetKind, sourceKind})

						msg := fmt.Sprintf("✓ Successfully linked %s %s to %s %s", targetKind, targetID, sourceKind, sourceID)
						logging.FluentEvent(proc.Logger()).Info(msg).Log()
						cmd.Println(msg)
					} else {
						cmd.Printf("Reference %s -> %s is already up to date (idempotent).\n", targetID, sourceID)
					}
					continue
				}
			}

			// Resolve reference field and whether it is a slice or scalar
			targetField := explicitField
			isSlice := false
			if targetField != "" {
				isSlice = isSliceField(sourceSpec, sourceObj, targetField)
			} else {
				var resolveErr error
				targetField, isSlice, resolveErr = resolveRefFieldForTarget(sourceSpec, sourceObj, targetKind, targetID)
				if resolveErr != nil {
					return cli.Guard(cmd).Err(resolveErr).Return()
				}
			}

			// Cross-plane validation: CAS source cannot reference draft-plane target
			sourceIsDraftOnly := storage.IsDraftPlaneOnly(proc.ProjectRoot(), sourceKind, sourceID)
			targetIsDraftOnly := storage.IsDraftPlaneOnly(proc.ProjectRoot(), targetKind, targetID)
			if !sourceIsDraftOnly && targetIsDraftOnly {
				return cli.Guard(cmd).Err(errfmt.Errorf("association denied: CAS object %s (%s) cannot reference draft-plane object %s (%s); cross-plane references prohibited", sourceID, sourceKind, targetID, targetKind)).Return()
			}

			// Sealed Priority Plan Scope-Lock Gate (BLI-REDACTED)
			if targetField == objects.FieldKeyPriorityPlanRef || targetKind == objects.KindPriorityPlan {
				statusRole := objects.GetGlobalStatusChecker()
				sourceStatus, _ := sourceObj[objects.FieldKeyStatus].(string)
				if statusRole.Role(sourceKind, sourceStatus) == objects.LifecycleRoleTerminal {
					return cli.Guard(cmd).Err(errfmt.Errorf("association denied: %s %s is in terminal status %s", sourceKind, sourceID, sourceStatus)).Return()
				}
				targetStatus, _ := targetObj[objects.FieldKeyStatus].(string)
				if statusRole.Role(objects.KindPriorityPlan, targetStatus) == objects.LifecycleRoleTerminal {
					return cli.Guard(cmd).Err(errfmt.Errorf("association denied: priority plan %s is in terminal status %s", targetID, targetStatus)).Return()
				}
				if targetStatus == objects.ObjectStatusInProgress || targetStatus == objects.ObjectStatusActive {
					if !override {
						return cli.Guard(cmd).Err(errfmt.Errorf("association denied: priority plan %s is '%s' (sealed / execution-facing); open a grooming plan for new work instead of stuffing the locked column", targetID, targetStatus)).Return()
					}
					if reasonCode == "" {
						return cli.Guard(cmd).Require(false, "--reason-code is required when using --override on sealed priority plans").Return()
					}
					associatedLivePlanID = targetID
				}
			}

			// Traversal and Cycle Prevention for related_object_refs (BLI-REDACTED)
			if targetField == objects.FieldKeyRelatedObjectRefs {
				// 1. Redundancy check: target must not already be present in typed reference fields
				if typedField := findInTypedRefFields(sourceObj, updates, targetID); typedField != "" {
					return cli.Guard(cmd).Err(errfmt.Errorf("association denied: target %s is already referenced in typed field '%s'; redundant reference in related_object_refs prohibited", targetID, typedField)).Return()
				}

				// 2. Direct cycle check: target must not already reference source
				if targetRelated, ok := targetObj[objects.FieldKeyRelatedObjectRefs].([]any); ok {
					for _, r := range targetRelated {
						if rs, ok := r.(string); ok && rs == sourceID {
							return cli.Guard(cmd).Err(errfmt.Errorf("cycle detected: target %s already references %s in related_object_refs; bidirectional cycle prohibited", targetID, sourceID)).Return()
						}
					}
				}

				// 3. Transitive cycle detection up to depth 5
				if hasTransitiveCycle(proc.OperationContext(), proc.SecurityContext(), proc.Storage(), targetID, sourceID, 5) {
					return cli.Guard(cmd).Err(errfmt.Errorf("cycle detected: adding %s to %s.related_object_refs creates a cycle across transitive links", targetID, sourceID)).Return()
				}
			}

			// Apply reference
			if isSlice {
				currentSlice := getRefSlice(sourceObj, updates, targetField)
				alreadyPresent := false
				for _, item := range currentSlice {
					if item == targetID {
						alreadyPresent = true
						break
					}
				}
				if !alreadyPresent {
					currentSlice = append(currentSlice, targetID)
					updates[targetField] = stringSliceToAny(currentSlice)
				}
			} else {
				currentVal := getRefScalar(sourceObj, updates, targetField)
				if currentVal != "" && currentVal != targetID {
					return cli.Guard(cmd).Err(errfmt.Errorf("cannot overwrite scalar reference '%s' (currently pointing to %s); remove existing reference first", targetField, currentVal)).Return()
				}
				updates[targetField] = targetID
			}
		}

		if len(updates) == 0 {
			cmd.Println("No reference changes required (idempotent).")
			continue
		}

		cleanLegacyKeys(sourceObj, updates)
		err = proc.Storage().Update(proc.OperationContext(), proc.SecurityContext(), sourceID, updates)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to update references on %s: %w", sourceID, err)).Return()
		}

		if associatedLivePlanID != "" {
			lifecycle.NoteOpenCountableMemberEntered(proc.OperationContext(), proc.Storage(), associatedLivePlanID)
		}

		msg := fmt.Sprintf("✓ Successfully added reference(s) to %s", sourceID)
		logging.FluentEvent(proc.Logger()).Info(msg).Log()
		cmd.Println(msg)
	}
	return nil
	})(cmd, args)
}

// NewRefRemoveCmd creates the 'object ref remove' subcommand.
func NewRefRemoveCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectRefRemoveCommandBuilder()
	cli.BindAsyncProgress(cmd, runRefRemove)
	return cmd
}

func runRefRemove(cmd *cobra.Command, args []string) error {
	sourceRawID := args[0]
	targetRawIDs := args[1:]

	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		allFlag, _ := cmd.Flags().GetBool("all")
		kindFlag, _ := cmd.Flags().GetString("kind")
		explicitField, _ := cmd.Flags().GetString("field")
		parkFlag, _ := cmd.Flags().GetBool("park")

		if !allFlag && kindFlag == "" && explicitField == "" && len(targetRawIDs) == 0 {
			return cli.Guard(cmd).Err(errfmt.Errorf("must specify target ID(s), --kind, --field, or --all to remove references")).Return()
		}

		sourceID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", sourceRawID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve source ID %s: %w", sourceRawID, err)).Return()
		}

		sourceObj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), sourceID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("source object %s not found: %w", sourceID, err)).Return()
		}

		sourceKind, _ := sourceObj[objects.FieldKeyKind].(string)
		sourceSpec := loadSpecForKind(sourceKind)

		updates := make(map[string]any)
		clearedPlanRef := ""

		if allFlag {
			for k, v := range sourceObj {
				if strings.HasSuffix(k, "_refs") || (strings.HasSuffix(k, "_ref") && k != objects.FieldKeyID) {
					if explicitField != "" && k != explicitField {
						continue
					}
					if isSliceVal(v) {
						updates[k] = []any{}
					} else {
						if k == objects.FieldKeyPriorityPlanRef {
							if oldPlan, ok := v.(string); ok && oldPlan != "" {
								clearedPlanRef = oldPlan
							}
						}
						updates[k] = storage.FieldUnset
					}
				}
			}
		} else if kindFlag != "" {
			targetKindCanonical := strings.ToLower(strings.TrimSpace(kindFlag))
			for k, v := range sourceObj {
				if !strings.HasSuffix(k, "_ref") && !strings.HasSuffix(k, "_refs") {
					continue
				}
				if explicitField != "" && k != explicitField {
					continue
				}
				// Kind-specific match
				if strings.HasPrefix(k, targetKindCanonical+"_") || strings.Contains(k, targetKindCanonical) {
					if isSliceVal(v) {
						updates[k] = []any{}
					} else {
						if k == objects.FieldKeyPriorityPlanRef {
							if oldPlan, ok := v.(string); ok && oldPlan != "" {
								clearedPlanRef = oldPlan
							}
						}
						updates[k] = storage.FieldUnset
					}
				} else if k == objects.FieldKeyRelatedObjectRefs {
					// In related_object_refs, inspect target objects and filter matching kind
					if slice, ok := v.([]any); ok {
						var kept []string
						for _, item := range slice {
							if itemID, ok := item.(string); ok {
								if obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), itemID); err == nil {
									if knd, _ := obj[objects.FieldKeyKind].(string); strings.EqualFold(knd, targetKindCanonical) {
										continue
									}
								}
								kept = append(kept, itemID)
							}
						}
						updates[k] = stringSliceToAny(kept)
					}
				}
			}
		} else if explicitField != "" && len(targetRawIDs) == 0 {
			v, exists := sourceObj[explicitField]
			if exists {
				if isSliceVal(v) {
					updates[explicitField] = []any{}
				} else {
					if explicitField == objects.FieldKeyPriorityPlanRef {
						if oldPlan, ok := v.(string); ok && oldPlan != "" {
							clearedPlanRef = oldPlan
						}
					}
					updates[explicitField] = storage.FieldUnset
				}
			}
		} else {
			// Specific target IDs provided
			targetIDs := make([]string, 0, len(targetRawIDs))
			for _, raw := range targetRawIDs {
				resolved, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", raw)
				if err != nil {
					return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve target ID %s: %w", raw, err)).Return()
				}
				targetIDs = append(targetIDs, resolved)
			}

			// Check for inverted target relationships:
			// e.g. source is priority_plan, target is backlog_item pointing to source
			var directTargetIDs []string
			for _, tid := range targetIDs {
				targetObj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), tid)
				if err != nil {
					directTargetIDs = append(directTargetIDs, tid)
					continue
				}
				targetKind, _ := targetObj[objects.FieldKeyKind].(string)
				if explicitField == "" && !hasSpecificRefField(sourceSpec, sourceObj, targetKind) {
					targetSpec := loadSpecForKind(targetKind)
					if hasSpecificRefField(targetSpec, targetObj, sourceKind) {
						invField, invIsSlice := resolveSpecificRefField(targetSpec, targetObj, sourceKind)
						if invField != "" {
							isReferenced := false
							targetUpdates := make(map[string]any)
							if invIsSlice {
								slice := getRefSlice(targetObj, nil, invField)
								var kept []string
								for _, item := range slice {
									if item != sourceID {
										kept = append(kept, item)
									} else {
										isReferenced = true
									}
								}
								if isReferenced {
									targetUpdates[invField] = stringSliceToAny(kept)
								}
							} else {
								currentVal := getRefScalar(targetObj, nil, invField)
								if currentVal == sourceID {
									isReferenced = true
									targetUpdates[invField] = storage.FieldUnset
								}
							}

							if isReferenced {
								statusChecker := objects.GetGlobalStatusChecker()
								targetStatus, _ := targetObj[objects.FieldKeyStatus].(string)
								if statusChecker.Role(targetKind, targetStatus) == objects.LifecycleRoleShovelReady {
									targetUpdates[objects.FieldKeyStatus] = demoteUnlinkedChildStatus(targetKind, targetStatus)
								}
								updateCtx := proc.OperationContext()
								if err := proc.Storage().Update(updateCtx, proc.SecurityContext(), tid, targetUpdates); err != nil {
									return cli.Guard(cmd).Err(errfmt.Errorf("failed to unlink %s from %s: %w", tid, sourceID, err)).Return()
								}
								if invField == objects.FieldKeyPriorityPlanRef || sourceKind == objects.KindPriorityPlan {
									lifecycle.ApplyPlanChildMembershipRemoved(
										proc.OperationContext(),
										proc.Logger(),
										proc.Storage(),
										proc.ProjectRoot(),
										sourceID,
										tid,
										lifecycle.WithPark(parkFlag),
									)
								}
								_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), targetKind)
								_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), sourceKind)
								proc.TriggerCacheFreshnessCheck("ref_remove", []string{targetKind, sourceKind})

								msg := fmt.Sprintf("✓ Successfully removed %s %s association from %s %s", targetKind, tid, sourceKind, sourceID)
								logging.FluentEvent(proc.Logger()).Info(msg).Log()
								cmd.Println(msg)
								continue
							}
						}
					}
				}
				directTargetIDs = append(directTargetIDs, tid)
			}

			if len(directTargetIDs) == 0 {
				return nil
			}

			targetSet := make(map[string]bool, len(directTargetIDs))
			for _, tid := range directTargetIDs {
				targetSet[tid] = true
			}

			for k, v := range sourceObj {
				if !strings.HasSuffix(k, "_ref") && !strings.HasSuffix(k, "_refs") {
					continue
				}
				if explicitField != "" && k != explicitField {
					continue
				}

				if isSliceVal(v) {
					if slice, ok := v.([]any); ok {
						var kept []string
						for _, item := range slice {
							if itemID, ok := item.(string); ok {
								if !targetSet[itemID] {
									kept = append(kept, itemID)
								}
							}
						}
						if len(kept) != len(slice) {
							if len(kept) == 0 {
								updates[k] = storage.FieldUnset
							} else {
								updates[k] = stringSliceToAny(kept)
							}
						}
					}
				} else {
					if scalarVal, ok := v.(string); ok && (targetSet[scalarVal] || (scalarVal == "" && explicitField == k)) {
						if k == objects.FieldKeyPriorityPlanRef {
							clearedPlanRef = scalarVal
						}
						updates[k] = storage.FieldUnset
					}
				}
			}
		}

		if len(updates) == 0 {
			cmd.Println("No matching references found to remove.")
			return nil
		}

		if clearedPlanRef != "" {
			statusChecker := objects.GetGlobalStatusChecker()
			sourceStatus, _ := sourceObj[objects.FieldKeyStatus].(string)
			if statusChecker.Role(sourceKind, sourceStatus) == objects.LifecycleRoleShovelReady {
				updates[objects.FieldKeyStatus] = demoteUnlinkedChildStatus(sourceKind, sourceStatus)
			}
		}

		updateCtx := proc.OperationContext()
		err = proc.Storage().Update(updateCtx, proc.SecurityContext(), sourceID, updates)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to remove references on %s: %w", sourceID, err)).Return()
		}

		if clearedPlanRef != "" {
			lifecycle.ApplyPlanChildMembershipRemoved(
				proc.OperationContext(),
				proc.Logger(),
				proc.Storage(),
				proc.ProjectRoot(),
				clearedPlanRef,
				sourceID,
				lifecycle.WithPark(parkFlag),
			)
			_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), objects.KindPriorityPlan)
			_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), sourceKind)
			proc.TriggerCacheFreshnessCheck("ref_remove", []string{sourceKind, objects.KindPriorityPlan})
		}

		msg := fmt.Sprintf("✓ Successfully removed reference(s) from %s", sourceID)
		logging.FluentEvent(proc.Logger()).Info(msg).Log()
		cmd.Println(msg)
		return nil
	})(cmd, args)
}

// demoteUnlinkedChildStatus finds the demoted status when an unlinked child leaves shovel_ready.
// Uses lifecycle roles dynamically: returns the highest-ranking grooming or realign status.
func demoteUnlinkedChildStatus(childKind, currentStatus string) string {
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		if childKind == objects.KindBacklogItem {
			return "validated"
		}
		return "grooming"
	}

	// 1. Try LifecycleRoleGrooming
	if statuses, err := loader.StatusesForRole(childKind, objects.LifecycleRoleGrooming); err == nil && len(statuses) > 0 {
		return statuses[0]
	}

	// 2. Try LifecycleRoleRealign (e.g. backlog_item has "validated", "exploring")
	if statuses, err := loader.StatusesForRole(childKind, objects.LifecycleRoleRealign); err == nil && len(statuses) > 0 {
		for _, s := range statuses {
			if s == "validated" {
				return s
			}
		}
		return statuses[0]
	}

	if childKind == objects.KindBacklogItem {
		return "validated"
	}
	return "grooming"
}

// hasSpecificRefField reports whether kind/obj contains a typed reference field for targetKind
// (i.e. not the generic related_object_refs).
func hasSpecificRefField(spec *objects.Spec, sourceObj map[string]any, targetKind string) bool {
	f, _ := resolveSpecificRefField(spec, sourceObj, targetKind)
	return f != ""
}

// resolveSpecificRefField resolves a typed reference field (non-related_object_refs) for targetKind.
func resolveSpecificRefField(spec *objects.Spec, sourceObj map[string]any, targetKind string) (string, bool) {
	kindClean := strings.ToLower(strings.TrimSpace(targetKind))

	hasField := func(name string) bool {
		if spec != nil {
			if spec.ResolvedFields != nil {
				if _, ok := spec.ResolvedFields[name]; ok {
					return true
				}
			}
			if spec.Fields != nil {
				if _, ok := spec.Fields[name]; ok {
					return true
				}
			}
		}
		if _, ok := sourceObj[name]; ok {
			return true
		}
		return false
	}

	// 1. Exact typed slice: <target_kind>_refs
	pluralField := kindClean + "_refs"
	if hasField(pluralField) {
		return pluralField, true
	}

	// 2. Exact typed scalar: <target_kind>_ref
	singularField := kindClean + "_ref"
	if hasField(singularField) {
		return singularField, false
	}

	// 3. Known ontology synonyms / mappings
	synonymMap := map[string][]struct {
		field   string
		isSlice bool
	}{
		"criteria": {
			{"criteria_refs", true},
			{"validation_criteria_refs", true},
		},
		"test_case": {
			{"test_case_refs", true},
			{"test_refs", true},
		},
		"persona": {
			{"assignee_persona_ref", false},
			{"stakeholder_persona_refs", true},
		},
		"backlog_item": {
			{"backlog_item_ref", false},
			{"backlog_item_refs", true},
		},
		"priority_plan": {
			{"priority_plan_ref", false},
			{"priority_plan_refs", true},
		},
		"policy": {
			{"policy_refs", true},
		},
		"goal": {
			{"goal_refs", true},
		},
		"requirement": {
			{"requirement_refs", true},
		},
		"workstream": {
			{"workstream_ref", false},
			{"workstream_refs", true},
		},
	}

	if syns, ok := synonymMap[kindClean]; ok {
		for _, s := range syns {
			if hasField(s.field) {
				return s.field, s.isSlice
			}
		}
	}

	return "", false
}

// resolveRefFieldForTarget determines the appropriate reference field on source for targetKind.
func resolveRefFieldForTarget(spec *objects.Spec, sourceObj map[string]any, targetKind, targetID string) (field string, isSlice bool, err error) {
	if f, isSlice := resolveSpecificRefField(spec, sourceObj, targetKind); f != "" {
		return f, isSlice, nil
	}

	// Fallback to generic related_object_refs
	hasField := func(name string) bool {
		if spec != nil {
			if spec.ResolvedFields != nil {
				if _, ok := spec.ResolvedFields[name]; ok {
					return true
				}
			}
			if spec.Fields != nil {
				if _, ok := spec.Fields[name]; ok {
					return true
				}
			}
		}
		if _, ok := sourceObj[name]; ok {
			return true
		}
		return false
	}

	if hasField(objects.FieldKeyRelatedObjectRefs) {
		return objects.FieldKeyRelatedObjectRefs, true, nil
	}

	sourceKind, _ := sourceObj[objects.FieldKeyKind].(string)
	return "", false, errfmt.Errorf("no compatible reference field found on %s (kind: %s) for target %s (kind: %s)", sourceObj[objects.FieldKeyID], sourceKind, targetID, targetKind)
}

func isSliceField(spec *objects.Spec, sourceObj map[string]any, field string) bool {
	if spec != nil {
		if spec.ResolvedFields != nil {
			if fDef, ok := spec.ResolvedFields[field].(map[string]any); ok {
				if typeVal, ok := fDef["type"].(string); ok && (typeVal == "list" || typeVal == "array") {
					return true
				}
			}
		}
		if spec.Fields != nil {
			if fDef, ok := spec.Fields[field].(map[string]any); ok {
				if typeVal, ok := fDef["type"].(string); ok && (typeVal == "list" || typeVal == "array") {
					return true
				}
			}
		}
	}
	if v, ok := sourceObj[field]; ok {
		return isSliceVal(v)
	}
	return strings.HasSuffix(field, "_refs")
}

func isSliceVal(v any) bool {
	if v == nil {
		return false
	}
	switch v.(type) {
	case []any, []string:
		return true
	default:
		return false
	}
}

func getRefSlice(sourceObj, updates map[string]any, field string) []string {
	var raw any
	if u, ok := updates[field]; ok {
		raw = u
	} else {
		raw = sourceObj[field]
	}
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func getRefScalar(sourceObj, updates map[string]any, field string) string {
	if u, ok := updates[field]; ok {
		if s, ok := u.(string); ok {
			return s
		}
	}
	if s, ok := sourceObj[field].(string); ok {
		return s
	}
	return ""
}

func findInTypedRefFields(sourceObj, updates map[string]any, targetID string) string {
	checkObj := func(m map[string]any) string {
		for k, v := range m {
			if k == objects.FieldKeyRelatedObjectRefs || (!strings.HasSuffix(k, "_ref") && !strings.HasSuffix(k, "_refs")) {
				continue
			}
			switch val := v.(type) {
			case string:
				if val == targetID {
					return k
				}
			case []any:
				for _, item := range val {
					if is, ok := item.(string); ok && is == targetID {
						return k
					}
				}
			case []string:
				for _, is := range val {
					if is == targetID {
						return k
					}
				}
			}
		}
		return ""
	}

	if f := checkObj(updates); f != "" {
		return f
	}
	return checkObj(sourceObj)
}

// hasTransitiveCycle performs bounded BFS to detect if targetID reaches sourceID via related_object_refs.
func hasTransitiveCycle(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider, startID, goalID string, maxDepth int) bool {
	visited := make(map[string]bool)
	queue := []string{startID}
	depth := 0

	for len(queue) > 0 && depth < maxDepth {
		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			curr := queue[0]
			queue = queue[1:]

			if curr == goalID {
				return true
			}
			if visited[curr] {
				continue
			}
			visited[curr] = true

			obj, err := store.Read(ctx, secCtx, curr)
			if err != nil {
				continue
			}
			if related, ok := obj[objects.FieldKeyRelatedObjectRefs].([]any); ok {
				for _, r := range related {
					if rs, ok := r.(string); ok && !visited[rs] {
						queue = append(queue, rs)
					}
				}
			}
		}
		depth++
	}
	return false
}

func cleanLegacyKeys(existingObj map[string]any, updates map[string]any) {
	legacyFields := []string{
		"category", "phase", "date_captured", "group", "origin_project", "origin_system",
		"acceptance_criteria", "spec_adherence", "context", "date", "spec_refs",
		"collection_count", "cron_restarts", "first_seen", "health_check_duration_ms",
		"health_checks", "last_seen", "measurement_window_end", "measurement_window_start",
		"metric_type", "missed_triggers", "recovered_jobs", "estimated_effort",
		"event_type", "operation", "benefits", "components", "considerations",
	}
	for _, f := range legacyFields {
		if _, ok := existingObj[f]; ok {
			updates[f] = storage.FieldUnset
		}
	}
}

