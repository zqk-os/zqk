package mcp

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// 1. Server lifecycle and hook registration tests
func TestDeep10_ServerLifecycle_ShutdownHook(t *testing.T) {
	s := NewServer()
	s.RegisterShutdownHook(func(ctx context.Context) error {
		return nil
	})

	if pgm := s.GetProcessGroupManager(); pgm != nil {
		t.Log("pgm is non-nil")
	}
}

// 2. ToolsCommon handlers (Get, Count, Status, Check)
func TestDeep10_ToolsCommon_HandleObjectGet(t *testing.T) {
	s := NewServer()
	RegisterCommonTools(s)
	ctx := context.Background()

	// Missing ID -> elicitation error
	_, err := HandleObjectGet(ctx, s, map[string]any{})
	if err == nil {
		t.Error("expected error on missing id")
	}

	// With full arguments
	_, _ = HandleObjectGet(ctx, s, map[string]any{
		objects.FieldKeyID:     "BLI-1",
		objects.FieldKeyFormat: "json",
		"view":                 "summary",
		"link_hydration":       "shallow",
		"fields":               []any{"title"},
	})

	// HandleObjectCount
	_, _ = HandleObjectCount(ctx, s, map[string]any{
		objects.FieldKeyKind:   "backlog_item",
		"filter":               "status=open",
		objects.FieldKeyFormat: "json",
	})

	// HandleSystemStatus
	_, _ = HandleSystemStatus(ctx, s, map[string]any{
		objects.FieldKeyFormat: "json",
	})

	// HandleSystemCheck
	_, _ = HandleSystemCheck(ctx, s, map[string]any{
		objects.FieldKeyID:     "CHK-SYS",
		objects.FieldKeyTier:   1,
		"auto_fix":             true,
		"force":                true,
		objects.FieldKeyFormat: "json",
	})
}

// 3. ToolsWorkflow handlers and lead plan extractors
func TestDeep10_ToolsWorkflow_Handlers(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	// Missing plan ID -> elicitation error
	_, err := HandleGetPriorityPlanItems(ctx, s, map[string]any{})
	if err == nil {
		t.Error("expected elicitation error for missing priority_plan_id")
	}

	// With plan ID
	_, _ = HandleGetPriorityPlanItems(ctx, s, map[string]any{
		"priority_plan_id":     "PRI-1",
		objects.FieldKeyFormat: "json",
	})

	// HandleGetCurrentBacklogItem
	_, _ = HandleGetCurrentBacklogItem(ctx, s, map[string]any{
		objects.FieldKeyFormat: "json",
	})

	// HandleGetNextBacklogItem
	_, _ = HandleGetNextBacklogItem(ctx, s, map[string]any{
		objects.FieldKeyFormat: "json",
	})

	// extractWhatsNextLeadPlan
	// 1. Direct map
	plan, ok, err := extractWhatsNextLeadPlan(map[string]any{
		"priority_plan": map[string]any{"id": "PRI-1"},
	})
	if err != nil || !ok || plan == nil {
		t.Errorf("expected plan from map: %v, %v, %v", plan, ok, err)
	}

	// 2. Data wrapper map
	plan, ok, err = extractWhatsNextLeadPlan(map[string]any{
		"data": map[string]any{
			"priority_plan": map[string]any{"id": "PRI-2"},
		},
	})
	if err != nil || !ok || plan == nil {
		t.Errorf("expected plan from data map: %v, %v, %v", plan, ok, err)
	}

	// 3. String json
	plan, ok, err = extractWhatsNextLeadPlan(`{"priority_plan":{"id":"PRI-3"}}`)
	if err != nil || !ok || plan == nil {
		t.Errorf("expected plan from string json: %v, %v, %v", plan, ok, err)
	}

	// 4. Invalid json string
	_, _, err = extractWhatsNextLeadPlan("invalid json {")
	if err == nil {
		t.Error("expected error for bad json in extractWhatsNextLeadPlan")
	}

	// 5. Unsupported type
	_, ok, _ = extractWhatsNextLeadPlan(999)
	if ok {
		t.Error("expected false for unsupported type in extractWhatsNextLeadPlan")
	}
}

// 4. ServerHandlersAuth accounts, permissions and strategy loading
func TestDeep10_ServerHandlersAuth_AccountsAndStrategies(t *testing.T) {
	s := NewServer()
	tmpDir := t.TempDir()
	s.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: tmpDir}
	ctx := context.Background()

	// extractRolesFromAccount
	roles := extractRolesFromAccount(map[string]any{
		"roles": []any{"developer", "admin"},
	})
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got: %d", len(roles))
	}

	// extractPermissionsFromAccount
	perms := extractPermissionsFromAccount(map[string]any{
		"permissions": []any{"read:*", "write:backlog_item"},
	})
	if len(perms) != 2 {
		t.Errorf("expected 2 perms, got: %d", len(perms))
	}

	// loadAccountByEmail (nonexistent)
	_, err := s.loadAccountByEmail("nonexistent@example.com", tmpDir)
	if err == nil {
		t.Error("expected error for nonexistent account email")
	}

	// loadEnabledAuthStrategies without root
	sNoRoot := NewServer()
	sNoRoot.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: ""}
	_, err = sNoRoot.loadEnabledAuthStrategies(ctx)
	if err == nil {
		t.Error("expected error without project root")
	}

	// loadEnabledAuthStrategies with root
	_, _ = s.loadEnabledAuthStrategies(ctx)
}

// 5. Proxy heartbeatLoop lifecycle test
func TestDeep10_Proxy_HeartbeatLoop(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	p := NewProxyDaemon("127.0.0.1:0", logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancelled to trigger select case <-ctx.Done()

	p.heartbeatLoop(ctx)
}

// 6. MCPSpecGenerator schema handlers and spec registration
func TestDeep10_MCPSpecGenerator_SchemaHandlers(t *testing.T) {
	s := NewServer()

	for _, name := range []string{"handleSchemaRegistry", "handleCLIOntology", "handleCommonFieldsSchema", "handleObjectSchema"} {
		sh := getSchemaHandlerByName(name)
		if sh == nil {
			t.Errorf("expected handler for %s", name)
		} else {
			_, _ = sh(s, "schema://object/backlog_item")
		}
	}

	if sh := getSchemaHandlerByName("nonexistent"); sh != nil {
		t.Error("expected nil for nonexistent schema handler")
	}

	gen := NewMCPSpecGenerator(s)

	// RegisterPrompt
	_ = gen.RegisterPrompt(PromptSpec{
		Name:        "deep10_prompt",
		Description: "Deep 10 test prompt",
		Arguments: []PromptArgSpec{
			{Name: "arg1", Description: "desc", Required: true},
		},
	})

	// registerResource
	_ = gen.registerResource(ResourceSpec{
		URI:         "res://basic",
		Name:        "basic_res",
		Description: "Basic resource",
		MimeType:    "text/plain",
	})
	_ = gen.registerResource(ResourceSpec{
		URI:         "res://meta",
		Name:        "meta_res",
		Description: "Meta resource",
		MimeType:    "text/plain",
		Category:    "docs",
		Priority:    "high",
		Tags:        []string{"tag1"},
		Metadata:    map[string]string{"k": "v"},
	})
}
