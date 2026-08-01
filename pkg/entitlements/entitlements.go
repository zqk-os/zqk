package entitlements

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/license"
)

type Tier string

const (
	TierCommunity  Tier = "community"
	TierEnterprise Tier = "enterprise"
)

// EntitlementChecker defines the interface for verifying platform tier capabilities.
type EntitlementChecker interface {
	CheckEvolveLimit(ctx context.Context, limit int) error
	CheckMeshEntitlement(ctx context.Context) error
	CheckBundle(ctx context.Context, bundle string) error
}

// CommunityChecker implements the default free community-tier capabilities and limits.
type CommunityChecker struct{}

func (c *CommunityChecker) CheckEvolveLimit(ctx context.Context, limit int) error {
	if limit > 3 {
		return errfmt.Errorf("community tier is limited to 3 evolution candidates per cycle (requested %d). Upgrade to Enterprise for unbounded autonomous refactoring.", limit)
	}
	return nil
}

func (c *CommunityChecker) CheckMeshEntitlement(ctx context.Context) error {
	return errfmt.Errorf("autonomous mesh networking is an Enterprise feature. Please upgrade your license.")
}

func (c *CommunityChecker) CheckBundle(ctx context.Context, bundle string) error {
	if bundle == "mesh" {
		return c.CheckMeshEntitlement(ctx)
	}
	// Add other bundles here as they are discovered
	return errfmt.Errorf("bundle '%s' requires an active subscription or an Enterprise license.", bundle)
}

// EnterpriseChecker implements the enterprise capabilities (unbounded limits).
type EnterpriseChecker struct {
	Features []string
}

func (e *EnterpriseChecker) CheckEvolveLimit(ctx context.Context, limit int) error {
	return nil
}

func (e *EnterpriseChecker) CheckMeshEntitlement(ctx context.Context) error {
	// Alternatively, verify that "mesh" is explicitly in Features, but Enterprise unlocks all.
	return nil
}

func (e *EnterpriseChecker) CheckBundle(ctx context.Context, bundle string) error {
	// Enterprise unlocks all bundles
	return nil
}

var globalChecker EntitlementChecker

func init() {
	// Initialize default
	globalChecker = &CommunityChecker{}
	LoadLicense()
}

// DefaultPublicKeyHex is the Ed25519 public key used to verify ZQK JWT licenses.
var DefaultPublicKeyHex = "f670b62c49174106c5787f90f1d7be0a71e166b3bfe316fd828bbdc27310cb1b"

func LoadLicense() {
	// 1. Check for license in environment variable
	if licenseStr := os.Getenv(brand.DefaultEnvPrefix + "_LICENSE_KEY"); licenseStr != "" {
		globalChecker = &EnterpriseChecker{}
		return
	}

	// 2. Check for cryptographic license file
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	licensePath := filepath.Join(home, ".zqk-state", "license.jwt")
	tokenBytes, err := os.ReadFile(licensePath)
	if err != nil {
		// Fallback to checking ~/.zqk/license.jwt for backwards compatibility
		licensePath = filepath.Join(home, ".zqk", "license.jwt")
		tokenBytes, err = os.ReadFile(licensePath)
		if err != nil {
			return
		}
	}

	pubKeyBytes, err := hex.DecodeString(DefaultPublicKeyHex)
	if err != nil {
		return
	}

	validator := license.NewValidator(pubKeyBytes)
	claims, err := validator.Verify(string(tokenBytes))
	if err != nil {
		// Invalid or expired, keep CommunityChecker
		return
	}

	// Upgrade to EnterpriseChecker with features from claims
	globalChecker = &EnterpriseChecker{
		Features: claims.Features,
	}
}

// RegisterChecker registers a custom entitlement checker implementation.
func RegisterChecker(checker EntitlementChecker) {
	if checker != nil {
		globalChecker = checker
	}
}

// CheckEvolveEntitlement verifies if the current runtime is entitled to execute a high-frequency autonomous refactoring cycle.
func CheckEvolveEntitlement(ctx context.Context, limit int) error {
	return globalChecker.CheckEvolveLimit(ctx, limit)
}

// CheckMeshEntitlement verifies if the current runtime is entitled to use the Autonomous Mesh.
func CheckMeshEntitlement(ctx context.Context) error {
	return globalChecker.CheckMeshEntitlement(ctx)
}

// CheckEntitlementBundle verifies if the current runtime is entitled to the specified feature bundle.
func CheckEntitlementBundle(ctx context.Context, bundle string) error {
	return globalChecker.CheckBundle(ctx, bundle)
}
