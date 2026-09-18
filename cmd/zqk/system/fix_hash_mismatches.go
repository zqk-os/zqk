package system

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
)

// NewFixHashMismatchesCmd creates a command to fix hash mismatches using strategies
func NewFixHashMismatchesCmd() *cobra.Command {
	var (
		strategyName string
		inputFile    string
		kind         string
		dryRun       bool
	)

	cmdLong := fmt.Sprintf(`Fix CAS hash mismatches using configurable strategies.

This command fixes hash mismatches by removing stale CAS index entries,
allowing auto-fix to re-index objects as "missing hash" cases.

Strategies:
  - remove-stale-index (default): Removes stale index entries, safest approach
  - recompute-hash: Recomputes hash from file content (more aggressive)
  - force-reindex: Forces immediate re-indexing (most aggressive)

Input can be provided via:
  - File: --input-file <path>
  - Stdin: pipe object IDs (one per line)
  - Arguments: object IDs as command arguments

Examples:
  # Fix hash mismatches from file using default strategy
  %s system fix-hash-mismatches --input-file .zqk/hash-mismatch-objects.txt

  # Fix specific objects using remove-stale-index strategy
  %s system fix-hash-mismatches --strategy remove-stale-index AUD-25803 SHM-1767592258

  # Fix objects from stdin
  echo -e "AUD-25803\nSHM-1767592258" | %s system fix-hash-mismatches

  # Dry run to see what would be fixed
  %s system fix-hash-mismatches --input-file .zqk/hash-mismatch-objects.txt --dry-run

  # Fix objects of a specific kind (explicit IDs)
  %s system fix-hash-mismatches --kind audit_event AUD-25803 AUD-25804

  # Fix all objects of a kind (no IDs: lists all objects of that kind)
  %s system fix-hash-mismatches --kind backlog_item --strategy force-reindex`,
		paths.CLICommandName, paths.CLICommandName, paths.CLICommandName,
		paths.CLICommandName, paths.CLICommandName, paths.CLICommandName)

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemFixHashMismatchesCommandBuilder(), &cobra.Command{
		Use:   "fix-hash-mismatches [object-id...]",
		Short: "Fix CAS hash mismatches using configurable strategies",
		Long:  cmdLong,
		Args:  cobra.ArbitraryArgs,
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runFixHashMismatches(cmd, args, strategyName, inputFile, kind, dryRun)
	})

	cmd.Flags().StringVar(&strategyName, "strategy", "remove-stale-index",
		"Strategy to use: remove-stale-index, recompute-hash, or force-reindex")
	cmd.Flags().StringVar(&inputFile, "input-file", "",
		"File containing object IDs (one per line). If not provided, reads from stdin or args")
	cmd.Flags().StringVar(&kind, "kind", "",
		"Object kind (if not provided, inferred from object ID prefix)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Show what would be fixed without making changes")

	return cmd
}

func runFixHashMismatches(cmd *cobra.Command, args []string, strategyName, inputFile, kind string, dryRun bool) error {
	ctx := cmd.Context()
	_ = ctx // Used by strategy methods
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Get project root
	projectRoot := ProjectRootOrResolve("")
	if when.IsEmpty(projectRoot) {
		return errfmt.Errorf("project root not found")
	}

	// Read object IDs
	objectIDs, err := readObjectIDs(args, inputFile)
	if err != nil {
		return errfmt.Newf("failed to read object IDs").Wrap(err)
	}

	// When --kind is set and no IDs from args/file/stdin, list all object IDs for that kind
	if len(objectIDs) == 0 && !when.IsEmpty(kind) {
		objectIDs, err = listObjectIDsByKind(cmd, kind)
		if err != nil {
			return errfmt.Newf("failed to list objects for kind %q", kind).Wrap(err)
		}
	}

	if len(objectIDs) == 0 {
		return errfmt.Errorf("no object IDs provided")
	}

	// Create strategy
	strategy, err := createStrategy(strategyName, logger)
	if err != nil {
		return errfmt.Newf("failed to create strategy").Wrap(err)
	}

	logging.Fluent(logger).Info("Fixing hash mismatches").
		String("strategy", strategy.Name()).
		String("description", strategy.Description()).
		ObjectCount(len(objectIDs)).
		String("dry_run", fmt.Sprintf("%v", dryRun)).
		Log()

	if dryRun {
		logging.Fluent(logger).Info("DRY RUN: Would fix the following objects").
			String("object_ids", strings.Join(objectIDs, ", ")).
			Log()
		return nil
	}

	// Create fixer
	fixer := storage.NewHashMismatchFixer(strategy, logger)

	// Fix hash mismatches
	var results []storage.HashMismatchFixResult
	if !when.IsEmpty(kind) {
		// Fix with explicit kind
		results, err = fixer.FixHashMismatches(ctx, objectIDs, kind, projectRoot)
	} else {
		// Infer kind from object IDs
		results, err = fixer.FixHashMismatchesByKind(ctx, objectIDs, projectRoot)
	}

	if err != nil {
		return errfmt.Newf("failed to fix hash mismatches").Wrap(err)
	}

	// Report results
	fixedCount := 0
	notFoundCount := 0
	errorCount := 0

	for _, result := range results {
		if result.Error != nil {
			errorCount++
			logging.Fluent(logger).Warn("Failed to fix hash mismatch").
				String("object_id", result.ObjectID).
				Kind(result.Kind).
				WithError(result.Error).
				Log()
		} else if result.Fixed {
			fixedCount++
			logging.Fluent(logger).Info("Fixed hash mismatch").
				String("object_id", result.ObjectID).
				Kind(result.Kind).
				String("strategy", result.Strategy).
				String("duration", result.Duration.String()).
				Log()
		} else {
			notFoundCount++
			logging.Fluent(logger).Debug("Object not in index").
				String("object_id", result.ObjectID).
				Kind(result.Kind).
				Log()
		}
	}

	// Summary (POL-CODE-007: cli.CommandOutputWriter aligns with WriteOutput / MCP)
	out := cli.CommandOutputWriter(cmd, nil)
	fmt.Fprintf(out, "\n=== Hash Mismatch Fix Results ===\n")
	fmt.Fprintf(out, "Strategy: %s\n", strategy.Name())
	fmt.Fprintf(out, "Total objects: %d\n", len(objectIDs))
	fmt.Fprintf(out, "Fixed: %d\n", fixedCount)
	fmt.Fprintf(out, "Not found in index: %d\n", notFoundCount)
	fmt.Fprintf(out, "Errors: %d\n", errorCount)
	fmt.Fprintf(out, "\nNext step: Run 'zqk system check --auto-fix --tier 0' to re-index\n")

	return nil
}

// listObjectIDsByKind returns all object IDs for the given kind by listing from storage.
// Caller must ensure kind is non-empty.
func listObjectIDsByKind(cmd *cobra.Command, kind string) ([]string, error) {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, errfmt.Newf("processor").Wrap(err)
	}
	secCtx := proc.SecurityContext()
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := proc.StorageContext()
	if storageCtx == nil {
		storageCtx = pkgctx.NewStorageContext()
	}
	result, err := proc.Storage().List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{
		Kind:  kind,
		Limit: 0, // no limit — get all IDs for this kind
	})
	if err != nil {
		return nil, err
	}
	if result == nil || len(result.Objects) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id, _ := obj[objects.FieldKeyID].(string); !when.IsEmpty(id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func readObjectIDs(args []string, inputFile string) ([]string, error) {
	objectIDs := make([]string, 0)

	// First, check if we have command arguments
	if len(args) > 0 {
		objectIDs = append(objectIDs, args...)
	}

	// Then, check if we have an input file
	if !when.IsEmpty(inputFile) {
		data, err := fileutil.ReadFile(inputFile)
		if err != nil {
			return nil, errfmt.Newf("failed to read input file").Wrap(err)
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !when.IsEmpty(line) && !strings.HasPrefix(line, "#") {
				objectIDs = append(objectIDs, line)
			}
		}
	}

	// Finally, check stdin if no args and no file
	if len(objectIDs) == 0 {
		stat, err := os.Stdin.Stat()
		if err != nil {
			return nil, errfmt.Newf("failed to stat stdin").Wrap(err)
		}
		if (stat.Mode() & fileutil.ModeCharDevice) == 0 {
			// Stdin has data
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if !when.IsEmpty(line) && !strings.HasPrefix(line, "#") {
					objectIDs = append(objectIDs, line)
				}
			}
			if err := scanner.Err(); err != nil {
				return nil, errfmt.Newf("failed to read from stdin").Wrap(err)
			}
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := make([]string, 0)
	for _, id := range objectIDs {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	return unique, nil
}

func createStrategy(strategyName string, logger logging.Logger) (storage.HashMismatchFixStrategy, error) {
	switch strategyName {
	case "remove-stale-index":
		return storage.NewRemoveStaleIndexStrategy(logger), nil
	case "recompute-hash":
		return storage.NewRecomputeHashStrategy(logger), nil
	case "force-reindex":
		return storage.NewForceReindexStrategy(logger), nil
	default:
		return nil, errfmt.Errorf("unknown strategy: %s (valid: remove-stale-index, recompute-hash, force-reindex)", strategyName)
	}
}
