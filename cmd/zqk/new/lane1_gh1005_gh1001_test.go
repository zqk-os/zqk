package newcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/cmd/zqk/workflow"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// TestGH1001_ApplyPriorityPlanRelationalFlags tests mapping --plan, --req, --goal into related_object_refs.
func TestGH1001_ApplyPriorityPlanRelationalFlags(t *testing.T) {
	objData := map[string]any{
		objects.FieldKeyKind:            objects.KindPriorityPlan,
		objects.FieldKeyTitle:           "Test Priority Plan",
		objects.FieldKeyRequirementRefs: []any{"REQ-OLD"},
		objects.FieldKeyGoalRefs:        []any{"GOAL-OLD"},
	}

	applyPriorityPlanRelationalFlags(
		objData,
		[]string{"PRI-PARENT-01"},
		[]string{"REQ-FEAT-01", "REQ-FEAT-02"},
		[]string{"GOAL-ROOT-01"},
	)

	// requirement_refs and goal_refs must be removed
	require.Nil(t, objData[objects.FieldKeyRequirementRefs])
	require.Nil(t, objData[objects.FieldKeyGoalRefs])

	// related_object_refs must contain all provided refs
	relRefs, ok := objData[objects.FieldKeyRelatedObjectRefs].([]string)
	require.True(t, ok, "related_object_refs should be []string")
	require.Equal(t, []string{"PRI-PARENT-01", "REQ-FEAT-01", "REQ-FEAT-02", "GOAL-ROOT-01"}, relRefs)
}

// TestGH1001_PriorityPlanCliFlagsMapsToRelatedObjectRefs tests running zqk new priority_plan with --req/--goal/--plan.
func TestGH1001_PriorityPlanCliFlagsMapsToRelatedObjectRefs(t *testing.T) {
	testRoot := setupTestProjectRoot(t)
	require.NotEmpty(t, testRoot)

	var outBuf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{
		"priority_plan",
		"--title", "Cycle 49 Sprint Plan",
		"--description", "Execution plan with relational targets",
		"--req", "REQ-1793942598159999000-1f95def0",
		"--goal", "GOAL-1793942555877398000-0e265c48",
		"--plan", "PRI-1793942634446815000-86ddef6c",
	})

	err := cmd.Execute()
	require.NoError(t, err)

	// Read the minted draft object
	targetPlan := readLatestDraft(t, testRoot, objects.KindPriorityPlan)
	require.NotNil(t, targetPlan, "expected to find minted plan")

	// Ensure requirement_refs and goal_refs are absent or empty
	require.Empty(t, targetPlan[objects.FieldKeyRequirementRefs])
	require.Empty(t, targetPlan[objects.FieldKeyGoalRefs])

	// Ensure related_object_refs contains REQ, GOAL, and parent PLAN
	relRefs := targetPlan[objects.FieldKeyRelatedObjectRefs]
	require.NotNil(t, relRefs)
	var asStrings []string
	switch v := relRefs.(type) {
	case []any:
		for _, item := range v {
			asStrings = append(asStrings, item.(string))
		}
	case []string:
		asStrings = v
	}
	require.Contains(t, asStrings, "REQ-1793942598159999000-1f95def0")
	require.Contains(t, asStrings, "GOAL-1793942555877398000-0e265c48")
	require.Contains(t, asStrings, "PRI-1793942634446815000-86ddef6c")
}

// TestGH1005_FlushAndRefreshRequirementCASIndex tests flushing .requirement.index and .test_case.index.
func TestGH1005_FlushAndRefreshRequirementCASIndex(t *testing.T) {
	testRoot := setupTestProjectRoot(t)
	require.NotEmpty(t, testRoot)

	// Mint a CAS requirement directly
	var outBuf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{
		"req",
		"--title", "CAS Traceability Sync Requirement",
		"--description", "Requirement to verify CAS index flush before trace pipeline generation",
		"--goal", "GOAL-TEST-001",
		"--cas",
	})

	err := cmd.Execute()
	require.NoError(t, err)

	// In CAS mode, find the created requirement in CAS storage
	reqDir := filepath.Join(testRoot, paths.ProcessDir, objects.GetDirectoryFromKind(objects.KindRequirement))
	cas := filecas.NewContentAddressableStorage(reqDir, objects.KindRequirement)
	require.NotNil(t, cas)
	_ = cas.EnsureIndexPopulated()

	var reqID string
	for id := range cas.GetIndex().Mappings {
		reqID = id
		break
	}
	require.NotEmpty(t, reqID, "expected requirement to be present in CAS")

	// Call FlushAndRefreshRequirementCASIndex
	err = FlushAndRefreshRequirementCASIndex(testRoot, reqID)
	require.NoError(t, err)

	// Verify .requirement.index exists and maps reqID
	hash, err := cas.GetHashForID(reqID)
	require.NoError(t, err, "reqID must be indexed in CAS after FlushAndRefreshRequirementCASIndex")
	require.NotEmpty(t, hash)

	// Verify that workflow.ApplyGeneratedTracePipelineFromCmd succeeds with the CAS requirement
	pipeCmd := &cobra.Command{Use: "test"}
	pipeCmd.Flags().Bool("dry-run", false, "")
	pipeErr := workflow.ApplyGeneratedTracePipelineFromCmd(pipeCmd, reqID)
	require.NoError(t, pipeErr, "ApplyGeneratedTracePipelineFromCmd must succeed for CAS requirement")

	// Verify that generated test cases have their entries added to .test_case.index
	testDir := filepath.Join(testRoot, paths.ProcessDir, objects.GetDirectoryFromKind(objects.KindTestCase))
	testCas := filecas.NewContentAddressableStorage(testDir, objects.KindTestCase)
	require.NotNil(t, testCas)
	require.Greater(t, testCas.GetIndex().Len(), 0, "test_case index must contain newly generated test cases")

	// Verify requirement in CAS now links to the new criteria
	updatedCas := filecas.NewContentAddressableStorage(reqDir, objects.KindRequirement)
	data, readErr := updatedCas.Read(reqID)
	require.NoError(t, readErr)
	var updatedReq map[string]any
	require.NoError(t, yaml.Unmarshal(data, &updatedReq))
	critRefs, ok := updatedReq[objects.FieldKeyCriteriaRefs].([]any)
	require.True(t, ok)
	require.NotEmpty(t, critRefs, "requirement in CAS must be updated with generated criteria refs")
}
