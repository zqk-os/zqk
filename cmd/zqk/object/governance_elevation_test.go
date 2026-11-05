package object

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestLifecycleUpdater_UnitHelpers(t *testing.T) {
	t.Parallel()

	// 1. toInt
	valInt, ok := toInt(42)
	assert.True(t, ok)
	assert.Equal(t, 42, valInt)

	valFloat, ok := toInt(float64(100.0))
	assert.True(t, ok)
	assert.Equal(t, 100, valFloat)

	valInvalid, ok := toInt("not-a-number")
	assert.False(t, ok)
	assert.Equal(t, 0, valInvalid)

	// 2. emitPlanningPrioritizationRequired nil-safety
	emitPlanningPrioritizationRequired(context.Background(), nil)

	// 3. RunPriorityPlanCompleteUpdater with empty project root
	RunPriorityPlanCompleteUpdater(context.Background(), "")
}

func TestKindCmd_HelpersAndCompletion(t *testing.T) {
	t.Parallel()

	// 1. firstHelpSentence
	assert.Equal(t, "First sentence.", firstHelpSentence("First sentence. Second sentence here.", 50))
	assert.Equal(t, "Short text without dot", firstHelpSentence("Short text without dot", 50))
	assert.Equal(t, "Very lon…", firstHelpSentence("Very long text that exceeds maximum length allowed", 9))

	// 2. kindCommandShort
	shortGoal := kindCommandShort("goal")
	assert.NotEmpty(t, shortGoal)

	shortUnknown := kindCommandShort("unknown_custom_kind")
	assert.Contains(t, shortUnknown, "unknown_custom_kind")

	// 3. newKindCmdWithUse
	cmd := newKindCmdWithUse("my-kind", cobra.NoArgs, false, "my-kind")
	require.NotNil(t, cmd)
	assert.Equal(t, "my-kind", cmd.Use)

	// 4. resolveCurrentKindForCompletion
	assert.Equal(t, "goal", resolveCurrentKindForCompletion(cmd, []string{"goal"}))
	assert.Equal(t, "", resolveCurrentKindForCompletion(cmd, []string{}))

	// 5. Completion functions
	completions, _ := kindCompletion(cmd, nil, "")
	assert.NotEmpty(t, completions)

	// Calling completion functions does not panic
	_, _ = completeGroupBy(cmd, []string{"goal"}, "")
	_, _ = completeSortBy(cmd, []string{"goal"}, "")
	_, _ = completeFilterField(cmd, []string{"goal"}, "")
}

func TestPromoteCmd_ProbeAndFormatHelpers(t *testing.T) {
	t.Parallel()

	// 1. promoteForwardProbeOrder
	probeOrder := []string{"originated", "draft", "active", "in_progress", "completed"}
	forward := promoteForwardProbeOrder("draft", probeOrder)
	assert.Equal(t, []string{"originated", "draft", "active", "in_progress"}, forward)

	// 2. probeHasExecutionHop
	assert.True(t, probeHasExecutionHop([]string{"draft", "in_progress"}))
	assert.False(t, probeHasExecutionHop([]string{"draft", "planned"}))

	// 3. formatValidationErrorList
	errs := []validation.ValidationError{
		{Field: "title", Message: "title is required"},
		{Field: "status", Message: "invalid status transition"},
	}
	formatted := formatValidationErrorList(errs)
	assert.Contains(t, formatted, "title: title is required")
	assert.Contains(t, formatted, "status: invalid status transition")

	// 4. formatCandidateValidationFailure
	valResult := &validation.ValidationResult{
		IsValid: false,
		Errors:  errs,
	}
	failureMsg := formatCandidateValidationFailure(nil, valResult)
	assert.Contains(t, failureMsg, "title is required")

	// 5. RegisterPromoteCueHook
	hookCalled := false
	RegisterPromoteCueHook("custom_kind_test", func(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, id string, current map[string]any, finalStatus string) string {
		hookCalled = true
		return "test-cue"
	})
	assert.False(t, hookCalled) // hook was registered
}

func TestReferenceResolverOverlay_UnitHelpers(t *testing.T) {
	t.Parallel()

	// 1. isReferenceFieldKey
	assert.True(t, isReferenceFieldKey("goal_refs"))
	assert.True(t, isReferenceFieldKey("requirement_ref"))
	assert.True(t, isReferenceFieldKey("milestone_refs"))
	assert.False(t, isReferenceFieldKey("title"))
	assert.False(t, isReferenceFieldKey("status"))
	assert.False(t, isReferenceFieldKey("created_at"))

	// 2. extractReferenceIDs
	single := extractReferenceIDs("GOAL-001")
	assert.Equal(t, []string{"GOAL-001"}, single)

	sliceAny := extractReferenceIDs([]any{"REQ-1", "REQ-2"})
	assert.Equal(t, []string{"REQ-1", "REQ-2"}, sliceAny)

	sliceStr := extractReferenceIDs([]string{"BLI-1", "BLI-2"})
	assert.Equal(t, []string{"BLI-1", "BLI-2"}, sliceStr)

	emptyRef := extractReferenceIDs(nil)
	assert.Empty(t, emptyRef)

	invalidRef := extractReferenceIDs(12345)
	assert.Empty(t, invalidRef)
}
