package system

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/pprof"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/appledouble"
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

// resolveCommandLogger returns the CLI context (which may be nil) and a logger resolved from its profile or fallbackProfile.
func resolveCommandLogger(cmd *cobra.Command, fallbackProfile string) (*cli.Context, logging.Logger) {
	ctx := cli.GetContext(cmd)
	profile := fallbackProfile
	if ctx != nil && ctx.Profile != emptyValue {
		profile = ctx.Profile
	}
	return ctx, logging.GetLoggerFromProfile(profile)
}

// resolveContextAndLogger resolves the CLI context and creates a logger using the specified fallback profile.
func resolveContextAndLogger(cmd *cobra.Command, fallbackProfile string) (*cli.Context, logging.Logger, error) {
	ctx, logger := resolveCommandLogger(cmd, fallbackProfile)
	if ctx == nil {
		return nil, nil, errfmt.Errorf("failed to get context")
	}
	return ctx, logger, nil
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

// resolveContextProjectRoot resolves and validates the project root from a CLI context.
func resolveContextProjectRoot(ctx *cli.Context) (string, error) {
	if ctx == nil {
		return "", errfmt.Errorf("failed to get context")
	}
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root not found")
	}
	return projectRoot, nil
}

func resolveCommandProjectRoot(cmd *cobra.Command) (string, error) {
	return resolveContextProjectRoot(cli.GetContext(cmd))
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

func runStorageMigration(cmd *cobra.Command, mode, emptyMsg string, removeOldFiles bool, migrateFn func(fileStorage *storage.FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext) (map[string]int, map[string][]error)) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}
	if _, err := fileutil.Stat(filepath.Join(projectRoot, paths.ProjectDataDir)); err != nil {
		return errfmt.Errorf("project root not found")
	}

	fileStorage, err := getFileObjectStorage(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"read:*", "write:*", "execute:*"})

	migratedByKind, errorsByKind := migrateFn(fileStorage, ctx, secCtx)
	summary := formatMigrationSummary(mode, emptyMsg, migratedByKind, errorsByKind, ctx)
	return cli.WriteOutput(cmd, []byte(summary))
}

// SystemStorageSession bundles CLI context, storage provider, and operational contexts.
type SystemStorageSession struct {
	Ctx             *cli.Context
	StorageProvider storage.ObjectStorageProvider
	SecCtx          *pkgctx.SecurityContext
	StorageCtx      *pkgctx.StorageContext
}

// List executes a list query using the session's security and storage contexts.
func (s *SystemStorageSession) List(ctx context.Context, filter storage.ListFilter) (*storage.QueryResult, error) {
	return s.StorageProvider.List(ctx, s.SecCtx, s.StorageCtx, filter)
}

// Update executes an update using the session's security context.
func (s *SystemStorageSession) Update(ctx context.Context, id string, updates map[string]any) error {
	return s.StorageProvider.Update(ctx, s.SecCtx, id, updates)
}

// withSystemStorageContext executes a function with an active SystemStorageSession, handling cleanup.
func withSystemStorageContext(cmd *cobra.Command, fn func(sess *SystemStorageSession) error) error {
	ctx, storageProvider, secCtx, storageCtx, cleanup, err := openSystemStorageWithContext(cmd)
	if err != nil {
		return err
	}
	defer cleanup()
	return fn(&SystemStorageSession{
		Ctx:             ctx,
		StorageProvider: storageProvider,
		SecCtx:          secCtx,
		StorageCtx:      storageCtx,
	})
}

// resolveProjectRootFromCommand resolves project root from CLI context or fallback detection.
func resolveProjectRootFromCommand(cmd *cobra.Command) string {
	projectRoot := ""
	if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	return ProjectRootOrResolve(projectRoot)
}

// resolveRequiredProjectRoot resolves project root or returns an error if not found.
func resolveRequiredProjectRoot(cmd *cobra.Command) (string, error) {
	projectRoot := resolveProjectRootFromCommand(cmd)
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root not found; run from repo or set --project-root")
	}
	return projectRoot, nil
}

// resolveCommandProfile extracts command profile or defaults to systemProfileHuman.
func resolveCommandProfile(cmd *cobra.Command) string {
	if c := cli.GetContext(cmd); c != nil && c.Profile != "" {
		return c.Profile
	}
	return systemProfileHuman
}

// resolveMigrationProjectAndLogger resolves projectRoot and logger, and primes the path alias cache.
func resolveMigrationProjectAndLogger(cmd *cobra.Command) (string, logging.Logger, error) {
	projectRoot, err := resolveRequiredProjectRoot(cmd)
	if err != nil {
		return "", nil, err
	}
	_, logger := resolveCommandLogger(cmd, systemProfileHuman)
	storage.BuildPathAliasCacheForProject(projectRoot)
	return projectRoot, logger, nil
}

// analyzeCommandMetrics runs metric analysis using the provided metrics store.
func analyzeCommandMetrics(store clipkg.MetricsStore) (*clipkg.MetricsAnalyzer, *clipkg.AnalysisResult, error) {
	analyzer := clipkg.NewMetricsAnalyzer(store)
	analysis, err := analyzer.Analyze()
	if err != nil {
		return nil, nil, errfmt.Newf("failed to analyze metrics").Wrap(err)
	}
	return analyzer, analysis, nil
}

// openAndAnalyzeCommandMetrics opens the metrics store for projectRoot and performs analysis.
func openAndAnalyzeCommandMetrics(projectRoot string) (*clipkg.MetricsAnalyzer, *clipkg.AnalysisResult, error) {
	store, err := openCommandMetricsStore(projectRoot)
	if err != nil {
		return nil, nil, err
	}
	return analyzeCommandMetrics(store)
}

// generateFormattedAnalysisReport formats an analysis result as JSON, YAML, or markdown table.
func generateFormattedAnalysisReport(analyzer *clipkg.MetricsAnalyzer, analysis *clipkg.AnalysisResult, format string) ([]byte, error) {
	switch format {
	case "json":
		return analyzer.GenerateReportJSON(analysis)
	case "yaml":
		return analyzer.GenerateReportYAML(analysis)
	default:
		return []byte(analyzer.GenerateReport(analysis)), nil
	}
}

// ConsoleColors holds standardized color sprint functions for consistent CLI output formatting.
type ConsoleColors struct {
	Bold   func(a ...any) string
	Cyan   func(a ...any) string
	Green  func(a ...any) string
	Yellow func(a ...any) string
	Red    func(a ...any) string
	Dim    func(a ...any) string
}

func newConsoleColors() ConsoleColors {
	return ConsoleColors{
		Bold:   color.New(color.Bold).SprintFunc(),
		Cyan:   color.New(color.FgCyan).SprintFunc(),
		Green:  color.New(color.FgGreen).SprintFunc(),
		Yellow: color.New(color.FgYellow).SprintFunc(),
		Red:    color.New(color.FgRed).SprintFunc(),
		Dim:    color.New(color.Faint).SprintFunc(),
	}
}

// walkYAMLFiles walks dir and invokes fn for each non-skipped YAML file (.yaml or .yml).
func walkYAMLFiles(dir string, fn func(path string, info fileutil.FileInfo) error) error {
	return filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if fileutil.IsYAMLPath(strings.ToLower(path)) {
			return fn(path, info)
		}
		return nil
	})
}

// writeGoroutineProfile writes a goroutine profile to file with the specified debug level (default 0).
func writeGoroutineProfile(filename string, debugLevel ...int) error {
	level := 0
	if len(debugLevel) > 0 {
		level = debugLevel[0]
	}
	f, err := fileutil.Create(filename)
	if err != nil {
		return errfmt.Newf("failed to create goroutine profile").Wrap(err)
	}
	defer f.Close()

	profile := pprof.Lookup("goroutine")
	if profile == nil {
		return errfmt.Errorf("goroutine profile not available")
	}

	if err := profile.WriteTo(f, level); err != nil {
		return errfmt.Newf("failed to write goroutine profile").Wrap(err)
	}

	return nil
}

// loadAndExpandSnapshot loads and expands a compressed snapshot from the specified input path flag.
func loadAndExpandSnapshot(cmd *cobra.Command, projectRoot string) ([]map[string]any, error) {
	inputPath, _ := cmd.Flags().GetString("input")
	if !filepath.IsAbs(inputPath) {
		inputPath = filepath.Join(projectRoot, inputPath)
	}

	if _, err := fileutil.Stat(inputPath); fileutil.IsNotExist(err) {
		return nil, errfmt.Errorf("state file %s does not exist", inputPath)
	}

	cmd.Printf("Reading compressed snapshot from %s...\n", inputPath)
	cs, err := storage.ReadCompressedSnapshot(inputPath)
	if err != nil {
		return nil, errfmt.Newf("failed to read compressed snapshot").Wrap(err)
	}

	cmd.Printf("Snapshot Checksum: %s\n", cs.Header.Checksum)
	cmd.Printf("Expanding %d objects...\n", cs.Header.ObjectCount)

	expanded, err := cs.Expand()
	if err != nil {
		return nil, errfmt.Newf("failed to expand snapshot").Wrap(err)
	}
	return expanded, nil
}


