package utility

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// initializeUtilityCommandContext initializes context, logger, and project root
func initializeUtilityCommandContext(cmd *cobra.Command) (*cli.Context, logging.Logger, string, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return nil, nil, "", errfmt.Errorf("failed to get context")
	}

	profile := ctx.Profile
	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman)
	}
	logger := logging.GetLoggerFromProfile(profile)

	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		return nil, nil, "", errfmt.Errorf("not a ZQK project (no project root found)")
	}

	return ctx, logger, projectRoot, nil
}

func shouldSkipFileWalkCandidate(path string, info fileutil.FileInfo, err error) bool {
	return err != nil || info.IsDir() || appledouble.SkipPathInTreeWalk(path)
}

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
		if err := cli.WriteOutput(cmd, []byte(progressMsg)); err != nil {
			return
		}
	}

	if total > 100 && current > 0 && current%100 == 0 {
		progressMsg := fmt.Sprintf("Processing %d/%d objects...\r", current, total)
		if err := cli.WriteOutput(cmd, []byte(progressMsg)); err != nil {
			return
		}
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
	if err := cli.WriteOutput(cmd, []byte(summary)); err != nil {
		return
	}

	if result.DryRun && result.FixedCount+result.SkippedCount+result.ErrorCount > 0 {
		if err := cli.WriteOutput(cmd, []byte("Dry run: no changes made.\n")); err != nil {
			return
		}
	}

	// Print errors if any
	if len(result.Errors) > 0 {
		errorMsg := "\nErrors:\n"
		for _, err := range result.Errors {
			errorMsg += fmt.Sprintf("  %s\n", err)
		}
		if err := cli.WriteOutput(cmd, []byte(errorMsg)); err != nil {
			return
		}
	}

	if !result.Quiet && result.FixedCount > 0 && !result.DryRun {
		successMsg := "All hashes fixed successfully\n"
		if err := cli.WriteOutput(cmd, []byte(successMsg)); err != nil {
			return
		}
	}
}
