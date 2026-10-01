package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// 1. CommandResultBuilder tests
func TestDeep_CommandResultBuilder_Comprehensive(t *testing.T) {
	errCases := []struct {
		errStr   string
		expected int
	}{
		{"permission denied by policy", PermissionDenied},
		{"target not found in storage", NotFound},
		{"already exists in registry", AlreadyExists},
		{"invalid parameter passed", InvalidParameter},
		{"operation timed out", OperationTimeout},
		{"random unclassified error", InternalError},
	}

	for _, tc := range errCases {
		res := NewCommandResultBuilder("object get", []string{"id"}).WithError(errors.New(tc.errStr)).Build()
		if code, ok := res[cmdResultKeyErrorCode].(int); !ok || code != tc.expected {
			t.Errorf("for error %q expected code %d, got %v", tc.errStr, tc.expected, res[cmdResultKeyErrorCode])
		}
	}

	// Explicit error code
	resExplicit := NewCommandResultFromError("object get", []string{"id"}, errors.New("boom"), 999).Build()
	if resExplicit[cmdResultKeyErrorCode] != 999 {
		t.Errorf("expected explicit error code 999, got %v", resExplicit[cmdResultKeyErrorCode])
	}

	// Success builder
	successRes := NewCommandResultFromSuccess("object list", []string{"--format", "json"}, map[string]any{
		"objects": []any{"obj1"},
	}).Build()
	if successRes["success"] != true || successRes["command"] != "object list" {
		t.Errorf("unexpected successRes: %v", successRes)
	}

	// Direct inferErrorCodeFromError with nil
	bNil := NewCommandResultBuilder("test", nil)
	bNil.inferErrorCodeFromError()

	// Direct commandResultContainsAny
	if !commandResultContainsAny("Hello WORLD", []string{"world"}) {
		t.Error("expected contains match")
	}
	if commandResultContainsAny("Hello", []string{"foo", "bar"}) {
		t.Error("expected no match")
	}
}

// 2. ConfigSecurity tests
func TestDeep_ConfigSecurity_Comprehensive(t *testing.T) {
	// Nil config
	allowed, reason := isCommandAllowed("object list", nil, nil)
	if !allowed || reason != "" {
		t.Errorf("expected allowed for nil config, got %v (%s)", allowed, reason)
	}

	// Blocked command
	cfg := &ServerConfig{}
	cfg.MCPServer.BlockedCommands = []string{"system destroy", "system reboot*"}
	cfg.MCPServer.ExposedCommands = []string{"object *", "system status"}

	allowed, _ = isCommandAllowed("system destroy", cfg, nil)
	if allowed {
		t.Error("expected blocked command to be denied")
	}

	// Exposed whitelist match
	allowed, _ = isCommandAllowed("object list", cfg, nil)
	if !allowed {
		t.Error("expected object list to be allowed")
	}

	// Not in exposed commands
	allowed, _ = isCommandAllowed("workflow run", cfg, nil)
	if allowed {
		t.Error("expected unexposed command to be denied")
	}

	// Admin role bypasses exposed_commands whitelist
	adminCtx := pkgctx.NewSecurityContext("ACC-ADMIN", []string{"admin"}, []string{"read"})
	allowed, _ = isCommandAllowed("workflow run", cfg, adminCtx)
	if !allowed {
		t.Error("expected admin to bypass exposed_commands whitelist")
	}

	// Wildcard permission bypasses exposed_commands whitelist
	wildcardCtx := pkgctx.NewSecurityContext("ACC-WILD", []string{"user"}, []string{"*"})
	allowed, _ = isCommandAllowed("workflow run", cfg, wildcardCtx)
	if !allowed {
		t.Error("expected wildcard perm to bypass exposed_commands whitelist")
	}

	// Blocked still blocks admin
	allowed, _ = isCommandAllowed("system destroy", cfg, adminCtx)
	if allowed {
		t.Error("expected blocked command to still block admin")
	}

	// Write operation checks
	allowed, _ = isWriteOperationAllowed("object list", cfg, nil)
	if !allowed {
		t.Error("expected read operation allowed")
	}

	// normalizeCommandPath and matchesCommandPattern
	norm := normalizeCommandPath("zqk object list")
	if norm != "object list" {
		t.Errorf("unexpected normalized command path: %s", norm)
	}

	if !matchesCommandPattern("object list", "object *") {
		t.Error("expected pattern to match")
	}
	if matchesCommandPattern("system status", "object *") {
		t.Error("expected pattern not to match")
	}
}

// 3. CoordinatorIntegration tests
func TestDeep_CoordinatorIntegration(t *testing.T) {
	metrics := NewMCPMetrics()
	router := CreateMCPMetricsRouter(metrics)
	if router == nil {
		t.Fatal("expected non-nil router")
	}
	ctx := context.Background()
	_ = router.Emit(ctx, map[string]any{"event": "test"})
}

// 4. Elicitation tests
func TestDeep_Elicitation_Comprehensive(t *testing.T) {
	err := NewElicitationError("need input", []ElicitationParam{
		{Name: "account_id", Description: "Your account", Type: "string", Required: true},
	})
	if err.Error() != "need input" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	// ValidateToolParams
	valErr := ValidateToolParams(map[string]any{"name": "foo"}, []string{"name", "age"}, "createUser")
	if valErr == nil {
		t.Error("expected missing parameter validation error")
	}
	valOk := ValidateToolParams(map[string]any{"name": "foo", "age": 30}, []string{"name", "age"}, "createUser")
	if valOk != nil {
		t.Errorf("expected nil error when all params present: %v", valOk)
	}
}

// 5. ErrorCodes & ErrorResponseBuilder tests
func TestDeep_ErrorCodesAndBuilder_Comprehensive(t *testing.T) {
	// Categorize tests
	if !IsClientError(InvalidParameter) {
		t.Error("expected InvalidParameter to be client error")
	}
	if !IsServerError(ServerError) {
		t.Error("expected ServerError to be server error")
	}
	if !IsStandardError(ParseError) {
		t.Error("expected ParseError to be standard error")
	}
	if !IsCriticalErrorCode(ServerError) {
		t.Error("expected ServerError to be critical")
	}
	if !IsCriticalErrorCode(PermissionDenied) {
		t.Error("expected PermissionDenied to be critical client error")
	}
	if IsCriticalErrorCode(ParseError) {
		t.Error("expected ParseError not to be critical")
	}

	// Builders
	vErr := NewValidationError("email", "bad format", "ValidateUser")
	if vErr.Code != ValidationError {
		t.Errorf("unexpected code: %d", vErr.Code)
	}

	opErr := NewOperationFailedError("sync", "timeout")
	if opErr.Code != OperationFailed {
		t.Errorf("unexpected code: %d", opErr.Code)
	}

	srvErr := NewServerError("crash", map[string]any{"trace": "stack"})
	if srvErr.Code != ServerError {
		t.Errorf("unexpected code: %d", srvErr.Code)
	}

	notInit := NewNotInitializedError("listTools")
	if notInit.Code != NotInitialized {
		t.Errorf("unexpected code: %d", notInit.Code)
	}

	alrExists := NewAlreadyExistsError("account", "ACC-123")
	if alrExists.Code != AlreadyExists {
		t.Errorf("unexpected code: %d", alrExists.Code)
	}

	invState := NewInvalidStateError("session", "closed", "active")
	if invState.Code != InvalidState {
		t.Errorf("unexpected code: %d", invState.Code)
	}

	stTrans := NewStateTransitionError("job", "pending", "completed", "invalid jump")
	if stTrans.Code != StateTransitionError {
		t.Errorf("unexpected code: %d", stTrans.Code)
	}
}

// 6. Access Control helpers tests
func TestDeep_AccessControlHelpers(t *testing.T) {
	// updateObjectCount
	mFloat := map[string]any{"count": float64(10)}
	updateObjectCount(mFloat, 3)
	if mFloat["count"] != float64(3) {
		t.Errorf("expected 3.0, got %v", mFloat["count"])
	}

	mInt := map[string]any{"count": 10}
	updateObjectCount(mInt, 5)
	if mInt["count"] != 5 {
		t.Errorf("expected 5, got %v", mInt["count"])
	}

	// applyAccessControlToObjectsList with non-objects
	nonObjRes := map[string]any{"objects": "not a slice"}
	res := applyAccessControlToObjectsList(nonObjRes, nil, nil, nil)
	if res["objects"] != "not a slice" {
		t.Error("expected unchanged nonObjRes")
	}

	// applyAccessControlToSingleObject with non-object
	nonSingle := map[string]any{"object": "not a map"}
	resSingle := applyAccessControlToSingleObject(nonSingle, nil, nil, nil)
	if resSingle["object"] != "not a map" {
		t.Error("expected unchanged resSingle")
	}

	// applyAccessControlToDirectObject without id
	noID := map[string]any{"title": "test"}
	resDirect := applyAccessControlToDirectObject(noID, nil, nil, nil)
	if resDirect["title"] != "test" {
		t.Error("expected unchanged resDirect")
	}
}

// 7. ClientEventContext tests
func TestDeep_ClientEventContext(t *testing.T) {
	server := NewServer()
	server.currentSequenceID = "seq-abc"
	server.clientID = "client-1"

	cec := NewClientEventContext(server)
	if !cec.CanRecord() {
		t.Error("expected CanRecord to be true")
	}
	if cec.GetSequenceID() != "seq-abc" {
		t.Errorf("unexpected sequence ID: %s", cec.GetSequenceID())
	}

	cec.RecordDeprecatedFeature("old_api", map[string]any{"info": "test"})
	if cec.GetClientEventContextStats() == 0 {
		t.Error("expected positive stats")
	}
	cec.Reset()
	if cec.canRecord {
		t.Error("expected canRecord to reset to false")
	}
}

// 8. Additional ErrorResponseBuilder helpers
func TestDeep_ErrorResponseBuilder_Additional(t *testing.T) {
	e1 := NewAuthenticationFailedError("bad credentials", map[string]any{"source": "login"})
	if e1.Code != AuthenticationFailed {
		t.Errorf("expected AuthenticationFailed, got %d", e1.Code)
	}

	e2 := NewUnauthenticatedError("call")
	if e2.Code != Unauthenticated {
		t.Errorf("expected Unauthenticated, got %d", e2.Code)
	}

	e3 := NewInvalidFormatError("xml", "json only", "formatHandler")
	if e3.Code != InvalidFormat {
		t.Errorf("expected InvalidFormat, got %d", e3.Code)
	}

	e4 := NewInvalidParameterError("limit", "must be positive", "search")
	if e4.Code != InvalidParameter {
		t.Errorf("expected InvalidParameter, got %d", e4.Code)
	}

	e5 := NewPermissionDeniedError("delete", "requires admin")
	if e5.Code != PermissionDenied {
		t.Errorf("expected PermissionDenied, got %d", e5.Code)
	}

	e6 := NewAccessDeniedError("secret_obj", "restricted")
	if e6.Code != AccessDenied {
		t.Errorf("expected AccessDenied, got %d", e6.Code)
	}

	e7 := NewNotFoundError("workflow", "WF-123")
	if e7.Code != NotFound {
		t.Errorf("expected NotFound, got %d", e7.Code)
	}

	e8 := NewConfigurationError("missing key", map[string]any{"key": "auth_init"})
	if e8.Code != ConfigurationError {
		t.Errorf("expected ConfigurationError, got %d", e8.Code)
	}
}

// 9. EventEmitter Buffer Size & MCPSpec
func TestDeep_EventEmitterAndSpec(t *testing.T) {
	ee := NewEventEmitter(42)
	if ee.GetBufferSize() != 42 {
		t.Errorf("expected buffer size 42, got %d", ee.GetBufferSize())
	}

	spec := &MCPSpec{Name: "custom_spec"}
	if spec.GetName() != "custom_spec" {
		t.Errorf("expected custom_spec, got %s", spec.GetName())
	}
}

// 10. GraphConnectionManager & MCPMetrics Reset
func TestDeep_GraphManagerAndMetricsReset(t *testing.T) {
	gm := GetGraphConnectionManager()
	if gm != nil {
		_ = gm.IsEnabled()
		_, _ = gm.GetPool(context.Background())
	}

	metrics := NewMCPMetrics()
	metrics.RecordToolCall("test_tool", time.Millisecond, nil)
	metrics.Reset()
	if metrics.ToolCallCount.Load() != 0 {
		t.Errorf("expected 0 tool calls after reset, got %d", metrics.ToolCallCount.Load())
	}
}

// 11. MessageQueue & QueueStats
func TestDeep_MessageQueueStatsAndCoordinator(t *testing.T) {
	statsHealthy := QueueStats{Active: true, Errors: 0, QueueDepth: 5, MaxQueueSize: 10}
	if !statsHealthy.IsHealthy() {
		t.Error("expected stats to be healthy")
	}

	statsUnhealthy := QueueStats{Active: true, Errors: 2, QueueDepth: 5, MaxQueueSize: 10}
	if statsUnhealthy.IsHealthy() {
		t.Error("expected stats with errors to be unhealthy")
	}

	q := NewMessageQueue(nil, nil, QueueConfig{MaxQueueSize: 10})
	q.SetCoordinator(nil, context.Background())
	q.Stop()
}

// 12. Server Locking & ForgetWriterSubscription
func TestDeep_ForgetWriterSubscription(t *testing.T) {
	s := NewServer()
	s.forgetWriterSubscription("sub-1") // empty map

	s.writerSubscriptions[nil] = []string{"sub-1", "sub-2"}
	s.forgetWriterSubscription("sub-1")
	if len(s.writerSubscriptions[nil]) != 1 || s.writerSubscriptions[nil][0] != "sub-2" {
		t.Errorf("unexpected subscriptions: %v", s.writerSubscriptions[nil])
	}
	s.forgetWriterSubscription("sub-2")
	if _, ok := s.writerSubscriptions[nil]; ok {
		t.Error("expected writer entry deleted when empty")
	}
}

// 13. FeedSteerMCPWake
func TestDeep_FeedSteerMCPWake(t *testing.T) {
	if ResolveDaemonTCP("") != DefaultDaemonTCP {
		t.Errorf("expected DefaultDaemonTCP, got %s", ResolveDaemonTCP(""))
	}
	if ResolveDaemonTCP("127.0.0.1:12345") != "127.0.0.1:12345" {
		t.Errorf("expected custom addr, got %s", ResolveDaemonTCP("127.0.0.1:12345"))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled context
	res := ProbeFeedSteerMCPWake(ctx, "127.0.0.1:9999", "msg", "agent-1", "evt-1", nil)
	if res.SubscriberCount != -1 {
		t.Errorf("expected -1 subscribers on failed probe, got %d", res.SubscriberCount)
	}
}

// 14. ListExposedTools
func TestDeep_ListExposedTools(t *testing.T) {
	tools := ListExposedTools(nil, nil, nil, nil)
	if len(tools) == 0 {
		t.Log("no tools exposed with empty config/root")
	}
}

// 15. PromptBuilder
func TestDeep_PromptBuilder(t *testing.T) {
	b := NewPromptBuilder("test_prompt", "A test prompt").
		AddArgument("arg1", "optional arg").
		AddRequiredArgument("arg2", "required arg")

	prompt := b.Build()
	if prompt.Name != "test_prompt" || len(prompt.Arguments) != 2 {
		t.Fatalf("unexpected prompt: %v", prompt)
	}
	if prompt.Arguments[0].Required != promptArgOptional || prompt.Arguments[1].Required != promptArgRequired {
		t.Errorf("unexpected argument requirements: %v", prompt.Arguments)
	}

	server := NewServer()
	b.Register(server)
	if _, exists := server.prompts["test_prompt"]; !exists {
		t.Error("expected prompt to be registered on server")
	}
}

// 16. Protocol Notifications and Params Unmarshaling
func TestDeep_Protocol_NotificationAndParams(t *testing.T) {
	notif := &JSONRPCRequest{JSONRPC: "2.0", Method: "ping"}
	if !notif.IsNotification() {
		t.Error("expected IsNotification to be true when ID is nil")
	}

	call := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "ping"}
	if call.IsNotification() {
		t.Error("expected IsNotification to be false when ID is set")
	}

	emptyParamsReq := &JSONRPCRequest{}
	var target map[string]any
	if err := emptyParamsReq.UnmarshalParams(&target); err != nil {
		t.Errorf("expected nil error for empty params, got: %v", err)
	}
}

// 17. ExportPromptsToSpec
func TestDeep_ExportPromptsToSpec(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "exported", "prompts.yaml")
	err := ExportPromptsToSpec(outPath)
	if err != nil {
		t.Errorf("ExportPromptsToSpec failed: %v", err)
	}
}

// 18. PermissionCache setters & getters
func TestDeep_PermissionCache_Setters(t *testing.T) {
	pc := NewPermissionCache(nil)
	pc.SetLogger(nil)
	ee := NewEventEmitter(10)
	pc.SetEventEmitter(ee)
	if pc.GetEventEmitter() != ee {
		t.Errorf("expected %v, got %v", ee, pc.GetEventEmitter())
	}
	pc.SetMCPServerContext(nil)
	if pc.GetSpecLoader() != nil {
		t.Error("expected nil spec loader")
	}
}

// 19. ObjectAccessControl & ObjectsSpecLoaderAdapter
func TestDeep_ObjectAccessControl_Comprehensive(t *testing.T) {
	oac := NewObjectAccessControl(nil)
	if oac.HasAccess(map[string]any{"id": "OBJ-1"}, nil) {
		t.Error("expected HasAccess=false for nil security context")
	}

	filtered := oac.FilterObjectsByAccess([]map[string]any{{"id": "OBJ-1"}}, nil)
	if len(filtered) != 0 {
		t.Errorf("expected empty filtered list for nil secCtx, got %v", filtered)
	}

	queryFilter := oac.AddAccessFilterToQuery(nil, map[string]any{"status": "active"})
	if queryFilter[objects.FieldKeyID] == nil {
		t.Error("expected no_access filter when secCtx is nil")
	}

	adminCtx := pkgctx.NewSecurityContext("ACC-ADMIN", []string{"admin"}, []string{"*"})
	queryAdmin := oac.AddAccessFilterToQuery(adminCtx, map[string]any{"status": "active"})
	if queryAdmin["status"] != "active" {
		t.Error("expected unmodified query for admin")
	}

	// ObjectsSpecLoaderAdapter
	adapter := &ObjectsSpecLoaderAdapter{Loader: nil}
	spec, err := adapter.LoadSpecWithInheritance("backlog_item.yaml")
	if err != nil || spec != nil {
		t.Errorf("expected nil, nil for nil Loader: %v, %v", err, spec)
	}

	objAdapter := &objectsSpecAdapter{spec: nil}
	if objAdapter.GetResolvedFields() != nil {
		t.Error("expected nil resolved fields for nil spec")
	}

	if NewSpecAccessControlFromObjectsLoader(nil) != nil {
		t.Error("expected nil SpecAccessControl for nil loader")
	}
}

// 20. CLI ontology jsonld helper
func TestDeep_CLIOntology_ExtractGroup(t *testing.T) {
	grp := extractGroupFromPath("object list")
	if grp != "object" {
		t.Errorf("expected 'object', got '%s'", grp)
	}
}

// 21. ResourceLoader Text helpers
func TestDeep_ResourceLoader_TextHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.txt")
	_ = fileutil.WriteStandardFile(filePath, []byte("hello world"))

	rl := NewResourceLoader(tmpDir, NewResourceURISchemeResolver(), NewResourceMIMEAdapterRegistry())
	txt, err := rl.LoadResourceText("file://" + filePath)
	if err != nil || txt != "hello world" {
		t.Errorf("unexpected LoadResourceText result: %s, %v", txt, err)
	}

	txtMime, mime, err := rl.LoadResourceTextWithMimeType("file://"+filePath, "text/plain")
	if err != nil || txtMime != "hello world" || mime != "text/plain" {
		t.Errorf("unexpected LoadResourceTextWithMimeType result: %s, %s, %v", txtMime, mime, err)
	}
}

// 22. ResourceURIScheme Rule Config & Resolver
func TestDeep_ResourceURIScheme_ConfigAndRules(t *testing.T) {
	cfgRule := ResourceURISchemeRuleConfig{
		Pattern:     "*.md",
		Scheme:      "docs://",
		Category:    "documentation",
		Description: "Markdown docs",
	}
	r := cfgRule.ToResourceURISchemeRule()
	if r.Pattern != "*.md" || r.Scheme != "docs://" {
		t.Errorf("unexpected rule: %v", r)
	}

	resolver := NewResourceURISchemeResolverWithRules([]ResourceURISchemeRule{r})
	resolver.AddRule(ResourceURISchemeRule{
		Pattern:  "*.json",
		Scheme:   "data://",
		Category: "data",
	})
	if len(resolver.GetRules()) != 2 {
		t.Errorf("expected 2 rules, got %d", len(resolver.GetRules()))
	}
	resolver.SetRules([]ResourceURISchemeRule{r})
	if len(resolver.GetRules()) != 1 {
		t.Errorf("expected 1 rule, got %d", len(resolver.GetRules()))
	}
}

// 23. ServerConfig Path Expansion & SaveMCPConfig
func TestDeep_ServerConfig_PathExpansionAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	expanded := expandPathSubstitutions("${PROJECT_ROOT}/config/${MCP_CONFIG_DIR}", tmpDir, filepath.Join(paths.ProjectDataDir, "mcp"))
	if expanded != filepath.Join(tmpDir, "config", paths.ProjectDataDir, "mcp") {
		t.Errorf("unexpected expanded path: %s", expanded)
	}

	cfg := &ServerConfig{}
	cfg.MCPServer.IdleTimeout = "10m"
	_ = fileutil.MkdirAll(filepath.Dir(paths.MCPConfigPath(tmpDir)), paths.DirPerm755)
	err := SaveMCPConfig(tmpDir, cfg)
	if err != nil {
		t.Errorf("SaveMCPConfig failed: %v", err)
	}
}

// 24. Server Event Handlers
func TestDeep_ServerEventHandlers(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	// handleEventsList
	resList, err := s.handleEventsList(ctx, "cid", nil)
	if err != nil || resList == nil {
		t.Errorf("handleEventsList failed: %v, %v", err, resList)
	}

	// handleEventsUnsubscribe with valid json
	paramBytes, _ := json.Marshal(map[string]any{"subscriptionId": "sub-123"})
	resUnsub, err := s.handleEventsUnsubscribe(ctx, "cid", paramBytes)
	if err != nil || resUnsub == nil {
		t.Errorf("handleEventsUnsubscribe failed: %v, %v", err, resUnsub)
	}

	// handleEventsUnsubscribe with invalid json
	_, err = s.handleEventsUnsubscribe(ctx, "cid", []byte(`invalid json`))
	if err == nil {
		t.Error("expected error for invalid json in handleEventsUnsubscribe")
	}
}

// 25. RoleEnforcement Helpers
func TestDeep_RoleEnforcement_Helpers(t *testing.T) {
	_, err := getPermissionsFromRoles([]string{"role-1"}, "")
	if err == nil {
		t.Error("expected error for empty projectRoot in getPermissionsFromRoles")
	}

	err = validateRolesAgainstSystem([]string{"role-1"}, "")
	if err != nil {
		t.Errorf("expected nil error for empty projectRoot in validateRolesAgainstSystem: %v", err)
	}
}

// 26. RolePromptRenderer renderTemplate
func TestDeep_RolePromptRenderer_RenderTemplate(t *testing.T) {
	renderer := &RolePromptRenderer{}
	guidance := &RoleGuidance{
		Description:    "Test role",
		InfluenceLevel: "high",
		Permissions:    []string{"read:task", "write:bli"},
	}
	rendered := renderer.renderTemplate("Role: {{role}}, Desc: {{description}}, Perms: {{read_permissions}} / {{write_permissions}}", guidance, "architect", "prompt-1")
	if rendered == "" {
		t.Error("expected non-empty rendered template")
	}
}

// 27. CLI Bridge Registration & Execution with Context
func TestDeep_CLIBridge_RegistrationAndExecution(t *testing.T) {
	s := NewServer()
	err := RegisterCLITools(s, nil, t.TempDir())
	if err == nil {
		t.Error("expected error for RegisterCLITools without root command")
	}

	if getRootCommandForMCP() != nil {
		t.Error("expected nil root command for placeholder")
	}

	_, _ = executeCLICommandWithContext(context.Background(), map[string]any{"command": "help"}, nil, t.TempDir(), nil)
}

// 28. Schema Resources Registration
func TestDeep_SchemaResources_Registration(t *testing.T) {
	s := NewServer()
	RegisterObjectSchemaResource(s, "backlog_item")
	if _, exists := s.resources["schema://object/backlog_item"]; !exists {
		t.Error("expected schema resource to be registered")
	}
	_ = RegisterAllObjectSchemaResources(s)
}
