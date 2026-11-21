package community

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testPipelineID   = "pipe-ci-assemble"
	testCmdBuild     = "go build ./..."
	testCmdLint      = "golangci-lint run"
	testCmdTest      = "go test ./..."
	testCmdDeploy    = "zqk deploy"
	testCapGo        = "golang-toolchain"
	testStageInit    = "stage-init"
	testStageLint    = "stage-lint"
	testStageUnit    = "stage-unit"
	testStagePackage = "stage-pkg"
)

func makeValidStage(id, cmd string, budget int, deps ...string) PipelineStage {
	return PipelineStage{
		ID:          id,
		Command:     cmd,
		Timeout:     2 * time.Second,
		TokenBudget: budget,
		DependsOn:   deps,
		Capabilities: []CapabilityDescriptor{
			{Name: testCapGo, MinVersion: "1.22", Required: true},
		},
	}
}

func assertCompileFailsWith(t *testing.T, spec PipelineSpec, expectedErr error) {
	t.Helper()
	assembler := NewWorkflowAssembler()
	compiled, err := assembler.Compile(spec)
	require.Error(t, err)
	assert.Nil(t, compiled)
	assert.ErrorIs(t, err, expectedErr)
}

func TestWorkflowAssembler_StaticFloorInvariant(t *testing.T) {
	assembler := NewWorkflowAssembler()

	validSpec := PipelineSpec{
		ID:               testPipelineID,
		TotalTokenBudget: 1000,
		MaxConcurrency:   2,
		Stages: []PipelineStage{
			makeValidStage(testStageInit, testCmdBuild, 200),
			makeValidStage(testStageLint, testCmdLint, 200, testStageInit),
		},
	}
	compiled, err := assembler.Compile(validSpec)
	require.NoError(t, err)
	require.NotNil(t, compiled)
	assert.Equal(t, testPipelineID, compiled.PipelineID)
	assert.Equal(t, 2, compiled.StageCount)
	assert.Len(t, compiled.Layers, 2)

	invalidCases := []struct {
		name string
		spec PipelineSpec
	}{
		{
			name: "blank pipeline id",
			spec: PipelineSpec{
				ID:               "   ",
				TotalTokenBudget: 500,
				Stages:           []PipelineStage{makeValidStage(testStageInit, testCmdBuild, 100)},
			},
		},
		{
			name: "zero total token budget",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 0,
				Stages:           []PipelineStage{makeValidStage(testStageInit, testCmdBuild, 100)},
			},
		},
		{
			name: "negative max concurrency",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 500,
				MaxConcurrency:   -1,
				Stages:           []PipelineStage{makeValidStage(testStageInit, testCmdBuild, 100)},
			},
		},
		{
			name: "empty stages",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 500,
			},
		},
		{
			name: "blank stage id",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 500,
				Stages:           []PipelineStage{makeValidStage("  ", testCmdBuild, 100)},
			},
		},
		{
			name: "blank command",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 500,
				Stages:           []PipelineStage{makeValidStage(testStageInit, "\t", 100)},
			},
		},
		{
			name: "zero timeout",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 500,
				Stages: []PipelineStage{{
					ID:          testStageInit,
					Command:     testCmdBuild,
					Timeout:     0,
					TokenBudget: 100,
				}},
			},
		},
		{
			name: "zero stage token budget",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 500,
				Stages:           []PipelineStage{makeValidStage(testStageInit, testCmdBuild, 0)},
			},
		},
		{
			name: "blank capability descriptor name",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 500,
				Stages: []PipelineStage{{
					ID:           testStageInit,
					Command:      testCmdBuild,
					Timeout:      time.Second,
					TokenBudget:  100,
					Capabilities: []CapabilityDescriptor{{Name: " "}},
				}},
			},
		},
		{
			name: "blank dependency reference",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 500,
				Stages:           []PipelineStage{makeValidStage(testStageInit, testCmdBuild, 100, " ")},
			},
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			assertCompileFailsWith(t, tc.spec, ErrWorkflowInvalidSpec)
		})
	}
}

func TestWorkflowAssembler_OperationalDynamicProof(t *testing.T) {
	assembler := NewWorkflowAssembler()
	spec := PipelineSpec{
		ID:               testPipelineID,
		TotalTokenBudget: 2000,
		MaxConcurrency:   2,
		Stages: []PipelineStage{
			makeValidStage(testStagePackage, testCmdDeploy, 400, testStageUnit, testStageLint),
			makeValidStage(testStageUnit, testCmdTest, 500, testStageInit),
			makeValidStage(testStageLint, testCmdLint, 300, testStageInit),
			makeValidStage(testStageInit, testCmdBuild, 200),
		},
	}

	compiled, err := assembler.Compile(spec)
	require.NoError(t, err)
	require.Len(t, compiled.Layers, 3)

	// Verify deterministic lexicographical ordering within each layer.
	assert.Equal(t, testStageInit, compiled.Layers[0][0].ID)
	assert.Equal(t, testStageLint, compiled.Layers[1][0].ID)
	assert.Equal(t, testStageUnit, compiled.Layers[1][1].ID)
	assert.Equal(t, testStagePackage, compiled.Layers[2][0].ID)

	// Barrier proves Layer 1 stages execute truly concurrently.
	var arrived atomic.Int32
	barrier := make(chan struct{})
	var completedOrder []string
	orderCh := make(chan string, 4)

	handler := func(ctx context.Context, stage PipelineStage) (StageOutput, error) {
		if stage.ID == testStageLint || stage.ID == testStageUnit {
			if arrived.Add(1) == 2 {
				close(barrier)
			}
			select {
			case <-barrier:
			case <-ctx.Done():
				return StageOutput{}, ctx.Err()
			}
		}
		orderCh <- stage.ID
		return StageOutput{Output: "ok-" + stage.ID, TokensUsed: stage.TokenBudget / 2}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report, err := assembler.Execute(ctx, compiled, handler)
	require.NoError(t, err)
	close(orderCh)
	for id := range orderCh {
		completedOrder = append(completedOrder, id)
	}

	assert.Equal(t, 4, report.TotalStages)
	assert.Equal(t, 4, report.SucceededStages)
	assert.Equal(t, 0, report.FailedStages)
	assert.Equal(t, 700, report.TotalTokensUsed)
	assert.Len(t, report.Results, 4)
	assert.Equal(t, testStageInit, completedOrder[0])
	assert.Equal(t, testStagePackage, completedOrder[3])

	// Verify concurrent AssembleAndExecute invocations are race-free.
	wg := goroutinelabels.StartTestGoroutines(t, 4, "wf_assembler", "concurrent compile and execute", func(_ int) {
		rep, execErr := assembler.AssembleAndExecute(ctx, spec, func(_ context.Context, st PipelineStage) (StageOutput, error) {
			return StageOutput{Output: st.Command, TokensUsed: 10}, nil
		})
		assert.NoError(t, execErr)
		assert.Equal(t, 4, rep.SucceededStages)
	})
	wg.Wait()
}

func TestWorkflowAssembler_NegativeAdversarialBoundary(t *testing.T) {
	assembler := NewWorkflowAssembler()

	adversarialSpecs := []struct {
		name        string
		spec        PipelineSpec
		expectedErr error
	}{
		{
			name: "duplicate stage id",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 1000,
				Stages: []PipelineStage{
					makeValidStage(testStageInit, testCmdBuild, 100),
					makeValidStage(testStageInit, testCmdLint, 100),
				},
			},
			expectedErr: ErrWorkflowDuplicateStage,
		},
		{
			name: "self dependency",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 1000,
				Stages: []PipelineStage{
					makeValidStage(testStageInit, testCmdBuild, 100, testStageInit),
				},
			},
			expectedErr: ErrWorkflowSelfDependency,
		},
		{
			name: "missing upstream dependency",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 1000,
				Stages: []PipelineStage{
					makeValidStage(testStageLint, testCmdLint, 100, "ghost-stage"),
				},
			},
			expectedErr: ErrWorkflowMissingDependency,
		},
		{
			name: "two-node cycle",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 1000,
				Stages: []PipelineStage{
					makeValidStage(testStageInit, testCmdBuild, 100, testStageLint),
					makeValidStage(testStageLint, testCmdLint, 100, testStageInit),
				},
			},
			expectedErr: ErrWorkflowCycleDetected,
		},
		{
			name: "single stage exceeds pipeline token budget",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 300,
				Stages: []PipelineStage{
					makeValidStage(testStageInit, testCmdBuild, 400),
				},
			},
			expectedErr: ErrWorkflowBudgetExceeded,
		},
		{
			name: "cumulative stages exceed pipeline token budget",
			spec: PipelineSpec{
				ID:               testPipelineID,
				TotalTokenBudget: 300,
				Stages: []PipelineStage{
					makeValidStage(testStageInit, testCmdBuild, 200),
					makeValidStage(testStageLint, testCmdLint, 200, testStageInit),
				},
			},
			expectedErr: ErrWorkflowBudgetExceeded,
		},
	}

	for _, tc := range adversarialSpecs {
		t.Run(tc.name, func(t *testing.T) {
			assertCompileFailsWith(t, tc.spec, tc.expectedErr)
		})
	}

	t.Run("nil handler rejected", func(t *testing.T) {
		compiled, err := assembler.Compile(PipelineSpec{
			ID:               testPipelineID,
			TotalTokenBudget: 500,
			Stages:           []PipelineStage{makeValidStage(testStageInit, testCmdBuild, 100)},
		})
		require.NoError(t, err)
		_, err = assembler.Execute(context.Background(), compiled, nil)
		assert.ErrorIs(t, err, ErrWorkflowNilHandler)
	})

	t.Run("stage failure aborts sibling and downstream layers", func(t *testing.T) {
		spec := PipelineSpec{
			ID:               testPipelineID,
			TotalTokenBudget: 1000,
			MaxConcurrency:   2,
			Stages: []PipelineStage{
				makeValidStage(testStageInit, testCmdBuild, 100),
				makeValidStage(testStageLint, testCmdLint, 100),
				makeValidStage(testStagePackage, testCmdDeploy, 100, testStageInit, testStageLint),
			},
		}
		boom := errors.New("boom")
		var pkgCalled atomic.Bool
		report, err := assembler.AssembleAndExecute(context.Background(), spec, func(ctx context.Context, st PipelineStage) (StageOutput, error) {
			if st.ID == testStageInit {
				return StageOutput{}, boom
			}
			if st.ID == testStagePackage {
				pkgCalled.Store(true)
			}
			<-ctx.Done()
			return StageOutput{}, ctx.Err()
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrWorkflowStageFailed)
		assert.ErrorIs(t, err, boom)
		assert.False(t, pkgCalled.Load())
		require.NotNil(t, report)
		assert.Equal(t, 2, report.FailedStages)
	})
}
