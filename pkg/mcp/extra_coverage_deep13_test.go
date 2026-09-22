package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// 1. MCPSpec Validate and GetName tests
func TestDeep13_MCPSpec_ValidationComprehensive(t *testing.T) {
	// Empty spec name
	sEmptyName := &MCPSpec{}
	if err := sEmptyName.Validate(); err == nil {
		t.Error("expected error for empty spec name")
	}

	// Empty prompt name
	sEmptyPrompt := &MCPSpec{
		Name: "test_spec",
		Prompts: []PromptSpec{
			{Name: ""},
		},
	}
	if err := sEmptyPrompt.Validate(); err == nil {
		t.Error("expected error for empty prompt name")
	}

	// Empty resource URI
	sEmptyResURI := &MCPSpec{
		Name: "test_spec",
		Resources: []ResourceSpec{
			{URI: "", Name: "res1"},
		},
	}
	if err := sEmptyResURI.Validate(); err == nil {
		t.Error("expected error for empty resource URI")
	}

	// Empty resource Name
	sEmptyResName := &MCPSpec{
		Name: "test_spec",
		Resources: []ResourceSpec{
			{URI: "file://test", Name: ""},
		},
	}
	if err := sEmptyResName.Validate(); err == nil {
		t.Error("expected error for empty resource name")
	}

	// Empty tool name
	sEmptyTool := &MCPSpec{
		Name: "test_spec",
		Tools: []ToolSpec{
			{Name: ""},
		},
	}
	if err := sEmptyTool.Validate(); err == nil {
		t.Error("expected error for empty tool name")
	}

	// Valid spec
	sValid := &MCPSpec{
		Name: "valid_spec",
		Prompts: []PromptSpec{
			{Name: "prompt1"},
		},
		Resources: []ResourceSpec{
			{URI: "file://test", Name: "res1"},
		},
		Tools: []ToolSpec{
			{Name: "tool1"},
		},
	}
	if err := sValid.Validate(); err != nil {
		t.Errorf("expected valid spec, got: %v", err)
	}
	if sValid.GetName() != "valid_spec" {
		t.Errorf("expected valid_spec name, got: %s", sValid.GetName())
	}
}

// 2. MCPServerAdapter operations tests
func TestDeep13_MCPServerAdapter_Operations(t *testing.T) {
	s := NewServer()
	s.initialized.Store(true)
	ctx := context.Background()

	adapter := NewMCPServerAdapter(s)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	// ListTools
	toolsRes, err := adapter.ListTools(ctx)
	if err != nil || toolsRes == nil {
		t.Errorf("ListTools failed: %v", err)
	}

	// ListResources
	resRes, err := adapter.ListResources(ctx, nil)
	if err != nil || resRes == nil {
		t.Errorf("ListResources failed: %v", err)
	}

	// ListPrompts
	promptsRes, err := adapter.ListPrompts(ctx)
	if err != nil || promptsRes == nil {
		t.Errorf("ListPrompts failed: %v", err)
	}

	// CallTool
	callRes, err := adapter.CallTool(ctx, &ToolCallParams{
		Name:      "test_echo",
		Arguments: map[string]any{"message": "adapter_pong"},
	})
	if err != nil || callRes == nil {
		t.Errorf("CallTool failed: %v", err)
	}

	// Shutdown
	_ = adapter.Shutdown(ctx)
}

// 3. LoggingChannel shutdown handling and metrics builder
func TestDeep13_LoggingChannel_ShutdownAndBuilders(t *testing.T) {
	// LogMetricsBuilder
	startTime := time.Now().Add(-10 * time.Millisecond)
	builder := NewLogMetricsBuilder(LogLevelInfo, "test log message", map[string]any{"k": "v"}, startTime)
	metrics := builder.DeliveryMethod(logDeliveryMethodTraceFile).
		Error(logErrorTypeWriteFailed).
		Success(false).
		Build()
	if metrics["delivery_method"] != logDeliveryMethodTraceFile {
		t.Errorf("expected delivery method trace_file: %v", metrics)
	}

	// Server SendLogMessage with shutdownFlag
	s := NewServer()
	var buf bytes.Buffer
	s.SetTraceWriter(&buf)
	s.shutdownFlag.Store(1)

	err := s.SendLogMessage(LogLevelInfo, "shutdown log message", map[string]any{"step": "cleanup"})
	if err != nil {
		t.Errorf("SendLogMessage failed during shutdown: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("shutdown log message")) {
		t.Errorf("expected trace log written: %s", buf.String())
	}

	// Server SendLogMessage with cancelled shutdownCtx
	s2 := NewServer()
	var buf2 bytes.Buffer
	s2.SetTraceWriter(&buf2)
	s2.shutdownCancel()

	err2 := s2.SendLogMessage(LogLevelWarn, "context cancelled log", map[string]any{"alert": true})
	if err2 != nil {
		t.Errorf("SendLogMessage failed with cancelled ctx: %v", err2)
	}
	if !bytes.Contains(buf2.Bytes(), []byte("context cancelled log")) {
		t.Errorf("expected trace log written: %s", buf2.String())
	}

	// Log level helpers
	s3 := NewServer()
	_ = s3.SendLogInfo("info log", nil)
	_ = s3.SendLogWarn("warn log", nil)
	_ = s3.SendLogError("error log", nil)
	_ = s3.SendLogDebug("debug log", nil)
}

// 4. LockingHelpers withClientsReadLock, queueForWriter, and writer subscriptions
func TestDeep13_LockingHelpers_Comprehensive(t *testing.T) {
	s := NewServer()

	// Normal withClientsReadLock
	called := false
	ok := s.withClientsReadLock(func() bool {
		called = true
		return true
	})
	if !ok || !called {
		t.Error("expected withClientsReadLock to execute callback")
	}

	// withClientsReadLock with shutdownFlag set
	s.shutdownFlag.Store(1)
	okFlag := s.withClientsReadLock(func() bool {
		return true
	})
	if okFlag {
		t.Error("expected withClientsReadLock to return false when shutdownFlag is set")
	}

	// withClientsReadLock with cancelled context
	s2 := NewServer()
	s2.shutdownCancel()
	okCtx := s2.withClientsReadLock(func() bool {
		return true
	})
	if okCtx {
		t.Error("expected withClientsReadLock to return false when context is cancelled")
	}

	// queueForWriter
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: true}

	q1 := s.queueForWriter(writer, format)
	if q1 == nil || !q1.IsActive() {
		t.Error("expected active queue for writer")
	}
	q2 := s.queueForWriter(writer, format)
	if q2 != q1 {
		t.Error("expected same active queue for same writer")
	}

	// track and forget writer subscriptions
	s.trackWriterSubscription(writer, "sub-123")
	s.forgetWriterSubscription("sub-123")
	s.forgetWriterSubscription("")
}

// 5. AutoInstall and config discovery helpers
func TestDeep13_AutoInstall_AndConfigs(t *testing.T) {
	tmpDir := t.TempDir()
	logger := testMCPLogger()

	// Normal AutoInstall with valid projectRoot
	_ = AutoInstall(tmpDir, logger)

	// AutoInstall with empty projectRoot -> expects error
	errEmpty := AutoInstall("", logger)
	if errEmpty == nil {
		t.Error("expected error for empty projectRoot")
	}

	// findMCPConfigs
	c1 := findMCPConfigs("", "")
	if len(c1) != 0 {
		t.Errorf("expected empty configs for empty home and project, got %d", len(c1))
	}
	c2 := findMCPConfigs("/fake/home", "/fake/proj")
	if len(c2) == 0 {
		t.Error("expected non-empty configs")
	}

	// isForeignMCPConfig
	if !isForeignMCPConfig("AGY", "/path/to/.gemini/mcp.json") {
		t.Error("expected foreign mcp config for AGY")
	}
	if !isForeignMCPConfig("Windsurf", "/path/to/mcp.json") {
		t.Error("expected foreign mcp config for Windsurf")
	}
	if !isForeignMCPConfig("IDE", "/path/to/.codeium/mcp.json") {
		t.Error("expected foreign mcp config for .codeium")
	}
	if isForeignMCPConfig("IDE", "/path/to/normal/mcp.json") {
		t.Error("expected non-foreign mcp config")
	}

	// isIDEStdioAdapterConfig
	if !isIDEStdioAdapterConfig("IDE (Workspace)", "/path/mcp.json") {
		t.Error("expected true for IDE (Workspace)")
	}
	if !isIDEStdioAdapterConfig("Cursor (Workspace)", "/path/mcp.json") {
		t.Error("expected true for Cursor (Workspace)")
	}
	if !isIDEStdioAdapterConfig("Cursor (Global)", "/path/mcp.json") {
		t.Error("expected true for Cursor (Global)")
	}
	if isIDEStdioAdapterConfig("Other", "/path/mcp.json") {
		t.Error("expected false for Other")
	}
}

type mockEventCoordinatorDeep13 struct {
	emitted []any
}

func (m *mockEventCoordinatorDeep13) Emit(ctx context.Context, eventCtx any) error {
	m.emitted = append(m.emitted, eventCtx)
	return nil
}

// 6. MCPServerAdapter Async & Batch Operations
func TestDeep13_MCPServerAdapter_AsyncAndBatch(t *testing.T) {
	s := NewServer()
	s.initialized.Store(true)
	resFile := filepath.Join(t.TempDir(), "info.txt")
	_ = os.WriteFile(resFile, []byte("hello resource"), 0644)
	fileURI := "file://" + filepath.ToSlash(resFile)

	s.RegisterResource(fileURI, "File Info", "Info", "text/plain")
	s.RegisterPrompt("welcome", "Welcome prompt", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coord := &mockEventCoordinatorDeep13{}
	adapter := NewMCPServerAdapter(s)
	adapter.SetCoordinator(coord)

	// CallToolAsync
	chTool := adapter.CallToolAsync(ctx, &ToolCallParams{
		Name:      "test_echo",
		Arguments: map[string]any{"message": "async_ping"},
	})
	resTool := <-chTool
	if resTool.Error != nil {
		t.Errorf("CallToolAsync error: %v", resTool.Error)
	}

	// ListToolsAsync
	chTools := adapter.ListToolsAsync(ctx)
	resTools := <-chTools
	if resTools.Error != nil {
		t.Errorf("ListToolsAsync error: %v", resTools.Error)
	}

	// GetResourceAsync
	chRes := adapter.GetResourceAsync(ctx, &ResourceGetParams{URI: fileURI})
	resRes := <-chRes
	if resRes.Error != nil {
		t.Errorf("GetResourceAsync error: %v", resRes.Error)
	}

	// ListResourcesAsync
	chResList := adapter.ListResourcesAsync(ctx, &ResourcesListParams{})
	resResList := <-chResList
	if resResList.Error != nil {
		t.Errorf("ListResourcesAsync error: %v", resResList.Error)
	}

	// GetPromptAsync
	chPrompt := adapter.GetPromptAsync(ctx, &PromptGetParams{Name: "welcome"})
	resPrompt := <-chPrompt
	if resPrompt.Error != nil {
		t.Errorf("GetPromptAsync error: %v", resPrompt.Error)
	}

	// ListPromptsAsync
	chPrompts := adapter.ListPromptsAsync(ctx)
	resPrompts := <-chPrompts
	if resPrompts.Error != nil {
		t.Errorf("ListPromptsAsync error: %v", resPrompts.Error)
	}

	// CallToolsBatch
	batchParams := []*ToolCallParams{
		{Name: "test_echo", Arguments: map[string]any{"message": "batch1"}},
		{Name: "test_echo", Arguments: map[string]any{"message": "batch2"}},
	}
	chBatch := adapter.CallToolsBatch(ctx, batchParams)
	for res := range chBatch {
		if res.Error != nil {
			t.Errorf("CallToolsBatch item %d error: %v", res.Index, res.Error)
		}
	}

	// GetResourcesBatch
	batchResParams := []*ResourceGetParams{
		{URI: fileURI},
	}
	chBatchRes := adapter.GetResourcesBatch(ctx, batchResParams)
	for res := range chBatchRes {
		if res.Error != nil {
			t.Errorf("GetResourcesBatch item %d error: %v", res.Index, res.Error)
		}
	}

	// GetMetricsSnapshot
	snap := adapter.GetMetricsSnapshot()
	if snap.Timestamp.IsZero() {
		t.Error("unexpected zero snapshot timestamp")
	}

	// SendLogMessage
	errLog := adapter.SendLogMessage(ctx, LogLevelInfo, "adapter log message", map[string]any{"k": "v"})
	if errLog != nil {
		t.Errorf("SendLogMessage error: %v", errLog)
	}

	canceledCtx, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if err := adapter.SendLogMessage(canceledCtx, LogLevelInfo, "canceled log", nil); err == nil {
		t.Error("expected error for canceled context in SendLogMessage")
	}

	// SendEvent
	_ = adapter.SendEvent(ctx, &Event{Type: EventType("test.event"), Message: "test", Fields: map[string]any{"key": "value"}})

	// SendMessage
	_ = adapter.SendMessage(ctx, "test message", "info", "high")

	// NotifyInitialized
	_ = adapter.NotifyInitialized(ctx, &InitializedParams{})

	// NotifyCancelled
	_ = adapter.NotifyCancelled(ctx, &CancelledParams{})

	// ListRoots
	_, _ = adapter.ListRoots(ctx)

	// GetResource
	_, _ = adapter.GetResource(ctx, &ResourceGetParams{URI: fileURI})

	// GetPrompt
	_, _ = adapter.GetPrompt(ctx, &PromptGetParams{Name: "welcome"})
}

// 7. ToolsWorkflow Extractors
func TestDeep13_ToolsWorkflow_Extractors(t *testing.T) {
	// extractWhatsNextLeadPlan
	p1, ok, err := extractWhatsNextLeadPlan(map[string]any{workflowKeyPriorityPlan: "plan1"})
	if err != nil || !ok || p1 != "plan1" {
		t.Errorf("extractWhatsNextLeadPlan map failed: %v, %v, %v", p1, ok, err)
	}

	p2, ok, err := extractWhatsNextLeadPlan(map[string]any{"data": map[string]any{workflowKeyPriorityPlan: "plan2"}})
	if err != nil || !ok || p2 != "plan2" {
		t.Errorf("extractWhatsNextLeadPlan data map failed: %v, %v, %v", p2, ok, err)
	}

	_, ok, err = extractWhatsNextLeadPlan(map[string]any{})
	if err != nil || ok {
		t.Errorf("expected not found for empty map: %v, %v", ok, err)
	}

	p3, ok, err := extractWhatsNextLeadPlan(`{"priority_plan": "plan3"}`)
	if err != nil || !ok || p3 != "plan3" {
		t.Errorf("extractWhatsNextLeadPlan string failed: %v, %v, %v", p3, ok, err)
	}

	_, _, err = extractWhatsNextLeadPlan("not json")
	if err == nil {
		t.Error("expected error for invalid json string")
	}

	_, _, err = extractWhatsNextLeadPlan(12345)
	if err == nil {
		t.Error("expected error for invalid type")
	}

	// extractWorkflowResultItem
	it1, ok, err := extractWorkflowResultItem(map[string]any{workflowKeyObjects: []any{"item1"}})
	if err != nil || !ok || it1 != "item1" {
		t.Errorf("extractWorkflowResultItem map []any failed: %v, %v, %v", it1, ok, err)
	}

	it2, ok, err := extractWorkflowResultItem(map[string]any{workflowKeyObjects: []map[string]any{{"key": "val"}}})
	if err != nil || !ok {
		t.Errorf("extractWorkflowResultItem map []map failed: %v, %v, %v", it2, ok, err)
	}

	it3, ok, err := extractWorkflowResultItem([]any{"item3"})
	if err != nil || !ok || it3 != "item3" {
		t.Errorf("extractWorkflowResultItem []any failed: %v, %v, %v", it3, ok, err)
	}

	it4, ok, err := extractWorkflowResultItem([]map[string]any{{"key": "val4"}})
	if err != nil || !ok {
		t.Errorf("extractWorkflowResultItem []map failed: %v, %v, %v", it4, ok, err)
	}

	it5, ok, err := extractWorkflowResultItem(`{"objects": ["item5"]}`)
	if err != nil || !ok || it5 != "item5" {
		t.Errorf("extractWorkflowResultItem string failed: %v, %v, %v", it5, ok, err)
	}

	_, ok, err = extractWorkflowResultItem(`{"objects": []}`)
	if err != nil || ok {
		t.Errorf("expected not found for empty objects: %v, %v", ok, err)
	}

	_, _, err = extractWorkflowResultItem("invalid json")
	if err == nil {
		t.Error("expected error for invalid json")
	}
}

type mockStorageProviderDeep13 struct {
	resSpecs []ResourceSpec
}

func (m *mockStorageProviderDeep13) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}

func (m *mockStorageProviderDeep13) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return nil, os.ErrNotExist
}

func (m *mockStorageProviderDeep13) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
	specName := "schema_resources"
	if filterMap, ok := filter.(map[string]any); ok {
		if n, ok := filterMap[objects.FieldKeyName].(string); ok && n != "" {
			specName = n
		}
	}
	specObj := map[string]any{
		objects.FieldKeyID:   "mcp-spec-schema-1",
		objects.FieldKeyKind: objects.KindMcpSpec,
		objects.FieldKeyName: specName,
		objects.FieldKeySpec: map[string]any{
			objects.FieldKeyName: specName,
			"resources":          m.resSpecs,
		},
	}
	return map[string]any{"objects": []any{specObj}}, nil
}

// 8. SchemaResources Spec and Handlers
func TestDeep13_SchemaResources_SpecAndHandlers(t *testing.T) {
	s := NewServer()
	s.storageProvider = &mockStorageProviderDeep13{
		resSpecs: []ResourceSpec{
			{
				URI:         "schema://object/task",
				Name:        "task_schema",
				Description: "Task schema",
				MimeType:    "application/json",
				Category:    "schema",
				Priority:    "high",
				Tags:        []string{"task", "schema"},
				Metadata:    map[string]string{"v": "1.0"},
			},
			{
				URI:  "other://ignore",
				Name: "ignore_me",
			},
		},
	}

	RegisterSchemaResources(s)
	_ = RegisterAllObjectSchemaResources(s)
	RegisterDefaultSchemaHandlers(s)
}

// 9. ClientMetrics UpdateClientID and Sanitize
func TestDeep13_ClientMetrics_UpdateClientIDAndSanitize(t *testing.T) {
	store, err := NewClientMetricsStore(filepath.Join(t.TempDir(), "metrics.json"), context.Background())
	if err != nil {
		t.Fatalf("failed to create metrics store: %v", err)
	}
	defer store.Close()

	// Update with empty clientID (noop)
	if err := store.UpdateClientID("seq1", ""); err != nil {
		t.Errorf("unexpected error on empty clientID: %v", err)
	}

	// Buffer an event without clientID
	_ = store.RecordEvent("seq1", "", "init", map[string]any{"stage": "bootstrap"})

	if err := store.UpdateClientID("seq1", "client-updated"); err != nil {
		t.Errorf("UpdateClientID error: %v", err)
	}

	// sanitizeToolName
	name1 := sanitizeToolName("<arg1,arg2> foo [--cascade] [opt] bar")
	if name1 != "foo_bar" {
		t.Errorf("unexpected sanitized name: %s", name1)
	}

	longWithUnderscores := "command_with_a_very_long_name_that_exceeds_fifty_three_characters_limit_by_a_wide_margin"
	name2 := sanitizeToolName(longWithUnderscores)
	if len(name2) > 53 {
		t.Errorf("expected length <= 53, got %d: %s", len(name2), name2)
	}

	longSingle := "singlelongnamewithnounderscoresthatexceedsfiftythreecharacters"
	name3 := sanitizeToolName(longSingle)
	if len(name3) != 53 {
		t.Errorf("expected truncated length 53, got %d: %s", len(name3), name3)
	}

	// Client logToSandbox
	c := &Client{}
	c.logToSandbox("test format %s\n", "arg")
}

// 10. InstallToIDE Branches
func TestDeep13_InstallToIDE_Branches(t *testing.T) {
	logger := testMCPLogger()
	tmpDir := t.TempDir()

	// 1. Directory does not exist and !isIDEStdioAdapterConfig
	errNoDir := InstallToIDE("Claude Desktop", filepath.Join(tmpDir, "nonexistent", "mcp.json"), "/bin/zqk", tmpDir, logger)
	if errNoDir == nil {
		t.Error("expected error for non-existent directory")
	}

	// 2. Directory exists, but file has invalid json and is not foreign
	validDir := filepath.Join(tmpDir, "valid_dir")
	_ = os.MkdirAll(validDir, 0755)
	badJSONFile := filepath.Join(validDir, "mcp.json")
	_ = os.WriteFile(badJSONFile, []byte("{invalid-json"), 0644)
	errBadJSON := InstallToIDE("Claude Desktop", badJSONFile, "/bin/zqk", tmpDir, logger)
	if errBadJSON == nil {
		t.Error("expected error for invalid json")
	}

	// 3. Existing config with previous EnvFile
	prevConfig := mcpConfig{
		MCPServers: map[string]mcpServerConfig{
			brand.ExecutableName() + "-" + filepath.Base(tmpDir): {
				EnvFile: "custom.env",
			},
		},
	}
	data, _ := json.Marshal(prevConfig)
	prevFile := filepath.Join(validDir, "prev_mcp.json")
	_ = os.WriteFile(prevFile, data, 0644)
	errPrev := InstallToIDE("Claude Desktop", prevFile, "/bin/zqk", tmpDir, logger)
	if errPrev != nil {
		t.Errorf("InstallToIDE failed with previous config: %v", errPrev)
	}

	// 4. Install without projectRoot (empty)
	bareFile := filepath.Join(validDir, "bare_mcp.json")
	_ = InstallToIDE("Bare IDE", bareFile, "/bin/zqk", "", logger)
}

// 11. HandlersGraph Branches
func TestDeep13_HandlersGraph_Branches(t *testing.T) {
	ctx := context.Background()

	// HandleResolveReferences nil / empty
	_, err1 := HandleResolveReferences(ctx, nil)
	if err1 == nil {
		t.Error("expected error for nil args in HandleResolveReferences")
	}

	_, err2 := HandleResolveReferences(ctx, map[string]any{"references": []any{}})
	if err2 == nil {
		t.Error("expected error for empty references in HandleResolveReferences")
	}

	// HandleStateAwareQuery missing query_type
	_, err3 := HandleStateAwareQuery(ctx, map[string]any{})
	if err3 == nil {
		t.Error("expected error for missing query_type in HandleStateAwareQuery")
	}

	// HandleStateAwareQuery with disabled graph backend
	_, err4 := HandleStateAwareQuery(ctx, map[string]any{
		"query_type": "active_workstreams",
		"filters":    map[string]any{"status": "in_progress"},
	})
	if err4 == nil {
		t.Error("expected error for disabled graph backend in HandleStateAwareQuery")
	}
}

// 12. ClientEventContext Branches
func TestDeep13_ClientEventContext_Branches(t *testing.T) {
	s := NewServer()
	cec := s.getClientEventContext()

	// RecordEvent with empty sequenceID (noop)
	cec.sequenceID = ""
	cec.RecordEvent("test_event", map[string]any{"k": "v"})

	// RecordEventWithClientID with empty sequenceID (noop)
	cec.RecordEventWithClientID("test_event", "client-1", nil)

	// Set sequenceID but empty clientID
	cec.sequenceID = "seq-test"
	cec.clientID = ""
	cec.RecordEventWithClientID("test_event", "", nil)

	// RecordEventWithClientID with valid clientID
	cec.RecordEventWithClientID("test_event", "client-1", map[string]any{"action": "click"})

	// RecordEvent with valid sequenceID
	cec.RecordEvent("test_event_2", nil)
}

// 13. OntologyJSONLD Branches
func TestDeep13_OntologyJSONLD_Branches(t *testing.T) {
	rootCmd := &cobra.Command{Use: "zqk"}
	subCmd := &cobra.Command{
		Use:   "sub",
		Short: "sub command",
	}
	subCmd.Flags().String("visible", "", "visible flag")
	subCmd.Flags().String("hidden", "", "hidden flag")
	_ = subCmd.Flags().MarkHidden("hidden")
	rootCmd.AddCommand(subCmd)

	res, err := ConvertCommandTreeToJSONLD(rootCmd, true)
	if err != nil || res == nil {
		t.Fatalf("ConvertCommandTreeToJSONLD failed: %v", err)
	}
	if len(res.Commands) == 0 {
		t.Error("expected discovered commands")
	}
}

// 14. ResourceURIScheme MatchesPattern
func TestDeep13_ResourceURIScheme_MatchesPattern(t *testing.T) {
	r := NewResourceURISchemeResolver()
	if !r.matchesPattern("a/b/c/doc.md", "**/doc.md") {
		t.Error("expected match for **/")
	}
	if !r.matchesPattern("docs/arch/overview.md", "docs/**") {
		t.Error("expected match for /**")
	}
	if !r.matchesPattern("exact/path.md", "exact/path.md") {
		t.Error("expected exact match")
	}
	if !r.matchesPattern("prefix/path.md", "prefix/*") {
		t.Error("expected prefix match")
	}
	if !r.matchesPattern("part1-mid-part2", "part1-*-part2") {
		t.Error("expected middle glob match")
	}
	if r.matchesPattern("foo", "bar") {
		t.Error("expected mismatch")
	}
}

// 15. ServerHelpers MapValues and Sequence
func TestDeep13_ServerHelpers_MapValuesAndSequence(t *testing.T) {
	// mapValues
	mEmpty := map[string]int{}
	if vals := mapValues(mEmpty); vals != nil {
		t.Error("expected nil for empty mapValues")
	}
	mNonEmpty := map[string]int{"a": 1, "b": 2}
	if vals := mapValues(mNonEmpty); len(vals) != 2 {
		t.Errorf("expected 2 values, got %d", len(vals))
	}

	// mapValuesDeref
	mDerefEmpty := map[string]*int{}
	if vals := mapValuesDeref(mDerefEmpty); vals != nil {
		t.Error("expected nil for empty mapValuesDeref")
	}
	val1 := 10
	mDeref := map[string]*int{"a": &val1, "b": nil}
	if vals := mapValuesDeref(mDeref); len(vals) != 1 || vals[0] != 10 {
		t.Errorf("expected [10], got %v", vals)
	}

	// Server sequence ID helpers
	s := NewServer()
	seq1 := s.generateSequenceID()
	if seq1 == "" {
		t.Error("expected non-empty sequence ID")
	}
	seq2 := s.getOrCreateSequenceID()
	if seq2 == "" {
		t.Error("expected non-empty or created sequence ID")
	}
	seq3 := s.getOrCreateSequenceID()
	if seq3 != seq2 {
		t.Errorf("expected same sequence ID, got %s vs %s", seq2, seq3)
	}
}

// 16. PermissionFormat and PromptCount
func TestDeep13_PermissionFormat_AndPromptCount(t *testing.T) {
	s := NewServer()
	_ = s.generatePermissionFormatExamples()
	_ = s.getPermissionOperations()
	_ = s.getObjectKindsForExamples()

	// getPromptCount without cache
	count := s.getPromptCount()
	if count != 0 {
		t.Errorf("expected 0 prompt count, got %d", count)
	}

	// getPromptCount with cache
	s.RegisterPrompt("p1", "prompt 1", nil)
	_ = s.ListPrompts()
	countCached := s.getPromptCount()
	if countCached != 1 {
		t.Errorf("expected 1 prompt count with cache, got %d", countCached)
	}
}

// 17. RoleAware MinInt and Instructions
func TestDeep13_RoleAware_MinIntAndInstructions(t *testing.T) {
	if minInt(1, 2) != 1 || minInt(5, 3) != 3 {
		t.Error("minInt failed")
	}

	g := &RoleAwarePromptGenerator{}
	instr := g.generateAccessUpgradeInstructions()
	if instr != "" {
		t.Errorf("expected empty instructions, got %s", instr)
	}
}

// 18. ObjectsSpecLoaderAdapter
func TestDeep13_ObjectsSpecLoaderAdapter(t *testing.T) {
	adapterNil := &ObjectsSpecLoaderAdapter{Loader: nil}
	specNil, errNil := adapterNil.LoadSpecWithInheritance("foo")
	if errNil != nil || specNil != nil {
		t.Errorf("expected nil, nil for nil Loader: %v, %v", specNil, errNil)
	}

	specAdapterNil := &objectsSpecAdapter{spec: nil}
	if fields := specAdapterNil.GetResolvedFields(); fields != nil {
		t.Error("expected nil fields for nil spec")
	}

	specAdapterVal := &objectsSpecAdapter{spec: &objects.Spec{ResolvedFields: map[string]any{"key": "val"}}}
	if fields := specAdapterVal.GetResolvedFields(); fields == nil || fields["key"] != "val" {
		t.Errorf("expected fields with key=val, got %v", fields)
	}

	if sac := NewSpecAccessControlFromObjectsLoader(nil); sac != nil {
		t.Error("expected nil for nil loader in NewSpecAccessControlFromObjectsLoader")
	}
	if sac := NewSpecAccessControlFromObjectsLoader(&objects.SpecLoader{}); sac == nil {
		t.Error("expected non-nil SpecAccessControl")
	}
}

// 19. Server Serve Lifecycle
func TestDeep13_Server_ServeLifecycle(t *testing.T) {
	s := NewServer()
	go func() {
		time.Sleep(10 * time.Millisecond)
		s.shutdownCancel()
	}()
	_ = s.Serve()
	_ = s.IsShutdownRequested()
	_ = s.GetMCPMetrics()
	_ = s.GetMCPMetricsSnapshot()
	if negotiateProtocolVersion("") != "2025-06-18" {
		t.Error("expected default protocol version")
	}
	if negotiateProtocolVersion("2024-11-05") != "2024-11-05" {
		t.Error("expected echoed protocol version")
	}
	_ = s.GetCurrentSessionID()
}

