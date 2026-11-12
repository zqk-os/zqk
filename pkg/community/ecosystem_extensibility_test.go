package community

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestEcosystemExtensibility_StaticFloorInvariant proves invariant defaults, supported lifecycle hook stages,
// and default timeout boundaries for provider descriptors.
func TestEcosystemExtensibility_StaticFloorInvariant(t *testing.T) {
	stages := []LifecycleHookStage{
		HookStagePreExecution,
		HookStagePostExecution,
		HookStageErrorRecovery,
		HookStageTelemetryEmit,
	}

	if len(stages) != 4 {
		t.Fatalf("expected 4 standard lifecycle hook stages, got %d", len(stages))
	}

	validDesc := ProviderDescriptor{
		Name:          "community-cloud-provider",
		Version:       "1.0.0",
		TargetVersion: "v0.3.6",
		Capabilities:  []string{"metrics", "tracing"},
	}

	if err := validDesc.Validate(); err != nil {
		t.Fatalf("expected valid descriptor to pass, got: %v", err)
	}

	if validDesc.Timeout != 5*time.Second {
		t.Fatalf("expected default timeout 5s, got: %v", validDesc.Timeout)
	}
}

// TestEcosystemExtensibility_OperationalDynamicProof proves registration, lookup, and dispatching of
// custom lifecycle hooks with payload transformation.
func TestEcosystemExtensibility_OperationalDynamicProof(t *testing.T) {
	reg := NewProviderRegistry()

	preExecuted := false
	desc := ProviderDescriptor{
		Name:          "custom-telemetry-hook",
		Version:       "0.1.0",
		TargetVersion: "v0.3.6",
		Capabilities:  []string{"telemetry"},
		Timeout:       2 * time.Second,
		HookHandlers: map[LifecycleHookStage]LifecycleHookHandler{
			HookStagePreExecution: func(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
				preExecuted = true
				payload["intercepted"] = true
				return payload, nil
			},
		},
	}

	if err := reg.Register(desc); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	if reg.Count() != 1 {
		t.Fatalf("expected 1 registered provider, got %d", reg.Count())
	}

	// Dispatch hook
	payload := map[string]interface{}{"event": "start"}
	res, err := reg.DispatchHook(context.Background(), "custom-telemetry-hook", HookStagePreExecution, payload)
	if err != nil {
		t.Fatalf("dispatch hook failed: %v", err)
	}

	if !preExecuted {
		t.Fatalf("expected hook to execute")
	}

	if res["intercepted"] != true {
		t.Fatalf("expected payload to contain intercepted=true, got: %v", res)
	}
}

// TestEcosystemExtensibility_NegativeBoundaryAndErrorRecovery proves fail-closed boundary enforcement:
// invalid descriptors, duplicate registration, timeout cancellation, and panic recovery.
func TestEcosystemExtensibility_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	reg := NewProviderRegistry()

	// 1. Invalid descriptors
	invalidDescriptors := []ProviderDescriptor{
		{Name: "", Version: "1.0.0", Capabilities: []string{"metrics"}},
		{Name: "invalid name!", Version: "1.0.0", Capabilities: []string{"metrics"}},
		{Name: "valid-name", Version: "", Capabilities: []string{"metrics"}},
		{Name: "valid-name", Version: "1.0.0", Capabilities: []string{}},
		{Name: "valid-name", Version: "1.0.0", Capabilities: []string{"metrics"}, Timeout: 60 * time.Second},
	}

	for _, d := range invalidDescriptors {
		if err := d.Validate(); !errors.Is(err, ErrInvalidProviderDescriptor) {
			t.Errorf("expected ErrInvalidProviderDescriptor for %+v, got %v", d, err)
		}
	}

	// 2. Duplicate registration
	validDesc := ProviderDescriptor{
		Name:         "provider-alpha",
		Version:      "1.0.0",
		Capabilities: []string{"logging"},
	}
	if err := reg.Register(validDesc); err != nil {
		t.Fatalf("initial registration failed: %v", err)
	}
	if err := reg.Register(validDesc); !errors.Is(err, ErrProviderAlreadyRegistered) {
		t.Fatalf("expected ErrProviderAlreadyRegistered on duplicate, got: %v", err)
	}

	// 3. Timeout cancellation
	slowDesc := ProviderDescriptor{
		Name:         "slow-provider",
		Version:      "1.0.0",
		Capabilities: []string{"slow"},
		Timeout:      20 * time.Millisecond,
		HookHandlers: map[LifecycleHookStage]LifecycleHookHandler{
			HookStagePostExecution: func(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
				time.Sleep(100 * time.Millisecond)
				return payload, nil
			},
		},
	}
	if err := reg.Register(slowDesc); err != nil {
		t.Fatalf("failed to register slow provider: %v", err)
	}

	_, err := reg.DispatchHook(context.Background(), "slow-provider", HookStagePostExecution, map[string]interface{}{})
	if !errors.Is(err, ErrHookTimeout) {
		t.Fatalf("expected ErrHookTimeout for slow hook, got: %v", err)
	}

	// 4. Panic isolation: plugin panic must not crash host
	panicDesc := ProviderDescriptor{
		Name:         "panicking-provider",
		Version:      "1.0.0",
		Capabilities: []string{"danger"},
		HookHandlers: map[LifecycleHookStage]LifecycleHookHandler{
			HookStageErrorRecovery: func(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
				panic("simulated fatal crash in third-party plugin")
			},
		},
	}
	if err := reg.Register(panicDesc); err != nil {
		t.Fatalf("failed to register panicking provider: %v", err)
	}

	_, err = reg.DispatchHook(context.Background(), "panicking-provider", HookStageErrorRecovery, map[string]interface{}{})
	if !errors.Is(err, ErrHookPanicked) {
		t.Fatalf("expected ErrHookPanicked from recovering panic, got: %v", err)
	}
}

// TestEcosystemExtensibility_DocumentationVerification verifies ADR-0038 presence and content completeness.
func TestEcosystemExtensibility_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_COMMUNITY_ECOSYSTEM_EXTENSIBILITY_AND_PROVIDER_SDK_V036.md")
	if !fileutil.Exists(adrPath) {
		t.Fatalf("expected ADR-0038 to exist at %s", adrPath)
	}

	content, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("failed to read ADR-0038 at %s: %v", adrPath, err)
	}

	adrText := string(content)
	requiredSections := []string{
		"Title: Community Ecosystem Extensibility, Custom Plugin Hooks and Provider SDK v0.3.6",
		"Status: Accepted",
		"Context",
		"Decision",
		"Consequences",
	}

	for _, sec := range requiredSections {
		if !strings.Contains(adrText, sec) {
			t.Errorf("expected ADR to contain %q", sec)
		}
	}
}
