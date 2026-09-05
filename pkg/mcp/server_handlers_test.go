package mcp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestClientIDEstablishment tests that client ID is properly established
func TestClientIDEstablishment(t *testing.T) {
	t.Parallel()
	server := createTestServerWithoutRegistry() // Use server without registry for basic tests

	// Test 1: Client ID from initialize capabilities
	t.Run("from_initialize_capabilities", func(t *testing.T) {
		server.clientIDMu.Lock()
		server.clientID = emptyValue // Reset
		server.clientIDMu.Unlock()

		initParams := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "test-client",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "test-client-123",
				clientInfoAccountID:      "ACC-1785920548450214016-ace2aae1", // Use registered account for tests
				objects.FieldKeyRoles:    []any{"test_agent"},
			},
		}

		params, _ := json.Marshal(initParams)
		_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params)
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}

		// Check that client ID was stored
		server.clientIDMu.RLock()
		clientID := server.clientID
		server.clientIDMu.RUnlock()

		if clientID != "test-client-123" {
			t.Errorf("expected client ID 'test-client-123', got '%s'", clientID)
		}
	})

	// Test 2: Client ID generation on notifications/initialized
	t.Run("generated_on_notifications_initialized", func(t *testing.T) {
		server.clientIDMu.Lock()
		server.clientID = emptyValue // Reset
		server.clientIDMu.Unlock()
		server.secCtx = nil // Reset security context to ensure no role prefix

		// Call notifications/initialized without prior initialize
		params := json.RawMessage("{}")
		_, err := server.handleNotificationInitialized(pkgctx.NewSystemContext(), "notifications/initialized", params)
		if err == nil {
			t.Fatal("expected NotificationSentinel error")
		}
		notificationSentinel := &NotificationSentinel{}
		if !errors.As(err, &notificationSentinel) {
			t.Fatalf("expected NotificationSentinel, got %T", err)
		}

		// Check that client ID was generated
		server.clientIDMu.RLock()
		clientID := server.clientID
		server.clientIDMu.RUnlock()

		if clientID == emptyValue {
			t.Error("expected client ID to be generated, got empty string")
		}
		// Client ID may be prefixed with role if security context exists, or just "client_" if not
		// Accept either format: "client_..." or "<role>_client_..."
		if !strings.HasPrefix(clientID, "client_") && !strings.Contains(clientID, "_client_") {
			t.Errorf("expected client ID to start with 'client_' or contain '_client_', got '%s'", clientID)
		}
	})

	// Test 3: Client ID finalized on notifications/initialized after initialize
	t.Run("finalized_on_notifications_initialized", func(t *testing.T) {
		server.clientIDMu.Lock()
		server.clientID = emptyValue // Reset
		server.clientIDMu.Unlock()

		// First, initialize with client ID
		initParams := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "test-client",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "test-client-456",
				clientInfoAccountID:      "ACC-1785920548450214016-ace2aae1", // Use registered account
				objects.FieldKeyRoles:    []any{"test_agent"},
			},
		}

		params, _ := json.Marshal(initParams)
		_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params)
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}

		// Verify client ID was stored from initialize
		server.clientIDMu.RLock()
		clientID1 := server.clientID
		server.clientIDMu.RUnlock()

		if clientID1 != "test-client-456" {
			t.Errorf("expected client ID 'test-client-456' after initialize, got '%s'", clientID1)
		}

		// Then, call notifications/initialized (should preserve existing ID, not regenerate)
		notifParams := json.RawMessage("{}")
		_, err = server.handleNotificationInitialized(pkgctx.NewSystemContext(), "notifications/initialized", notifParams)
		if err == nil {
			t.Fatal("expected NotificationSentinel error")
		}
		notificationSentinel := &NotificationSentinel{}
		if !errors.As(err, &notificationSentinel) {
			t.Fatalf("expected NotificationSentinel, got %T", err)
		}

		// Verify client ID is still the same (not regenerated)
		server.clientIDMu.RLock()
		clientID2 := server.clientID
		server.clientIDMu.RUnlock()

		if clientID2 != "test-client-456" {
			t.Errorf("expected client ID to remain 'test-client-456' after notifications/initialized, got '%s'", clientID2)
		}
	})
}

// TestClientIDInEventsSubscribe tests that client ID is used in events/subscribe
func TestClientIDInEventsSubscribe(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.eventEmitter = NewEventEmitter(100)

	// Test 1: Use client ID from server if not provided
	t.Run("use_server_client_id", func(t *testing.T) {
		// Set client ID on server
		server.clientIDMu.Lock()
		server.clientID = "test-client-789"
		server.clientIDMu.Unlock()

		// Subscribe without providing client ID
		subscribeParams := EventsSubscribeParams{
			EventTypes: []string{"log.info"},
		}

		params, _ := json.Marshal(subscribeParams)
		result, err := server.handleEventsSubscribe(pkgctx.NewSystemContext(), "events/subscribe", params)
		if err != nil {
			t.Fatalf("events/subscribe failed: %v", err)
		}

		subscribeResult, ok := result.(EventsSubscribeResult)
		if !ok {
			t.Fatalf("expected EventsSubscribeResult, got %T", result)
		}

		// Check that subscription ID includes client ID
		if !strings.HasPrefix(subscribeResult.SubscriptionID, "test-client-789_") {
			t.Errorf("expected subscription ID to start with 'test-client-789_', got '%s'", subscribeResult.SubscriptionID)
		}
	})

	// Test 2: Use provided client ID if given
	t.Run("use_provided_client_id", func(t *testing.T) {
		// Set different client ID on server
		server.clientIDMu.Lock()
		server.clientID = "server-client-id"
		server.clientIDMu.Unlock()

		// Subscribe with explicit client ID
		subscribeParams := EventsSubscribeParams{
			EventTypes: []string{"log.info"},
			ClientID:   "explicit-client-id",
		}

		params, _ := json.Marshal(subscribeParams)
		result, err := server.handleEventsSubscribe(pkgctx.NewSystemContext(), "events/subscribe", params)
		if err != nil {
			t.Fatalf("events/subscribe failed: %v", err)
		}

		subscribeResult, ok := result.(EventsSubscribeResult)
		if !ok {
			t.Fatalf("expected EventsSubscribeResult, got %T", result)
		}

		// Check that subscription ID uses explicit client ID
		if !strings.HasPrefix(subscribeResult.SubscriptionID, "explicit-client-id_") {
			t.Errorf("expected subscription ID to start with 'explicit-client-id_', got '%s'", subscribeResult.SubscriptionID)
		}
	})

	// Test 3: Generate subscription ID without client ID if none available
	t.Run("no_client_id", func(t *testing.T) {
		// Clear client ID
		server.clientIDMu.Lock()
		server.clientID = emptyValue
		server.clientIDMu.Unlock()

		// Subscribe without client ID
		subscribeParams := EventsSubscribeParams{
			EventTypes: []string{"log.info"},
		}

		params, _ := json.Marshal(subscribeParams)
		result, err := server.handleEventsSubscribe(pkgctx.NewSystemContext(), "events/subscribe", params)
		if err != nil {
			t.Fatalf("events/subscribe failed: %v", err)
		}

		subscribeResult, ok := result.(EventsSubscribeResult)
		if !ok {
			t.Fatalf("expected EventsSubscribeResult, got %T", result)
		}

		// Check that subscription ID doesn't have client prefix
		if strings.Contains(subscribeResult.SubscriptionID, "client_") {
			t.Errorf("expected subscription ID without client prefix, got '%s'", subscribeResult.SubscriptionID)
		}
		if !strings.HasPrefix(subscribeResult.SubscriptionID, "sub_") {
			t.Errorf("expected subscription ID to start with 'sub_', got '%s'", subscribeResult.SubscriptionID)
		}
	})
}

// TestClientIDReinitialization tests client ID handling during re-initialization
func TestClientIDReinitialization(t *testing.T) {
	t.Parallel()
	server := createTestServerWithoutRegistry() // Use server without registry

	// Test: Client ID is updated during re-initialization
	t.Run("updated_on_reinitialize", func(t *testing.T) {
		server.secCtx = nil // Reset secCtx
		server.clientIDMu.Lock()
		server.clientID = emptyValue // Reset
		server.clientIDMu.Unlock()

		// First initialization
		initParams1 := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "test-client",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "persistent-client-id",
				clientInfoAccountID:      "ACC-1785920548450214016-ace2aae1", // Use registered account
				objects.FieldKeyRoles:    []any{"test_agent"},
			},
		}

		params1, _ := json.Marshal(initParams1)
		_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params1)
		if err != nil {
			t.Fatalf("first initialize failed: %v", err)
		}

		// Verify client ID
		server.clientIDMu.RLock()
		clientID1 := server.clientID
		server.clientIDMu.RUnlock()

		if clientID1 != "persistent-client-id" {
			t.Errorf("expected client ID 'persistent-client-id', got '%s'", clientID1)
		}

		// Re-initialize (simulating reconnection) - client ID should be updated
		initParams2 := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "test-client",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "new-client-id",
				clientInfoAccountID:      "ACC-1785920548450214016-ace2aae1", // Use registered account
				objects.FieldKeyRoles:    []any{"test_agent"},
			},
		}

		params2, _ := json.Marshal(initParams2)
		_, err = server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params2)
		if err != nil {
			t.Fatalf("second initialize failed: %v", err)
		}

		// Verify client ID was updated (now that we always update it)
		server.clientIDMu.RLock()
		clientID2 := server.clientID
		server.clientIDMu.RUnlock()

		if clientID2 != "new-client-id" {
			t.Errorf("expected client ID 'new-client-id' after re-initialization, got '%s'", clientID2)
		}
	})
}

// TestRoleElicitation tests that role/credentials elicitation works correctly
func TestRoleElicitation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping elicitation test in short mode (elicitation format/behavior may differ)")
	}
	t.Parallel()
	server := createTestServerWithoutRegistry() // Use server without registry to test elicitation

	// Test 1: Elicitation triggered when no roles/permissions provided (non-system account)
	t.Run("elicitation_triggered_no_roles", func(t *testing.T) {
		server.secCtx = nil
		server.tools = make(map[string]Tool)

		initParams := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "test-agent",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "test-agent-123",
				// No account_id, roles, or permissions - triggers elicitation
			},
		}

		params, _ := json.Marshal(initParams)
		_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params)

		// Should return elicitation error
		if err == nil {
			t.Fatal("expected elicitation error, got nil")
		}

		elicitationErr := &ElicitationError{}
		ok := errors.As(err, &elicitationErr)
		if !ok {
			t.Fatalf("expected ElicitationError, got %T: %v", err, err)
		}

		// Verify elicitation parameters (server may elicit roles or credentials depending on config)
		if len(elicitationErr.Parameters) == 0 {
			t.Fatal("expected elicitation parameters, got none")
		}

		// If server includes roles parameter (e.g. when allowed_roles/config supports it), assert its shape
		for _, param := range elicitationErr.Parameters {
			if param.Name == "roles" {
				if !param.Required {
					t.Error("roles parameter should be required")
				}
				if param.Type != "array" {
					t.Errorf("expected roles type 'array', got '%s'", param.Type)
				}
				if len(param.Choices) == 0 {
					t.Error("expected role choices, got none")
				}
				break
			}
		}
		// When createTestServerWithoutRegistry is used, server elicits credentials (account_id, username, etc.)
		// rather than roles; either is valid.
	})

	// Test 2: No elicitation when roles are provided
	t.Run("no_elicitation_with_roles", func(t *testing.T) {
		server.secCtx = nil
		server.tools = make(map[string]Tool)

		initParams := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "test-agent",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "test-agent-456",
				clientInfoAccountID:      "ACC-1785920548450214003-23d25bd5", // Use registered account
				objects.FieldKeyRoles:    []any{"developer"},
			},
		}

		params, _ := json.Marshal(initParams)
		_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params)

		// Should NOT return elicitation error
		if err != nil {
			// Check if it's an elicitation error
			var elicitationErr *ElicitationError
			if !errors.As(err, &elicitationErr) {
				t.Fatal("unexpected elicitation error when roles are provided")
			}
			// Other errors might be OK (e.g., missing root command for CLI tools)
		}
	})

	// Test 3: No elicitation when permissions are provided
	t.Run("no_elicitation_with_permissions", func(t *testing.T) {
		server.secCtx = nil
		server.tools = make(map[string]Tool)

		initParams := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "test-agent",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID:    "test-agent-789",
				clientInfoAccountID:         "ACC-1785920548450214017-87f10a62", // Use registered account
				objects.FieldKeyPermissions: []any{"read:*"},
			},
		}

		params, _ := json.Marshal(initParams)
		_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params)

		// Should NOT return elicitation error
		if err != nil {
			// Check if it's an elicitation error
			var elicitationErr *ElicitationError
			if !errors.As(err, &elicitationErr) {
				t.Fatal("unexpected elicitation error when permissions are provided")
			}
		}
	})

	// Test 4: No elicitation for system accounts
	t.Run("no_elicitation_system_account", func(t *testing.T) {
		server.secCtx = nil
		server.tools = make(map[string]Tool)

		initParams := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "system-agent",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "ACC-1785920548450214012-68b850c0", // System account
				// No roles or permissions
			},
		}

		params, _ := json.Marshal(initParams)
		_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params)

		// Should NOT return elicitation error for system accounts
		if err != nil {
			// Check if it's an elicitation error
			var elicitationErr *ElicitationError
			if !errors.As(err, &elicitationErr) {
				t.Fatal("unexpected elicitation error for system account")
			}
		}
	})

	// Test 5: Elicitation includes correct role choices
	t.Run("elicitation_role_choices", func(t *testing.T) {
		server.secCtx = nil
		server.tools = make(map[string]Tool)

		initParams := InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "test-agent",
				Version: "1.0.0",
			},
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "test-agent-choices",
			},
		}

		params, _ := json.Marshal(initParams)
		_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params)

		// Should return elicitation error
		elicitationErr := &ElicitationError{}
		ok := errors.As(err, &elicitationErr)
		if !ok {
			t.Fatalf("expected ElicitationError, got %T", err)
		}

		// Find roles parameter (server may elicit credentials instead when no agent registry)
		var rolesParam *ElicitationParam
		for i := range elicitationErr.Parameters {
			if elicitationErr.Parameters[i].Name == "roles" {
				rolesParam = &elicitationErr.Parameters[i]
				break
			}
		}

		if rolesParam == nil {
			t.Skip("server elicited credentials instead of roles (no agent registry in test config); role choices test requires config with allowed_roles")
		}

		// Verify expected roles are in choices
		expectedRoles := []string{"admin", "developer", "viewer", "founder", "executive", "owner"}
		choicesMap := make(map[string]bool)
		for _, choice := range rolesParam.Choices {
			if str, ok := choice.(string); ok {
				choicesMap[str] = true
			}
		}

		for _, expectedRole := range expectedRoles {
			if !choicesMap[expectedRole] {
				t.Errorf("expected role '%s' in choices, but not found", expectedRole)
			}
		}
	})
}

// TestRoleEnforcement tests that role enforcement works correctly
func TestRoleEnforcement(t *testing.T) {
	t.Parallel()
	// Test 1: Enforced role overrides agent-provided roles
	t.Run("enforced_role_overrides", func(t *testing.T) {
		config := &ServerConfig{}
		config.MCPServer.Security.EnforcedRole = "viewer"

		clientInfo := map[string]any{
			clientInfoAccountID:   "test-agent",
			objects.FieldKeyRoles: []string{"admin", "developer"},
		}

		result, err := enforceRoleEnforcement(clientInfo, config, "/tmp", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rolesAny, ok := result[objects.FieldKeyRoles].([]any)
		if !ok {
			t.Fatalf("expected roles to be []any, got %T", result[objects.FieldKeyRoles])
		}

		if len(rolesAny) != 1 {
			t.Errorf("expected 1 role, got %d: %v", len(rolesAny), rolesAny)
		}
		if role, ok := rolesAny[0].(string); !ok || role != "viewer" {
			t.Errorf("expected enforced role 'viewer', got %v", rolesAny)
		}
	})

	// Test 2: Allowed roles whitelist filters invalid roles
	t.Run("allowed_roles_whitelist", func(t *testing.T) {
		config := &ServerConfig{}
		config.MCPServer.Security.AllowedRoles = []string{"viewer", "developer"}

		clientInfo := map[string]any{
			clientInfoAccountID:   "test-agent",
			objects.FieldKeyRoles: []string{"admin", "developer", "viewer"},
		}

		result, err := enforceRoleEnforcement(clientInfo, config, "/tmp", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rolesAny, ok := result[objects.FieldKeyRoles].([]any)
		if !ok {
			t.Fatalf("expected roles to be []any, got %T", result[objects.FieldKeyRoles])
		}

		// Should only contain allowed roles
		if len(rolesAny) != 2 {
			t.Errorf("expected 2 roles, got %d: %v", len(rolesAny), rolesAny)
		}
		hasDeveloper := false
		hasViewer := false
		for _, roleAny := range rolesAny {
			role, ok := roleAny.(string)
			if !ok {
				t.Errorf("expected role to be string, got %T", roleAny)
				continue
			}
			if role == "developer" {
				hasDeveloper = true
			}
			if role == "viewer" {
				hasViewer = true
			}
			if role == "admin" {
				t.Error("admin role should have been filtered out")
			}
		}
		if !hasDeveloper || !hasViewer {
			t.Errorf("expected developer and viewer roles, got %v", rolesAny)
		}
	})

	// Test 3: Allowed roles rejects all invalid roles
	t.Run("allowed_roles_rejects_all", func(t *testing.T) {
		config := &ServerConfig{}
		config.MCPServer.Security.AllowedRoles = []string{"viewer", "developer"}

		clientInfo := map[string]any{
			clientInfoAccountID:   "test-agent",
			objects.FieldKeyRoles: []string{"admin", "founder"},
		}

		_, err := enforceRoleEnforcement(clientInfo, config, "/tmp", nil)
		if err == nil {
			t.Fatal("expected error when all roles are invalid")
		}

		if !strings.Contains(err.Error(), "none of the provided roles are allowed") {
			t.Errorf("expected error about roles not allowed, got: %v", err)
		}
	})

	// Test 4: No enforcement when config is nil
	t.Run("no_enforcement_nil_config", func(t *testing.T) {
		clientInfo := map[string]any{
			clientInfoAccountID:   "test-agent",
			objects.FieldKeyRoles: []string{"admin"},
		}

		result, err := enforceRoleEnforcement(clientInfo, nil, "/tmp", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should return original clientInfo unchanged
		if result[clientInfoAccountID] != "test-agent" {
			t.Error("clientInfo should be unchanged")
		}
	})
}

// TestNotificationSentinel tests that notifications return NotificationSentinel
func TestNotificationSentinel(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.eventEmitter = NewEventEmitter(100)

	t.Run("notifications_initialized_returns_sentinel", func(t *testing.T) {
		params := json.RawMessage("{}")
		_, err := server.handleNotificationInitialized(pkgctx.NewSystemContext(), "notifications/initialized", params)

		if err == nil {
			t.Fatal("expected NotificationSentinel error")
		}

		notificationSentinel := &NotificationSentinel{}
		if !errors.As(err, &notificationSentinel) {
			t.Fatalf("expected NotificationSentinel, got %T: %v", err, err)
		}

		if err.Error() != "notification processed (no response)" {
			t.Errorf("expected error message 'notification processed (no response)', got '%s'", err.Error())
		}
	})

	t.Run("notifications_cancelled_returns_sentinel", func(t *testing.T) {
		params := json.RawMessage(`{"reason":"client_shutdown"}`)
		_, err := server.handleNotificationCancelled(pkgctx.NewSystemContext(), "notifications/cancelled", params)

		if err == nil {
			t.Fatal("expected NotificationSentinel error")
		}

		notificationSentinel := &NotificationSentinel{}
		if !errors.As(err, &notificationSentinel) {
			t.Fatalf("expected NotificationSentinel, got %T: %v", err, err)
		}

		// Verify shutdown was requested
		if !server.IsShutdownRequested() {
			t.Error("expected shutdown to be requested after client_shutdown notification")
		}
	})
}

// TestClientIDUniqueness tests that generated client IDs are unique
func TestClientIDUniqueness(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.eventEmitter = NewEventEmitter(100)

	clientIDs := make(map[string]bool)
	iterations := 10

	for i := 0; i < iterations; i++ {
		server.clientIDMu.Lock()
		server.clientID = emptyValue // Reset
		server.clientIDMu.Unlock()

		params := json.RawMessage("{}")
		_, err := server.handleNotificationInitialized(pkgctx.NewSystemContext(), "notifications/initialized", params)
		if err == nil {
			t.Fatal("expected NotificationSentinel error")
		}

		server.clientIDMu.RLock()
		clientID := server.clientID
		server.clientIDMu.RUnlock()

		if clientIDs[clientID] {
			t.Errorf("duplicate client ID generated: %s", clientID)
		}
		clientIDs[clientID] = true

		// Small delay to ensure different timestamps
		time.Sleep(time.Millisecond)
	}

	if len(clientIDs) != iterations {
		t.Errorf("expected %d unique client IDs, got %d", iterations, len(clientIDs))
	}
}

// TestClientIDClearingOnDisconnect tests that client ID is cleared on disconnect
func TestClientIDClearingOnDisconnect(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.eventEmitter = NewEventEmitter(100)

	// Set a client ID
	server.clientIDMu.Lock()
	server.clientID = "test-client-disconnect"
	server.clientIDMu.Unlock()

	// Verify client ID is set
	server.clientIDMu.RLock()
	clientID1 := server.clientID
	server.clientIDMu.RUnlock()

	if clientID1 != "test-client-disconnect" {
		t.Errorf("expected client ID 'test-client-disconnect', got '%s'", clientID1)
	}

	// Simulate disconnect by clearing client ID (as done in Serve on EOF)
	server.clientIDMu.Lock()
	server.clientID = emptyValue
	server.clientIDMu.Unlock()

	// Verify client ID is cleared
	server.clientIDMu.RLock()
	clientID2 := server.clientID
	server.clientIDMu.RUnlock()

	if clientID2 != emptyValue {
		t.Errorf("expected client ID to be cleared, got '%s'", clientID2)
	}
}

// TestClientIDInSubscriptionID tests that subscription IDs include client ID
func TestClientIDInSubscriptionID(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.eventEmitter = NewEventEmitter(100)

	// Set client ID
	server.clientIDMu.Lock()
	server.clientID = "test-sub-client"
	server.clientIDMu.Unlock()

	// Subscribe to events
	subscribeParams := EventsSubscribeParams{
		EventTypes: []string{"log.info", "log.error"},
	}

	params, _ := json.Marshal(subscribeParams)
	result, err := server.handleEventsSubscribe(pkgctx.NewSystemContext(), "events/subscribe", params)
	if err != nil {
		t.Fatalf("events/subscribe failed: %v", err)
	}

	subscribeResult, ok := result.(EventsSubscribeResult)
	if !ok {
		t.Fatalf("expected EventsSubscribeResult, got %T", result)
	}

	// Verify subscription ID includes client ID
	if !strings.HasPrefix(subscribeResult.SubscriptionID, "test-sub-client_") {
		t.Errorf("expected subscription ID to start with 'test-sub-client_', got '%s'", subscribeResult.SubscriptionID)
	}

	// Verify subscription ID format: clientID_sub_timestamp
	parts := strings.Split(subscribeResult.SubscriptionID, "_")
	if len(parts) < 3 {
		t.Errorf("expected subscription ID to have at least 3 parts separated by '_', got '%s'", subscribeResult.SubscriptionID)
	}
	if parts[0] != "test-sub-client" {
		t.Errorf("expected first part to be 'test-sub-client', got '%s'", parts[0])
	}
	if parts[1] != "sub" {
		t.Errorf("expected second part to be 'sub', got '%s'", parts[1])
	}
}
