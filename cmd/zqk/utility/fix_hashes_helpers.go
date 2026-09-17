package utility

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// FixHashesFlags contains parsed fix-hashes command flags
type FixHashesFlags struct {
	Recursive bool
	Force     bool
	Quiet     bool
	Kind      string
	Internal  bool
	DryRun    bool
}

// FixHashesResult contains the results of hash fixing
type FixHashesResult struct {
	FixedCount   int
	SkippedCount int
	ErrorCount   int
	TotalFiles   int
	Errors       []string
	Quiet        bool
	DryRun       bool
}

// collectFilesFromArgs collects files from command arguments
func collectFilesFromArgs(args []string, recursive, quiet bool) (files, errors []string) {
	files = []string{}
	errors = []string{}

	for _, arg := range args {
		info, err := fileutil.Stat(arg)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error accessing %s: %v", arg, err))
			continue
		}

		if info.IsDir() {
			if !recursive {
				errors = append(errors, fmt.Sprintf("%s is a directory (use --recursive to process directories)", arg))
				continue
			}
			// Recursively find YAML files
			err := filepath.Walk(arg, func(path string, info fileutil.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && (filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
					if appledouble.SkipPathInTreeWalk(path) {
						return nil
					}
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				errors = append(errors, fmt.Sprintf("Error walking directory %s: %v", arg, err))
			}
		} else {
			// Single file
			if filepath.Ext(arg) != ".yaml" && filepath.Ext(arg) != ".yml" {
				if !quiet {
					errors = append(errors, fmt.Sprintf("Warning: %s does not have .yaml or .yml extension", arg))
				}
			}
			files = append(files, arg)
		}
	}

	return files, errors
}

// validateFixHashesFlags validates fix-hashes command flags
func validateFixHashesFlags(flags *FixHashesFlags, args []string) error {
	if flags.Kind != emptyValue {
		if flags.Internal {
			return errfmt.Errorf("cannot use --kind and --internal together")
		}
		if len(args) > 0 {
			return errfmt.Errorf("cannot specify paths when using --kind")
		}
	} else if flags.Internal {
		if len(args) > 0 {
			return errfmt.Errorf("cannot specify paths when using --internal")
		}
	}
	return nil
}

// showProgress shows progress for large batches (wording per CRIT-9048: "objects")
func showProgress(cmd *cobra.Command, quiet bool, current, total int) {
	if quiet {
		return
	}

	if total > 10 && current == 0 {
		progressMsg := fmt.Sprintf("Processing %d objects...\n", total)
		_ = cli.WriteOutput(cmd, []byte(progressMsg)) //nolint:errcheck
	}

	if total > 100 && current > 0 && current%100 == 0 {
		progressMsg := fmt.Sprintf("Processing %d/%d objects...\r", current, total)
		_ = cli.WriteOutput(cmd, []byte(progressMsg)) //nolint:errcheck
	}
}

// outputSummary outputs the hash fixing summary
func outputSummary(cmd *cobra.Command, result *FixHashesResult) {
	fixedVerb := "fixed"
	if result.DryRun && result.FixedCount > 0 {
		fixedVerb = "would be fixed"
	}
	summary := fmt.Sprintf("\nHash fix summary: %d %s, %d skipped, %d errors (total: %d objects)\n",
		result.FixedCount, fixedVerb, result.SkippedCount, result.ErrorCount, result.TotalFiles)
	_ = cli.WriteOutput(cmd, []byte(summary)) //nolint:errcheck

	if result.DryRun && result.FixedCount+result.SkippedCount+result.ErrorCount > 0 {
		_ = cli.WriteOutput(cmd, []byte("Dry run: no changes made.\n")) //nolint:errcheck
	}

	// Print errors if any
	if len(result.Errors) > 0 {
		errorMsg := "\nErrors:\n"
		for _, err := range result.Errors {
			errorMsg += fmt.Sprintf("  %s\n", err)
		}
		_ = cli.WriteOutput(cmd, []byte(errorMsg)) //nolint:errcheck
	}

	if !result.Quiet && result.FixedCount > 0 && !result.DryRun {
		successMsg := "All hashes fixed successfully\n"
		_ = cli.WriteOutput(cmd, []byte(successMsg)) //nolint:errcheck
	}
}
