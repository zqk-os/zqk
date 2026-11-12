package community

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestResourceGovernor_StaticFloorInvariant(t *testing.T) {
	gov, err := NewProviderResourceGovernor()
	if err != nil {
		t.Fatalf("expected default governor creation to succeed, got %v", err)
	}
	if gov.ActiveCount() != 0 {
		t.Errorf("expected 0 active slots, got %d", gov.ActiveCount())
	}

	badOpts := GovernorOptions{
		MaxConcurrency:     -1,
		RateLimitPerSecond: 10,
	}
	if _, err := NewProviderResourceGovernor(badOpts); !errors.Is(err, ErrInvalidQuotaConfig) {
		t.Errorf("expected ErrInvalidQuotaConfig for negative concurrency, got: %v", err)
	}
}

func TestResourceGovernor_OperationalDynamicProof(t *testing.T) {
	opts := GovernorOptions{
		MaxConcurrency:     2,
		RateLimitPerSecond: 50,
	}
	gov, err := NewProviderResourceGovernor(opts)
	if err != nil {
		t.Fatalf("failed to create governor: %v", err)
	}

	ctx := context.Background()

	// Acquire slot 1
	rel1, err := gov.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire slot 1: %v", err)
	}
	if gov.ActiveCount() != 1 {
		t.Errorf("expected activeCount=1, got %d", gov.ActiveCount())
	}

	// Acquire slot 2
	rel2, err := gov.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire slot 2: %v", err)
	}
	if gov.ActiveCount() != 2 {
		t.Errorf("expected activeCount=2, got %d", gov.ActiveCount())
	}

	// Release slot 1
	rel1()
	if gov.ActiveCount() != 1 {
		t.Errorf("expected activeCount=1 after release, got %d", gov.ActiveCount())
	}

	// Double release should be idempotent
	rel1()
	if gov.ActiveCount() != 1 {
		t.Errorf("expected activeCount=1 after redundant release, got %d", gov.ActiveCount())
	}

	// Release slot 2
	rel2()
	if gov.ActiveCount() != 0 {
		t.Errorf("expected activeCount=0, got %d", gov.ActiveCount())
	}
}

func TestResourceGovernor_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	opts := GovernorOptions{
		MaxConcurrency:     1,
		RateLimitPerSecond: 2,
	}
	gov, err := NewProviderResourceGovernor(opts)
	if err != nil {
		t.Fatalf("failed to create governor: %v", err)
	}

	ctx := context.Background()

	rel1, err := gov.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire slot 1: %v", err)
	}
	defer rel1()

	// Quota exceeded: max concurrency is 1
	if _, err := gov.Acquire(ctx); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("expected ErrQuotaExceeded, got: %v", err)
	}

	// Test rate limit exhaustion
	rel1() // free slot so concurrency isn't the blocker

	// Consume remaining token (rate limit was 2, we used 1 above)
	rel2, err := gov.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected acquire to succeed: %v", err)
	}
	rel2()

	// Third attempt should hit rate limit
	if _, err := gov.Acquire(ctx); !errors.Is(err, ErrRateLimitExceeded) {
		t.Errorf("expected ErrRateLimitExceeded, got: %v", err)
	}

	// Canceled context check
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// Wait 1s for rate refill to test context cancellation priority
	time.Sleep(1050 * time.Millisecond)
	if _, err := gov.Acquire(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestResourceGovernor_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_SANDBOX_RESOURCE_GOVERNOR_AND_QUOTA_ENFORCEMENT_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0043 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0043 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0043 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
