package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestDecodeNonNilPayload(t *testing.T) {
	val, ok := DecodeNonNilPayload[string]("hello")
	if !ok || val != "hello" {
		t.Fatalf("expected ('hello', true), got (%q, %v)", val, ok)
	}

	_, ok = DecodeNonNilPayload[string](nil)
	if ok {
		t.Fatal("expected false for nil payload")
	}

	_, ok = DecodeNonNilPayload[string](123)
	if ok {
		t.Fatal("expected false for type mismatch")
	}
}

func TestTaskSession_State(t *testing.T) {
	session := &TaskSession{}
	session.SetState("step", "init")

	val, ok := session.GetState("step")
	if !ok || val != "init" {
		t.Fatalf("expected ('init', true), got (%v, %v)", val, ok)
	}

	_, ok = session.GetState("missing")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestLoggerMetricsSink_RecordStage(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sink := LoggerMetricsSink{logger: logger}

	ctx := context.Background()
	sink.RecordStage(ctx, "pipeline", "stage_init", 10*time.Millisecond, nil)
	sink.RecordStageWithBuckets(ctx, "pipeline", "stage_exec", 20*time.Millisecond, errors.New("fail"), map[string]string{"env": "test"})

	// Nil logger should no-op safely
	nilSink := LoggerMetricsSink{}
	nilSink.RecordStage(ctx, "pipeline", "stage_init", time.Millisecond, nil)
	nilSink.RecordStageWithBuckets(ctx, "pipeline", "stage_init", time.Millisecond, nil, nil)
}

func TestBuilder_WithProfile_WithMetrics(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	b := NewBuilder("test-pipeline", logger)
	b.WithProfile("system")
	if b.profile != "system" {
		t.Errorf("expected profile 'system', got %q", b.profile)
	}

	b.WithMetrics(nil)
	if b.metricsConfig != nil {
		t.Error("expected metricsConfig to remain nil when passing nil sink")
	}

	sink := LoggerMetricsSink{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))}
	b.WithMetrics(sink)
	if b.metricsConfig == nil || b.metricsConfig.Sink == nil {
		t.Error("expected metricsConfig to be populated")
	}
}

type mockPipelineStorage struct {
	pipeline map[string]any
	stage    map[string]any
	readErr  error
}

func (m *mockPipelineStorage) Read(ctx context.Context, secCtx any, id string) (map[string]any, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	if id == "PIP-TEST" {
		return m.pipeline, nil
	}
	if id == "STG-1" {
		return m.stage, nil
	}
	return nil, nil
}

func TestLoadPipeline(t *testing.T) {
	ctx := context.Background()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Nil storage
	if _, err := LoadPipeline(ctx, nil, nil, logger, "PIP-TEST"); err == nil {
		t.Fatal("expected error for nil storage")
	}

	// Empty ID
	sp := &mockPipelineStorage{}
	if _, err := LoadPipeline(ctx, sp, nil, logger, ""); err == nil {
		t.Fatal("expected error for empty ID")
	}

	// Read error
	failSp := &mockPipelineStorage{readErr: errors.New("read failed")}
	if _, err := LoadPipeline(ctx, failSp, nil, logger, "PIP-TEST"); err == nil {
		t.Fatal("expected error on read failure")
	}

	// Not a pipeline
	wrongKindSp := &mockPipelineStorage{
		pipeline: map[string]any{
			objects.FieldKeyKind: "task",
		},
	}
	if _, err := LoadPipeline(ctx, wrongKindSp, nil, logger, "PIP-TEST"); err == nil {
		t.Fatal("expected error for wrong kind")
	}

	// Valid pipeline with stages
	validSp := &mockPipelineStorage{
		pipeline: map[string]any{
			objects.FieldKeyKind:   objects.KindPipeline,
			objects.FieldKeyStages: []any{"STG-1"},
		},
		stage: map[string]any{
			objects.FieldKeyKind:          "pipeline_stage",
			objects.FieldKeyTitle:         "Build Stage",
			objects.FieldKeyExecutionMode: "sequential",
		},
	}
	builder, err := LoadPipeline(ctx, validSp, nil, logger, "PIP-TEST")
	if err != nil {
		t.Fatalf("unexpected error loading pipeline: %v", err)
	}
	if builder == nil {
		t.Fatal("expected non-nil builder")
	}
}

type mockOrchStore struct {
	objects map[string]map[string]any
}

func (m *mockOrchStore) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, errors.New("not found")
}

func (m *mockOrchStore) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if obj, ok := m.objects[id]; ok {
		for k, v := range updates {
			obj[k] = v
		}
		return nil
	}
	return errors.New("not found")
}

type dummyPlugin struct {
	name string
}

func (d *dummyPlugin) Name() string { return d.name }
func (d *dummyPlugin) Execute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	payload["processed"] = true
	return payload, nil
}
func (d *dummyPlugin) Validate(ctx context.Context, output map[string]any) error {
	return nil
}

func TestOrchestrator_Advance(t *testing.T) {
	store := &mockOrchStore{
		objects: map[string]map[string]any{
			"EXEC-1": {
				objects.FieldKeyKind:         "pipeline_execution",
				objects.FieldKeyCurrentStage: "pending",
			},
		},
	}

	orch := NewOrchestrator(store)
	plugin := &dummyPlugin{name: "preprocessing"}
	orch.RegisterPlugin(plugin)

	secCtx := pkgctx.NewSystemSecurityContext()
	if err := orch.Advance(context.Background(), secCtx, "EXEC-1"); err != nil {
		t.Fatalf("Advance failed: %v", err)
	}

	// Verify advanced to pre_processing
	execObj := store.objects["EXEC-1"]
	if execObj[objects.FieldKeyCurrentStage] != "pre_processing" {
		t.Fatalf("expected stage pre_processing, got %v", execObj[objects.FieldKeyCurrentStage])
	}
}
