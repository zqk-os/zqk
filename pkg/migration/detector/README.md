# Migration Binary Detector

**Status**: Implemented  
**Related**: BLI-623, Migration Binary Detection Strategy v1.0

This package provides binary detection and integrity verification for the `zqk-migrate` binary. It ensures that only properly signed and verified binaries are executed.

## Features

- ✅ **Runtime Detection**: Finds binary in PATH and common locations
- ✅ **SHA-256 Hash Verification**: Verifies binary integrity
- ✅ **Code Signing Verification**: **REQUIRED** - Platform-specific:
  - macOS: Developer ID Application signature
  - Linux: GPG detached signature
  - Windows: Authenticode signature
- ✅ **Version Verification**: Checks binary version matches expected
- ✅ **Manifest-Based Verification**: Platform-specific manifest support
- ✅ **Trusted Signer Validation**: Only executes binaries from trusted signers

## Usage

### Basic Detection

```go
import "github.com/zqk-os/zqk/pkg/migration/detector"

// Check if migration is available
if detector.SupportsMigration() {
    // Migration binary is available and verified
}

// Get capabilities
caps, err := detector.NewBinaryDetector().GetCapabilities()
if err != nil {
    // Handle error
}
fmt.Printf("Binary: %s, Version: %s\n", caps.BinaryPath, caps.Version)
```

### With Custom Configuration

```go
d := detector.NewBinaryDetector().
    WithExpectedHash("abc123...").
    WithExpectedVersion("1.0.0").
    WithTrustedSigners([]string{
        "Developer ID Application: ZQK",
    }).
    WithSearchPaths([]string{"/custom/path"})

if !d.IsAvailable() {
    // Binary not available or verification failed
}
```

### Integrity Verification

```go
d := detector.NewBinaryDetector()
path, _ := d.GetPath()

// Verify integrity (includes code signing)
if err := d.VerifyIntegrity(path); err != nil {
    // Verification failed - binary should not be executed
    log.Fatal(err)
}
```

### Manifest Management

```go
// Load manifest
manifest, err := detector.LoadBinaryManifest("")
if err != nil {
    // Handle error
}

// Verify against manifest
if err := detector.VerifyBinaryAgainstManifest(binaryPath, manifest); err != nil {
    // Verification failed
}

// Update manifest after installation
if err := detector.UpdateManifestAfterInstall(binaryPath, "1.0.0", ""); err != nil {
    // Handle error
}
```

## Security

### Code Signing Requirements

**All binaries must be code-signed** before execution:

- **macOS**: Developer ID Application signature (notarization recommended)
- **Linux**: GPG detached signature (`.sig` file required)
- **Windows**: Authenticode signature

### Verification Process

1. **Find Binary**: Search PATH and configured paths
2. **Verify Hash**: SHA-256 hash verification (if expected hash provided)
3. **Verify Signature**: **REQUIRED** - Platform-specific code signing
4. **Verify Signer**: Check signer is in trusted list
5. **Verify Version**: Version check (if expected version provided)

### Trusted Signers

Configure trusted signers via:
- `WithTrustedSigners()` method
- Configuration file (future enhancement)

Default trusted signers:
- `Developer ID Application: ZQK`

## Error Handling

The detector provides clear error messages when verification fails:

```
❌ Migration binary integrity check failed.

Code Signature Verification: FAILED
  Error: code signature verification failed: invalid signature

This indicates:
  - The binary is not signed (REQUIRED)
  - The binary signature is invalid or expired
  - The binary has been tampered with
  - The binary is from an untrusted source
```

## Testing

Run tests:
```bash
go test ./pkg/migration/detector -v
```

Tests cover:
- Binary path detection
- Hash calculation and verification
- Integrity verification
- Trusted signer validation
- Manifest management

## Architecture

```
detector/
├── detector.go      # Main detection and verification logic
├── manifest.go     # Manifest management
├── detector_test.go # Tests
└── example_usage.go # Usage examples
```

## Integration

See `example_usage.go` for complete CLI integration examples.

The detector is designed to be used by the zqk orchestrator to:
1. Detect if migration binary is available
2. Verify binary integrity before execution
3. Provide clear error messages when verification fails
4. Support graceful degradation when binary is not available

## Related Documentation

- [Storage Architecture](../../../docs/architecture/TIERED_STORAGE_AND_ARCHIVAL_LIFECYCLE.md) - Storage lifecycle and retention
- [Architecture Overview](../../../docs/architecture/README.md) - Architecture documentation

