package internal

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// NewInternalListCmd creates a list command for internal objects
func NewInternalListCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List internal or built-in objects",
		"List internal or built-in objects.",
		"",
		"By default, shows only built-in or internal objects (excludes regular public objects).",
		"Use --built-in to filter for only built-in instances.",
		"Use --internal to filter for only internal objects (visibility: internal).",
		"Use --all to show all objects including regular public ones.",
		"",
		"If no kind is specified, lists all internal/built-in objects across all kinds.",
	).
		AddExample("List all built-in and internal objects (across all kinds)", "%s internal list").
		AddExample("List only built-in objects (across all kinds)", "%s internal list --built-in").
		AddExample("List built-in and internal components only", "%s internal list component").
		AddExample("List only built-in component types", "%s internal list component --built-in").
		AddExample("List internal objects only", "%s internal list lifecycle --internal").
		AddExample("List all objects (including regular public ones)", "%s internal list backlog_item --all").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "list [kind] [flags]",
		Args: cobra.MaximumNArgs(1),
		RunE: runInternalList,
	}

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	// Trait harness list flags first (so AddCommonFlags skips format if already added)
	clipkg.AddListFlags(cmd)
	cli.AddCommonFlags(cmd)

	// Internal-specific flags
	cmd.Flags().Bool("built-in", false, "Filter for built-in instances only")
	cmd.Flags().Bool("internal", false, "Filter for internal objects (visibility: internal) only")
	cmd.Flags().Bool("all", false, "Show all objects (including regular public objects)")

	// Field-aware flag completions for internal list (group-by/sort-by/filter).
	_ = cmd.RegisterFlagCompletionFunc("group-by", internalCompleteGroupBy)
	_ = cmd.RegisterFlagCompletionFunc("sort-by", internalCompleteSortBy)
	_ = cmd.RegisterFlagCompletionFunc("filter", internalCompleteFilterField)

	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidateInternalListOptional

	return cmd
}

func runInternalList(cmd *cobra.Command, args []string) error {
	proc, err := newInternalProcessor(cmd)
	if err != nil {
		return err
	}

	projectRoot := proc.ProjectRoot()

	// Get appropriate storage provider (file or graph) - internal commands need graph support
	storageProvider, err := getStorageProvider(projectRoot)
	if err != nil {
		profile := proc.Context().Profile
		if profile == emptyValue {
			profile = string(pkgctx.ProfileSystem)
		}
		emitInternalListErrorViaCoordinator(
			proc.OperationContext(),
			projectRoot,
			storageProvider,
			"initialize_storage",
			"Failed to initialize storage",
			err,
			profile,
		)
		return errfmt.Newf("failed to initialize storage").Wrap(err)
	}

	// Parse flags using shared utilities
	flags, err := parseInternalListFlags(cmd, proc)
	if err != nil {
		return err
	}

	// Extract kind from args
	kind := extractKindFromArgs(args)
	if k, ok := kindCanonicalFromInternalPRERun(cmd); ok {
		kind = k
	} else if kind != emptyValue && kind != internalKindLifecycle && kind != internalKindObjectSpec {
		nk, vErr := objects.ResolveAndValidateKindForProject(projectRoot, kind)
		if vErr != nil {
			return errfmt.Newf("invalid object kind").Wrap(vErr)
		}
		kind = nk
	}

	// Route to appropriate handler
	if err := routeToListHandler(kind, cmd, proc, storageProvider, projectRoot, flags); err != nil {
		return err
	}

	// Single-kind path: use trait harness (ListSource + Run)
	if kind != emptyValue && kind != internalKindLifecycle && kind != internalKindObjectSpec {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = pkgctx.NewSystemContext()
		}
		source := internalListSource{
			proc:            proc,
			storageProvider: storageProvider,
			kind:            kind,
			builtInOnly:     flags.BuiltInOnly,
			internalOnly:    flags.InternalOnly,
			allObjects:      flags.AllObjects,
		}
		cfg := &clipkg.ListConfig{
			TraitGroups:   []string{"read_only_group"},
			TraitExpander: objects.NewTraitRegistry(),
		}
		if err := clipkg.Run(ctx, cmd, source, cfg, cli.CommandOutputWriter(cmd, ctx)); err != nil {
			profile := proc.Context().Profile
			if profile == emptyValue {
				profile = string(pkgctx.ProfileSystem)
			}
			logging.FluentEvent(proc.Logger()).Error("Failed to list objects", err).
				Kind(kind).
				Log()
			emitInternalListErrorViaCoordinator(proc.OperationContext(), projectRoot, storageProvider, "list_kind", "Failed to list objects", err, profile)
			return err
		}
		return nil
	}

	return nil
}

// listAllInternalObjects lists all internal/built-in objects across all kinds
func listAllInternalObjects(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, builtInOnly, internalOnly, allObjects bool, projectRoot string, groupBy string, idsOnly bool) error {
	// Get all discoverable kinds
	fieldRegistry := objects.GetGlobalFieldRegistry()
	profile := proc.Context().Profile
	if profile == emptyValue {
		profile = string(pkgctx.ProfileSystem)
	}
	if err := fieldRegistry.LoadFields(); err != nil {
		emitInternalListErrorViaCoordinator(
			proc.OperationContext(),
			projectRoot,
			storageProvider,
			"load_field_registry",
			"Failed to load field registry",
			err,
			profile,
		)
		return errfmt.Newf("failed to load field registry").Wrap(err)
	}

	kinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		emitInternalListErrorViaCoordinator(
			proc.OperationContext(),
			projectRoot,
			storageProvider,
			"get_all_kinds",
			"Failed to get all kinds",
			err,
			profile,
		)
		return errfmt.Newf("failed to get all kinds").Wrap(err)
	}

	// Calculate per-kind limit
	perKindLimit := calculatePerKindLimit(storageCtx)

	allResults := make([]map[string]any, 0)
	kindCounts := make(map[string]int)
	kindTruncated := make(map[string]bool)

	// Add lifecycle definitions
	allResults = addLifecycleResults(cmd, proc, storageProvider, projectRoot, builtInOnly, internalOnly, allObjects, allResults, kindCounts, kindTruncated, perKindLimit)

	// Add object specifications
	allResults = addSpecResults(cmd, proc, storageProvider, projectRoot, builtInOnly, internalOnly, allObjects, allResults, kindCounts, kindTruncated, perKindLimit)

	// List objects for each kind
	for _, kind := range kinds {
		// Skip lifecycle and object_spec since we already handled them
		if kind == internalKindLifecycle || kind == internalKindObjectSpec {
			continue
		}
		allResults = addKindResults(proc, storageProvider, secCtx, storageCtx, kind, builtInOnly, internalOnly, allObjects, allResults, kindCounts, kindTruncated, perKindLimit)
	}

	// Build grouped result
	result := buildGroupedResult(allResults, groupBy, idsOnly)

	// Output results
	return outputAllInternalResults(cmd, proc, result, allResults, kindCounts, kindTruncated, builtInOnly, internalOnly, allObjects, groupBy, idsOnly)
}
