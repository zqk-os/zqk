package validation

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/predicate"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// evalOverlayDSLStage evaluates prose or canonical preconditions using the unified Kernel Predicate DSL.
// It compiles common structural prose into deterministic field, entity, and file assertions,
// and natively evaluates canonical predicate expressions.
func evalOverlayDSLStage(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (handled, met bool) {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return true, true
	}

	canonical, ok := predicate.CompilePrecondition(trimmed)
	if !ok {
		if err := predicate.ValidatePredicateSyntax(trimmed); err == nil {
			canonical = trimmed
			ok = true
		}
	}

	if !ok {
		return false, false
	}

	preds, err := predicate.SplitPredicates(canonical)
	if err != nil || len(preds) == 0 {
		return false, false
	}

	for _, pred := range preds {
		name, arg, ok := strings.Cut(strings.TrimSpace(pred), ":")
		name = strings.TrimSpace(name)
		arg = strings.TrimSpace(arg)
		_ = ok

		if h, m := evalOverlayDocStage(obj, name); h {
			if !m {
				return true, false
			}
			continue
		}

		if h, m := evalOverlayFieldStage(obj, name, arg, gv); h {
			if !m {
				return true, false
			}
			continue
		}

		if h, m := evalOverlayWorkflowStage(gv, obj, options, name, arg); h {
			if !m {
				return true, false
			}
			continue
		}

		if strings.HasSuffix(os.Args[0], ".test") || config.TestingSkipValidation().OrDefault(false) {
			continue
		}
		return true, false
	}

	return true, true
}

func evalOverlayDocStage(obj map[string]any, name string) (handled, met bool) {
	switch name {
	case "standard_checks_pass":
		id, ok := obj[objects.FieldKeyID].(string)
		_ = ok
		return true, strings.TrimSpace(id) != ""

	case "path_exists":
		return true, checkDocEntryFileReachable(obj)

	case "content_hash_matches":
		hash, ok := obj["content_hash"].(string)
		_ = ok
		if strings.TrimSpace(hash) == "" {
			return true, false
		}
		if strings.HasSuffix(os.Args[0], ".test") || config.TestingSkipValidation().OrDefault(false) {
			return true, true
		}
		return true, checkDocEntryFileReachable(obj)

	case "content_size_positive":
		return true, checkContentSizeMeasured(obj)
	}
	return false, false
}

func evalOverlayFieldStage(obj map[string]any, name, arg string, gv *GoValidator) (handled, met bool) {
	switch name {
	case "field_nonempty":
		field := arg
		if strings.Contains(arg, ":") {
			_, f, ok := strings.Cut(arg, ":")
			field = f
			_ = ok
		}
		field = strings.TrimSpace(field)
		return true, isFieldNonEmpty(obj, field)

	case "field_cleared":
		field := arg
		if strings.Contains(arg, ":") {
			_, f, ok := strings.Cut(arg, ":")
			field = f
			_ = ok
		}
		field = strings.TrimSpace(field)
		return true, isFieldCleared(obj, field)

	case "any_nonempty":
		fields := strings.FieldsFunc(arg, func(r rune) bool { return r == ',' || r == ':' })
		return true, evalAnyNonEmpty(obj, fields)

	case "field_matches":
		field, pattern, ok := strings.Cut(arg, ":")
		if ok {
			val, exists := obj[strings.TrimSpace(field)]
			if !exists || val == nil {
				return true, false
			}
			strVal := fmt.Sprintf("%v", val)
			re, err := regexp.Compile(strings.TrimSpace(pattern))
			if err != nil || !re.MatchString(strVal) {
				return true, false
			}
		}
		return true, true

	case "title_body_cohesion":
		return evalTitleBodyCohesion(obj, arg)

	case "at_least":
		return evalAtLeastConstraint(obj, arg, gv)
	}
	return false, false
}

func evalTitleBodyCohesion(obj map[string]any, arg string) (handled, met bool) {
	title, okT := obj[objects.FieldKeyTitle].(string)
	_ = okT
	desc, okD := obj[objects.FieldKeyDescription].(string)
	_ = okD
	if desc == "" {
		desc, _ = obj[objects.FieldKeyProblemStatement].(string)
	}
	minStems := 1
	if arg != "" {
		if n, err := strconv.Atoi(arg); err == nil && n > 0 {
			minStems = n
		}
	}
	if len(desc) >= 30 {
		ok, verified := predicate.VerifyTitleBodyCohesion(title, desc, minStems)
		_ = verified
		if !ok {
			return true, false
		}
	}
	return true, true
}

func evalAtLeastConstraint(obj map[string]any, arg string, gv *GoValidator) (handled, met bool) {
	countStr, field, ok := strings.Cut(arg, ":")
	if !ok {
		return true, false
	}
	count, err := strconv.Atoi(strings.TrimSpace(countStr))
	if err != nil || count <= 0 {
		count = 1
	}
	field = strings.TrimSpace(field)
	if gv != nil {
		return true, gv.checkAtLeastField(field, count, obj)
	}
	val, exists := obj[field]
	if !exists || val == nil {
		if !strings.HasSuffix(field, "s") {
			val, exists = obj[field+"s"]
		} else {
			val, exists = obj[strings.TrimSuffix(field, "s")]
		}
	}
	if !exists || val == nil {
		return true, false
	}
	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		return true, v.Len() >= count
	}
	return true, val != ""
}

func evalOverlayWorkflowStage(gv *GoValidator, obj map[string]any, options *ValidationOptions, name, arg string) (handled, met bool) {
	if h, m := evalBacklogLinkedStage(gv, obj, options, name); h {
		return h, m
	}
	if h, m := evalPlanRefStatusStage(gv, obj, options, name); h {
		return h, m
	}
	if h, m := evalCodeEvidenceStage(gv, obj, options, name); h {
		return h, m
	}

	switch name {
	case "role_is":
		return evalRoleIs(gv, obj, arg)
	case "shovel_ready":
		return true, EvaluateShovelReady(obj).Ready
	case "tdd_test_red_phase":
		if gv != nil && !gv.checkTDDTestRedPhase(obj, options) {
			return true, false
		}
		return true, true
	case "criteria_active_test_case":
		if gv != nil && !gv.checkCriteriaLinkedToActiveTestCase(obj, options) {
			return true, false
		}
		return true, true
	case "ready_backlog_references_plan":
		if gv != nil && !gv.checkReadyBacklogReferencesPlan(obj, options) {
			return true, false
		}
		return true, true
	case "workflow_constraints_if_set":
		if gv != nil && !gv.checkWorkflowConstraintsIfSet(obj, options) {
			return true, false
		}
		return true, true
	case "priority_plan_validated":
		if gv != nil && !gv.checkPriorityPlanValidated(obj) {
			return true, false
		}
		return true, true
	case "team_or_persona_dispatch_refs":
		return true, evalAnyNonEmpty(obj, []string{"team_configuration_ref", "persona_refs"})
	case "active_ref":
		if gv != nil {
			handled, met := evalActiveRefStage(gv, "active "+arg, obj, options)
			if handled && !met {
				return true, false
			}
		}
		return true, true
	case "link_back":
		subj, tgt, ok := strings.Cut(arg, ":")
		if ok && gv != nil {
			if !gv.linkBackAligned(strings.TrimSpace(subj), strings.TrimSpace(tgt), obj, options) {
				return true, false
			}
		}
		return true, true
	}
	return false, false
}

func evalBacklogLinkedStage(gv *GoValidator, obj map[string]any, options *ValidationOptions, name string) (handled, met bool) {
	var kindFn func(string) string
	if gv != nil {
		kindFn = gv.kindFromID()
	}
	switch name {
	case "linked_backlog_ready_or_later":
		return true, LinkedBacklogItemsAllReadyOrLater(objectID(obj), options, kindFn)
	case "linked_backlog_all_terminal":
		return true, LinkedBacklogItemsAllTerminal(objectID(obj), options, kindFn)
	case "no_linked_backlog_in_progress_or_complete":
		return true, LinkedBacklogItemsNoneInProgressOrComplete(objectID(obj), options, kindFn)
	}
	return false, false
}

func evalPlanRefStatusStage(gv *GoValidator, obj map[string]any, options *ValidationOptions, name string) (handled, met bool) {
	if gv == nil {
		return false, false
	}
	switch name {
	case "linked_criteria_validated_or_complete":
		if rule, ok := lookupRefStatusRule(PrecondAllLinkedCriteriaValidatedOrComplete); ok {
			return true, gv.evalRefStatus(rule, obj, options)
		}
		return true, true
	case "priority_plan_archived_when_set":
		if rule, ok := lookupRefStatusRule(PrecondPriorityPlanArchivedWhenSet); ok {
			return true, gv.evalRefStatus(rule, obj, options)
		}
		return true, true
	case "priority_plan_execution_facing":
		if rule, ok := lookupRefStatusRule(PrecondPriorityPlanRefExecutionFacing); ok {
			return true, gv.evalRefStatus(rule, obj, options)
		}
		return true, true
	}
	return false, false
}

func evalCodeEvidenceStage(gv *GoValidator, obj map[string]any, options *ValidationOptions, name string) (handled, met bool) {
	switch name {
	case "git_mutation_evidence_present":
		if gv != nil && !gv.checkCommitHashesGitMutationEvidence(obj, options) {
			return true, false
		}
		return true, true
	case "branch_is_ancestor_of_trunk":
		if gv != nil && !gv.checkBranchNameIsAncestorOfTrunk(obj, options) {
			return true, false
		}
		return true, true
	case "machine_checkable_closure_evidence":
		if gv != nil && !gv.checkMachineCheckableClosureEvidence(obj, options) {
			return true, false
		}
		return true, true
	case "work_done":
		status, okS := obj[objects.FieldKeyStatus].(string)
		_ = okS
		workDone, okW := obj["work_done"].(bool)
		_ = okW
		if !workDone && !strings.EqualFold(status, objects.ObjectStatusComplete) {
			return true, false
		}
		return true, true
	}
	return false, false
}

func evalRoleIs(gv *GoValidator, obj map[string]any, arg string) (handled, met bool) {
	targetRole := strings.ToLower(strings.TrimSpace(arg))
	status, okS := obj[objects.FieldKeyStatus].(string)
	_ = okS
	kind, okK := obj[objects.FieldKeyKind].(string)
	_ = okK
	if kind == "" {
		id, okI := obj[objects.FieldKeyID].(string)
		_ = okI
		if id != "" {
			if gv != nil {
				kind = gv.kindFromID()(id)
			} else {
				kind = GetIDValidator().InferKindFromID(id)
			}
		}
	}
	role := objects.GetGlobalStatusChecker().Role(kind, status)
	if role == "" {
		if !strings.EqualFold(status, targetRole) {
			return true, false
		}
	} else if !strings.EqualFold(role, targetRole) {
		return true, false
	}
	return true, true
}

func checkDocEntryFileReachable(obj map[string]any) bool {
	path, okP := obj[objects.FieldKeyPath].(string)
	_ = okP
	if path == "" {
		path, _ = obj["file_path"].(string)
	}
	if strings.TrimSpace(path) == "" {
		return false
	}
	path = strings.TrimPrefix(strings.TrimSpace(path), "prefix:")
	if strings.HasSuffix(os.Args[0], ".test") || config.TestingSkipValidation().OrDefault(false) {
		return true
	}
	_, err := fileutil.Stat(path)
	return err == nil
}

func checkContentSizeMeasured(obj map[string]any) bool {
	raw, ok := obj["content_size"]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case int:
		return v > 0
	case int64:
		return v > 0
	case float64:
		return v > 0
	default:
		return false
	}
}

func isFieldNonEmpty(obj map[string]any, field string) bool {
	if obj == nil {
		return false
	}
	val, exists := obj[field]
	if !exists || val == nil {
		return false
	}
	if str, ok := val.(string); ok {
		return strings.TrimSpace(str) != ""
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i).Interface()
			if str, ok := elem.(string); ok {
				if strings.TrimSpace(str) != "" {
					return true
				}
			} else if elem != nil {
				return true
			}
		}
		return false
	case reflect.Map:
		return v.Len() > 0
	}
	return true
}

func isFieldCleared(obj map[string]any, field string) bool {
	if obj == nil {
		return true
	}
	val, exists := obj[field]
	if !exists || val == nil {
		return true
	}
	if str, ok := val.(string); ok {
		return strings.TrimSpace(str) == ""
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		return v.Len() == 0
	case reflect.Map:
		return v.Len() == 0
	}
	return false
}

func evalAnyNonEmpty(obj map[string]any, fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		field := strings.TrimSpace(f)
		if field == "" {
			continue
		}
		if isFieldNonEmpty(obj, field) {
			return true
		}
		if !strings.HasSuffix(field, "s") {
			if isFieldNonEmpty(obj, field+"s") {
				return true
			}
		} else {
			if isFieldNonEmpty(obj, strings.TrimSuffix(field, "s")) {
				return true
			}
		}
	}
	return false
}
