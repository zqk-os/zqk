package scheduler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testjobgen"
	"github.com/zqk-os/zqk/pkg/testscan"
	"github.com/spf13/cobra"
)

const (
	pipelineKindScanTests             = "scheduler.scan_tests"
	outcomeScanTestsEarlyExit         = "scan_tests_early_exit"
	outcomeScanTestsBundleCount       = "bundle_count"
	outcomeScanTestsJobCount          = "job_count"
	outcomeScanTestsLogDir            = "log_dir"
	outcomeScanTestsSkipJobGeneration = "skip_job_generation"
)

// errScanTestsEarlyExit is returned from resolve when --show-bundle printed details for a single loaded bundle and the command must exit before scheduling (same behavior as the pre-pipeline code path).
var errScanTestsEarlyExit = errfmt.Errorf("scan-tests: show-bundle details complete")

// scanTestsPayload carries scan-tests state through pipeline stages.
type scanTestsPayload struct {
	cliCtx *cli.Context
	cmd    *cobra.Command
	ctx    context.Context
	out    io.Writer

	projectRoot     string // studio root: jobs, logs, bundle storage, concurrency limits
	sourceRoot      string // go test / scan module root (Local CI workdir when set)
	storageProvider storage.ObjectStorageProvider
	secCtx          *pkgctx.SecurityContext
	logger          logging.Logger

	packagePath    string
	singleTest     string
	multipleTests  string
	loadBundle     string
	loadBundlesStr string
	saveBundle     string
	onlyIndexes    []int
	skipIndexes    []int
	allTests       bool
	maxBundleSize  int
	maxParallel    int
	overwrite      bool
	merge          bool
	removeMissing  bool
	showBundle     bool

	scheduledBundles []*testscan.TestBundle
	bundleStorage    *testscan.BundleStorage

	skipJobGeneration bool
	jobIDs            []string
}

func (st *scanTestsPayload) resolveScheduledBundles() error {
	loadBundleNames := parseLoadBundlesList(st.loadBundlesStr)
	if len(loadBundleNames) > 0 {
		st.bundleStorage = testscan.NewBundleStorage(st.projectRoot)
		scanner := testscan.NewScanner(st.sourceRoot)
		var allBundles []*testscan.TestBundle
		for _, name := range loadBundleNames {
			resolved, err := st.bundleStorage.ResolveBundleNames(name)
			if err != nil {
				return errfmt.Errorf("failed to resolve bundle %s: %w", name, err)
			}
			for _, resolvedName := range resolved {
				bundleFile, err := st.bundleStorage.LoadBundle(resolvedName)
				if err != nil {
					return errfmt.Errorf("failed to load bundle %s: %w", resolvedName, err)
				}
				filtered := bundleFile.FilterTests(st.onlyIndexes, st.skipIndexes)
				if filtered != nil && len(filtered.Tests) > 0 {
					filtered.ID = resolvedName
					allBundles = append(allBundles, filtered)
				}
			}
		}
		if len(allBundles) == 0 {
			return errfmt.Errorf("no tests remaining after loading %d bundle(s)", len(loadBundleNames))
		}
		st.scheduledBundles = scanner.ScheduleBundles(allBundles, st.maxParallel)
		cli.TouchMeaningfulActivity()
		return nil
	}

	if st.loadBundle != emptyValue {
		st.bundleStorage = testscan.NewBundleStorage(st.projectRoot)
		resolved, err := st.bundleStorage.ResolveBundleNames(st.loadBundle)
		if err != nil {
			return errfmt.Errorf("failed to resolve bundle %s: %w", st.loadBundle, err)
		}
		var bundlesToSchedule []*testscan.TestBundle
		var singleBundleFile *testscan.BundleFile
		var singleFiltered *testscan.TestBundle
		for _, resolvedName := range resolved {
			bundleFile, err := st.bundleStorage.LoadBundle(resolvedName)
			if err != nil {
				return errfmt.Errorf("failed to load bundle %s: %w", resolvedName, err)
			}
			filtered := bundleFile.FilterTests(st.onlyIndexes, st.skipIndexes)
			if filtered != nil && len(filtered.Tests) > 0 {
				filtered.ID = resolvedName
				bundlesToSchedule = append(bundlesToSchedule, filtered)
				if len(resolved) == 1 {
					singleBundleFile = bundleFile
					singleFiltered = filtered
				}
			}
		}
		if len(bundlesToSchedule) == 0 {
			return errfmt.Errorf("no tests remaining after filtering")
		}

		if st.showBundle && singleBundleFile != nil && singleFiltered != nil {
			if err := showBundleDetails(singleBundleFile, singleFiltered, st.out); err != nil {
				return err
			}
			return errScanTestsEarlyExit
		}

		scanner := testscan.NewScanner(st.sourceRoot)
		st.scheduledBundles = scanner.ScheduleBundles(bundlesToSchedule, st.maxParallel)
		cli.TouchMeaningfulActivity()
		return nil
	}

	// Normal scanning mode (packages under source root — Local CI workdir when set)
	scanner := testscan.NewScanner(st.sourceRoot)
	var tests []*testscan.TestFunction

	switch {
	case st.singleTest != emptyValue:
		schedpkg.SLog(st.logger).Info("Scanning for single test").
			TestName(st.singleTest).
			Log()
		scannedTests, err := scanSingleTest(scanner, st.singleTest)
		if err != nil {
			return errfmt.Errorf("failed to scan test %s: %w", st.singleTest, err)
		}
		if len(scannedTests) == 0 {
			return errfmt.Errorf("test %s not found", st.singleTest)
		}
		tests = scannedTests

	case st.multipleTests != emptyValue:
		testNames := strings.Split(st.multipleTests, ",")
		for i := range testNames {
			testNames[i] = strings.TrimSpace(testNames[i])
		}
		schedpkg.SLog(st.logger).Info("Scanning for multiple tests").
			Count(len(testNames)).
			Log()
		scannedTests, err := scanMultipleTests(scanner, testNames)
		if err != nil {
			return errfmt.Newf("failed to scan tests").Wrap(err)
		}
		if len(scannedTests) == 0 {
			return errfmt.Errorf("none of the specified tests were found")
		}
		tests = scannedTests

	case st.packagePath != emptyValue:
		pkgs := parsePackageList(st.packagePath)
		if len(pkgs) == 0 {
			return errfmt.Errorf("no package paths in --package")
		}
		for _, pkg := range pkgs {
			schedpkg.SLog(st.logger).Info("Scanning package").
				PackageName(pkg).
				Log()
			scannedTests, err := scanPackage(scanner, pkg)
			if err != nil {
				return errfmt.Errorf("failed to scan package %s: %w", pkg, err)
			}
			tests = append(tests, scannedTests...)
		}
		tests = dedupeTests(tests)
		if len(tests) == 0 {
			return errfmt.Errorf("no tests found in package(s) %s", strings.Join(pkgs, ", "))
		}

	case st.allTests:
		logging.Fluent(st.logger).Info("Scanning all tests in project").Log()
		if err := cli.WriteOutput(st.cmd, []byte("Scanning all test files (this may take a few minutes)...\n")); err != nil {
			return err
		}
		scannedTests, err := scanner.Scan()
		if err != nil {
			return errfmt.Newf("failed to scan project").Wrap(err)
		}
		if len(scannedTests) == 0 {
			return errfmt.Errorf("no tests found in project")
		}
		tests = scannedTests

	default:
		return errfmt.Errorf("must specify one of: --test, --tests, --package, or --all")
	}

	schedpkg.SLog(st.logger).Info("Tests found").
		Count(len(tests)).
		Log()

	var listBuf strings.Builder
	fmt.Fprintf(&listBuf, "\nFound %d test(s):\n", len(tests))
	for _, test := range tests {
		parallel := "sequential"
		if test.IsParallel {
			parallel = "parallel"
		}
		fmt.Fprintf(&listBuf, "  - %s (%s) [%s] - %s\n",
			test.Name, test.PackagePath, parallel, test.EstimatedDuration.Round(time.Second))
	}
	if err := cli.WriteOutput(st.cmd, []byte(listBuf.String())); err != nil {
		return err
	}

	bundles := scanner.BundleTests(tests, st.maxBundleSize)
	schedpkg.SLog(st.logger).Info("Tests bundled").
		Int("bundle_count", len(bundles)).
		Log()

	if st.saveBundle != emptyValue {
		st.bundleStorage = testscan.NewBundleStorage(st.projectRoot)
		saveOptions := testscan.SaveOptions{
			Overwrite:     st.overwrite,
			Merge:         st.merge,
			RemoveMissing: st.removeMissing,
		}
		for i := range bundles {
			bundleName := st.saveBundle
			if len(bundles) > 1 {
				bundleName = fmt.Sprintf("%s-%d", st.saveBundle, i)
			}
			// Align bundle.ID with the on-disk name so GenerateJobs uses stable SCH-run-<bundleName>
			// (without this, IDs stay bundle-0..N and collide across separate scan-tests invocations).
			bundles[i].ID = bundleName
			if err := st.bundleStorage.SaveBundleWithOptions(bundles[i], bundleName, saveOptions); err != nil {
				return errfmt.Errorf("failed to save bundle %s: %w", bundleName, err)
			}
			if err := cli.WriteOutput(st.cmd, []byte(fmt.Sprintf("💾 Saved bundle: %s\n", bundleName))); err != nil {
				return err
			}
		}
	}

	st.scheduledBundles = scanner.ScheduleBundles(bundles, st.maxParallel)
	cli.TouchMeaningfulActivity()
	return nil
}

func scanTestsStageIngest(pctx *pipeline.Context, payload any) (any, error) {
	st := payload.(*scanTestsPayload)
	err := st.resolveScheduledBundles()
	if err != nil {
		if errors.Is(err, errScanTestsEarlyExit) {
			pctx.Outcome[outcomeScanTestsEarlyExit] = true
			return st, nil
		}
		return nil, err
	}
	return st, nil
}

func scanTestsStageNormalize(pctx *pipeline.Context, payload any) (any, error) {
	st := payload.(*scanTestsPayload)
	if pctx.Outcome[outcomeScanTestsEarlyExit] == true {
		return st, nil
	}

	var normBuf strings.Builder
	fmt.Fprintf(&normBuf, "\nCreated %d bundle(s):\n", len(st.scheduledBundles))
	for i, bundle := range st.scheduledBundles {
		parallel := "sequential"
		if bundle.IsParallel {
			parallel = "parallel"
		}
		fmt.Fprintf(&normBuf, "  Bundle %d: %d test(s) [%s] - %s\n",
			i+1, len(bundle.Tests), parallel, bundle.EstimatedDuration.Round(time.Second))
	}
	if err := cli.WriteOutput(st.cmd, []byte(normBuf.String())); err != nil {
		return nil, err
	}

	if st.showBundle {
		st.skipJobGeneration = true
		pctx.Outcome[outcomeScanTestsSkipJobGeneration] = true
	}
	pctx.Outcome[outcomeScanTestsBundleCount] = len(st.scheduledBundles)
	return st, nil
}

func scanTestsStageCommit(pctx *pipeline.Context, payload any) (any, error) {
	st := payload.(*scanTestsPayload)
	if pctx.Outcome[outcomeScanTestsEarlyExit] == true {
		return st, nil
	}
	if st.skipJobGeneration {
		return st, nil
	}

	if err := testscan.WritePackageConcurrencyLimitsPatch(st.projectRoot, st.scheduledBundles, st.maxParallel); err != nil {
		return nil, errfmt.Newf("write package concurrency limits").Wrap(err)
	}

	generator := testjobgen.NewJobGenerator(st.storageProvider, st.secCtx)
	jobIDs, err := generator.GenerateJobs(st.ctx, st.scheduledBundles, st.projectRoot, st.sourceRoot, st.maxParallel)
	if err != nil {
		return nil, errfmt.Newf("failed to generate jobs").Wrap(err)
	}
	st.jobIDs = jobIDs
	cli.TouchMeaningfulActivity()
	pctx.Outcome[outcomeScanTestsJobCount] = len(jobIDs)
	logDir := schedpkg.JobLogsTestBundlesDir(st.projectRoot)
	pctx.Outcome[outcomeScanTestsLogDir] = logDir
	return st, nil
}

func scanTestsStageFinalize(pctx *pipeline.Context, payload any) (any, error) {
	st := payload.(*scanTestsPayload)

	if pctx.Outcome[outcomeScanTestsEarlyExit] == true {
		return st, nil
	}

	if st.skipJobGeneration {
		if err := cli.WriteOutput(st.cmd, []byte("\n(Show-bundle mode: not creating scheduler jobs. Omit --show-bundle to schedule.)\n")); err != nil {
			return nil, err
		}
		return st, nil
	}

	var finBuf strings.Builder
	fmt.Fprintf(&finBuf, "\n✅ Created %d scheduler job(s) and enqueued for immediate execution.\n", len(st.jobIDs))
	fmt.Fprintf(&finBuf, "   The scheduler daemon will run them when it processes the trigger queue (ensure it is running: zqk scheduler start).\n")
	if st.sourceRoot != emptyValue && st.sourceRoot != st.projectRoot {
		fmt.Fprintf(&finBuf, "   Source root (go test cwd): %s\n", st.sourceRoot)
		fmt.Fprintf(&finBuf, "   Tip: use --live-source to test studio HEAD instead of Local CI workdir.\n")
	} else {
		fmt.Fprintf(&finBuf, "   Source root (go test cwd): %s\n", st.sourceRoot)
	}
	logDir, _ := pctx.Outcome[outcomeScanTestsLogDir].(string)
	if logDir == emptyValue {
		logDir = schedpkg.JobLogsTestBundlesDir(st.projectRoot)
	}
	for i, jobID := range st.jobIDs {
		bundle := st.scheduledBundles[i]
		timestamp := time.Now().UTC().Unix()
		logFileName := fmt.Sprintf("bundle-%s-%d.log", bundle.ID, timestamp)
		logFilePath := filepath.Join(logDir, logFileName)
		fmt.Fprintf(&finBuf, "  - %s\n", jobID)
		fmt.Fprintf(&finBuf, "    Log: %s\n", logFilePath)
	}
	fmt.Fprintf(&finBuf, "\nDo not assume jobs started: check Executing via zqk scheduler activity (daemon snapshot),\n")
	fmt.Fprintf(&finBuf, "or last_run_at on the SCH-run-* object. If stuck with empty last_run_at, look for\n")
	fmt.Fprintf(&finBuf, "package_concurrency_acquire_failed in .zqk/scheduler/diagnostics.jsonl / dispatch_pressure.jsonl\n")
	fmt.Fprintf(&finBuf, "(one go-test package can hold the only concurrency slot for up to ~30m).\n")
	fmt.Fprintf(&finBuf, "\nView status: zqk scheduler activity\n")
	fmt.Fprintf(&finBuf, "View history: zqk scheduler history\n")
	fmt.Fprintf(&finBuf, "View logs: ls -lh %s\n", logDir)

	if err := cli.WriteOutput(st.cmd, []byte(finBuf.String())); err != nil {
		return nil, err
	}

	return st, nil
}

func runScanTestsSchedulePipeline(st *scanTestsPayload) error {
	logger := st.logger
	pl := pipeline.NewBuilder(pipelineKindScanTests, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, scanTestsStageIngest).
		AddStage(pipeline.StageNormalize, scanTestsStageNormalize).
		AddStage(pipeline.StageCommit, scanTestsStageCommit).
		AddStage(pipeline.StageFinalize, scanTestsStageFinalize).
		Build()

	pctx := &pipeline.Context{Ctx: st.ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, st)
	return err
}
