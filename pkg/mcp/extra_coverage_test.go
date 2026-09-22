package mcp

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestAsyncHelpers_Comprehensive(t *testing.T) {
	// ParallelExecutor
	pe := NewParallelExecutor()
	pe.Execute(func() error {
		return nil
	})
	pe.Execute(func() error {
		return errors.New("err 1")
	})
	pe.Execute(func() error {
		panic("test panic")
	})

	err := pe.Wait()
	if err == nil {
		t.Error("expected error from ParallelExecutor")
	}
	errs := pe.AllErrors()
	if len(errs) < 1 {
		t.Errorf("expected at least 1 error, got %d", len(errs))
	}

	// ParallelExecutorWithContext
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pec := NewParallelExecutorWithContext(ctx)
	pec.Execute(func(c context.Context) error {
		return nil
	})
	pec.Execute(func(c context.Context) error {
		return errors.New("ctx err")
	})

	ctxErr := pec.Wait()
	if ctxErr == nil {
		t.Error("expected error from ParallelExecutorWithContext")
	}
	ctxErrs := pec.AllErrors()
	if len(ctxErrs) < 1 {
		t.Errorf("expected at least 1 error in ParallelExecutorWithContext, got %d", len(ctxErrs))
	}

	// ExecuteAsync
	ch := ExecuteAsync(func() (string, error) {
		return "hello", nil
	})
	res := <-ch
	if res.Value != "hello" || res.Error != nil {
		t.Errorf("ExecuteAsync failed: %v", res)
	}

	// ExecuteAsyncWithContext
	chCtx := ExecuteAsyncWithContext(ctx, func(c context.Context) (int, error) {
		return 42, nil
	})
	resCtx := <-chCtx
	if resCtx.Value != 42 || resCtx.Error != nil {
		t.Errorf("ExecuteAsyncWithContext failed: %v", resCtx)
	}
}

func TestUnixPermissions_OctalAndCheck(t *testing.T) {
	p := ReadWriteExec
	if p.Octal() != 7 {
		t.Errorf("expected octal 7, got %d", p.Octal())
	}
	if !p.CheckPermission("read") || !p.CheckPermission("write") || !p.CheckPermission("execute") || !p.CheckPermission("exec") {
		t.Error("expected all permissions to pass")
	}
	if p.CheckPermission("unknown") {
		t.Error("expected unknown permission to fail")
	}
}

func TestUtilsJsonLD_Comprehensive(t *testing.T) {
	if MapFlagTypeToJSONLDType("bool") != "xsd:boolean" {
		t.Errorf("expected xsd:boolean, got %s", MapFlagTypeToJSONLDType("bool"))
	}
	if MapFlagTypeToJSONLDType("int") != "xsd:integer" {
		t.Errorf("expected xsd:integer, got %s", MapFlagTypeToJSONLDType("int"))
	}
	if MapFlagTypeToJSONLDType("float64") != "xsd:double" {
		t.Errorf("expected xsd:double, got %s", MapFlagTypeToJSONLDType("float64"))
	}
	if MapFlagTypeToJSONLDType("stringSlice") != "xsd:array" {
		t.Errorf("expected xsd:array, got %s", MapFlagTypeToJSONLDType("stringSlice"))
	}
	if MapFlagTypeToJSONLDType("other") != "xsd:string" {
		t.Errorf("expected xsd:string, got %s", MapFlagTypeToJSONLDType("other"))
	}

	// ParseDefaultValue
	if ParseDefaultValue("true", "bool") != true {
		t.Error("expected bool true")
	}
	if ParseDefaultValue("false", "bool") != false {
		t.Error("expected bool false")
	}
	if ParseDefaultValue("10", "int") != "10" {
		t.Error("expected string 10")
	}
	if ParseDefaultValue("3.14", "float64") != "3.14" {
		t.Error("expected string 3.14")
	}
	if ParseDefaultValue("default", "string") != "default" {
		t.Error("expected default")
	}

	// MapFieldTypeToXSD
	if MapFieldTypeToXSD("string") != "xsd:string" {
		t.Error("expected xsd:string")
	}
	if MapFieldTypeToXSD("integer") != "xsd:integer" {
		t.Error("expected xsd:integer")
	}
	if MapFieldTypeToXSD("float") != "xsd:double" {
		t.Error("expected xsd:double")
	}
	if MapFieldTypeToXSD("boolean") != "xsd:boolean" {
		t.Error("expected xsd:boolean")
	}
	if MapFieldTypeToXSD("list") != "xsd:array" {
		t.Error("expected xsd:array")
	}
	if MapFieldTypeToXSD("object") != "xsd:object" {
		t.Error("expected xsd:object")
	}
	if MapFieldTypeToXSD("enum") != "zqk:Enum" {
		t.Error("expected zqk:Enum")
	}
	if MapFieldTypeToXSD("reference") != "zqk:Reference" {
		t.Error("expected zqk:Reference")
	}
	if MapFieldTypeToXSD("custom") != "xsd:string" {
		t.Error("expected fallback xsd:string")
	}

	// MapFieldTypeToElicitationType
	if MapFieldTypeToElicitationType("string") != "string" {
		t.Error("expected string")
	}
	if MapFieldTypeToElicitationType("int") != "number" {
		t.Error("expected number")
	}
	if MapFieldTypeToElicitationType("float") != "number" {
		t.Error("expected number")
	}
	if MapFieldTypeToElicitationType("bool") != "boolean" {
		t.Error("expected boolean")
	}
}

func TestUtilsPath_ExtractGroupFromPath(t *testing.T) {
	if ExtractGroupFromPath("object list") != "object" {
		t.Errorf("expected object, got %s", ExtractGroupFromPath("object list"))
	}
	if ExtractGroupFromPath("single") != "" {
		t.Errorf("expected empty string, got %s", ExtractGroupFromPath("single"))
	}
	if ExtractGroupFromPath("") != "" {
		t.Errorf("expected empty string, got %s", ExtractGroupFromPath(""))
	}
}

func TestUtilsOutput_ParseCommandOutput(t *testing.T) {
	// Valid json in output
	res1 := ParseCommandOutput(`{"status": "ok"}`, bytes.Buffer{})
	if res1["status"] != "ok" {
		t.Errorf("expected status ok, got %v", res1)
	}

	// Empty output, fallback to stderr json
	var stderrBuf bytes.Buffer
	stderrBuf.WriteString(`{"stderr_key": "val"}`)
	res2 := ParseCommandOutput("", stderrBuf)
	if res2["stderr_key"] != "val" {
		t.Errorf("expected stderr_key val, got %v", res2)
	}

	// Empty output, empty stderr
	res3 := ParseCommandOutput("", bytes.Buffer{})
	if res3["success"] != false || res3["error_type"] != "empty_output" {
		t.Errorf("expected empty_output error, got %v", res3)
	}

	// Invalid JSON
	res4 := ParseCommandOutput("not valid json", bytes.Buffer{})
	if res4["success"] != false || res4["error_type"] != "parse_error" {
		t.Errorf("expected parse_error error, got %v", res4)
	}
}

func TestServer_MiscMethods(t *testing.T) {
	server := NewServer()

	// Diagnostics & stats
	server.SetMCPSpecDiagnostics("kernel_storage", 5, 2)
	r, s, sh := server.GetServerStats()
	if r != 0 || s != 0 || sh != 0 {
		t.Errorf("expected 0 stats, got %d, %d, %d", r, s, sh)
	}

	// Format & project root
	server.SetAllowedFormats([]string{"json", "yaml"})
	server.SetProjectRoot("/tmp/project")
	if server.GetProjectRoot() != "/tmp/project" {
		t.Errorf("expected /tmp/project, got %s", server.GetProjectRoot())
	}

	// Connected clients
	clients := server.GetConnectedClients()
	if len(clients) != 0 {
		t.Errorf("expected 0 connected clients, got %d", len(clients))
	}

	// SendNotificationToClient when not connected
	err := server.SendNotificationToClient("nonexistent", "notify", map[string]any{})
	if err == nil {
		t.Error("expected error for nonexistent client notification")
	}

	// Operation tracker count
	tracker := server.operationTracker
	if tracker.GetOperationCount() != 0 {
		t.Errorf("expected 0 operations, got %d", tracker.GetOperationCount())
	}
}

type mockCoordinator struct {
	emitted []any
}

func (m *mockCoordinator) Emit(ctx context.Context, eventCtx any) error {
	m.emitted = append(m.emitted, eventCtx)
	return nil
}

func TestMCPServerAdapter_Comprehensive(t *testing.T) {
	server := NewServer()
	server.SetProjectRoot(t.TempDir())
	RegisterEchoTool(server)

	coord := &mockCoordinator{}
	adapter := NewMCPServerAdapterWithCoordinator(server, coord)
	if adapter.coordinator == nil {
		t.Fatal("expected coordinator to be set")
	}
	adapter.SetCoordinator(coord)

	ctx := pkgctx.NewSystemContext()

	// Initialize
	initParams := &InitializeParams{
		ProtocolVersion: "2025-06-18",
		Capabilities: map[string]any{
			objects.FieldKeyClientID: "test-client-123",
			clientInfoAccountID:      "ACC-TEST-AGENT",
			objects.FieldKeyRoles:    []any{"test_agent"},
		},
	}
	initParams.ClientInfo.Name = "test-client"
	initParams.ClientInfo.Version = "1.0"
	initRes, err := adapter.Initialize(ctx, initParams)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if initRes == nil {
		t.Fatal("expected InitializeResult")
	}

	// NotifyInitialized
	err = adapter.NotifyInitialized(ctx, &InitializedParams{})
	if err != nil {
		t.Fatalf("NotifyInitialized failed: %v", err)
	}

	// ListTools
	toolsRes, err := adapter.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if toolsRes == nil || len(toolsRes.Tools) == 0 {
		t.Error("expected at least one tool registered")
	}

	// CallTool
	toolCallRes, err := adapter.CallTool(ctx, &ToolCallParams{
		Name:      "test_echo",
		Arguments: map[string]any{"message": "ping"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if toolCallRes == nil {
		t.Fatal("expected ToolCallResult")
	}

	// ListResources
	resList, err := adapter.ListResources(ctx, &ResourcesListParams{})
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if resList == nil {
		t.Fatal("expected ResourcesListResult")
	}

	// GetResource
	_, _ = adapter.GetResource(ctx, &ResourceGetParams{URI: "schema://registry"})

	// ListPrompts
	promptsList, err := adapter.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts failed: %v", err)
	}
	if promptsList == nil {
		t.Fatal("expected PromptsListResult")
	}

	// GetPrompt
	_, _ = adapter.GetPrompt(ctx, &PromptGetParams{Name: "getting_started"})

	// Roots, Logs, Events
	_, _ = adapter.ListRoots(ctx)
	_ = adapter.SendLogMessage(ctx, LogLevelInfo, "test log", nil)
	_ = adapter.SendEvent(ctx, &Event{Type: EventTypeToolStarted})
	_ = adapter.SendMessage(ctx, "msg", "type", "prio")
	_ = adapter.NotifyCancelled(ctx, &CancelledParams{})

	// Metrics snapshot
	_ = adapter.GetMetricsSnapshot()

	// Async calls
	chTools := adapter.ListToolsAsync(ctx)
	rTools := <-chTools
	if rTools.Error != nil {
		t.Errorf("ListToolsAsync error: %v", rTools.Error)
	}

	chCall := adapter.CallToolAsync(ctx, &ToolCallParams{
		Name:      "test_echo",
		Arguments: map[string]any{"message": "pong"},
	})
	rCall := <-chCall
	if rCall.Error != nil {
		t.Errorf("CallToolAsync error: %v", rCall.Error)
	}

	chRes := adapter.ListResourcesAsync(ctx, &ResourcesListParams{})
	rRes := <-chRes
	if rRes.Error != nil {
		t.Errorf("ListResourcesAsync error: %v", rRes.Error)
	}

	chGetRes := adapter.GetResourceAsync(ctx, &ResourceGetParams{URI: "schema://registry"})
	<-chGetRes

	chPrompts := adapter.ListPromptsAsync(ctx)
	rPrompts := <-chPrompts
	if rPrompts.Error != nil {
		t.Errorf("ListPromptsAsync error: %v", rPrompts.Error)
	}

	chGetPrompt := adapter.GetPromptAsync(ctx, &PromptGetParams{Name: "getting_started"})
	<-chGetPrompt

	// Batch calls
	chBatchTools := adapter.CallToolsBatch(ctx, []*ToolCallParams{
		{Name: "test_echo", Arguments: map[string]any{"message": "batch"}},
	})
	<-chBatchTools

	chBatchRes := adapter.GetResourcesBatch(ctx, []*ResourceGetParams{
		{URI: "schema://registry"},
	})
	<-chBatchRes

	// Shutdown
	err = adapter.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// NewMCPServerAdapter without coordinator
	adapter2 := NewMCPServerAdapter(server)
	if adapter2 == nil {
		t.Fatal("expected adapter2")
	}
}

func TestGraphConnectionManager_MockOperations(t *testing.T) {
	ctx := context.Background()
	mockProvider := provider.NewMockGraphProvider()
	pool, err := mockProvider.CreatePool(ctx, provider.ConnectionConfig{})
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}

	mgr := &GraphConnectionManager{
		enabled: true,
		pool:    pool,
	}
	_ = mgr.IsEnabled()

	// Populate mock nodes and edges via pool.Execute
	_ = pool.Execute(ctx, func(conn provider.GraphConnection) error {
		_ = conn.CreateNode(ctx, provider.Node{
			ID:     "task-1",
			Labels: []string{"task"},
			Properties: map[string]any{
				objects.FieldKeyID:     "task-1",
				objects.FieldKeyStatus: "in_progress",
				objects.FieldKeyKind:   "task",
			},
		})
		_ = conn.CreateNode(ctx, provider.Node{
			ID:     "task-2",
			Labels: []string{"task"},
			Properties: map[string]any{
				objects.FieldKeyID:     "task-2",
				objects.FieldKeyStatus: objects.ObjectStatusBlocked,
				objects.FieldKeyKind:   "task",
			},
		})
		_ = conn.CreateNode(ctx, provider.Node{
			ID:     "task-3",
			Labels: []string{"task"},
			Properties: map[string]any{
				objects.FieldKeyID:     "task-3",
				objects.FieldKeyStatus: "complete",
				objects.FieldKeyKind:   "task",
			},
		})
		_ = conn.CreateEdge(ctx, provider.Edge{
			Type:   "DEPENDS_ON",
			FromID: "task-2",
			ToID:   "task-1",
		})
		_ = conn.CreateEdge(ctx, provider.Edge{
			Type:   "BLOCKS",
			FromID: "task-1",
			ToID:   "task-2",
		})
		return nil
	})

	// ExecuteTraversal
	_, _, err = mgr.ExecuteTraversal(ctx, "task-1", "DEPENDS_ON", "outgoing", 2, nil, nil, 10)
	if err != nil {
		t.Logf("ExecuteTraversal outgoing: %v", err)
	}
	_, _, err = mgr.ExecuteTraversal(ctx, "task-1", "DEPENDS_ON", "incoming", 2, nil, nil, 10)
	if err != nil {
		t.Logf("ExecuteTraversal incoming: %v", err)
	}
	_, _, err = mgr.ExecuteTraversal(ctx, "task-1", "DEPENDS_ON", "both", 2, nil, nil, 10)
	if err != nil {
		t.Logf("ExecuteTraversal both: %v", err)
	}

	// ResolveReferences
	resolved, unresolved, err := mgr.ResolveReferences(ctx, []string{"task:task-1", "task-2", "nonexistent-ref"}, true, 2, 5)
	if err != nil {
		t.Logf("ResolveReferences error: %v", err)
	}
	if len(resolved) == 0 && len(unresolved) == 0 {
		t.Error("expected some resolved or unresolved references")
	}

	// QueryStateAware
	_, _, _ = mgr.QueryStateAware(ctx, "active_items", nil, true)
	_, _, _ = mgr.QueryStateAware(ctx, "blocked_items", nil, true)
	_, _, _ = mgr.QueryStateAware(ctx, "dependencies", map[string]any{objects.FieldKeyID: "task-2"}, true)
	_, _, _ = mgr.QueryStateAware(ctx, "dependencies", nil, true)
	_, _, _ = mgr.QueryStateAware(ctx, "progress", nil, true)
	_, _, _ = mgr.QueryStateAware(ctx, "unknown_type", nil, false)

	// Direct helper checks
	_ = pool.Execute(ctx, func(conn provider.GraphConnection) error {
		_, _ = calculateProgressForNode(ctx, conn, "task-1")
		_, _ = findRelatedNodesWithDepth(ctx, conn, "task-1", 2, 5)
		return nil
	})

	nMap := nodeToMap(&provider.Node{ID: "n1", Properties: map[string]any{"p": "v"}})
	if nMap["id"] != "n1" {
		t.Errorf("unexpected nodeToMap result: %v", nMap)
	}
	eMap := edgeToMap(&provider.Edge{FromID: "n1", ToID: "n2", Type: "REL"})
	if eMap["from_id"] != "n1" || eMap["to_id"] != "n2" {
		t.Errorf("unexpected edgeToMap result: %v", eMap)
	}

	// Helper functions
	if getEnvOrDefaultAny([]string{"NONEXISTENT_VAR_123"}, "default") != "default" {
		t.Error("expected default value")
	}
	if getEnvIntOrDefaultAny([]string{"NONEXISTENT_VAR_123"}, 42) != 42 {
		t.Error("expected default int value")
	}
	cfg := getGraphConfig()
	if cfg.Port == 0 {
		t.Error("expected non-zero port")
	}
	_ = isGraphBackendEnabled()
}

func TestProjectContextAssembler_FullHierarchy(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories
	dirs := []string{
		paths.ProcessMissionsDir,
		paths.ProcessVisionsDir,
		paths.ProcessPoliciesDir,
		paths.ProcessInternalLifecyclesDir,
		paths.ProcessGoalsDir,
		paths.ProcessWorkstreamsDir,
		paths.ProcessPriorityPlansDir,
	}
	for _, d := range dirs {
		if err := fileutil.MkdirAll(filepath.Join(tmpDir, d), paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Write mission
	missionYAML := `id: MSN-001
title: Core Mission
mission_statement: Deliver high quality software
status: active
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, paths.ProcessMissionsDir, "MSN-001.yaml"), []byte(missionYAML))

	// Write vision
	visionYAML := `id: VIS-001
title: Vision 2026
vision_statement: Autonomous workflows
status: active
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, paths.ProcessVisionsDir, "VIS-001.yaml"), []byte(visionYAML))

	// Write policy
	policyYAML := `id: POL-001
title: Git Workflow
category: workflow
policy_type: standard
body: Follow PR protocol
status: active
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, paths.ProcessPoliciesDir, "POL-001.yaml"), []byte(policyYAML))

	// Write lifecycle
	lifecycleYAML := `object_type: backlog_item
statuses:
  - value: originated
  - value: planned
  - value: complete
transitions:
  - from: originated
    to: planned
    manual: true
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir, "backlog_item.yaml"), []byte(lifecycleYAML))

	// Write goal
	goalYAML := `id: GOAL-001
title: Test Coverage
status: active
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, paths.ProcessGoalsDir, "GOAL-001.yaml"), []byte(goalYAML))

	// Write workstream
	wsYAML := `id: WS-001
title: Core Engine
status: active
priority: high
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, paths.ProcessWorkstreamsDir, "WS-001.yaml"), []byte(wsYAML))

	// Write priority plan
	priYAML := `id: PRI-001
title: Coverage Elevation
status: active
priority: P0
`
	_ = fileutil.WriteSecureFile(filepath.Join(tmpDir, paths.ProcessPriorityPlansDir, "PRI-001.yaml"), []byte(priYAML))

	// Assemble
	pca := NewProjectContextAssembler(tmpDir)
	pCtx, err := pca.AssembleContext()
	if err != nil {
		t.Fatalf("AssembleContext failed: %v", err)
	}

	if pCtx.Mission == nil || pCtx.Mission.ID != "MSN-001" {
		t.Errorf("expected mission MSN-001, got %v", pCtx.Mission)
	}
	if pCtx.Vision == nil || pCtx.Vision.ID != "VIS-001" {
		t.Errorf("expected vision VIS-001, got %v", pCtx.Vision)
	}
	if pCtx.Policies == nil || len(pCtx.Policies.Workflow) == 0 {
		t.Errorf("expected workflow policy, got %v", pCtx.Policies)
	}
	if pCtx.Lifecycles == nil || len(pCtx.Lifecycles.ObjectKinds) == 0 {
		t.Errorf("expected lifecycles, got %v", pCtx.Lifecycles)
	}
	if pCtx.Strategy == nil || len(pCtx.Strategy.Goals) == 0 {
		t.Errorf("expected strategy goals, got %v", pCtx.Strategy)
	}
	if pCtx.CurrentState == nil || len(pCtx.CurrentState.ActivePriorityPlans) == 0 {
		t.Errorf("expected active priority plans, got %v", pCtx.CurrentState)
	}
}

func TestLifecycleEnumerator_AllMethods(t *testing.T) {
	lifecycles := &LifecyclesContext{
		ObjectKinds: []string{"backlog_item"},
		LifecycleMap: map[string]LifecycleSummary{
			"backlog_item": {
				ObjectType: "backlog_item",
				Statuses:   []string{"originated", "planned", "in_progress", "complete"},
				Transitions: []TransitionSummary{
					{From: "originated", To: "planned", Manual: true, Preconditions: []string{"has criteria"}},
					{From: "planned", To: "in_progress", Manual: false},
					{From: "in_progress", To: "complete", Manual: true},
				},
			},
		},
	}

	le := NewLifecycleEnumerator(lifecycles)
	leWithLoader := NewLifecycleEnumeratorWithLoader(lifecycles, nil)
	if leWithLoader == nil {
		t.Fatal("expected leWithLoader")
	}

	kinds := le.GetObjectKinds()
	if len(kinds) != 1 || kinds[0] != "backlog_item" {
		t.Errorf("unexpected kinds: %v", kinds)
	}

	if !le.HasLifecycle("backlog_item") {
		t.Error("expected HasLifecycle to be true")
	}
	if le.HasLifecycle("unknown") {
		t.Error("expected HasLifecycle to be false for unknown")
	}

	summary, ok := le.GetLifecycleSummary("backlog_item")
	if !ok || summary.ObjectType != "backlog_item" {
		t.Errorf("unexpected summary: %v", summary)
	}

	statuses := le.GetStatuses("backlog_item")
	if len(statuses) != 4 {
		t.Errorf("unexpected statuses length: %d", len(statuses))
	}
	if le.GetStatuses("unknown") != nil {
		t.Error("expected nil statuses for unknown")
	}

	trans := le.GetTransitions("backlog_item", 2)
	if len(trans) != 2 {
		t.Errorf("expected 2 transitions with limit, got %d", len(trans))
	}
	if le.GetTransitions("unknown", 0) != nil {
		t.Error("expected nil transitions for unknown")
	}

	overview := le.FormatLifecycleOverview()
	if overview == "" {
		t.Error("expected non-empty overview")
	}

	detail := le.FormatLifecycleForKind("backlog_item")
	if detail == "" {
		t.Error("expected non-empty detail")
	}
	if le.FormatLifecycleForKind("unknown") == "" {
		t.Error("expected non-empty message for unknown kind")
	}

	reqs := le.FormatLifecycleRequirements()
	if reqs == "" {
		t.Error("expected non-empty requirements")
	}

	valid, _ := le.ValidateTransition("backlog_item", "originated", "planned")
	if !valid {
		t.Error("expected transition to be valid")
	}
	valid2, _ := le.ValidateTransition("backlog_item", "originated", "complete")
	if valid2 {
		t.Error("expected transition to be invalid")
	}
	valid3, _ := le.ValidateTransition("unknown", "a", "b")
	if valid3 {
		t.Error("expected transition to be invalid for unknown kind")
	}

	validFrom := le.GetValidTransitionsFrom("backlog_item", "originated")
	if len(validFrom) != 1 {
		t.Errorf("expected 1 valid transition from originated, got %d", len(validFrom))
	}
	if le.GetValidTransitionsFrom("unknown", "originated") != nil {
		t.Error("expected nil for unknown kind")
	}

	validTo := le.GetValidTransitionsTo("backlog_item", "planned")
	if len(validTo) != 1 {
		t.Errorf("expected 1 valid transition to planned, got %d", len(validTo))
	}
	if le.GetValidTransitionsTo("unknown", "planned") != nil {
		t.Error("expected nil for unknown kind")
	}

	// Test nil receiver context
	leNil := NewLifecycleEnumerator(nil)
	if leNil.GetObjectKinds() != nil {
		t.Error("expected nil kinds")
	}
	if leNil.HasLifecycle("backlog_item") {
		t.Error("expected false")
	}
	if leNil.FormatLifecycleOverview() != "" {
		t.Error("expected empty string")
	}
	if leNil.FormatLifecycleRequirements() != "" {
		t.Error("expected empty string")
	}
}

func TestResourceMIMEAdapters_AllFormats(t *testing.T) {
	tmpDir := t.TempDir()

	// Markdown
	mdPath := filepath.Join(tmpDir, "test.md")
	mdContent := `---
title: Sample Document
description: Document description in frontmatter
category: docs
---

# Main Heading

This is the first paragraph describing the content.
`
	_ = fileutil.WriteSecureFile(mdPath, []byte(mdContent))

	// JSON
	jsonPath := filepath.Join(tmpDir, "test.json")
	jsonContent := `{"title": "JSON Title", "description": "JSON Description", "version": "1.0"}`
	_ = fileutil.WriteSecureFile(jsonPath, []byte(jsonContent))

	// YAML
	yamlPath := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `title: YAML Title
description: YAML Description
type: spec
`
	_ = fileutil.WriteSecureFile(yamlPath, []byte(yamlContent))

	// Plain text
	txtPath := filepath.Join(tmpDir, "test.txt")
	txtContent := "First line of text is description\nSecond line"
	_ = fileutil.WriteSecureFile(txtPath, []byte(txtContent))

	reg := NewResourceMIMEAdapterRegistry()

	// Test Markdown
	desc := reg.ExtractDescription(mdPath, "text/markdown; charset=utf-8")
	title := reg.ExtractTitle(mdPath, "text/markdown")
	meta := reg.ExtractMetadata(mdPath, "text/markdown")
	if desc == "" || title == "" || len(meta) == 0 {
		t.Errorf("markdown extraction failed: desc=%s, title=%s, meta=%v", desc, title, meta)
	}

	// Test JSON
	jDesc := reg.ExtractDescription(jsonPath, "application/json")
	jTitle := reg.ExtractTitle(jsonPath, "application/json")
	jMeta := reg.ExtractMetadata(jsonPath, "application/json")
	if jDesc != "JSON Description" || jTitle != "JSON Title" || len(jMeta) == 0 {
		t.Errorf("json extraction failed: desc=%s, title=%s, meta=%v", jDesc, jTitle, jMeta)
	}

	// Test YAML
	yDesc := reg.ExtractDescription(yamlPath, "application/yaml")
	yTitle := reg.ExtractTitle(yamlPath, "text/yaml")
	yMeta := reg.ExtractMetadata(yamlPath, "application/x-yaml")
	if yDesc != "YAML Description" || yTitle != "YAML Title" || len(yMeta) == 0 {
		t.Errorf("yaml extraction failed: desc=%s, title=%s, meta=%v", yDesc, yTitle, yMeta)
	}

	// Test PlainText
	tDesc := reg.ExtractDescription(txtPath, "text/plain")
	tTitle := reg.ExtractTitle(txtPath, "text/plain")
	tMeta := reg.ExtractMetadata(txtPath, "text/plain")
	if tDesc == "" || tTitle == "" || tMeta == nil {
		t.Errorf("plain text extraction failed: desc=%s, title=%s, meta=%v", tDesc, tTitle, tMeta)
	}

	// Fallback to unknown MIME type
	fDesc := reg.ExtractDescription(txtPath, "unknown/type")
	if fDesc == "" {
		t.Error("expected fallback extraction")
	}
}

func TestResourceLoader_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc.md")
	_ = fileutil.WriteSecureFile(filePath, []byte("# Markdown Content"))

	schemeResolver := NewResourceURISchemeResolver()
	mimeReg := NewResourceMIMEAdapterRegistry()
	loader := NewResourceLoader(tmpDir, schemeResolver, mimeReg)

	// Valid file://
	res, err := loader.LoadResource("file://" + filePath)
	if err != nil {
		t.Fatalf("LoadResource file:// failed: %v", err)
	}
	if string(res.Content) != "# Markdown Content" {
		t.Errorf("unexpected content: %s", string(res.Content))
	}
	if res.MimeType != "text/markdown" {
		t.Errorf("unexpected mimeType: %s", res.MimeType)
	}

	// LoadResourceWithMimeType override
	resOverride, err := loader.LoadResourceWithMimeType("file://"+filePath, "text/plain")
	if err != nil {
		t.Fatalf("LoadResourceWithMimeType failed: %v", err)
	}
	if resOverride.MimeType != "text/plain" {
		t.Errorf("expected overridden mimeType text/plain, got %s", resOverride.MimeType)
	}

	// Invalid URI (no scheme)
	_, err = loader.LoadResource("just_a_path.txt")
	if err == nil {
		t.Error("expected error for URI without scheme")
	}

	// Missing file
	_, err = loader.LoadResource("file:///nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}

	// Loader from context
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: tmpDir,
	}
	loaderCtx := NewResourceLoaderFromContext(initCtx, schemeResolver, mimeReg)
	if loaderCtx == nil {
		t.Fatal("expected non-nil loader from context")
	}
}

func TestRoleEnforcement_Comprehensive(t *testing.T) {
	e, o, errCount := GetRoleEnforcementStats()
	_ = e
	_ = o
	_ = errCount

	// Nil config
	res1, err := enforceRoleEnforcement(map[string]any{"client": "test"}, nil, "/tmp", nil)
	if err != nil || res1["client"] != "test" {
		t.Errorf("enforceRoleEnforcement with nil config failed: %v, %v", res1, err)
	}

	// EnforcedRole set
	cfg := &ServerConfig{}
	cfg.MCPServer.Security.EnforcedRole = "admin"
	res2, err := enforceRoleEnforcement(map[string]any{
		objects.FieldKeyRoles: []any{"viewer"},
	}, cfg, "/tmp", nil)
	if err != nil {
		t.Fatalf("enforceRoleEnforcement failed: %v", err)
	}
	roles, ok := res2[objects.FieldKeyRoles].([]any)
	if !ok || len(roles) != 1 || roles[0] != "admin" {
		t.Errorf("expected enforced role admin, got %v", res2[objects.FieldKeyRoles])
	}

	// AllowedRoles filtering
	cfg2 := &ServerConfig{}
	cfg2.MCPServer.Security.AllowedRoles = []string{"developer", "operator"}
	res3, err := enforceRoleEnforcement(map[string]any{
		objects.FieldKeyRoles: "developer,admin,guest",
	}, cfg2, "/tmp", nil)
	if err != nil {
		t.Fatalf("enforceRoleEnforcement failed: %v", err)
	}
	rList, _ := res3[objects.FieldKeyRoles].([]any)
	if len(rList) != 1 || rList[0] != "developer" {
		t.Errorf("expected filtered role [developer], got %v", rList)
	}
}

func TestCLIBridgeAccessControl_Comprehensive(t *testing.T) {
	// Nil secCtx
	res1 := applyObjectAccessControl(map[string]any{"key": "val"}, nil, nil)
	if res1["key"] != "val" {
		t.Errorf("expected unchanged map with nil secCtx, got %v", res1)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	// Nil cache
	res2 := applyObjectAccessControl(map[string]any{"key": "val"}, secCtx, nil)
	if res2["key"] != "val" {
		t.Errorf("expected unchanged map with nil cache, got %v", res2)
	}

	// With permission cache
	pc := NewPermissionCache(nil)
	res3 := applyObjectAccessControl(map[string]any{
		"objects": []any{
			map[string]any{objects.FieldKeyID: "BLI-1", objects.FieldKeyKind: "backlog_item"},
			"not a map",
		},
		"count": 2,
	}, secCtx, pc)
	if res3 == nil {
		t.Error("expected non-nil result")
	}

	// Single object
	res4 := applyObjectAccessControl(map[string]any{
		"object": map[string]any{objects.FieldKeyID: "BLI-1", objects.FieldKeyKind: "backlog_item"},
	}, secCtx, pc)
	if res4 == nil {
		t.Error("expected non-nil result")
	}

	// Direct object
	res5 := applyObjectAccessControl(map[string]any{
		objects.FieldKeyID:   "BLI-1",
		objects.FieldKeyKind: "backlog_item",
	}, secCtx, pc)
	if res5 == nil {
		t.Error("expected non-nil result")
	}
}

func TestJSONLDCLIOntology_Comprehensive(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "testapp",
		Short: "Test application for CLI ontology",
	}

	subCmd := &cobra.Command{
		Use:   "subcmd",
		Short: "Subcommand description",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	subCmd.Flags().String("name", "default", "User name")
	subCmd.Flags().Bool("verbose", false, "Verbose output")
	subCmd.Flags().Int("count", 5, "Item count")
	subCmd.Flags().StringSlice("tags", []string{"a", "b"}, "Tags list")

	rootCmd.AddCommand(subCmd)

	// Without metrics
	onto1, err := ConvertCommandTreeToJSONLD(rootCmd, false)
	if err != nil {
		t.Fatalf("ConvertCommandTreeToJSONLD without metrics failed: %v", err)
	}
	if onto1 == nil || len(onto1.Commands) == 0 {
		t.Errorf("expected non-empty commands, got %v", onto1)
	}

	// With metrics
	onto2, err := ConvertCommandTreeToJSONLD(rootCmd, true)
	if err != nil {
		t.Fatalf("ConvertCommandTreeToJSONLD with metrics failed: %v", err)
	}
	if onto2 == nil || len(onto2.Commands) == 0 {
		t.Error("expected non-empty commands in ontology")
	}
}

func TestServerErrors_Comprehensive(t *testing.T) {
	server := NewServer()

	// isCriticalError nil
	if server.isCriticalError(nil) {
		t.Error("expected false for nil error")
	}

	// isCriticalError with regular error
	if server.isCriticalError(errors.New("generic error")) {
		t.Error("expected false for generic error")
	}

	// isCriticalError with JSONRPCError
	jsonErr := &JSONRPCError{Code: AuthenticationFailed, Message: "auth failed"}
	if !server.isCriticalError(jsonErr) {
		t.Error("expected AuthenticationFailed to be critical")
	}
	nonCritErr := &JSONRPCError{Code: InternalError, Message: "internal error"}
	if server.isCriticalError(nonCritErr) {
		t.Error("expected InternalError not to be critical")
	}

	// isCriticalErrorCode with custom config
	cfg := &ServerConfig{}
	cfg.MCPServer.ErrorHandling.CriticalErrorCodes = []int{42}
	server.SetConfig(cfg)
	if !server.isCriticalErrorCode(42) {
		t.Error("expected custom critical error code 42 to be critical")
	}
	if server.isCriticalErrorCode(999) {
		t.Error("expected 999 not to be critical with custom config")
	}

	// SendCriticalError with various severities and suppression
	err := server.SendCriticalError(errors.New("some error"), "fatal", "system", "message", []string{"fix it"}, map[string]any{"k": "v"}, "cli-1", true)
	if err != nil {
		t.Errorf("SendCriticalError suppressed returned error: %v", err)
	}

	err = server.SendCriticalError(errors.New("some error"), "warning", "system", "message", []string{"fix it"}, nil, "", true)
	if err != nil {
		t.Errorf("SendCriticalError warning returned error: %v", err)
	}

	err = server.SendCriticalError(nil, "error", "cat", "msg", nil, nil, "", false)
	if err != nil {
		t.Errorf("SendCriticalError with nil error returned error: %v", err)
	}
}

func TestLoggingChannel_Comprehensive(t *testing.T) {
	b := NewLogMetricsBuilder(LogLevelInfo, "test log message", map[string]any{"key": "value"}, time.Now())
	metrics := b.TransportReady(true).DeliveryMethod("stdio").Success(true).Error("").Build()
	if metrics["level"] != "info" || metrics["delivery_method"] != "stdio" {
		t.Errorf("unexpected metrics built: %v", metrics)
	}

	server := NewServer()
	// Test when shutdown flag is ordered
	server.shutdownFlag.Store(1)

	_ = server.SendLogMessage(LogLevelDebug, "debug msg", nil)
	_ = server.SendLogMessage(LogLevelInfo, "info msg", nil)
	_ = server.SendLogMessage(LogLevelWarn, "warn msg", nil)
	_ = server.SendLogMessage(LogLevelError, "error msg", nil)

	_ = server.SendLogDebug("debug convenience", nil)
	_ = server.SendLogInfo("info convenience", nil)
	_ = server.SendLogWarn("warn convenience", nil)
	_ = server.SendLogError("error convenience", nil)
}

func TestProxyDaemon_Comprehensive(t *testing.T) {
	p := NewProxyDaemon("127.0.0.1:9999", logging.GetLoggerFromProfile(""))
	hSent, hFail, recons := p.GetProxyDaemonStats()
	if hSent != 0 || hFail != 0 || recons != 0 {
		t.Errorf("expected 0 stats initially, got %d, %d, %d", hSent, hFail, recons)
	}

	// Nil receiver stats
	var pNil *ProxyDaemon
	s1, s2, s3 := pNil.GetProxyDaemonStats()
	if s1 != 0 || s2 != 0 || s3 != 0 {
		t.Errorf("expected 0 stats for nil proxy, got %d, %d, %d", s1, s2, s3)
	}

	// StampIDEInitializeClientInfo
	initRaw := []byte(`{"jsonrpc":"2.0","method":"initialize","params":{"clientInfo":{"name":"client-x","version":"1.0"}}}`)
	stamped := StampIDEInitializeClientInfo(initRaw)
	if len(stamped) == 0 {
		t.Error("expected non-empty stamped bytes")
	}

	// isConnRefused
	if isConnRefused(nil) {
		t.Error("expected false for nil error")
	}
	if !isConnRefused(syscall.ECONNREFUSED) {
		t.Error("expected true for ECONNREFUSED")
	}
	if !isConnRefused(errors.New("dial tcp 127.0.0.1:9999: connection refused")) {
		t.Error("expected true for string match connection refused")
	}
	if isConnRefused(errors.New("some other error")) {
		t.Error("expected false for other error")
	}

	opErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &os.SyscallError{
			Syscall: "connect",
			Err:     syscall.ECONNREFUSED,
		},
	}
	if !isConnRefused(opErr) {
		t.Error("expected true for nested OpError with ECONNREFUSED")
	}
}

func TestToolsExternal_Comprehensive(t *testing.T) {
	server := NewServer()
	RegisterExternalTools(server)

	ctx := context.Background()

	// Missing secret
	t.Setenv(zqkenv.MCPExternalSecretKey().Name(), "")
	_, err := HandleGetSignedUrl(ctx, map[string]any{
		objects.FieldKeyPath: "/tmp/data.bin",
		"ttl_seconds":        300.0,
	})
	if err == nil {
		t.Error("expected error when MCPExternalSecretKey is not set")
	}

	// With secret
	t.Setenv(zqkenv.MCPExternalSecretKey().Name(), "super-secret-signing-key-for-tests-12345")
	res, err := HandleGetSignedUrl(ctx, map[string]any{
		objects.FieldKeyPath: "/tmp/data.bin",
		"ttl_seconds":        300.0,
	})
	if err != nil {
		t.Fatalf("HandleGetSignedUrl failed: %v", err)
	}
	resMap, ok := res.(map[string]any)
	if !ok || resMap["signed_url"] == nil {
		t.Errorf("expected signed_url in result: %v", res)
	}

	// Invalid args for HandleGetSignedUrl
	_, err = HandleGetSignedUrl(ctx, map[string]any{"ttl_seconds": 300.0})
	if err == nil {
		t.Error("expected error for missing path")
	}
	_, err = HandleGetSignedUrl(ctx, map[string]any{objects.FieldKeyPath: "/tmp/file"})
	if err == nil {
		t.Error("expected error for missing ttl_seconds")
	}

	// HandleFetchChunk
	tmpFile := filepath.Join(t.TempDir(), "chunk_test.bin")
	_ = fileutil.WriteSecureFile(tmpFile, []byte("0123456789ABCDEF"))

	chunkRes, err := HandleFetchChunk(ctx, map[string]any{
		objects.FieldKeyPath: tmpFile,
		"chunk_size":         4.0,
		"chunk_index":        1.0,
	})
	if err != nil {
		t.Fatalf("HandleFetchChunk failed: %v", err)
	}
	cMap, ok := chunkRes.(map[string]any)
	if !ok || cMap["data"] == nil {
		t.Errorf("expected chunk data in result: %v", chunkRes)
	}

	// Invalid chunk args
	_, err = HandleFetchChunk(ctx, map[string]any{})
	if err == nil {
		t.Error("expected error for missing path")
	}
	_, err = HandleFetchChunk(ctx, map[string]any{objects.FieldKeyPath: tmpFile})
	if err == nil {
		t.Error("expected error for missing chunk_size")
	}
	_, err = HandleFetchChunk(ctx, map[string]any{objects.FieldKeyPath: tmpFile, "chunk_size": 4.0})
	if err == nil {
		t.Error("expected error for missing chunk_index")
	}
}

func TestSchemaHandlers_Comprehensive(t *testing.T) {
	server := NewServer()

	// Custom handler
	server.RegisterSchemaHandler("schema://custom/test", func(s *Server, uri string) (any, error) {
		return map[string]string{"custom": "ok"}, nil
	}, false)

	resCustom, err := server.handleSchemaResource("schema://custom/test")
	if err != nil {
		t.Fatalf("handleSchemaResource custom failed: %v", err)
	}
	if resMap, ok := resCustom.(map[string]string); !ok || resMap["custom"] != "ok" {
		t.Errorf("unexpected custom schema result: %v", resCustom)
	}

	// Fallback schema://registry
	resReg, err := server.handleSchemaResource("schema://registry")
	if err != nil {
		t.Logf("schema://registry error: %v", err)
	} else if resReg == nil {
		t.Error("expected non-nil registry schema")
	}

	// Fallback schema://cli
	resCli, err := server.handleSchemaResource("schema://cli")
	if err != nil {
		t.Logf("schema://cli error: %v", err)
	} else if resCli == nil {
		t.Error("expected non-nil cli schema")
	}

	// Fallback schema://common_fields
	resCommon, err := server.handleSchemaResource("schema://common_fields")
	if err != nil {
		t.Logf("schema://common_fields error: %v", err)
	} else if resCommon == nil {
		t.Error("expected non-nil common fields schema")
	}

	// Fallback schema://object/{kind}
	resObj, err := server.handleSchemaResource("schema://object/backlog_item")
	if err != nil {
		t.Logf("schema://object/backlog_item error: %v", err)
	} else if resObj == nil {
		t.Error("expected non-nil object schema")
	}

	// Unknown schema
	_, err = server.handleSchemaResource("schema://completely_unknown")
	if err == nil {
		t.Error("expected error for unknown schema")
	}
}

func TestClientMetricsStore_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	metricsPath := filepath.Join(tmpDir, "client_metrics.json")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := NewClientMetricsStore(metricsPath, ctx)
	if err != nil {
		t.Fatalf("NewClientMetricsStore failed: %v", err)
	}

	// Nil store stats
	var nilStore *ClientMetricsStore
	sE, sS := nilStore.GetClientMetricsStats()
	if sE != 0 || sS != 0 {
		t.Errorf("expected 0 for nil store stats, got %d, %d", sE, sS)
	}

	// Buffered event (empty clientID)
	err = store.RecordEvent("seq-1", "", "connect", map[string]any{"source": "test"})
	if err != nil {
		t.Fatalf("RecordEvent buffered failed: %v", err)
	}

	// Update clientID - should flush buffered event
	err = store.UpdateClientID("seq-1", "client-alpha")
	if err != nil {
		t.Fatalf("UpdateClientID failed: %v", err)
	}

	// Direct event with clientID
	err = store.RecordEvent("seq-1", "client-alpha", "initialize", map[string]any{
		"tools_count": 12,
	})
	if err != nil {
		t.Fatalf("RecordEvent direct failed: %v", err)
	}

	// Tools list event
	err = store.RecordEvent("seq-1", "client-alpha", "tools_list", map[string]any{
		"tools_count": 12.0,
	})
	if err != nil {
		t.Fatalf("RecordEvent tools_list failed: %v", err)
	}

	events, seqs := store.GetClientMetricsStats()
	if events < 2 || seqs < 1 {
		t.Errorf("unexpected metrics stats: events=%d, seqs=%d", events, seqs)
	}

	// Save
	_ = store.Save()
	_ = store.saveNow()

	// Close
	_ = store.Close()

	// Reload in a new store
	store2, err := NewClientMetricsStore(metricsPath, ctx)
	if err != nil {
		t.Fatalf("NewClientMetricsStore reload failed: %v", err)
	}
	if len(store2.metrics) == 0 {
		t.Error("expected reloaded metrics to be present")
	}
}

func TestServerToolExecution_Builtins(t *testing.T) {
	server := NewServer()
	server.SetProjectRoot(t.TempDir())
	RegisterEchoTool(server)

	ctx := context.Background()

	// test_echo
	resEcho, err := server.handleToolCallWithContext(ctx, "test_echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Errorf("test_echo failed: %v", err)
	}
	if resEcho == nil {
		t.Error("expected non-nil echo response")
	}

	// echo alias
	resEcho2, err := server.handleToolCallWithContext(ctx, "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Errorf("echo failed: %v", err)
	}
	if resEcho2 == nil {
		t.Error("expected non-nil echo response")
	}

	// system_status
	_, _ = server.handleToolCallWithContext(ctx, "system_status", map[string]any{})

	// system_check
	_, _ = server.handleToolCallWithContext(ctx, "system_check", map[string]any{})
}

func TestRoleGuidance_StorageResultAdapter(t *testing.T) {
	// Adapter with map of []map[string]any
	res1 := AdaptStorageResult(map[string]any{
		"objects": []map[string]any{{"id": "1"}},
		"meta":    map[string]any{"count": 1},
	})
	if len(res1.Objects) != 1 || res1.Meta["count"] != 1 {
		t.Errorf("unexpected AdaptStorageResult result: %v", res1)
	}

	// Adapter with map of []any
	res2 := AdaptStorageResult(map[string]any{
		"objects": []any{map[string]any{"id": "2"}, "invalid"},
	})
	if len(res2.Objects) != 1 {
		t.Errorf("expected 1 object, got %d", len(res2.Objects))
	}

	// Non-map input
	res3 := AdaptStorageResult("invalid")
	if res3.Objects != nil || res3.Meta != nil {
		t.Errorf("expected nil result for invalid input, got %v", res3)
	}
}

func TestRoleAwarePrompts_AllPersonas(t *testing.T) {
	pCtx := &ProjectContext{
		Mission: &MissionContext{ID: "M-1", Title: "Mission", Statement: "Mission statement"},
		Vision:  &VisionContext{ID: "V-1", Title: "Vision", Statement: "Vision statement"},
		Policies: &PoliciesContext{
			Workflow: []PolicySummary{{ID: "P-1", Title: "Git", Body: "Rule"}},
		},
		Lifecycles: &LifecyclesContext{
			ObjectKinds: []string{"backlog_item"},
		},
		Strategy: &StrategyContext{
			Goals: []GoalSummary{{ID: "G-1", Title: "Goal 1"}},
		},
		CurrentState: &CurrentStateContext{
			ActivePriorityPlans: []PriorityPlanSummary{{ID: "PRI-1", Title: "Pri 1"}},
		},
	}

	personas := []string{"coder", "backend", "tpm", "orchestrator", "observer", "architect", "unknown"}
	for _, p := range personas {
		storage := &testStorageProvider{}
		gen := NewRoleAwarePromptGenerator(pCtx, []string{"viewer"}).
			WithAssigneePersona(p).
			WithStorageProvider(storage).
			WithRoleGuidanceGenerator(NewRoleGuidanceGenerator(storage)).
			WithSecurityContext(pkgctx.NewSystemSecurityContext())

		welcome, start := gen.GenerateWelcomeMessage()
		if welcome == "" || start == "" {
			t.Errorf("expected welcome message for persona %s", p)
		}

		bigPic := gen.GenerateBigPicturePrompt()
		if bigPic == "" {
			t.Errorf("expected big picture prompt for persona %s", p)
		}

		execCtx := gen.GenerateExecutionContextPrompt()
		if execCtx == "" {
			t.Errorf("expected execution context prompt for persona %s", p)
		}

		myRole := gen.GenerateMyRolePrompt()
		if myRole == "" {
			t.Errorf("expected my role prompt for persona %s", p)
		}
	}
}

func TestServerInitHelpers_ExtractAndPrepare(t *testing.T) {
	initParams := InitializeParams{
		Capabilities: map[string]any{
			clientInfoAccountID: "ACC-001",
			objects.FieldKeySessionID: "sess-123",
			clientInfoKeystoreKeyID: "key-99",
			objects.FieldKeyUsername: "user-1",
			"password": "secret-password",
			"oauth_token": "token-xyz",
			"personal_access_token": "pat-abc",
			clientInfoRoles: []any{"developer", "operator"},
			clientInfoPermissions: []any{"read:all", "write:code"},
		},
	}
	initParams.ClientInfo.Name = "agent-1"
	initParams.ClientInfo.Version = "2.1"

	info := extractClientInfoFromInitParams(initParams)
	if info[clientInfoName] != "agent-1" || info[clientInfoAccountID] != "ACC-001" {
		t.Errorf("unexpected extracted info: %v", info)
	}

	server := NewServer()
	// First init (not initialized yet)
	server.prepareReinitialization()

	// Multi-client mode
	server.initialized.Store(true)
	server.multiClient.Store(true)
	server.prepareReinitialization()

	// Single client re-init (clears tools)
	server.multiClient.Store(false)
	server.prepareReinitialization()
	if server.initialized.Load() {
		t.Error("expected server to be uninitialized after prepareReinitialization")
	}
}

func TestServerHandlersAuth_ValidateProjectRoot(t *testing.T) {
	// Empty
	if err := validateProjectRootForProcessData(""); err == nil {
		t.Error("expected error for empty project root")
	}

	// Inside pkg/mcp
	if err := validateProjectRootForProcessData("/root/repo/pkg/mcp/sub"); err == nil {
		t.Error("expected error for project root inside pkg/mcp")
	}

	// Inside cmd/
	if err := validateProjectRootForProcessData("/root/repo/cmd/zqk"); err == nil {
		t.Error("expected error for project root inside cmd/")
	}

	// Directory without .zqk/process
	tmpDir := t.TempDir()
	if err := validateProjectRootForProcessData(tmpDir); err == nil {
		t.Error("expected error for directory missing .zqk/process")
	}

	// Directory with .zqk/process
	procDir := filepath.Join(tmpDir, paths.ProcessDir)
	_ = fileutil.MkdirAll(procDir, paths.DirPerm755)
	if err := validateProjectRootForProcessData(tmpDir); err != nil {
		t.Errorf("expected valid project root, got error: %v", err)
	}
}
