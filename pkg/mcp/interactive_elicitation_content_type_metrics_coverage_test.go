package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/interactive"
)

// 1. Interactive elicitation conversions
func TestDeep12_InteractiveElicitation_Comprehensive(t *testing.T) {
	tokenWithEnum := &interactive.FieldTokenInfo{
		Name:        "format",
		Description: "Output format",
		Type:        "string",
		Required:    true,
		EnumValues:  []string{"json", "yaml", "table"},
	}
	p1 := ConvertFieldTokenInfoToElicitationParam(tokenWithEnum)
	if p1.Name != "format" || len(p1.Choices) != 3 {
		t.Errorf("unexpected elicitation param with enum: %v", p1)
	}

	tokenWithoutEnum := &interactive.FieldTokenInfo{
		Name:        "count",
		Description: "Count limit",
		Type:        "int",
		Required:    false,
	}
	p2 := ConvertFieldTokenInfoToElicitationParam(tokenWithoutEnum)
	if p2.Name != "count" || len(p2.Choices) != 0 {
		t.Errorf("unexpected elicitation param without enum: %v", p2)
	}

	params := ConvertFieldTokenInfosToElicitationParams([]*interactive.FieldTokenInfo{tokenWithEnum, tokenWithoutEnum})
	if len(params) != 2 {
		t.Errorf("expected 2 params, got: %d", len(params))
	}

	for _, ft := range []string{"string", "int", "boolean", "array", "unknown"} {
		_ = mapFieldTypeToElicitationType(ft)
	}
}

// 2. Content type helpers and tool call content building
func TestDeep12_ContentTypeHelpers_DetermineContentType(t *testing.T) {
	s := NewServer()

	// Default JSON format
	cType, mType := s.determineContentType(nil, "")
	if cType != "text" || mType != "application/json; charset=utf-8" {
		t.Errorf("unexpected default content type: %s, %s", cType, mType)
	}

	// Format hint from config default
	s.config = &ServerConfig{}
	s.config.MCPServer.Security.DefaultFormat = "yaml"
	cType, mType = s.determineContentType(nil, "")
	if mType != "application/yaml; charset=utf-8" {
		t.Errorf("unexpected config default format: %s", mType)
	}

	// Specific formats
	for _, f := range []string{"yaml", "table", "markdown", "csv", "unknown_fmt"} {
		_, _ = s.determineContentType(nil, f)
	}

	// Allowed formats restrictions
	s.SetAllowedFormats([]string{"yaml"})
	_, mTypeAllowed := s.determineContentType(nil, "json")
	if mTypeAllowed != "application/yaml; charset=utf-8" {
		t.Errorf("expected fallback to allowed format yaml, got: %s", mTypeAllowed)
	}

	// buildToolCallContent
	contents := s.buildToolCallContent("hello result", "yaml")
	if len(contents) != 1 || contents[0].Text != "hello result" {
		t.Errorf("unexpected tool call content: %v", contents)
	}
}

// 3. ClientMetrics anomaly detection and sequence retrieval
func TestDeep12_ClientMetrics_DetectAnomalies(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewClientMetricsStore(tmpDir+"/metrics.json", context.Background())
	if err != nil {
		t.Fatalf("failed to create metrics store: %v", err)
	}

	metrics := &ClientSequenceMetrics{
		SequenceID:  "seq-test",
		Initialized: false,
		Events:      make([]ClientSequenceEvent, 0),
	}

	// tools_list before initialize anomaly
	store.detectAnomalies(metrics, ClientSequenceEvent{
		EventType: "tools_list",
	})
	if len(metrics.Anomalies) != 1 || metrics.Anomalies[0] != "tools_list_before_initialize" {
		t.Errorf("expected tools_list_before_initialize anomaly: %v", metrics.Anomalies)
	}

	// duplicate anomaly not re-added
	store.detectAnomalies(metrics, ClientSequenceEvent{
		EventType: "tools_list",
	})
	if len(metrics.Anomalies) != 1 {
		t.Errorf("expected duplicate anomaly skipped: %v", metrics.Anomalies)
	}

	// Long gap anomaly
	metrics.Events = append(metrics.Events, ClientSequenceEvent{EventType: "e1"}, ClientSequenceEvent{EventType: "e2"})
	store.detectAnomalies(metrics, ClientSequenceEvent{
		EventType: "e3",
		Duration:  10 * time.Minute,
	})
	if len(metrics.Anomalies) != 2 {
		t.Errorf("expected long gap anomaly added: %v", metrics.Anomalies)
	}

	// GetSequenceMetrics
	store.metrics["seq-test"] = metrics
	seqGot, err := store.GetSequenceMetrics("seq-test")
	if err != nil || seqGot == nil {
		t.Errorf("expected found sequence: %v, %v", seqGot, err)
	}
	_, errNotFound := store.GetSequenceMetrics("nonexistent-seq")
	if errNotFound == nil {
		t.Error("expected error for nonexistent sequence")
	}
}

// 4. Context and Actor extraction helpers
func TestDeep12_ContextAndActor_Extract(t *testing.T) {
	// EnsureContext
	if EnsureContext(nil) == nil {
		t.Error("expected non-nil context from EnsureContext(nil)")
	}
	ctx := context.Background()
	if EnsureContext(ctx) != ctx {
		t.Error("expected same context from EnsureContext")
	}

	// ExtractActorContext
	if ExtractActorContext(ctx, nil) != ctx {
		t.Error("expected same ctx for nil params")
	}
	if ExtractActorContext(ctx, []byte(`invalid json`)) != ctx {
		t.Error("expected same ctx for invalid json")
	}
	if ExtractActorContextFromArgs(ctx, nil) != ctx {
		t.Error("expected same ctx for nil args")
	}
	if ExtractActorContextFromArgs(ctx, map[string]any{"other": 1}) != ctx {
		t.Error("expected same ctx for missing _meta")
	}
	if ExtractActorContextFromArgs(ctx, map[string]any{"_meta": "not a map"}) != ctx {
		t.Error("expected same ctx for non-map _meta")
	}

	metaValid := map[string]any{
		"_meta": map[string]any{
			"account_id": "ACC-TESTER",
			"roles":      []any{"developer"},
		},
	}
	ctxWithActor := ExtractActorContextFromArgs(ctx, metaValid)
	if ctxWithActor == ctx {
		t.Error("expected new context with security context")
	}
}

// 5. EventEmitter, Adapter, stats and cleanup
func TestDeep12_EventEmitter_AdapterAndStats(t *testing.T) {
	ee0 := NewEventEmitter(0)
	if ee0.GetBufferSize() <= 0 {
		t.Error("expected default buffer size for 0")
	}

	ee := NewEventEmitter(20)
	if ee.GetBufferSize() != 20 {
		t.Errorf("expected buffer size 20, got: %d", ee.GetBufferSize())
	}

	emitted, subs := ee.GetEventEmitterStats()
	if emitted != 0 || subs != 0 {
		t.Errorf("expected 0 stats, got: %d, %d", emitted, subs)
	}

	// Nil adapter
	if NewEventEmitterAdapter(nil) != nil {
		t.Error("expected nil adapter for nil emitter")
	}

	adapter := NewEventEmitterAdapter(ee)
	if adapter.Emit(nil) != 0 {
		t.Error("expected 0 emitted for nil")
	}

	// Emit typed event
	_ = adapter.Emit(&Event{
		Type:     EventTypeLogInfo,
		Message:  "adapter typed event",
		Severity: "info",
	})

	// Emit map event
	_ = adapter.Emit(map[string]any{
		"type":      "log.warn",
		"message":   "adapter map event",
		"timestamp": time.Now().Format(time.RFC3339),
		"fields":    map[string]any{"k": "v"},
	})

	ee.Cleanup()
}

// 6. Brand prefix and executable naming
func TestDeep12_BrandPrefix_Naming(t *testing.T) {
	_ = GetExecutableName()
	_ = stripBrandSuffixes("zqk-stable")
	_ = stripBrandSuffixes("custom-binary")
	_ = GetCommandPath("object list")
	_ = NormalizeCommandPath("zqk object get")
}
