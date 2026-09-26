package mutation

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
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

// StatusResolverFunc resolves the current status of an object by its ID.
type StatusResolverFunc func(ctx context.Context, id string) (string, bool)

// DefaultPreflightValidator evaluates candidate mutations in-memory without initiating disk writes.
type DefaultPreflightValidator struct {
	validActions       map[Action]bool
	validSafetyClasses map[SafetyClass]bool
	validPriorityTiers map[string]bool
	validStatuses      map[string]bool
	systemFields       map[string]bool
	idPattern          *regexp.Regexp
	specLoader         *objects.SpecLoader
	lifecycleLoader    *objects.LifecycleLoader
	statusResolver     StatusResolverFunc
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
		systemFields: map[string]bool{
			"created_at":      true,
			"created_by":      true,
			"updated_at":      true,
			"updated_by":      true,
			"archived_at":     true,
			"archived_by":     true,
			"status_history":  true,
			"provenance":      true,
			"change_log":      true,
			"version":         true,
			"schema_version":  true,
			"schema_ref":      true,
			"namespace_id":    true,
			"version_context": true,
		},
		idPattern:       regexp.MustCompile(`^[a-zA-Z0-9]+(-[a-zA-Z0-9_-]+)+$`),
		specLoader:      objects.GetGlobalSpecLoader(),
		lifecycleLoader: objects.GetGlobalLifecycleLoader(),
	}
}

// WithStatusResolver sets a status resolver function for inspecting existing object state.
func (v *DefaultPreflightValidator) WithStatusResolver(fn StatusResolverFunc) *DefaultPreflightValidator {
	v.statusResolver = fn
	return v
}

// WithSpecLoader sets the spec loader used for schema constraint evaluation.
func (v *DefaultPreflightValidator) WithSpecLoader(sl *objects.SpecLoader) *DefaultPreflightValidator {
	v.specLoader = sl
	return v
}

// WithLifecycleLoader sets the lifecycle loader used for transition evaluation.
func (v *DefaultPreflightValidator) WithLifecycleLoader(ll *objects.LifecycleLoader) *DefaultPreflightValidator {
	v.lifecycleLoader = ll
	return v
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
		// 5a. System-Managed Fields Check (forbidden unless BreakGlass authorized)
		if !pkgctx.IsLifecycleBreakGlass(ctx) {
			for fName, fVal := range mut.Fields {
				if fName == "id" || fName == "kind" {
					continue
				}
				isSys := v.systemFields[fName]
				if !isSys {
					if reg := objects.GetGlobalSystemFieldsRegistry(); reg != nil {
						if isRegSys, err := reg.IsSystemGeneratedField(fName); err == nil && isRegSys {
							isSys = true
						}
					}
				}
				if isSys {
					receipt.Violations = append(receipt.Violations, SchemaViolation{
						FieldPath:         "fields." + fName,
						FailingConstraint: "system_managed_field",
						Expected:          "field value must be computed by system runtime; manual modification forbidden without break-glass",
						Actual:            fmt.Sprintf("%v", fVal),
						Remediation:       fmt.Sprintf("Remove system-managed field %q from mutation payload or use --break-glass with justification", fName),
					})
				}
			}
		}

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

		// Status Enum and Lifecycle Transition Check
		if statusVal, hasStatus := mut.Fields["status"]; hasStatus {
			if statusStr, ok := statusVal.(string); ok {
				var statusValid bool
				if v.lifecycleLoader != nil && mut.TargetKind != "" {
					if sv, err := v.lifecycleLoader.IsValidStatus(mut.TargetKind, statusStr); err == nil {
						statusValid = sv
					} else {
						statusValid = v.validStatuses[statusStr]
					}
				} else {
					statusValid = v.validStatuses[statusStr]
				}

				if !statusValid {
					receipt.Violations = append(receipt.Violations, SchemaViolation{
						FieldPath:         "fields.status",
						FailingConstraint: "enum_membership",
						Expected:          fmt.Sprintf("valid lifecycle status for %s", mut.TargetKind),
						Actual:            statusStr,
						Remediation:       fmt.Sprintf("Use an authorized lifecycle status string for %s", mut.TargetKind),
					})
				} else if !pkgctx.IsLifecycleBreakGlass(ctx) {
					// Validate status transitions on update
					if mut.Action == ActionUpdateNode && mut.TargetID != "" {
						var oldStatus string
						if v.statusResolver != nil {
							if s, found := v.statusResolver(ctx, mut.TargetID); found {
								oldStatus = s
							}
						}
						if oldStatus != "" && oldStatus != statusStr && v.lifecycleLoader != nil {
							validTrans, transErr := v.lifecycleLoader.IsValidTransition(mut.TargetKind, oldStatus, statusStr)
							if transErr != nil || !validTrans {
								receipt.Violations = append(receipt.Violations, SchemaViolation{
									FieldPath:         "fields.status",
									FailingConstraint: "invalid_lifecycle_transition",
									Expected:          fmt.Sprintf("valid lifecycle transition from %q", oldStatus),
									Actual:            statusStr,
									Remediation:       fmt.Sprintf("Status cannot transition directly from %q to %q for %s without break-glass authorization", oldStatus, statusStr, mut.TargetKind),
								})
							}
						}
					} else if mut.Action == ActionCreateNode && v.lifecycleLoader != nil {
						if isTerm, err := v.lifecycleLoader.IsTerminalStatusForKind(mut.TargetKind, statusStr); err == nil && isTerm {
							receipt.Violations = append(receipt.Violations, SchemaViolation{
								FieldPath:         "fields.status",
								FailingConstraint: "invalid_lifecycle_transition",
								Expected:          "non-terminal status upon object creation",
								Actual:            statusStr,
								Remediation:       "Newly created objects cannot start in a terminal status without break-glass authorization",
							})
						}
					}
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

		// Spec-driven validation for value-restricted enum and pattern fields
		if v.specLoader != nil && mut.TargetKind != "" {
			if spec, err := v.specLoader.LoadSpecWithInheritance(mut.TargetKind + ".yaml"); err == nil && spec != nil && spec.ResolvedFields != nil {
				for fName, fVal := range mut.Fields {
					if fName == "status" || fName == "priority_tier" || fName == "title" {
						continue
					}
					fieldDefRaw, exists := spec.ResolvedFields[fName]
					if !exists {
						continue
					}
					fieldDef, ok := fieldDefRaw.(map[string]any)
					if !ok {
						continue
					}
					if valMap, ok := fieldDef["validation"].(map[string]any); ok {
						if enumRaw, hasEnum := valMap["enum"]; hasEnum && enumRaw != nil {
							allowedValues := parseEnumStrings(enumRaw)
							if len(allowedValues) > 0 {
								valStr := fmt.Sprintf("%v", fVal)
								matched := false
								for _, allowed := range allowedValues {
									if valStr == allowed {
										matched = true
										break
									}
								}
								if !matched {
									receipt.Violations = append(receipt.Violations, SchemaViolation{
										FieldPath:         "fields." + fName,
										FailingConstraint: "enum_membership",
										Expected:          fmt.Sprintf("one of: [%s]", strings.Join(allowedValues, ", ")),
										Actual:            valStr,
										Remediation:       fmt.Sprintf("Set %s to one of the authorized spec enum values: %s", fName, strings.Join(allowedValues, ", ")),
									})
								}
							}
						}
					}
				}
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

func parseEnumStrings(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		res := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				res = append(res, s)
			} else if item != nil {
				res = append(res, fmt.Sprintf("%v", item))
			}
		}
		return res
	default:
		return nil
	}
}
