package object

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/spf13/cobra"
)

// parseCountFilters parses filter flags for count command using shared utilities
func parseCountFilters(cmd *cobra.Command, proc *cli.Processor) (map[string]any, error) {
	filters, _, err := clipkg.ParseCountFlags(cmd, proc.Logger(), ParseFilterString)
	return filters, err
}

// countAllKinds counts objects for all discoverable kinds
func countAllKinds(cmd *cobra.Command, proc *cli.Processor, filters map[string]any, groupBy string, includeZeroCount bool) error {
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to load field registry", err).Log()
		return errfmt.Newf("failed to load field registry").Wrap(err)
	}

	kinds, err := registry.GetAllKinds()
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to get available kinds", err).Log()
		return errfmt.Newf("failed to get available kinds").Wrap(err)
	}

	// Validate group-by for all kinds
	if groupBy != emptyValue && groupBy != "kind" {
		return errfmt.Errorf("--group-by can only be used with 'kind' when counting all kinds, or with a specific kind")
	}

	// Count each kind (skip internal kinds to match object list behavior)
	counts := make(map[string]int)
	logging.FluentEvent(proc.Logger()).Debug("Counting objects for all kinds").
		Int("kind_count", len(kinds)).
		Int("filter_count", len(filters)).
		Log()

	useList := len(filters) > 0
	for _, kind := range kinds {
		process.TouchMeaningfulActivity()
		// Skip internal kinds (same behavior as object list --count --group-by kind)
		if isInternalKind(kind) {
			continue
		}
		count, err := countSingleKind(proc, kind, filters, useList)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Debug("Skipping kind due to error").
				String("kind", kind).
				WithError(err).
				Log()

			continue
		}
		counts[kind] = count
	}

	// Default: omit zero-count kinds to reduce noise; use --include-zero-count to show them (parity with internal count)
	if !includeZeroCount {
		for kind, n := range counts {
			if n == 0 {
				delete(counts, kind)
			}
		}
	}

	format := proc.Format()
	// Scope note: count includes every kind from the field registry. System check validates only objects
	// in the object ID cache (built from process data dir), so "Validating N objects" may be lower than this total.
	data, err := clipkg.OutputAllKindsCount(counts, string(format), "all objects in system (all kinds). zqk system check validates a subset from the object ID cache (built from "+paths.ProcessDir+")")
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, data)
}

// countSingleKind counts objects for a single kind
func countSingleKind(proc *cli.Processor, kind string, filters map[string]any, useList bool) (int, error) {
	listFilter := storage.ListFilter{
		Kind:    kind,
		Filters: filters,
	}

	if useList {
		// Use List() when filters are applied
		result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), proc.StorageContext(), listFilter)
		if err != nil {
			return 0, err
		}
		if total, ok := result.Meta["total_count"].(int); ok {
			return total, nil
		}
		return len(result.Objects), nil
	}

	// Use efficient Count() method when no filters
	return proc.Storage().Count(proc.OperationContext(), proc.SecurityContext(), listFilter)
}

// countMultipleKinds counts objects for multiple kinds
func countMultipleKinds(cmd *cobra.Command, proc *cli.Processor, kinds []string, filters map[string]any, includeZeroCount bool) error {
	counts := make(map[string]int)
	format := proc.Format()
	logging.FluentEvent(proc.Logger()).Debug("Counting objects for multiple kinds").
		Int("kind_count", len(kinds)).
		Log()

	for _, kind := range kinds {
		listFilter := storage.ListFilter{
			Kind:    kind,
			Filters: filters,
		}
		count, err := proc.Storage().Count(proc.OperationContext(), proc.SecurityContext(), listFilter)
		when.When(func() bool { return err != nil }).Then(func() {
			logging.FluentEvent(proc.Logger()).Warn("Failed to count objects for kind").
				String("kind", kind).
				WithError(err).
				Log()
			counts[kind] = 0
		}).OrElse(func() {
			counts[kind] = count
		}).Run()
	}

	// Default: omit zero-count kinds to reduce noise; use --include-zero-count to show them (parity with internal count)
	if !includeZeroCount {
		for kind, n := range counts {
			if n == 0 {
				delete(counts, kind)
			}
		}
	}

	return outputAllKindsCount(cmd, counts, string(format), "all objects in system (all kinds). zqk system check validates a subset from the object ID cache")
}

// countSingleKindWithGrouping counts a single kind with grouping
func countSingleKindWithGrouping(cmd *cobra.Command, proc *cli.Processor, kind string, filters map[string]any, groupBy string) error {
	listFilter := storage.ListFilter{
		Kind:    kind,
		Filters: filters,
		GroupBy: groupBy,
	}

	storageCtx := pkgctx.NewGroupingStorageContext(0)
	storageCtx.EnableGrouping = true

	result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, listFilter)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to count objects", err).
			String("kind", kind).
			Log()
		return errfmt.Newf("failed to count objects").Wrap(err)
	}

	format := proc.Format()
	countResult := &clipkg.CountResult{
		Objects: result.Objects,
		Groups:  result.Groups,
		Meta:    result.Meta,
	}
	data, err := clipkg.OutputCount(countResult, string(format), kind, groupBy)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, data)
}

// countSingleKindWithoutGrouping counts a single kind without grouping
func countSingleKindWithoutGrouping(cmd *cobra.Command, proc *cli.Processor, kind string, filters map[string]any) error {
	listFilter := storage.ListFilter{
		Kind:    kind,
		Filters: filters,
	}

	count, err := proc.Storage().Count(proc.OperationContext(), proc.SecurityContext(), listFilter)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to count objects", err).
			String("kind", kind).
			Log()
		return errfmt.Newf("failed to count objects").Wrap(err)
	}

	format := proc.Format()
	data, err := clipkg.OutputCountDirect(count, string(format), kind)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, data)
}
