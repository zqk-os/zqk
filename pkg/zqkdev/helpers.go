package zqkdev

import (
	"fmt"

	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ShouldProcessYAMLDirEntry checks standard exclusions (.git, node_modules, AppleDouble, non-yaml, placeholder, hashed).
func ShouldProcessYAMLDirEntry(d os.DirEntry, walkErr error) (skipDir bool, process bool) {
	if walkErr != nil || d == nil {
		return false, false
	}
	if d.IsDir() {
		if d.Name() == ".git" || d.Name() == "node_modules" {
			return true, false
		}
		return false, false
	}
	if appledouble.SkipNameInReadDir(d.Name()) {
		return false, false
	}
	if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml") {
		return false, false
	}
	if d.Name() == "_placeholder.yaml" || objects.IsHashedFilename(d.Name()) {
		return false, false
	}
	return false, true
}

// cmdLogger resolves a logger for a cobra command based on its context profile.
func cmdLogger(cmd *cobra.Command) logging.Logger {
	ctx := cli.GetContext(cmd)
	if ctx != nil && ctx.Profile != EmptyValue {
		return logging.GetLoggerFromProfile(ctx.Profile)
	}
	return logging.GetLoggerFromProfile(SystemProfileHuman)
}

// LogSummaryAndCheckErrors logs the generation summary and returns an error if any errors occurred.
func LogSummaryAndCheckErrors(logger logging.Logger, generated, skipped, errors int) error {
	logging.Fluent(logger).Info(fmt.Sprintf("Summary: Generated %d, Skipped %d, Errors %d", generated, skipped, errors)).Log()
	if errors > 0 {
		return errfmt.Errorf("generation completed with %d errors", errors)
	}
	return nil
}

// logSummaryAndCheckErrors is the package-internal alias for LogSummaryAndCheckErrors.
func logSummaryAndCheckErrors(logger logging.Logger, generated, skipped, errors int) error {
	return LogSummaryAndCheckErrors(logger, generated, skipped, errors)
}

// AddBuilderFlags binds the standard builder generator flags (output-dir, overwrite, and a custom source dir flag) to a command.
func AddBuilderFlags(cmd *cobra.Command, sourceDir *string, sourceFlag, sourceDefault, sourceUsage string, outputDir *string, defaultOutput string, overwrite *bool) {
	cmd.Flags().StringVar(sourceDir, sourceFlag, "", fmt.Sprintf("%s (default: %s)", sourceUsage, sourceDefault))
	cmd.Flags().StringVar(outputDir, "output-dir", "", fmt.Sprintf("Output directory for generated builder files (default: %s)", defaultOutput))
	cmd.Flags().BoolVar(overwrite, "overwrite", false, "Overwrite existing builder files")
	cli.AddCommonFlags(cmd)
}
