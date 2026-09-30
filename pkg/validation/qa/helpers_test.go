package qa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestQAHelpers_IsCompleteStatus(t *testing.T) {
	t.Parallel()

	require.True(t, IsCompleteStatus(objects.ObjectStatusComplete))
	require.True(t, IsCompleteStatus(objects.ObjectStatusCompleted))
	require.True(t, IsCompleteStatus("complete"))
	require.True(t, IsCompleteStatus("COMPLETE"))
	require.True(t, IsCompleteStatus(" completed "))

	require.False(t, IsCompleteStatus("in_progress"))
	require.False(t, IsCompleteStatus("originated"))
	require.False(t, IsCompleteStatus("planned"))
	require.False(t, IsCompleteStatus("testing"))
	require.False(t, IsCompleteStatus(""))
}

func TestQAHelpers_IsObjectComplete(t *testing.T) {
	t.Parallel()

	require.False(t, IsObjectComplete(nil))

	objEmpty := map[string]any{}
	require.False(t, IsObjectComplete(objEmpty))

	objInProgress := map[string]any{
		objects.FieldKeyStatus: "in_progress",
	}
	require.False(t, IsObjectComplete(objInProgress))

	objComplete := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	require.True(t, IsObjectComplete(objComplete))

	objCompleted := map[string]any{
		objects.FieldKeyStatus: "completed",
	}
	require.True(t, IsObjectComplete(objCompleted))
}

func TestQAHelpers_HasStringEvidence(t *testing.T) {
	t.Parallel()

	require.False(t, HasStringEvidence(nil))
	require.False(t, HasStringEvidence(""))
	require.False(t, HasStringEvidence("   "))
	require.False(t, HasStringEvidence([]string{}))
	require.False(t, HasStringEvidence([]string{"", "   "}))
	require.False(t, HasStringEvidence([]any{}))
	require.False(t, HasStringEvidence([]any{"", 123}))

	require.True(t, HasStringEvidence("commit-sha-123"))
	require.True(t, HasStringEvidence([]string{"sha-1"}))
	require.True(t, HasStringEvidence([]any{"sha-2"}))

	// Backward compatibility alias test
	require.True(t, hasStringEvidence("alias-test"))
}
