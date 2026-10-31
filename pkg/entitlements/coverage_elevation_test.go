package entitlements

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/license"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func generateEntitlementTestKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate ed25519 keys: %v", err)
	}
	return pub, priv
}

func createSignedLicenseToken(t *testing.T, priv ed25519.PrivateKey, features []string, exp time.Time) string {
	claims := license.Claims{
		Features: features,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			Issuer:    "zqk-billing",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestEntitlementTiersAndConstants(t *testing.T) {
	if TierCommunity != "community" {
		t.Errorf("expected TierCommunity to be 'community', got %s", TierCommunity)
	}
	if TierEnterprise != "enterprise" {
		t.Errorf("expected TierEnterprise to be 'enterprise', got %s", TierEnterprise)
	}
	if BundleElevatedObject != "elevated_object" {
		t.Errorf("expected BundleElevatedObject to be 'elevated_object', got %s", BundleElevatedObject)
	}
}

func TestLoadLicense_ValidCryptoToken_StateDir(t *testing.T) {
	priorChecker := CurrentCheckerForTest()
	priorKey := DefaultPublicKeyHex
	t.Cleanup(func() {
		RegisterChecker(priorChecker)
		DefaultPublicKeyHex = priorKey
		_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")
	})
	_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")

	pub, priv := generateEntitlementTestKeys(t)
	DefaultPublicKeyHex = hex.EncodeToString(pub)

	home := t.TempDir()
	t.Setenv("HOME", home)

	stateDir := filepath.Join(home, paths.ProjectStateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	tokenStr := createSignedLicenseToken(t, priv, []string{"mesh", "unbounded_evolve"}, time.Now().Add(24*time.Hour))
	if err := fileutil.WriteFile(filepath.Join(stateDir, "license.jwt"), []byte(tokenStr), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write license file: %v", err)
	}

	RegisterChecker(&CommunityChecker{})
	LoadLicense()

	activeChecker := CurrentCheckerForTest()
	entChecker, ok := activeChecker.(*EnterpriseChecker)
	if !ok {
		t.Fatalf("expected EnterpriseChecker after loading valid cryptographic license, got %T", activeChecker)
	}
	if len(entChecker.Features) != 2 || entChecker.Features[0] != "mesh" || entChecker.Features[1] != "unbounded_evolve" {
		t.Fatalf("unexpected features loaded in EnterpriseChecker: %v", entChecker.Features)
	}
}

func TestLoadLicense_ValidCryptoToken_FallbackDataDir(t *testing.T) {
	priorChecker := CurrentCheckerForTest()
	priorKey := DefaultPublicKeyHex
	t.Cleanup(func() {
		RegisterChecker(priorChecker)
		DefaultPublicKeyHex = priorKey
		_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")
	})
	_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")

	pub, priv := generateEntitlementTestKeys(t)
	DefaultPublicKeyHex = hex.EncodeToString(pub)

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Do NOT create license in ProjectStateDir; create only in ProjectDataDir (~/.zqk/license.jwt)
	dataDir := filepath.Join(home, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(dataDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	tokenStr := createSignedLicenseToken(t, priv, []string{"enterprise_all"}, time.Now().Add(12*time.Hour))
	if err := fileutil.WriteFile(filepath.Join(dataDir, "license.jwt"), []byte(tokenStr), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write license file to dataDir: %v", err)
	}

	RegisterChecker(&CommunityChecker{})
	LoadLicense()

	activeChecker := CurrentCheckerForTest()
	entChecker, ok := activeChecker.(*EnterpriseChecker)
	if !ok {
		t.Fatalf("expected fallback dataDir to load EnterpriseChecker, got %T", activeChecker)
	}
	if len(entChecker.Features) != 1 || entChecker.Features[0] != "enterprise_all" {
		t.Fatalf("unexpected features: %v", entChecker.Features)
	}
}

func TestLoadLicense_InvalidHexPublicKey(t *testing.T) {
	priorChecker := CurrentCheckerForTest()
	priorKey := DefaultPublicKeyHex
	t.Cleanup(func() {
		RegisterChecker(priorChecker)
		DefaultPublicKeyHex = priorKey
		_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")
	})
	_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")

	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(home, paths.ProjectStateDir)
	_ = fileutil.MkdirAll(stateDir, paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(stateDir, "license.jwt"), []byte("some-jwt-token"), paths.FilePerm600)

	// Corrupt the public key hex string
	DefaultPublicKeyHex = "not-a-valid-hex-key"

	RegisterChecker(&CommunityChecker{})
	LoadLicense()

	// Should safely return without upgrading
	if _, ok := CurrentCheckerForTest().(*CommunityChecker); !ok {
		t.Fatalf("expected CommunityChecker to remain active on invalid pubkey hex, got %T", CurrentCheckerForTest())
	}
}

func TestLoadLicense_NoLicenseFiles(t *testing.T) {
	priorChecker := CurrentCheckerForTest()
	t.Cleanup(func() {
		RegisterChecker(priorChecker)
		_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")
	})
	_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")

	home := t.TempDir()
	t.Setenv("HOME", home)

	RegisterChecker(&CommunityChecker{})
	LoadLicense()

	if _, ok := CurrentCheckerForTest().(*CommunityChecker); !ok {
		t.Fatalf("expected CommunityChecker to remain when no license files exist, got %T", CurrentCheckerForTest())
	}
}

func TestLoadLicense_UserHomeDirError(t *testing.T) {
	priorChecker := CurrentCheckerForTest()
	t.Cleanup(func() {
		RegisterChecker(priorChecker)
		_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")
	})
	_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")

	t.Setenv("HOME", "")
	RegisterChecker(&CommunityChecker{})
	LoadLicense()

	if _, ok := CurrentCheckerForTest().(*CommunityChecker); !ok {
		t.Fatalf("expected CommunityChecker when HOME is empty, got %T", CurrentCheckerForTest())
	}
}
