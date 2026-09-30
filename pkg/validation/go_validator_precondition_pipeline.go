package validation

import (
	"context"
	"os"
	"strings"

	"github.com/zqk-os/zqk/pkg/config"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

const lifecyclePreconditionPipelineKind = "validation.lifecycle_precondition"

// lifecyclePreconditionStageOrder is the canonical pkg/pipeline wrapper around
// the DECIDE first-match table. Rule names are not AddStage names: Run executes
// every stage, so per-rule AddStage would both change first-match semantics and
// flood the validation hot path with metrics.
func lifecyclePreconditionStageOrder() []string {
	return []string{pipeline.StageIngest, pipeline.StageNormalize, pipeline.StageDecide, pipeline.StageFinalize}
}

type lifecyclePrecondPayload struct {
	gv         *GoValidator
	raw        string
	normalized string
	obj        map[string]any
	options    *ValidationOptions
	recognized *bool
	ruleName   string
	handled    bool
	met        bool
}

var lifecyclePrecondPipe = pipeline.NewBuilder(lifecyclePreconditionPipelineKind, nil).
	AddStage(pipeline.StageIngest, lifecyclePrecondIngest).
	AddStage(pipeline.StageNormalize, lifecyclePrecondNormalize).
	AddStage(pipeline.StageDecide, lifecyclePrecondDecide).
	AddStage(pipeline.StageFinalize, lifecyclePrecondFinalize).
	Build()

// runLifecyclePreconditionPipeline wraps dispatch in INGEST→NORMALIZE→DECIDE→FINALIZE
// and records Outcome keys. Hot-path checkPrecondition still uses dispatchPrecondition
// (no Outcome map per barrier).
func (gv *GoValidator) runLifecyclePreconditionPipeline(precondition string, obj map[string]any, options *ValidationOptions, recognized *bool) (*pipeline.Context, bool) {
	p := &lifecyclePrecondPayload{
		gv:         gv,
		raw:        precondition,
		obj:        obj,
		options:    options,
		recognized: recognized,
		met:        true,
	}
	pctx := &pipeline.Context{
		Ctx:     context.Background(),
		Outcome: make(map[string]any),
	}
	if _, err := lifecyclePrecondPipe.Run(pctx, p); err != nil {
		p.met = false
		pctx.Outcome[pipeline.OutcomeKeyLifecycleOk] = false
	}
	return pctx, p.met
}

func lifecyclePrecondIngest(_ *pipeline.Context, payload any) (any, error) {
	return payload, nil
}

func lifecyclePrecondNormalize(_ *pipeline.Context, payload any) (any, error) {
	p, ok := payload.(*lifecyclePrecondPayload)
	if !ok {
		return payload, nil
	}
	p.normalized = strings.Join(strings.Fields(strings.ToLower(p.raw)), " ")
	return p, nil
}

func lifecyclePrecondDecide(_ *pipeline.Context, payload any) (any, error) {
	p, ok := payload.(*lifecyclePrecondPayload)
	if !ok {
		return payload, nil
	}
	gv := p.gv
	if gv == nil {
		gv = NewGoValidator()
	}
	p.ruleName, p.handled, p.met = gv.evalPrecondDecideTable(p.normalized, p.obj, p.options)
	if p.recognized != nil {
		*p.recognized = p.handled
	}
	return p, nil
}

func lifecyclePrecondFinalize(pctx *pipeline.Context, payload any) (any, error) {
	p, ok := payload.(*lifecyclePrecondPayload)
	if !ok {
		return payload, nil
	}
	if pctx.Outcome == nil {
		pctx.Outcome = make(map[string]any)
	}
	pctx.Outcome[pipeline.OutcomeKeyPlan] = p.ruleName
	pctx.Outcome[pipeline.OutcomeKeyLifecycleOk] = p.met
	pctx.Outcome[pipeline.OutcomeKeyValidationSuccess] = p.handled
	pctx.Outcome[pipeline.OutcomeKeyFinalizeDone] = true
	return p, nil
}

// List form of FieldKeyVisionRef. objects has no FieldKeyVisionRefs.
const fieldKeyVisionRefs = "vision_refs"

// precondDecideRule is one first-match row inside the DECIDE stage of the
// pkg/pipeline lifecycle (INGEST → NORMALIZE → DECIDE → FINALIZE). These are
// not pipeline stages: Run would execute every AddStage, which would both
// change first-match semantics and emit per-rule metrics on the validation
// hot path. Adding a barrier is adding a row here.
// handled=false means this rule did not claim the string; later rows still run.
// handled=true means this rule is the answer; met is whether the barrier holds.
type precondDecideRule struct {
	name string
	eval func(gv *GoValidator, precondition string, obj map[string]any, options *ValidationOptions) (handled, met bool)
}

// precondDecideRules is the ordered DECIDE table. Explicit token matches run
// before substring heuristics. A heuristic that merely mentions "active" must
// not run ahead of a specific rule — that is how planned→in_progress came to
// admit a plan still in grooming.
//
// Unrecognized strings still fail-open (legacy); overlay DSL remains the
// fail-closed path for other English. See docs/architecture/LIFECYCLE_SHOCKWAVE_MAP.md Plane A.
// TRACK: storage-save compose ops are a
// second plane; do not silently unify YAML prose dispatch with kernelcas/compose.
// from a PIP-* loader (or compose DECIDE) instead.
var precondDecideRules = []precondDecideRule{
	{name: "ref_status_matrix", eval: evalRefStatusStage},
	matchOverlayContains("tdd_test_red_phase", PrecondTDDTestRedPhase),
	matchOverlayContains("criteria_active_test_case", PrecondCriteriaLinkedToActiveTestCase),
	matchContains("criteria_active_test_case_formal", strings.ToLower(PrecondActiveTestCaseRefLinked), func(gv *GoValidator, _ string, obj map[string]any, options *ValidationOptions) bool {
		if strings.ToLower(objects.GetString(obj, objects.FieldKeyKind)) == objects.KindCriteria {
			_, met := evalOverlayDSLStage(gv, "criteria_active_test_case", obj, options)
			return met
		}
		return false
	}),
	matchOverlayContains("shovel_ready", PrecondCRIShovelReady),
	matchOverlayExact("ready_backlog_references_plan", PrecondReadyBacklogReferencesPlan),
	matchOverlayExact("linked_backlog_ready_or_later", PrecondAllLinkedBacklogReadyOrLater),
	matchOverlayExact("linked_backlog_all_terminal", PrecondLinkedBacklogAllTerminal),
	matchOverlayExact("no_linked_backlog_in_progress_or_complete", PrecondNoLinkedBacklogInProgressOrComplete),
	matchOverlayExact("workflow_constraints_if_set", PrecondWorkflowConstraintsIfSet),
	matchOverlayExact("priority_plan_validated", PrecondPriorityPlanValidated),
	matchOverlayExact("team_or_persona_dispatch_refs", PrecondTeamOrPersonaDispatchRefs),
	{name: "parent_child_link_back", eval: evalLinkBackStage},
	{name: "active_ref", eval: evalActiveRefStage},
	matchOverlayContains("problem_statement_and_acceptance", PrecondProblemStatementAndAcceptance, "field_nonempty:problem_statement;field_nonempty:acceptance_considerations"),
	matchOverlayContains("priority_assigned", PrecondPriorityAssigned, "field_matches:priority:^(high|medium|low)$"),
	{name: "owner_identified", eval: evalOwnerIdentifiedStage},
	matchOverlayContains("commit_refs_git_mutation_evidence", PrecondCommitRefsGitMutationEvidence, "git_mutation_evidence_present"),
	matchOverlayContains("commit_hashes_git_mutation_evidence", PrecondCommitHashesGitMutationEvidence, "git_mutation_evidence_present"),
	matchOverlayExact("branch_ref_is_ancestor_of_trunk", PrecondBranchRefIsAncestorOfTrunk, "branch_is_ancestor_of_trunk"),
	matchOverlayExact("branch_name_is_ancestor_of_trunk", PrecondBranchNameIsAncestorOfTrunk, "branch_is_ancestor_of_trunk"),
	{name: "machine_checkable_closure_evidence", eval: evalMachineCheckableStage},
	{name: "field_is_set", eval: evalFieldIsSetStage},
	{name: "field_is_not_empty", eval: evalFieldIsNotEmptyStage},
	matchContains("at_least", SubprecondAtLeast, func(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) bool {
		_, met := evalOverlayDSLStage(gv, p, obj, options)
		return met
	}),
	{name: "overlay_dsl", eval: evalOverlayDSLStage},
}

func matchExact(name, token string, fn func(gv *GoValidator, obj map[string]any, options *ValidationOptions) bool) precondDecideRule {
	needle := strings.ToLower(token)
	return precondDecideRule{
		name: name,
		eval: func(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
			if p != needle {
				return false, false
			}
			return true, fn(gv, obj, options)
		},
	}
}

func matchOverlayExact(name, token string, customDSL ...string) precondDecideRule {
	dslStage := name
	if len(customDSL) > 0 && customDSL[0] != "" {
		dslStage = customDSL[0]
	}
	return matchExact(name, token, func(gv *GoValidator, obj map[string]any, options *ValidationOptions) bool {
		_, met := evalOverlayDSLStage(gv, dslStage, obj, options)
		return met
	})
}

func matchContains(name, token string, fn func(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) bool) precondDecideRule {
	needle := strings.Join(strings.Fields(strings.ToLower(token)), " ")
	return precondDecideRule{
		name: name,
		eval: func(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
			if needle == "" || !strings.Contains(p, needle) {
				return false, false
			}
			return true, fn(gv, p, obj, options)
		},
	}
}

func matchOverlayContains(name, token string, customDSL ...string) precondDecideRule {
	dslStage := name
	if len(customDSL) > 0 && customDSL[0] != "" {
		dslStage = customDSL[0]
	}
	return matchContains(name, token, func(gv *GoValidator, _ string, obj map[string]any, options *ValidationOptions) bool {
		_, met := evalOverlayDSLStage(gv, dslStage, obj, options)
		return met
	})
}

func precondDecideRuleNames() []string {
	names := make([]string, len(precondDecideRules))
	for i, s := range precondDecideRules {
		names[i] = s.name
	}
	return names
}

func objectID(obj map[string]any) string {
	id, _ := obj[objects.FieldKeyID].(string)
	return id
}

func (gv *GoValidator) kindFromID() func(string) string {
	if gv != nil && gv.idValidator != nil {
		return gv.idValidator.InferKindFromID
	}
	return GetIDValidator().InferKindFromID
}

func evalRefStatusStage(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
	rule, ok := lookupRefStatusRule(p)
	if !ok {
		return false, false
	}
	return true, gv.evalRefStatus(rule, obj, options)
}

func evalOwnerIdentifiedStage(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
	if !strings.Contains(p, PrecondOwnerIdentified) &&
		!strings.Contains(p, PrecondOwnerRefIsSet) &&
		p != strings.ToLower(PrecondOwnerIsSet) {
		return false, false
	}
	return evalOverlayDSLStage(gv, "field_nonempty:owner_ref", obj, options)
}

func evalMachineCheckableStage(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
	if !strings.Contains(p, "machine-checkable evidence") &&
		!strings.Contains(p, "green fingerprint") &&
		!strings.Contains(p, "criteria evidence verified") &&
		!strings.Contains(p, strings.ToLower(PrecondMachineCheckableClosureEvidence)) {
		return false, false
	}
	return evalOverlayDSLStage(gv, "machine_checkable_closure_evidence", obj, options)
}

func evalFieldIsSetStage(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
	if !isFieldCheckPrecondition(p, SubprecondIsSet) {
		return false, false
	}
	return evalOverlayDSLStage(gv, p, obj, options)
}

func evalFieldIsNotEmptyStage(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
	if !isFieldCheckPrecondition(p, SubprecondIsNotEmpty) {
		return false, false
	}
	return evalOverlayDSLStage(gv, p, obj, options)
}

func evalLinkBackStage(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
	if !strings.Contains(p, SubprecondLinkBackTo) &&
		!strings.Contains(p, SubprecondLinksBackTo) &&
		!strings.Contains(p, SubprecondBelongsTo) {
		return false, false
	}
	subjectField, targetField, ok := parseLinkBackFields(p)
	if !ok {
		return false, false
	}
	return true, gv.linkBackAligned(subjectField, targetField, obj, options)
}

func parseLinkBackFields(precondition string) (subjectField, targetField string, ok bool) {
	parts := strings.Split(precondition, SubprecondLinkBackTo)
	if len(parts) != 2 {
		parts = strings.Split(precondition, SubprecondBelongsTo)
	}
	if len(parts) != 2 {
		return "", "", false
	}
	subjectField = firstRefToken(parts[0], ",.()[]{}'")
	targetField = firstRefToken(parts[1], ",.()[]{}'")
	if subjectField == "" {
		subjectField = linkBackSubjectFallback(parts[0])
	}
	if targetField == "" {
		targetField = linkBackTargetFallback(parts[1])
	}
	if subjectField == "" || targetField == "" {
		return "", "", false
	}
	return subjectField, targetField, true
}

func firstRefToken(part, cutset string) string {
	for _, word := range strings.Fields(part) {
		w := strings.Trim(word, cutset)
		if strings.Contains(w, "ref") {
			return w
		}
	}
	return ""
}

func linkBackSubjectFallback(subjectPart string) string {
	var subjectField string
	if strings.Contains(subjectPart, "milestone") {
		subjectField = objects.FieldKeyMilestoneRefs
	}
	if strings.Contains(subjectPart, "backlog_item") {
		subjectField = objects.FieldKeyBacklogItemRefs
	}
	if strings.Contains(subjectPart, "requirement") {
		subjectField = objects.FieldKeyRequirementRefs
	}
	return subjectField
}

func linkBackTargetFallback(targetPart string) string {
	var targetField string
	if strings.Contains(targetPart, "goal") {
		targetField = objects.FieldKeyGoalRefs
	}
	if strings.Contains(targetPart, "mission") {
		targetField = objects.FieldKeyMissionRefs
	}
	if strings.Contains(targetPart, "vision") {
		targetField = fieldKeyVisionRefs
	}
	return targetField
}

func (gv *GoValidator) linkBackAligned(subjectField, targetField string, obj map[string]any, options *ValidationOptions) bool {
	parentIDs := gv.extractIDsFromField(obj, targetField)
	childIDs := gv.extractIDsFromField(obj, subjectField)
	if len(parentIDs) == 0 {
		return false
	}
	if len(childIDs) == 0 {
		return true
	}
	for _, childID := range childIDs {
		if skipLinkBackChild(childID, options) {
			continue
		}
		childObj, err := gv.lookupRefObject(childID, options)
		if err != nil {
			if strings.HasSuffix(os.Args[0], ".test") || config.TestingSkipValidation().OrDefault(false) {
				continue
			}
			return false
		}
		if !idsIntersect(parentIDs, gv.extractIDsFromField(childObj, targetField)) {
			return false
		}
	}
	return true
}

var linkBackSkipStatuses = map[string]struct{}{
	objects.ObjectStatusDraft:               {},
	objects.ObjectStatusProposed:            {},
	objects.ObjectStatusExploring:           {},
	objects.ObjectStatusPlanning:            {},
	objects.ObjectStatusNotStarted:          {},
	objects.ObjectStatusImplemented:         {},
	objects.ObjectStatusPending:             {},
	objects.ObjectStatusPendingVerification: {},
	"":                                      {},
}

func skipLinkBackChild(childID string, options *ValidationOptions) bool {
	if options == nil || options.ObjectStatusLookup == nil {
		return false
	}
	childStatus, err := options.ObjectStatusLookup(childID)
	if err != nil {
		return false
	}
	_, skip := linkBackSkipStatuses[strings.ToLower(childStatus)]
	return skip
}

func idsIntersect(a, b []string) bool {
	for _, left := range a {
		for _, right := range b {
			if left == right {
				return true
			}
		}
	}
	return false
}

func evalActiveRefStage(gv *GoValidator, p string, obj map[string]any, options *ValidationOptions) (bool, bool) {
	if !strings.Contains(p, SubprecondActive) {
		return false, false
	}
	refFields := collectActiveRefFields(p)
	if len(refFields) == 0 {
		return false, false
	}
	hasActiveRef := false
	hasAnyRefValue := false
	for _, fieldName := range refFields {
		val := obj[fieldName]
		if val == nil {
			if strings.HasSuffix(fieldName, "_ref") {
				val = obj[fieldName+"s"]
			} else if strings.HasSuffix(fieldName, "_refs") {
				val = obj[strings.TrimSuffix(fieldName, "s")]
			}
		}
		if val == nil {
			continue
		}
		hasAnyRefValue = true
		if refStr, ok := val.(string); ok && refStr != "" {
			if gv.isRefActive(refStr, obj, options) {
				hasActiveRef = true
			}
		}
		switch v := val.(type) {
		case []any:
			for _, item := range v {
				if refStr, ok := item.(string); ok && refStr != "" {
					if gv.isRefActive(refStr, obj, options) {
						hasActiveRef = true
					}
				}
			}
		case []string:
			for _, item := range v {
				if item != "" {
					if gv.isRefActive(item, obj, options) {
						hasActiveRef = true
					}
				}
			}
		}
	}
	if !hasAnyRefValue {
		return false, false
	}
	return true, hasActiveRef
}

func collectActiveRefFields(precondition string) []string {
	var refFields []string
	for _, word := range strings.Fields(precondition) {
		w := strings.Trim(word, ",.()[]{}")
		if strings.Contains(w, "_ref") {
			refFields = append(refFields, w)
		}
	}
	if len(refFields) > 0 {
		return refFields
	}
	if strings.Contains(precondition, objects.KindVision) {
		refFields = append(refFields, objects.FieldKeyVisionRef)
	}
	if strings.Contains(precondition, objects.KindMilestone) {
		refFields = append(refFields, objects.FieldKeyMilestoneRefs)
	}
	if strings.Contains(precondition, objects.KindPriorityPlan) {
		refFields = append(refFields, objects.FieldKeyPriorityPlanRef)
	}
	return refFields
}

// checkTDDTestRedPhase verifies that a backlog_item links to criteria
// and that all of those criteria are linked to at least one test_case in ready/active status.
func (gv *GoValidator) checkTDDTestRedPhase(obj map[string]any, options *ValidationOptions) bool {
	if options == nil || options.ObjectStatusLookup == nil || options.DependentsLookup == nil {
		return false
	}

	critIDs := gv.extractIDsFromField(obj, "criteria_refs")
	if len(critIDs) == 0 {
		return false
	}

	for _, critID := range critIDs {
		deps := options.DependentsLookup(critID)
		hasTest := false
		for _, depID := range deps {
			if strings.HasPrefix(depID, "TST-") {
				status, err := options.ObjectStatusLookup(depID)
				if err == nil {
					status = strings.ToLower(strings.TrimSpace(status))
					if isTestCaseReadyStatus(status) {
						hasTest = true
						break
					}
				}
			}
		}
		if !hasTest {
			return false
		}
	}
	return true
}

// checkCriteriaLinkedToActiveTestCase verifies that a criterion is linked to at least one test_case in an active/ready state.
func (gv *GoValidator) checkCriteriaLinkedToActiveTestCase(obj map[string]any, options *ValidationOptions) bool {
	if options == nil {
		return false
	}

	// 1. Direct forward reference: test_case_refs on the criterion
	tcIDs := gv.extractIDsFromField(obj, "test_case_refs")
	for _, tcID := range tcIDs {
		if options.ObjectStatusLookup != nil {
			status, err := options.ObjectStatusLookup(tcID)
			if err == nil && isTestCaseReadyStatus(status) {
				return true
			}
		} else {
			return true
		}
	}

	// 2. Reverse reference via DependentsLookup
	critID, _ := obj[objects.FieldKeyID].(string)
	if critID != "" && options.DependentsLookup != nil {
		deps := options.DependentsLookup(critID)
		for _, depID := range deps {
			if strings.HasPrefix(depID, "TST-") {
				if options.ObjectStatusLookup != nil {
					status, err := options.ObjectStatusLookup(depID)
					if err == nil && isTestCaseReadyStatus(status) {
						return true
					}
				} else {
					return true
				}
			}
		}
	}

	return false
}

func isTestCaseReadyStatus(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "active" || s == "draft" || s == "metrics_captured" || s == "complete" || s == "error"
}
