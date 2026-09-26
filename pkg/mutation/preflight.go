package mutation

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// SchemaViolation details a specific schema or constraint violation.
type SchemaViolation struct {
	FieldPath         string `json:"field_path"`
	FailingConstraint string `json:"failing_constraint"`
	Expected          string `json:"expected"`
	Actual            string `json:"actual"`
	Remediation       string `json:"remediation"`
}

// PreflightDiagnosticReceipt captures the preflight validation results for a mutation.
type PreflightDiagnosticReceipt struct {
	Valid       bool              `json:"valid"`
	Disposition string            `json:"disposition"` // "accepted", "rejected_failclosed", "quarantined"
	TargetKind  string            `json:"target_kind"`
	TargetID    string            `json:"target_id,omitempty"`
	Violations  []SchemaViolation `json:"violations"`
}

// PreflightBatchReceipt aggregates preflight diagnostics for a batch of mutations.
type PreflightBatchReceipt struct {
	Total        int                           `json:"total"`
	ValidCount   int                           `json:"valid_count"`
	InvalidCount int                           `json:"invalid_count"`
	AllValid     bool                          `json:"all_valid"`
	Receipts     []*PreflightDiagnosticReceipt `json:"receipts"`
}

// PreflightValidator defines the contract for in-memory pre-persistence schema and invariant validation.
type PreflightValidator interface {
	Validate(ctx context.Context, mut *Mutation) (*PreflightDiagnosticReceipt, error)
	ValidateBatch(ctx context.Context, muts []Mutation) (*PreflightBatchReceipt, error)
}

// DefaultPreflightValidator evaluates candidate mutations in-memory without initiating disk writes.
type DefaultPreflightValidator struct {
	validActions       map[Action]bool
	validSafetyClasses map[SafetyClass]bool
	validPriorityTiers map[string]bool
	validStatuses      map[string]bool
	idPattern          *regexp.Regexp
}

// NewDefaultPreflightValidator creates a configured preflight validator with kernel schema invariants.
func NewDefaultPreflightValidator() *DefaultPreflightValidator {
	return &DefaultPreflightValidator{
		validActions: map[Action]bool{
			ActionCreateNode: true,
			ActionUpdateNode: true,
			ActionAddEdge:    true,
			ActionRemoveEdge: true,
		},
		validSafetyClasses: map[SafetyClass]bool{
			SafetyRead:        true,
			SafetyWrite:       true,
			SafetyDestructive: true,
			SafetyHilRequired: true,
		},
		validPriorityTiers: map[string]bool{
			"P0": true, "P1": true, "P2": true, "P3": true, "P4": true, "P5": true,
		},
		validStatuses: map[string]bool{
			"originated":  true,
			"draft":       true,
			"planned":     true,
			"testing":     true,
			"in_progress": true,
			"validated":   true,
			"complete":    true,
			"active":      true,
			"archived":    true,
		},
		idPattern: regexp.MustCompile(`^[a-zA-Z0-9]+(-[a-zA-Z0-9_-]+)+$`),
	}
}

// Validate evaluates an individual mutation entirely in-memory.
func (v *DefaultPreflightValidator) Validate(ctx context.Context, mut *Mutation) (*PreflightDiagnosticReceipt, error) {
	if mut == nil {
		return &PreflightDiagnosticReceipt{
			Valid:       false,
			Disposition: "rejected_failclosed",
			Violations: []SchemaViolation{
				{
					FieldPath:         "mutation",
					FailingConstraint: "non_null",
					Expected:          "non-nil mutation object",
					Actual:            "nil",
					Remediation:       "Provide a populated Mutation object",
				},
			},
		}, nil
	}

	receipt := &PreflightDiagnosticReceipt{
		Valid:       true,
		Disposition: "accepted",
		TargetKind:  mut.TargetKind,
		TargetID:    mut.TargetID,
		Violations:  make([]SchemaViolation, 0),
	}

	// 1. Validate Action
	if mut.Action == "" {
		receipt.Violations = append(receipt.Violations, SchemaViolation{
			FieldPath:         "action",
			FailingConstraint: "mandatory_field",
			Expected:          "one of: [create_node, update_node, add_edge, remove_edge]",
			Actual:            "empty",
			Remediation:       "Specify a valid mutation action (create_node, update_node, etc.)",
		})
	} else if !v.validActions[mut.Action] {
		receipt.Violations = append(receipt.Violations, SchemaViolation{
			FieldPath:         "action",
			FailingConstraint: "enum_membership",
			Expected:          "one of: [create_node, update_node, add_edge, remove_edge]",
			Actual:            string(mut.Action),
			Remediation:       "Use a standard kernel action constant",
		})
	}

	// 2. Validate TargetKind
	if strings.TrimSpace(mut.TargetKind) == "" {
		receipt.Violations = append(receipt.Violations, SchemaViolation{
			FieldPath:         "target_kind",
			FailingConstraint: "mandatory_field",
			Expected:          "non-empty kernel kind identifier",
			Actual:            "empty",
			Remediation:       "Specify target_kind (e.g. backlog_item, criteria, test_case, goal)",
		})
	}

	// 3. Validate TargetID format
	if mut.TargetID != "" && !v.idPattern.MatchString(mut.TargetID) {
		receipt.Violations = append(receipt.Violations, SchemaViolation{
			FieldPath:         "target_id",
			FailingConstraint: "id_pattern_mismatch",
			Expected:          "formatted identifier conforming to KIND-XXXX or PREFIX-XXXX pattern",
			Actual:            mut.TargetID,
			Remediation:       "Ensure target_id follows standard hyphenated prefix format (e.g. BLI-1001)",
		})
	}

	// 4. Validate SafetyClass if present
	if mut.SafetyClass != "" && !v.validSafetyClasses[mut.SafetyClass] {
		receipt.Violations = append(receipt.Violations, SchemaViolation{
			FieldPath:         "safety_class",
			FailingConstraint: "enum_membership",
			Expected:          "one of: [read, write, destructive, hil_required]",
			Actual:            string(mut.SafetyClass),
			Remediation:       "Use a valid safety class constant or leave unset for default",
		})
	}

	// 5. Validate Fields Map for Type Invariants and Constraints
	if mut.Fields != nil {
		// Priority Tier Enum Check
		if ptVal, hasPT := mut.Fields["priority_tier"]; hasPT {
			if ptStr, ok := ptVal.(string); ok {
				if !v.validPriorityTiers[ptStr] {
					receipt.Violations = append(receipt.Violations, SchemaViolation{
						FieldPath:         "fields.priority_tier",
						FailingConstraint: "enum_membership",
						Expected:          "one of: [P0, P1, P2, P3, P4, P5]",
						Actual:            ptStr,
						Remediation:       "Set priority_tier to a valid canonical tier (P0 through P5)",
					})
				}
			} else {
				receipt.Violations = append(receipt.Violations, SchemaViolation{
					FieldPath:         "fields.priority_tier",
					FailingConstraint: "type_mismatch",
					Expected:          "string",
					Actual:            fmt.Sprintf("%T", ptVal),
					Remediation:       "Wrap priority_tier in quotes as a string",
				})
			}
		}

		// Status Enum Check
		if statusVal, hasStatus := mut.Fields["status"]; hasStatus {
			if statusStr, ok := statusVal.(string); ok {
				if !v.validStatuses[statusStr] {
					receipt.Violations = append(receipt.Violations, SchemaViolation{
						FieldPath:         "fields.status",
						FailingConstraint: "enum_membership",
						Expected:          "valid kernel status constant (e.g. planned, testing, active, complete)",
						Actual:            statusStr,
						Remediation:       "Use an authorized lifecycle status string",
					})
				}
			} else {
				receipt.Violations = append(receipt.Violations, SchemaViolation{
					FieldPath:         "fields.status",
					FailingConstraint: "type_mismatch",
					Expected:          "string",
					Actual:            fmt.Sprintf("%T", statusVal),
					Remediation:       "Status must be a string",
				})
			}
		}

		// Title Type and Mandatory Check for create_node
		if titleVal, hasTitle := mut.Fields["title"]; hasTitle {
			if _, ok := titleVal.(string); !ok {
				receipt.Violations = append(receipt.Violations, SchemaViolation{
					FieldPath:         "fields.title",
					FailingConstraint: "type_mismatch",
					Expected:          "string",
					Actual:            fmt.Sprintf("%T", titleVal),
					Remediation:       "Ensure title is a string type",
				})
			}
		} else if mut.Action == ActionCreateNode && (mut.TargetKind == "backlog_item" || mut.TargetKind == "test_case" || mut.TargetKind == "goal") {
			receipt.Violations = append(receipt.Violations, SchemaViolation{
				FieldPath:         "fields.title",
				FailingConstraint: "mandatory_field",
				Expected:          "non-empty title string for node creation",
				Actual:            "missing",
				Remediation:       "Provide a descriptive title for the newly created entity",
			})
		}

		// Array Refs Check (e.g. criteria_refs, requirement_refs)
		for _, refField := range []string{"criteria_refs", "requirement_refs", "goal_refs", "milestone_refs"} {
			if refVal, hasRef := mut.Fields[refField]; hasRef {
				valKind := reflect.TypeOf(refVal).Kind()
				if valKind != reflect.Slice && valKind != reflect.Array {
					receipt.Violations = append(receipt.Violations, SchemaViolation{
						FieldPath:         "fields." + refField,
						FailingConstraint: "type_mismatch",
						Expected:          "array/slice of strings",
						Actual:            fmt.Sprintf("%T", refVal),
						Remediation:       "Format reference fields as an array of string identifiers",
					})
				}
			}
		}
	}

	// 6. Fail-closed disposition logic
	if len(receipt.Violations) > 0 {
		receipt.Valid = false
		receipt.Disposition = "rejected_failclosed"
	}

	return receipt, nil
}

// ValidateBatch evaluates a batch of mutations in-memory, aggregating results.
func (v *DefaultPreflightValidator) ValidateBatch(ctx context.Context, muts []Mutation) (*PreflightBatchReceipt, error) {
	batch := &PreflightBatchReceipt{
		Total:    len(muts),
		Receipts: make([]*PreflightDiagnosticReceipt, len(muts)),
		AllValid: true,
	}

	for i := range muts {
		r, err := v.Validate(ctx, &muts[i])
		if err != nil {
			return nil, err
		}
		batch.Receipts[i] = r
		if r.Valid {
			batch.ValidCount++
		} else {
			batch.InvalidCount++
			batch.AllValid = false
		}
	}

	return batch, nil
}
