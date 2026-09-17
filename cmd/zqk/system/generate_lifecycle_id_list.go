package system

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// NewGenerateLifecycleIDListCmd creates a command to generate lifecycle ID list file
func NewGenerateLifecycleIDListCmd() *cobra.Command {
	var (
		output string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate ID list file for lifecycle ID migration",
		"Generate a list of lifecycle object IDs that need to be migrated from old format",
		"to new standardized format.",
		"",
		"This command:",
		"  1. Lists all lifecycle objects from storage",
		"  2. Filters to IDs matching old format (LIFECYCLE-{object_type}-{version})",
		"  3. Outputs IDs as YAML list to lifecycle_ids_to_migrate.yaml",
		"",
		"The generated file can be used as input for the lifecycle-id-migration.yaml spec.",
	).
		AddExample("Generate ID list file (default location)", "%s system generate-lifecycle-id-list").
		AddExample("Generate ID list to custom file", "%s system generate-lifecycle-id-list --output /path/to/id_list.yaml").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-lifecycle-id-list",
	}
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runGenerateLifecycleIDList(cmd, output)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&output, "output", "", "Output file path (default: "+filepath.Join(paths.ProcessInternalDir, "migrations", "lifecycle_ids_to_migrate.yaml")+")")

	return cmd
}

// runGenerateLifecycleIDList generates the lifecycle ID list file
func runGenerateLifecycleIDList(cmd *cobra.Command, outputFile string) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Determine output file path
	if outputFile == emptyValue {
		migrationsDir := filepath.Join(projectRoot, paths.ProcessInternalDir, "migrations")
		outputFile = filepath.Join(migrationsDir, "lifecycle_ids_to_migrate.yaml")
	}

	// Create storage provider
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	fileStorage := factory.GetStorage()
	if fileStorage != nil {
		defer func() { _ = fileStorage.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}

	// List all lifecycle objects
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	filter := storage.ListFilter{
		Kind:    objects.KindLifecycle,
		Filters: make(map[string]any),
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return errfmt.Newf("failed to list lifecycle objects").Wrap(err)
	}

	logging.Fluent(logger).Info("Listed lifecycle objects").
		Int("total_count", len(result.Objects)).
		Log()

	// Filter to old ID format: LIFECYCLE-{object_type}-{version}
	// Old format pattern: LIFECYCLE-[a-z_]+-v\d+(_\d+)*
	// New format pattern: ^LIFECYCLE-[A-Z]+-\d{3,}$
	oldFormatPattern := regexp.MustCompile(`^LIFECYCLE-[a-z_]+-v\d+(_\d+)*$`)
	newFormatPattern := regexp.MustCompile(`^LIFECYCLE-[A-Z]+-\d{3,}$`)

	var oldIDs []string
	for _, obj := range result.Objects {
		objID, ok := obj[objects.FieldKeyID].(string)
		if !ok {
			continue
		}

		// Check if it matches old format (and not new format)
		if oldFormatPattern.MatchString(objID) && !newFormatPattern.MatchString(objID) {
			oldIDs = append(oldIDs, objID)
		}
	}

	logging.Fluent(logger).Info("Filtered to old format IDs").
		Int("old_format_count", len(oldIDs)).
		Int("total_count", len(result.Objects)).
		Log()

	if len(oldIDs) == 0 {
		out := cli.CommandOutputWriter(cmd, ctx)
		fmt.Fprintf(out, "No lifecycle objects with old ID format found.\n")
		fmt.Fprintf(out, "All lifecycle objects already use the new standardized format.\n")
		return nil
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputFile)
	if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create output directory").Wrap(err)
	}

	// Write IDs as YAML list
	data, err := yaml.Marshal(oldIDs)
	if err != nil {
		return errfmt.Newf("failed to marshal ID list").Wrap(err)
	}

	// Write to file
	if err := fileutil.WriteFile(outputFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Generated files - 0600 is acceptable for user-readable files
		return errfmt.Newf("failed to write ID list file").Wrap(err)
	}

	out := cli.CommandOutputWriter(cmd, ctx)
	fmt.Fprintf(out, "Generated lifecycle ID list: %s\n", outputFile)
	fmt.Fprintf(out, "Found %d lifecycle objects with old ID format\n", len(oldIDs))
	fmt.Fprintf(out, "\nTo migrate these IDs, run:\n")
	fmt.Fprintf(out, "  zqk system migrate lifecycle-id-migration.yaml\n")

	return nil
}
