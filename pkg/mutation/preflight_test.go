package mutation_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Satisfies CRIT-ZQL-INMEMORY-VALIDATION-CONTRACT:
// Specification defining the pre-persistence validation interface and protocol that validates
// structural schemas, required properties, and field type invariants entirely in-memory without initiating disk writes.
func TestPreflight_InMemoryValidationContract(t *testing.T) {
	// 1. Verify formal specification document exists and contains required contract definitions
	specCandidates := []string{
		filepath.Join("..", "..", "docs", "specs", "SPEC-ZQL-PREFLIGHT-VALIDATION.md"),
		filepath.Join("docs", "specs", "SPEC-ZQL-PREFLIGHT-VALIDATION.md"),
	}
	var specContent string
	var found bool
	for _, p := range specCandidates {
		if data, err := fileutil.ReadFile(p); err == nil {
			specContent = string(data)
			found = true
			break
		}
	}
	require.True(t, found, "SPEC-ZQL-PREFLIGHT-VALIDATION.md must exist in docs/specs/")
	require.Contains(t, specContent, "PreflightValidator")
	require.Contains(t, specContent, "PreflightDiagnosticReceipt")
	require.Contains(t, specContent, "Zero Disk I/O Invariant")

	// 2. In-Memory Validation Protocol Test (No Disk I/O)
	ctx := context.Background()
	validator := mutation.NewDefaultPreflightValidator()

	validMut := mutation.Mutation{
		Action:     mutation.ActionCreateNode,
		TargetKind: "backlog_item",
		TargetID:   "BLI-1001",
		Fields: map[string]any{
			"title":         "Implement Streaming",
			"priority_tier": "P1",
			"status":        "planned",
			"criteria_refs": []string{"CRIT-101", "CRIT-102"},
		},
	}

	receipt, err := validator.Validate(ctx, &validMut)
	require.NoError(t, err)
	require.True(t, receipt.Valid)
	require.Equal(t, "accepted", receipt.Disposition)
	require.Empty(t, receipt.Violations)
}

// Satisfies CRIT-ZQL-PREFLIGHT-DIAGNOSTIC-RECEIPT:
// Validation pass produces machine-parseable, structured diagnostic receipts detailing exact schema violations,
// target field paths, failing constraints, and recommended remediation actions for invalid mutation candidates.
func TestPreflight_DiagnosticReceiptGeneration(t *testing.T) {
	ctx := context.Background()
	validator := mutation.NewDefaultPreflightValidator()

	// Candidate with multiple simultaneous violations
	invalidMut := mutation.Mutation{
		Action:     "invalid_action",
		TargetKind: "backlog_item",
		TargetID:   "INVALID_ID_WITHOUT_HYPHEN",
		Fields: map[string]any{
			"priority_tier": "URGENT",   // Invalid enum
			"status":        "exploded", // Invalid enum
			"title":         12345,      // Type mismatch (int instead of string)
			"criteria_refs": "CRIT-001", // Type mismatch (string instead of array)
		},
	}

	receipt, err := validator.Validate(ctx, &invalidMut)
	require.NoError(t, err)
	require.False(t, receipt.Valid)
	require.Equal(t, "rejected_failclosed", receipt.Disposition)
	require.NotEmpty(t, receipt.Violations)

	// Ensure structured violation fields exist and are populated
	foundPriorityViolation := false
	foundTitleViolation := false
	foundActionViolation := false
	foundIDViolation := false

	for _, v := range receipt.Violations {
		require.NotEmpty(t, v.FieldPath, "violation must have field_path")
		require.NotEmpty(t, v.FailingConstraint, "violation must have failing_constraint")
		require.NotEmpty(t, v.Expected, "violation must have expected description")
		require.NotEmpty(t, v.Actual, "violation must have actual description")
		require.NotEmpty(t, v.Remediation, "violation must have recommended remediation")

		if v.FieldPath == "fields.priority_tier" {
			foundPriorityViolation = true
			require.Equal(t, "enum_membership", v.FailingConstraint)
		}
		if v.FieldPath == "fields.title" {
			foundTitleViolation = true
			require.Equal(t, "type_mismatch", v.FailingConstraint)
		}
		if v.FieldPath == "action" {
			foundActionViolation = true
			require.Equal(t, "enum_membership", v.FailingConstraint)
		}
		if v.FieldPath == "target_id" {
			foundIDViolation = true
			require.Equal(t, "id_pattern_mismatch", v.FailingConstraint)
		}
	}

	require.True(t, foundPriorityViolation, "must diagnose priority_tier enum violation")
	require.True(t, foundTitleViolation, "must diagnose title type mismatch violation")
	require.True(t, foundActionViolation, "must diagnose action violation")
	require.True(t, foundIDViolation, "must diagnose target_id pattern mismatch")
}

// Satisfies CRIT-ZQL-SCHEMA-CORRUPTION-FAILCLOSED-NEGATIVE:
// Validation engine verifies that candidate entities with type mismatches, disallowed enum constants,
// pattern violations, or missing mandatory fields fail closed, preventing storage layer mutation dispatch.
func TestPreflight_SchemaCorruptionFailClosedNegative(t *testing.T) {
	ctx := context.Background()
	validator := mutation.NewDefaultPreflightValidator()

	testCases := []struct {
		name                 string
		mutation             mutation.Mutation
		expectedViolatedPath string
		expectedConstraint   string
	}{
		{
			name: "missing mandatory action",
			mutation: mutation.Mutation{
				TargetKind: "backlog_item",
				TargetID:   "BLI-001",
			},
			expectedViolatedPath: "action",
			expectedConstraint:   "mandatory_field",
		},
		{
			name: "missing mandatory target_kind",
			mutation: mutation.Mutation{
				Action:   mutation.ActionCreateNode,
				TargetID: "BLI-002",
			},
			expectedViolatedPath: "target_kind",
			expectedConstraint:   "mandatory_field",
		},
		{
			name: "disallowed priority_tier enum",
			mutation: mutation.Mutation{
				Action:     mutation.ActionCreateNode,
				TargetKind: "backlog_item",
				TargetID:   "BLI-003",
				Fields: map[string]any{
					"title":         "Valid title",
					"priority_tier": "CRITICAL_P99",
				},
			},
			expectedViolatedPath: "fields.priority_tier",
			expectedConstraint:   "enum_membership",
		},
		{
			name: "type mismatch in field",
			mutation: mutation.Mutation{
				Action:     mutation.ActionCreateNode,
				TargetKind: "backlog_item",
				TargetID:   "BLI-004",
				Fields: map[string]any{
					"title": 99999, // int instead of string
				},
			},
			expectedViolatedPath: "fields.title",
			expectedConstraint:   "type_mismatch",
		},
		{
			name: "target_id pattern violation",
			mutation: mutation.Mutation{
				Action:     mutation.ActionCreateNode,
				TargetKind: "backlog_item",
				TargetID:   "malformed_id_without_prefix",
				Fields: map[string]any{
					"title": "Valid title",
				},
			},
			expectedViolatedPath: "target_id",
			expectedConstraint:   "id_pattern_mismatch",
		},
		{
			name: "missing mandatory title for node creation",
			mutation: mutation.Mutation{
				Action:     mutation.ActionCreateNode,
				TargetKind: "backlog_item",
				TargetID:   "BLI-005",
				Fields:     map[string]any{},
			},
			expectedViolatedPath: "fields.title",
			expectedConstraint:   "mandatory_field",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			receipt, err := validator.Validate(ctx, &tc.mutation)
			require.NoError(t, err)
			require.False(t, receipt.Valid, "expected fail-closed invalid disposition")
			require.Equal(t, "rejected_failclosed", receipt.Disposition)

			matched := false
			for _, v := range receipt.Violations {
				if v.FieldPath == tc.expectedViolatedPath && v.FailingConstraint == tc.expectedConstraint {
					matched = true
					break
				}
			}
			require.True(t, matched, "expected violation on %s with constraint %s", tc.expectedViolatedPath, tc.expectedConstraint)
		})
	}

	// Batch validation negative test
	var muts []mutation.Mutation
	for _, tc := range testCases {
		muts = append(muts, tc.mutation)
	}
	batchReceipt, err := validator.ValidateBatch(ctx, muts)
	require.NoError(t, err)
	require.False(t, batchReceipt.AllValid)
	require.Equal(t, len(testCases), batchReceipt.InvalidCount)
	require.Equal(t, 0, batchReceipt.ValidCount)
}

func TestPreflight_SystemManagedFieldsRestriction(t *testing.T) {
	ctx := context.Background()
	validator := mutation.NewDefaultPreflightValidator()

	systemFieldsToTest := []string{
		"created_at",
		"created_by",
		"updated_at",
		"updated_by",
		"archived_at",
		"archived_by",
		"version",
		"status_history",
		"change_log",
	}

	for _, sf := range systemFieldsToTest {
		t.Run("rejects_"+sf, func(t *testing.T) {
			mut := mutation.Mutation{
				Action:     mutation.ActionCreateNode,
				TargetKind: "backlog_item",
				TargetID:   "BLI-SYS-001",
				Fields: map[string]any{
					"title": "Legitimate Title",
					sf:      "malicious_or_manual_value",
				},
			}

			receipt, err := validator.Validate(ctx, &mut)
			require.NoError(t, err)
			require.False(t, receipt.Valid, "expected rejection when %s is explicitly specified", sf)
			require.Equal(t, "rejected_failclosed", receipt.Disposition)

			found := false
			for _, v := range receipt.Violations {
				if v.FieldPath == "fields."+sf && v.FailingConstraint == "system_managed_field" {
					found = true
					break
				}
			}
			require.True(t, found, "expected system_managed_field violation for %s", sf)
		})
	}

	t.Run("allows_system_fields_with_break_glass", func(t *testing.T) {
		bgCtx := pkgctx.WithLifecycleBreakGlass(ctx, "authorized emergency data migration")
		mut := mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "backlog_item",
			TargetID:   "BLI-SYS-002",
			Fields: map[string]any{
				"title":      "Emergency Migrated Task",
				"created_at": "2026-01-01T00:00:00Z",
				"created_by": "ACC-LEGACY-MIGRATOR",
			},
		}

		receipt, err := validator.Validate(bgCtx, &mut)
		require.NoError(t, err)
		require.True(t, receipt.Valid, "break-glass must permit system-managed fields")
		require.Equal(t, "accepted", receipt.Disposition)
		require.Empty(t, receipt.Violations)
	})
}

func TestPreflight_SpecValueRestrictedEnumValidation(t *testing.T) {
	ctx := context.Background()
	validator := mutation.NewDefaultPreflightValidator()

	t.Run("criteria_category_validation", func(t *testing.T) {
		// Disallowed category
		invalidMut := mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "criteria",
			TargetID:   "CRIT-CAT-001",
			Fields: map[string]any{
				"title":    "Valid Criteria Title",
				"category": "not_a_valid_category_enum",
			},
		}
		receipt, err := validator.Validate(ctx, &invalidMut)
		require.NoError(t, err)
		require.False(t, receipt.Valid)

		found := false
		for _, v := range receipt.Violations {
			if v.FieldPath == "fields.category" && v.FailingConstraint == "enum_membership" {
				found = true
				require.Contains(t, v.Expected, "functional")
				require.Contains(t, v.Expected, "security")
				break
			}
		}
		require.True(t, found, "must diagnose category enum violation against spec")

		// Allowed category
		validMut := mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "criteria",
			TargetID:   "CRIT-CAT-002",
			Fields: map[string]any{
				"title":    "Valid Security Criteria",
				"category": "security",
			},
		}
		validReceipt, err := validator.Validate(ctx, &validMut)
		require.NoError(t, err)
		require.True(t, validReceipt.Valid)
	})

	t.Run("milestone_stage_type_validation", func(t *testing.T) {
		invalidMut := mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "milestone",
			TargetID:   "MIL-STG-001",
			Fields: map[string]any{
				"title":      "Valid Milestone Title",
				"stage_type": "disallowed_type",
			},
		}
		receipt, err := validator.Validate(ctx, &invalidMut)
		require.NoError(t, err)
		require.False(t, receipt.Valid)

		found := false
		for _, v := range receipt.Violations {
			if v.FieldPath == "fields.stage_type" && v.FailingConstraint == "enum_membership" {
				found = true
				require.Contains(t, v.Expected, "tier")
				require.Contains(t, v.Expected, "stage")
				break
			}
		}
		require.True(t, found, "must diagnose stage_type enum violation against spec")

		validMut := mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "milestone",
			TargetID:   "MIL-STG-002",
			Fields: map[string]any{
				"title":      "Valid Milestone Title",
				"stage_type": "stage",
			},
		}
		validReceipt, err := validator.Validate(ctx, &validMut)
		require.NoError(t, err)
		require.True(t, validReceipt.Valid)
	})
}

func TestPreflight_LifecycleStateTransitionEnforcement(t *testing.T) {
	ctx := context.Background()

	t.Run("enforces_valid_transition_on_update", func(t *testing.T) {
		// Mock status resolver returning "in_progress" for PRI-PLAN-001
		validator := mutation.NewDefaultPreflightValidator().WithStatusResolver(func(ctx context.Context, id string) (string, bool) {
			if id == "PRI-PLAN-001" {
				return "in_progress", true
			}
			return "", false
		})

		// Transition from in_progress to active is illegal in priority_plan_lifecycle.yaml (check valve)
		illegalMut := mutation.Mutation{
			Action:     mutation.ActionUpdateNode,
			TargetKind: "priority_plan",
			TargetID:   "PRI-PLAN-001",
			Fields: map[string]any{
				"status": "active",
			},
		}

		receipt, err := validator.Validate(ctx, &illegalMut)
		require.NoError(t, err)
		require.False(t, receipt.Valid, "illegal transition in_progress -> active must fail preflight")

		found := false
		for _, v := range receipt.Violations {
			if v.FieldPath == "fields.status" && v.FailingConstraint == "invalid_lifecycle_transition" {
				found = true
				break
			}
		}
		require.True(t, found, "expected invalid_lifecycle_transition violation")

		// Legal transition: in_progress -> paused
		legalMut := mutation.Mutation{
			Action:     mutation.ActionUpdateNode,
			TargetKind: "priority_plan",
			TargetID:   "PRI-PLAN-001",
			Fields: map[string]any{
				"status": "paused",
			},
		}
		legalReceipt, err := validator.Validate(ctx, &legalMut)
		require.NoError(t, err)
		require.True(t, legalReceipt.Valid, "legal transition in_progress -> paused must pass")

		// Break-glass allows emergency transition override
		bgCtx := pkgctx.WithLifecycleBreakGlass(ctx, "operator emergency realignment of stalled plan")
		bgReceipt, err := validator.Validate(bgCtx, &illegalMut)
		require.NoError(t, err)
		require.True(t, bgReceipt.Valid, "break-glass must allow emergency transition override")
	})

	t.Run("rejects_terminal_status_on_create_without_break_glass", func(t *testing.T) {
		validator := mutation.NewDefaultPreflightValidator()

		// Creating directly into complete
		terminalMut := mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "backlog_item",
			TargetID:   "BLI-TERM-001",
			Fields: map[string]any{
				"title":  "Instant Done Item",
				"status": "complete",
			},
		}

		receipt, err := validator.Validate(ctx, &terminalMut)
		require.NoError(t, err)
		require.False(t, receipt.Valid, "cannot create object directly into terminal status")

		found := false
		for _, v := range receipt.Violations {
			if v.FieldPath == "fields.status" && v.FailingConstraint == "invalid_lifecycle_transition" {
				found = true
				break
			}
		}
		require.True(t, found, "expected invalid_lifecycle_transition violation on terminal create")

		// With break-glass
		bgCtx := pkgctx.WithLifecycleBreakGlass(ctx, "seeding historically completed backlog records")
		bgReceipt, err := validator.Validate(bgCtx, &terminalMut)
		require.NoError(t, err)
		require.True(t, bgReceipt.Valid, "break-glass allows seeding completed objects")
	})

	t.Run("rejects_illegal_directed_transition_and_reports_allowed_targets", func(t *testing.T) {
		validator := mutation.NewDefaultPreflightValidator().WithStatusResolver(func(ctx context.Context, id string) (string, bool) {
			switch id {
			case "BLI-PRE-001":
				return "conceptual", true
			case "TST-PRE-001":
				return "draft", true
			case "CRIT-PRE-001":
				return "conceptual", true
			}
			return "", false
		})

		// 1. Illegal transition: backlog_item conceptual -> complete
		illegalMut := mutation.Mutation{
			Action:     mutation.ActionUpdateNode,
			TargetKind: "backlog_item",
			TargetID:   "BLI-PRE-001",
			Fields: map[string]any{
				"status": "complete",
			},
		}

		receipt, err := validator.Validate(ctx, &illegalMut)
		require.NoError(t, err)
		require.False(t, receipt.Valid, "illegal transition conceptual -> complete must fail")

		var matchedViolation *mutation.SchemaViolation
		for i := range receipt.Violations {
			if receipt.Violations[i].FieldPath == "fields.status" && receipt.Violations[i].FailingConstraint == "invalid_lifecycle_transition" {
				matchedViolation = &receipt.Violations[i]
				break
			}
		}
		require.NotNil(t, matchedViolation, "expected invalid_lifecycle_transition violation")
		require.Contains(t, matchedViolation.Expected, "allowed targets")
		require.Contains(t, matchedViolation.Expected, "originated")
		require.Contains(t, matchedViolation.Remediation, "allowed targets")
		require.Contains(t, matchedViolation.Remediation, "originated")

		// 2. Legal transition: test_case draft -> active
		legalMut := mutation.Mutation{
			Action:     mutation.ActionUpdateNode,
			TargetKind: "test_case",
			TargetID:   "TST-PRE-001",
			Fields: map[string]any{
				"status": "active",
			},
		}
		legalReceipt, err := validator.Validate(ctx, &legalMut)
		require.NoError(t, err)
		require.True(t, legalReceipt.Valid, "legal transition draft -> active must pass")

		// 3. Legal transition: criteria conceptual -> originated
		legalCritMut := mutation.Mutation{
			Action:     mutation.ActionUpdateNode,
			TargetKind: "criteria",
			TargetID:   "CRIT-PRE-001",
			Fields: map[string]any{
				"status": "originated",
			},
		}
		legalCritReceipt, err := validator.Validate(ctx, &legalCritMut)
		require.NoError(t, err)
		require.True(t, legalCritReceipt.Valid, "legal transition conceptual -> originated must pass")

		// 4. Break-glass allows emergency override of illegal transition
		bgCtx := pkgctx.WithLifecycleBreakGlass(ctx, "emergency hotfix test validation")
		bgReceipt, err := validator.Validate(bgCtx, &illegalMut)
		require.NoError(t, err)
		require.True(t, bgReceipt.Valid, "break-glass allows emergency override")
	})
}
