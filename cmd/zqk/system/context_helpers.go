package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/strutil"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// getSystemCliContext returns a CLI context initialized for system commands.
func getSystemCliContext(cmd *cobra.Command) (*cli.Context, error) {
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: ProjectRootOrResolve(""),
	}
	return cli.GetContextFromCommand(cmd, initCtx)
}

// bindSystemCliContextRunner binds an async progress handler that resolves a system CLI context.
func bindSystemCliContextRunner(cmd *cobra.Command, fn func(cmd *cobra.Command, ctx *cli.Context, args []string) error) {
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx, err := getSystemCliContext(cmd)
		if err != nil {
			return err
		}
		return fn(cmd, ctx, args)
	})
}

func profileOrDefault(profile, fallback string) string {
	return strutil.OrDefault(profile, fallback)
}

func ProjectRootOrResolve(projectRoot string) string {
	return strutil.OrDefault(projectRoot, cli.ResolveProjectRoot("."))
}

// ProjectRootOrResolveDot resolves when projectRoot is empty or the "." sentinel (unresolved cwd).
// Non-empty paths other than "." are returned unchanged.
func ProjectRootOrResolveDot(projectRoot string) string {
	if projectRoot == emptyValue || projectRoot == "." {
		return cli.ResolveProjectRoot(".")
	}
	return projectRoot
}

func resolveCommandProjectRoot(cmd *cobra.Command) (string, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return "", errfmt.Errorf("failed to get context")
	}
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root not found")
	}
	return projectRoot, nil
}

// openStorageProvider creates a StorageFactory for projectRoot and returns its ObjectStorageProvider along with a deferrable cleanup function.
func openStorageProvider(ctx context.Context, projectRoot string) (storage.ObjectStorageProvider, func(), error) {
	factory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return nil, nil, errfmt.Newf("storage factory").Wrap(err)
	}
	storageProvider := factory.GetStorage()
	cleanup := func() {
		if storageProvider != nil {
			_ = storageProvider.Shutdown(context.Background())
		}
	}
	return storageProvider, cleanup, nil
}

// initSeedingStorage initializes storage provider and security context for pack seeding operations.
func initSeedingStorage(ctx context.Context, projectRoot, purpose string) (storage.ObjectStorageProvider, *pkgctx.SecurityContext, error) {
	if projectRoot == emptyValue {
		return nil, nil, errfmt.Errorf("project root is empty")
	}
	factory, ferr := storage.NewStorageFactory(ctx, projectRoot)
	if ferr != nil {
		return nil, nil, errfmt.Newf("storage factory for %s", purpose).Wrap(ferr)
	}
	sp := factory.GetStorage()
	if sp == nil {
		return nil, nil, errfmt.Errorf("storage provider is nil")
	}
	return sp, pkgctx.NewSystemSecurityContext(), nil
}

// openSystemStorageWithContext resolves CLI context and project root from cmd and opens storage provider with security & storage contexts.
func openSystemStorageWithContext(cmd *cobra.Command) (*cli.Context, storage.ObjectStorageProvider, *pkgctx.SecurityContext, *pkgctx.StorageContext, func(), error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return nil, nil, nil, nil, nil, errfmt.Errorf("failed to get context")
	}

	projectRoot := ProjectRootOrResolveDot(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return nil, nil, nil, nil, nil, errfmt.Errorf("not a ZQK project (no project root found)")
	}

	storageProvider, cleanup, err := openStorageProvider(cmd.Context(), projectRoot)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return ctx, storageProvider, pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), cleanup, nil
}

// initAuditAggregationSetup initializes project root, storage provider, audit aggregation service, and logger from command.
func initAuditAggregationSetup(cmd *cobra.Command) (string, storage.ObjectStorageProvider, *storage.AuditAggregationService, logging.Logger, error) {
	cliCtx := cli.GetContext(cmd)
	projectRoot := ProjectRootOrResolve("")
	if cliCtx != nil && cliCtx.ProjectRoot != "" {
		projectRoot = ProjectRootOrResolve(cliCtx.ProjectRoot)
	}
	if projectRoot == emptyValue {
		return "", nil, nil, nil, errfmt.Errorf("project root not found")
	}

	storageProvider, err := getStorageProvider(cmd, projectRoot)
	if err != nil {
		return "", nil, nil, nil, errfmt.Newf("failed to initialize storage").Wrap(err)
	}

	service := storage.NewAuditAggregationService(storageProvider)

	profile := systemProfileSystem
	if cliCtx != nil && cliCtx.Profile != emptyValue {
		profile = cliCtx.Profile
	}
	logger := logging.GetLoggerFromProfile(profile)

	return projectRoot, storageProvider, service, logger, nil
}

// openCommandMetricsStore ensures the metrics directory exists and returns a FileMetricsStore for the project root.
func openCommandMetricsStore(projectRoot string) (*clipkg.FileMetricsStore, error) {
	metricsPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.CommandMetricsFile)
	dir := filepath.Dir(metricsPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf("failed to create metrics directory").Wrap(err)
	}
	store, err := clipkg.NewFileMetricsStore(metricsPath)
	if err != nil {
		return nil, errfmt.Newf("failed to load metrics store").Wrap(err)
	}
	return store, nil
}

// getFileObjectStorage initializes a storage factory for projectRoot and unwraps its FileObjectStorage.
func getFileObjectStorage(ctx context.Context, projectRoot string) (*storage.FileObjectStorage, error) {
	factory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	mainStorage := factory.GetStorage()
	if fileStorage, ok := mainStorage.(*storage.FileObjectStorage); ok {
		return fileStorage, nil
	}
	if hybrid, isHybrid := mainStorage.(*storage.HybridObjectStorage); isHybrid {
		if fileStorage, ok := hybrid.GetPrimary().(*storage.FileObjectStorage); ok {
			return fileStorage, nil
		}
		return nil, errfmt.Errorf("primary storage in HybridObjectStorage is not FileObjectStorage")
	}
	return nil, errfmt.Errorf("storage provider is not a FileObjectStorage")
}

// formatMigrationSummary formats the report for CAS and DSIA migration operations.
func formatMigrationSummary(title, emptyMsg string, migratedByKind map[string]int, errorsByKind map[string][]error, ctx context.Context) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s Migration completed.\n", title)

	totalMigrated := 0
	for kind, count := range migratedByKind {
		fmt.Fprintf(&b, "  %s: %d objects migrated\n", kind, count)
		totalMigrated += count
	}

	if totalMigrated == 0 {
		fmt.Fprintf(&b, "  %s\n", emptyMsg)
	}

	for kind, errs := range errorsByKind {
		if len(errs) > 0 {
			fmt.Fprintf(&b, "  %s: %d errors\n", kind, len(errs))
			for _, err := range errs {
				fmt.Fprintf(&b, "    - %v\n", err)
			}
		}
	}

	q := caspkg.GetGlobalCASOrphanCleanupQueue()
	if q != nil {
		fmt.Fprintf(&b, "\nRunning Zero-Orphan Cleanup Pipeline...\n")
		processed, err := q.ProcessQueueIfIdle(ctx)
		if err != nil {
			fmt.Fprintf(&b, "  Warning: failed to process orphan cleanup queue: %v\n", err)
		} else {
			fmt.Fprintf(&b, "  %d orphan files processed and safely cleaned up.\n", processed)
		}
	} else {
		fmt.Fprintf(&b, "\nWarning: Zero-Orphan Cleanup Queue is not initialized.\n")
	}

	return b.String()
}


