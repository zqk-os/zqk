package system

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
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

