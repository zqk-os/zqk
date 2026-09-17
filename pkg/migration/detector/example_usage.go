package detector

// Example usage of the migration binary detector
//
// This file demonstrates how to integrate the detector into the zqk orchestrator.
// The actual CLI implementation would be in cmd/zqk/migrate.go

/*
Example CLI integration:

package main

import (
	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/detector"
	"github.com/spf13/cobra"
	"os/exec"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate data between backends",
	Long: `Migrate data between file-based and graph backends.

This command requires the zqk-migrate binary to be installed and verified.
The binary must be code-signed and pass integrity verification.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create detector with default settings
		d := detector.NewBinaryDetector()

		// Check if binary is available and verified
		if !d.IsAvailable() {
			path, err := d.GetPath()
			if err != nil {
				return fmt.
					Errorf(`❌ Migration tool not available.

The migration tool (zqk-migrate) is not installed or not in PATH.

Install with:
  go install github.com/lanceman/zqk/cmd/zqk-migrate@latest

Or download from:
  https://github.com/lanceman/zqk/releases`)
			}

			// Binary found but verification failed
			if err := d.VerifyIntegrity(path); err != nil {
				return fmt.
					Errorf(`❌ Migration binary integrity check failed.

The migration binary was found but failed verification:
  %v

This may indicate:
  - The binary has been tampered with
  - The binary is not properly code-signed (REQUIRED)
  - The binary is from an untrusted source

Actions:
  1. Re-download the binary from official release:
     https://github.com/lanceman/zqk/releases/latest

  2. Ensure you download both the binary AND signature file:
     - zqk-migrate (binary)
     - zqk-migrate.sig (GPG signature, Linux)
     - Or use signed installer (macOS/Windows)

  3. Verify the download source is legitimate

Code signing is REQUIRED for security.`, err)
			}
		}

		// Get capabilities
		caps, err := d.GetCapabilities()
		if err != nil {
			return errfmt.Newf("failed to get capabilities").Wrap(err)
		}

		logging.FluentEvent(logging.GetLogger()).Info("Using migration tool").Path(caps.BinaryPath).Version(caps.Version).Log()

		// Execute migration binary as subprocess
		migrateCmd := execwrap.Command(caps.BinaryPath, args...)
		migrateCmd.Stdout = os.Stdout
		migrateCmd.Stderr = os.Stderr
		migrateCmd.Stdin = os.Stdin

		return migrateCmd.Run()
	},
}

// Example: Verify binary before use
func ExampleVerifyBeforeUse() {
	d := detector.NewBinaryDetector().
		WithExpectedHash("abc123...").
		WithExpectedVersion("1.0.0").
		WithTrustedSigners([]string{
			"Developer ID Application: ZQK",
		})

	if !d.IsAvailable() {
		logging.FluentEvent(logging.GetLogger()).Error("Migration binary not available or verification failed", nil).Log()
		return
	}

	caps, _ := d.GetCapabilities()
	logging.FluentEvent(logging.GetLogger()).Info("Binary verified").Path(caps.BinaryPath).Log()
}

// Example: Update manifest after installation
func ExampleUpdateManifest() {
	binaryPath := "/usr/local/bin/zqk-migrate"
	version := "1.0.0"

	if err := detector.UpdateManifestAfterInstall(binaryPath, version, ""); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("Failed to update manifest", err).Log()
		return
	}

	logging.FluentEvent(logging.GetLogger()).Info("Manifest updated successfully").Log()
}

// Example: Verify against manifest
func ExampleVerifyAgainstManifest() {
	binaryPath := "/usr/local/bin/zqk-migrate"
	manifest, err := detector.LoadBinaryManifest("")
	if err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("Failed to load manifest", err).Log()
		return
	}

	if err := detector.VerifyBinaryAgainstManifest(binaryPath, manifest); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("Manifest verification failed", err).Log()
		return
	}

	logging.FluentEvent(logging.GetLogger()).Info("Binary verified against manifest").Log()
}
*/
