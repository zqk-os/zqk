package object

import (
	"fmt"
	"os"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewImportCmd creates the object import command
func NewImportCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectImportCommandBuilder(), &cobra.Command{
		Use:   "import",
		Short: "Import objects from YAML or JSON file",
		Long: fmt.Sprintf(`Import objects from a file (YAML or JSON array).

Examples:
  # Create new objects only (skip existing)
  %s object import --file items.yaml --mode create_only
  # Create or update by ID
  %s object import --file items.json --mode upsert
  # Validate without writing
  %s object import --file items.yaml --dry-run`,
			paths.CLICommandName, paths.CLICommandName, paths.CLICommandName),
		Args: cobra.NoArgs,
		RunE: runImport,
	})
	cli.AddCommonFlags(cmd)
	cmd.Flags().String("file", "", "Path to YAML or JSON file to import")
	cmd.Flags().String("mode", "create_only", "Import mode (create_only, upsert)")
	cmd.Flags().Bool("dry-run", false, "Validate without writing")
	cmd.Flags().Bool("continue-on-error", false, "Continue importing other objects even if one fails")
	cmd.Flags().String("input-format", "", "Input format (yaml, json)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func runImport(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err

		filePath, _ := cmd.Flags().GetString("file")
		if filePath == emptyValue {
			return cli.Guard(cmd).Require(false, "--file is required").Return()
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to read file", err).
				File(filePath).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to read file: %w").Return()
		}

		formatStr, _ := cmd.Flags().GetString("input-format")
		format := storage.ExportFormat(formatStr)

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		modeStr, _ := cmd.Flags().GetString("mode")

		mode := storage.ImportModeCreateOnly
		if modeStr == "upsert" {
			mode = storage.ImportModeUpsert
		}

		options := storage.ImportOptions{
			Mode:            mode,
			ValidateOnly:    dryRun,
			ContinueOnError: continueOnError,
		}

		result, err := storage.ImportObjects(
			proc.OperationContext(),
			proc.Storage(),
			proc.SecurityContext(),
			data,
			format,
			options,
		)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Import failed", err).Log()
			return cli.Guard(cmd).Err(err).Wrapf("import failed: %w").Return()
		}

		if dryRun {
			logging.FluentEvent(proc.Logger()).Info("Import dry-run completed").
				Int("objects", result.Skipped).
				Log()
			return nil
		}

		logging.FluentEvent(proc.Logger()).Info("Import completed").
			Int("created", result.Created).
			Int("updated", result.Updated).
			Int("failed", result.Failed).
			Log()
		if result.Failed > 0 && len(result.Errors) > 0 {
			for _, e := range result.Errors {
				logging.FluentEvent(proc.Logger()).Warn("Import error").
					String("id", e.ID).
					Int("index", e.Index).
					String("message", e.Message).
					Log()
			}
		}
		return nil
	})(cmd, nil)
}
