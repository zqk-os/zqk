package object

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
)

// NewListCmd creates a new list command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewListCmd() *cobra.Command {
	// Use generated builder from object/list DNA (path-qualified object_list stem).
	// shallow DNA must not share list_command_builder.go with scheduler/list.
	cmd := bldr_cli_cmd_v1.NewObjectListCommandBuilder()
	cli.BindAsyncProgress(cmd, runList)

	cmd.Aliases = []string{"ls", "find"}
	// Complete optional [kind] argument with registered object kinds
	cmd.ValidArgsFunction = kindCompletion

	// Field-aware flag completions (kind-specific when a kind is present).
	_ = cmd.RegisterFlagCompletionFunc("group-by", completeGroupBy)
	_ = cmd.RegisterFlagCompletionFunc("sort-by", completeSortBy)
	_ = cmd.RegisterFlagCompletionFunc("filter", completeFilterField)
	_ = cmd.RegisterFlagCompletionFunc("fields", completeListProjectFields)

	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidatePositional0Opt
	addAllKindsFlag(cmd)

	return cmd
}

// ParseFilterString is a re-export of the shared filter parser for backward compatibility
// New code should use clipkg.ParseFilterString directly
var ParseFilterString = clipkg.ParseFilterString

// applyQuietProfileFilters applies terminal status filtering when quiet profile is active
// This automatically excludes complete, archived, closed, and cancelled items
func applyQuietProfileFilters(cmd *cobra.Command, filters map[string]any) {
	ctx := cli.GetContext(cmd)
	if ctx == nil || ctx.Profile != objectProfileQuiet {
		return
	}

	// Terminal statuses to exclude when quiet profile is active
	terminalStatuses := []string{objectStatusComplete, objectStatusArchived, objectStatusClosed, objectStatusCancelled}

	// Check if status filter already exists
	if _, exists := filters[objects.FieldKeyStatus]; exists {
		// If there's already a status filter, don't override it
		// User explicitly set a status filter, so respect it
		// TODO: Could enhance this to merge filters intelligently (e.g., AND the exclusion with existing filter)
		return
	}

	// Apply $nin (not in) filter to exclude terminal statuses
	filters[objects.FieldKeyStatus] = map[string]any{
		"$nin": terminalStatuses,
	}
}

func runList(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		if err := RequireElevatedInternal(cmd); err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		kind, ok := kindCanonicalFromPRERun(cmd)
		if !ok && len(args) > 0 {
			var err error
			kind, err = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), args[0])
			if err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
			ok = true
		}

		if ok && kind != "" {
			return runListSingleKind(cmd, proc, kind)
		}

		return runListAllKinds(cmd, proc)
	})(cmd, args)
}

// runListAllKinds handles listing objects from all kinds
func runListAllKinds(cmd *cobra.Command, proc *cli.Processor) error {
	// Parse flags
	flags, nsScope, err := parseListFlags(cmd, proc)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}

	// Get all kinds
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to load field registry: %w").Return()
	}

	kinds, err := registry.GetAllKinds()
	if err != nil {
		profile := proc.Context().Profile
		if profile == emptyValue {
			profile = string(pkgctx.ProfileSystem)
		}
		emitObjectListErrorViaCoordinator(
			proc.OperationContext(),
			proc.ProjectRoot(),
			proc.Storage(),
			"get_available_kinds",
			"Failed to get available kinds",
			"",
			err,
			profile,
		)
		return cli.Guard(cmd).Err(err).Wrapf("failed to get available kinds: %w").Return()
	}

	kinds, lane, allKinds := applyDiscoveryMembrane(cmd, proc, kinds)

	profile := proc.Context().Profile
	if profile == emptyValue {
		profile = string(pkgctx.ProfileSystem)
	}
	emitObjectListDebugViaCoordinator(
		proc.OperationContext(),
		proc.ProjectRoot(),
		proc.Storage(),
		"list_all_kinds",
		"",
		profile,
		map[string]any{
			"kind_count":     len(kinds),
			"discovery_lane": string(lane),
			"all_kinds":      allKinds,
		},
	)

	// Build storage context
	storageCtx := proc.StorageContext()
	storageCtx.EnableGrouping = true

	// Handle grouping by field vs grouping by kind
	if flags.GroupBy != emptyValue {
		return cli.EnhanceError(cmd, handleListAllKindsWithFieldGrouping(cmd, proc, kinds, flags, storageCtx, nsScope))
	}

	return cli.EnhanceError(cmd, handleListAllKindsByKind(cmd, proc, kinds, flags, storageCtx, nsScope))
}

// handleListAllKindsWithFieldGrouping handles listing all kinds grouped by a specific field
func handleListAllKindsWithFieldGrouping(cmd *cobra.Command, proc *cli.Processor, kinds []string, flags *ListFlags, storageCtx *storage.StorageContext, nsScope NamespaceQueryScope) error {
	elevated := ElevatedInternalRequested(cmd)
	// Collect all objects from all kinds
	allObjects := collectObjectsFromAllKinds(proc, kinds, flags, storageCtx, elevated)

	// Group by specified field
	grouped := groupObjectsByField(allObjects, flags.GroupBy)

	// Apply group limits
	effectiveLimit := calculateEffectiveLimitFromContext(flags.Limit, storageCtx)
	grouped = applyGroupLimits(grouped, flags.GroupLimit, effectiveLimit)

	// Build result
	result := buildGroupedResult(grouped, allObjects)
	result.Meta = attachNamespaceScopeMeta(result.Meta, nsScope)

	// Output results
	return cli.EnhanceError(cmd, outputListResults(cmd, proc, result, "", flags.GroupBy, flags.CountOnly, listTableFieldOrder(flags)))
}

// handleListAllKindsByKind handles listing all kinds grouped by kind
func handleListAllKindsByKind(cmd *cobra.Command, proc *cli.Processor, kinds []string, flags *ListFlags, storageCtx *storage.StorageContext, nsScope NamespaceQueryScope) error {
	elevated := ElevatedInternalRequested(cmd)
	allGroups, totalCount := collectObjectsGroupedByKind(proc, kinds, flags, storageCtx, elevated)

	// Count internal kinds (informational when not elevated)
	internalKindsCount, internalKindsList := countInternalKinds(proc, kinds)

	note := fmt.Sprintf("Showing %d public objects. %d internal objects (audit_event, change_journal_entry, metrics, etc.) are excluded from this list. Use '%s object list --internal' or '%s object list <kind>' for elevated/single-kind access.", totalCount, internalKindsCount, paths.CLICommandName, paths.CLICommandName)
	if elevated {
		note = fmt.Sprintf("Elevated mode (--internal): showing %d objects including internal kinds.", totalCount)
	}

	// Build result
	result := &storage.QueryResult{
		Groups: allGroups,
		Meta: map[string]any{
			"total_groups":     len(allGroups),
			"total_count":      totalCount,
			"public_objects":   totalCount,
			"internal_objects": internalKindsCount,
			"internal_kinds":   internalKindsList,
			"elevated":         elevated,
			"note":             note,
		},
	}
	result.Meta = attachNamespaceScopeMeta(result.Meta, nsScope)

	// Output results
	return cli.EnhanceError(cmd, outputListResults(cmd, proc, result, "", "", flags.CountOnly, listTableFieldOrder(flags)))
}

// runListSingleKind handles listing objects of a specific kind
func runListSingleKind(cmd *cobra.Command, proc *cli.Processor, kind string) error {

	// Parse flags
	flags, nsScope, err := parseListFlags(cmd, proc)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}

	if err := clipkg.ValidateFilterFields(kind, flags.Filters); err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}

	profile := proc.Context().Profile
	if profile == emptyValue {
		profile = string(pkgctx.ProfileSystem)
	}
	emitObjectListDebugViaCoordinator(
		proc.OperationContext(),
		proc.ProjectRoot(),
		proc.Storage(),
		"list_objects",
		kind,
		profile,
		map[string]any{
			"filter_count": len(flags.Filters),
			"sort_by":      flags.SortBy,
			"sort_asc":     fmt.Sprintf("%v", flags.SortAsc),
			"offset":       flags.Offset,
			"limit":        flags.Limit,
			"group_by":     flags.GroupBy,
		},
	)

	// Build storage context
	storageCtx := proc.StorageContext()
	if flags.GroupBy != emptyValue {
		storageCtx.EnableGrouping = true
	}
	if flags.GroupLimit > 0 {
		storageCtx.MaxGroupSize = flags.GroupLimit
	}

	// Build list filter
	listFilter := buildListFilter(kind, flags, storageCtx)

	// Optimization: If only counting, use storage.Count directly
	// This avoids loading all objects into memory, which causes hangs for high-volume streams.
	if flags.CountOnly {
		count, err := proc.Storage().Count(proc.OperationContext(), proc.SecurityContext(), listFilter)
		if err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("failed to count objects: %w").Return()
		}
		enrichHiddenOutsideScope(proc, kind, flags.Filters, count, &nsScope)
		result := &storage.QueryResult{
			Objects: []map[string]any{},
			Meta:    attachNamespaceScopeMeta(map[string]any{"total_count": count}, nsScope),
		}
		return cli.EnhanceError(cmd, outputListResults(cmd, proc, result, kind, flags.GroupBy, true, listTableFieldOrder(flags)))
	}

	// List objects
	result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, listFilter)
	if err != nil {
		profile := proc.Context().Profile
		if profile == emptyValue {
			profile = string(pkgctx.ProfileSystem)
		}
		emitObjectListErrorViaCoordinator(
			proc.OperationContext(),
			proc.ProjectRoot(),
			proc.Storage(),
			"list_objects",
			"Failed to list objects",
			kind,
			err,
			profile,
		)
		return cli.Guard(cmd).Err(err).Wrapf("failed to list objects: %w").Return()
	}

	enrichHiddenOutsideScope(proc, kind, flags.Filters, scopedCountFromMeta(result.Meta), &nsScope)
	if result.Meta == nil {
		result.Meta = make(map[string]any)
	}
	result.Meta = attachNamespaceScopeMeta(result.Meta, nsScope)

	// Reuse profile from error handling above
	emitObjectListDebugViaCoordinator(
		proc.OperationContext(),
		proc.ProjectRoot(),
		proc.Storage(),
		"list_operation_completed",
		kind,
		profile,
		map[string]any{
			"object_count": len(result.Objects),
			"group_count":  len(result.Groups),
		},
	)

	// Output results
	return cli.EnhanceError(cmd, outputListResults(cmd, proc, result, kind, flags.GroupBy, flags.CountOnly, listTableFieldOrder(flags)))
}

// collectObjectsFromAllKinds collects objects from all kinds
func collectObjectsFromAllKinds(proc *cli.Processor, kinds []string, flags *ListFlags, storageCtx *storage.StorageContext, elevated bool) []map[string]any {
	allObjects := make([]map[string]any, 0)

	for _, kind := range kinds {
		if ShouldSkipInternalKind(kind, elevated) {
			continue
		}

		listFilter := storage.ListFilter{
			Kind:    kind,
			Filters: flags.Filters,
			SortBy:  flags.SortBy,
			SortAsc: flags.SortAsc,
			Offset:  0, // Don't apply offset per kind, apply after grouping
			Limit:   0, // Don't apply limit per kind, apply after grouping
			GroupBy: flags.GroupBy,
			Fields:  flags.Fields,
		}

		result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, listFilter)
		if err != nil {
			// Skip kinds that fail (might not have storage directory yet)
			profile := proc.Context().Profile
			if profile == emptyValue {
				profile = string(pkgctx.ProfileSystem)
			}
			emitObjectListDebugViaCoordinator(
				proc.OperationContext(),
				proc.ProjectRoot(),
				proc.Storage(),
				"skip_kind_error",
				kind,
				profile,
				map[string]any{
					"error": err.Error(),
				},
			)
			continue
		}

		allObjects = append(allObjects, result.Objects...)
	}

	return allObjects
}

// applyGroupLimits applies group limits and total limits to grouped results
func applyGroupLimits(grouped map[string][]map[string]any, groupLimit, totalLimit int) map[string][]map[string]any {
	// Store original group sizes for display
	originalGroupSizes := make(map[string]int)
	for groupKey, groupObjects := range grouped {
		originalGroupSizes[groupKey] = len(groupObjects)
	}

	// Apply group limit (items per group) if specified
	if groupLimit > 0 {
		limitedGroups := make(map[string][]map[string]any)
		for groupKey, groupObjects := range grouped {
			when.When(func() bool { return len(groupObjects) > groupLimit }).Then(func() {
				limitedGroups[groupKey] = groupObjects[:groupLimit]
			}).OrElse(func() {
				limitedGroups[groupKey] = groupObjects
			}).Run()
		}
		grouped = limitedGroups
	}

	// Apply total limit across all groups if specified
	if totalLimit > 0 {
		totalShown := 0
		limitedGroups := make(map[string][]map[string]any)
		for groupKey, groupObjects := range grouped {
			if totalShown >= totalLimit {
				break
			}
			remaining := totalLimit - totalShown
			when.When(func() bool { return len(groupObjects) > remaining }).Then(func() {
				limitedGroups[groupKey] = groupObjects[:remaining]
				totalShown += remaining
			}).OrElse(func() {
				limitedGroups[groupKey] = groupObjects
				totalShown += len(groupObjects)
			}).Run()
		}
		grouped = limitedGroups
	}

	return grouped
}

// buildGroupedResult builds a QueryResult from grouped objects
func buildGroupedResult(grouped map[string][]map[string]any, allObjects []map[string]any) *storage.QueryResult {
	meta := map[string]any{
		"total_groups": len(grouped),
		"total_count":  len(allObjects),
	}

	return &storage.QueryResult{
		Groups: grouped,
		Meta:   meta,
	}
}

// collectObjectsGroupedByKind collects objects from all kinds, grouped by kind
func collectObjectsGroupedByKind(proc *cli.Processor, kinds []string, flags *ListFlags, storageCtx *storage.StorageContext, elevated bool) (groups map[string][]map[string]any, totalCount int) {
	allGroups := make(map[string][]map[string]any)
	totalCount = 0
	effectiveLimit := calculateEffectiveLimitFromContext(flags.Limit, storageCtx)

	for _, kind := range kinds {
		if ShouldSkipInternalKind(kind, elevated) {
			continue
		}

		listFilter := storage.ListFilter{
			Kind:    kind,
			Filters: flags.Filters,
			SortBy:  flags.SortBy,
			SortAsc: flags.SortAsc,
			Offset:  flags.Offset,
			Limit:   effectiveLimit,
			GroupBy: flags.GroupBy,
			Fields:  flags.Fields,
		}

		result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, listFilter)
		if err != nil {
			// Skip kinds that fail (might not have storage directory yet)
			profile := proc.Context().Profile
			if profile == emptyValue {
				profile = string(pkgctx.ProfileSystem)
			}
			emitObjectListDebugViaCoordinator(
				proc.OperationContext(),
				proc.ProjectRoot(),
				proc.Storage(),
				"skip_kind_error",
				kind,
				profile,
				map[string]any{
					"error": err.Error(),
				},
			)
			continue
		}

		// Add objects to the group for this kind
		if len(result.Objects) > 0 {
			allGroups[kind] = result.Objects
			totalCount += len(result.Objects)
		}
	}

	return allGroups, totalCount
}

// countInternalKinds counts objects in internal kinds
func countInternalKinds(proc *cli.Processor, kinds []string) (count int, internalKinds []string) {
	internalKindsCount := 0
	internalKindsList := []string{}

	for _, kind := range kinds {
		if isInternalKind(kind) {
			// Count objects in this internal kind
			internalFilter := storage.ListFilter{Kind: kind}
			internalResult, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), proc.StorageContext(), internalFilter)
			if err == nil && internalResult != nil {
				internalKindsCount += len(internalResult.Objects)
				if len(internalResult.Objects) > 0 {
					internalKindsList = append(internalKindsList, kind)
				}
			}
		}
	}

	return internalKindsCount, internalKindsList
}

// The old runList function has been refactored into:
// - runList (main entry point)
// - runListAllKinds (handles listing all kinds)
// - runListSingleKind (handles listing a specific kind)
// - Helper functions in list_helpers.go
// - Output formatting functions in list_output.go

// isInternalKind checks if a kind has visibility: internal in its spec
// Internal kinds should only be accessible via 'internal list' command
func isInternalKind(kind string) bool {
	// Check known internal kinds first (fast path)
	knownInternalKinds := map[string]bool{
		"audit_event":          true,
		"change_journal_entry": true,
		objectKindLifecycle:    true,
		objectKindObjectSpec:   true,
		"template":             true,
		"integrity_manifest":   true,
		"kind_synonym":         true,
	}
	if knownInternalKinds[kind] {
		return true
	}

	// Load spec to check visibility
	specLoader := objects.NewSpecLoader("")
	specFile := kind + ".yaml"
	spec, err := specLoader.LoadSpecWithInheritance(specFile)
	if err != nil {
		// If spec can't be loaded, assume it's not internal (safer default)
		return false
	}

	// Check visibility from spec - only the spec's own visibility matters
	// Visibility is NOT inherited from parent specs
	if spec.Visibility == "internal" {
		return true
	}

	return false
}

// groupObjectsByField groups objects by a specified field value or comma-separated fields
func groupObjectsByField(kindObjects []map[string]any, fieldName string) map[string][]map[string]any {
	groups := make(map[string][]map[string]any)

	fields := strings.Split(fieldName, ",")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}

	for _, obj := range kindObjects {
		var groupKeyParts []string
		for _, field := range fields {
			var part string
			val, ok := obj[field]
			when.When(func() bool { return ok && val != nil && val != emptyValue }).Then(func() {
				str, strOk := val.(string)
				when.When(func() bool { return strOk }).Then(func() {
					part = str
				}).OrElse(func() {
					part = fmt.Sprintf("%v", val)
				}).Run()
			}).OrElse(func() {
				part = "(none)"
			}).Run()
			groupKeyParts = append(groupKeyParts, part)
		}
		groupKey := strings.Join(groupKeyParts, ", ")

		if groups[groupKey] == nil {
			groups[groupKey] = make([]map[string]any, 0)
		}
		groups[groupKey] = append(groups[groupKey], obj)
	}

	return groups
}

// resolveDynamicSynonym resolves dynamic synonym values that require runtime evaluation
// Currently supports:
//   - "current" for priority_plan_ref -> resolves to current priority plan ID
func resolveDynamicSynonym(field, value string, cmdCtx context.Context, storageProvider storage.ObjectStorageProvider) (string, error) {
	normalizedValue := strings.ToLower(strings.TrimSpace(value))

	// Resolve "current" for priority_plan_ref field
	if field == pplanFieldPriorityRef && normalizedValue == "current" {
		return resolveCurrentPriorityPlanID(cmdCtx, storageProvider)
	}

	// Not a dynamic synonym, return as-is
	return value, nil
}

// resolveCurrentPriorityPlanID finds the current priority plan ID
//
//nolint:gocyclo // Function orchestrates priority plan resolution; complexity reduced via helper functions
func resolveCurrentPriorityPlanID(cmdCtx context.Context, storageProvider storage.ObjectStorageProvider) (string, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// First, try to find in_progress plan
	if planID, err := findInProgressPriorityPlan(cmdCtx, storageProvider, secCtx, storageCtx); err == nil && planID != emptyValue {
		return planID, nil
	}

	// If no in_progress, find active plans
	return findActivePriorityPlan(cmdCtx, storageProvider, secCtx, storageCtx)
}

// findInProgressPriorityPlan finds an in_progress priority plan
func findInProgressPriorityPlan(cmdCtx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext) (string, error) {
	filter := storage.ListFilter{
		Kind:    objectKindPriorityPlan,
		Filters: map[string]any{pplanFieldStatus: objectStatusInProgress},
	}

	result, err := storageProvider.List(cmdCtx, secCtx, storageCtx, filter)
	if err != nil {
		return "", errfmt.Newf("failed to query priority plans").Wrap(err)
	}

	if len(result.Objects) == 0 {
		return "", errfmt.Errorf("no in_progress plan found")
	}

	// Return first in_progress plan (should only be one)
	planID, _ := result.Objects[0][objects.FieldKeyID].(string)
	if planID == emptyValue {
		return "", errfmt.Errorf("in_progress plan has no ID")
	}

	return planID, nil
}

// findActivePriorityPlan finds the best active priority plan
func findActivePriorityPlan(cmdCtx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext) (string, error) {
	filter := storage.ListFilter{
		Kind:    objectKindPriorityPlan,
		Filters: map[string]any{pplanFieldStatus: objectStatusActive},
	}

	result, err := storageProvider.List(cmdCtx, secCtx, storageCtx, filter)
	if err != nil {
		return "", errfmt.Newf("failed to query active priority plans").Wrap(err)
	}

	if len(result.Objects) == 0 {
		return "", errfmt.Errorf("no active or in_progress priority plans found")
	}

	// Build plan list with sorting info
	plans := buildPriorityPlanList(result.Objects)

	// Sort plans by active_order and plan_date
	sortPriorityPlans(plans)

	if len(plans) == 0 {
		return "", errfmt.Errorf("no valid priority plan found")
	}

	return plans[0].ID, nil
}

// priorityPlanInfo represents a priority plan with sorting information
type priorityPlanInfo struct {
	ID          string
	ActiveOrder *int
	PlanDate    string
}

// buildPriorityPlanList builds a list of priority plan info from objects
func buildPriorityPlanList(objectList []map[string]any) []priorityPlanInfo {
	plans := make([]priorityPlanInfo, 0, len(objectList))

	for _, obj := range objectList {
		planID, _ := obj[objects.FieldKeyID].(string)
		if planID == emptyValue {
			continue
		}

		activeOrder := extractActiveOrder(obj)
		planDate, _ := obj[objects.FieldKeyPlanDate].(string)

		plans = append(plans, priorityPlanInfo{
			ID:          planID,
			ActiveOrder: activeOrder,
			PlanDate:    planDate,
		})
	}

	return plans
}

// extractActiveOrder extracts active_order from an object
func extractActiveOrder(obj map[string]any) *int {
	switch ao := obj[objects.FieldKeyActiveOrder].(type) {
	case int:
		return &ao
	case float64:
		aoInt := int(ao)
		return &aoInt
	default:
		return nil
	}
}

// sortPriorityPlans sorts plans by active_order (lower first), then plan_date (desc)
func sortPriorityPlans(plans []priorityPlanInfo) {
	sort.Slice(plans, func(i, j int) bool {
		if plans[i].ActiveOrder != nil && plans[j].ActiveOrder != nil {
			if *plans[i].ActiveOrder != *plans[j].ActiveOrder {
				return *plans[i].ActiveOrder < *plans[j].ActiveOrder
			}
		} else if plans[i].ActiveOrder != nil {
			return true
		} else if plans[j].ActiveOrder != nil {
			return false
		}
		return plans[i].PlanDate > plans[j].PlanDate
	})
}
