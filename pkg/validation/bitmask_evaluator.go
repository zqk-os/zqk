package validation

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// BitmaskEvaluator is a helper utility for fast sequential bitmask validations
type BitmaskEvaluator struct {
	validator *GoValidator
}

// NewBitmaskEvaluator creates a new instance of the bitmask evaluator
func NewBitmaskEvaluator(v *GoValidator) *BitmaskEvaluator {
	return &BitmaskEvaluator{validator: v}
}

// ClassifyPrecondition dynamically categorizes a precondition into its sequence level
func ClassifyPrecondition(precondition string) int {
	precondition = strings.ToLower(strings.TrimSpace(precondition))
	if strings.Contains(precondition, SubprecondLinkBackTo) ||
		strings.Contains(precondition, SubprecondLinksBackTo) ||
		strings.Contains(precondition, SubprecondBelongsTo) {
		return 3
	}
	if strings.Contains(precondition, SubprecondActive) {
		return 2
	}
	return 1
}

// ValidatePreconditionsSequential evaluates preconditions in order of their sequence tiers, short-circuiting on failure
func (b *BitmaskEvaluator) ValidatePreconditionsSequential(preconditions []string, obj map[string]any, options *ValidationOptions, statusMsgTemplate string, status string) []ValidationError {
	var errors []ValidationError

	// Classify preconditions dynamically into their sequence tiers
	var seq1 []string
	var seq2 []string
	var seq3 []string

	statusVal, _ := obj[objects.FieldKeyStatus].(string)
	statusVal = strings.ToLower(statusVal)
	isNonActive := statusVal == objects.ObjectStatusDraft || statusVal == objects.ObjectStatusProposed || statusVal == objects.ObjectStatusExploring || statusVal == objects.ObjectStatusDeferred || statusVal == objects.ObjectStatusPlanning || statusVal == objects.ObjectStatusNotStarted || statusVal == objects.ObjectStatusError || statusVal == objects.ObjectStatusCancelled || statusVal == objects.ObjectStatusImplemented || statusVal == objects.ObjectStatusPending || statusVal == objects.ObjectStatusPendingVerification || statusVal == ""

	for _, pc := range preconditions {
		switch ClassifyPrecondition(pc) {
		case 1:
			seq1 = append(seq1, pc)
		case 2:
			if !isNonActive {
				seq2 = append(seq2, pc)
			}
		case 3:
			if !isNonActive {
				seq3 = append(seq3, pc)
			}
		}
	}

	// --- Sequence 1: Local Completeness Attributes ---
	if len(seq1) > 0 {
		var currentSeq1 uint64
		for i, pc := range seq1 {
			if b.validator.checkPrecondition(pc, obj, options) {
				currentSeq1 |= (1 << i)
			}
		}
		requiredSeq1 := uint64((1 << len(seq1)) - 1)
		if currentSeq1 != requiredSeq1 {
			for i, pc := range seq1 {
				if (currentSeq1 & (1 << i)) == 0 {
					errors = append(errors, ValidationError{
						Field:   objects.FieldKeyStatus,
						Message: fmt.Sprintf(statusMsgTemplate, status, pc),
						Rule:    validationRuleLifecycle(),
					})
				}
			}
			return errors // Short-circuit: skip Seq2 and Seq3
		}
	}

	// --- Sequence 2: Deeper Dereferences / Status Checks ---
	if len(seq2) > 0 {
		var currentSeq2 uint64
		for i, pc := range seq2 {
			if b.validator.checkPrecondition(pc, obj, options) {
				currentSeq2 |= (1 << i)
			}
		}
		requiredSeq2 := uint64((1 << len(seq2)) - 1)
		if currentSeq2 != requiredSeq2 {
			for i, pc := range seq2 {
				if (currentSeq2 & (1 << i)) == 0 {
					errors = append(errors, ValidationError{
						Field:   objects.FieldKeyStatus,
						Message: fmt.Sprintf(statusMsgTemplate, status, pc),
						Rule:    validationRuleLifecycle(),
					})
				}
			}
			return errors // Short-circuit: skip Seq3
		}
	}

	// --- Sequence 3: Relational / Alignment checks ---
	if len(seq3) > 0 {
		var currentSeq3 uint64
		for i, pc := range seq3 {
			if b.validator.checkPrecondition(pc, obj, options) {
				currentSeq3 |= (1 << i)
			}
		}
		requiredSeq3 := uint64((1 << len(seq3)) - 1)
		if currentSeq3 != requiredSeq3 {
			for i, pc := range seq3 {
				if (currentSeq3 & (1 << i)) == 0 {
					errors = append(errors, ValidationError{
						Field:   objects.FieldKeyStatus,
						Message: fmt.Sprintf(statusMsgTemplate, status, pc),
						Rule:    validationRuleLifecycle(),
					})
				}
			}
			return errors
		}
	}

	return errors
}
