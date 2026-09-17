package detector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// BinaryDetector detects and verifies the migration binary
type BinaryDetector struct {
	binaryName          string
	searchPaths         []string
	requireVerification bool
	expectedHash        string
	expectedVersion     string
	versionConstraint   *VersionConstraint // Version compatibility constraint
	trustedSigners      []string
}

// NewBinaryDetector creates a new binary detector
func NewBinaryDetector() *BinaryDetector {
	//nolint:errcheck // Executable path - error acceptable
	execPath, _ := fileutil.Executable()
	return &BinaryDetector{
		binaryName:          "zqk-migrate",
		requireVerification: true,
		searchPaths: []string{
			filepath.Dir(execPath), // Same directory as main binary
			"",                     // PATH
			"/usr/local/bin",
			"/opt/zqk/bin",
		},
		trustedSigners: []string{
			"Developer ID Application: ZQK",
		},
	}
}

// WithExpectedHash sets the expected SHA-256 hash
func (d *BinaryDetector) WithExpectedHash(hash string) *BinaryDetector {
	d.expectedHash = hash
	return d
}

// WithExpectedVersion sets the expected exact version
func (d *BinaryDetector) WithExpectedVersion(version string) *BinaryDetector {
	d.expectedVersion = version
	return d
}

// WithVersionConstraint sets a version compatibility constraint
// Constraint can be: ">=1.0.0", "^1.0.0", ">=1.0.0 <2.0.0", etc.
func (d *BinaryDetector) WithVersionConstraint(constraintStr string) (*BinaryDetector, error) {
	constraint, err := ParseVersionConstraint(constraintStr)
	if err != nil {
		return nil, errfmt.Newf("invalid version constraint").Wrap(err)
	}
	d.versionConstraint = constraint
	return d, nil
}

// WithTrustedSigners sets the list of trusted signers
func (d *BinaryDetector) WithTrustedSigners(signers []string) *BinaryDetector {
	d.trustedSigners = signers
	return d
}

// WithSearchPaths sets custom search paths
func (d *BinaryDetector) WithSearchPaths(paths []string) *BinaryDetector {
	d.searchPaths = paths
	return d
}

// IsAvailable checks if the migration binary is available and verified
func (d *BinaryDetector) IsAvailable() bool {
	path, err := d.GetPath()
	if err != nil {
		return false
	}

	// If verification required, verify integrity
	if d.requireVerification {
		if err := d.VerifyIntegrity(path); err != nil {
			return false
		}
	}

	return true
}

// GetPath returns the full path to the migration binary if available
func (d *BinaryDetector) GetPath() (string, error) {
	// Try PATH first
	if path, err := exec.LookPath(d.binaryName); err == nil {
		if info, err := fileutil.Stat(path); err == nil {
			if info.Mode().Perm()&0111 != 0 {
				return path, nil
			}
		}
	}

	// Check explicit paths
	for _, searchPath := range d.searchPaths {
		if searchPath == emptyValue {
			continue
		}
		fullPath := filepath.Join(searchPath, d.binaryName)
		if info, err := fileutil.Stat(fullPath); err == nil {
			if info.Mode().Perm()&0111 != 0 {
				return fullPath, nil
			}
		}
	}

	return "", errfmt.Errorf("migration binary '%s' not found", d.binaryName)
}

// GetCapabilities returns migration tool capabilities
func (d *BinaryDetector) GetCapabilities() (*Capabilities, error) {
	path, err := d.GetPath()
	if err != nil {
		return &Capabilities{
			Available: false,
		}, nil
	}

	caps := &Capabilities{
		Available:         true,
		BinaryPath:        path,
		SupportedPhases:   []string{"document", "entity", "relationship"},
		SupportedBackends: []string{"file", "memgraph", "neo4j"},
	}

	// Get version
	if version, err := d.GetVersion(path); err == nil {
		caps.Version = version
	}

	// Get hash
	if hash, err := GetBinaryHash(path); err == nil {
		caps.Hash = hash
	}

	return caps, nil
}

// GetVersion gets the version from the binary
func (d *BinaryDetector) GetVersion(binaryPath string) (string, error) {
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	cmd := execwrap.CommandContext(ctx, binaryPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", errfmt.Newf("failed to get version").Wrap(err)
	}

	// Parse version from output (e.g., "zqk-migrate version 1.0.0")
	versionStr := strings.TrimSpace(string(output))
	parts := strings.Fields(versionStr)
	for i, part := range parts {
		if part == "version" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}

	// If no "version" keyword, try to extract version-like string
	if len(parts) > 0 {
		return parts[len(parts)-1], nil
	}

	return "", errfmt.Errorf("could not parse version from output: %s", versionStr)
}

// VerifyIntegrity performs comprehensive integrity verification
func (d *BinaryDetector) VerifyIntegrity(binaryPath string) error {
	// 1. Verify SHA-256 hash (if expected hash provided)
	if d.expectedHash != emptyValue {
		if err := VerifyBinaryIntegrity(binaryPath, d.expectedHash); err != nil {
			return errfmt.Newf("hash verification failed").Wrap(err)
		}
	}

	// 2. Verify code signature (REQUIRED)
	if err := d.verifyCodeSignature(binaryPath); err != nil {
		return errfmt.Newf("code signature verification failed (REQUIRED)").Wrap(err)
	}

	// 3. Verify version (if expected version or constraint provided)
	if d.expectedVersion != emptyValue {
		actualVersion, err := d.GetVersion(binaryPath)
		if err != nil {
			return errfmt.Newf("failed to get version").Wrap(err)
		}
		if actualVersion != d.expectedVersion {
			return errfmt.Errorf("version mismatch: expected %s, got %s", d.expectedVersion, actualVersion)
		}
	} else if d.versionConstraint != nil {
		actualVersion, err := d.GetVersion(binaryPath)
		if err != nil {
			return errfmt.Newf("failed to get version").Wrap(err)
		}
		if err := VerifyVersionCompatibility(actualVersion, d.versionConstraint); err != nil {
			return errfmt.Newf("version compatibility check failed").Wrap(err)
		}
	}

	// 4. Verify compatibility constraints from manifest (if available)
	manifestPath := filepath.Join(paths.ProjectDataDir, "migration-binary-manifest.yaml")
	if manifest, err := LoadBinaryManifest(manifestPath); err == nil {
		// Verify compatibility before using binary
		if err := VerifyBinaryAgainstManifest(binaryPath, manifest); err != nil {
			return errfmt.Newf("manifest compatibility check failed").Wrap(err)
		}
	}

	return nil
}

// verifyCodeSignature verifies code signature (platform-specific, REQUIRED)
func (d *BinaryDetector) verifyCodeSignature(binaryPath string) error {
	switch runtime.GOOS {
	case "darwin":
		return d.verifyMacOSSignature(binaryPath)
	case "linux":
		return d.verifyLinuxSignature(binaryPath)
	case "windows":
		return d.verifyWindowsSignature(binaryPath)
	default:
		// For other platforms, at minimum require hash verification
		// Code signing may not be available
		if d.expectedHash == emptyValue {
			return errfmt.Errorf("code signing not available for platform %s and no expected hash provided", runtime.GOOS)
		}
		return nil
	}
}

// verifyMacOSSignature verifies macOS code signature (REQUIRED)
func (d *BinaryDetector) verifyMacOSSignature(binaryPath string) error {
	// Check if binary is signed
	cmd := execwrap.Command("codesign", "--verify", "--verbose", binaryPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Errorf("code signature verification failed: %w\nOutput: %s", err, string(output))
	}

	// Verify signature is valid and not expired (strict check)
	cmd = execwrap.Command("codesign", "--verify", "--deep", "--strict", binaryPath)
	if err := cmd.Run(); err != nil {
		return errfmt.Newf("strict code signature verification failed").Wrap(err)
	}

	// Get signer identity
	cmd = execwrap.Command("codesign", "-d", "-vvv", binaryPath)
	identityOutput, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Newf("failed to get signer identity").Wrap(err)
	}

	// Verify signer is trusted
	if !d.isTrustedSigner(string(identityOutput)) {
		return errfmt.Errorf("binary signed by untrusted signer. Signer must be in trusted list")
	}

	return nil
}

// verifyLinuxSignature verifies Linux GPG signature (REQUIRED)
func (d *BinaryDetector) verifyLinuxSignature(binaryPath string) error {
	// Look for .sig file alongside binary
	sigPath := binaryPath + ".sig"
	if _, err := fileutil.Stat(sigPath); fileutil.IsNotExist(err) {
		return errfmt.Errorf("GPG signature file not found (REQUIRED): %s", sigPath)
	}

	// Verify signature
	cmd := execwrap.Command("gpg", "--verify", sigPath, binaryPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Errorf("GPG signature verification failed: %w\nOutput: %s", err, string(output))
	}

	// Parse output to verify signature is valid
	if !strings.Contains(string(output), "Good signature") {
		return errfmt.Errorf("GPG signature is not valid")
	}

	// Verify signer is trusted (check key fingerprint)
	if err := d.verifyTrustedGPGKey(string(output)); err != nil {
		return errfmt.Newf("signature from untrusted key").Wrap(err)
	}

	return nil
}

// verifyWindowsSignature verifies Windows Authenticode signature (REQUIRED)
func (d *BinaryDetector) verifyWindowsSignature(binaryPath string) error {
	// Use PowerShell to verify signature
	psCmd := fmt.Sprintf("Get-AuthenticodeSignature -FilePath '%s' | Select-Object Status, SignerCertificate", binaryPath)
	cmd := execwrap.Command("powershell", "-Command", psCmd)
	output, err := cmd.Output()
	if err != nil {
		return errfmt.Newf("failed to check Windows signature").Wrap(err)
	}

	// Check if status is "Valid"
	if !strings.Contains(string(output), "Valid") {
		return errfmt.Errorf("windows signature is not valid")
	}

	// Verify signer is trusted
	if !d.isTrustedWindowsSigner(string(output)) {
		return errfmt.Errorf("binary signed by untrusted signer")
	}

	return nil
}

// isTrustedSigner checks if the signer identity is in the trusted list (macOS)
func (d *BinaryDetector) isTrustedSigner(identityOutput string) bool {
	for _, trusted := range d.trustedSigners {
		if strings.Contains(identityOutput, trusted) {
			return true
		}
	}
	return false
}

// verifyTrustedGPGKey verifies the GPG key fingerprint is trusted (Linux)
func (d *BinaryDetector) verifyTrustedGPGKey(gpgOutput string) error {
	// Extract key fingerprint from output
	// GPG output format: "Primary key fingerprint: ABCD 1234 ..."
	for line := range strings.SplitSeq(gpgOutput, "\n") {
		if strings.Contains(line, "Primary key fingerprint:") {
			// Extract fingerprint
			parts := strings.Split(line, "Primary key fingerprint:")
			if len(parts) > 1 {
				fingerprint := strings.TrimSpace(strings.ReplaceAll(parts[1], " ", ""))
				// Check against trusted keys
				for _, trusted := range d.trustedSigners {
					if strings.EqualFold(fingerprint, trusted) || strings.Contains(fingerprint, trusted) {
						return nil
					}
				}
			}
		}
	}

	return errfmt.Errorf("GPG key fingerprint not in trusted list")
}

// isTrustedWindowsSigner checks if Windows signer is trusted
func (d *BinaryDetector) isTrustedWindowsSigner(output string) bool {
	// Parse certificate thumbprint or subject from output
	// Check against trusted signers list
	for _, trusted := range d.trustedSigners {
		if strings.Contains(output, trusted) {
			return true
		}
	}
	return false
}

// GetBinaryHash calculates SHA-256 hash of the binary
func GetBinaryHash(binaryPath string) (string, error) {
	file, err := fileutil.Open(binaryPath)
	if err != nil {
		return "", errfmt.Newf("failed to open binary").Wrap(err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", errfmt.Newf("failed to read binary").Wrap(err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// VerifyBinaryIntegrity verifies the migration binary against expected checksum
func VerifyBinaryIntegrity(binaryPath, expectedHash string) error {
	actualHash, err := GetBinaryHash(binaryPath)
	if err != nil {
		return err
	}

	if actualHash != expectedHash {
		return errfmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

// Capabilities describes migration tool capabilities
type Capabilities struct {
	Available         bool
	BinaryPath        string
	Version           string
	Hash              string
	SupportedPhases   []string
	SupportedBackends []string
}

// SupportsMigration checks if migration is supported
func SupportsMigration() bool {
	detector := NewBinaryDetector()
	return detector.IsAvailable()
}
