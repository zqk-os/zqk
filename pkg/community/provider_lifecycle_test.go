package community

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestProviderLifecycle_StaticFloorInvariant(t *testing.T) {
	lm := NewProviderLifecycleManager()
	if lm.State() != LifecycleActive {
		t.Fatalf("expected initial state LifecycleActive, got %v", lm.State())
	}
	if lm.ActiveCount() != 0 {
		t.Errorf("expected 0 active executions, got %d", lm.ActiveCount())
	}

	if err := lm.Execute(context.Background(), nil); err == nil {
		t.Errorf("expected error for nil handler")
	}
}

func TestProviderLifecycle_OperationalDynamicProof(t *testing.T) {
	lm := NewProviderLifecycleManager(2 * time.Second)
	ctx := context.Background()

	execStarted := make(chan struct{})
	allowFinish := make(chan struct{})

	// Start long-running in-flight execution using managed goroutine
	goroutinelabels.NewGoroutine("test_lifecycle_inflight", "simulate in-flight execution").StartSimple(func() {
		if err := lm.Execute(ctx, func(c context.Context) error {
			close(execStarted)
			<-allowFinish
			return nil
		}); err != nil {
			// In-flight error handled
		}
	})

	<-execStarted
	if lm.ActiveCount() != 1 {
		t.Fatalf("expected activeCount=1, got %d", lm.ActiveCount())
	}

	// Trigger Drain concurrently
	drainDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_lifecycle_drain", "run drain").StartSimple(func() {
		drainDone <- lm.Drain(ctx)
	})

	// Give drain time to enter Draining
	time.Sleep(20 * time.Millisecond)
	if lm.State() != LifecycleDraining {
		t.Errorf("expected LifecycleDraining, got %v", lm.State())
	}

	// New requests should be rejected with ErrProviderDraining
	err := lm.Execute(ctx, func(c context.Context) error { return nil })
	if !errors.Is(err, ErrProviderDraining) {
		t.Errorf("expected ErrProviderDraining, got %v", err)
	}

	// Allow in-flight execution to finish
	close(allowFinish)

	// Wait for drain to complete
	drainErr := <-drainDone
	if drainErr != nil {
		t.Fatalf("expected clean drain, got error: %v", drainErr)
	}

	if lm.State() != LifecycleDecommissioned {
		t.Errorf("expected LifecycleDecommissioned after drain, got %v", lm.State())
	}
	if lm.ActiveCount() != 0 {
		t.Errorf("expected 0 active count, got %d", lm.ActiveCount())
	}

	// Subsequent calls must return ErrProviderDecommissioned
	err = lm.Execute(ctx, func(c context.Context) error { return nil })
	if !errors.Is(err, ErrProviderDecommissioned) {
		t.Errorf("expected ErrProviderDecommissioned, got %v", err)
	}
}

func TestProviderLifecycle_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	// Drain timeout scenario: execution never finishes within timeout
	lm := NewProviderLifecycleManager(50 * time.Millisecond)
	ctx := context.Background()

	execStarted := make(chan struct{})
	goroutinelabels.NewGoroutine("test_lifecycle_hang", "simulate hanging execution").StartSimple(func() {
		if err := lm.Execute(ctx, func(c context.Context) error {
			close(execStarted)
			time.Sleep(500 * time.Millisecond)
			return nil
		}); err != nil {
			// Hanging error handled
		}
	})

	<-execStarted
	err := lm.Drain(ctx)
	if !errors.Is(err, ErrDrainTimeout) {
		t.Errorf("expected ErrDrainTimeout, got: %v", err)
	}

	if lm.State() != LifecycleDecommissioned {
		t.Errorf("expected LifecycleDecommissioned after timeout drain, got %v", lm.State())
	}
}

func TestProviderLifecycle_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_HOT_RELOAD_AND_DYNAMIC_CAPABILITY_LIFECYCLE_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0044 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0044 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0044 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
