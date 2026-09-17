package validation

import (
	"slices"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// Pointer-transitive lifecycle constraints, expressed as data.
//
// Most lifecycle preconditions are one shape: while the subject holds some status, the object(s)
// named by a ref field must themselves hold a status drawn from an allowed set. "A backlog_item
// may only start work under a plan that is executing" and "a backlog_item may only complete when
// its criteria are validated" differ in the field name and the allowed set, nothing else.
//
// Writing each one as a bespoke Go function plus a prose string match made the count of
// special cases grow with the count of barriers, and made ordering inside the dispatch
// load-bearing: a generic heuristic branch that merely mentions "active" could shadow a
// specific rule and quietly widen it. These rows are matched on an explicit precondition token
// ahead of any heuristic, so adding a barrier is adding a row.
//
// The generalization stops at the ref-status shape on purpose. Shape-only constraints
// ("field X is set at status Y") already have declarative ops in pkg/kernelcas/compose
// (OpRequireFieldWhenStatus), and the refuse-form of this same constraint exists there as
// OpRefuseChildStatus. Unifying those two planes changes which path enforces what on the
// storage save path, so it needs a decision rather than a refactor.
// TRACK: BLI-REDACTED — remove this note when lifecycle preconditions are
// sourced from one plane; it carries the triage of the 56 barriers still authored as prose.
type refStatusRule struct {
	// Precondition is the lifecycle token this row answers. Matched as a substring so a
	// lifecycle may carry a trailing parenthetical without breaking the binding.
	Precondition string
	// Field is the ref or ref-list field on the subject.
	Field string
	// RefKind is the kind the referenced ids are, used for status classification.
	RefKind string
	// Require lists referenced statuses that satisfy the constraint.
	Require []string
	// Ignore lists referenced statuses that neither satisfy nor refuse. A rejected
	// acceptance criterion, for instance, is not evidence and is not an obstacle.
	Ignore []string
	// ArchivedLineageSatisfies allows a subject whose refs are *all* archived to pass.
	// Without it, the cheapest way to clear the barrier would be to delete the archived
	// history, which is the opposite of what the barrier is for.
	ArchivedLineageSatisfies bool
}

// refStatusRules is the matrix: one row per (precondition token → ref field, allowed statuses).
var refStatusRules = []refStatusRule{
	{
		Precondition: PrecondPriorityPlanRefExecutionFacing,
		Field:        objects.FieldKeyPriorityPlanRef,
		RefKind:      objects.KindPriorityPlan,
		Require:      []string{objects.ObjectStatusActive, objects.ObjectStatusInProgress},
	},
	{
		Precondition:             PrecondAllLinkedCriteriaValidatedOrComplete,
		Field:                    objects.FieldKeyCriteriaRefs,
		RefKind:                  objects.KindCriteria,
		Require:                  []string{objects.ObjectStatusValidated, objects.ObjectStatusComplete, objects.ObjectStatusCompleted},
		Ignore:                   []string{objects.ObjectStatusRejected},
		ArchivedLineageSatisfies: true,
	},
}

// lookupRefStatusRule returns the row bound to precondition, which must already be lowercased.
func lookupRefStatusRule(precondition string) (refStatusRule, bool) {
	for _, r := range refStatusRules {
		if strings.Contains(precondition, strings.ToLower(r.Precondition)) {
			return r, true
		}
	}
	return refStatusRule{}, false
}

// evalRefStatus reports whether the referenced objects satisfy rule.
//
// Fail-closed on every absence: no refs, no status lookup, or a ref that does not resolve. A
// barrier that passes when it cannot see the evidence is not a barrier, and an unresolvable ref
// is a ghost reference rather than a satisfied one.
func (gv *GoValidator) evalRefStatus(rule refStatusRule, obj map[string]any, options *ValidationOptions) bool {
	ids := gv.extractIDsFromField(obj, rule.Field)
	if len(ids) == 0 {
		return false
	}
	if options == nil || options.ObjectStatusLookup == nil {
		return false
	}
	statusChecker := objects.GetGlobalStatusChecker()
	satisfied, archived := 0, 0
	for _, id := range ids {
		status, err := options.ObjectStatusLookup(id)
		if err != nil {
			return false
		}
		status = strings.ToLower(strings.TrimSpace(status))
		if slices.Contains(rule.Ignore, status) {
			continue
		}
		if rule.ArchivedLineageSatisfies && isArchivedStatus(statusChecker, rule.RefKind, status) {
			archived++
			continue
		}
		if !slices.Contains(rule.Require, status) {
			return false
		}
		satisfied++
	}
	if rule.ArchivedLineageSatisfies {
		return satisfied > 0 || archived == len(ids)
	}
	return satisfied > 0
}

func isArchivedStatus(statusChecker objects.IStatusChecker, kind, status string) bool {
	if status == objects.ObjectStatusArchived {
		return true
	}
	return statusChecker != nil && statusChecker.IsArchive(kind, status)
}
