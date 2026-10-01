package utility

import (
	"os"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/detector"
)

// NewMigrateCmd creates the migrate command
func NewMigrateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Migrate data between backends",
		"Migrate data between file-based and graph backends.",
		"",
		"This command requires the migration binary to be installed and verified.",
		"The binary must be code-signed and pass integrity verification.",
		"",
		"The migration tool is a separate binary that can be installed via:",
		"  - go install github.com/zqk-os/zqk/cmd/zqk-migrate@latest",
		"  - Download from: https://github.com/zqk-os/zqk/releases",
	).
		AddExample("Migrate from file to graph backend", "%s utility migrate --source . --target memgraph").
		AddExample("Migrate specific phase", "%s utility migrate --source . --target memgraph --phase entity").
		ExcludeCommonFlags()

	migrateCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewUtilityMigrateCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use:  "migrate",
		RunE: runMigrate,
	})

	helpBuilder.ApplyToCommand(migrateCmd)

	// Add migration-specific flags
	migrateCmd.Flags().String("source", "", "Source path for migration")
	migrateCmd.Flags().String("target", "", "Target backend (memgraph, neo4j, file)")
	migrateCmd.Flags().String("phase", "", "Migration phase (document, entity, relationship, all)")
	migrateCmd.Flags().String("version-constraint", "", "Version constraint (e.g., '>=1.0.0', '^1.0.0', '>=1.0.0 <2.0.0')")

	return migrateCmd
}

func runMigrate(cmd *cobra.Command, args []string) error {
	// Create detector with default settings
	d := detector.NewBinaryDetector()

	// Check for version constraint flag
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	if versionConstraint, _ := cmd.Flags().GetString("version-constraint"); versionConstraint != emptyValue {
		var err error
		d, err = d.WithVersionConstraint(versionConstraint)
		if err != nil {
			return errfmt.Newf("invalid version constraint").Wrap(err)
		}
	}

	// Check if binary is available and verified
	if !d.IsAvailable() {
		path, err := d.GetPath()
		if err != nil {
			return errfmt.Errorf(`❌ Migration tool not available.

The migration tool (zqk-migrate) is not installed or not in PATH.

Install with:
  go install github.com/zqk-os/zqk/cmd/zqk-migrate@latest

Or download from:
  https://github.com/zqk-os/zqk/releases`)
		}

		// Binary found but verification failed
		if err := d.VerifyIntegrity(path); err != nil {
			return errfmt.Errorf("migration binary integrity check failed: %w. The migration binary was found but failed verification. This may indicate: the binary has been tampered with, the binary is not properly code-signed (REQUIRED), or the binary is from an untrusted source. Actions: 1. Re-download the binary from official release: https://github.com/zqk-os/zqk/releases/latest. 2. Ensure you download both the binary AND signature file: zqk-migrate (binary), zqk-migrate.sig (GPG signature, Linux), or use signed installer (macOS/Windows). 3. Verify the download source is legitimate. Code signing is REQUIRED for security", err)
		}
	}

	// Get capabilities
	caps, err := d.GetCapabilities()
	if err != nil {
		return errfmt.Newf("failed to get capabilities").Wrap(err)
	}

	if cli.IsVerbose(cmd) {
		if eventLogger := logging.GetLoggerFromContext(cmd.Context()); eventLogger != nil {
			logging.FluentEvent(eventLogger).Info("Using migration tool").
				String("binary", caps.BinaryPath).
				String("version", caps.Version).
				Log()
		} else {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Info("Using migration tool").
				String("binary", caps.BinaryPath).
				String("version", caps.Version).
				Log()
		}
	}

	// Build command arguments from flags
	migrateArgs := args
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	if source, _ := cmd.Flags().GetString("source"); source != emptyValue {
		migrateArgs = append(migrateArgs, "--source", source)
	}
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	if target, _ := cmd.Flags().GetString("target"); target != emptyValue {
		migrateArgs = append(migrateArgs, "--target", target)
	}
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	if phase, _ := cmd.Flags().GetString("phase"); phase != emptyValue {
		migrateArgs = append(migrateArgs, "--phase", phase)
	}

	// Execute migration binary as subprocess
	//nolint:gosec // G204: Migration binary path is validated, args are controlled
	migrateExec := execwrap.Command(caps.BinaryPath, migrateArgs...)
	migrateExec.Stdout = os.Stdout
	migrateExec.Stderr = os.Stderr
	migrateExec.Stdin = os.Stdin

	if err := migrateExec.Run(); err != nil {
		return errfmt.Newf("migration failed").Wrap(err)
	}

	return nil
}
