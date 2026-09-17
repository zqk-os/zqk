package utility

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ValidateYAMLFlags contains parsed validate-yaml command flags
type ValidateYAMLFlags struct {
	Recursive      bool
	Quiet          bool
	RegisterHashes bool
}

// ValidateYAMLResult contains the results of YAML validation
type ValidateYAMLResult struct {
	ValidCount     int
	InvalidCount   int
	FixedCount     int
	Errors         []string
	Quiet          bool
	RegisterHashes bool
}

// collectYAMLFiles collects YAML files from arguments
func collectYAMLFiles(args []string, recursive, quiet bool) (files, errors []string) {
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
				errors = append(errors, fmt.Sprintf("%s is a directory (use --recursive to validate directories)", arg))
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

// initializeValidationContext initializes context and logger for hash registration
func initializeValidationContext(cmd *cobra.Command, registerHashes bool) (*cli.Context, logging.Logger, error) {
	if !registerHashes {
		return nil, nil, nil
	}

	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return nil, nil, errfmt.Errorf("failed to get context (required for --register-hashes)")
	}

	profile := ctx.Profile
	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman)
	}
	logger := logging.GetLoggerFromProfile(profile)

	return ctx, logger, nil
}

// outputValidationSummary outputs the validation summary
func outputValidationSummary(cmd *cobra.Command, result *ValidateYAMLResult) {
	summary := fmt.Sprintf("\nValidation summary: %d valid, %d invalid", result.ValidCount, result.InvalidCount)
	if result.RegisterHashes && result.FixedCount > 0 {
		summary += fmt.Sprintf(", %d hashes registered", result.FixedCount)
	}
	summary += "\n"
	_ = cli.WriteOutput(cmd, []byte(summary)) //nolint:errcheck

	// Print errors if any
	if len(result.Errors) > 0 {
		errorMsg := "\nErrors:\n"
		for _, err := range result.Errors {
			errorMsg += fmt.Sprintf("  %s\n", err)
		}
		_ = cli.WriteOutput(cmd, []byte(errorMsg)) //nolint:errcheck
	}

	if !result.Quiet && result.ValidCount > 0 {
		successMsg := "All YAML files are valid"
		if result.RegisterHashes && result.FixedCount > 0 {
			successMsg += fmt.Sprintf(" (%d hashes registered)", result.FixedCount)
		}
		successMsg += "\n"
		_ = cli.WriteOutput(cmd, []byte(successMsg)) //nolint:errcheck
	}
}
