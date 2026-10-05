package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewMigrateCmd creates the migrate command
func NewMigrateCmd() *cobra.Command {
	var (
		dryRun  bool
		force   bool
		spec    string
		backend string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Execute migration specs",
		"Execute migration specs to transform system state from A to B.",
		"",
		"Migrations are spec-driven and automatically create snapshots for rollback capability.",
		"Migrations maintain system coherence and support efficient system copying (spores).",
	).
		AddExample("Run a migration spec", "%s system migrate lifecycle-files-to-objects.yaml").
		AddExample("Migrate to graph backend explicitly", "%s system migrate file-to-graph.yaml --backend graph").
		AddExample("Dry run (preview changes)", "%s system migrate lifecycle-files-to-objects.yaml --dry-run").
		AddExample("Force (skip validation, overwrite existing)", "%s system migrate lifecycle-files-to-objects.yaml --force").
		AddExample("List available migration specs", "%s system migrate --list").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemMigrateCommandBuilder(), &cobra.Command{
		Use:  "migrate [spec-file]",
		Args: cobra.MaximumNArgs(1),
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		if list, _ := cmd.Flags().GetBool("list"); list {
			return listMigrationSpecs(cmd)
		}
		var specFile string
		if len(args) > 0 {
			specFile = args[0]
		} else if spec != emptyValue {
			specFile = spec
		} else {
			return errfmt.Errorf("migration spec file required (use --spec or provide as argument)")
		}
		return runMigrate(cmd, specFile, dryRun, force, backend)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be migrated without actually migrating")
	cmd.Flags().BoolVar(&force, "force", false, "Force migration (skip validation, overwrite existing)")
	cmd.Flags().StringVar(&spec, "spec", "", "Migration spec file (alternative to positional argument)")
	cmd.Flags().StringVar(&backend, "backend", "", "Target storage backend (e.g., 'graph', 'file', 'hybrid')")
	cmd.Flags().Bool("list", false, "List available migration specs")

	return cmd
}

// runMigrate executes a migration spec
func runMigrate(cmd *cobra.Command, specFile string, dryRun, force bool, backend string) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Resolve spec file path
	specPath := specFile
	if !filepath.IsAbs(specPath) {
		// Try migrations directory first
		migrationsDir := filepath.Join(projectRoot, paths.ProcessInternalDir, "migrations")
		migrationsPath := filepath.Join(migrationsDir, specFile)
		if _, err := fileutil.Stat(migrationsPath); err == nil {
			specPath = migrationsPath
		} else {
			// Try relative to current directory
			if _, err := fileutil.Stat(specFile); err != nil {
				return errfmt.Errorf("migration spec file not found: %s", specFile)
			}
		}
	}

	// Load migration spec
	spec, err := migration.LoadSpec(specPath)
	if err != nil {
		return errfmt.Newf("failed to load migration spec").Wrap(err)
	}

	logging.Fluent(logger).Info("Loaded migration spec").
		String("id", spec.ID).
		String("name", spec.Name).
		File(specPath).
		Log()

	// Create storage factory to get appropriate storage provider (supports Graph/Hybrid)
	factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	// Use routing storage for executor by default
	var mainStorage storage.ObjectStorageProvider = storage.NewRoutingObjectStorage(factory)

	// If explicit backend requested, override
	if backend != emptyValue {
		switch strings.ToLower(backend) {
		case "graph":
			// Try to get a graph storage for a common kind to find the provider
			storageForKind := factory.GetStorageForKind(objects.KindBacklogItem)
			// If it's a hybrid storage, we want the underlying graph storage (primary)
			if hybrid, ok := storageForKind.(*storage.HybridObjectStorage); ok {
				mainStorage = hybrid.GetPrimary()
			} else {
				mainStorage = storageForKind
			}
		case "file":
			mainStorage = factory.GetStorage()
		}
	}

	// Create executor
	executor := migration.NewExecutor(mainStorage, projectRoot, logger)
	if spec.SnapshotCompatible {
		executor.SetSnapshotCreator(NewMigrationSnapshotCreator(projectRoot, logger))
	}

	// Execute migration
	ctx := pkgctx.NewSystemContext()
	options := migration.ExecutionOptions{
		DryRun: &dryRun,
		Force:  &force,
	}

	result, err := executor.Execute(ctx, spec, options)
	if err != nil {
		return errfmt.Newf("migration failed").Wrap(err)
	}

	// Output results (POL-CODE-007: command output via cli.WriteOutput)
	var b strings.Builder
	fmt.Fprintf(&b, "\nMigration completed: %s\n", result.Status)
	fmt.Fprintf(&b, "Steps: %d/%d completed\n", result.StepsCompleted, result.StepsTotal)
	fmt.Fprintf(&b, "Duration: %v\n", result.Duration)
	if result.PreSnapshotID != emptyValue {
		fmt.Fprintf(&b, "Pre-migration snapshot: %s\n", result.PreSnapshotID)
	}
	if result.PostSnapshotID != emptyValue {
		fmt.Fprintf(&b, "Post-migration snapshot: %s\n", result.PostSnapshotID)
	}

	if len(result.StepResults) > 0 {
		fmt.Fprintf(&b, "\nStep Results:\n")
		for _, stepResult := range result.StepResults {
			status := "✓"
			if !stepResult.Success {
				status = "✗"
			}
			fmt.Fprintf(&b, "  %s %s", status, stepResult.StepID)
			if stepResult.Metadata != nil {
				if count, ok := stepResult.Metadata["count"].(int); ok {
					fmt.Fprintf(&b, " (count: %d)", count)
				}
				if created, ok := stepResult.Metadata["created"].(int); ok {
					fmt.Fprintf(&b, " (created: %d)", created)
				}
				if skipped, ok := stepResult.Metadata[objects.FieldKeySkipped].(int); ok {
					fmt.Fprintf(&b, " (skipped: %d)", skipped)
				}
				if errors, ok := stepResult.Metadata["errors"].(int); ok && errors > 0 {
					fmt.Fprintf(&b, " (errors: %d)", errors)
				}
			}
			fmt.Fprintf(&b, "\n")
			if stepResult.Error != nil {
				fmt.Fprintf(&b, "    Error: %v\n", stepResult.Error)
			}
		}
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(&b, "\nErrors: %d\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Fprintf(&b, "  - %v\n", err)
		}
	}

	return cli.WriteOutput(cmd, []byte(b.String()))
}

// listMigrationSpecs lists available migration specs and their run status from history.
func listMigrationSpecs(cmd *cobra.Command) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	specs, err := migration.ListSpecs(projectRoot)
	if err != nil {
		return errfmt.Newf("list migration specs").Wrap(err)
	}
	if len(specs) == 0 {
		return cli.WriteOutput(cmd, []byte(fmt.Sprintf(
			"No migration specs found in %s\n",
			filepath.Join(paths.ProcessInternalDir, "migrations"),
		)))
	}

	history, _ := migration.LoadHistory(projectRoot)
	if history == nil {
		history = &migration.MigrationHistory{}
	}
	hasRun := make(map[string]bool)
	for _, e := range history.Entries {
		hasRun[e.MigrationID] = true
	}

	var lb strings.Builder
	fmt.Fprintf(&lb, "Available migration specs:\n\n")
	for _, s := range specs {
		status := "pending"
		if hasRun[s.ID] {
			status = "run"
		}
		fmt.Fprintf(&lb, "  %s - %s [%s]\n", filepath.Base(s.Path), s.Name, status)
		if s.Description != emptyValue {
			fmt.Fprintf(&lb, "    %s\n", s.Description)
		}
	}
	return cli.WriteOutput(cmd, []byte(lb.String()))
}
