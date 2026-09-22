package mcp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// 1. ProxyDaemon comprehensive tests
func TestDeep2_ProxyDaemon_Comprehensive(t *testing.T) {
	var nilP *ProxyDaemon
	hSent, hFail, reconn := nilP.GetProxyDaemonStats()
	if hSent != 0 || hFail != 0 || reconn != 0 {
		t.Errorf("expected 0 stats for nil proxy, got %d, %d, %d", hSent, hFail, reconn)
	}

	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(context.Background()))
	proxy := NewProxyDaemon("127.0.0.1:0", logger)
	if proxy.GetTCPAddr() != "127.0.0.1:0" {
		t.Errorf("unexpected tcp addr: %s", proxy.GetTCPAddr())
	}
	if proxy.IsDaemonAlive() {
		t.Error("expected daemon not alive initially")
	}

	if proxy.ProbeDaemon(10 * time.Millisecond) {
		t.Error("expected probe to fail on unbound port")
	}
	if proxy.ProbeDaemonAndSync(10 * time.Millisecond) {
		t.Error("expected probe and sync to fail on unbound port")
	}

	var out bytes.Buffer
	proxy.stdout = &out
	proxy.replyDaemonUnavailable([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), nil)
	if out.Len() == 0 {
		t.Error("expected replyDaemonUnavailable to write response")
	}

	out.Reset()
	proxy.replyDaemonUnavailable([]byte(`{"jsonrpc":"2.0","method":"ping"}`), nil)
	if out.Len() != 0 {
		t.Error("expected no response for notification")
	}

	if isConnRefused(nil) {
		t.Error("expected false for nil error")
	}
	if !isConnRefused(errors.New("connection refused")) {
		t.Error("expected true for connection refused string")
	}
	if !isConnRefused(&net.OpError{Err: syscall.ECONNREFUSED}) {
		t.Error("expected true for syscall.ECONNREFUSED")
	}
	if !isConnRefused(&net.OpError{Err: &os.SyscallError{Err: syscall.ECONNREFUSED}}) {
		t.Error("expected true for nested syscall.ECONNREFUSED")
	}
	if isConnRefused(errors.New("something else")) {
		t.Error("expected false for generic error")
	}

	nonJSON := []byte("invalid json")
	if string(StampIDEInitializeClientInfo(nonJSON)) != "invalid json" {
		t.Error("expected nonJSON to return unmodified")
	}

	humanInit := []byte(`{"params":{"clientInfo":{"name":"claude"}}}`)
	resHuman := StampIDEInitializeClientInfo(humanInit)
	if !strings.Contains(string(resHuman), "claude") {
		t.Errorf("unexpected human init result: %s", string(resHuman))
	}

	genericInit := []byte(`{"params":{"clientInfo":{"name":"my-bot"}}}`)
	resStamped := StampIDEInitializeClientInfo(genericInit)
	if !strings.Contains(string(resStamped), IDEProxySubscriberClientID) {
		t.Errorf("expected stamped client ID, got: %s", string(resStamped))
	}

	emptyInit := []byte(`{"jsonrpc":"2.0"}`)
	resEmpty := StampIDEInitializeClientInfo(emptyInit)
	if !strings.Contains(string(resEmpty), IDEProxySubscriberClientID) {
		t.Errorf("expected stamped client ID for empty init, got: %s", string(resEmpty))
	}
}

// 2. LoggingChannel comprehensive tests
func TestDeep2_LoggingChannel_Comprehensive(t *testing.T) {
	startTime := time.Now()
	builder := NewLogMetricsBuilder(LogLevelInfo, "test msg", map[string]any{"k": "v"}, startTime).
		TransportReady(true).
		DeliveryMethod(logDeliveryMethodTraceFile).
		Success(true).
		Error("")
	built := builder.Build()
	if built["transport_ready"] != true || built["delivery_method"] != logDeliveryMethodTraceFile {
		t.Errorf("unexpected built metrics: %v", built)
	}

	s := NewServer()
	var traceBuf bytes.Buffer
	s.traceWriter = &traceBuf

	s.shutdownFlag.Store(1)
	err := s.SendLogMessage(LogLevelDebug, "shutdown debug", map[string]any{"foo": "bar"})
	if err != nil {
		t.Errorf("unexpected error on shutdown: %v", err)
	}
	if !strings.Contains(traceBuf.String(), "[MCP_DEBUG] shutdown debug") {
		t.Errorf("expected trace output, got: %s", traceBuf.String())
	}

	s.shutdownFlag.Store(0)
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	s.shutdownCtx = cancelCtx
	traceBuf.Reset()
	err = s.SendLogMessage(LogLevelWarn, "ctx cancelled", nil)
	if err != nil {
		t.Errorf("unexpected error on cancelled ctx: %v", err)
	}
	if !strings.Contains(traceBuf.String(), "[MCP_WARN] ctx cancelled") {
		t.Errorf("expected trace output, got: %s", traceBuf.String())
	}

	s.shutdownCtx = context.Background()
	traceBuf.Reset()
	_ = s.SendLogDebug("debug msg", map[string]any{"step": 1})
	_ = s.SendLogInfo("info msg", nil)
	_ = s.SendLogWarn("warn msg", nil)
	_ = s.SendLogError("error msg", map[string]any{"code": 500})
	_ = s.SendProgressLog("download", 0.75, "downloading files")
	if !strings.Contains(traceBuf.String(), "downloading files") {
		t.Errorf("expected progress log in traceBuf, got: %s", traceBuf.String())
	}
}

// 3. ServeCoordinator comprehensive tests
func TestDeep2_ServeCoordinator_Comprehensive(t *testing.T) {
	var nilSC *ServeCoordinator
	iter, tout := nilSC.GetServeCoordinatorStats()
	if iter != 0 || tout != 0 {
		t.Errorf("expected 0 for nil sc stats, got %d, %d", iter, tout)
	}

	s := NewServer()
	var traceBuf bytes.Buffer
	s.traceWriter = &traceBuf
	s.config = &ServerConfig{}
	s.config.MCPServer.IdleTimeout = "45s"

	proc := NewMessageProcessor(s, HandlerFunc(nil), NewDefaultTransport())
	lifecycle := NewServerLifecycleBuilder(s)
	sc := NewServeCoordinator(s, proc, lifecycle)

	sc.SetTimeoutManager(nil)

	iter, tout = sc.GetServeCoordinatorStats()
	if iter != 0 || tout != 0 {
		t.Errorf("expected 0 stats initially, got %d, %d", iter, tout)
	}

	td := sc.getTimeoutDuration()
	if td != "45s" {
		t.Errorf("expected 45s, got: %s", td)
	}

	sc.logWaitingState()
	sc.logReadResult(nil)
	sc.logReadResult(errors.New("read failed"))

	if sc.endConnectionOnly("test", nil) {
		t.Error("expected false when multiClient is false")
	}
	s.multiClient.Store(true)
	if !sc.endConnectionOnly("test", nil) {
		t.Error("expected true when multiClient is true")
	}

	s.multiClient.Store(false)
	var writerBuf bytes.Buffer
	w := bufio.NewWriter(&writerBuf)
	err := sc.handleReadError(io.EOF, context.Background(), w, true, &traceBuf)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF, got: %v", err)
	}

	err = sc.handleReadError(errors.New("write: broken pipe"), context.Background(), w, true, &traceBuf)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF for broken pipe, got: %v", err)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = sc.handleReadError(cancelCtx.Err(), cancelCtx, w, true, &traceBuf)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("expected nil or EOF on cancelled ctx, got: %v", err)
	}

	sc.resetServerState()
	sc.resetTraceWriter()
}

// 4. ClientMetrics comprehensive tests
func TestDeep2_ClientMetrics_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	metricsPath := filepath.Join(tmpDir, "client_metrics.json")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := NewClientMetricsStore(metricsPath, ctx)
	if err != nil {
		t.Fatalf("failed to create client metrics store: %v", err)
	}
	defer store.Close()

	rec, seq := store.GetClientMetricsStats()
	if rec != 0 || seq != 0 {
		t.Errorf("expected 0 stats initially, got %d, %d", rec, seq)
	}

	seqID := "seq-test-001"
	err = store.RecordEvent(seqID, "", "connect", map[string]any{"ip": "127.0.0.1"})
	if err != nil {
		t.Fatalf("failed to record connect event: %v", err)
	}
	err = store.RecordEvent(seqID, "client-A", "initialize", map[string]any{"client": "claude"})
	if err != nil {
		t.Fatalf("failed to record initialize event: %v", err)
	}
	err = store.RecordEvent(seqID, "client-A", "tools_list", map[string]any{"count": 10})
	if err != nil {
		t.Fatalf("failed to record tools_list event: %v", err)
	}

	_ = store.UpdateClientID(seqID, "client-A-updated")

	sm, err := store.GetSequenceMetrics(seqID)
	if err != nil || sm == nil {
		t.Fatalf("failed to get sequence metrics: %v", err)
	}
	if sm.TotalEvents != 3 {
		t.Errorf("expected 3 total events, got: %d", sm.TotalEvents)
	}

	all, err := store.GetAllMetrics()
	if err != nil || len(all) == 0 {
		t.Fatalf("expected at least 1 sequence in GetAllMetrics: %v", err)
	}

	byClient, err := store.GetMetricsByClientID("client-A")
	if err != nil || len(byClient) == 0 {
		t.Fatalf("expected metrics for client-A: %v", err)
	}

	anomSeq := &ClientSequenceMetrics{
		SequenceID:  "seq-anom",
		Initialized: false,
	}
	store.detectAnomalies(anomSeq, ClientSequenceEvent{
		EventType: "tools_list",
		Timestamp: time.Now(),
	})
	if len(anomSeq.Anomalies) == 0 {
		t.Error("expected anomaly for tools_list before initialize")
	}

	err = store.Save()
	if err != nil {
		t.Errorf("failed to save metrics: %v", err)
	}
	err = store.Load()
	if err != nil {
		t.Errorf("failed to load metrics: %v", err)
	}

	err = store.CompressMetrics(0)
	if err != nil {
		t.Errorf("failed to compress metrics: %v", err)
	}

	cloned := cloneClientSequenceMetrics(sm)
	if cloned.SequenceID != sm.SequenceID {
		t.Error("expected matching clone")
	}
	clonedMap := cloneClientSequenceMetricsMap(all)
	if len(clonedMap) != len(all) {
		t.Error("expected matching map clone")
	}
}

// 5. RoleGuidance comprehensive tests
type mockStorageProviderDeep2 struct {
	objs []map[string]any
}

func (m *mockStorageProviderDeep2) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
	return map[string]any{"objects": m.objs}, nil
}

func (m *mockStorageProviderDeep2) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}

func (m *mockStorageProviderDeep2) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	for _, obj := range m.objs {
		if obj["id"] == id {
			return obj, nil
		}
	}
	return nil, errors.New("not found")
}

func TestDeep2_RoleGuidance_Comprehensive(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSecurityContext("ACC-DEV", []string{"developer"}, []string{"read"})

	roleObj := map[string]any{
		"id":                        "role-dev",
		objects.FieldKeyDescription: "Developer",
		objects.FieldKeyPermissions: []any{"create:task", "update:bli"},
		"responsibilities": []any{
			map[string]any{
				objects.FieldKeyDescription:  "Write code",
				objects.FieldKeyCriteriaRefs: []any{"crit-code-quality"},
			},
		},
		"privileges":           []any{"create:task", "update:bli"},
		"access_upgrade_steps": []any{"request role elevation"},
		"prompt_templates": []any{
			map[string]any{
				objects.FieldKeyName:    "welcome",
				objects.FieldKeyContent: "Welcome to ZQK",
			},
		},
		"criteria_ids": []any{"crit-code-quality"},
	}

	storage := &mockStorageProviderDeep2{objs: []map[string]any{roleObj}}

	rl := &storageRoleLoader{storage: storage}
	loadedRole, err := rl.LoadRole(ctx, secCtx, "role-dev")
	if err != nil || loadedRole == nil {
		t.Fatalf("failed to load role: %v", err)
	}
	roles, err := rl.LoadRolesByIDs(ctx, secCtx, []string{"role-dev"})
	if err != nil || len(roles) == 0 {
		t.Fatalf("failed to load roles by IDs: %v", err)
	}

	critObj := map[string]any{"id": "crit-code-quality", "title": "High Quality"}
	storageCrit := &mockStorageProviderDeep2{objs: []map[string]any{critObj}}
	cl := &storageCriteriaLoader{storage: storageCrit}
	cObj, err := cl.LoadCriteria(ctx, secCtx, "crit-code-quality")
	if err != nil || cObj == nil {
		t.Fatalf("failed to load criteria: %v", err)
	}
	cObjs, err := cl.LoadCriteriaByIDs(ctx, secCtx, []string{"crit-code-quality"})
	if err != nil || len(cObjs) == 0 {
		t.Fatalf("failed to load criteria by IDs: %v", err)
	}

	nl := &noOpRoleLoader{}
	_, _ = nl.LoadRole(ctx, secCtx, "role-1")
	_, _ = nl.LoadRolesByIDs(ctx, secCtx, []string{"role-1"})

	nc := &noOpCriteriaLoader{}
	_, _ = nc.LoadCriteria(ctx, secCtx, "crit-1")
	_, _ = nc.LoadCriteriaByIDs(ctx, secCtx, []string{"crit-1"})

	gen := NewRoleGuidanceGenerator(storage)
	resps := gen.extractResponsibilities(ctx, secCtx, roleObj)
	if len(resps) == 0 {
		t.Error("expected extracted responsibilities")
	}
	privs := gen.extractPrivileges(roleObj)
	if len(privs) == 0 {
		t.Error("expected extracted privileges")
	}
	restrs := gen.extractRestrictions(roleObj)
	if len(restrs) == 0 {
		t.Error("expected extracted restrictions")
	}
	steps := gen.extractAccessUpgradeSteps(roleObj)
	if len(steps) == 0 {
		t.Error("expected extracted steps")
	}
	prompts := gen.extractPromptTemplates(roleObj)
	if len(prompts) == 0 {
		t.Error("expected extracted prompt templates")
	}

	guidance, err := gen.GenerateRoleGuidance(ctx, secCtx, "role-dev")
	if err != nil || guidance == nil {
		t.Fatalf("failed to generate role guidance: %v", err)
	}
	formatted := gen.FormatRoleGuidance(ctx, secCtx, guidance)
	if !strings.Contains(formatted, "role-dev") {
		t.Errorf("expected formatted guidance to contain role-dev, got: %s", formatted)
	}

	_ = AdaptStorageResult(nil)
	_ = AdaptStorageResult(map[string]any{"objects": []map[string]any{roleObj}})
}

// 6. ToolsSandbox comprehensive tests
func TestDeep2_ToolsSandbox_Comprehensive(t *testing.T) {
	if !targetsGuardedKernelDir(".zqk/process/bli.json") {
		t.Error("expected true for .zqk/process path")
	}
	if targetsGuardedKernelDir("pkg/mcp/proxy.go") {
		t.Error("expected false for pkg/mcp path")
	}

	_ = isPermittedKernelDirInspectionSegment("cat .zqk/config.json")
	_ = isGuardedKernelDirCommandPermitted("cat .zqk/config.json")

	if err := checkAgentShellGuard("echo hello", true); err != nil {
		t.Errorf("expected safe echo to pass, got: %v", err)
	}
	if err := checkAgentShellGuard("echo hello > .zqk/process/item.json", true); err == nil {
		t.Error("expected destructive guard check to fail")
	}

	p, c, err := requireSandboxPathAndContent(map[string]any{"path": "foo.txt", "content": "hello"})
	if err != nil || p != "foo.txt" || c != "hello" {
		t.Errorf("unexpected requireSandboxPathAndContent: %s, %s, %v", p, c, err)
	}
	_, _, err = requireSandboxPathAndContent(map[string]any{"path": ""})
	if err == nil {
		t.Error("expected error for missing content")
	}

	tmpDir := t.TempDir()
	root := fileSandboxRoot(tmpDir)
	if root == "" {
		t.Error("expected non-empty root")
	}

	resolved, err := resolveSandboxPath(tmpDir, "sub/file.txt")
	if err != nil || !strings.Contains(resolved, "sub/file.txt") {
		t.Errorf("unexpected resolved sandbox path: %s, %v", resolved, err)
	}
	_, err = resolveSandboxPath(tmpDir, "../../../etc/passwd")
	if err == nil {
		t.Error("expected traversal path to fail")
	}

	err = writeGuardedWorkspaceFileWithRoot(tmpDir, "file.txt", "sample-data")
	if err != nil {
		t.Fatalf("failed writeGuardedWorkspaceFileWithRoot: %v", err)
	}
	data, err := readGuardedWorkspaceFileWithRoot(tmpDir, "file.txt")
	if err != nil || string(data) != "sample-data" {
		t.Fatalf("failed readGuardedWorkspaceFileWithRoot: %s, %v", string(data), err)
	}

	s := NewServer()
	s.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: tmpDir}
	ctx := context.Background()

	if !s.isHighRiskBashCommand(ctx, "rm -rf /") {
		t.Error("expected rm -rf / to be high risk")
	}
	if s.isHighRiskBashCommand(ctx, "ls -la") {
		t.Error("expected ls -la not to be high risk")
	}

	RegisterAgentExecutionTools(s)

	_, _ = s.handleAgentReadFileTool(ctx, map[string]any{"path": "file.txt"})
	_, _ = s.handleAgentWriteFileTool(ctx, map[string]any{"path": "out.txt", "content": "test"})
	_, _ = s.handleAgentReadCodeTool(ctx, map[string]any{"path": "file.txt"})
	_, _ = s.handleAgentWriteCodeTool(ctx, map[string]any{"path": "out_code.txt", "content": "code"})
	_, _ = s.handleAgentExecuteBashTool(ctx, map[string]any{"command": "echo test"})
}

// 7. ResourceMIMEAdapters comprehensive tests
func TestDeep2_ResourceMIMEAdapters_Comprehensive(t *testing.T) {
	reg := NewResourceMIMEAdapterRegistry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	if reg.GetAdapter("text/markdown") == nil {
		t.Error("expected markdown adapter")
	}
	if reg.GetAdapter("text/plain") == nil {
		t.Error("expected text adapter")
	}
	if reg.GetAdapter("application/json") == nil {
		t.Error("expected json adapter")
	}
	if reg.GetAdapter("application/yaml") == nil {
		t.Error("expected yaml adapter")
	}

	tmpDir := t.TempDir()
	mdFile := filepath.Join(tmpDir, "test.md")
	_ = os.WriteFile(mdFile, []byte("# Markdown Title\nThis is a markdown description.\n"), 0644)

	txtFile := filepath.Join(tmpDir, "test.txt")
	_ = os.WriteFile(txtFile, []byte("Simple plain text file\n"), 0644)

	jsonFile := filepath.Join(tmpDir, "test.json")
	_ = os.WriteFile(jsonFile, []byte(`{"title":"JSON Title","description":"JSON Description"}`), 0644)

	yamlFile := filepath.Join(tmpDir, "test.yaml")
	_ = os.WriteFile(yamlFile, []byte("title: YAML Title\ndescription: YAML Description\n"), 0644)

	_ = reg.ExtractTitle(mdFile, "text/markdown")
	_ = reg.ExtractDescription(mdFile, "text/markdown")
	_ = reg.ExtractMetadata(mdFile, "text/markdown")

	_ = reg.ExtractTitle(txtFile, "text/plain")
	_ = reg.ExtractDescription(txtFile, "text/plain")
	_ = reg.ExtractMetadata(txtFile, "text/plain")

	_ = reg.ExtractTitle(jsonFile, "application/json")
	_ = reg.ExtractDescription(jsonFile, "application/json")
	_ = reg.ExtractMetadata(jsonFile, "application/json")

	_ = reg.ExtractTitle(yamlFile, "application/yaml")
	_ = reg.ExtractDescription(yamlFile, "application/yaml")
	_ = reg.ExtractMetadata(yamlFile, "application/yaml")

	if DetectMIMEType("file.md") != "text/markdown" {
		t.Errorf("unexpected mime for .md: %s", DetectMIMEType("file.md"))
	}
	if !strings.HasPrefix(DetectMIMEType("file.json"), "application/json") {
		t.Errorf("unexpected mime for .json: %s", DetectMIMEType("file.json"))
	}
	if !strings.HasPrefix(DetectMIMEType("file.yaml"), "application/yaml") {
		t.Errorf("unexpected mime for .yaml: %s", DetectMIMEType("file.yaml"))
	}
	if DetectMIMEType("file.unknown") != "application/octet-stream" {
		t.Errorf("unexpected mime for .unknown: %s", DetectMIMEType("file.unknown"))
	}
	if normalizeMIMEType("Text/Markdown; charset=utf-8") != "text/markdown" {
		t.Errorf("unexpected normalizeMIMEType: %s", normalizeMIMEType("Text/Markdown; charset=utf-8"))
	}
}

// 8. RoleAwarePrompts comprehensive tests
func TestDeep2_RoleAwarePrompts_Comprehensive(t *testing.T) {
	pCtx := &ProjectContext{Mission: &MissionContext{Title: "TestMCPProject", Statement: "Build great software"}}
	secCtx := pkgctx.NewSecurityContext("ACC-DEV", []string{"architect"}, []string{"read"})
	roleObj := map[string]any{"id": "architect", objects.FieldKeyDescription: "System Architect"}
	storage := &mockStorageProviderDeep2{objs: []map[string]any{roleObj}}
	guidanceGen := NewRoleGuidanceGenerator(storage)

	gen := NewRoleAwarePromptGenerator(pCtx, []string{"architect"}).
		WithRoleGuidanceGenerator(guidanceGen).
		WithSecurityContext(secCtx).
		WithAssigneePersona("Principal Architect")

	wBase, wQuick := gen.GenerateWelcomeMessage()
	if wBase == "" || wQuick == "" {
		t.Error("expected non-empty welcome message")
	}

	bp := gen.GenerateBigPicturePrompt()
	if !strings.Contains(bp, "TestMCPProject") {
		t.Errorf("expected TestMCPProject in big picture prompt: %s", bp)
	}

	ec := gen.GenerateExecutionContextPrompt()
	if ec == "" {
		t.Error("expected non-empty execution context prompt")
	}

	mr := gen.GenerateMyRolePrompt()
	if mr == "" {
		t.Error("expected non-empty role prompt")
	}

	formatted := formatPolicyBody("line 1\nline 2")
	if formatted == "" {
		t.Error("expected formatted policy body")
	}

	if minInt(3, 5) != 3 || minInt(8, 2) != 2 {
		t.Error("unexpected minInt result")
	}
}

// 9. MCPInterfaceAdapter comprehensive tests
func TestDeep2_MCPInterfaceAdapter_Comprehensive(t *testing.T) {
	s := NewServer()
	adapter := NewMCPServerAdapter(s)
	adapter.SetCoordinator(nil)
	ctx := context.Background()

	initRes, err := adapter.Initialize(ctx, &InitializeParams{
		ClientInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    "cursor",
			Version: "1.0",
		},
	})
	if err != nil || initRes == nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}

	_ = adapter.NotifyInitialized(ctx, &InitializedParams{})
	_, _ = adapter.ListRoots(ctx)
	_ = adapter.SendLogMessage(ctx, LogLevelInfo, "adapter log message", nil)
	_ = adapter.SendEvent(ctx, &Event{Type: "action.required"})
	_ = adapter.SendMessage(ctx, "test message", "chat", "normal")
	_ = adapter.NotifyCancelled(ctx, &CancelledParams{RequestID: "req-1"})
	_ = adapter.GetMetricsSnapshot()

	chTool := adapter.CallToolAsync(ctx, &ToolCallParams{Name: "unknown_tool"})
	<-chTool

	chList := adapter.ListToolsAsync(ctx)
	<-chList

	chRes := adapter.GetResourceAsync(ctx, &ResourceGetParams{URI: "zqk://invalid"})
	<-chRes

	chListRes := adapter.ListResourcesAsync(ctx, &ResourcesListParams{})
	<-chListRes

	chPrompt := adapter.GetPromptAsync(ctx, &PromptGetParams{Name: "unknown_prompt"})
	<-chPrompt

	chListPrompts := adapter.ListPromptsAsync(ctx)
	<-chListPrompts

	chBatch := adapter.CallToolsBatch(ctx, []*ToolCallParams{{Name: "t1"}})
	<-chBatch

	chBatchRes := adapter.GetResourcesBatch(ctx, []*ResourceGetParams{{URI: "u1"}})
	<-chBatchRes
}

type mockSpecLoaderDeep2 struct{}

func (m *mockSpecLoaderDeep2) LoadSpecWithInheritance(filename string) (Spec, error) {
	return nil, errors.New("not found")
}

// 10. PermissionCache comprehensive tests
func TestDeep2_PermissionCache_Comprehensive(t *testing.T) {
	pc := NewPermissionCache(&mockSpecLoaderDeep2{})
	pc.SetLogger(nil)
	pc.SetEventEmitter(nil)
	pc.SetMCPServerContext(nil)

	if pc.GetEventEmitter() != nil {
		t.Error("expected nil event emitter")
	}
	if pc.GetSpecLoader() == nil || pc.GetSpecLoaderTyped() == nil {
		t.Error("expected non-nil spec loader")
	}

	hits, misses, inv := pc.GetPermissionCacheStats()
	if hits != 0 || misses != 0 || inv != 0 {
		t.Errorf("expected 0 stats, got %d, %d, %d", hits, misses, inv)
	}

	secCtx := pkgctx.NewSecurityContext("ACC-USER1", []string{"developer"}, []string{"read", "write"})
	_ = pc.BuildPermissionCache(secCtx)
	_ = pc.ActivateUser("ACC-USER1")
	if !pc.IsUserActive("ACC-USER1") {
		t.Error("expected user active")
	}
	_ = pc.DeactivateUser("ACC-USER1")
	if pc.IsUserActive("ACC-USER1") {
		t.Error("expected user inactive")
	}
	_ = pc.RefreshUserPermissions(secCtx)
	_, _ = pc.GetUserPermissions("ACC-USER1")

	pc.InvalidateUserCache("ACC-USER1")
	pc.InvalidateObjectAccess("OBJ-123")
	pc.ClearCache()

	tags := extractPrivilegeTags(map[string]any{
		objects.FieldKeyTags: []any{"access:team-alpha", "privilege:confidential"},
	})
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got: %v", tags)
	}

	accessPerms := extractAccessPermissions([]string{"access:read", "write", "access:admin"})
	if len(accessPerms) == 0 {
		t.Error("expected extracted access perms")
	}

	rules := extractFieldAccessRules(map[string]any{
		"access": map[string]any{
			"read": map[string]any{
				objects.FieldKeyRoles: []any{"developer"},
			},
		},
	})
	if rules == nil {
		t.Error("expected extracted field access rules")
	}
}

// 11. ResourceBuilder and ResourceLoader comprehensive tests
func TestDeep2_ResourceBuilderAndLoader_Comprehensive(t *testing.T) {
	rb := NewResourceBuilder("zqk://project/config", "Config", "System config", "application/json")
	res := rb.WithCategory("system").
		WithPriority("high").
		AddTag("core").
		AddTags("mcp", "test").
		WithMetadata("env", "prod").
		WithMetadataMap(map[string]string{"version": "1.0"}).
		Build()

	if res.URI != "zqk://project/config" {
		t.Errorf("unexpected URI: %s", res.URI)
	}
	if res.Name != "Config" || res.MimeType != "application/json" {
		t.Errorf("unexpected resource fields: %+v", res)
	}

	s := NewServer()
	rb.Register(s)

	tmpDir := t.TempDir()
	rl := NewResourceLoader(tmpDir, nil, nil)
	scheme, path, err := rl.parseAndValidateURI("file:///test.txt")
	if err != nil || scheme != "file" || path != "/test.txt" {
		t.Errorf("unexpected parseAndValidateURI: %s, %s, %v", scheme, path, err)
	}

	schemes := rl.getSupportedSchemes()
	if !schemes["file"] {
		t.Error("expected file scheme supported")
	}

	resolved := rl.resolveFilePath("test.txt")
	if resolved == "" {
		t.Error("expected non-empty resolved path")
	}

	mime := rl.detectMIMEType("test.json")
	if !strings.HasPrefix(mime, "application/json") {
		t.Errorf("unexpected mime: %s", mime)
	}
}

// 12. CLIBridge comprehensive tests
func TestDeep2_CLIBridge_Comprehensive(t *testing.T) {
	_ = GetCLIBridgeStats()

	if !isUnsafeCLIBridgeBinary("mcp.test") {
		t.Error("expected mcp.test to be unsafe")
	}
	if !isUnsafeCLIBridgeBinary("") {
		t.Error("expected empty string to be unsafe")
	}
	if isUnsafeCLIBridgeBinary("/usr/local/bin/zqk") {
		t.Error("expected zqk path not to be unsafe")
	}

	sanitized := sanitizeToolName("object list")
	if sanitized != "object_list" {
		t.Errorf("unexpected sanitized tool name: %s", sanitized)
	}

	cmd := &DiscoveredCommand{
		Path:        "test_cmd",
		Short:       "Test command description",
		Permissions: []string{"read"},
		Roles:       []string{"user"},
	}
	tool := ConvertCommandToMCPTool(cmd)
	if tool.Name == "" || tool.Description == "" {
		t.Error("expected valid tool converted from command")
	}

	secCtx := pkgctx.NewSecurityContext("ACC-DEV", []string{"user"}, []string{"read"})
	filtered := FilterCommandsByPermissions([]*DiscoveredCommand{cmd}, secCtx)
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered command, got %d", len(filtered))
	}

	rootCmd := &cobra.Command{Use: "zqk"}
	subCmd := &cobra.Command{Use: "status", Short: "System status"}
	rootCmd.AddCommand(subCmd)
	discovered := DiscoverCLICommands(rootCmd)
	if len(discovered) == 0 {
		t.Error("expected discovered commands")
	}
}
