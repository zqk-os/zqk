package mutation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mutation"
)

// TestProvenanceFields_PreflightRejection tests that system provenance fields
// (created_at, created_by, updated_at, updated_by, cas_address, hash)
// cannot be manually set in ZQL mutations without explicit break-glass authorization.
// Satisfies REQ-1790462550862131000-8e210c0f and CRIT-1790462551210900000-7142b1ba.
func TestProvenanceFields_PreflightRejection(t *testing.T) {
	ctx := context.Background()
	validator := mutation.NewDefaultPreflightValidator()

	provenanceFields := []string{
		"created_at",
		"created_by",
		"updated_at",
		"updated_by",
		"cas_address",
		"hash",
	}

	for _, field := range provenanceFields {
		t.Run("reject_"+field, func(t *testing.T) {
			mut := mutation.Mutation{
				Action:     mutation.ActionCreateNode,
				TargetKind: "backlog_item",
				TargetID:   "BLI-9001",
				Fields: map[string]any{
					"title": "Test Item",
					field:   "unauthorized-value",
				},
			}

			receipt, err := validator.Validate(ctx, &mut)
			require.NoError(t, err)
			require.False(t, receipt.Valid, "Validation should fail when system provenance field %s is manually specified", field)
			require.Equal(t, "rejected_failclosed", receipt.Disposition)
			require.NotEmpty(t, receipt.Violations)

			var foundViolation bool
			for _, v := range receipt.Violations {
				if v.FieldPath == "fields."+field && v.FailingConstraint == "system_managed_field" {
					foundViolation = true
					require.Contains(t, v.Expected, "field value must be computed by system runtime")
					break
				}
			}
			require.True(t, foundViolation, "Expected system_managed_field violation for %s", field)
		})
	}
}

// TestProvenanceFields_BreakGlassOverride tests that when BreakGlass context
// is armed with a valid justification reason, system provenance fields pass preflight validation.
// Satisfies CRIT-1790462551210901000-66cae168.
func TestProvenanceFields_BreakGlassOverride(t *testing.T) {
	bgCtx := pkgctx.WithLifecycleBreakGlass(context.Background(), "emergency audit data repair protocol")
	validator := mutation.NewDefaultPreflightValidator()

	mut := mutation.Mutation{
		Action:     mutation.ActionCreateNode,
		TargetKind: "backlog_item",
		TargetID:   "BLI-9002",
		Fields: map[string]any{
			"title":       "Emergency Repaired Item",
			"created_at":  "2026-01-01T00:00:00Z",
			"created_by":  "ACC-SYSTEM-AUDIT",
			"cas_address": "cas://dummy-address",
			"hash":        "deadbeef12345678",
		},
	}

	receipt, err := validator.Validate(bgCtx, &mut)
	require.NoError(t, err)
	require.True(t, receipt.Valid, "Validation should pass with break glass authorization")
	require.Equal(t, "accepted", receipt.Disposition)
	require.Empty(t, receipt.Violations)
}

// TestProvenanceFields_ZQLExecutorTransactionRollback tests that when a ZQL script
// includes statements attempting to set provenance fields, the transaction execution
// fails fail-closed, leaving zero mutations committed.
// Satisfies CRIT-1790462551210902000-20cd049a.
func TestProvenanceFields_ZQLExecutorTransactionRollback(t *testing.T) {
	ctx := context.Background()
	engine := mutation.NewTransactionEngine()
	executor := mutation.NewZQLExecutor(engine)

	script := `
BEGIN TRANSACTION ISOLATION LEVEL STAGED_SNAPSHOT;
UPSERT backlog_item {
    id: "BLI-ROLLBACK-001",
    title: "Should Not Commit",
    created_at: "2020-01-01T00:00:00Z"
};
COMMIT;
`
	program, err := mutation.ParseZQL(script)
	require.NoError(t, err)

	receipt, err := executor.Execute(ctx, program)
	require.Error(t, err)
	require.False(t, receipt.Committed)

	// Verify nothing was persisted into the engine
	_, exists := engine.Get(ctx, "BLI-ROLLBACK-001")
	require.False(t, exists, "Entity with unauthorized provenance field must not exist in engine")
}
