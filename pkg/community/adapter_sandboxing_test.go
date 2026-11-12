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

// TestAdapterSandboxing_StaticFloorInvariant proves standard defaults, isolation invariants,
// and default timeout structures for adapter execution sandboxes.
func TestAdapterSandboxing_StaticFloorInvariant(t *testing.T) {
	opts := DefaultSandboxOptions()

	if opts.MaxTimeout != 5*time.Second {
		t.Fatalf("expected default timeout 5s, got: %v", opts.MaxTimeout)
	}

	if opts.AllowGoroutines {
		t.Fatalf("expected unmanaged goroutines to be disallowed by default")
	}

	if opts.MaxMemoryBytes < 1024*1024 {
		t.Fatalf("expected memory limit >= 1MB, got: %d", opts.MaxMemoryBytes)
	}
}

// TestAdapterSandboxing_OperationalDynamicProof proves successful protected execution of an adapter
// inside the sandbox boundary.
func TestAdapterSandboxing_OperationalDynamicProof(t *testing.T) {
	sandbox := NewSandboxedAdapter(DefaultSandboxOptions())

	executed := false
	err := sandbox.Execute(context.Background(), func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("expected successful execution, got: %v", err)
	}

	if !executed {
		t.Fatalf("expected adapter handler to execute")
	}
}

// TestAdapterSandboxing_NegativeBoundaryAndErrorRecovery proves panic interception, timeout cancellation,
// and nil handler rejection.
func TestAdapterSandboxing_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	// 1. Nil handler
	sandbox := NewSandboxedAdapter(DefaultSandboxOptions())
	if err := sandbox.Execute(context.Background(), nil); !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("expected ErrInvalidAdapter for nil handler, got: %v", err)
	}

	// 2. Panic interception
	panicErr := sandbox.Execute(context.Background(), func(ctx context.Context) error {
		panic("unhandled crash in external adapter")
	})
	if !errors.Is(panicErr, ErrAdapterPanicked) {
		t.Fatalf("expected ErrAdapterPanicked, got: %v", panicErr)
	}

	// 3. Timeout cancellation
	fastTimeoutSandbox := NewSandboxedAdapter(SandboxOptions{
		MaxTimeout: 20 * time.Millisecond,
	})
	timeoutErr := fastTimeoutSandbox.Execute(context.Background(), func(ctx context.Context) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	if !errors.Is(timeoutErr, ErrAdapterTimeout) {
		t.Fatalf("expected ErrAdapterTimeout, got: %v", timeoutErr)
	}
}

// TestAdapterSandboxing_DocumentationVerification verifies ADR-0039 presence and required sections.
func TestAdapterSandboxing_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_ECOSYSTEM_ADAPTER_SANDBOXING_AND_PROVIDER_REGISTRY_V036.md")
	if !fileutil.Exists(adrPath) {
		t.Fatalf("expected ADR-0039 to exist at %s", adrPath)
	}

	content, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("failed to read ADR-0039 at %s: %v", adrPath, err)
	}

	adrText := string(content)
	requiredSections := []string{
		"Title: Autonomous Ecosystem Adapter Sandboxing & Provider Registry v0.3.6",
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
