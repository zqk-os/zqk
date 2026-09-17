package logging

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/strutil"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
)

var (
	globalRouter     *LogRouter
	globalRouterOnce sync.Once // For InitializeGlobalRouter (file destinations)
	globalRouterMu   sync.RWMutex
)

// InitializeGlobalRouter initializes the global log router with file destinations
// This should be called early in application startup (e.g., in root command init)
// If projectRoot is empty, file logging will be skipped
// formatter determines the log file format and extension:
//   - JSONFormatter -> log-events-{profile}.json
//   - TextFormatter -> log-events-{profile}.log
//   - CompactFormatter -> log-events-{profile}.log
//
// profile is included in the filename to separate logs by profile (e.g., mcp, human, system)
// fileLevel is the minimum level written to the log file (e.g. InfoLevel for info and above).
// Typically from project config logging.level (debug|info|warn|error); default InfoLevel.
func InitializeGlobalRouter(projectRoot string, formatter Formatter, profile string, fileLevel LogLevel) error {
	var initErr error
	globalRouterOnce.Do(func() {
		router := NewLogRouter()

		// Determine log file path based on formatter type and profile
		// Include profile in filename to separate logs by profile
		// Special case: MCP profile uses dedicated trace log location
		var logFilePath string
		profile = strutil.OrDefault(profile, "default")

		// MCP profile uses dedicated trace log location (matches logging_decision_context.go)
		// JSON logs go to .json file, plain text trace logs go to .log file
		when.When(func() bool { return profile == "mcp" }).Then(func() {
			logFilePath = filepath.Join(paths.ProjectDataDir, paths.MCPDir, paths.MCPLogsDir, "mcp-trace.json")
		}).OrElseWhen(func() bool { _, ok := formatter.(*JSONFormatter); return ok }).Then(func() {
			logFilePath = filepath.Join(paths.ProjectDataDir, paths.LogsDir, fmt.Sprintf("log-events-%s.json", profile))
		}).OrElse(func() {
			logFilePath = filepath.Join(paths.ProjectDataDir, paths.LogsDir, fmt.Sprintf("log-events-%s.log", profile))
		}).Run()

		if projectRoot == emptyValue {
			// Skip file logging if projectRoot is empty (as documented)
			_ = concurrency.WithLock(
				&globalRouterMu,
				"router_init_set_global",
				func() error {
					globalRouter = router
					return nil
				},
			)
			return
		}

		// Resolve relative paths relative to project root
		// Absolute paths (starting with /) are used as-is
		var resolvedPath string
		if !filepath.IsAbs(logFilePath) {
			resolvedPath = filepath.Join(projectRoot, logFilePath)
		} else {
			resolvedPath = logFilePath
		}

		// Create directory if needed
		if err := fileutil.EnsureDir(filepath.Dir(resolvedPath)); err != nil {
			initErr = errfmt.Newf("failed to create log directory").Wrap(err)
			return
		}

		// Add file destination with the specified formatter and configured level (from config or default InfoLevel)
		// Use default rolling policy (size-based, 10MB, 5 files) to prevent unbounded growth
		defaultPolicy := DefaultRollingPolicy()
		if err := router.AddFileDestinationWithPolicy("file", resolvedPath, fileLevel, formatter, &defaultPolicy); err != nil {
			initErr = errfmt.Newf("failed to add file destination").Wrap(err)
			return
		}

		_ = concurrency.WithLock(
			&globalRouterMu,
			"router_init_set_global",
			func() error {
				globalRouter = router
				return nil
			},
		)
	})

	return initErr
}

// GetGlobalRouter returns the global log router
// Lazy-initializes with a default router if not yet initialized (or after CloseGlobalRouter)
// Never returns nil
func GetGlobalRouter() *LogRouter {
	var router *LogRouter
	_ = concurrency.WithRLock(
		&globalRouterMu,
		"router_init_get_global_fast",
		func() error {
			router = globalRouter
			return nil
		},
	)
	if router != nil {
		return router
	}

	// Slow path: lazy init or re-init after Close (no Once so we can re-init)
	_ = concurrency.WithLock(
		&globalRouterMu,
		"router_init_lazy_init",
		func() error {
			if globalRouter == nil {
				globalRouter = NewLogRouter()
			}
			return nil
		},
	)
	_ = concurrency.WithRLock(
		&globalRouterMu,
		"router_init_get_global_slow",
		func() error {
			router = globalRouter
			return nil
		},
	)
	return router
}

// CloseGlobalRouter closes all destinations in the global router
// Should be called during application shutdown
func CloseGlobalRouter() error {
	var err error
	_ = concurrency.WithLock(
		&globalRouterMu,
		"router_init_close_global",
		func() error {
			if globalRouter == nil {
				return nil
			}

			err = globalRouter.Close()
			globalRouter = nil
			return nil
		},
	)
	return err
}
