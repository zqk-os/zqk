package workflow

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestValidateTraceTargetKind(t *testing.T) {
	require.NoError(t, validateTraceTargetKind("REQ-001"))
	require.NoError(t, validateTraceTargetKind("GOAL-001"))
	require.NoError(t, validateTraceTargetKind("MIL-001"))

	err := validateTraceTargetKind("BLI-001")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only supported for requirements, goals, and milestones")

	err = validateTraceTargetKind("TST-001")
	assert.Error(t, err)
}

func TestCoerceRequirementPriorityForCASUpdate_Extended(t *testing.T) {
	// 1. Nil map
	coerceRequirementPriorityForCASUpdate(nil)

	// 2. Critical / p0
	obj := map[string]any{objects.FieldKeyPriority: "critical"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "p0", obj[objects.FieldKeyPriority])

	obj = map[string]any{objects.FieldKeyPriority: "P0"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "p0", obj[objects.FieldKeyPriority])

	// 3. High / p1
	obj = map[string]any{objects.FieldKeyPriority: "high"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "p1", obj[objects.FieldKeyPriority])

	obj = map[string]any{objects.FieldKeyPriority: "P1"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "p1", obj[objects.FieldKeyPriority])

	// 4. Medium / p2
	obj = map[string]any{objects.FieldKeyPriority: "medium"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "p2", obj[objects.FieldKeyPriority])

	obj = map[string]any{objects.FieldKeyPriority: "P2"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "p2", obj[objects.FieldKeyPriority])

	// 5. Low / p3
	obj = map[string]any{objects.FieldKeyPriority: "low"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "p3", obj[objects.FieldKeyPriority])

	obj = map[string]any{objects.FieldKeyPriority: "P3"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "p3", obj[objects.FieldKeyPriority])

	// 6. Unknown / custom
	obj = map[string]any{objects.FieldKeyPriority: "custom"}
	coerceRequirementPriorityForCASUpdate(obj)
	assert.Equal(t, "custom", obj[objects.FieldKeyPriority])
}

func TestPulseMeaningfulActivityWhileWaiting(t *testing.T) {
	proc := &cli.Processor{}
	stop := pulseMeaningfulActivityWhileWaiting(proc)
	require.NotNil(t, stop)
	stop()
}

type mockPipelineStorage struct {
	storage.ObjectStorageProvider
	data      map[string]map[string]any
	neighbors map[string][]map[string]any
}

func (m *mockPipelineStorage) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.data[id]; ok {
		return obj, nil
	}
	return nil, fmt.Errorf("object %s not found", id)
}

func (m *mockPipelineStorage) GetNeighbors(ctx context.Context, sec *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	if n, ok := m.neighbors[id]; ok {
		return n, nil
	}
	return nil, nil
}

func (m *mockPipelineStorage) Create(ctx context.Context, sec *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	if id != "" {
		m.data[id] = obj
	}
	return nil
}

func (m *mockPipelineStorage) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, obj map[string]any) error {
	m.data[id] = obj
	return nil
}

func TestRunGenTracePipeline_ValidationAndErrors(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockPipelineStorage{
		data:      make(map[string]map[string]any),
		neighbors: make(map[string][]map[string]any),
	}

	cmd := NewGenTracePipelineCmd()
	initTestCmdContext(t, cmd, proj.Root, store)

	// 1. Missing args
	err := runGenTracePipeline(cmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing object ID")

	// 2. Invalid target kind
	err = runGenTracePipeline(cmd, []string{"BLI-1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only supported for requirements, goals, and milestones")

	// 3. Target not found
	err = runGenTracePipeline(cmd, []string{"REQ-NOT-FOUND"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load target object")

	// 4. ApplyGeneratedTracePipelineFromCmd error
	err = ApplyGeneratedTracePipelineFromCmd(cmd, "REQ-NOT-FOUND")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load target object")
}

func TestApplyGeneratedTracePipeline_AlreadyEstablished(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Requirement with 3 criteria and already has test cases and BLIs
	reqID := "REQ-ESTABLISHED"
	critIDs := []any{"CRIT-1", "CRIT-2", "CRIT-3"}
	critObj1 := map[string]any{objects.FieldKeyID: "CRIT-1", "_kind": objects.KindCriteria}
	critObj2 := map[string]any{objects.FieldKeyID: "CRIT-2", "_kind": objects.KindCriteria}
	critObj3 := map[string]any{objects.FieldKeyID: "CRIT-3", "_kind": objects.KindCriteria}
	tcObj := map[string]any{
		objects.FieldKeyID:              "TST-1",
		"_kind":                         objects.KindTestCase,
		objects.FieldKeyCriteriaRefs:    critIDs,
		objects.FieldKeyRequirementRefs: []any{reqID},
	}
	bliObj1 := map[string]any{
		objects.FieldKeyID:           "BLI-1",
		"_kind":                      objects.KindBacklogItem,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
	}
	bliObj2 := map[string]any{
		objects.FieldKeyID:           "BLI-2",
		"_kind":                      objects.KindBacklogItem,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-2"},
	}
	bliObj3 := map[string]any{
		objects.FieldKeyID:           "BLI-3",
		"_kind":                      objects.KindBacklogItem,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-3"},
	}

	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		"_kind":                      objects.KindRequirement,
		objects.FieldKeyTitle:        "Established Req",
		objects.FieldKeyCriteriaRefs: critIDs,
	}

	store := &mockPipelineStorage{
		data: map[string]map[string]any{
			reqID: reqObj,
		},
		neighbors: map[string][]map[string]any{
			reqID: {critObj1, critObj2, critObj3, tcObj, bliObj1, bliObj2, bliObj3},
		},
	}

	cmd := NewGenTracePipelineCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	initTestCmdContext(t, cmd, proj.Root, store)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	err = applyGeneratedTracePipelineForID(cmd, proc, reqID, 3)
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "Pipeline is already fully established")
}

func TestApplyGeneratedTracePipelineForID_GoalBranch(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	goalID := "GOAL-TEST-001"
	reqID := "REQ-LINKED-001"
	goalObj := map[string]any{
		objects.FieldKeyID:    goalID,
		"_kind":               objects.KindGoal,
		objects.FieldKeyTitle: "Test Goal",
	}
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		"_kind":                      objects.KindRequirement,
		objects.FieldKeyTitle:        "Linked Req",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-101"},
	}

	store := &mockPipelineStorage{
		data: map[string]map[string]any{
			goalID: goalObj,
			reqID:  reqObj,
		},
		neighbors: map[string][]map[string]any{
			goalID: {reqObj},
			reqID:  {reqObj},
		},
	}

	cmd := NewGenTracePipelineCmd()
	initTestCmdContext(t, cmd, proj.Root, store)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	// Call applyGeneratedTracePipelineForID which will execute goal inspection branches (lines 430-454)
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	_ = applyGeneratedTracePipelineForID(cmd, proc, goalID, 3)
}
