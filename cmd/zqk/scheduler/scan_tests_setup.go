package scheduler

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testscan"
)

const setupBundlesLockFileName = "scan-tests-setup-bundles.lock"

// setupInitialBundles sets up initial test bundles for the project
// Scans all tests and creates bundles organized by package.
// Uses a lock file under .zqk/lock to prevent duplicate concurrent runs.
func setupInitialBundles(projectRoot string, maxBundleSize int, overwrite bool, out io.Writer) error {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Info("Setting up initial test bundles for project").Log()

	lockPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LockDir, setupBundlesLockFileName)
	fileLock, err := storage.NewFileLock(lockPath)
	if err != nil {
		return errfmt.Newf("failed to create setup-bundles lock").Wrap(err)
	}
	defer func() { _ = fileLock.Close() }()

	acquired, err := fileLock.TryLock()
	if err != nil {
		return errfmt.Newf("failed to acquire setup-bundles lock").Wrap(err)
	}
	if !acquired {
		return errfmt.Errorf("another scan-tests --setup-bundles is already running; only one run is allowed (lock: %s). Wait for it to finish or remove the lock file if the other process exited abnormally", lockPath)
	}

	// Create scanner
	scanner := testscan.NewScanner(projectRoot)

	// Scan all tests
	allTests, err := scanner.Scan()
	if err != nil {
		return errfmt.Newf("failed to scan tests").Wrap(err)
	}

	if len(allTests) == 0 {
		return errfmt.Errorf("no tests found in project")
	}

	fmt.Fprintf(out, "\n📦 Setting up test bundles\n")
	fmt.Fprintf(out, "Found %d test(s) across the project\n\n", len(allTests))

	// Group tests by package path
	packageTests := make(map[string][]*testscan.TestFunction)
	for _, test := range allTests {
		packagePath := test.PackagePath
		if packagePath == emptyValue {
			packagePath = "root"
		}
		packageTests[packagePath] = append(packageTests[packagePath], test)
	}

	// Create bundle storage
	bundleStorage := testscan.NewBundleStorage(projectRoot)

	// Create bundles for each package
	saveOptions := testscan.SaveOptions{
		Overwrite: overwrite,
		Merge:     false, // Initial setup - don't merge
	}

	var savedBundles []string
	totalBundles := 0

	for packagePath, tests := range packageTests {
		// Create a safe bundle name from package path
		bundleName := sanitizeBundleName(packagePath)
		if bundleName == emptyValue {
			bundleName = "root"
		}

		// Bundle tests for this package
		bundles := scanner.BundleTests(tests, maxBundleSize)

		// Save each bundle
		for i, bundle := range bundles {
			finalName := bundleName
			if len(bundles) > 1 {
				finalName = fmt.Sprintf("%s-%d", bundleName, i)
			}

			// Check if bundle exists
			exists := false
			if _, err := bundleStorage.LoadBundle(finalName); err == nil {
				exists = true
			}

			if exists && !overwrite {
				fmt.Fprintf(out, "⏭️  Skipping %s (already exists, use --overwrite to replace)\n", finalName)
				continue
			}

			if err := bundleStorage.SaveBundleWithOptions(bundle, finalName, saveOptions); err != nil {
				return errfmt.Errorf("failed to save bundle %s: %w", finalName, err)
			}

			action := "Created"
			if exists {
				action = "Updated"
			}

			fmt.Fprintf(out, "✅ %s bundle: %s (%d tests, %s)\n",
				action, finalName, len(bundle.Tests), packagePath)
			savedBundles = append(savedBundles, finalName)
			totalBundles++
		}
	}

	// Create a summary bundle for all tests
	allBundles := scanner.BundleTests(allTests, maxBundleSize)
	summaryName := "all-tests"

	exists := false
	if _, err := bundleStorage.LoadBundle(summaryName); err == nil {
		exists = true
	}

	if !exists || overwrite {
		// Save first bundle as summary (or create a combined one)
		if len(allBundles) > 0 {
			if err := bundleStorage.SaveBundleWithOptions(allBundles[0], summaryName, saveOptions); err == nil {
				action := "Created"
				if exists {
					action = "Updated"
				}
				fmt.Fprintf(out, "✅ %s summary bundle: %s (%d tests)\n",
					action, summaryName, len(allBundles[0].Tests))
				savedBundles = append(savedBundles, summaryName)
				totalBundles++
			}
		}
	} else {
		fmt.Fprintf(out, "⏭️  Skipping %s (already exists, use --overwrite to replace)\n", summaryName)
	}

	fmt.Fprintf(out, "\n✨ Setup complete: %d bundle(s) created/updated\n", totalBundles)
	fmt.Fprintf(out, "\nBundles saved:\n")
	for _, name := range savedBundles {
		fmt.Fprintf(out, "  - %s\n", name)
	}
	fmt.Fprintf(out, "%s", paths.RewriteCanonicalCLIInvocations("\nView bundles: zqk scheduler scan-tests --list-bundles\n"))
	fmt.Fprintf(out, "%s", paths.RewriteCanonicalCLIInvocations("Load bundle: zqk scheduler scan-tests --load-bundle <name>\n"))

	return nil
}

// sanitizeBundleName creates a safe bundle name from a package path
func sanitizeBundleName(packagePath string) string {
	// Replace path separators and special characters
	name := strings.ReplaceAll(packagePath, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, " ", "-")

	// Remove leading/trailing dashes
	name = strings.Trim(name, "-")

	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}

	return name
}
