package utility

import (
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/appledouble"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewFixHashesCmd creates a hash fixing command
func NewFixHashesCmd() *cobra.Command {
	var (
		recursive bool
		force     bool
		quiet     bool
		kind      string
		internal  bool
		dryRun    bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Fix missing or mismatched integrity hashes",
		"Fix missing or mismatched integrity hashes for object files.",
		"",
		"This command scans YAML files and registers/updates their integrity hashes in",
		"the hash registry. It's useful for fixing files created outside the CLI or",
		"resolving hash mismatches.",
	).
		AddExample("Fix hashes for a single file", "%s utility fix-hashes "+filepath.Join(paths.ProcessBacklogDir, "BLI-001.yaml")).
		AddExample("Fix hashes for all YAML files in a directory (recursive)", "%s utility fix-hashes --recursive "+paths.ProcessBacklogDir+"/").
		AddExample("Fix hashes for all objects of a specific kind", "%s utility fix-hashes --kind audit_event").
		AddExample("Fix hashes for all internal objects", "%s utility fix-hashes --internal").
		AddExample("Force fix hash mismatches (creates audit events)", "%s utility fix-hashes --force "+filepath.Join(paths.ProcessBacklogDir, "BLI-001.yaml")).
		AddExample("Dry run (preview changes without writing)", "%s utility fix-hashes --dry-run --kind backlog_item").
		AddExample("Quiet mode (only show errors)", "%s utility fix-hashes --quiet --recursive "+paths.ProcessDir+"/").
		ExcludeCommonFlags()

	fixHashesCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewUtilityFixHashesCommandBuilder(), &cobra.Command{
		Use: "fix-hashes [path...]",
		Args: func(cmd *cobra.Command, args []string) error {
			// If --kind or --internal is specified, paths are optional
			if kind != emptyValue || internal {
				return nil
			}
			// Otherwise, require at least one path
			if len(args) == 0 {
				return errfmt.Errorf("requires at least one path, or use --kind or --internal")
			}
			return nil
		},
	})
	cli.BindAsyncProgress(fixHashesCmd, func(cmd *cobra.Command, args []string) error {
		return runFixHashes(cmd, args, recursive, force, quiet, kind, internal, dryRun)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(fixHashesCmd)

	fixHashesCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursively fix hashes for YAML files in directories")
	fixHashesCmd.Flags().BoolVarP(&force, "force", "F", false, "Force fix hash mismatches (creates audit events)")
	fixHashesCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only show errors, suppress success messages")
	fixHashesCmd.Flags().StringVar(&kind, "kind", "", "Fix hashes for all objects of the specified kind")
	fixHashesCmd.Flags().BoolVar(&internal, "internal", false, "Fix hashes for all internal objects (audit_event, change_journal_entry)")
	fixHashesCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be fixed without making changes")

	return fixHashesCmd
}

//nolint:gocyclo // Function orchestrates hash fixing; complexity reduced via helper functions
func runFixHashes(cmd *cobra.Command, args []string, recursive, force, quiet bool, kind string, internal bool, dryRun bool) error {
	// Initialize context and logger
	logger, projectRoot, err := initializeFixHashesContext(cmd)
	if err != nil {
		return err
	}

	// Validate flags
	flags := &FixHashesFlags{
		Recursive: recursive,
		Force:     force,
		Quiet:     quiet,
		Kind:      kind,
		Internal:  internal,
		DryRun:    dryRun,
	}
	if err := validateFixHashesFlags(flags, args); err != nil {
		return err
	}

	// Collect files to process
	files, errors := collectFilesForHashFix(projectRoot, args, flags, logger)
	if len(files) == 0 {
		return errfmt.Errorf("no YAML files found to process")
	}

	stdCtx := cmd.Context()
	if stdCtx == nil {
		stdCtx = context.Background() // Background: request-or-shutdown derived
	}

	// Process files and fix hashes
	result := processFilesForHashFix(cmd, stdCtx, files, projectRoot, flags, logger)
	result.Errors = append(result.Errors, errors...)
	result.Quiet = quiet
	result.DryRun = dryRun

	// Output summary
	outputSummary(cmd, result)

	// Return error if there were processing errors
	if result.ErrorCount > 0 {
		return errfmt.Errorf("hash fixing completed with %d error(s)", result.ErrorCount)
	}

	return nil
}

// initializeFixHashesContext initializes context, logger, and project root
func initializeFixHashesContext(cmd *cobra.Command) (logging.Logger, string, error) {
	_, logger, projectRoot, err := initializeUtilityCommandContext(cmd)
	return logger, projectRoot, err
}

// collectFilesForHashFix collects files based on flags and arguments.
// TODO(BLI-903/CRIT-9048): Graph backend — fix-hashes is file-based only. When storage is graph-backed, add no-op with clear message or implement graph hash repair so the command works with both backends.
func collectFilesForHashFix(projectRoot string, args []string, flags *FixHashesFlags, logger logging.Logger) (files, errors []string) {
	if flags.Kind != emptyValue {
		return collectFilesByKind(projectRoot, flags.Kind, logger, flags.Quiet)
	}

	if flags.Internal {
		return collectInternalFiles(projectRoot, logger, flags.Quiet)
	}

	return collectFilesFromArgs(args, flags.Recursive, flags.Quiet)
}

// processFilesForHashFix processes files and fixes hashes
func processFilesForHashFix(cmd *cobra.Command, stdCtx context.Context, files []string, projectRoot string, flags *FixHashesFlags, logger logging.Logger) *FixHashesResult {
	result := &FixHashesResult{
		TotalFiles: len(files),
		Errors:     []string{},
	}

	for i, file := range files {
		showProgress(cmd, flags.Quiet, i, result.TotalFiles)
		processFileForHashFix(cmd, stdCtx, file, projectRoot, flags, logger, result)
	}

	return result
}

// processFileForHashFix processes a single file for hash fixing
func processFileForHashFix(cmd *cobra.Command, stdCtx context.Context, file, projectRoot string, flags *FixHashesFlags, logger logging.Logger, result *FixHashesResult) {
	relPath, err := filepath.Rel(projectRoot, file)
	if err != nil {
		result.ErrorCount++
		result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to get relative path: %v", file, err))
		return
	}

	// Read and parse file
	data, obj, err := readAndParseYAMLFile(file)
	if err != nil {
		result.ErrorCount++
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file, err))
		return
	}

	// Validate object has required fields
	objID, kind, ok := validateObjectFields(obj, relPath, flags.Quiet, logger)
	if !ok {
		result.SkippedCount++
		return
	}

	// Fix hash for the file
	if fixed := fixHashForFile(stdCtx, file, data, kind, objID, relPath, projectRoot, flags, logger, result); fixed {
		result.FixedCount++
		if !flags.Quiet {
			msg := "✓ Fixed hash for %s (%s)\n"
			if flags.DryRun {
				msg = "✓ Would fix hash for %s (%s)\n"
			}
			if outErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf(msg, relPath, objID))); outErr != nil {
				// best effort output
			}
		}
	}
}

// readAndParseYAMLFile reads and parses a YAML file
func readAndParseYAMLFile(file string) (data []byte, obj map[string]any, err error) {
	data, err = fileutil.ReadFile(file)
	if err != nil {
		err = errfmt.Errorf("failed to read file: %w", err)
		return
	}

	obj = make(map[string]any)
	if err = yaml.Unmarshal(data, &obj); err != nil {
		err = errfmt.Errorf("failed to parse YAML: %w", err)
		return
	}

	return
}

// validateObjectFields validates that object has required fields
func validateObjectFields(obj map[string]any, relPath string, quiet bool, logger logging.Logger) (objID, kind string, valid bool) {
	var ok bool
	objID, ok = obj[objects.FieldKeyID].(string)
	if !ok || objID == emptyValue {
		if !quiet {
			logging.Fluent(logger).Debug("Skipping file without ID").
				File(relPath).
				Log()
		}
		return "", "", false
	}

	kind, ok = obj[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		if !quiet {
			logging.Fluent(logger).Debug("Skipping file without kind").
				File(relPath).
				Log()
		}
		return "", "", false
	}

	return objID, kind, true
}

// fixHashForFile fixes the hash for a single file. When dry-run is set, reports what would be fixed without writing.
func fixHashForFile(stdCtx context.Context, file string, data []byte, kind, objID, relPath, projectRoot string, flags *FixHashesFlags, logger logging.Logger, result *FixHashesResult) bool {
	// Get the kind directory
	kindDir := filepath.Dir(file)
	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), kind, kindDir)

	// Load existing registry
	if err := registry.Load(); err != nil && !fileutil.IsNotExist(err) {
		result.ErrorCount++
		result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to load hash registry: %v", file, err))
		return false
	}

	// Compute current hash (SHA256)
	hasher := sha256.New()
	hasher.Write(data)
	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	// Check if hash already exists and matches
	filename := filepath.Base(file)
	existingHash := registry.GetHash(filename)
	if existingHash == hash {
		// Hash already correct, skip
		result.SkippedCount++
		if !flags.Quiet {
			logging.Fluent(logger).Debug("Hash already correct").
				File(relPath).
				Log()
		}
		return false
	}

	// Check for hash mismatch
	isMismatch := existingHash != emptyValue && existingHash != hash
	if isMismatch {
		if !flags.Force {
			result.ErrorCount++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: hash mismatch detected (use --force to fix)", file))
			return false
		}
		// Force fix: update hash
		if !flags.Quiet {
			logging.Fluent(logger).Info("Fixing hash mismatch").
				File(relPath).
				ObjectID(objID).
				Log()
		}
	} else if !flags.Quiet {
		// Missing hash: add it
		logging.Fluent(logger).Info("Registering missing hash").
			File(relPath).
			ObjectID(objID).
			Log()
	}

	// Dry-run: report what would be fixed without persisting
	if flags.DryRun {
		return true
	}

	// Update hash in registry
	registry.SetHash(filename, hash)
	if err := registry.Save(); err != nil {
		result.ErrorCount++
		result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to save hash registry: %v", file, err))
		return false
	}

	// Audit trail: log hash mismatch fix (CRIT-9048)
	if isMismatch && projectRoot != emptyValue {
		if auditErr := storage.CreateAuditEventWithBuilder(stdCtx, projectRoot, nil, nil, &storage.AuditEventOptions{
			EventType:      "hash_mismatch_fix",
			Operation:      fmt.Sprintf("Regenerated integrity hash for %s (hash mismatch resolved)", objID),
			Severity:       "high",
			TargetKind:     kind,
			TargetID:       objID,
			TargetPath:     file,
			OriginalValue:  existingHash,
			NewValue:       hash,
			RecoveryMethod: "force",
		}); auditErr != nil {
			// best effort audit logging
		}
	}

	return true
}

// collectFilesByKind collects all YAML files for objects of a specific kind
func collectFilesByKind(projectRoot, kind string, logger logging.Logger, quiet bool) (files, errors []string) {

	processDir := datacell.ProcessPrimaryDir(projectRoot)
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		errors = append(errors, fmt.Sprintf("unknown object kind: %s", kind))
		return files, errors
	}

	kindDir := filepath.Join(processDir, dirName)
	if _, err := fileutil.Stat(kindDir); fileutil.IsNotExist(err) {
		if !quiet {
			logging.Fluent(logger).Debug("Kind directory does not exist").
				String("kind_dir", kindDir).
				Log()
		}
		return files, errors
	}

	// Walk the kind directory recursively to find all YAML files
	err := filepath.Walk(kindDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if appledouble.SkipDirOrSidecar(info.IsDir(), path) {
			return nil
		}
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			// Skip hash registry files
			if strings.HasPrefix(info.Name(), ".") {
				return nil
			}
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Sprintf("error walking directory %s: %v", kindDir, err))
	}

	return files, errors
}

// collectInternalFiles collects all YAML files for internal objects (audit_event, change_journal_entry)
func collectInternalFiles(projectRoot string, logger logging.Logger, quiet bool) (files, errors []string) {

	internalKinds := []string{objects.KindAuditEvent, objects.KindChangeJournalEntry}

	for _, kind := range internalKinds {
		kindFiles, kindErrors := collectFilesByKind(projectRoot, kind, logger, quiet)
		files = append(files, kindFiles...)
		errors = append(errors, kindErrors...)
	}

	return files, errors
}
