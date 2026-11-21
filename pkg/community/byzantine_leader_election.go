// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	bftQuorumMultiplier = 3
	bftSupermajorityMul = 2
	minByzantineNodes   = 4
)

var (
	// ErrInvalidHeartbeatTimeout indicates that the configured heartbeat timeout is non-positive.
	ErrInvalidHeartbeatTimeout = errors.New("heartbeat timeout must be positive")

	// ErrInvalidFaultTolerance indicates that the configured fault tolerance f is negative.
	ErrInvalidFaultTolerance = errors.New("fault tolerance must be non-negative")

	// ErrInsufficientValidatorSet indicates that the validator count violates the 3f+1 BFT floor.
	ErrInsufficientValidatorSet = errors.New("validator set must satisfy 3f+1 Byzantine floor")

	// ErrBlankValidatorID indicates that a validator identifier is empty or whitespace.
	ErrBlankValidatorID = errors.New("validator ID cannot be blank")

	// ErrDuplicateValidatorID indicates that the validator list contains duplicate identifiers.
	ErrDuplicateValidatorID = errors.New("duplicate validator ID")

	// ErrInsufficientViewQuorum indicates that fewer than 2f+1 valid view-change claims were collected.
	ErrInsufficientViewQuorum = errors.New("insufficient supermajority view-change quorum")

	// ErrSplitBrainLeaderConflict indicates an attempt to assert or elect a conflicting leader in a view.
	ErrSplitBrainLeaderConflict = errors.New("split-brain leader conflict detected")

	// ErrStaleViewClaim indicates a view-change claim or certificate targeting a non-advancing view.
	ErrStaleViewClaim = errors.New("stale or downgrade view-change claim")

	// ErrInvalidStallProof indicates a missing, blank, or forged stall proof digest.
	ErrInvalidStallProof = errors.New("invalid or missing stall proof digest")

	// ErrUnknownValidator indicates a claim or leader from a node outside the validator set.
	ErrUnknownValidator = errors.New("unknown validator ID")

	// ErrInvalidCertificate indicates a new-view certificate with a mismatched or forged digest.
	ErrInvalidCertificate = errors.New("invalid new-view certificate digest")
)

// ElectionConfig configures an ElectionCoordinator for Byzantine leader election.
type ElectionConfig struct {
	ClusterID        string
	ValidatorIDs     []string
	HeartbeatTimeout time.Duration
	FaultTolerance   int
}

// ViewChangeClaim represents a validator's signed proof of leader stall for view progression.
type ViewChangeClaim struct {
	ClusterID        string
	CurrentView      uint64
	TargetView       uint64
	ValidatorID      string
	StallProofDigest string
	LastPreparedView uint64
	Timestamp        time.Time
}

// NewViewCertificate proves that a 2f+1 supermajority agreed to advance to View under LeaderID.
type NewViewCertificate struct {
	ClusterID         string
	View              uint64
	LeaderID          string
	Claims            []ViewChangeClaim
	QuorumCount       int
	CertificateDigest string
	IssuedAt          time.Time
}

// LeaderLease represents a time-bounded fencing lease bound to a certified view and leader.
type LeaderLease struct {
	ClusterID         string
	View              uint64
	LeaderID          string
	CertificateDigest string
	IssuedAt          time.Time
	ExpiresAt         time.Time
}

// ElectionCoordinator manages Byzantine view-change claims, deterministic leader rotation, and partition recovery.
type ElectionCoordinator struct {
	mu               sync.RWMutex
	clusterID        string
	validators       []string
	validatorSet     map[string]struct{}
	faultTolerance   int
	quorumSize       int
	heartbeatTimeout time.Duration
	currentView      uint64
	claimsByView     map[uint64]map[string]ViewChangeClaim
	activeCert       *NewViewCertificate
	activeLease      *LeaderLease
}

// ComputeStallProofDigest returns the deterministic SHA-256 stall proof digest for a view-change claim.
func ComputeStallProofDigest(clusterID string, fromView, targetView uint64, validatorID string) string {
	payload := fmt.Sprintf("stall|%s|%d|%d|%s", clusterID, fromView, targetView, validatorID)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// ComputeCertificateDigest computes the canonical SHA-256 digest over a NewViewCertificate's claims.
func ComputeCertificateDigest(clusterID string, view uint64, leaderID string, claims []ViewChangeClaim) string {
	sorted := cloneAndSortClaims(claims)
	var b strings.Builder
	fmt.Fprintf(&b, "cert|%s|%d|%s", clusterID, view, leaderID)
	for _, c := range sorted {
		fmt.Fprintf(&b, "|%s:%d:%s", c.ValidatorID, c.CurrentView, c.StallProofDigest)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// NewElectionCoordinator validates ElectionConfig and initializes a Byzantine ElectionCoordinator.
func NewElectionCoordinator(cfg ElectionConfig) (*ElectionCoordinator, error) {
	if strutil.IsBlank(cfg.ClusterID) {
		return nil, ErrBlankClusterID
	}
	if cfg.HeartbeatTimeout <= 0 {
		return nil, ErrInvalidHeartbeatTimeout
	}
	if cfg.FaultTolerance < 0 {
		return nil, ErrInvalidFaultTolerance
	}
	validators, vSet, err := validateValidators(cfg.ValidatorIDs)
	if err != nil {
		return nil, err
	}
	f, quorum, err := computeBFTQuorum(len(validators), cfg.FaultTolerance)
	if err != nil {
		return nil, err
	}
	return &ElectionCoordinator{
		clusterID:        cfg.ClusterID,
		validators:       validators,
		validatorSet:     vSet,
		faultTolerance:   f,
		quorumSize:       quorum,
		heartbeatTimeout: cfg.HeartbeatTimeout,
		claimsByView:     make(map[uint64]map[string]ViewChangeClaim),
	}, nil
}

func validateValidators(ids []string) ([]string, map[string]struct{}, error) {
	if len(ids) < minByzantineNodes {
		return nil, nil, ErrInsufficientValidatorSet
	}
	seen := make(map[string]struct{}, len(ids))
	ordered := make([]string, 0, len(ids))
	for _, id := range ids {
		if strutil.IsBlank(id) {
			return nil, nil, ErrBlankValidatorID
		}
		if _, dup := seen[id]; dup {
			return nil, nil, ErrDuplicateValidatorID
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}
	return ordered, seen, nil
}

func computeBFTQuorum(nodeCount, configuredF int) (int, int, error) {
	f := configuredF
	if f == 0 {
		f = (nodeCount - 1) / bftQuorumMultiplier
	}
	if f < 1 || nodeCount < bftQuorumMultiplier*f+1 {
		return 0, 0, ErrInsufficientValidatorSet
	}
	return f, bftSupermajorityMul*f + 1, nil
}

// QuorumSize returns the 2f+1 supermajority quorum threshold.
func (c *ElectionCoordinator) QuorumSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quorumSize
}

// FaultTolerance returns the tolerated Byzantine fault count f.
func (c *ElectionCoordinator) FaultTolerance() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.faultTolerance
}

// CurrentView returns the coordinator's current monotonic view number.
func (c *ElectionCoordinator) CurrentView() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentView
}

// LeaderForView deterministically maps a view number to a validator via round-robin rotation.
func (c *ElectionCoordinator) LeaderForView(view uint64) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leaderForViewLocked(view)
}

func (c *ElectionCoordinator) leaderForViewLocked(view uint64) string {
	idx := int(view % uint64(len(c.validators)))
	return c.validators[idx]
}

// ActiveCertificate returns a copy of the latest certified NewViewCertificate, if any.
func (c *ElectionCoordinator) ActiveCertificate() *NewViewCertificate {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeCert == nil {
		return nil
	}
	return cloneCertificate(c.activeCert)
}

// ActiveLease returns a copy of the latest issued LeaderLease, if any.
func (c *ElectionCoordinator) ActiveLease() *LeaderLease {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeLease == nil {
		return nil
	}
	leaseCopy := *c.activeLease
	return &leaseCopy
}

// BuildViewChangeClaim constructs a valid ViewChangeClaim for targetView from validatorID.
func (c *ElectionCoordinator) BuildViewChangeClaim(validatorID string, targetView uint64, now time.Time) (ViewChangeClaim, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.validatorSet[validatorID]; !ok {
		return ViewChangeClaim{}, ErrUnknownValidator
	}
	if targetView <= c.currentView {
		return ViewChangeClaim{}, ErrStaleViewClaim
	}
	ts := normalizeTimestamp(now)
	return ViewChangeClaim{
		ClusterID:        c.clusterID,
		CurrentView:      c.currentView,
		TargetView:       targetView,
		ValidatorID:      validatorID,
		StallProofDigest: ComputeStallProofDigest(c.clusterID, c.currentView, targetView, validatorID),
		LastPreparedView: c.currentView,
		Timestamp:        ts,
	}, nil
}

// SubmitViewChangeClaim validates a ViewChangeClaim and advances the view once 2f+1 claims are collected.
func (c *ElectionCoordinator) SubmitViewChangeClaim(claim ViewChangeClaim) (*NewViewCertificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.validateClaimLocked(claim, c.currentView); err != nil {
		return nil, err
	}
	viewClaims, exists := c.claimsByView[claim.TargetView]
	if !exists {
		viewClaims = make(map[string]ViewChangeClaim)
		c.claimsByView[claim.TargetView] = viewClaims
	}
	viewClaims[claim.ValidatorID] = claim

	if len(viewClaims) < c.quorumSize {
		return nil, nil
	}
	cert := c.promoteViewLocked(claim.TargetView, viewClaims, normalizeTimestamp(claim.Timestamp))
	return cloneCertificate(&cert), nil
}

// TryFormNewViewCertificate attempts to form a NewViewCertificate for targetView, failing if quorum is unmet.
func (c *ElectionCoordinator) TryFormNewViewCertificate(targetView uint64) (*NewViewCertificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if targetView <= c.currentView {
		if c.activeCert != nil && c.activeCert.View == targetView {
			return cloneCertificate(c.activeCert), nil
		}
		return nil, ErrStaleViewClaim
	}
	viewClaims := c.claimsByView[targetView]
	if len(viewClaims) < c.quorumSize {
		return nil, ErrInsufficientViewQuorum
	}
	cert := c.promoteViewLocked(targetView, viewClaims, time.Now().UTC())
	return cloneCertificate(&cert), nil
}

// AssertLeader verifies that leaderID is the sole deterministic leader for view with no split-brain conflict.
func (c *ElectionCoordinator) AssertLeader(view uint64, leaderID string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.checkLeaderAuthorityLocked(view, leaderID)
}

// IssueLeaderLease issues a fencing lease for leaderID in view if view is certified and split-brain-free.
func (c *ElectionCoordinator) IssueLeaderLease(view uint64, leaderID string, now time.Time) (*LeaderLease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkLeaderAuthorityLocked(view, leaderID); err != nil {
		return nil, err
	}
	if view > 0 && (c.activeCert == nil || c.activeCert.View < view) {
		return nil, ErrInsufficientViewQuorum
	}
	certDigest := ""
	if c.activeCert != nil && c.activeCert.View == view {
		certDigest = c.activeCert.CertificateDigest
	}
	lease := c.newLeaseLocked(view, leaderID, certDigest, now)
	c.activeLease = &lease
	leaseCopy := lease
	return &leaseCopy, nil
}

// ConvergeToCertificate fast-forwards a lagging partition coordinator to a verified NewViewCertificate.
func (c *ElectionCoordinator) ConvergeToCertificate(cert NewViewCertificate) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cert.ClusterID != c.clusterID {
		return ErrClusterMismatch
	}
	if cert.View <= c.currentView {
		return ErrStaleViewClaim
	}
	if cert.LeaderID != c.leaderForViewLocked(cert.View) {
		return ErrSplitBrainLeaderConflict
	}
	if err := c.verifyCertificateClaimsLocked(cert); err != nil {
		return err
	}
	expectedDigest := ComputeCertificateDigest(cert.ClusterID, cert.View, cert.LeaderID, cert.Claims)
	if cert.CertificateDigest != expectedDigest {
		return ErrInvalidCertificate
	}
	c.applyCertificateLocked(cert)
	return nil
}

func (c *ElectionCoordinator) validateClaimLocked(claim ViewChangeClaim, floorView uint64) error {
	if claim.ClusterID != c.clusterID {
		return ErrClusterMismatch
	}
	if _, ok := c.validatorSet[claim.ValidatorID]; !ok {
		return ErrUnknownValidator
	}
	if claim.TargetView <= floorView {
		return ErrStaleViewClaim
	}
	if strutil.IsBlank(claim.StallProofDigest) {
		return ErrInvalidStallProof
	}
	expected := ComputeStallProofDigest(c.clusterID, claim.CurrentView, claim.TargetView, claim.ValidatorID)
	if claim.StallProofDigest != expected {
		return ErrInvalidStallProof
	}
	return nil
}

func (c *ElectionCoordinator) checkLeaderAuthorityLocked(view uint64, leaderID string) error {
	if view < c.currentView {
		return ErrStaleViewClaim
	}
	if _, ok := c.validatorSet[leaderID]; !ok {
		return ErrUnknownValidator
	}
	if leaderID != c.leaderForViewLocked(view) {
		return ErrSplitBrainLeaderConflict
	}
	if c.activeLease != nil && c.activeLease.View == view && c.activeLease.LeaderID != leaderID {
		return ErrSplitBrainLeaderConflict
	}
	return nil
}

func (c *ElectionCoordinator) verifyCertificateClaimsLocked(cert NewViewCertificate) error {
	unique := make(map[string]struct{}, len(cert.Claims))
	for _, claim := range cert.Claims {
		if claim.TargetView != cert.View {
			return ErrStaleViewClaim
		}
		if err := c.validateClaimLocked(claim, c.currentView); err != nil {
			return err
		}
		unique[claim.ValidatorID] = struct{}{}
	}
	if len(unique) < c.quorumSize {
		return ErrInsufficientViewQuorum
	}
	return nil
}

func (c *ElectionCoordinator) promoteViewLocked(targetView uint64, viewClaims map[string]ViewChangeClaim, issuedAt time.Time) NewViewCertificate {
	claims := make([]ViewChangeClaim, 0, len(viewClaims))
	for _, cl := range viewClaims {
		claims = append(claims, cl)
	}
	sortedClaims := cloneAndSortClaims(claims)
	leader := c.leaderForViewLocked(targetView)
	cert := NewViewCertificate{
		ClusterID:         c.clusterID,
		View:              targetView,
		LeaderID:          leader,
		Claims:            sortedClaims,
		QuorumCount:       len(sortedClaims),
		CertificateDigest: ComputeCertificateDigest(c.clusterID, targetView, leader, sortedClaims),
		IssuedAt:          issuedAt,
	}
	c.applyCertificateLocked(cert)
	return cert
}

func (c *ElectionCoordinator) applyCertificateLocked(cert NewViewCertificate) {
	certCopy := cloneCertificate(&cert)
	c.currentView = cert.View
	c.activeCert = certCopy
	lease := c.newLeaseLocked(cert.View, cert.LeaderID, cert.CertificateDigest, cert.IssuedAt)
	c.activeLease = &lease
}

func (c *ElectionCoordinator) newLeaseLocked(view uint64, leaderID, certDigest string, issuedAt time.Time) LeaderLease {
	ts := normalizeTimestamp(issuedAt)
	return LeaderLease{
		ClusterID:         c.clusterID,
		View:              view,
		LeaderID:          leaderID,
		CertificateDigest: certDigest,
		IssuedAt:          ts,
		ExpiresAt:         ts.Add(c.heartbeatTimeout),
	}
}

func cloneAndSortClaims(claims []ViewChangeClaim) []ViewChangeClaim {
	out := make([]ViewChangeClaim, len(claims))
	copy(out, claims)
	sort.Slice(out, func(i, j int) bool {
		return out[i].ValidatorID < out[j].ValidatorID
	})
	return out
}

func cloneCertificate(cert *NewViewCertificate) *NewViewCertificate {
	if cert == nil {
		return nil
	}
	cp := *cert
	cp.Claims = cloneAndSortClaims(cert.Claims)
	return &cp
}

func normalizeTimestamp(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now().UTC()
	}
	return ts.UTC()
}
