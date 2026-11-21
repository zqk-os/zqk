package community

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrInvalidTenantConfig indicates invalid tenant, namespace, or quota parameters.
	ErrInvalidTenantConfig = errors.New("invalid tenant namespace configuration")
	// ErrTenantNotFound indicates the requested tenant namespace is not registered.
	ErrTenantNotFound = errors.New("tenant namespace not found")
	// ErrTenantAlreadyExists indicates a tenant or namespace ID is already registered.
	ErrTenantAlreadyExists = errors.New("tenant namespace already exists")
	// ErrTenantImpersonation indicates a forged, invalid, or mismatched tenant session token.
	ErrTenantImpersonation = errors.New("tenant impersonation detected")
	// ErrNamespaceBoundaryViolation indicates an unauthorized cross-namespace access attempt.
	ErrNamespaceBoundaryViolation = errors.New("namespace boundary violation")
	// ErrConcurrencyLimitReached indicates the tenant's max concurrent operations limit is reached.
	ErrConcurrencyLimitReached = errors.New("tenant concurrency limit reached")
	// ErrInvalidLease indicates an unknown, expired, or mismatched namespace lease.
	ErrInvalidLease = errors.New("invalid namespace lease")
)

const (
	hmacKeyBytes = 32
	nonceBytes   = 16
	tokenParts   = 4
)

// TenantQuota defines the concurrency and token consumption limits for a tenant namespace.
type TenantQuota struct {
	MaxConcurrentOps int
	MaxTokenQuota    int64
}

// Validate verifies that the quota satisfies positive floor invariants.
func (q TenantQuota) Validate() error {
	if q.MaxConcurrentOps <= 0 || q.MaxTokenQuota <= 0 {
		return fmt.Errorf("%w: MaxConcurrentOps and MaxTokenQuota must be positive", ErrInvalidTenantConfig)
	}
	return nil
}

// TenantUsageTelemetry captures a point-in-time snapshot of tenant resource usage and rejections.
type TenantUsageTelemetry struct {
	TenantID            string
	NamespaceID         string
	ActiveOps           int
	MaxConcurrentOps    int
	TokensUsed          int64
	TokensRemaining     int64
	MaxTokenQuota       int64
	TotalAcquires       int64
	TotalReleases       int64
	RejectedConcurrency int64
	RejectedQuota       int64
	RejectedBoundary    int64
}

// NamespaceLease represents an active concurrency slot lease within a tenant namespace.
type NamespaceLease struct {
	LeaseID     string
	TenantID    string
	NamespaceID string
	AcquiredAt  time.Time
}

// TenantNamespace represents an isolated, quota-governed execution and storage partition for a tenant.
type TenantNamespace struct {
	TenantID            string
	NamespaceID         string
	Quota               TenantQuota
	activeOps           int
	tokensUsed          int64
	totalAcquires       int64
	totalReleases       int64
	rejectedConcurrency int64
	rejectedQuota       int64
	rejectedBoundary    int64
	activeLeases        map[string]NamespaceLease
	objects             map[string]string
}

// TenantNamespaceManager enforces sovereign multi-tenant namespace partitioning,
// resource quota governance, concurrency throttling, and cryptographic session binding.
type TenantNamespaceManager struct {
	mu         sync.RWMutex
	hmacKey    []byte
	tenants    map[string]*TenantNamespace
	namespaces map[string]string
}

// NewTenantNamespaceManager initializes a manager with a cryptographically random HMAC key.
func NewTenantNamespaceManager() (*TenantNamespaceManager, error) {
	key := make([]byte, hmacKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to initialize tenant HMAC key: %w", err)
	}
	return &TenantNamespaceManager{
		hmacKey:    key,
		tenants:    make(map[string]*TenantNamespace),
		namespaces: make(map[string]string),
	}, nil
}

// RegisterTenant provisions a sovereign namespace partition for a tenant with the given quota.
func (m *TenantNamespaceManager) RegisterTenant(tenantID, namespaceID string, quota TenantQuota) (*TenantNamespace, error) {
	if err := validateTenantNamespaceIDs(tenantID, namespaceID); err != nil {
		return nil, err
	}
	if err := quota.Validate(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tenants[tenantID]; exists {
		return nil, fmt.Errorf("%w: tenant %q", ErrTenantAlreadyExists, tenantID)
	}
	if owner, claimed := m.namespaces[namespaceID]; claimed && owner != tenantID {
		return nil, fmt.Errorf("%w: namespace %q is bound to another tenant", ErrNamespaceBoundaryViolation, namespaceID)
	}

	ns := &TenantNamespace{
		TenantID:     tenantID,
		NamespaceID:  namespaceID,
		Quota:        quota,
		activeLeases: make(map[string]NamespaceLease),
		objects:      make(map[string]string),
	}
	m.tenants[tenantID] = ns
	m.namespaces[namespaceID] = tenantID

	return &TenantNamespace{
		TenantID:    ns.TenantID,
		NamespaceID: ns.NamespaceID,
		Quota:       ns.Quota,
	}, nil
}

// IssueSessionToken mints an HMAC-SHA256 bound session token for a registered tenant and namespace.
func (m *TenantNamespaceManager) IssueSessionToken(tenantID, namespaceID string) (string, error) {
	if err := validateTenantNamespaceIDs(tenantID, namespaceID); err != nil {
		return "", err
	}

	var token string
	err := m.withRLockedNamespace(tenantID, func(ns *TenantNamespace) error {
		if ns.NamespaceID != namespaceID {
			return fmt.Errorf("%w: tenant %q does not own namespace %q", ErrNamespaceBoundaryViolation, tenantID, namespaceID)
		}
		nonce, genErr := randomHex(nonceBytes)
		if genErr != nil {
			return genErr
		}
		sig, sigErr := m.signPayload(tenantID, namespaceID, nonce)
		if sigErr != nil {
			return sigErr
		}
		token = strings.Join([]string{tenantID, namespaceID, nonce, sig}, "|")
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// ValidateAccess verifies that the session token is cryptographically authentic and
// authorized for the target tenant and namespace.
func (m *TenantNamespaceManager) ValidateAccess(token, tenantID, namespaceID string) error {
	return m.withValidatedAccess(token, tenantID, namespaceID, func(_ *TenantNamespace) error {
		return nil
	})
}

// AcquireSlot reserves a concurrency slot for the authenticated tenant namespace.
func (m *TenantNamespaceManager) AcquireSlot(token, tenantID, namespaceID string) (NamespaceLease, error) {
	var lease NamespaceLease
	err := m.withValidatedAccess(token, tenantID, namespaceID, func(ns *TenantNamespace) error {
		if ns.activeOps >= ns.Quota.MaxConcurrentOps {
			ns.rejectedConcurrency++
			return fmt.Errorf("%w: tenant %q at %d/%d active ops", ErrConcurrencyLimitReached, tenantID, ns.activeOps, ns.Quota.MaxConcurrentOps)
		}
		leaseID, genErr := randomHex(nonceBytes)
		if genErr != nil {
			return genErr
		}
		lease = NamespaceLease{
			LeaseID:     leaseID,
			TenantID:    tenantID,
			NamespaceID: namespaceID,
			AcquiredAt:  time.Now().UTC(),
		}
		ns.activeOps++
		ns.totalAcquires++
		ns.activeLeases[leaseID] = lease
		return nil
	})
	if err != nil {
		return NamespaceLease{}, err
	}
	return lease, nil
}

// ConsumeTokens deducts tokens from the tenant's quota under an active lease.
func (m *TenantNamespaceManager) ConsumeTokens(token string, lease NamespaceLease, tokens int64) error {
	if tokens <= 0 {
		return fmt.Errorf("%w: token consumption must be positive", ErrInvalidTenantConfig)
	}
	return m.withValidatedLease(token, lease, func(ns *TenantNamespace) error {
		if tokens > ns.Quota.MaxTokenQuota-ns.tokensUsed {
			ns.rejectedQuota++
			return fmt.Errorf("%w: requested %d tokens exceeds remaining %d", ErrQuotaExceeded, tokens, ns.Quota.MaxTokenQuota-ns.tokensUsed)
		}
		ns.tokensUsed += tokens
		return nil
	})
}

// ReleaseSlot releases an active concurrency lease for the authenticated tenant namespace.
func (m *TenantNamespaceManager) ReleaseSlot(token string, lease NamespaceLease) error {
	return m.withValidatedLease(token, lease, func(ns *TenantNamespace) error {
		delete(ns.activeLeases, lease.LeaseID)
		if ns.activeOps > 0 {
			ns.activeOps--
		}
		ns.totalReleases++
		return nil
	})
}

// WriteObject stores a key-value entry inside the tenant's sovereign namespace partition.
func (m *TenantNamespaceManager) WriteObject(token, tenantID, namespaceID, key, value string) error {
	if err := strutil.ErrorOnBlank(key, "%w: object key cannot be blank", ErrInvalidTenantConfig); err != nil {
		return err
	}
	return m.withValidatedAccess(token, tenantID, namespaceID, func(ns *TenantNamespace) error {
		ns.objects[key] = value
		return nil
	})
}

// ReadObject retrieves a key-value entry from the tenant's sovereign namespace partition.
func (m *TenantNamespaceManager) ReadObject(token, tenantID, namespaceID, key string) (string, error) {
	if err := strutil.ErrorOnBlank(key, "%w: object key cannot be blank", ErrInvalidTenantConfig); err != nil {
		return "", err
	}
	var value string
	err := m.withValidatedAccess(token, tenantID, namespaceID, func(ns *TenantNamespace) error {
		val, exists := ns.objects[key]
		if !exists {
			return fmt.Errorf("%w: key %q", ErrObjectNotFound, key)
		}
		value = val
		return nil
	})
	if err != nil {
		return "", err
	}
	return value, nil
}

// Telemetry returns a thread-safe snapshot of resource usage and rejection counters for a tenant.
func (m *TenantNamespaceManager) Telemetry(tenantID string) (TenantUsageTelemetry, error) {
	if err := strutil.ErrorOnBlank(tenantID, "%w: tenant_id cannot be blank", ErrInvalidTenantConfig); err != nil {
		return TenantUsageTelemetry{}, err
	}

	var snapshot TenantUsageTelemetry
	err := m.withRLockedNamespace(tenantID, func(ns *TenantNamespace) error {
		snapshot = TenantUsageTelemetry{
			TenantID:            ns.TenantID,
			NamespaceID:         ns.NamespaceID,
			ActiveOps:           ns.activeOps,
			MaxConcurrentOps:    ns.Quota.MaxConcurrentOps,
			TokensUsed:          ns.tokensUsed,
			TokensRemaining:     ns.Quota.MaxTokenQuota - ns.tokensUsed,
			MaxTokenQuota:       ns.Quota.MaxTokenQuota,
			TotalAcquires:       ns.totalAcquires,
			TotalReleases:       ns.totalReleases,
			RejectedConcurrency: ns.rejectedConcurrency,
			RejectedQuota:       ns.rejectedQuota,
			RejectedBoundary:    ns.rejectedBoundary,
		}
		return nil
	})
	if err != nil {
		return TenantUsageTelemetry{}, err
	}
	return snapshot, nil
}

func (m *TenantNamespaceManager) withRLockedNamespace(tenantID string, fn func(ns *TenantNamespace) error) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ns, exists := m.tenants[tenantID]
	if !exists {
		return fmt.Errorf("%w: tenant %q", ErrTenantNotFound, tenantID)
	}
	return fn(ns)
}

func (m *TenantNamespaceManager) withValidatedAccess(token, tenantID, namespaceID string, fn func(ns *TenantNamespace) error) error {
	if err := strutil.ErrorOnBlank(token, "%w: session token cannot be blank", ErrTenantImpersonation); err != nil {
		return err
	}
	if err := validateTenantNamespaceIDs(tenantID, namespaceID); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	ns, err := m.validateAccessLocked(token, tenantID, namespaceID)
	if err != nil {
		return err
	}
	return fn(ns)
}

func (m *TenantNamespaceManager) withValidatedLease(token string, lease NamespaceLease, fn func(ns *TenantNamespace) error) error {
	return m.withValidatedAccess(token, lease.TenantID, lease.NamespaceID, func(ns *TenantNamespace) error {
		if err := validateLeaseLocked(ns, lease); err != nil {
			return err
		}
		return fn(ns)
	})
}

func (m *TenantNamespaceManager) validateAccessLocked(token, tenantID, namespaceID string) (*TenantNamespace, error) {
	parts := strings.Split(token, "|")
	if len(parts) != tokenParts {
		return nil, fmt.Errorf("%w: malformed session token", ErrTenantImpersonation)
	}
	tokTenant, tokNamespace, nonce, sigHex := parts[0], parts[1], parts[2], parts[3]
	expectedSig, err := m.signPayload(tokTenant, tokNamespace, nonce)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal([]byte(sigHex), []byte(expectedSig)) {
		return nil, fmt.Errorf("%w: invalid token signature", ErrTenantImpersonation)
	}
	if tokTenant != tenantID {
		return nil, fmt.Errorf("%w: token bound to %q cannot act as %q", ErrTenantImpersonation, tokTenant, tenantID)
	}
	if tokNamespace != namespaceID {
		if ns, ok := m.tenants[tokTenant]; ok {
			ns.rejectedBoundary++
		}
		return nil, fmt.Errorf("%w: token bound to namespace %q cannot access %q", ErrNamespaceBoundaryViolation, tokNamespace, namespaceID)
	}

	ns, exists := m.tenants[tenantID]
	if !exists {
		return nil, fmt.Errorf("%w: tenant %q", ErrTenantNotFound, tenantID)
	}
	if ns.NamespaceID != namespaceID {
		ns.rejectedBoundary++
		return nil, fmt.Errorf("%w: tenant %q does not own namespace %q", ErrNamespaceBoundaryViolation, tenantID, namespaceID)
	}
	return ns, nil
}

func (m *TenantNamespaceManager) signPayload(tenantID, namespaceID, nonce string) (string, error) {
	mac := hmac.New(sha256.New, m.hmacKey)
	if _, err := mac.Write([]byte(tenantID + "|" + namespaceID + "|" + nonce)); err != nil {
		return "", fmt.Errorf("failed to compute HMAC signature: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func validateTenantNamespaceIDs(tenantID, namespaceID string) error {
	if err := strutil.ErrorOnBlank(tenantID, "%w: tenant_id cannot be blank", ErrInvalidTenantConfig); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(namespaceID, "%w: namespace_id cannot be blank", ErrInvalidTenantConfig); err != nil {
		return err
	}
	return nil
}

func validateLeaseLocked(ns *TenantNamespace, lease NamespaceLease) error {
	if err := strutil.ErrorOnBlank(lease.LeaseID, "%w: lease_id cannot be blank", ErrInvalidLease); err != nil {
		return err
	}
	stored, exists := ns.activeLeases[lease.LeaseID]
	if !exists || stored.TenantID != lease.TenantID || stored.NamespaceID != lease.NamespaceID {
		return fmt.Errorf("%w: lease %q is not active", ErrInvalidLease, lease.LeaseID)
	}
	return nil
}

func randomHex(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cryptographic entropy failure: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
