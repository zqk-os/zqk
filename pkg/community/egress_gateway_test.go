package community

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testEgressClusterID      = "cluster-sovereign-01"
	testForeignClusterID     = "cluster-foreign-99"
	testAllowedDomainPrimary = "api.partner.example"
	testAllowedDomainBackup  = "sync.federation.example"
	testUnauthorizedDomain   = "exfil.untrusted.example"
	testSigningKeyMaterial   = "sovereign-egress-hmac-key-2026-32b"
	testSafePayload          = "telemetry summary: healthy=true"
	testSecretPayload        = "Bearer tok_abc123 sk-1234567890abcdef api_key=xyz987 123-45-6789"
)

func defaultTestSovereigntyPolicy() SovereigntyPolicy {
	return SovereigntyPolicy{
		ClusterID:      testEgressClusterID,
		AllowedDomains: []string{testAllowedDomainPrimary, testAllowedDomainBackup},
		Rules: []TransitRule{
			{Classification: ClassificationPublic, AllowEgress: true, RequireRedaction: false, RequireToken: false},
			{Classification: ClassificationInternal, AllowEgress: true, RequireRedaction: true, RequireToken: true},
			{Classification: ClassificationConfidential, AllowEgress: true, RequireRedaction: true, RequireToken: true},
			{Classification: ClassificationRestricted, AllowEgress: false, RequireRedaction: true, RequireToken: true},
		},
		SigningKey: []byte(testSigningKeyMaterial),
	}
}

func newTestEgressGateway(t *testing.T) *EgressGateway {
	t.Helper()
	gw, err := NewEgressGateway(defaultTestSovereigntyPolicy())
	require.NoError(t, err)
	require.NotNil(t, gw)
	return gw
}

// TestEgressGateway_StaticFloorInvariant verifies schema validation for classification tags,
// destination domains, transit rules, and signing key material.
func TestEgressGateway_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	require.NoError(t, defaultTestSovereigntyPolicy().Validate())

	invalidCases := []struct {
		name    string
		mutate  func(*SovereigntyPolicy)
		wantErr error
	}{
		{
			name:    "blank cluster ID",
			mutate:  func(p *SovereigntyPolicy) { p.ClusterID = "   " },
			wantErr: ErrBlankClusterID,
		},
		{
			name:    "undersized signing key",
			mutate:  func(p *SovereigntyPolicy) { p.SigningKey = []byte("short") },
			wantErr: ErrInvalidSigningKey,
		},
		{
			name:    "empty allowed domains",
			mutate:  func(p *SovereigntyPolicy) { p.AllowedDomains = nil },
			wantErr: ErrInvalidDestinationDomain,
		},
		{
			name:    "malformed destination domain",
			mutate:  func(p *SovereigntyPolicy) { p.AllowedDomains = []string{"bad domain!"} },
			wantErr: ErrInvalidDestinationDomain,
		},
		{
			name:    "empty transit rules",
			mutate:  func(p *SovereigntyPolicy) { p.Rules = nil },
			wantErr: ErrInvalidTransitPolicy,
		},
		{
			name: "invalid classification label",
			mutate: func(p *SovereigntyPolicy) {
				p.Rules = []TransitRule{{Classification: ClassificationLevel("top-secret"), AllowEgress: false}}
			},
			wantErr: ErrInvalidClassification,
		},
		{
			name: "restricted classification cannot allow egress",
			mutate: func(p *SovereigntyPolicy) {
				p.Rules = []TransitRule{{Classification: ClassificationRestricted, AllowEgress: true}}
			},
			wantErr: ErrInvalidTransitPolicy,
		},
		{
			name: "duplicate classification rule",
			mutate: func(p *SovereigntyPolicy) {
				p.Rules = append(p.Rules, TransitRule{Classification: ClassificationPublic, AllowEgress: true})
			},
			wantErr: ErrInvalidTransitPolicy,
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy := defaultTestSovereigntyPolicy()
			tc.mutate(&policy)
			gw, err := NewEgressGateway(policy)
			assert.Nil(t, gw)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// TestEgressGateway_OperationalDynamicProof verifies credential redaction, cryptographic token
// minting/gating, audit telemetry, and concurrent egress evaluation under race detection.
func TestEgressGateway_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	gw := newTestEgressGateway(t)

	// Public egress succeeds without a clearance token.
	pubDecision, err := gw.GateEgress(EgressRequest{
		ClusterID:      testEgressClusterID,
		Domain:         testAllowedDomainPrimary,
		Classification: ClassificationPublic,
		Payload:        testSafePayload,
	})
	require.NoError(t, err)
	assert.True(t, pubDecision.Allowed)
	assert.Equal(t, testSafePayload, pubDecision.RedactedPayload)
	assert.Equal(t, 0, pubDecision.RedactionCount)

	// Confidential egress redacts credentials and validates bound clearance token.
	pemStart := pemBeginHeader + pemPrivateKeySuffix
	pemEnd := pemEndFooter + pemPrivateKeySuffix
	privKeyPayload := testSecretPayload + " " + pemStart + "\nSYNTHETIC_TEST_BLOCK\n" + pemEnd
	tok, err := gw.MintClearanceToken(testAllowedDomainBackup, ClassificationConfidential, privKeyPayload, 5*time.Minute)
	require.NoError(t, err)

	confDecision, err := gw.GateEgress(EgressRequest{
		ClusterID:      testEgressClusterID,
		Domain:         testAllowedDomainBackup,
		Classification: ClassificationConfidential,
		Payload:        privKeyPayload,
		Token:          &tok,
	})
	require.NoError(t, err)
	assert.True(t, confDecision.Allowed)
	assert.Equal(t, 5, confDecision.RedactionCount)
	assert.NotContains(t, confDecision.RedactedPayload, "tok_abc123")
	assert.NotContains(t, confDecision.RedactedPayload, "123-45-6789")
	assert.NotContains(t, confDecision.RedactedPayload, "BEGIN PRIVATE KEY")

	// Concurrent egress gating stress across worker goroutines.
	var allowedCounter atomic.Int64
	wg := goroutinelabels.StartTestGoroutines(t, 8, "egress_gateway_worker", "concurrent egress gating", func(id int) {
		payload := fmt.Sprintf("worker=%d status=ok secret=s3cr3t_%d", id, id)
		workerTok, mintErr := gw.MintClearanceToken(testAllowedDomainPrimary, ClassificationInternal, payload, time.Minute)
		assert.NoError(t, mintErr)

		dec, gateErr := gw.GateEgress(EgressRequest{
			ClusterID:      testEgressClusterID,
			Domain:         testAllowedDomainPrimary,
			Classification: ClassificationInternal,
			Payload:        payload,
			Token:          &workerTok,
		})
		assert.NoError(t, gateErr)
		if dec.Allowed {
			allowedCounter.Add(1)
		}
	})
	wg.Wait()

	assert.Equal(t, int64(8), allowedCounter.Load())
	telemetry := gw.Telemetry()
	assert.Equal(t, 10, telemetry.TotalRequests)
	assert.Equal(t, 10, telemetry.AllowedCount)
	assert.Equal(t, 0, telemetry.InterceptedCount)
	assert.Equal(t, 9, telemetry.RedactedCount)
	assert.Len(t, gw.AuditEvents(), 10)
	assert.Empty(t, gw.IntrusionAlerts())
}

// TestEgressGateway_NegativeAdversarialBoundary verifies fail-closed interception of restricted
// payloads, unauthorized domains, cluster mismatches, and forged/expired/mismatched tokens.
func TestEgressGateway_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	baseNow := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

	adversarialCases := []struct {
		name      string
		setupReq  func(t *testing.T, gw *EgressGateway) EgressRequest
		wantErr   error
		wantAlert bool
	}{
		{
			name: "restricted classification intercepted",
			setupReq: func(_ *testing.T, _ *EgressGateway) EgressRequest {
				return EgressRequest{
					ClusterID:      testEgressClusterID,
					Domain:         testAllowedDomainPrimary,
					Classification: ClassificationRestricted,
					Payload:        testSafePayload,
				}
			},
			wantErr:   ErrRestrictedEgress,
			wantAlert: true,
		},
		{
			name: "unauthorized destination domain intercepted",
			setupReq: func(_ *testing.T, _ *EgressGateway) EgressRequest {
				return EgressRequest{
					ClusterID:      testEgressClusterID,
					Domain:         testUnauthorizedDomain,
					Classification: ClassificationPublic,
					Payload:        testSafePayload,
				}
			},
			wantErr:   ErrUnauthorizedDomain,
			wantAlert: true,
		},
		{
			name: "foreign cluster mismatch intercepted",
			setupReq: func(_ *testing.T, _ *EgressGateway) EgressRequest {
				return EgressRequest{
					ClusterID:      testForeignClusterID,
					Domain:         testAllowedDomainPrimary,
					Classification: ClassificationPublic,
					Payload:        testSafePayload,
				}
			},
			wantErr:   ErrClusterMismatch,
			wantAlert: true,
		},
		{
			name: "missing clearance token for confidential payload",
			setupReq: func(_ *testing.T, _ *EgressGateway) EgressRequest {
				return EgressRequest{
					ClusterID:      testEgressClusterID,
					Domain:         testAllowedDomainPrimary,
					Classification: ClassificationConfidential,
					Payload:        testSafePayload,
				}
			},
			wantErr:   ErrInvalidEgressToken,
			wantAlert: true,
		},
		{
			name: "forged clearance token signature",
			setupReq: func(t *testing.T, gw *EgressGateway) EgressRequest {
				tok, err := gw.MintClearanceToken(testAllowedDomainPrimary, ClassificationConfidential, testSafePayload, time.Minute)
				require.NoError(t, err)
				tok.Signature = "deadbeefcafebabe00112233445566778899aabbccddeeff0011223344556677"
				return EgressRequest{
					ClusterID:      testEgressClusterID,
					Domain:         testAllowedDomainPrimary,
					Classification: ClassificationConfidential,
					Payload:        testSafePayload,
					Token:          &tok,
				}
			},
			wantErr:   ErrInvalidEgressToken,
			wantAlert: true,
		},
		{
			name: "expired clearance token",
			setupReq: func(t *testing.T, gw *EgressGateway) EgressRequest {
				gw.nowFn = func() time.Time { return baseNow.Add(-10 * time.Minute) }
				tok, err := gw.MintClearanceToken(testAllowedDomainPrimary, ClassificationConfidential, testSafePayload, time.Minute)
				require.NoError(t, err)
				gw.nowFn = func() time.Time { return baseNow }
				return EgressRequest{
					ClusterID:      testEgressClusterID,
					Domain:         testAllowedDomainPrimary,
					Classification: ClassificationConfidential,
					Payload:        testSafePayload,
					Token:          &tok,
				}
			},
			wantErr:   ErrExpiredEgressToken,
			wantAlert: true,
		},
		{
			name: "payload digest mismatch",
			setupReq: func(t *testing.T, gw *EgressGateway) EgressRequest {
				tok, err := gw.MintClearanceToken(testAllowedDomainPrimary, ClassificationConfidential, testSafePayload, time.Minute)
				require.NoError(t, err)
				return EgressRequest{
					ClusterID:      testEgressClusterID,
					Domain:         testAllowedDomainPrimary,
					Classification: ClassificationConfidential,
					Payload:        "tampered outbound payload",
					Token:          &tok,
				}
			},
			wantErr:   ErrPayloadDigestMismatch,
			wantAlert: true,
		},
	}

	for _, tc := range adversarialCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gw := newTestEgressGateway(t)
			gw.nowFn = func() time.Time { return baseNow }

			req := tc.setupReq(t, gw)
			dec, err := gw.GateEgress(req)
			assert.False(t, dec.Allowed)
			assert.ErrorIs(t, err, tc.wantErr)

			telemetry := gw.Telemetry()
			assert.Equal(t, 1, telemetry.InterceptedCount)
			assert.Equal(t, 0, telemetry.AllowedCount)
			if tc.wantAlert {
				assert.Equal(t, 1, telemetry.AlertsCount)
				assert.Len(t, gw.IntrusionAlerts(), 1)
			}
		})
	}

	// Minting a clearance token for restricted classification must also fail closed.
	gw := newTestEgressGateway(t)
	_, mintErr := gw.MintClearanceToken(testAllowedDomainPrimary, ClassificationRestricted, testSafePayload, time.Minute)
	assert.ErrorIs(t, mintErr, ErrRestrictedEgress)
}
