package community

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func newTestNamespaceManager(t *testing.T) *TenantNamespaceManager {
	t.Helper()
	mgr, err := NewTenantNamespaceManager()
	if err != nil {
		t.Fatalf("NewTenantNamespaceManager failed: %v", err)
	}
	return mgr
}

func registerAndIssueToken(t *testing.T, mgr *TenantNamespaceManager, tenantID, nsID string, quota TenantQuota) string {
	t.Helper()
	ns, err := mgr.RegisterTenant(tenantID, nsID, quota)
	if err != nil {
		t.Fatalf("RegisterTenant(%q, %q) failed: %v", tenantID, nsID, err)
	}
	if ns.TenantID != tenantID || ns.NamespaceID != nsID {
		t.Fatalf("registered namespace mismatch: got (%q, %q), want (%q, %q)", ns.TenantID, ns.NamespaceID, tenantID, nsID)
	}
	token, err := mgr.IssueSessionToken(tenantID, nsID)
	if err != nil {
		t.Fatalf("IssueSessionToken(%q, %q) failed: %v", tenantID, nsID, err)
	}
	return token
}

func assertErrorIs(t *testing.T, err, target error, context string) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s: expected error %v, got %v", context, target, err)
	}
}

func TestTenantNamespace_StaticFloorInvariant(t *testing.T) {
	mgr := newTestNamespaceManager(t)

	invalidRegistrations := []struct {
		name     string
		tenantID string
		nsID     string
		quota    TenantQuota
		wantErr  error
	}{
		{"blank tenant", "", "ns-alpha", TenantQuota{MaxConcurrentOps: 2, MaxTokenQuota: 100}, ErrInvalidTenantConfig},
		{"whitespace tenant", "   ", "ns-alpha", TenantQuota{MaxConcurrentOps: 2, MaxTokenQuota: 100}, ErrInvalidTenantConfig},
		{"blank namespace", "tenant-a", " \t ", TenantQuota{MaxConcurrentOps: 2, MaxTokenQuota: 100}, ErrInvalidTenantConfig},
		{"zero concurrency", "tenant-a", "ns-alpha", TenantQuota{MaxConcurrentOps: 0, MaxTokenQuota: 100}, ErrInvalidTenantConfig},
		{"negative concurrency", "tenant-a", "ns-alpha", TenantQuota{MaxConcurrentOps: -1, MaxTokenQuota: 100}, ErrInvalidTenantConfig},
		{"zero token quota", "tenant-a", "ns-alpha", TenantQuota{MaxConcurrentOps: 2, MaxTokenQuota: 0}, ErrInvalidTenantConfig},
		{"negative token quota", "tenant-a", "ns-alpha", TenantQuota{MaxConcurrentOps: 2, MaxTokenQuota: -50}, ErrInvalidTenantConfig},
	}

	for _, tc := range invalidRegistrations {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mgr.RegisterTenant(tc.tenantID, tc.nsID, tc.quota)
			assertErrorIs(t, err, tc.wantErr, tc.name)
		})
	}

	tokenA := registerAndIssueToken(t, mgr, "tenant-alpha", "ns-alpha", TenantQuota{MaxConcurrentOps: 3, MaxTokenQuota: 500})
	if err := mgr.ValidateAccess(tokenA, "tenant-alpha", "ns-alpha"); err != nil {
		t.Fatalf("expected valid access for registered tenant: %v", err)
	}

	// Duplicate tenant or claimed namespace must fail closed.
	_, dupTenantErr := mgr.RegisterTenant("tenant-alpha", "ns-other", TenantQuota{MaxConcurrentOps: 1, MaxTokenQuota: 10})
	assertErrorIs(t, dupTenantErr, ErrTenantAlreadyExists, "duplicate tenant")

	_, dupNSErr := mgr.RegisterTenant("tenant-beta", "ns-alpha", TenantQuota{MaxConcurrentOps: 1, MaxTokenQuota: 10})
	assertErrorIs(t, dupNSErr, ErrNamespaceBoundaryViolation, "claimed namespace")
}

func TestTenantNamespace_OperationalDynamicProof(t *testing.T) {
	mgr := newTestNamespaceManager(t)
	tokenA := registerAndIssueToken(t, mgr, "tenant-alpha", "ns-alpha", TenantQuota{MaxConcurrentOps: 2, MaxTokenQuota: 100})
	tokenB := registerAndIssueToken(t, mgr, "tenant-beta", "ns-beta", TenantQuota{MaxConcurrentOps: 3, MaxTokenQuota: 300})

	// Sovereign object isolation: same key in different namespaces stays partitioned.
	if err := mgr.WriteObject(tokenA, "tenant-alpha", "ns-alpha", "config", "alpha-secret"); err != nil {
		t.Fatalf("WriteObject alpha failed: %v", err)
	}
	if err := mgr.WriteObject(tokenB, "tenant-beta", "ns-beta", "config", "beta-secret"); err != nil {
		t.Fatalf("WriteObject beta failed: %v", err)
	}
	valA, err := mgr.ReadObject(tokenA, "tenant-alpha", "ns-alpha", "config")
	if err != nil || valA != "alpha-secret" {
		t.Fatalf("ReadObject alpha = (%q, %v), want (%q, nil)", valA, err, "alpha-secret")
	}

	// Concurrent slot acquisition and token deduction under load.
	var succeeded atomic.Int64
	var throttled atomic.Int64
	var quotaRejected atomic.Int64

	wg := goroutinelabels.StartTestGoroutines(t, 8, "tenant_worker", "concurrent quota enforcement", func(id int) {
		lease, acqErr := mgr.AcquireSlot(tokenA, "tenant-alpha", "ns-alpha")
		if acqErr != nil {
			if errors.Is(acqErr, ErrConcurrencyLimitReached) {
				throttled.Add(1)
			}
			return
		}
		if consErr := mgr.ConsumeTokens(tokenA, lease, 30); consErr != nil {
			if errors.Is(consErr, ErrQuotaExceeded) {
				quotaRejected.Add(1)
			}
		} else {
			succeeded.Add(1)
		}
		if relErr := mgr.ReleaseSlot(tokenA, lease); relErr != nil {
			t.Errorf("worker %d ReleaseSlot failed: %v", id, relErr)
		}
	})
	wg.Wait()

	// Also verify deterministic concurrency saturation when holding MaxConcurrentOps=2 slots.
	l1, err := mgr.AcquireSlot(tokenA, "tenant-alpha", "ns-alpha")
	if err != nil {
		t.Fatalf("AcquireSlot l1 failed: %v", err)
	}
	l2, err := mgr.AcquireSlot(tokenA, "tenant-alpha", "ns-alpha")
	if err != nil {
		t.Fatalf("AcquireSlot l2 failed: %v", err)
	}
	_, l3Err := mgr.AcquireSlot(tokenA, "tenant-alpha", "ns-alpha")
	assertErrorIs(t, l3Err, ErrConcurrencyLimitReached, "third concurrent slot")
	if err := mgr.ReleaseSlot(tokenA, l1); err != nil {
		t.Fatalf("ReleaseSlot l1 failed: %v", err)
	}
	if err := mgr.ReleaseSlot(tokenA, l2); err != nil {
		t.Fatalf("ReleaseSlot l2 failed: %v", err)
	}

	telA, err := mgr.Telemetry("tenant-alpha")
	if err != nil {
		t.Fatalf("Telemetry alpha failed: %v", err)
	}
	if telA.ActiveOps != 0 {
		t.Fatalf("expected 0 active ops after releases, got %d", telA.ActiveOps)
	}
	if telA.TokensUsed > 100 || telA.TokensUsed != succeeded.Load()*30 {
		t.Fatalf("unexpected tokens used: %d (succeeded=%d)", telA.TokensUsed, succeeded.Load())
	}
	if telA.RejectedConcurrency < 1 {
		t.Fatalf("expected at least 1 concurrency rejection in telemetry, got %d", telA.RejectedConcurrency)
	}
	if telA.TotalAcquires != telA.TotalReleases {
		t.Fatalf("expected acquires (%d) == releases (%d)", telA.TotalAcquires, telA.TotalReleases)
	}

	// Tenant Beta must remain completely unaffected by Alpha's saturation.
	telB, err := mgr.Telemetry("tenant-beta")
	if err != nil {
		t.Fatalf("Telemetry beta failed: %v", err)
	}
	if telB.TokensUsed != 0 || telB.RejectedConcurrency != 0 || telB.RejectedQuota != 0 {
		t.Fatalf("expected tenant-beta telemetry unaffected, got %+v", telB)
	}
}

func TestTenantNamespace_NegativeAdversarialBoundary(t *testing.T) {
	mgr := newTestNamespaceManager(t)
	tokenA := registerAndIssueToken(t, mgr, "tenant-alpha", "ns-alpha", TenantQuota{MaxConcurrentOps: 2, MaxTokenQuota: 50})
	tokenB := registerAndIssueToken(t, mgr, "tenant-beta", "ns-beta", TenantQuota{MaxConcurrentOps: 2, MaxTokenQuota: 50})

	if err := mgr.WriteObject(tokenB, "tenant-beta", "ns-beta", "secret", "top-secret-beta"); err != nil {
		t.Fatalf("WriteObject beta failed: %v", err)
	}

	for _, tc := range buildAdversarialBoundaryCases(t, mgr, tokenA) {
		t.Run(tc.name, func(t *testing.T) {
			assertErrorIs(t, tc.exec(), tc.wantErr, tc.name)
		})
	}

	telA, err := mgr.Telemetry("tenant-alpha")
	if err != nil {
		t.Fatalf("Telemetry alpha failed: %v", err)
	}
	if telA.RejectedBoundary < 2 {
		t.Fatalf("expected >=2 boundary rejections recorded, got %d", telA.RejectedBoundary)
	}
	if telA.RejectedQuota != 1 {
		t.Fatalf("expected 1 quota rejection recorded, got %d", telA.RejectedQuota)
	}
}

type adversarialBoundaryCase struct {
	name    string
	exec    func() error
	wantErr error
}

func consumeWithAutoRelease(t *testing.T, mgr *TenantNamespaceManager, token, tenantID, nsID string, tokens int64) error {
	t.Helper()
	lease, err := mgr.AcquireSlot(token, tenantID, nsID)
	if err != nil {
		return err
	}
	defer func() {
		if relErr := mgr.ReleaseSlot(token, lease); relErr != nil {
			t.Errorf("cleanup ReleaseSlot failed: %v", relErr)
		}
	}()
	return mgr.ConsumeTokens(token, lease, tokens)
}

func buildAdversarialBoundaryCases(t *testing.T, mgr *TenantNamespaceManager, tokenA string) []adversarialBoundaryCase {
	t.Helper()
	parts := strings.Split(tokenA, "|")
	tamperedSig := fmt.Sprintf("%s|%s|%s|%s", parts[0], parts[1], parts[2], strings.Repeat("0", len(parts[3])))

	return []adversarialBoundaryCase{
		{
			name:    "blank token rejected",
			exec:    func() error { return mgr.ValidateAccess("", "tenant-alpha", "ns-alpha") },
			wantErr: ErrTenantImpersonation,
		},
		{
			name:    "malformed token rejected",
			exec:    func() error { return mgr.ValidateAccess("bad-token", "tenant-alpha", "ns-alpha") },
			wantErr: ErrTenantImpersonation,
		},
		{
			name:    "tampered signature rejected",
			exec:    func() error { return mgr.ValidateAccess(tamperedSig, "tenant-alpha", "ns-alpha") },
			wantErr: ErrTenantImpersonation,
		},
		{
			name:    "tenant context switching impersonation",
			exec:    func() error { return mgr.ValidateAccess(tokenA, "tenant-beta", "ns-beta") },
			wantErr: ErrTenantImpersonation,
		},
		{
			name:    "namespace spoofing with valid tenant",
			exec:    func() error { return mgr.ValidateAccess(tokenA, "tenant-alpha", "ns-beta") },
			wantErr: ErrNamespaceBoundaryViolation,
		},
		{
			name: "cross-namespace object read blocked",
			exec: func() error {
				_, readErr := mgr.ReadObject(tokenA, "tenant-alpha", "ns-beta", "secret")
				return readErr
			},
			wantErr: ErrNamespaceBoundaryViolation,
		},
		{
			name: "cross-tenant object read blocked",
			exec: func() error {
				_, readErr := mgr.ReadObject(tokenA, "tenant-beta", "ns-beta", "secret")
				return readErr
			},
			wantErr: ErrTenantImpersonation,
		},
		{
			name:    "non-positive token consumption rejected",
			exec:    func() error { return consumeWithAutoRelease(t, mgr, tokenA, "tenant-alpha", "ns-alpha", 0) },
			wantErr: ErrInvalidTenantConfig,
		},
		{
			name:    "quota exhaustion fails closed",
			exec:    func() error { return consumeWithAutoRelease(t, mgr, tokenA, "tenant-alpha", "ns-alpha", 51) },
			wantErr: ErrQuotaExceeded,
		},
		{
			name: "unknown lease rejected",
			exec: func() error {
				return mgr.ConsumeTokens(tokenA, NamespaceLease{
					LeaseID:     "forged-lease-id",
					TenantID:    "tenant-alpha",
					NamespaceID: "ns-alpha",
				}, 10)
			},
			wantErr: ErrInvalidLease,
		},
	}
}
