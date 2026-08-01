package federation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/errfmt"
)

const (
	// KernelIDPrefix is the canonical prefix for kernel identifiers.
	KernelIDPrefix = "KER"
	// PublicKeyPrefix is the canonical prefix for public keys in the mesh.
	PublicKeyPrefix = "PUB"
)

// IdentityManager manages the local kernel's identity and credentials.
type IdentityManager struct {
	projectRoot string
}

// NewIdentityManager creates a new IdentityManager.
func NewIdentityManager(projectRoot string) *IdentityManager {
	return &IdentityManager{
		projectRoot: projectRoot,
	}
}

// GetKernelID returns the unique identifier for the local kernel.
// It is derived from the project root hash to ensure it's stable and unique per installation.
func (m *IdentityManager) GetKernelID() (string, error) {
	absRoot, err := filepath.Abs(m.projectRoot)
	if err != nil {
		return "", errfmt.Newf("failed to get absolute project root").Wrap(err)
	}

	// Simple deterministic ID based on project root hash
	hash := sha256.Sum256([]byte(absRoot))
	shortHash := fmt.Sprintf("%x", hash)[:12]

	return fmt.Sprintf("%s-%s-%s",
		KernelIDPrefix,
		strings.ToUpper(brand.NamespacePrefix()),
		strings.ToUpper(shortHash)), nil
}

// GetPublicKey returns the public key for the local kernel.
func (m *IdentityManager) GetPublicKey() (string, error) {
	// Future: Load from pkg/keystore or secure vault.
	// For now, derive from hostname for uniqueness in local mesh tests.
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Normalize hostname for key format
	hostname = strings.ReplaceAll(hostname, ".", "-")

	return fmt.Sprintf("%s-%s-%s",
		PublicKeyPrefix,
		strings.ToUpper(brand.NamespacePrefix()),
		strings.ToUpper(hostname)), nil
}

// Global provider for dependency injection
var globalIdentityManagerProvider = func() *IdentityManager {
	return NewIdentityManager("")
}

// RegisterIdentityManagerProvider registers a custom provider.
func RegisterIdentityManagerProvider(p func() *IdentityManager) {
	globalIdentityManagerProvider = p
}

// GetIdentityManager returns an IdentityManager via the registered provider.
func GetIdentityManager() *IdentityManager {
	if globalIdentityManagerProvider == nil {
		return NewIdentityManager("")
	}
	return globalIdentityManagerProvider()
}
