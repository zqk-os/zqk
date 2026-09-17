package system

import (
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// NewDetectSpecChangesCmd creates a command to detect manually modified spec files
func NewDetectSpecChangesCmd() *cobra.Command {
	var (
		specsDir    string
		generate    bool
		newVersion  string
		interactive bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Detect manually modified spec files and optionally generate updated builders",
		"Detect manually modified spec files by comparing them with generated builder versions.",
		"",
		"This command helps maintain code-driven specs by:",
		"  - Detecting when YAML spec files differ from builder-generated versions",
		"  - Offering to generate new builder versions to capture manual changes",
		"  - Enabling safe schema evolution through version coexistence",
		"",
		"When a spec file is manually modified, you can generate a new builder version",
		"to capture the change. This ensures:",
		"  - Manual changes become code (builders)",
		"  - Objects with different schema_versions can coexist safely",
		"  - Safe upgrades: old objects keep using old builders, new objects use new builders",
	).
		AddExample("Detect changes (dry-run, just report)", "%s system detect-spec-changes").
		AddExample("Detect and automatically generate new builder versions", "%s system detect-spec-changes --generate").
		AddExample("Generate specific version", "%s system detect-spec-changes --generate --new-version v1_1_0").
		AddExample("Interactive mode (prompt for each change)", "%s system detect-spec-changes --interactive").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemDetectSpecChangesCommandBuilder(), &cobra.Command{
		Use: "detect-spec-changes",
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(systemProfileHuman)
		if ctx != nil && ctx.Profile != emptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		// Determine specs directory
		if specsDir == emptyValue {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return errfmt.Newf("failed to determine project root").Wrap(err)
			}
			specsDir = filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
		}

		// Verify specs directory exists
		if _, err := fileutil.Stat(specsDir); fileutil.IsNotExist(err) {
			return errfmt.Newf("specs directory not found: %s", specsDir).Wrap(err)
		}

		// Create detector
		detector := builders.NewSpecChangeDetector(specsDir)

		// Detect changes
		changes, err := detector.DetectChanges()
		if err != nil {
			return errfmt.Newf("failed to detect changes").Wrap(err)
		}

		if len(changes) == 0 {
			logging.Fluent(logger).Info("✅ No changes detected - all spec files match their builder versions").Log()
			return nil
		}

		// Report changes
		logging.Fluent(logger).Info(fmt.Sprintf("\n📋 Found %d spec(s) with manual modifications:\n", len(changes))).Log()
		for _, change := range changes {
			filename := filepath.Base(change.Filepath)
			logging.Fluent(logger).Info(fmt.Sprintf("\n  📄 %s (%s)", filename, change.Ontology)).Log()
			logging.Fluent(logger).Info(fmt.Sprintf("     Current version: %s", change.CurrentVersion)).Log()
			logging.Fluent(logger).Info(fmt.Sprintf("     Latest builder: %s", change.LatestBuilder)).Log()
			logging.Fluent(logger).Info(fmt.Sprintf("     Changes: %s", change.DiffSummary)).Log()
		}

		// Handle generation
		if generate {
			return generateNewBuilders(changes, newVersion, interactive, logger, specsDir)
		}

		logging.Fluent(logger).Info("\n💡 Tip: Use --generate to create new builder versions for these changes").Log()
		return nil
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&specsDir, "specs-dir", "", "Directory containing spec YAML files (default: "+paths.ProcessInternalObjectSpecsDir+")")
	cmd.Flags().BoolVar(&generate, "generate", false, "Generate new builder versions for detected changes")
	cmd.Flags().StringVar(&newVersion, "new-version", "", "Specific version to generate (e.g., v1_1_0). If not specified, auto-increments from latest")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Interactive mode: prompt for each change")

	cli.AddCommonFlags(cmd)
	return cmd
}

// generateNewBuilders generates new builder versions for detected changes
func generateNewBuilders(changes []builders.SpecChange, newVersion string, interactive bool, logger logging.Logger, specsDir string) error {
	logging.Fluent(logger).Info("\n🔄 Generating new builder versions...\n").Log()

	for _, change := range changes {
		if interactive {
			// TODO: Add interactive prompt
			logging.Fluent(logger).Info(fmt.Sprintf("Would generate builder for %s (interactive mode not yet implemented)", change.Ontology)).Log()
			continue
		}

		// Determine version to use
		version := newVersion
		if version == emptyValue {
			// Auto-increment from latest (patch increment by default)
			registry := builders.GetGlobalRegistry()
			nextVersion, err := builders.GetNextVersion(registry, change.Ontology, "patch")
			if err != nil {
				logging.Fluent(logger).Error(fmt.Sprintf("Failed to determine next version for %s", change.Ontology), err).Log()
				continue
			}
			version = nextVersion
			logging.Fluent(logger).Info(fmt.Sprintf("📦 Auto-incremented version for %s: %s -> %s", change.Ontology, change.LatestBuilder, version)).Log()
		}

		// Generate builder from modified YAML file
		// Find project root to determine output directory
		projectRoot, err := findProjectRoot()
		if err != nil {
			logging.Fluent(logger).Error(fmt.Sprintf("Failed to find project root for %s", change.Ontology), err).Log()
			continue
		}
		outputDir := filepath.Join(projectRoot, "pkg", "specbuilder", "builders")

		// Pass nil for constantsFactory - it will create a new one internally
		// This is fine for individual builder generation
		if err := builders.GenerateBuilderFromYAML(change.Filepath, outputDir, version, nil); err != nil {
			logging.Fluent(logger).Error(fmt.Sprintf("Failed to generate builder for %s", change.Ontology), err).Log()
			continue
		}

		logging.Fluent(logger).Debug(fmt.Sprintf("Generated builder for %s at version %s", change.Ontology, version)).Log()
	}

	return nil
}

// findProjectRoot finds the project root directory
func findProjectRoot() (string, error) {
	// Try ZQK_ROOT env var
	if root := zqkenv.Root().Get(); root != emptyValue {
		return root, nil
	}

	// Try to find from current working directory
	wd, err := fileutil.Getwd()
	if err != nil {
		return "", errfmt.Newf("failed to get working directory").Wrap(err)
	}

	// Walk up to find .zqk or go.mod
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
			return dir, nil
		}
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// Verify it has .zqk/process
			if _, err := fileutil.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
				return dir, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errfmt.Errorf("could not find project root")
}
