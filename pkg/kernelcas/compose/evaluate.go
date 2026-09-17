package compose

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/shovelready"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DecideOutcome is the result of evaluating DECIDE rules for a mutation.
type DecideOutcome struct {
	Plan          string
	LifecycleOK   bool
	ErasePolicy   string
	BreakGlass    string
	RefuseReason  string
	UnlinkPlanned bool
}

// MutationInput is the minimal envelope for DECIDE (avoids importing Mutation cycle details).
type MutationInput struct {
	Kind   string
	ID     string
	Intent string
	Reason string
}

// Decide evaluates the composed definition for a mutation.
func Decide(ctx context.Context, reg *Registry, pipelineKind string, in MutationInput) DecideOutcome {
	if reg == nil {
		reg = Default()
	}
	if err := WarmDefaultRegistry(""); err != nil {
		panic("WarmDefaultRegistry failed: " + err.Error())
	}
	def, ok := reg.GetByParts(in.Kind, pipelineKind, in.Intent)
	if !ok || def == nil {
		// Fail closed for critical kinds once registry is warm; allow empty registry during early boot.
		if reg.Len() > 0 && IsCriticalObjectKind(in.Kind) {
			return DecideOutcome{
				Plan:         PlanRefuse,
				RefuseReason: fmt.Sprintf("no composed pipeline_definition for %s × %s (fail closed)", in.Kind, pipelineKind),
			}
		}
		// Non-critical or unwarmed: legacy-compatible allow.
		if strings.TrimSpace(in.Reason) != "" {
			return DecideOutcome{Plan: PlanBreakGlass, LifecycleOK: true, BreakGlass: in.Reason}
		}
		return DecideOutcome{Plan: PlanCasSync, LifecycleOK: true}
	}

	out := DecideOutcome{LifecycleOK: true, Plan: PlanCasSync}
	for _, st := range def.Stages {
		if st.Name != pipeline.StageDecide {
			continue
		}
		for _, rule := range st.Rules {
			applyDecideRule(ctx, &out, in, rule)
			if out.Plan == PlanRefuse {
				return out
			}
		}
	}
	return out
}

func applyDecideRule(ctx context.Context, out *DecideOutcome, in MutationInput, rule Rule) {
	switch rule.Op {
	case OpBreakGlassIfReason:
		if strings.TrimSpace(in.Reason) != "" {
			out.Plan = PlanBreakGlass
			out.BreakGlass = in.Reason
		}
	case OpAllowCasSync:
		if out.Plan == "" || out.Plan == PlanCasSync {
			if strings.TrimSpace(in.Reason) == "" {
				out.Plan = PlanCasSync
			}
		}
	case OpEraseCriticalPolicy:
		if IsCriticalObjectKind(in.Kind) {
			// Fail closed without audited reason / AllowCoreObjectDelete / elevated delete.
			// Do NOT treat ZQK_TEST_ROOT as a Decide bypass — that made
			// TestDecide_eraseCriticalRefusesWithoutReason flake under scheduler
			// (plan=break_glass instead of refuse). TRACK: BLI-1785723654802038000-b14064bc
			elevated := pkgctx.MayHardDeleteCoreWithoutReason(pkgctx.GetSecurityContext(ctx))
			allowed := pkgctx.GetAllowCoreObjectDelete(ctx) || strings.TrimSpace(in.Reason) != "" || elevated
			if !allowed {
				out.Plan = PlanRefuse
				out.ErasePolicy = ErasePolicyRefuse
				out.RefuseReason = "hard delete refused for core/critical kind; archive+aggregate, pass --reason-code, or use elevated delete:*/delete:core"
				return
			}
			out.Plan = PlanBreakGlass
			out.ErasePolicy = ErasePolicyBreak
			out.UnlinkPlanned = true
			if strings.TrimSpace(in.Reason) != "" {
				out.BreakGlass = in.Reason
			} else if elevated {
				out.BreakGlass = "elevated_delete_privilege"
			} else {
				out.BreakGlass = "AllowCoreObjectDelete"
			}
			return
		}
		out.Plan = PlanEraseUnlink
		out.ErasePolicy = ErasePolicyAllow
		out.UnlinkPlanned = true
	case OpReconcileIndexOnly:
		out.Plan = PlanReindexOnly
		out.ErasePolicy = ErasePolicyRefuse
	case OpBlobGC:
		out.Plan = PlanBlobGC
	case OpLifecyclePreconditions:
		// Attached for observability / materialize. Transition and status-hold
		// tokens are evaluated by GoValidator.dispatchPrecondition on the storage
		// save path (Plane A). TRACK: BLI-1785439365092316000-2c09c364
	default:
		// Object-shape overlay rules are evaluated on the object during validation, not here.
	}
}

// ValidationError is a minimal error shape for validation bridge (avoids importing validation package).
type ValidationError struct {
	Field   string
	Message string
	Rule    string
}

// ObjectLookup resolves an object by ID for plan-status checks.
type ObjectLookup func(id string) (map[string]any, error)

// ValidateObject runs composed overlay rules for an object (replaces customRuleValidators).
// Uses UpdateFields definition by default.
func ValidateObject(ctx context.Context, reg *Registry, kind string, obj map[string]any, lookup ObjectLookup) []ValidationError {
	return ValidateObjectIntent(ctx, reg, kind, KindUpdate, IntentUpdateFields, obj, lookup)
}

// ValidateObjectIntent evaluates DECIDE overlay rules for a specific pipeline kind×intent.
func ValidateObjectIntent(ctx context.Context, reg *Registry, kind, pipelineKind, intent string, obj map[string]any, lookup ObjectLookup) []ValidationError {
	if reg == nil {
		reg = Default()
	}
	if err := WarmDefaultRegistry(""); err != nil {
		panic("WarmDefaultRegistry failed: " + err.Error())
	}
	def, ok := reg.GetByParts(kind, pipelineKind, intent)
	if !ok || def == nil {
		c := NewCompiler("")
		if d, err := c.Compile(kind, pipelineKind, intent); err == nil && d != nil {
			reg.Put(d)
			def = d
		} else {
			return nil
		}
	}
	var errs []ValidationError
	for _, st := range def.Stages {
		if st.Name != pipeline.StageDecide {
			continue
		}
		for _, rule := range st.Rules {
			errs = append(errs, evalOverlayRule(ctx, kind, obj, lookup, rule)...)
		}
	}
	return errs
}

func evalOverlayRule(ctx context.Context, kind string, obj map[string]any, lookup ObjectLookup, rule Rule) []ValidationError {
	cfg := rule.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	switch rule.Op {
	case OpRequireRefAny:
		return evalRequireRefAny(ctx, kind, obj, cfg)
	case OpRequireFieldWhenStatus:
		return evalRequireFieldWhenStatus(obj, cfg)
	case OpRequireField:
		return evalRequireField(ctx, kind, obj, cfg)
	case OpRefusePlanStatus:
		return evalRefusePlanStatus(ctx, obj, lookup, cfg)
	case OpRequireFieldWhenActive:
		return evalRequireFieldWhenActive(kind, obj, cfg)
	case OpMinStringLen:
		return evalMinStringLen(obj, cfg)
	case OpRefuseExecutionFacingMembership:
		return evalRefuseExecutionFacingMembership(ctx, kind, obj, lookup, cfg)
	case OpRefuseFieldPresent:
		return evalRefuseFieldPresent(obj, cfg)
	case OpRefuseFieldWhenStatus:
		return evalRefuseFieldWhenStatus(obj, cfg)
	case OpRefuseRefPrefix:
		return evalRefuseRefPrefix(obj, cfg)
	case OpRefuseChildStatus:
		return evalRefuseChildStatus(obj, lookup, cfg)
	case OpRefuseSelfRef:
		return evalRefuseSelfRef(obj, cfg)
	case OpRefuseTwoCycle:
		return evalRefuseTwoCycle(obj, lookup, cfg)
	case OpShovelReadyWhenStatus:
		return evalShovelReadyWhenStatus(obj, cfg)
	case OpRefuseDuplicateRefs:
		return evalRefuseDuplicateRefs(obj, cfg)
	case OpRefuseUnknownFields:
		return evalRefuseUnknownFields(kind, obj, cfg)
	case OpValidatePriorityValues:
		return evalValidatePriorityValues(obj, cfg)
	default:
		return nil
	}
}

func evalRefuseFieldPresent(obj map[string]any, cfg map[string]any) []ValidationError {
	field, _ := cfg[objects.FieldKeyField].(string)
	if field == "" {
		return nil
	}
	if _, ok := obj[field]; !ok {
		return nil
	}
	msg, _ := cfg["message"].(string)
	if msg == "" {
		msg = fmt.Sprintf("field %s must not be present", field)
	}
	return []ValidationError{{
		Field:   field,
		Message: msg,
		Rule:    "composed_integrity",
	}}
}

func evalRefuseRefPrefix(obj map[string]any, cfg map[string]any) []ValidationError {
	field, _ := cfg[objects.FieldKeyField].(string)
	prefix, _ := cfg["prefix"].(string)
	if field == "" || prefix == "" {
		return nil
	}
	for _, ref := range stringRefs(obj[field]) {
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		msg, _ := cfg["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("%s must not contain %s references", field, prefix)
		}
		return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
	}
	return nil
}

func stringRefs(value any) []string {
	switch refs := value.(type) {
	case string:
		if refs == "" {
			return nil
		}
		return []string{refs}
	case []string:
		return refs
	case []any:
		out := make([]string, 0, len(refs))
		for _, ref := range refs {
			if s, ok := ref.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func evalRefuseDuplicateRefs(obj map[string]any, cfg map[string]any) []ValidationError {
	var errs []ValidationError
	refOccurrences := make(map[string]string) // targetID -> first field name
	seenInField := make(map[string]map[string]bool)

	// Ref fields to inspect: any field ending with _refs or _ref, or named dependencies
	for k, v := range obj {
		if !strings.HasSuffix(k, "_refs") && !strings.HasSuffix(k, "_ref") && k != "dependencies" {
			continue
		}
		seenInField[k] = make(map[string]bool)
		for _, targetID := range stringRefs(v) {
			targetID = strings.TrimSpace(targetID)
			if targetID == "" {
				continue
			}
			if seenInField[k][targetID] {
				errs = append(errs, ValidationError{
					Field:   k,
					Message: fmt.Sprintf("duplicate reference %q within %s", targetID, k),
					Rule:    "duplicate_reference",
				})
				continue
			}
			seenInField[k][targetID] = true

			if firstField, ok := refOccurrences[targetID]; ok {
				errs = append(errs, ValidationError{
					Field:   k,
					Message: fmt.Sprintf("duplicate reference %q: target ID appears in both %s and %s", targetID, firstField, k),
					Rule:    "intra_object_duplicate_ref",
				})
			} else {
				refOccurrences[targetID] = k
			}
		}
	}
	return errs
}

func evalRefuseUnknownFields(kind string, obj map[string]any, cfg map[string]any) []ValidationError {
	loader := objects.NewSpecLoader("")
	spec, err := loader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil || spec == nil || spec.ResolvedFields == nil {
		return nil
	}
	var errs []ValidationError
	for k := range obj {
		if _, ok := spec.ResolvedFields[k]; !ok {
			if objects.IsCompositionFieldAllowed(kind, k, spec) {
				continue
			}
			errs = append(errs, ValidationError{
				Field:   k,
				Message: fmt.Sprintf("field %q is not defined on resolved spec for kind %s", k, kind),
				Rule:    "unknown_field",
			})
		}
	}
	return errs
}

func evalRefuseSelfRef(obj map[string]any, cfg map[string]any) []ValidationError {
	field, _ := cfg[objects.FieldKeyField].(string)
	id, _ := obj[objects.FieldKeyID].(string)
	if field == "" || id == "" {
		return nil
	}
	for _, ref := range stringRefs(obj[field]) {
		if ref != id {
			continue
		}
		msg, _ := cfg["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("%s must not contain this object's id", field)
		}
		return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
	}
	return nil
}

func evalRefuseTwoCycle(obj map[string]any, lookup ObjectLookup, cfg map[string]any) []ValidationError {
	field, _ := cfg[objects.FieldKeyField].(string)
	id, _ := obj[objects.FieldKeyID].(string)
	if field == "" || id == "" || lookup == nil {
		return nil
	}
	for _, peer := range stringRefs(obj[field]) {
		if peer == "" || peer == id {
			continue
		}
		peerObj, err := lookup(peer)
		if err != nil || peerObj == nil {
			continue
		}
		for _, back := range stringRefs(peerObj[field]) {
			if back != id {
				continue
			}
			msg, _ := cfg["message"].(string)
			if msg == "" {
				msg = fmt.Sprintf("%s must not form a 2-cycle", field)
			}
			return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
		}
	}
	return nil
}

func evalRefuseChildStatus(obj map[string]any, lookup ObjectLookup, cfg map[string]any) []ValidationError {
	parentStatus, _ := obj[objects.FieldKeyStatus].(string)
	whenParentStatus, _ := cfg["when_parent_status"].([]any)
	match := false
	for _, s := range whenParentStatus {
		if ss, ok := s.(string); ok && ss == parentStatus {
			match = true
			break
		}
	}
	if !match {
		return nil
	}
	childField, _ := cfg["child_field"].(string)
	refuseChildStatus, _ := cfg["refuse_child_status"].([]any)
	var childRefs []any
	switch v := obj[childField].(type) {
	case []any:
		childRefs = v
	case []string:
		for _, s := range v {
			childRefs = append(childRefs, s)
		}
	}
	var errs []ValidationError
	for _, childRef := range childRefs {
		childID, _ := childRef.(string)
		if childID == "" {
			continue
		}
		childStatus := lookupPlanStatus(childID, lookup)
		for _, s := range refuseChildStatus {
			if ss, ok := s.(string); ok && ss == childStatus {
				msgFmt, _ := cfg["message_fmt"].(string)
				if msgFmt == "" {
					msgFmt = "child %s is in forbidden status '%s'"
				}
				errs = append(errs, ValidationError{
					Field:   childField,
					Message: fmt.Sprintf(msgFmt, childID, childStatus),
					Rule:    "composed_integrity",
				})
			}
		}
	}
	return errs
}

func evalRefuseExecutionFacingMembership(ctx context.Context, kind string, obj map[string]any, lookup ObjectLookup, cfg map[string]any) []ValidationError {
	if pkgctx.IsLifecycleBreakGlass(ctx) {
		return nil
	}
	planField, _ := cfg["plan_field"].(string)
	if planField == "" {
		planField = objects.FieldKeyPriorityPlanRef
	}
	planID, _ := obj[planField].(string)
	if strings.TrimSpace(planID) == "" {
		return nil
	}
	status, _ := obj[objects.FieldKeyStatus].(string)
	objKind := kind
	if objKind == "" {
		objKind = objects.KindBacklogItem
	}
	sc := objects.GetGlobalStatusChecker()
	role := sc.Role(objKind, status)
	allowed := false
	if role != "" {
		allowed = objects.RoleAllowedOnExecutionFacingPlan(role)
	} else {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case objects.ObjectStatusPlanned, objects.ObjectStatusInProgress,
			objects.ObjectStatusComplete, objects.ObjectStatusArchived, objects.ObjectStatusError:
			allowed = true
		}
	}
	if allowed {
		return nil
	}
	if status == "originated" || status == "validated" || status == "exploring" {
		if id, _ := obj[objects.FieldKeyID].(string); id != "" && lookup != nil {
			if old, err := lookup(id); err == nil && old != nil {
				oldRef, _ := old[planField].(string)
				if oldRef == planID {
					return nil
				}
			}
		}
	}
	planStatus := lookupPlanStatus(planID, lookup)
	if planStatus == "" {
		return nil
	}
	planRole := sc.Role(objects.KindPriorityPlan, planStatus)
	requiresReady := false
	if planRole != "" {
		requiresReady = objects.RolePlanRequiresReadyChildren(planRole)
	} else {
		switch strings.ToLower(strings.TrimSpace(planStatus)) {
		case objects.ObjectStatusActive, objects.ObjectStatusInProgress:
			requiresReady = true
		}
	}
	if !requiresReady {
		return nil
	}
	msgFmt, _ := cfg["message_fmt"].(string)
	if msgFmt == "" {
		msgFmt = "backlog item status %q cannot link to %s priority plan %s"
	}
	// Distinct rule id so system-check can map this to tier 1 (blocking summary).
	// Generic composed_integrity defaults to warning and disappears from agent "pristine" glances.
	// TRACK: BLI-KERNEL-CHECK-UNBLIND-MEMBERSHIP-001
	return []ValidationError{{
		Field:   objects.FieldKeyStatus,
		Message: fmt.Sprintf(msgFmt, strings.ToLower(strings.TrimSpace(status)), strings.ToLower(strings.TrimSpace(planStatus)), planID),
		Rule:    "execution_facing_membership",
	}}
}

func evalRequireRefAny(ctx context.Context, kind string, obj map[string]any, cfg map[string]any) []ValidationError {
	fields, _ := cfg["fields"].([]any)
	status, _ := obj[objects.FieldKeyStatus].(string)
	sc := objects.GetGlobalStatusChecker()
	objKind, _ := cfg[objects.FieldKeyObjectKind].(string)
	if objKind == "" {
		objKind = kind
	}
	if skip, _ := cfg["skip_terminal"].(bool); skip {
		if sc.IsTerminal(objKind, status) || sc.IsArchive(objKind, status) {
			return nil
		}
	}
	// A preliminary status is the draft plane: the object exists to be written down before
	// its place in the hierarchy is known. Requiring the link here does not gate anything,
	// because a draft makes no shovel-ready claim — it only makes the draft unwritable, since
	// this fires on every update and not just on a transition. The gate that matters still
	// holds: planned and later are neither preliminary nor terminal, so promoting an
	// unlinked item is still refused.
	if skip, _ := cfg["skip_preliminary"].(bool); skip {
		if sc.IsPreliminary(objKind, status) {
			return nil
		}
	}
	if skip, _ := cfg["skip_heal_glass"].(bool); skip {
		if pkgctx.IsLifecycleBreakGlass(ctx) && strings.Contains(pkgctx.GetLifecycleBreakGlassReason(ctx), "heal-dangling") {
			return nil
		}
	}
	okAny := false
	for _, f := range fields {
		fname, _ := f.(string)
		if hasNonEmpty(obj, fname) {
			okAny = true
			break
		}
	}
	if okAny {
		return nil
	}
	msg, _ := cfg["message"].(string)
	label, _ := cfg["field_label"].(string)
	if label == "" {
		label = "refs"
	}
	return []ValidationError{{Field: label, Message: msg, Rule: "composed_integrity"}}
}

func statusMatchesConfig(obj map[string]any, cfg map[string]any) bool {
	status, _ := obj[objects.FieldKeyStatus].(string)
	statuses, _ := cfg[objects.FieldKeyStatuses].([]any)
	for _, s := range statuses {
		if ss, ok := s.(string); ok && ss == status {
			return true
		}
	}
	return false
}

func evalRequireField(ctx context.Context, kind string, obj map[string]any, cfg map[string]any) []ValidationError {
	field, _ := cfg[objects.FieldKeyField].(string)
	if field == "" || hasNonEmpty(obj, field) {
		return nil
	}
	if skip, _ := cfg["skip_preliminary"].(bool); skip {
		status, _ := obj[objects.FieldKeyStatus].(string)
		sc := objects.GetGlobalStatusChecker()
		objKind, _ := cfg[objects.FieldKeyObjectKind].(string)
		if objKind == "" {
			objKind = kind
		}
		if sc.IsPreliminary(objKind, status) {
			return nil
		}
	}
	if skip, _ := cfg["skip_terminal"].(bool); skip {
		status, _ := obj[objects.FieldKeyStatus].(string)
		sc := objects.GetGlobalStatusChecker()
		objKind, _ := cfg[objects.FieldKeyObjectKind].(string)
		if objKind == "" {
			objKind = kind
		}
		if sc.IsTerminal(objKind, status) || sc.IsArchive(objKind, status) {
			return nil
		}
	}
	msg, _ := cfg["message"].(string)
	if msg == "" {
		msg = fmt.Sprintf("%s must be set", field)
	}
	return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
}

func evalRequireFieldWhenStatus(obj map[string]any, cfg map[string]any) []ValidationError {
	if !statusMatchesConfig(obj, cfg) {
		return nil
	}
	if fieldsRaw, ok := cfg["fields"]; ok {
		for _, f := range stringRefs(fieldsRaw) {
			if hasNonEmpty(obj, f) {
				return nil
			}
		}
		msg, _ := cfg["message"].(string)
		field, _ := cfg[objects.FieldKeyField].(string)
		if field == "" && len(stringRefs(fieldsRaw)) > 0 {
			field = stringRefs(fieldsRaw)[0]
		}
		return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
	}
	field, _ := cfg[objects.FieldKeyField].(string)
	if hasNonEmpty(obj, field) {
		return nil
	}
	msg, _ := cfg["message"].(string)
	return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
}

func evalShovelReadyWhenStatus(obj map[string]any, cfg map[string]any) []ValidationError {
	if !statusMatchesConfig(obj, cfg) {
		return nil
	}
	res := shovelready.Evaluate(obj)
	if res.Ready {
		return nil
	}
	msg, _ := cfg["message"].(string)
	if msg == "" {
		msg = shovelready.Precondition + " fields are missing"
	}
	if len(res.Missing) > 0 {
		msg = msg + " (missing: " + strings.Join(res.Missing, ",") + ")"
	}
	return []ValidationError{{
		Field:   FieldCRIShovelReady,
		Message: msg,
		Rule:    "composed_integrity",
	}}
}

func evalRefuseFieldWhenStatus(obj map[string]any, cfg map[string]any) []ValidationError {
	field, _ := cfg[objects.FieldKeyField].(string)
	if field == "" || !statusMatchesConfig(obj, cfg) {
		return nil
	}
	if !hasNonEmpty(obj, field) {
		return nil
	}
	msg, _ := cfg["message"].(string)
	if msg == "" {
		msg = fmt.Sprintf("field %s must not be set when status is %v", field, obj[objects.FieldKeyStatus])
	}
	return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
}

func evalRequireFieldWhenActive(kind string, obj map[string]any, cfg map[string]any) []ValidationError {
	field, _ := cfg[objects.FieldKeyField].(string)
	status, _ := obj[objects.FieldKeyStatus].(string)
	objKind, _ := cfg[objects.FieldKeyKind].(string)
	if objKind == "" {
		objKind = kind
	}
	sc := objects.GetGlobalStatusChecker()
	includeTerminal, _ := cfg["include_terminal"].(bool)
	active := sc.IsActive(objKind, status) || status == objects.ObjectStatusActive || status == objects.ObjectStatusInProgress
	if includeTerminal {
		active = active || sc.IsTerminal(objKind, status)
	}
	if !active {
		return nil
	}
	if hasNonEmpty(obj, field) {
		return nil
	}
	msg, _ := cfg["message"].(string)
	if strings.Contains(msg, "%s") {
		msg = fmt.Sprintf(msg, status)
	}
	return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
}

func evalMinStringLen(obj map[string]any, cfg map[string]any) []ValidationError {
	field, _ := cfg[objects.FieldKeyField].(string)
	minLen := 0
	switch v := cfg["min_len"].(type) {
	case int:
		minLen = v
	case float64:
		minLen = int(v)
	}
	raw, _ := obj[field].(string)
	if len(raw) >= minLen {
		return nil
	}
	msg, _ := cfg["message"].(string)
	return []ValidationError{{Field: field, Message: msg, Rule: "composed_integrity"}}
}

func evalRefusePlanStatus(ctx context.Context, obj map[string]any, lookup ObjectLookup, cfg map[string]any) []ValidationError {
	planField, _ := cfg["plan_field"].(string)
	planRef, _ := obj[planField].(string)
	if planRef == "" {
		return nil
	}
	if newOnly, _ := cfg["new_link_only"].(bool); newOnly {
		if pkgctx.IsLifecycleBreakGlass(ctx) {
			return nil
		}
		id, _ := obj[objects.FieldKeyID].(string)
		if id != "" && lookup != nil {
			if old, err := lookup(id); err == nil && old != nil {
				oldRef, _ := old[planField].(string)
				if oldRef == planRef {
					return nil
				}
			}
		}
	}
	// when_object_status gate
	if when, ok := cfg["when_object_status"].([]any); ok && len(when) > 0 {
		status, _ := obj[objects.FieldKeyStatus].(string)
		match := false
		for _, s := range when {
			if ss, _ := s.(string); ss == status {
				match = true
				break
			}
		}
		if !match {
			return nil
		}
	}
	planStatus := lookupPlanStatus(planRef, lookup)
	if planStatus == "" {
		return nil
	}
	if req, ok := cfg["require_plan_status"].([]any); ok && len(req) > 0 {
		okStatus := false
		for _, s := range req {
			if ss, _ := s.(string); strings.EqualFold(ss, planStatus) {
				okStatus = true
				break
			}
		}
		if okStatus {
			return nil
		}
		msgFmt, _ := cfg["message_fmt"].(string)
		return []ValidationError{{
			Field:   objects.FieldKeyStatus,
			Message: fmt.Sprintf(msgFmt, planRef, planStatus),
			Rule:    "composed_integrity",
		}}
	}
	refuseWhen, _ := cfg["refuse_when"].([]any)
	for _, s := range refuseWhen {
		if ss, _ := s.(string); strings.EqualFold(ss, planStatus) {
			msgFmt, _ := cfg["message_fmt"].(string)
			return []ValidationError{{
				Field:   planField,
				Message: fmt.Sprintf(msgFmt, planRef, planStatus),
				Rule:    "scope_creep_protection",
			}}
		}
	}
	return nil
}

func lookupPlanStatus(planRef string, lookup ObjectLookup) string {
	if lookup != nil {
		if obj, err := lookup(planRef); err == nil && obj != nil {
			st, _ := obj[objects.FieldKeyStatus].(string)
			return st
		}
	}
	// Disk fallback (same spirit as former getPriorityPlanStatus).
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	root, err := paths.ModuleRootFromPath(wd)
	if err != nil {
		root = wd
	}
	dir := filepath.Join(root, paths.ProcessDir, "priority_plans")
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := fileutil.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var m map[string]any
		if yaml.Unmarshal(b, &m) != nil {
			continue
		}
		if id, _ := m[objects.FieldKeyID].(string); id == planRef {
			st, _ := m[objects.FieldKeyStatus].(string)
			return st
		}
	}
	return ""
}

func hasNonEmpty(obj map[string]any, field string) bool {
	v, ok := obj[field]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []any:
		return len(t) > 0
	case []string:
		return len(t) > 0
	default:
		return true
	}
}

func evalValidatePriorityValues(obj map[string]any, cfg map[string]any) []ValidationError {
	var errs []ValidationError
	if rawPri, ok := obj[objects.FieldKeyPriority]; ok && rawPri != nil {
		if s, ok := rawPri.(string); ok && strings.TrimSpace(s) != "" {
			if !objects.IsLegitimatePriority(s) {
				msg, _ := cfg["priority_message"].(string)
				if msg == "" {
					msg = fmt.Sprintf("invalid priority %q: must be one of critical, high, medium, low", s)
				}
				errs = append(errs, ValidationError{
					Field:   objects.FieldKeyPriority,
					Message: msg,
					Rule:    "composed_integrity",
				})
			}
		}
	}
	if rawTier, ok := obj[objects.FieldKeyPriorityTier]; ok && rawTier != nil {
		if s, ok := rawTier.(string); ok && strings.TrimSpace(s) != "" {
			if !objects.IsLegitimatePriorityTier(s) {
				msg, _ := cfg["tier_message"].(string)
				if msg == "" {
					msg = fmt.Sprintf("invalid priority_tier %q: must be one of P0, P1, P2, P3", s)
				}
				errs = append(errs, ValidationError{
					Field:   objects.FieldKeyPriorityTier,
					Message: msg,
					Rule:    "composed_integrity",
				})
			}
		}
	}
	return errs
}
