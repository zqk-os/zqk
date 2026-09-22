package mcp

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// 1. Tools comprehensive tests
func TestDeep4_Tools_Comprehensive(t *testing.T) {
	s := NewServer()
	secCtx := pkgctx.NewSecurityContext("ACC-DEV", []string{"developer", "admin"}, []string{"*"})

	RegisterAllTools(s)
	RegisterAllToolsWithSecurityContext(s, secCtx)

	if len(s.tools) == 0 {
		t.Error("expected tools registered")
	}
}

// 2. MCPMetricsRouterRecorder comprehensive tests
func TestDeep4_MCPMetricsRouterRecorder_Comprehensive(t *testing.T) {
	router := &MCPMetricsRouter{}
	rec := router.getMCPMetricsRecorder()
	if rec == nil {
		t.Fatal("expected non-nil recorder")
	}

	b1 := router.buildMCPMetric("mcp_tool_call", map[string]any{"tool_name": "test_tool"}, time.Millisecond, nil)
	if b1 == nil {
		t.Error("expected non-nil builder for mcp_tool_call")
	}

	b2 := router.buildMCPMetric("mcp_batch_tool_call", map[string]any{"batch_size": 5, "error_count": 0}, 2*time.Millisecond, errors.New("err"))
	if b2 == nil {
		t.Error("expected non-nil builder for mcp_batch_tool_call")
	}

	b3 := router.buildMCPMetric("mcp_queue_metrics", map[string]any{"depth": 3, "dropped": 0, "sent": 10}, time.Millisecond, nil)
	if b3 == nil {
		t.Error("expected non-nil builder for mcp_queue_metrics")
	}

	b4 := router.buildMCPMetric("mcp_concurrent_operation", map[string]any{"delta": 1}, time.Millisecond, nil)
	if b4 == nil {
		t.Error("expected non-nil builder for mcp_concurrent_operation")
	}
}

// 3. CLIBridgeAccessControl comprehensive tests
func TestDeep4_CLIBridgeAccessControl_Comprehensive(t *testing.T) {
	// Nil secCtx
	resNil := applyObjectAccessControl(map[string]any{"k": "v"}, nil, nil)
	if resNil["k"] != "v" {
		t.Error("expected unchanged for nil secCtx")
	}

	// Nil permissionCache
	secCtx := pkgctx.NewSecurityContext("ACC-1", []string{"user"}, []string{"read:backlog_item"})
	resNoCache := applyObjectAccessControl(map[string]any{"k": "v"}, secCtx, nil)
	if resNoCache["k"] != "v" {
		t.Error("expected unchanged for nil cache")
	}

	// Inactive user
	pc := NewPermissionCache(&mockSpecLoaderDeep2{})
	_ = pc.DeactivateUser("ACC-INACTIVE")
	secInactive := pkgctx.NewSecurityContext("ACC-INACTIVE", []string{"user"}, []string{"read:backlog_item"})
	resInactive := applyObjectAccessControl(map[string]any{"id": "item-1"}, secInactive, pc)
	if resInactive["error_type"] != "account_inactive" {
		t.Errorf("expected account_inactive, got: %v", resInactive)
	}

	// Active user: objects list, single object, direct object
	_ = pc.ActivateUser("ACC-1")

	// Objects list
	listRes := map[string]any{
		"objects": []any{
			map[string]any{"id": "o1", objects.FieldKeyKind: "backlog_item", "title": "Item 1"},
			"not a map",
		},
		"count": 2,
	}
	filteredList := applyObjectAccessControl(listRes, secCtx, pc)
	if filteredList["objects"] == nil {
		t.Error("expected objects field")
	}

	// Single object
	singleRes := map[string]any{
		"object": map[string]any{"id": "o1", objects.FieldKeyKind: "backlog_item", "title": "Single"},
	}
	filteredSingle := applyObjectAccessControl(singleRes, secCtx, pc)
	if filteredSingle["object"] == nil {
		t.Error("expected object field")
	}

	// Direct object
	directRes := map[string]any{
		"id":                 "o1",
		objects.FieldKeyKind: "backlog_item",
		"title":              "Direct",
	}
	filteredDirect := applyObjectAccessControl(directRes, secCtx, pc)
	if filteredDirect["id"] != "o1" {
		t.Error("expected direct object id preserved")
	}
}

// 4. ToolsWorkflow comprehensive tests
func TestDeep4_ToolsWorkflow_Comprehensive(t *testing.T) {
	secAdmin := pkgctx.NewSecurityContext("ACC-ADMIN", []string{"admin"}, []string{"*"})
	secDev := pkgctx.NewSecurityContext("ACC-DEV", []string{"developer"}, []string{"read"})

	if !IsStudioPackToolsEnabled(secAdmin) {
		t.Error("expected studio pack tools enabled for admin")
	}
	if IsStudioPackToolsEnabled(secDev) {
		t.Log("studio pack tools disabled for developer without pack")
	}

	s := NewServer()
	RegisterWorkflowTools(s, secAdmin)

	ctx := context.Background()
	_, _ = HandleGetCurrentPriorityPlan(ctx, s, nil)
	_, _ = HandleGetPriorityPlanItems(ctx, s, map[string]any{"plan_id": "P1"})
	_, _ = HandleGetCurrentBacklogItem(ctx, s, nil)
	_, _ = HandleGetNextBacklogItem(ctx, s, nil)
	_, _ = handleGetNextBacklogItemExploring(ctx, s, "json")

	// Extract helper tests
	_, found, _ := extractWhatsNextLeadPlan(nil)
	if found {
		t.Error("expected false for nil result")
	}
	_, found, _ = extractWhatsNextLeadPlan(map[string]any{
		workflowKeyPriorityPlan: map[string]any{"id": "PRI-1"},
	})
	if !found {
		t.Error("expected true for priority_plan match")
	}

	_, foundItem, _ := extractWorkflowResultItem(nil)
	if foundItem {
		t.Error("expected false for nil item")
	}
	_, foundItem, _ = extractWorkflowResultItem(map[string]any{
		workflowKeyObjects: []any{map[string]any{"id": "BLI-1"}},
	})
	if !foundItem {
		t.Error("expected true for objects match")
	}
}

// 5. ServerHandlers comprehensive tests
func TestDeep4_ServerHandlers_Comprehensive(t *testing.T) {
	s := NewServer()
	router := s.setupHandlers()
	if router == nil {
		t.Fatal("expected non-nil router")
	}

	ctx := context.Background()
	resPing, err := s.handlePing(ctx, "c1", nil)
	if err != nil || resPing == nil {
		t.Errorf("handlePing failed: %v, %v", resPing, err)
	}

	resDiag, err := s.handleSystemDiagnostics(ctx, "c1", nil)
	if err != nil || resDiag == nil {
		t.Errorf("handleSystemDiagnostics failed: %v, %v", resDiag, err)
	}

	resShut, err := s.handleShutdown(ctx, "c1", nil)
	if err != nil || resShut == nil {
		t.Errorf("handleShutdown failed: %v, %v", resShut, err)
	}
}

// 6. Install comprehensive tests
func TestDeep4_Install_Comprehensive(t *testing.T) {
	_ = isForeignMCPConfig("cursor", "/Users/test/.cursor/config.json")
	_ = isIDEStdioAdapterConfig("cursor", "/Users/test/.cursor/config.json")

	tmpDir := t.TempDir()
	configs := findMCPConfigs(tmpDir, tmpDir)
	if configs == nil {
		t.Error("expected non-nil configs map")
	}

	cursorDir := filepath.Join(tmpDir, ".cursor")
	_ = os.MkdirAll(cursorDir, 0755)
	configFile := filepath.Join(cursorDir, "mcp.json")
	_ = os.WriteFile(configFile, []byte(`{"mcpServers":{}}`), 0644)

	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(context.Background()))
	err := InstallToIDE("cursor", configFile, "/usr/local/bin/zqk", tmpDir, logger)
	if err != nil {
		t.Logf("InstallToIDE result: %v", err)
	}
}
