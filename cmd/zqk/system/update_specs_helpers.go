package system

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"

	"github.com/zqk-os/zqk/pkg/objects"
)

// UpdateSpecsFlags holds parsed flags for update-specs command
type UpdateSpecsFlags struct {
	DryRun         bool
	TraitGroups    bool
	Files          []string
	Validate       bool
	Verbose        bool
	FieldName      string
	Operation      string
	BreakingChange bool
	Reason         string
	MigrationNotes string
	ReplacedBy     string
}

// parseUpdateSpecsFlags parses all flags for update-specs command
func parseUpdateSpecsFlags(cmd *cobra.Command) *UpdateSpecsFlags {
	flags := &UpdateSpecsFlags{}
	flags.DryRun, _ = cmd.Flags().GetBool("dry-run")
	flags.TraitGroups, _ = cmd.Flags().GetBool("trait-groups")
	flags.Files, _ = cmd.Flags().GetStringSlice("files")
	flags.Validate, _ = cmd.Flags().GetBool("validate")
	flags.Verbose, _ = cmd.Flags().GetBool("verbose")
	flags.FieldName, _ = cmd.Flags().GetString("field")
	flags.Operation, _ = cmd.Flags().GetString("operation")
	flags.BreakingChange, _ = cmd.Flags().GetBool("breaking-change")
	flags.Reason, _ = cmd.Flags().GetString("reason")
	flags.MigrationNotes, _ = cmd.Flags().GetString("migration-notes")
	flags.ReplacedBy, _ = cmd.Flags().GetString("replaced-by")
	return flags
}

// getProjectRoot gets the project root from context or finds it
func getProjectRootForUpdateSpecs(ctx *cli.Context) (string, error) {
	return resolveContextProjectRoot(ctx)
}

// handleFieldOperation handles field operation mode (early return)
func handleFieldOperation(cmd *cobra.Command, projectRoot string, flags *UpdateSpecsFlags) error {
	if flags.FieldName != emptyValue && flags.Operation != emptyValue {
		return runFieldOperation(cmd, projectRoot, flags.FieldName, flags.Operation,
			flags.BreakingChange, flags.Reason, flags.MigrationNotes, flags.ReplacedBy,
			flags.DryRun, flags.Verbose, flags.Validate)
	}
	return nil
}

// validateUpdateSpecsInputs validates that required flags are provided
func validateUpdateSpecsInputs(flags *UpdateSpecsFlags) error {
	if flags.FieldName == emptyValue && flags.Operation == emptyValue {
		if !flags.TraitGroups && len(flags.Files) == 0 {
			return errfmt.Errorf("must specify --trait-groups, --files, or field operation (--field + --operation)")
		}
	}
	return nil
}

// getSpecsDirectory gets the specs directory path
func getSpecsDirectory(projectRoot string) (string, error) {
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := fileutil.Stat(specsDir); fileutil.IsNotExist(err) {
		return "", errfmt.Errorf("specs directory not found: %s", specsDir)
	}
	return specsDir, nil
}

// collectFilesToProcess collects the list of files to process
func collectFilesToProcess(specsDir string, files []string, logger logging.Logger) ([]string, error) {
	if len(files) > 0 {
		return collectSpecificFiles(specsDir, files, logger)
	}
	return collectAllSpecFiles(specsDir)
}

// collectSpecificFiles collects specific files requested via flags
func collectSpecificFiles(specsDir string, files []string, logger logging.Logger) ([]string, error) {
	var filesToProcess []string
	for _, f := range files {
		fp := filepath.Join(specsDir, f)
		if _, err := fileutil.Stat(fp); err == nil {
			filesToProcess = append(filesToProcess, fp)
		} else {
			logging.Fluent(logger).Warn("File not found, skipping").
				File(f).
				Log()
		}
	}
	return filesToProcess, nil
}

// collectAllSpecFiles collects all YAML files from the specs directory
func collectAllSpecFiles(specsDir string) ([]string, error) {
	entries, err := fileutil.ReadDir(specsDir)
	if err != nil {
		return nil, errfmt.Newf("failed to read specs directory").Wrap(err)
	}

	var filesToProcess []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		if appledouble.SkipNameInReadDir(entry.Name()) {
			continue
		}
		filesToProcess = append(filesToProcess, filepath.Join(specsDir, entry.Name()))
	}
	return filesToProcess, nil
}

// processSpecFiles processes all spec files and returns updates
func processSpecFiles(filesToProcess []string, traitGroups bool, logger logging.Logger, verbose bool) []specUpdate {
	traitRegistry := objects.NewTraitRegistry()
	var updates []specUpdate

	for _, filepath := range filesToProcess {
		update, err := processSpecFile(filepath, traitGroups, traitRegistry, logger, verbose)
		if err != nil {
			logging.Fluent(logger).Warn("Failed to process file").
				File(filepath).
				WithError(err).
				Log()
			continue
		}
		if update != nil && len(update.changes) > 0 {
			updates = append(updates, *update)
		}
	}

	return updates
}

// showUpdatePreview shows a preview of what will be updated
func showUpdatePreview(updates []specUpdate, out io.Writer) {
	if len(updates) == 0 {
		_, _ = fmt.Fprintf(out, "✅ No updates needed\n")
		return
	}

	_, _ = fmt.Fprintf(out, "\n📋 Found %d spec(s) to update:\n\n", len(updates))
	for _, update := range updates {
		filename := filepath.Base(update.filepath)
		specName := update.specName
		if specName == emptyValue {
			specName = filename
		}
		_, _ = fmt.Fprintf(out, "  📄 %s (%s)\n", filename, specName)
		for _, change := range update.changes {
			_, _ = fmt.Fprintf(out, "    ✓ %s\n", change)
		}
	}
}

// handleDryRunMode handles dry-run mode output
func handleDryRunMode(updates []specUpdate, verbose bool, out io.Writer) {
	_, _ = fmt.Fprintf(out, "\n🔍 Dry-run mode: no changes applied\n")
	if !verbose {
		return
	}

	_, _ = fmt.Fprintf(out, "\n📊 Detailed changes:\n")
	for _, update := range updates {
		_, _ = fmt.Fprintf(out, "\n--- %s ---\n", filepath.Base(update.filepath))
		showDiff(out, update.oldContent, update.content)
	}
}

// applyUpdates applies all updates to files
func applyUpdates(updates []specUpdate, logger logging.Logger, verbose bool, out io.Writer) (int, error) {
	_, _ = fmt.Fprintf(out, "\n🔄 Applying updates...\n")
	updated := 0
	for _, update := range updates {
		if err := fileutil.WriteFile(update.filepath, []byte(update.content), paths.FilePerm644); err != nil { //nolint:gosec // Spec files - 0600 is acceptable
			logging.Fluent(logger).Error("Failed to write file", err).
				File(update.filepath).
				Log()
			continue
		}
		updated++
		if verbose {
			_, _ = fmt.Fprintf(out, "  ✅ Updated: %s\n", filepath.Base(update.filepath))
		}
	}
	return updated, nil
}

// handleValidation handles validation if requested
func handleValidation(validate bool, updates []specUpdate, projectRoot string, logger logging.Logger, out io.Writer) error {
	if !validate {
		return nil
	}

	_, _ = fmt.Fprintf(out, "\n🔍 Validating updated specs...\n")
	if err := validateUpdatedSpecs(updates, projectRoot, logger); err != nil {
		return errfmt.Newf("validation failed").Wrap(err)
	}
	_, _ = fmt.Fprintf(out, "✅ All updated specs passed validation\n")
	return nil
}
