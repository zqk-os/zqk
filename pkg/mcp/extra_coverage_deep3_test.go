package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// 1. ServerInit comprehensive tests
func TestDeep3_ServerInit_Comprehensive(t *testing.T) {
	w := &SpecLoaderWrapper{}
	_, err := w.LoadSpecWithInheritance("item.yaml")
	if err == nil {
		t.Error("expected error for unimplemented wrapper")
	}
	if w.GetResolvedFields() != nil {
		t.Error("expected nil resolved fields")
	}

	s := NewServer()
	s.eventEmitter = NewEventEmitter(10)
	secCtx := pkgctx.NewSecurityContext("ACC-DEV", []string{"developer"}, []string{"read"})
	s.secCtx = secCtx

	pc, sac, err := InitializePermissionCache(s, nil, w, nil)
	if err != nil {
		t.Fatalf("unexpected error from InitializePermissionCache: %v", err)
	}
	if pc == nil || sac == nil {
		t.Fatal("expected non-nil pc and sac")
	}
}

// 2. ProjectContextTool comprehensive tests
func TestDeep3_ProjectContextTool_Comprehensive(t *testing.T) {
	s := NewServer()
	RegisterProjectContextTool(s)
	tool, ok := s.tools[GetToolName("get_project_context")]
	if !ok || tool.Handler == nil {
		t.Fatal("expected get_project_context tool registered")
	}
	_, _ = tool.Handler(context.Background(), nil)
}

// 3. ResourceURISchemeConfig comprehensive tests
func TestDeep3_ResourceURISchemeConfig_Comprehensive(t *testing.T) {
	cfg := &ResourceURISchemeRuleConfig{
		Pattern:     "*.md",
		Scheme:      "docs",
		Category:    "documentation",
		Description: "markdown files",
	}
	rule := cfg.ToResourceURISchemeRule()
	if rule.Pattern != "*.md" || rule.Scheme != "docs" {
		t.Errorf("unexpected rule: %+v", rule)
	}

	defaultRules := LoadResourceURISchemeRulesFromConfig(nil)
	if len(defaultRules) == 0 {
		t.Error("expected default rules for nil config")
	}

	sc := &ServerConfig{}
	sc.MCPServer.Resources.URISchemeRules = []ResourceURISchemeRuleConfig{*cfg}
	customRules := LoadResourceURISchemeRulesFromConfig(sc)
	if len(customRules) != 1 || customRules[0].Scheme != "docs" {
		t.Errorf("unexpected custom rules: %+v", customRules)
	}
}

// 4. SessionDisconnect comprehensive tests
func TestDeep3_SessionDisconnect_Comprehensive(t *testing.T) {
	s := NewServer()
	MarkSessionDisconnected(s) // empty sessionID returns immediately

	s.SetCurrentSessionID("session-999")
	MarkSessionDisconnected(s)
	time.Sleep(20 * time.Millisecond)
	if s.GetCurrentSessionID() != "" {
		t.Errorf("expected session cleared, got: %s", s.GetCurrentSessionID())
	}
}

// 5. MCPSpecApplier comprehensive tests
func TestDeep3_MCPSpecApplier_Comprehensive(t *testing.T) {
	s := NewServer()
	spec := ConvertPromptsToSpec()
	if spec == nil || spec.Name != "onboarding_prompts" {
		t.Fatalf("unexpected spec: %+v", spec)
	}

	err := ApplyMCPSpec(s, spec)
	if err != nil {
		t.Errorf("failed ApplyMCPSpec: %v", err)
	}

	err = ApplyMCPSpecs(s, []*MCPSpec{spec})
	if err != nil {
		t.Errorf("failed ApplyMCPSpecs: %v", err)
	}

	_ = LoadAndApplyMCPSpecs(s, "/nonexistent/path")
	_ = RegisterOnboardingPromptsFromSpec(s, "/nonexistent/path")
}

// 6. ReportTools comprehensive tests
func TestDeep3_ReportTools_Comprehensive(t *testing.T) {
	s := NewServer()
	RegisterReportTools(s)

	tools := []string{"report_pcs", "report_edd", "report_blockers"}
	for _, name := range tools {
		tool, ok := s.tools[GetToolName(name)]
		if !ok || tool.Handler == nil {
			t.Fatalf("expected tool %s registered", name)
		}
		_, _ = tool.Handler(context.Background(), map[string]any{objects.FieldKeyFormat: "json"})
		_, _ = tool.Handler(context.Background(), map[string]any{})
	}
}

// 7. VerificationTools comprehensive tests
func TestDeep3_VerificationTools_Comprehensive(t *testing.T) {
	s := NewServer()
	RegisterVerificationTools(s)

	res, err := s.handleAgentTriggerVerificationTool(context.Background(), map[string]any{
		objects.FieldKeyTarget: "nonexistent_target",
	})
	if err != nil || res == nil {
		t.Errorf("unexpected handleAgentTriggerVerificationTool result: %v, %v", res, err)
	}
}

// 8. ServerConfig comprehensive tests
func TestDeep3_ServerConfig_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".zqk", "mcp"), 0755)
	cfg := &ServerConfig{}
	cfg.MCPServer.IdleTimeout = "10m"

	err := SaveMCPConfig(tmpDir, cfg)
	if err != nil {
		t.Fatalf("failed to save MCP config: %v", err)
	}

	loaded, err := LoadMCPConfig(tmpDir)
	if err != nil || loaded == nil {
		t.Fatalf("failed to load MCP config: %v", err)
	}

	expandConfigSubstitutions(loaded, tmpDir, tmpDir)
	expanded := expandPathSubstitutions("${PROJECT_ROOT}/test.json", tmpDir, tmpDir)
	if expanded == "" {
		t.Error("expected non-empty expanded path")
	}

	envExp := expandEnvVars("${HOME}/test")
	if envExp == "" {
		t.Error("expected non-empty env expanded")
	}
}

// 9. ShutdownHooks comprehensive tests
func TestDeep3_ShutdownHooks_Comprehensive(t *testing.T) {
	m := NewShutdownHookManager()
	reg, exec, errs := m.GetShutdownHookStats()
	if reg != 0 || exec != 0 || errs != 0 {
		t.Errorf("expected 0 stats, got %d, %d, %d", reg, exec, errs)
	}

	if m.IsShutdownOrdered() {
		t.Error("expected shutdown not ordered initially")
	}

	called := false
	m.RegisterHook(func(ctx context.Context) error {
		called = true
		return nil
	})

	ctx := m.OrderShutdown()
	if ctx == nil || !m.IsShutdownOrdered() {
		t.Error("expected shutdown ordered")
	}
	if m.GetShutdownContext() == nil {
		t.Error("expected non-nil shutdown context")
	}

	notifyErrs := m.NotifyHooks()
	if len(notifyErrs) != 0 || !called {
		t.Errorf("expected hook called without errors: %v", notifyErrs)
	}
}

// 10. SchemaResources comprehensive tests
func TestDeep3_SchemaResources_Comprehensive(t *testing.T) {
	s := NewServer()
	RegisterSchemaResources(s)
	RegisterObjectSchemaResource(s, "backlog_item")
	_ = RegisterAllObjectSchemaResources(s)
	RegisterDefaultSchemaHandlers(s)
}

// 11. ServerMessaging comprehensive tests
func TestDeep3_ServerMessaging_Comprehensive(t *testing.T) {
	s := NewServer()
	if s.canSendNotifications() {
		t.Log("notifications capability checked")
	}
	_ = s.getQueueConfig()
	_ = s.SendMessageToClient("hello client", "info", "high")
	_ = s.SendMessageToClientByID("client-xyz", "hello", "info", "normal")
}

// 12. ObserverTools comprehensive tests
func TestDeep3_ObserverTools_Comprehensive(t *testing.T) {
	s := NewServer()
	RegisterObserverTools(s)

	if observerSearchLimit(map[string]any{"limit": 20}) != 20 {
		t.Error("expected 20")
	}
	if observerSearchLimit(map[string]any{}) <= 0 {
		t.Error("expected positive default limit")
	}

	tmpDir := t.TempDir()
	_, _ = HandleObserverSearch(context.Background(), tmpDir, map[string]any{"query": "status"})
	_, _ = s.handleObserverSearchTool(context.Background(), map[string]any{"query": "test"})
	_, _ = s.handleObserverRegisterTool(context.Background(), map[string]any{"name": "agent_alpha"})
}

// 13. RoleEnforcement comprehensive tests
func TestDeep3_RoleEnforcement_Comprehensive(t *testing.T) {
	enfs, ovrs, errs := GetRoleEnforcementStats()
	if enfs < 0 || ovrs < 0 || errs < 0 {
		t.Error("unexpected negative stats")
	}

	tmpDir := t.TempDir()
	cfg := &ServerConfig{}
	clientInfo := map[string]any{"name": "cursor"}
	enforced, err := enforceRoleEnforcement(clientInfo, cfg, tmpDir, nil)
	if err != nil || enforced == nil {
		t.Errorf("unexpected role enforcement: %v, %v", enforced, err)
	}

	_ = validateRolesAgainstSystem([]string{"admin"}, tmpDir)
	_, _ = getPermissionsFromRoles([]string{"admin"}, tmpDir)
	_, _, _ = getAccountRolesAndPermissions("ACC-1", tmpDir)
}

// 14. ObjectAccessControl comprehensive tests
func TestDeep3_ObjectAccessControl_Comprehensive(t *testing.T) {
	oac := NewObjectAccessControl(nil)
	secCtx := pkgctx.NewSecurityContext("ACC-DEV", []string{"developer"}, []string{"read:backlog_item"})

	obj := map[string]any{"id": "item-1", objects.FieldKeyKind: "backlog_item", "title": "First Item"}
	if !oac.HasAccess(obj, secCtx) {
		t.Error("expected access to object with kind permission")
	}

	filtered := oac.FilterObjectsByAccess([]map[string]any{obj}, secCtx)
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered object, got %d", len(filtered))
	}

	query := oac.AddAccessFilterToQuery(secCtx, map[string]any{"status": "open"})
	if query["status"] != "open" {
		t.Errorf("unexpected query: %v", query)
	}
}
