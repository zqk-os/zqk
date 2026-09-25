package pipeline_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

// TestPackageStutterElimination verifies that canonical non-stuttering types and aliases
// (pipeline.Router, pipeline.Plugin, pipeline.StorageProvider) exist and match their legacy names.
func TestPackageStutterElimination(t *testing.T) {
	// 1. Router & PlanRouter
	planRouter := pipeline.NewPlanRouter()
	require.NotNil(t, planRouter)
	legacyRouter := pipeline.NewPipelineRouter()
	require.NotNil(t, legacyRouter)

	// 2. StorageProvider
	var _ pipeline.StorageProvider = (pipeline.PipelineStorageProvider)(nil)

	// 3. Plugin
	var _ pipeline.Plugin = (pipeline.PipelinePlugin)(nil)
	var _ pipeline.Plugin = (*pipeline.ContextHydrationPlugin)(nil)
}
