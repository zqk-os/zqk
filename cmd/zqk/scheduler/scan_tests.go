package scheduler

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testscan"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewScanTestsCmd creates the scan-tests command.
// Flags, help text, and exclude_flags are driven by the command spec at
// .zqk/cli/specs/scheduler/scan_tests_command.yaml via the generated builder.
func NewScanTestsCmd() *cobra.Command {
	cmd := bldr.NewSchedulerScanTestsCommandBuilder()
	cli.BindAsyncProgress(cmd, runScanTests)
	return cmd
}

func runScanTests(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return scanAndScheduleTests(ctx, cmd)
}

func scanAndScheduleTests(cliCtx *cli.Context, cmd *cobra.Command) error {
	ctx := pkgctx.NewSystemContext()
	cli.TouchMeaningfulActivity() // idle watchdog: we started real work

	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue && cliCtx != nil {
		projectRoot = cliCtx.ProjectRoot
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	// Get flags needed for early exits
	packagePath, _ := cmd.Flags().GetString("package")
	suggestTimeout, _ := cmd.Flags().GetBool("suggest-timeout")
	if suggestTimeout {
		if packagePath == emptyValue {
			return errfmt.Errorf("--suggest-timeout requires --package")
		}
		pkgs := parsePackageList(packagePath)
		if len(pkgs) == 0 {
			return errfmt.Errorf("--package must list at least one path when using --suggest-timeout")
		}
		maxSec := 0
		for _, pkg := range pkgs {
			seconds, err := testscan.SuggestedTimeoutForPackage(projectRoot, pkg)
			if err != nil {
				return errfmt.Errorf("suggest timeout for %s: %w", pkg, err)
			}
			if seconds > maxSec {
				maxSec = seconds
			}
		}
		if err := cli.WriteOutput(cmd, []byte(fmt.Sprintf("%d\n", maxSec))); err != nil {
			return err
		}
		return nil
	}

	// Create storage provider
	storageFactory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	secCtx := pkgctx.NewSystemSecurityContext()

	// Get flags (packagePath and suggestTimeout read above for early --suggest-timeout exit)
	singleTest, _ := cmd.Flags().GetString("test")
	multipleTests, _ := cmd.Flags().GetString("tests")
	allTests, _ := cmd.Flags().GetBool("all")
	maxBundleSize, _ := cmd.Flags().GetInt("max-bundle-size")
	maxParallel, _ := cmd.Flags().GetInt("max-parallel")

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Get bundle management flags
	saveBundle, _ := cmd.Flags().GetString("save-bundle")
	loadBundle, _ := cmd.Flags().GetString("load-bundle")
	loadBundlesStr, _ := cmd.Flags().GetString("load-bundles")
	listBundles, _ := cmd.Flags().GetBool("list-bundles")
	deleteAllBundles, _ := cmd.Flags().GetBool("delete-all-bundles")
	showBundle, _ := cmd.Flags().GetBool("show-bundle")
	onlyIndexesStr, _ := cmd.Flags().GetString("only")
	skipIndexesStr, _ := cmd.Flags().GetString("skip")
	overwrite, _ := cmd.Flags().GetBool("overwrite")
	merge, _ := cmd.Flags().GetBool("merge")
	removeMissing, _ := cmd.Flags().GetBool("remove-missing")
	setupBundles, _ := cmd.Flags().GetBool("setup-bundles")
	out := cli.CommandOutputWriter(cmd, ctx)
	if logger != nil {
		logger.Warn("zqk scheduler scan-tests is deprecated; migrate to zqk test discover and zqk test run",
			logging.String("deprecation_code", "DEP-SCAN-TESTS-001"),
			logging.String("alternative", "zqk test run"),
		)
	}
	if out != nil {
		_, _ = out.Write([]byte("⚠️  DEPRECATION NOTICE: 'zqk scheduler scan-tests' and disk test-bundles are deprecated. Use 'zqk test discover' and 'zqk test run' for universal test_case orchestration.\n"))
	}

	// Handle bundle listing
	if listBundles {
		return listSavedBundles(projectRoot, out)
	}

	// Handle delete all bundles
	if deleteAllBundles {
		return deleteAllSavedBundles(projectRoot, out)
	}

	// Handle initial bundle setup
	if setupBundles {
		return setupInitialBundles(projectRoot, maxBundleSize, overwrite, out)
	}

	onlyIndexes, err := parseIndexes(onlyIndexesStr)
	if err != nil {
		return errfmt.Newf("invalid --only indexes").Wrap(err)
	}
	skipIndexes, err := parseIndexes(skipIndexesStr)
	if err != nil {
		return errfmt.Newf("invalid --skip indexes").Wrap(err)
	}

	sourceRoot, err := resolveScanTestsSourceRoot(projectRoot, cmd)
	if err != nil {
		return err
	}

	st := &scanTestsPayload{
		cliCtx:          cliCtx,
		cmd:             cmd,
		ctx:             ctx,
		out:             out,
		projectRoot:     projectRoot,
		sourceRoot:      sourceRoot,
		storageProvider: storageProvider,
		secCtx:          secCtx,
		logger:          logger,
		packagePath:     packagePath,
		singleTest:      singleTest,
		multipleTests:   multipleTests,
		loadBundle:      loadBundle,
		loadBundlesStr:  loadBundlesStr,
		saveBundle:      saveBundle,
		onlyIndexes:     onlyIndexes,
		skipIndexes:     skipIndexes,
		allTests:        allTests,
		maxBundleSize:   maxBundleSize,
		maxParallel:     maxParallel,
		overwrite:       overwrite,
		merge:           merge,
		removeMissing:   removeMissing,
		showBundle:      showBundle,
	}
	return runScanTestsSchedulePipeline(st)
}

// resolveScanTestsSourceRoot picks the go test / scan module root.
// Default: .zqk/local-ci/workdir when it has go.mod (Local CI), else studio.
// --live-source forces studio; --source-root overrides the path.
func resolveScanTestsSourceRoot(studioRoot string, cmd *cobra.Command) (string, error) {
	liveSource, _ := cmd.Flags().GetBool("live-source")
	explicit, _ := cmd.Flags().GetString("source-root")
	if liveSource {
		return studioRoot, nil
	}
	if strings.TrimSpace(explicit) != emptyValue {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return emptyValue, errfmt.Errorf("source-root: %w", err)
		}
		if _, err := fileutil.Stat(filepath.Join(abs, "go.mod")); err != nil {
			return emptyValue, errfmt.Errorf("source-root %s: go.mod not found", abs)
		}
		return abs, nil
	}
	candidate := filepath.Join(studioRoot, ".zqk", "local-ci", "workdir")
	if st, err := fileutil.Stat(filepath.Join(candidate, "go.mod")); err == nil && !st.IsDir() {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			return emptyValue, err
		}
		return abs, nil
	}
	return studioRoot, nil
}

func scanSingleTest(scanner *testscan.Scanner, testName string) ([]*testscan.TestFunction, error) {
	allTests, err := scanner.Scan()
	if err != nil {
		return nil, err
	}

	var found []*testscan.TestFunction
	for _, test := range allTests {
		if test.Name == testName {
			found = append(found, test)
		}
	}

	return found, nil
}

func scanMultipleTests(scanner *testscan.Scanner, testNames []string) ([]*testscan.TestFunction, error) {
	allTests, err := scanner.Scan()
	if err != nil {
		return nil, err
	}

	nameMap := make(map[string]bool)
	for _, name := range testNames {
		nameMap[name] = true
	}

	var found []*testscan.TestFunction
	for _, test := range allTests {
		if nameMap[test.Name] {
			found = append(found, test)
		}
	}

	return found, nil
}

// parsePackageList splits a --package value by comma (same spirit as --tests).
// Trims whitespace; empty segments are skipped.
func parsePackageList(s string) []string {
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != emptyValue {
			out = append(out, p)
		}
	}
	return out
}

// dedupeTests removes duplicate tests (same package path + name), preserving order.
func dedupeTests(tests []*testscan.TestFunction) []*testscan.TestFunction {
	if len(tests) < 2 {
		return tests
	}
	seen := make(map[string]bool, len(tests))
	var out []*testscan.TestFunction
	for _, t := range tests {
		if t == nil {
			continue
		}
		key := t.PackagePath + "\x00" + t.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

func scanPackage(scanner *testscan.Scanner, packagePath string) ([]*testscan.TestFunction, error) {
	allTests, err := scanner.Scan()
	if err != nil {
		return nil, err
	}

	// Normalize package path (remove ./ prefix if present)
	normalizedPath := strings.TrimPrefix(packagePath, "./")

	var found []*testscan.TestFunction
	for _, test := range allTests {
		if test.PackagePath == normalizedPath || strings.HasSuffix(test.PackagePath, "/"+normalizedPath) {
			found = append(found, test)
		}
	}

	return found, nil
}
