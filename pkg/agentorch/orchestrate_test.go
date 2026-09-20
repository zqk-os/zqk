package agentorch_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentorch"
	"github.com/stretchr/testify/require"
)

func TestOrchestrationEngine(t *testing.T) {
	ctx := context.Background()
	engine := agentorch.NewEngine("antigravity-1")

	err := engine.Dispatch(ctx, "ATK-001", "antigravity-1")
	require.NoError(t, err)
	require.True(t, engine.IsActive("ATK-001"))
}
