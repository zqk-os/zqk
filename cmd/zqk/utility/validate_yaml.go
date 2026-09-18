package utility

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewValidateYAMLCmd creates a YAML validation command
func NewValidateYAMLCmd() *cobra.Command {
	var (
		recursive      bool
		quiet          bool
		registerHashes bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Validate YAML file syntax",
		"Validate YAML file syntax without loading as objects.",
		"",
		"This command performs a simple \"gut check\" to verify YAML files are syntactically",
		"valid. It does not validate against object schemas or check references - use",
		"'%s system validate' for that.",
		"",
		"With --register-hashes, valid YAML object files (files with a 'kind' field) will",
		"also have their integrity hashes registered in the hash registry. This is useful",
		"for files created outside the CLI.",
		"",
		"IMPORTANT: Hash registration uses '%s system check --auto-fix' under",
		"the hood to validate objects and register missing hashes. This reuses existing",
		"validation logic instead of duplicating it.",
		"",
		"Security: This command differentiates between:",
		"  - Missing hash (new object): Safe to register automatically",
		"  - Hash mismatch (tampering): Detected and reported, requires --force to fix",
		"",
		"Objects that fail validation will be skipped with a warning.",
		"Use '%s object create' to create properly validated objects, or run",
		"'%s system check' after hash registration to validate references.",
	).
		AddExample("Validate a single file", "%s utility validate-yaml "+filepath.Join(paths.ProcessInternalObjectSpecsDir, "organization.yaml")).
		AddExample("Validate multiple files", "%s utility validate-yaml file1.yaml file2.yaml file3.yaml").
		AddExample("Validate all YAML files in a directory (recursive)", "%s utility validate-yaml --recursive "+paths.ProcessInternalObjectSpecsDir+"/").
		AddExample("Validate and register hashes for valid object files", "%s utility validate-yaml --register-hashes --recursive "+paths.ProcessBacklogDir+"/").
		AddExample("Quiet mode (only show errors)", "%s utility validate-yaml --quiet file.yaml").
		ExcludeCommonFlags()

	validateYAMLCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewUtilityValidateYamlCommandBuilder(), &cobra.Command{
		Use:  "validate-yaml [file...]",
		Args: cobra.MinimumNArgs(1),
	})
	cli.BindAsyncProgress(validateYAMLCmd, func(cmd *cobra.Command, args []string) error {
		return runValidateYAML(cmd, args, recursive, quiet, registerHashes)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(validateYAMLCmd)

	validateYAMLCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursively validate YAML files in directories")
	validateYAMLCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only show errors, suppress success messages")
	validateYAMLCmd.Flags().BoolVar(&registerHashes, "register-hashes", false, "Register integrity hashes for valid YAML object files (does not fix YAML syntax)")

	return validateYAMLCmd
}

//nolint:gocyclo // Function orchestrates YAML validation; complexity reduced via helper functions
func runValidateYAML(cmd *cobra.Command, args []string, recursive, quiet, registerHashes bool) error {
	flags := &ValidateYAMLFlags{
		Recursive:      recursive,
		Quiet:          quiet,
		RegisterHashes: registerHashes,
	}

	// Collect files to validate
	files, errors := collectYAMLFiles(args, flags.Recursive, flags.Quiet)
	if len(files) == 0 {
		return errfmt.Errorf("no YAML files found to validate")
	}

	// Initialize context for hash registration if needed
	ctx, logger, err := initializeValidationContext(cmd, flags.RegisterHashes)
	if err != nil {
		return err
	}

	// Validate each file
	result := processYAMLFiles(cmd, files, ctx, logger, flags)
	result.Errors = append(result.Errors, errors...)

	// Output summary
	outputValidationSummary(cmd, result)

	// Return error if validation failed
	if result.InvalidCount > 0 {
		return errfmt.Errorf("validation failed: %d file(s) have errors", result.InvalidCount)
	}

	return nil
}

// processYAMLFiles processes and validates YAML files
func processYAMLFiles(cmd *cobra.Command, files []string, ctx *cli.Context, logger logging.Logger, flags *ValidateYAMLFlags) *ValidateYAMLResult {
	result := &ValidateYAMLResult{
		Errors:         []string{},
		Quiet:          flags.Quiet,
		RegisterHashes: flags.RegisterHashes,
	}

	for _, file := range files {
		processSingleYAMLFile(cmd, file, ctx, logger, flags, result)
	}

	return result
}

// processSingleYAMLFile processes and validates a single YAML file
func processSingleYAMLFile(cmd *cobra.Command, file string, ctx *cli.Context, logger logging.Logger, flags *ValidateYAMLFlags, result *ValidateYAMLResult) {
	err := validateYAMLFile(file)
	if err != nil {
		result.InvalidCount++
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file, err))
		return
	}

	result.ValidCount++
	if !flags.Quiet {
		// Output success message for each file
		msg := fmt.Sprintf("✓ %s is valid\n", file)
		_ = cli.WriteOutput(cmd, []byte(msg)) //nolint:errcheck
	}

	// Register hash if requested
	if flags.RegisterHashes {
		if err := registerHashViaSystemCheck(ctx, logger, file, flags.Quiet); err != nil {
			// Don't fail validation if hash registration fails, just warn
			if !flags.Quiet {
				warnMsg := fmt.Sprintf("  Warning: failed to register hash for %s: %v\n", file, err)
				_ = cli.WriteOutput(cmd, []byte(warnMsg)) //nolint:errcheck
			}
		} else {
			result.FixedCount++
		}
	}
}

// registerHashViaSystemCheck registers the integrity hash by calling system check
// This reuses the existing validation and hash registration logic from system check
// instead of duplicating it. --auto-fix registers missing hashes. Do not pass --fast:
// reduced check surface cannot combine with --auto-fix (check_fast_contract).
//
// IMPORTANT: This differentiates between:
//   - Missing hash (new object): Safe to register with --auto-fix
//   - Hash mismatch (tampering): Requires --force flag, creates audit event
//
// We check for hash mismatches first and warn the user instead of silently fixing them.
//
//nolint:gocyclo // Function orchestrates hash registration; complexity reduced via helper functions
func registerHashViaSystemCheck(ctx *cli.Context, logger logging.Logger, filePath string, quiet bool) error {
	// Read file content
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return errfmt.Newf("failed to read file").Wrap(err)
	}

	// Parse YAML to get object ID and kind
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Skip files without kind (might be spec files, config files, etc.)
	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		return nil
	}

	// Get object ID
	objID, ok := obj[objects.FieldKeyID].(string)
	if !ok || objID == emptyValue {
		return nil
	}

	// Check for hash mismatch BEFORE calling system check
	// This differentiates between new objects (missing hash) and tampering (mismatch)
	kindDir := filepath.Dir(filePath)
	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), kind, kindDir)
	if err := registry.Load(); err != nil && !fileutil.IsNotExist(err) {
		// Registry load failed - continue anyway, system check will handle it
	} else {
		filename := filepath.Base(filePath)
		existingHash := registry.GetHash(filename)

		if existingHash != emptyValue {
			// Hash exists - check if it matches current file content
			hasher := sha256.New()
			hasher.Write(data)
			currentHash := fmt.Sprintf("%x", hasher.Sum(nil))

			if existingHash != currentHash {
				// Hash mismatch detected - this indicates tampering, not a new object
				relPath, _ := filepath.Rel(ctx.ProjectRoot, filePath) //nolint:errcheck // Path errors use empty string fallback
				if relPath == emptyValue {
					relPath = filePath
				}
				if !quiet {
					logging.Fluent(logger).Warn("Hash mismatch detected - file may have been tampered with").
						File(relPath).
						ObjectID(objID).
						String("expected_hash", existingHash[:16]+"...").
						String("current_hash", currentHash[:16]+"...").
						Log()
				}
				return errfmt.Errorf("hash mismatch detected - file may have been tampered with (use 'zqk system check %s --force' to regenerate hash, creates audit event)", objID)
			}
			// Hash matches - nothing to do
			return nil
		}
		// Hash is missing - safe to register (new object)
	}

	// Register hash via system check
	return registerHashViaSystemCheckCommand(ctx, logger, filePath, objID, quiet)
}

// getRelativePath gets relative path from project root
func getRelativePath(projectRoot, filePath string) string {
	relPath, _ := filepath.Rel(projectRoot, filePath) //nolint:errcheck // Path errors use empty string fallback
	if relPath == emptyValue {
		return filePath
	}
	return relPath
}

// registerHashViaSystemCheckCommand registers hash via system check command
func registerHashViaSystemCheckCommand(ctx *cli.Context, logger logging.Logger, filePath, objID string, quiet bool) error {
	zqkPath := findZQKPath(ctx.ProjectRoot)

	checkCmd := execwrap.Command(zqkPath, "system", "check", objID, "--auto-fix", "--quiet")
	zqkenv.WireExecForIsolatedProject(checkCmd, ctx.ProjectRoot)
	output, err := checkCmd.CombinedOutput()
	if err != nil {
		// System check failed - object may be invalid or have issues
		relPath := getRelativePath(ctx.ProjectRoot, filePath)
		if !quiet {
			logging.Fluent(logger).Debug("System check failed (object may be invalid)").
				File(relPath).
				ObjectID(objID).
				String("output", string(output)).
				Log()
		}
		return errfmt.Errorf("system check failed: %v (object may be invalid - use 'zqk object create' to create valid objects)", err)
	}

	// Hash registration completed (if hash was missing and object is valid)
	if !quiet {
		relPath := getRelativePath(ctx.ProjectRoot, filePath)
		logging.Fluent(logger).Debug("Hash registration completed via system check").
			File(relPath).
			ObjectID(objID).
			Log()
	}

	return nil
}

// findZQKPath finds the zqk executable path
func findZQKPath(projectRoot string) string {
	zqkPath := filepath.Join(projectRoot, "zqk")
	if _, err := fileutil.Stat(zqkPath); fileutil.IsNotExist(err) {
		// Try to find zqk in PATH
		return "zqk"
	}
	return zqkPath
}

func validateYAMLFile(filePath string) error {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return errfmt.Newf("failed to read file").Wrap(err)
	}

	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return errfmt.Newf("YAML syntax error").Wrap(err)
	}

	return nil
}
