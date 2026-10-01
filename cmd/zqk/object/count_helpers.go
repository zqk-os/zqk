package object

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
)

// Cap process-dir YAML walk for orphan estimates so all-kinds count stays interactive.
const processDirYAMLWalkCap = 50000

// parseCountFilters parses filter flags for count command using shared utilities
func parseCountFilters(cmd *cobra.Command, proc *cli.Processor) (map[string]any, NamespaceQueryScope, error) {
	filters, _, err := clipkg.ParseCountFlags(cmd, proc.Logger(), ParseFilterString)
	if err != nil {
		return nil, NamespaceQueryScope{}, err
	}
	if filters == nil {
		filters = make(map[string]any)
	}
	scope := applyNamespaceBoundaries(cmd, filters)
	return filters, scope, nil
}

// countAllKinds counts objects for all discoverable kinds
func countAllKinds(cmd *cobra.Command, proc *cli.Processor, filters map[string]any, groupBy string, includeZeroCount bool, nsScope NamespaceQueryScope) error {
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

	kinds, lane, allKindsFlag := applyDiscoveryMembrane(cmd, proc, kinds)

	// Validate group-by for all kinds
	if groupBy != emptyValue && groupBy != "kind" {
		return errfmt.Errorf("--group-by can only be used with 'kind' when counting all kinds, or with a specific kind")
	}

	// Count each kind (skip internal kinds to match object list behavior)
	counts := make(map[string]int)
	var skippedInternal []string
	var skippedErrors []string
	logging.FluentEvent(proc.Logger()).Debug("Counting objects for all kinds").
		Int("kind_count", len(kinds)).
		Int("filter_count", len(filters)).
		String("discovery_lane", string(lane)).
		Bool("all_kinds", allKindsFlag).
		Log()

	// Always Count() — List() was used when filters were present to dodge a countWithFilters
	// deadlock (small workCh buffer). Count is fixed; List would load every object into memory.
	elevated := ElevatedInternalRequested(cmd)
	for _, kind := range kinds {
		process.TouchMeaningfulActivity()
		if ShouldSkipInternalKind(kind, elevated) {
			skippedInternal = append(skippedInternal, kind)
			continue
		}
		count, err := countSingleKind(proc, kind, filters, false)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Debug("Skipping kind due to error").
				String("kind", kind).
				WithError(err).
				Log()
			skippedErrors = append(skippedErrors, kind)
			continue
		}
		counts[kind] = count
	}

	omittedZero := 0
	// Default: omit zero-count kinds to reduce noise; use --include-zero-count to show them (parity with internal count)
	if !includeZeroCount {
		for kind, n := range counts {
			if n == 0 {
				omittedZero++
				delete(counts, kind)
			}
		}
	}

	format := proc.Format()
	scopeNote := allKindsCountScopeNote(elevated)
	hvRollup := rollupHighVolumeInventory(proc)
	meta := buildAllKindsCountMeta(nsScope, elevated, len(kinds), skippedInternal, skippedErrors, omittedZero, includeZeroCount, hvRollup)
	if meta == nil {
		meta = map[string]any{}
	}
	meta["discovery_lane"] = string(lane)
	meta["all_kinds"] = allKindsFlag
	meta["membrane"] = "persona_rbac_discovery"
	data, err := clipkg.OutputAllKindsCount(counts, string(format), scopeNote, meta)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, data)
}

// allKindsCountScopeNote documents default public-vs-internal inventory policy.
func allKindsCountScopeNote(elevated bool) string {
	if elevated {
		return "all registry kinds including visibility:internal (--internal). Zero-count kinds omitted unless --include-zero-count."
	}
	return "public kinds only (visibility≠internal). Internals: object count --internal (entitled) or object count <kind>. " +
		"system check validates a subset from the object ID cache (built from " + paths.ProcessDir + ")."
}

// hvInventoryRollup distinguishes stream vs cas_disk object counts and process-dir orphan YAML.
type hvInventoryRollup struct {
	Stream  int
	CASDisk int
	Orphan  int
}

// rollupHighVolumeInventory counts every high_volume_kinds entry (independent of visibility skip)
// and estimates orphan CAS YAML left under .zqk/process for stream-primary kinds.
func rollupHighVolumeInventory(proc *cli.Processor) hvInventoryRollup {
	var out hvInventoryRollup
	if proc == nil || proc.Storage() == nil {
		return out
	}
	root := proc.ProjectRoot()
	for _, kind := range storage.HighVolumeKindsForCacheBuild() {
		process.TouchMeaningfulActivity()
		n, err := countSingleKind(proc, kind, nil, false)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Debug("HV rollup skip kind").
				String("kind", kind).
				WithError(err).
				Log()
			continue
		}
		if storage.StreamStorageEnabledForKind(kind) {
			out.Stream += n
			// Leftover CAS YAML under process dir while primary inventory is stream (e.g. mcp_sessions).
			if disk := countProcessDirYAMLFiles(root, kind); disk > 0 {
				out.Orphan += disk
			}
			continue
		}
		out.CASDisk += n
		if disk := countProcessDirYAMLFiles(root, kind); disk > n {
			out.Orphan += disk - n
		}
	}
	return out
}

// countProcessDirYAMLFiles counts .yaml/.yml under .zqk/process/<kind-dir> (CAS-shaped leftovers).
func countProcessDirYAMLFiles(projectRoot, kind string) int {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" || projectRoot == "" {
		return 0
	}
	base := filepath.Join(projectRoot, paths.ProcessDir, dirName)
	info, err := fileutil.Stat(base)
	if err != nil || !info.IsDir() {
		return 0
	}
	count := 0
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // best-effort directory walk
		}
		name := d.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			count++
		}
		if count >= processDirYAMLWalkCap {
			return fs.SkipAll
		}
		return nil
	})
	return count
}

// buildAllKindsCountMeta records intentional exclusions + HV stream/cas/orphan rollup for find-vs-count audits.
func buildAllKindsCountMeta(
	nsScope NamespaceQueryScope,
	elevated bool,
	registryKindCount int,
	skippedInternal, skippedErrors []string,
	omittedZero int,
	includeZeroCount bool,
	hv hvInventoryRollup,
) map[string]any {
	vis := "public_kinds_only"
	if elevated {
		vis = "elevated_includes_internal"
	}
	meta := map[string]any{
		"visibility_scope":            vis,
		"registry_kind_count":         registryKindCount,
		"skipped_internal_kind_count": len(skippedInternal),
		"skipped_count_error_kinds":   len(skippedErrors),
		"omitted_zero_count_kinds":    omittedZero,
		"include_zero_count":          includeZeroCount,
		"high_volume_object_counts": map[string]any{
			"stream":   hv.Stream,
			"cas_disk": hv.CASDisk,
			"orphan":   hv.Orphan,
		},
		"intentional_exclusions": "Default all-kinds count skips visibility:internal kinds (match object list) and zero-count kinds. Use --internal and/or --include-zero-count for full registry inventory. HV rollup in meta always includes high_volume_kinds (stream/cas_disk/orphan).",
		"find_vs_count_recipe":   paths.RewriteCanonicalCLIInvocations("Public: zqk object count. Internal inventory: zqk object count --internal (entitled) or zqk object count <kind>. Dir↔kind: .zqk/process/<plural> via kind_mappings_config (list-vocabulary / object fields --list-kinds). HV orphan ≈ process-dir YAML leftovers for stream kinds (or disk−count for cas)."),
	}
	if len(skippedInternal) > 0 && len(skippedInternal) <= 80 {
		meta["skipped_internal_kinds"] = skippedInternal
	} else if len(skippedInternal) > 80 {
		meta["skipped_internal_kinds_sample"] = skippedInternal[:80]
	}
	if len(skippedErrors) > 0 && len(skippedErrors) <= 40 {
		meta["kinds_skipped_on_count_error"] = skippedErrors
	}
	return attachNamespaceScopeMeta(meta, nsScope)
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
func countMultipleKinds(cmd *cobra.Command, proc *cli.Processor, kinds []string, filters map[string]any, includeZeroCount bool, nsScope NamespaceQueryScope) error {
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

	return outputAllKindsCount(cmd, counts, string(format), paths.RewriteCanonicalCLIInvocations("all objects in system (all kinds). zqk system check validates a subset from the object ID cache"), nsScope)
}

// countSingleKindWithGrouping counts a single kind with grouping
func countSingleKindWithGrouping(cmd *cobra.Command, proc *cli.Processor, kind string, filters map[string]any, groupBy string, nsScope NamespaceQueryScope) error {
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

	enrichHiddenOutsideScope(proc, kind, filters, scopedCountFromMeta(result.Meta), &nsScope)
	result.Meta = attachNamespaceScopeMeta(result.Meta, nsScope)

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
func countSingleKindWithoutGrouping(cmd *cobra.Command, proc *cli.Processor, kind string, filters map[string]any, nsScope NamespaceQueryScope) error {
	process.TouchMeaningfulActivity()
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

	enrichHiddenOutsideScope(proc, kind, filters, count, &nsScope)
	meta := attachNamespaceScopeMeta(map[string]any{"total_count": count}, nsScope)

	format := proc.Format()
	data, err := clipkg.OutputCountDirect(count, string(format), kind, meta)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, data)
}
