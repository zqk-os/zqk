package pipeline

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func cycle144TestLogger() logging.Logger {
	return logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
}

func TestCycle144_MutationPipeline_ExecutionLifecycle(t *testing.T) {
	t.Parallel()

	exec := NewExecution()
	require.NotNil(t, exec)
	assert.Equal(t, []string{"init"}, exec.StageLog)
	assert.Equal(t, int64(0), exec.readProcessClock())

	m1 := Mutation{
		Type:        "Process",
		Payload:     map[string]any{"key": "val1"},
		StatusClass: StatusClassInternal,
	}

	rec1, err := pipelineMutate(exec, m1)
	require.NoError(t, err)
	require.NotNil(t, rec1)
	assert.True(t, rec1.ViaPipeline)
	assert.True(t, rec1.PipelineMutationApplied)
	assert.Equal(t, int64(1), rec1.ProcessClockSequence)
	assert.Equal(t, int64(1), exec.readProcessClock())
	assert.Equal(t, []string{"init", "mutated:Process"}, exec.StageLog)

	m2 := Mutation{
		Type:        "StatusUpdate",
		Payload:     map[string]any{"status": "active"},
		StatusClass: StatusClassExternal,
	}
	rec2, err := pipelineMutate(exec, m2)
	require.NoError(t, err)
	require.NotNil(t, rec2)
	assert.Equal(t, int64(2), rec2.ProcessClockSequence)
	assert.Equal(t, int64(2), exec.readProcessClock())
}

func TestCycle144_MutationPipeline_GatesAndValidation(t *testing.T) {
	t.Parallel()

	recViaPipeline := MutationRecord{
		Mutation:    Mutation{Type: "Process"},
		ViaPipeline: true,
	}
	assert.NoError(t, MutationsMustGoThroughPipeline(recViaPipeline))
	assert.NoError(t, validatePipelineGate(recViaPipeline))

	recBypassProcess := MutationRecord{
		Mutation:    Mutation{Type: "Process"},
		ViaPipeline: false,
	}
	errBypass := MutationsMustGoThroughPipeline(recBypassProcess)
	require.Error(t, errBypass)
	var gateErr ErrNonProcessMutationBypassingPipeline
	assert.True(t, errors.As(errBypass, &gateErr))
	assert.Contains(t, errBypass.Error(), "non-process mutation bypassed the pipeline")

	recBypassOther := MutationRecord{
		Mutation:    Mutation{Type: "Telemetry"},
		ViaPipeline: false,
	}
	assert.NoError(t, MutationsMustGoThroughPipeline(recBypassOther))

	errGate := validatePipelineGate(recBypassOther)
	require.Error(t, errGate)
	assert.Contains(t, errGate.Error(), "mutation did not go through pipeline")

	internalExec := newPipelineMutator()
	require.NotNil(t, internalExec)
	assert.Equal(t, int64(0), internalExec.readProcessClock())
}

func TestCycle144_Pipeline_ContextAndRunWithContext(t *testing.T) {
	t.Parallel()

	pctxDefault := NewContext(nil)
	require.NotNil(t, pctxDefault)
	assert.NotNil(t, pctxDefault.Ctx)
	assert.NotNil(t, pctxDefault.Outcome)

	customCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pctxCustom := NewContext(customCtx)
	require.NotNil(t, pctxCustom)
	assert.Equal(t, customCtx, pctxCustom.Ctx)

	builder := NewBuilder("test_ctx", cycle144TestLogger())
	builder.AddStage("step_increment", func(ctx *Context, payload any) (any, error) {
		val, ok := payload.(int)
		if !ok {
			return nil, errors.New("expected int payload")
		}
		ctx.Outcome["incremented"] = true
		return val + 10, nil
	})
	p := builder.Build()

	res, err := p.RunWithContext(context.Background(), 5)
	require.NoError(t, err)
	assert.Equal(t, 15, res)
}

func TestCycle144_Pipeline_AddRetryStage_Scenarios(t *testing.T) {
	t.Parallel()

	logger := cycle144TestLogger()
	builder := NewBuilder("retry_test", logger)
	assert.Equal(t, builder, builder.AddRetryStage("", 3, 10*time.Millisecond, nil, nil))
	assert.Equal(t, builder, builder.AddRetryStage("retry_nil", 3, 10*time.Millisecond, nil, nil))

	sentinelErr := errors.New("transient error")
	attempts := 0
	builder.AddRetryStage("retry_transient", 3, 5*time.Millisecond, sentinelErr, func(ctx *Context, payload any) (any, error) {
		attempts++
		if attempts < 2 {
			return nil, sentinelErr
		}
		return "recovered", nil
	})

	p := builder.Build()
	res, err := p.Run(nil, "init")
	require.NoError(t, err)
	assert.Equal(t, "recovered", res)
	assert.Equal(t, 2, attempts)
}

func TestCycle144_Pipeline_AddRetryStage_Failures(t *testing.T) {
	t.Parallel()

	logger := cycle144TestLogger()
	sentinelErr := errors.New("retriable error")
	diffErr := errors.New("permanent error")

	bNonMatching := NewBuilder("retry_fail", logger)
	bNonMatching.AddRetryStage("retry_mismatch", 3, 5*time.Millisecond, sentinelErr, func(ctx *Context, payload any) (any, error) {
		return nil, diffErr
	})
	pMismatch := bNonMatching.Build()
	_, err := pMismatch.Run(nil, "start")
	require.ErrorIs(t, err, diffErr)

	bExhausted := NewBuilder("retry_exhaust_flow", logger)
	exhaustCount := 0
	bExhausted.AddRetryStage("retry_exhaust", 2, 5*time.Millisecond, sentinelErr, func(ctx *Context, payload any) (any, error) {
		exhaustCount++
		return nil, sentinelErr
	})
	pExhausted := bExhausted.Build()
	_, err = pExhausted.Run(nil, "start")
	require.ErrorIs(t, err, sentinelErr)
	assert.Equal(t, 3, exhaustCount)
}

func TestCycle144_Pipeline_AddTPMStage(t *testing.T) {
	t.Parallel()

	logger := cycle144TestLogger()
	bEmpty := NewBuilder("tpm_flow", logger)
	assert.Equal(t, bEmpty, bEmpty.AddTPMStage("", nil, nil))

	limiter := circuitbreaker.NewTPMRateLimiter(10000)
	builder := NewBuilder("tpm_flow_valid", logger)
	assert.Equal(t, builder, builder.AddTPMStage("", limiter, nil))

	builder.AddTPMStage("tpm_default", limiter, nil)
	customEstimator := func(p any) int { return 5 }
	builder.AddTPMStage("tpm_custom", limiter, customEstimator)
	p := builder.Build()

	res, err := p.Run(nil, "payload")
	require.NoError(t, err)
	assert.Equal(t, "payload", res)

	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel()
	pctxCancelled := NewContext(ctxCancelled)
	_, err = p.Run(pctxCancelled, "payload")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

type cycle144IdleDetector struct{ isIdle bool }

func (m *cycle144IdleDetector) IsIdle(ctx context.Context, agentID string) (bool, error) {
	return m.isIdle, nil
}

func (m *cycle144IdleDetector) MarkActive(ctx context.Context, agentID string) error {
	return nil
}

type cycle144ChatResponder struct{ messages []string }

func (m *cycle144ChatResponder) Engage(ctx context.Context, agentID string, message string) error {
	m.messages = append(m.messages, message)
	return nil
}

func TestCycle144_Pipeline_AddIdlePreventionStage(t *testing.T) {
	t.Parallel()

	logger := cycle144TestLogger()
	builder := NewBuilder("idle_flow", logger)
	assert.Equal(t, builder, builder.AddIdlePreventionStage("", "agent-1", IdlePreventionOptions{}, nil))

	opts := IdlePreventionOptions{
		Detector:      &cycle144IdleDetector{isIdle: false},
		Responder:     &cycle144ChatResponder{},
		IdleThreshold: 10 * time.Millisecond,
		WakeupMessage: "wake up",
	}

	builder.AddIdlePreventionStage("idle_prevent", "agent-1", opts, func(ctx *Context, payload any) (any, error) {
		return "idle_ok", nil
	})

	p := builder.Build()
	res, err := p.Run(nil, "start")
	require.NoError(t, err)
	assert.Equal(t, "idle_ok", res)
}

func TestCycle144_Pipeline_Run_CancellationAndFailure(t *testing.T) {
	t.Parallel()

	logger := cycle144TestLogger()
	stepErr := errors.New("step execution failed")
	builderErr := NewBuilder("fail_flow", logger)
	builderErr.AddStage("failing_step", func(ctx *Context, payload any) (any, error) {
		return nil, stepErr
	})
	pErr := builderErr.Build()
	res, err := pErr.Run(nil, "data")
	assert.Nil(t, res)
	require.ErrorIs(t, err, stepErr)

	ctxCancel, cancel := context.WithCancel(context.Background())
	builderCancel := NewBuilder("cancel_flow", logger)
	builderCancel.AddStage("step_cancel", func(ctx *Context, payload any) (any, error) {
		cancel()
		return "passed_1", nil
	})
	builderCancel.AddStage("step_unreachable", func(ctx *Context, payload any) (any, error) {
		return "passed_2", nil
	})
	pCancel := builderCancel.Build()
	resCancel, errCancel := pCancel.Run(NewContext(ctxCancel), "initial")
	require.ErrorIs(t, errCancel, context.Canceled)
	assert.Equal(t, "passed_1", resCancel)
}

type cycle144ExecutionStore struct {
	data      map[string]map[string]any
	readErr   error
	updateErr error
}

func (m *cycle144ExecutionStore) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	obj, ok := m.data[id]
	if !ok {
		return nil, errors.New("object not found")
	}
	return obj, nil
}

func (m *cycle144ExecutionStore) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, data map[string]any) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.data[id] = data
	return nil
}

func TestCycle144_Orchestrator_Advance_Transitions(t *testing.T) {
	t.Parallel()

	stages := []struct {
		current string
		next    string
	}{
		{"pending", "pre_processing"},
		{"pre_processing", "agent_dispatch"},
		{"agent_dispatch", "agent_execution"},
		{"agent_execution", "handoff_verification"},
		{"handoff_verification", "post_processing"},
		{"post_processing", "success"},
		{"unknown_stage", "failure"},
	}

	for _, tc := range stages {
		store := &cycle144ExecutionStore{
			data: map[string]map[string]any{
				"exec-1": {
					objects.FieldKeyKind:         "pipeline_execution",
					objects.FieldKeyCurrentStage: tc.current,
				},
			},
		}
		orch := NewOrchestrator(store)
		err := orch.Advance(context.Background(), nil, "exec-1")
		require.NoError(t, err)
		assert.Equal(t, tc.next, store.data["exec-1"][objects.FieldKeyCurrentStage])
	}
}

func TestCycle144_Orchestrator_Advance_EdgeCases(t *testing.T) {
	t.Parallel()

	terminalStages := []string{"success", "failure"}
	for _, term := range terminalStages {
		store := &cycle144ExecutionStore{
			data: map[string]map[string]any{
				"exec-term": {
					objects.FieldKeyKind:         "pipeline_execution",
					objects.FieldKeyCurrentStage: term,
				},
			},
		}
		orch := NewOrchestrator(store)
		assert.NoError(t, orch.Advance(context.Background(), nil, "exec-term"))
	}

	storeReadErr := &cycle144ExecutionStore{readErr: errors.New("storage offline")}
	orchReadErr := NewOrchestrator(storeReadErr)
	require.Error(t, orchReadErr.Advance(context.Background(), nil, "exec-x"))

	storeWrongKind := &cycle144ExecutionStore{
		data: map[string]map[string]any{
			"exec-wrong": {objects.FieldKeyKind: "task"},
		},
	}
	orchWrongKind := NewOrchestrator(storeWrongKind)
	require.Error(t, orchWrongKind.Advance(context.Background(), nil, "exec-wrong"))

	storeUpdateErr := &cycle144ExecutionStore{
		data: map[string]map[string]any{
			"exec-up": {
				objects.FieldKeyKind:         "pipeline_execution",
				objects.FieldKeyCurrentStage: "pending",
			},
		},
		updateErr: errors.New("update write failed"),
	}
	orchUpdateErr := NewOrchestrator(storeUpdateErr)
	require.Error(t, orchUpdateErr.Advance(context.Background(), nil, "exec-up"))
}

func TestCycle144_OutcomeKeysGenerate_LoadFromYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	_, err := LoadOutcomeKeyNamesFromYAML(filepath.Join(tmpDir, "missing.yaml"))
	require.Error(t, err)

	corruptPath := filepath.Join(tmpDir, "corrupt.yaml")
	require.NoError(t, fileutil.WriteSecureFile(corruptPath, []byte(":: invalid yaml ::")))
	_, err = LoadOutcomeKeyNamesFromYAML(corruptPath)
	require.Error(t, err)

	badVersionPath := filepath.Join(tmpDir, "bad_version.yaml")
	require.NoError(t, fileutil.WriteSecureFile(badVersionPath, []byte("schema_version: 2\nkeys:\n  - key_a\n")))
	_, err = LoadOutcomeKeyNamesFromYAML(badVersionPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported schema_version")

	emptyKeysPath := filepath.Join(tmpDir, "empty_keys.yaml")
	require.NoError(t, fileutil.WriteSecureFile(emptyKeysPath, []byte("schema_version: 1\nkeys: []\n")))
	_, err = LoadOutcomeKeyNamesFromYAML(emptyKeysPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keys list is empty")

	blankKeyPath := filepath.Join(tmpDir, "blank_key.yaml")
	require.NoError(t, fileutil.WriteSecureFile(blankKeyPath, []byte("schema_version: 1\nkeys:\n  - '   '\n")))
	_, err = LoadOutcomeKeyNamesFromYAML(blankKeyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty key entry")

	validPath := filepath.Join(tmpDir, "valid.yaml")
	require.NoError(t, fileutil.WriteSecureFile(validPath, []byte("schema_version: 1\nkeys:\n  - zebra_key\n  - apple_key\n")))
	keys, err := LoadOutcomeKeyNamesFromYAML(validPath)
	require.NoError(t, err)
	assert.Equal(t, []string{"apple_key", "zebra_key"}, keys)
}

func TestCycle144_OutcomeKeysGenerate_GenerateGoSource(t *testing.T) {
	t.Parallel()

	_, err := GenerateOutcomeKeysGoSource("registry.yaml", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no outcome key names")

	dupKeys := []string{"output_metric", "output_metric"}
	_, err = GenerateOutcomeKeysGoSource("registry.yaml", dupKeys)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate generated const")

	validKeys := []string{"summary", "artifacts"}
	src, err := GenerateOutcomeKeysGoSource("registry.yaml", validKeys)
	require.NoError(t, err)
	assert.Contains(t, string(src), "OutcomeKeySummary")
	assert.Contains(t, string(src), "OutcomeKeyArtifacts")
}

type cycle144SwarmEngine struct {
	enqueued []any
	err      error
}

func (m *cycle144SwarmEngine) Enqueue(ctx context.Context, task any) error {
	if m.err != nil {
		return m.err
	}
	m.enqueued = append(m.enqueued, task)
	return nil
}

func TestCycle144_NativeDispatcher_Dispatch(t *testing.T) {
	t.Parallel()

	engine := &cycle144SwarmEngine{}
	dispatcher := NewNativeSwarmDispatcher(nil, engine)

	_, err := dispatcher.Dispatch(context.Background(), "invalid-payload-type")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payload must be a map[string]any")

	payloadNoID := map[string]any{
		"system_prompt": "sys instructions",
		"user_prompt":   "user query",
	}
	res, err := dispatcher.Dispatch(context.Background(), payloadNoID)
	require.NoError(t, err)
	resMap, ok := res.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "default-task", resMap["task_id"])
	require.Len(t, engine.enqueued, 1)
	st, ok := engine.enqueued[0].(SwarmTask)
	require.True(t, ok)
	assert.Equal(t, "default-task", st.ID)

	engine.err = errors.New("queue full")
	_, err = dispatcher.Dispatch(context.Background(), payloadNoID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to enqueue swarm task")
}

type cycle144PipelineStorage struct {
	objects map[string]map[string]any
	readErr error
}

func (m *cycle144PipelineStorage) Read(ctx context.Context, secCtx any, id string) (map[string]any, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	obj, ok := m.objects[id]
	if !ok {
		return nil, nil
	}
	return obj, nil
}

func TestCycle144_Loader_LoadPipeline(t *testing.T) {
	t.Parallel()

	logger := cycle144TestLogger()
	_, err := LoadPipeline(context.Background(), nil, nil, logger, "pip-1")
	require.Error(t, err)

	store := &cycle144PipelineStorage{objects: make(map[string]map[string]any)}
	_, err = LoadPipeline(context.Background(), store, nil, logger, "")
	require.Error(t, err)

	store.readErr = errors.New("read failed")
	_, err = LoadPipeline(context.Background(), store, nil, logger, "pip-err")
	require.Error(t, err)

	store.readErr = nil
	_, err = LoadPipeline(context.Background(), store, nil, logger, "pip-not-found")
	require.Error(t, err)

	store.objects["pip-wrong-kind"] = map[string]any{
		objects.FieldKeyKind: "task",
	}
	_, err = LoadPipeline(context.Background(), store, nil, logger, "pip-wrong-kind")
	require.Error(t, err)

	store.objects["pip-valid"] = map[string]any{
		objects.FieldKeyKind:   objects.KindPipeline,
		objects.FieldKeyStages: []any{"stage-1", 12345, "stage-2"},
	}
	store.objects["stage-1"] = map[string]any{
		objects.FieldKeyTitle:         "Sequential Step",
		objects.FieldKeyExecutionMode: "sequential",
	}
	store.objects["stage-2"] = map[string]any{
		objects.FieldKeyTitle:         "",
		objects.FieldKeyExecutionMode: "parallel",
	}

	builder, err := LoadPipeline(context.Background(), store, nil, logger, "pip-valid")
	require.NoError(t, err)
	require.NotNil(t, builder)
	p := builder.Build()
	res, err := p.Run(nil, "sample_input")
	require.NoError(t, err)
	assert.Equal(t, "sample_input", res)
}
