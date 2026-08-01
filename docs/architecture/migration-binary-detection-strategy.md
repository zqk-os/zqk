# Migration Binary Detection Strategy

**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Design  
**Related:** BLI-623, Migration Strategy v1.0

## Overview

This document defines how the zqk orchestrator detects and handles the optional migration binary. The migration tool is designed as a separate binary that can be excluded from mobile builds while remaining available for desktop/server deployments.

## Detection Strategies

### 1. Runtime Detection (Primary)

The orchestrator checks for the migration binary at runtime using standard OS utilities.

**Implementation Pattern** (similar to git hooks):
```go
package migration

import (
    "os/exec"
    "path/filepath"
)

// MigrationBinaryDetector detects if the migration binary is available
type MigrationBinaryDetector struct {
    binaryName string
    searchPaths []string
}

func NewMigrationBinaryDetector() *MigrationBinaryDetector {
    return &MigrationBinaryDetector{
        binaryName: "zqk-migrate",
        searchPaths: []string{
            // Check in same directory as main binary
            filepath.Dir(os.Executable()),
            // Check in PATH
            "", // empty string means use PATH
            // Check in common installation paths
            "/usr/local/bin",
            "/opt/zqk/bin",
        },
    }
}

// IsAvailable checks if the migration binary is available
func (d *MigrationBinaryDetector) IsAvailable() bool {
    // First, try to find in PATH (most common case)
    if path, err := exec.LookPath(d.binaryName); err == nil {
        // Verify it's actually executable
        if _, err := os.Stat(path); err == nil {
            return true
        }
    }
    
    // Check in explicit search paths
    for _, searchPath := range d.searchPaths {
        var fullPath string
        if searchPath == "" {
            // Use PATH
            if path, err := exec.LookPath(d.binaryName); err == nil {
                fullPath = path
            } else {
                continue
            }
        } else {
            fullPath = filepath.Join(searchPath, d.binaryName)
        }
        
        if info, err := os.Stat(fullPath); err == nil {
            // Check if executable
            if info.Mode().Perm()&0111 != 0 {
                return true
            }
        }
    }
    
    return false
}

// GetPath returns the full path to the migration binary if available
func (d *MigrationBinaryDetector) GetPath() (string, error) {
    // Try PATH first
    if path, err := exec.LookPath(d.binaryName); err == nil {
        return path, nil
    }
    
    // Check explicit paths
    for _, searchPath := range d.searchPaths {
        if searchPath == "" {
            continue
        }
        fullPath := filepath.Join(searchPath, d.binaryName)
        if info, err := os.Stat(fullPath); err == nil {
            if info.Mode().Perm()&0111 != 0 {
                return fullPath, nil
            }
        }
    }
    
    return "", fmt.Errorf("migration binary '%s' not found", d.binaryName)
}
```

### 2. Build-Time Feature Flags (Secondary)

Use Go build tags to conditionally compile migration support.

**Build Tags**:
```go
// +build !mobile

package migration

// Migration support only compiled when not building for mobile
```

**Usage**:
```bash
# Desktop/Server build (includes migration)
go build -tags "!mobile" ./cmd/zqk

# Mobile build (excludes migration)
go build -tags "mobile" ./cmd/zqk
```

### 3. Capability Interface (Tertiary)

Similar to `GraphProvider.SupportsFeature()`, provide a capability check interface.

```go
package migration

// MigrationCapabilities describes migration tool capabilities
type MigrationCapabilities struct {
    Available        bool
    BinaryPath       string
    Version          string
    SupportedPhases  []string
    SupportedBackends []string
}

// GetCapabilities returns migration tool capabilities
func GetCapabilities() MigrationCapabilities {
    detector := NewMigrationBinaryDetector()
    available := detector.IsAvailable()
    
    caps := MigrationCapabilities{
        Available: available,
        SupportedPhases: []string{"document", "entity", "relationship"},
        SupportedBackends: []string{"file", "memgraph", "neo4j"},
    }
    
    if available {
        if path, err := detector.GetPath(); err == nil {
            caps.BinaryPath = path
            // Try to get version
            if version, err := getBinaryVersion(path); err == nil {
                caps.Version = version
            }
        }
    }
    
    return caps
}

// SupportsMigration checks if migration is supported
func SupportsMigration() bool {
    return GetCapabilities().Available
}
```

## Integration with Orchestrator

### CLI Command Integration

```go
// cmd/zqk/migrate.go
package main

import (
    "github.com/lanceman/zqk/pkg/migration"
)

var migrateCmd = &cobra.Command{
    Use:   "migrate",
    Short: "Migrate data between backends",
    Long: `Migrate data between file-based and graph backends.
    
This command requires the zqk-migrate binary to be installed.
Install with: go install github.com/lanceman/zqk/cmd/zqk-migrate@latest`,
    RunE: func(cmd *cobra.Command, args []string) error {
        // Check if migration binary is available
        if !migration.SupportsMigration() {
            return fmt.Errorf(`migration tool not available.

Install the migration tool:
  go install github.com/lanceman/zqk/cmd/zqk-migrate@latest

Or download from: https://github.com/lanceman/zqk/releases`)
        }
        
        // Get binary path
        caps := migration.GetCapabilities()
        
        // Execute migration binary as subprocess
        migrateCmd := exec.Command(caps.BinaryPath, args...)
        migrateCmd.Stdout = os.Stdout
        migrateCmd.Stderr = os.Stderr
        migrateCmd.Stdin = os.Stdin
        
        return migrateCmd.Run()
    },
}
```

### Graceful Degradation

```go
// pkg/storage/kernel_connector.go
package storage

import (
    "github.com/lanceman/zqk/pkg/migration"
)

// KernelConnector handles storage operations
type KernelConnector struct {
    provider GraphProvider
    migrationAvailable bool
}

func NewKernelConnector(provider GraphProvider) *KernelConnector {
    return &KernelConnector{
        provider: provider,
        migrationAvailable: migration.SupportsMigration(),
    }
}

// MigrateFromFiles migrates from file-based storage
func (kc *KernelConnector) MigrateFromFiles(ctx context.Context, sourcePath string) error {
    if !kc.migrationAvailable {
        return fmt.Errorf("migration tool not available. Install with: go install github.com/lanceman/zqk/cmd/zqk-migrate@latest")
    }
    
    // Use migration tool
    caps := migration.GetCapabilities()
    cmd := exec.CommandContext(ctx, caps.BinaryPath, "migrate", "--source", sourcePath)
    return cmd.Run()
}
```

## Error Messages

### When Binary Not Available

**CLI Command**:
```
$ zqk migrate --source docs (PRUNED)/process

❌ Migration tool not available.

The migration tool (zqk-migrate) is not installed or not in PATH.

Install with:
  go install github.com/lanceman/zqk/cmd/zqk-migrate@latest

Or download from:
  https://github.com/lanceman/zqk/releases

For mobile builds, migration is not included. Use a desktop/server
build or install the migration tool separately.
```

**Programmatic Check**:
```go
if !migration.SupportsMigration() {
    log.Warn("Migration tool not available - migration features disabled")
    // Continue with reduced functionality
}
```

## Configuration

### Explicit Binary Path

Allow users to specify migration binary path in config:

```yaml
# .zqk/config.yaml
migration:
  binary_path: "/usr/local/bin/zqk-migrate"  # Optional override
  auto_detect: true  # Default: try to find in PATH
```

## Mobile Build Considerations

### Build Tags

```go
// pkg/migration/detector.go
// +build !mobile

package migration

// Migration detection only compiled for non-mobile builds
```

### Mobile Build Script

```bash
#!/bin/bash
# build-mobile.sh

# Build main binary without migration support
go build -tags "mobile" -o zqk-mobile ./cmd/zqk

# Migration binary is separate (not built for mobile)
# Users can install separately if needed
```

### Build and Signing Process

**Standard Build Script**:
```bash
#!/bin/bash
# build-migrate.sh

set -e

# Build binary
go build -o zqk-migrate ./cmd/zqk-migrate

# Sign binary (platform-specific)
case "$(uname -s)" in
    Darwin)
        # macOS: Sign with Developer ID
        codesign --sign "Developer ID Application: zqk (TEAM_ID)" \
                 --timestamp \
                 --options runtime \
                 --deep \
                 --force \
                 zqk-migrate
        # Verify signature
        codesign --verify --verbose zqk-migrate
        ;;
    Linux)
        # Linux: Create GPG signature
        gpg --detach-sign --armor zqk-migrate
        # Verify signature
        gpg --verify zqk-migrate.sig zqk-migrate
        ;;
    MINGW*|MSYS*|CYGWIN*)
        # Windows: Sign with Authenticode
        signtool sign /f certificate.pfx /p "$SIGNING_PASSWORD" \
                 /t http://timestamp.digicert.com \
                 zqk-migrate.exe
        # Verify signature
        signtool verify /pa zqk-migrate.exe
        ;;
esac

# Calculate and store hash
sha256sum zqk-migrate > zqk-migrate.sha256
# Or on macOS: shasum -a 256 zqk-migrate > zqk-migrate.sha256

echo "✅ Binary built and signed successfully"
```

**Source Build Requirements**:

If building from source, users **must** sign the binary before use:

```bash
# Build from source
go build -o zqk-migrate ./cmd/zqk-migrate

# Sign the binary (REQUIRED)
# macOS:
codesign --sign "Developer ID Application: Your Name (TEAM_ID)" zqk-migrate

# Linux:
gpg --detach-sign --armor zqk-migrate

# Windows:
signtool sign /f your-certificate.pfx zqk-migrate.exe

# Update manifest with your signature
zqk migration verify --update-manifest
```

**Note**: Self-signed binaries will only work if the signer is added to the trusted signers list in config.

## Testing

### Unit Tests

```go
func TestMigrationBinaryDetector(t *testing.T) {
    detector := NewMigrationBinaryDetector()
    
    // Test with mock binary
    available := detector.IsAvailable()
    // ... assertions
}
```

### Integration Tests

```go
func TestMigrationIntegration(t *testing.T) {
    if !migration.SupportsMigration() {
        t.Skip("Migration binary not available - skipping integration test")
    }
    
    // Run integration tests
}
```

## Binary Integrity Verification

### Security Requirements

The migration binary must be verified before execution to prevent tampering. This uses the same SHA-256 hashing infrastructure as the file integrity system (BLI-070, BLI-062).

### Verification Methods

#### 1. SHA-256 Checksum Verification (Primary)

**Implementation** (using existing integrity patterns):
```go
package migration

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "os"
    "github.com/lanceman/zqk/pkg/storage"
)

// VerifyBinaryIntegrity verifies the migration binary against expected checksum
func VerifyBinaryIntegrity(binaryPath string, expectedHash string) error {
    // Read binary file
    file, err := os.Open(binaryPath)
    if err != nil {
        return fmt.Errorf("failed to open binary: %w", err)
    }
    defer file.Close()
    
    // Calculate SHA-256 hash
    hasher := sha256.New()
    if _, err := io.Copy(hasher, file); err != nil {
        return fmt.Errorf("failed to read binary: %w", err)
    }
    
    actualHash := hex.EncodeToString(hasher.Sum(nil))
    
    // Compare with expected hash
    if actualHash != expectedHash {
        return fmt.Errorf("binary integrity check failed: hash mismatch (expected %s, got %s)", expectedHash, actualHash)
    }
    
    return nil
}

// GetBinaryHash calculates SHA-256 hash of the binary
func GetBinaryHash(binaryPath string) (string, error) {
    file, err := os.Open(binaryPath)
    if err != nil {
        return "", fmt.Errorf("failed to open binary: %w", err)
    }
    defer file.Close()
    
    hasher := sha256.New()
    if _, err := io.Copy(hasher, file); err != nil {
        return "", fmt.Errorf("failed to read binary: %w", err)
    }
    
    return hex.EncodeToString(hasher.Sum(nil)), nil
}
```

#### 2. Manifest-Based Verification (Secondary)

**Binary Manifest Format**:
```yaml
# .zqk/migration-binary-manifest.yaml
version: "1.0.0"
algorithm: "sha256"
entries:
  - binary: "zqk-migrate"
    version: "1.0.0"
    platform: "darwin/arm64"
    hash: "abc123def456..."
    signature: "..." # Optional: code signing signature
    source: "github.com/lanceman/zqk/cmd/zqk-migrate"
    verified_at: "2025-12-24T10:00:00Z"
```

**Manifest Verification**:
```go
// LoadBinaryManifest loads the migration binary manifest
func LoadBinaryManifest() (*BinaryManifest, error) {
    manifestPath := filepath.Join(".zqk", "migration-binary-manifest.yaml")
    // Load and parse manifest
}

// VerifyBinaryAgainstManifest verifies binary against manifest entry
func VerifyBinaryAgainstManifest(binaryPath string, manifest *BinaryManifest) error {
    // Get platform-specific entry
    entry := manifest.GetEntryForPlatform(runtime.GOOS, runtime.GOARCH)
    if entry == nil {
        return fmt.Errorf("no manifest entry for platform %s/%s", runtime.GOOS, runtime.GOARCH)
    }
    
    // Verify hash
    return VerifyBinaryIntegrity(binaryPath, entry.Hash)
}
```

#### 3. Version Verification (Tertiary)

**Version Check**:
```go
// GetBinaryVersion gets version from binary (via --version flag)
func GetBinaryVersion(binaryPath string) (string, error) {
    cmd := exec.Command(binaryPath, "--version")
    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("failed to get version: %w", err)
    }
    // Parse version from output
    return parseVersion(string(output)), nil
}

// VerifyBinaryVersion verifies binary version matches expected
func VerifyBinaryVersion(binaryPath string, expectedVersion string) error {
    actualVersion, err := GetBinaryVersion(binaryPath)
    if err != nil {
        return err
    }
    
    if actualVersion != expectedVersion {
        return fmt.Errorf("version mismatch: expected %s, got %s", expectedVersion, actualVersion)
    }
    
    return nil
}
```

#### 4. Code Signing Verification (Required)

Code signing is **required** for all migration binaries. This ensures the binary is authentic and hasn't been tampered with.

**macOS Code Signing**:
```go
// VerifyCodeSignature verifies macOS code signature (REQUIRED)
func VerifyCodeSignature(binaryPath string) error {
    // Check if binary is signed
    cmd := exec.Command("codesign", "--verify", "--verbose", binaryPath)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("code signature verification failed: %w\nOutput: %s", err, string(output))
    }
    
    // Verify signature is valid and not expired
    cmd = exec.Command("codesign", "--verify", "--deep", "--strict", binaryPath)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("strict code signature verification failed: %w", err)
    }
    
    // Get signer identity
    cmd = exec.Command("codesign", "-d", "-vvv", binaryPath)
    identityOutput, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("failed to get signer identity: %w", err)
    }
    
    // Verify signer is trusted (e.g., matches expected developer ID)
    if !isTrustedSigner(string(identityOutput)) {
        return fmt.Errorf("binary signed by untrusted signer")
    }
    
    return nil
}

// isTrustedSigner checks if the signer identity is in the trusted list
func isTrustedSigner(identityOutput string) bool {
    // Parse identity from output
    // Check against trusted signers list (from config or hardcoded)
    // Example: "Developer ID Application: Your Company (TEAM_ID)"
    trustedSigners := []string{
        "Developer ID Application: zqk",
        // Add other trusted signers
    }
    
    for _, trusted := range trustedSigners {
        if strings.Contains(identityOutput, trusted) {
            return true
        }
    }
    
    return false
}
```

**Linux GPG Verification** (Required):
```go
// VerifyGPGSignature verifies GPG signature (REQUIRED)
func VerifyGPGSignature(binaryPath string, signaturePath string) error {
    // Check if signature file exists
    if _, err := os.Stat(signaturePath); os.IsNotExist(err) {
        return fmt.Errorf("GPG signature file not found: %s", signaturePath)
    }
    
    // Verify signature
    cmd := exec.Command("gpg", "--verify", signaturePath, binaryPath)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("GPG signature verification failed: %w\nOutput: %s", err, string(output))
    }
    
    // Parse output to verify signature is valid
    if !strings.Contains(string(output), "Good signature") {
        return fmt.Errorf("GPG signature is not valid")
    }
    
    // Verify signer is trusted (check key fingerprint)
    if err := verifyTrustedGPGKey(string(output)); err != nil {
        return fmt.Errorf("signature from untrusted key: %w", err)
    }
    
    return nil
}

// verifyTrustedGPGKey verifies the GPG key fingerprint is trusted
func verifyTrustedGPGKey(gpgOutput string) error {
    // Extract key fingerprint from output
    // Check against trusted keys list
    trustedKeys := []string{
        // Add trusted GPG key fingerprints
    }
    
    // Parse and verify fingerprint
    return nil
}
```

**Windows Code Signing** (Required):
```go
// VerifyWindowsSignature verifies Windows Authenticode signature (REQUIRED)
func VerifyWindowsSignature(binaryPath string) error {
    // Use signtool or PowerShell to verify signature
    cmd := exec.Command("powershell", "-Command", 
        fmt.Sprintf("Get-AuthenticodeSignature -FilePath '%s' | Select-Object Status", binaryPath))
    output, err := cmd.Output()
    if err != nil {
        return fmt.Errorf("failed to check Windows signature: %w", err)
    }
    
    // Check if status is "Valid"
    if !strings.Contains(string(output), "Valid") {
        return fmt.Errorf("Windows signature is not valid")
    }
    
    return nil
}
```

### Enhanced Detection with Integrity Check

```go
// MigrationBinaryDetector with integrity verification
type MigrationBinaryDetector struct {
    binaryName string
    searchPaths []string
    requireVerification bool
    expectedHash string // Optional: expected SHA-256 hash
    expectedVersion string // Optional: expected version
}

// IsAvailable checks if binary is available AND verified
func (d *MigrationBinaryDetector) IsAvailable() bool {
    path, err := d.GetPath()
    if err != nil {
        return false
    }
    
    // If verification required, verify integrity
    if d.requireVerification {
        if err := d.VerifyIntegrity(path); err != nil {
            log.Warn("Migration binary found but integrity check failed: %v", err)
            return false
        }
    }
    
    return true
}

// VerifyIntegrity performs comprehensive integrity verification
func (d *MigrationBinaryDetector) VerifyIntegrity(binaryPath string) error {
    // 1. Verify SHA-256 hash (if expected hash provided)
    if d.expectedHash != "" {
        if err := VerifyBinaryIntegrity(binaryPath, d.expectedHash); err != nil {
            return fmt.Errorf("hash verification failed: %w", err)
        }
    } else {
        // Try to verify against manifest
        manifest, err := LoadBinaryManifest()
        if err == nil {
            if err := VerifyBinaryAgainstManifest(binaryPath, manifest); err != nil {
                return fmt.Errorf("manifest verification failed: %w", err)
            }
        }
    }
    
    // 2. Verify version (if expected version provided)
    if d.expectedVersion != "" {
        if err := VerifyBinaryVersion(binaryPath, d.expectedVersion); err != nil {
            return fmt.Errorf("version verification failed: %w", err)
        }
    }
    
    // 3. Verify code signature (platform-specific, REQUIRED)
    switch runtime.GOOS {
    case "darwin":
        if err := VerifyCodeSignature(binaryPath); err != nil {
            return fmt.Errorf("code signature verification failed (REQUIRED): %w", err)
        }
    case "linux":
        // Look for .sig file alongside binary
        sigPath := binaryPath + ".sig"
        if _, err := os.Stat(sigPath); err == nil {
            if err := VerifyGPGSignature(binaryPath, sigPath); err != nil {
                return fmt.Errorf("GPG signature verification failed (REQUIRED): %w", err)
            }
        } else {
            return fmt.Errorf("GPG signature file not found (REQUIRED): %s", sigPath)
        }
    case "windows":
        if err := VerifyWindowsSignature(binaryPath); err != nil {
            return fmt.Errorf("Windows signature verification failed (REQUIRED): %w", err)
        }
    default:
        // For other platforms, at minimum require hash verification
        log.Warn("Code signing not available for platform %s, relying on hash verification only", runtime.GOOS)
    }
    
    return nil
}
```

### Configuration

**Config File**:
```yaml
# .zqk/config.yaml
migration:
  binary_path: "/usr/local/bin/zqk-migrate"  # Optional override
  auto_detect: true  # Default: try to find in PATH
  require_verification: true  # Default: verify integrity before use (always true for code signing)
  expected_hash: "abc123def456..."  # Optional: expected SHA-256 hash
  expected_version: "1.0.0"  # Optional: expected version
  manifest_path: ".zqk/migration-binary-manifest.yaml"  # Optional: custom manifest path
  
  # Code signing configuration
  code_signing:
    required: true  # Code signing is always required
    trusted_signers:
      # macOS Developer ID
      - "Developer ID Application: zqk (TEAM_ID)"
      - "Developer ID Application: Your Company (TEAM_ID)"
      # Linux GPG key fingerprints
      - "ABCD1234EFGH5678IJKL9012MNOP3456QRST7890"  # GPG key fingerprint
      # Windows certificate thumbprints
      - "A1B2C3D4E5F6..."  # Certificate thumbprint
    # Platform-specific settings
    macos:
      require_notarization: true  # Require notarized binaries (recommended)
      allow_ad_hoc: false  # Disallow ad-hoc signatures
    linux:
      require_gpg: true  # Require GPG signature
      signature_extension: ".sig"  # Signature file extension
    windows:
      require_authenticode: true  # Require Authenticode signature
```

**Trusted Signer Management**:
```go
// LoadTrustedSigners loads trusted signers from config
func LoadTrustedSigners() ([]string, error) {
    // Load from .zqk/config.yaml
    // Fallback to default trusted signers if not configured
    return []string{
        "Developer ID Application: zqk",
        // Add other default trusted signers
    }, nil
}

// IsTrustedSigner checks if a signer is in the trusted list
func IsTrustedSigner(signerIdentity string, trustedSigners []string) bool {
    for _, trusted := range trustedSigners {
        if strings.Contains(signerIdentity, trusted) {
            return true
        }
    }
    return false
}
```

### Error Messages

**When Integrity Check Fails**:
```
$ zqk migrate --source docs (PRUNED)/process

❌ Migration binary integrity check failed.

The migration binary (zqk-migrate) was found but failed integrity verification:

Code Signature Verification: FAILED
  Error: code signature verification failed: invalid signature

This indicates:
  - The binary is not signed (REQUIRED)
  - The binary signature is invalid or expired
  - The binary has been tampered with
  - The binary is from an untrusted source

Actions:
  1. Re-download the binary from official release:
     https://github.com/lanceman/zqk/releases/latest
  
  2. Ensure you download both the binary AND signature file:
     - zqk-migrate (binary)
     - zqk-migrate.sig (GPG signature, Linux)
     - Or use signed installer (macOS/Windows)
  
  3. Verify the download source is legitimate (check GitHub URL)
  
  4. If you built from source, you must sign the binary:
     macOS: codesign --sign "Developer ID Application: ..." zqk-migrate
     Linux: gpg --detach-sign zqk-migrate

Code signing is REQUIRED for security. Unsigned binaries will not be executed.
```

### Binary Signing Requirements

**Build/Release Process**:

All migration binaries must be signed before distribution:

**macOS Signing**:
```bash
# Sign binary with Developer ID
codesign --sign "Developer ID Application: zqk (TEAM_ID)" \
         --timestamp \
         --options runtime \
         --deep \
         --force \
         zqk-migrate

# Verify signature
codesign --verify --verbose zqk-migrate
```

**Linux GPG Signing**:
```bash
# Create detached signature
gpg --detach-sign --armor zqk-migrate

# This creates zqk-migrate.sig
# Both binary and .sig file must be distributed together
```

**Windows Signing**:
```bash
# Sign with Authenticode certificate
signtool sign /f certificate.pfx /p password /t http://timestamp.digicert.com zqk-migrate.exe

# Verify signature
signtool verify /pa zqk-migrate.exe
```

**Release Checklist**:
- [ ] Binary built and tested
- [ ] Binary signed with platform-specific tool
- [ ] Signature verified
- [ ] SHA-256 hash calculated
- [ ] Manifest updated with hash and signature info
- [ ] Binary and signature files packaged together

### Secure Download Verification

**Download with Verification**:
```go
// DownloadBinaryWithVerification downloads binary and verifies integrity
func DownloadBinaryWithVerification(url string, expectedHash string) (string, error) {
    // Download binary
    binaryPath, err := downloadBinary(url)
    if err != nil {
        return "", err
    }
    
    // Verify hash
    if err := VerifyBinaryIntegrity(binaryPath, expectedHash); err != nil {
        os.Remove(binaryPath) // Clean up on failure
        return "", fmt.Errorf("downloaded binary failed integrity check: %w", err)
    }
    
    return binaryPath, nil
}
```

### Handling Verification Failures

**Strict Mode (Default)**:
- Verification failure = binary not available
- System refuses to execute unverified binary
- User must fix verification issue before proceeding

**Strict Mode Enforcement**:
```go
// Code signing verification is ALWAYS required - no bypass
if err := detector.VerifyIntegrity(binaryPath); err != nil {
    return fmt.Errorf("integrity check failed (code signing required): %w", err)
}

// Note: There is NO --skip-verification flag for code signing
// Code signing is a hard requirement for security
```

**Error Handling**:
- **Hash Mismatch**: Binary rejected, must re-download
- **Missing Signature**: Binary rejected, must obtain signed version
- **Invalid Signature**: Binary rejected, may be tampered with
- **Untrusted Signer**: Binary rejected, signer not in trusted list
- **Expired Signature**: Binary rejected, must obtain new signed version

**Recovery Actions**:
1. **Re-download Signed Binary**: Download from official release with signature
2. **Verify Signature**: Ensure signature file is present and valid
3. **Check Trusted Signers**: Verify signer is in trusted list (configurable)
4. **Update Manifest**: After successful verification, update manifest with new hash

**Note**: Manual override of code signing verification is NOT allowed. All binaries must be properly signed.

### Manifest Management

**Updating Manifest After Installation**:
```go
// UpdateManifestAfterInstall updates manifest with new binary hash
func UpdateManifestAfterInstall(binaryPath string, version string) error {
    // Calculate hash of installed binary
    hash, err := GetBinaryHash(binaryPath)
    if err != nil {
        return err
    }
    
    // Load or create manifest
    manifest, err := LoadBinaryManifest()
    if err != nil {
        manifest = NewBinaryManifest()
    }
    
    // Add/update entry for current platform
    entry := &BinaryManifestEntry{
        Binary:   "zqk-migrate",
        Version:  version,
        Platform: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
        Hash:     hash,
        VerifiedAt: time.Now().UTC().Format(time.RFC3339),
    }
    
    manifest.UpdateEntry(entry)
    
    // Save manifest
    return SaveBinaryManifest(manifest)
}
```

**CLI Command for Manifest Management**:
```bash
# Update manifest after installing new binary
zqk migration verify --update-manifest

# Verify binary against manifest
zqk migration verify

# Show binary information
zqk migration info
```

## Summary

**Detection Strategy**:
1. **Runtime**: Check PATH and common locations (primary)
2. **Build-time**: Use build tags to exclude from mobile (secondary)
3. **Capability Interface**: Provide `SupportsMigration()` check (tertiary)

**Integrity Verification**:
1. **SHA-256 Hash**: Verify binary against expected checksum (primary)
2. **Code Signing**: Platform-specific signature verification (**REQUIRED**)
3. **Manifest-Based**: Verify against platform-specific manifest (secondary)
4. **Version Check**: Verify binary version matches expected (tertiary)

**Benefits**:
- ✅ Works across all platforms
- ✅ Graceful degradation when not available
- ✅ Clear error messages for users
- ✅ Mobile builds stay minimal
- ✅ Desktop/server builds get full functionality
- ✅ **Security**: Prevents execution of tampered binaries
- ✅ **Trust**: Verifies binary authenticity before use
- ✅ **Code Signing Required**: All binaries must be signed (macOS, Linux GPG, Windows Authenticode)

**Pattern**: Similar to how git hooks detect `golangci-lint` - try to use, fall back gracefully if not available. **Plus**: Verify integrity before execution to prevent tampering.

