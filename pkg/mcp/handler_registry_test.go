package mcp

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestToolHandlerRegistry(t *testing.T) {
	registry := NewToolHandlerRegistry()

	dummyHandler := func(ctx context.Context, args map[string]any) (any, error) {
		return "ok", nil
	}

	registry.Register("dummy_handler", dummyHandler)

	h, ok := registry.Get("dummy_handler")
	if !ok {
		t.Fatalf("Expected dummy_handler to be registered")
	}

	res, err := h(context.Background(), nil)
	if err != nil {
		t.Fatalf("Handler call failed: %v", err)
	}
	if res != "ok" {
		t.Errorf("Expected 'ok', got '%v'", res)
	}

	_, missingOk := registry.Get("nonexistent")
	if missingOk {
		t.Errorf("Expected nonexistent handler to return false")
	}
}

func TestToolHandlerRegistry_BootRegistration(t *testing.T) {
	h, ok := GlobalToolHandlerRegistry.Get("echo")
	if !ok {
		t.Fatalf("Expected built-in 'echo' handler in GlobalToolHandlerRegistry")
	}

	res, err := h(context.Background(), map[string]any{"message": "boot_test"})
	if err != nil {
		t.Fatalf("Echo handler call failed: %v", err)
	}

	resMap, ok := res.(map[string]any)
	if !ok || resMap["echo"] != "boot_test" {
		t.Errorf("Expected echo result 'boot_test', got %v", res)
	}
}

func TestToolHandlerRegistry_ExpandedRegistration(t *testing.T) {
	h, ok := GlobalToolHandlerRegistry.Get("get_metrics")
	if !ok {
		t.Fatalf("Expected 'get_metrics' handler in GlobalToolHandlerRegistry")
	}

	res, err := h(context.Background(), map[string]any{objects.FieldKeyFormat: "json"})
	if err != nil {
		t.Fatalf("Metrics handler call failed: %v", err)
	}

	resMap, ok := res.(map[string]any)
	if !ok || resMap[objects.FieldKeyStatus] != "success" {
		t.Errorf("Expected status 'success', got %v", res)
	}
}

func TestMCPSpecGenerator_UnregisteredHandlerError(t *testing.T) {
	server := NewServer()
	gen := NewMCPSpecGenerator(server)

	spec := ToolSpec{
		Name:        "test_tool",
		Description: "test tool with missing handler",
		Handler:     "unknown_handler_xyz",
	}

	err := gen.registerTool(spec)
	if err == nil {
		t.Fatalf("Expected error when registering tool with unknown_handler_xyz, got nil")
	}
}

func TestToolHandlerRegistry_LifetimeCounters(t *testing.T) {
	var nilReg *ToolHandlerRegistry
	regNil, looNil, hitNil := nilReg.GetToolHandlerRegistryStats()
	if regNil != 0 || looNil != 0 || hitNil != 0 {
		t.Fatalf("expected nil stats (0, 0, 0), got (%d, %d, %d)", regNil, looNil, hitNil)
	}

	reg := NewToolHandlerRegistry()
	regInit, looInit, hitInit := reg.GetToolHandlerRegistryStats()
	if regInit != 0 || looInit != 0 || hitInit != 0 {
		t.Fatalf("expected initial stats (0, 0, 0), got (%d, %d, %d)", regInit, looInit, hitInit)
	}

	dummyHandler := func(ctx context.Context, args map[string]any) (any, error) {
		return "ok", nil
	}

	reg.Register("h1", dummyHandler)

	// Hit
	_, found := reg.Get("h1")
	if !found {
		t.Fatalf("expected h1 found")
	}

	// Miss
	_, foundMissing := reg.Get("missing")
	if foundMissing {
		t.Fatalf("expected missing not found")
	}

	registered, lookups, hits := reg.GetToolHandlerRegistryStats()
	if registered != 1 || lookups != 2 || hits != 1 {
		t.Fatalf("expected stats (1, 2, 1), got (%d, %d, %d)", registered, lookups, hits)
	}
}
