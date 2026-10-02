package internal

import (
	"context"
	"fmt"
	"sync"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewInternalCountCmd creates a new count command for internal objects
func NewInternalCountCmd() *cobra.Command {
	countCmdLong := fmt.Sprintf(`Count internal or built-in objects by kind with optional filtering.

By default, counts only internal or built-in objects (excludes regular public objects).
For total system object count (all objects), use '%s object count' instead.

When no kind is specified, counts all internal/built-in object kinds.
By default, kinds with 0 objects are omitted to reduce noise; use --include-zero-count to include them.
When a single kind is specified, counts objects of that kind.
When multiple kinds are specified (comma-separated), counts each kind.

Use --built-in to filter for only built-in instances.
Use --internal to filter for only internal objects (visibility: internal).
Use --all to count all objects including regular public ones (same scope as 'object count').

This command respects the 'countable' trait - only objects with this trait can be counted.
Use '%s internal <kind> fields' to see which traits are available for a kind.

Examples:
  # Count all internal/built-in object kinds
  %s internal count

  # Count a single kind
  %s internal count test_audit_aggregation_metric

  # Count multiple kinds (comma-separated)
  %s internal count audit_event,change_journal_entry

  # Count with filtering
  %s internal count test_audit_aggregation_metric --filter event_count=20

  # Count with multiple filters
  %s internal count test_audit_aggregation_metric --filter event_count=20 --filter status=active

  # Count grouped by field
  %s internal count test_audit_aggregation_metric --group-by event_count

  # Count only built-in objects
  %s internal count component --built-in

  # Count only internal objects
  %s internal count object_spec --internal

  # Count all objects (including regular public ones)
  %s internal count backlog_item --all

  # Include kinds with 0 objects (e.g. to spot kinds that should have data)
  %s internal count --include-zero-count`, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName)
	countCmd := &cobra.Command{
		Use:   "count [kind|kinds...] [flags]",
		Short: "Count internal or built-in objects by kind",
		Long:  countCmdLong,
		Args:  cobra.MaximumNArgs(1),
		RunE:  runInternalCount,
	}

	// Add common flags
	cli.AddCommonFlags(countCmd)

	// Add count-specific flags using shared utility (includes --include-zero-count; parity with object count)
	clipkg.AddCountFlags(countCmd)
	countCmd.Flags().Bool("built-in", false, "Filter for built-in instances only")
	countCmd.Flags().Bool("internal", false, "Filter for internal objects (visibility: internal) only")
	countCmd.Flags().Bool("all", false, "Count all objects (including regular public objects)")

	// Field-aware flag completions for internal count (group-by + filter).
	_ = countCmd.RegisterFlagCompletionFunc("group-by", internalCompleteGroupBy)
	_ = countCmd.RegisterFlagCompletionFunc("filter", internalCompleteFilterField)

	ensureCmdAnnotations(countCmd)
	countCmd.Annotations[AnnotationKindValidate] = KindValidateCountArg0

	return countCmd
}

//nolint:gocyclo // Function orchestrates count operations; complexity reduced via helper functions
func runInternalCount(cmd *cobra.Command, args []string) error {
	proc, err := newInternalProcessor(cmd)
	if err != nil {
		return err
	}

	projectRoot := proc.ProjectRoot()

	var validatedKinds []string
	if len(args) > 0 {
		if list, ok := kindsListFromInternalPRERun(cmd); ok {
			validatedKinds = list
		} else {
			validatedKinds, err = parseKindsFromArg(projectRoot, args[0])
			if err != nil {
				return err
			}
		}
	}

	// Parse flags
	countFlags, err := parseCountFlags(cmd, proc)
	if err != nil {
		return err
	}

	// Use the same storage as internal list (getStorageProvider) so count and list see the same backend.
	// When graph is enabled, both use graph; when disabled, both use file. Avoids disparity where
	// count used processor/file (e.g. 2045 mcp_sessions) and list used graph (empty).
	storageProvider, err := getStorageProvider(projectRoot)
	if err != nil {
		return errfmt.Newf("failed to initialize storage").Wrap(err)
	}
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()

	// If no kind specified, count all discoverable kinds
	if len(args) == 0 {
		return countAllKinds(cmd, proc, storageProvider, secCtx, storageCtx, countFlags)
	}

	// Handle multiple kinds
	if len(validatedKinds) > 1 {
		return countMultipleKinds(cmd, proc, storageProvider, secCtx, storageCtx, validatedKinds, countFlags)
	}

	// Single kind
	return countSingleKind(cmd, proc, storageProvider, secCtx, storageCtx, validatedKinds[0], countFlags)
}

// countAllKinds counts objects for all discoverable kinds
func countAllKinds(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, flags *CountFlags) error {
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to load field registry", err).Log()
		return errfmt.Newf("failed to load field registry").Wrap(err)
	}

	allKinds, err := registry.GetAllKinds()
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to get available kinds", err).Log()
		return errfmt.Newf("failed to get available kinds").Wrap(err)
	}

	format := proc.Format()

	// Validate group-by
	if flags.GroupBy != emptyValue && flags.GroupBy != "kind" {
		return errfmt.Errorf("--group-by can only be used with 'kind' when counting all kinds, or with a specific kind")
	}

	// Filter kinds based on flags
	var kinds []string
	if flags.AllObjects {
		// Count all kinds (including public ones)
		kinds = allKinds
	} else {
		// Only count internal kinds when not using --all
		kinds = filterInternalKinds(allKinds)
	}

	// Count each kind
	counts := make(map[string]int)
	logging.FluentEvent(proc.Logger()).Debug("Counting objects").
		Int("kind_count", len(kinds)).
		Int("filter_count", len(flags.Filters)).
		Bool("all_objects", flags.AllObjects).
		Log()

	scopeNote := ""
	if !flags.AllObjects {
		scopeNote = paths.RewriteCanonicalCLIInvocations("internal/built-in only; run 'zqk object count' for total system object count")
	}

	useList := shouldUseListForCount(flags)
	if !flags.AllObjects {
		useList = len(flags.Filters) > 0 && !flags.BuiltInOnly && !flags.InternalOnly
	}

	counts, err = executeParallelKindCounts(proc.OperationContext(), proc, storageProvider, secCtx, storageCtx, kinds, flags, useList, false)
	if err != nil {
		return err
	}

	return outputAllKindsCount(cmd, counts, string(format), scopeNote)
}

// countMultipleKinds counts objects for multiple specified kinds
func countMultipleKinds(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kinds []string, flags *CountFlags) error {
	format := proc.Format()
	logging.FluentEvent(proc.Logger()).Debug("Counting objects for multiple kinds").
		Int("kind_count", len(kinds)).
		Log()

	scopeNote := ""
	if !flags.AllObjects {
		scopeNote = paths.RewriteCanonicalCLIInvocations("internal/built-in only; run 'zqk object count' for total system object count")
	}

	useList := shouldUseListForCount(flags)
	counts, err := executeParallelKindCounts(proc.OperationContext(), proc, storageProvider, secCtx, storageCtx, kinds, flags, useList, true)
	if err != nil {
		return err
	}

	return outputAllKindsCount(cmd, counts, string(format), scopeNote)
}

func executeParallelKindCounts(
	ctx context.Context,
	proc *cli.Processor,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	kinds []string,
	flags *CountFlags,
	useList bool,
	warnOnError bool,
) (map[string]int, error) {
	counts := make(map[string]int)
	const maxWorkers = 10 // Limit concurrent counts to avoid overwhelming I/O
	numWorkers := maxWorkers
	if len(kinds) < numWorkers {
		numWorkers = len(kinds)
	}
	if numWorkers == 0 {
		return counts, nil
	}

	workCh := make(chan string, len(kinds))
	for _, kind := range kinds {
		select {
		case <-ctx.Done():
			return nil, errfmt.Newf("context cancelled").Wrap(ctx.Err())
		case workCh <- kind:
		}
	}
	close(workCh)

	type countResult struct {
		kind  string
		count int
		err   error
	}
	results := make(chan countResult, len(kinds))

	var wg sync.WaitGroup
	bud := goroutinelabels.DefaultBudget()
	for w := 0; w < numWorkers; w++ {
		workerID := w
		workerBuilder := goroutinelabels.NewGoroutine("internal_count_worker", fmt.Sprintf("counting objects (worker %d of %d)", workerID, numWorkers))
		if bud != nil {
			workerBuilder = workerBuilder.WithBudget(bud)
		}
		workerBuilder.WithWaitGroup(&wg).StartWithContext(ctx, func(workerCtx context.Context) error {
			for {
				select {
				case <-workerCtx.Done():
					return workerCtx.Err()
				case kind, ok := <-workCh:
					if !ok {
						return nil
					}
					count, err := countKind(proc, storageProvider, secCtx, storageCtx, kind, flags, useList)
					if err != nil && warnOnError {
						logging.FluentEvent(proc.Logger()).Warn("Failed to count objects for kind").
							String("kind", kind).
							WithError(err).
							Log()
						count = 0
					}
					select {
					case <-workerCtx.Done():
						return workerCtx.Err()
					case results <- countResult{kind: kind, count: count, err: err}:
					}
				}
			}
		})
	}

	closerBuilder := goroutinelabels.NewGoroutine("internal_count_results_closer", "waiting for count workers and closing results channel").
		WithCleanup(func() {
			close(results)
		})
	if bud != nil {
		closerBuilder = closerBuilder.WithBudget(bud)
	}
	closerBuilder.StartSimple(func() {
		wg.Wait()
	})

	for res := range results {
		if res.err != nil {
			if !warnOnError {
				logging.FluentEvent(proc.Logger()).Debug("Skipping kind due to error").
					String("kind", res.kind).
					WithError(res.err).
					Log()
				continue
			}
		}
		counts[res.kind] = res.count
	}

	if !flags.IncludeZeroCount {
		for kind, n := range counts {
			if n == 0 {
				delete(counts, kind)
			}
		}
	}

	return counts, nil
}

// countSingleKind counts objects for a single kind
func countSingleKind(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, flags *CountFlags) error {
	logging.FluentEvent(proc.Logger()).Debug("Counting internal objects").
		String("kind", kind).
		Int("filter_count", len(flags.Filters)).
		String("group_by", flags.GroupBy).
		Log()

	// If groupBy is specified, use List() to get grouped results
	if flags.GroupBy != emptyValue {
		return countSingleKindWithGrouping(cmd, proc, storageProvider, secCtx, kind, flags)
	}

	// Use efficient Count() method when no grouping
	return countSingleKindWithoutGrouping(cmd, proc, storageProvider, secCtx, storageCtx, kind, flags)
}

// countKind counts objects for a single kind
func countKind(proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, flags *CountFlags, useList bool) (int, error) {
	listFilter := buildCountFilter(kind, flags)

	if useList {
		result, err := storageProvider.List(proc.OperationContext(), secCtx, storageCtx, listFilter)
		if err != nil {
			return 0, err
		}

		// Filter results by visibility if needed
		result.Objects = filterObjectsByVisibility(result.Objects, flags)
		return getCountFromResult(result), nil
	}

	// Use efficient Count() method when no filters
	return storageProvider.Count(proc.OperationContext(), secCtx, listFilter)
}

// countSingleKindWithGrouping counts a single kind with grouping
func countSingleKindWithGrouping(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, kind string, flags *CountFlags) error {
	listFilter := buildCountFilter(kind, flags)

	// Build storage context for grouping
	groupingCtx := pkgctx.NewGroupingStorageContext(0)
	groupingCtx.EnableGrouping = true

	// List objects (we only need metadata for count)
	result, err := storageProvider.List(proc.OperationContext(), secCtx, groupingCtx, listFilter)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to count objects", err).
			String("kind", kind).
			Log()
		return errfmt.Newf("failed to count objects").Wrap(err)
	}

	// Filter results by visibility if needed
	result.Objects = filterObjectsByVisibility(result.Objects, flags)

	// Output count based on format
	format := proc.Format()
	return outputCount(cmd, result, string(format), kind, flags.GroupBy)
}

// countSingleKindWithoutGrouping counts a single kind without grouping
func countSingleKindWithoutGrouping(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kind string, flags *CountFlags) error {
	// Optimize: if kind is internal and !AllObjects, all objects in that kind are internal
	// so we can use Count() directly without filtering
	kindIsInternal := isInternalKind(kind)
	canUseCountDirectly := flags.AllObjects || kindIsInternal || flags.BuiltInOnly || flags.InternalOnly || len(flags.Filters) == 0

	useList := !canUseCountDirectly && shouldUseListForCount(flags)

	if useList {
		listFilter := buildCountFilter(kind, flags)

		result, err := storageProvider.List(proc.OperationContext(), secCtx, storageCtx, listFilter)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to count objects", err).
				String("kind", kind).
				Log()
			return errfmt.Newf("failed to count objects").Wrap(err)
		}

		// Filter results by visibility if needed
		result.Objects = filterObjectsByVisibility(result.Objects, flags)

		count := getCountFromResult(result)

		// Output count based on format
		format := proc.Format()
		return outputCountDirect(cmd, count, string(format), kind)
	}

	// Use efficient Count() method
	listFilter := buildCountFilter(kind, flags)
	count, err := storageProvider.Count(proc.OperationContext(), secCtx, listFilter)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to count objects", err).
			String("kind", kind).
			Log()
		return errfmt.Newf("failed to count objects").Wrap(err)
	}

	// Output count based on format
	format := proc.Format()
	return outputCountDirect(cmd, count, string(format), kind)
}

// The old runInternalCount function has been refactored into:
// - runInternalCount (main entry point)
// - countAllKinds (handles counting all kinds)
// - countMultipleKinds (handles counting multiple kinds)
// - countSingleKind (handles counting a single kind)
// - Helper functions in count_helpers.go

// outputCount is now a wrapper around shared utility for backward compatibility
func outputCount(cmd *cobra.Command, result *storage.QueryResult, format, kind, groupBy string) error {
	countResult := &clipkg.CountResult{
		Objects: result.Objects,
		Groups:  result.Groups,
		Meta:    result.Meta,
	}
	data, err := clipkg.OutputCount(countResult, format, kind, groupBy)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, data)
}

// outputCountDirect is now a wrapper around shared utility for backward compatibility
func outputCountDirect(cmd *cobra.Command, count int, format, kind string) error {
	data, err := clipkg.OutputCountDirect(count, format, kind, nil)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, data)
}

// outputAllKindsCount is now a wrapper around shared utility for backward compatibility
func outputAllKindsCount(cmd *cobra.Command, counts map[string]int, format string, scopeNote string) error {
	data, err := clipkg.OutputAllKindsCount(counts, format, scopeNote, nil)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, data)
}
