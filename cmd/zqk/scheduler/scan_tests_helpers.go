package scheduler

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/testscan"
)

// deleteAllSavedBundles deletes all saved test bundles from .zqk/test-bundles.
func deleteAllSavedBundles(projectRoot string, out io.Writer) error {
	bundleStorage := testscan.NewBundleStorage(projectRoot)
	names, err := bundleStorage.ListBundles()
	if err != nil {
		return errfmt.Newf("failed to list bundles").Wrap(err)
	}
	if len(names) == 0 {
		fmt.Fprintf(out, "No saved bundles to delete.\n")
		return nil
	}
	for _, name := range names {
		if err := bundleStorage.DeleteBundle(name); err != nil {
			return errfmt.Errorf("failed to delete bundle %s: %w", name, err)
		}
		fmt.Fprintf(out, "Deleted bundle: %s\n", name)
	}
	fmt.Fprintf(out, "Deleted %d bundle(s).\n", len(names))
	return nil
}

// parseIndexes parses a comma-separated list of indexes
func parseIndexes(indexesStr string) ([]int, error) {
	if indexesStr == emptyValue {
		return []int{}, nil
	}

	parts := strings.Split(indexesStr, ",")
	indexes := make([]int, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == emptyValue {
			continue
		}
		idx, err := strconv.Atoi(part)
		if err != nil {
			return nil, errfmt.Errorf("invalid index %q: %w", part, err)
		}
		indexes = append(indexes, idx)
	}

	return indexes, nil
}

// parseLoadBundlesList parses a comma-separated list of bundle names (trimmed, non-empty).
func parseLoadBundlesList(s string) []string {
	if s == emptyValue {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != emptyValue {
			out = append(out, p)
		}
	}
	return out
}

// listSavedBundles lists all saved bundles
func listSavedBundles(projectRoot string, out io.Writer) error {
	bundleStorage := testscan.NewBundleStorage(projectRoot)
	bundles, err := bundleStorage.ListBundles()
	if err != nil {
		return errfmt.Newf("failed to list bundles").Wrap(err)
	}

	if len(bundles) == 0 {
		fmt.Fprintf(out, "No saved bundles found.\n")
		return nil
	}

	fmt.Fprintf(out, "Saved bundles:\n")
	for _, name := range bundles {
		// Load bundle to show details
		bundleFile, err := bundleStorage.LoadBundle(name)
		if err != nil {
			fmt.Fprintf(out, "  - %s (error loading: %v)\n", name, err)
			continue
		}

		testCount := 0
		if bundleFile.Bundle != nil {
			testCount = len(bundleFile.Bundle.Tests)
		}

		fmt.Fprintf(out, "  - %s (%d tests, created: %s)\n",
			name, testCount, bundleFile.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}

// showBundleDetails shows detailed information about a bundle
func showBundleDetails(bundleFile *testscan.BundleFile, filteredBundle *testscan.TestBundle, out io.Writer) error {
	fmt.Fprintf(out, "\n📦 Bundle: %s\n", bundleFile.Name)
	fmt.Fprintf(out, "   Created: %s\n", bundleFile.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(out, "   Package: %s\n", bundleFile.Bundle.PackagePath)
	fmt.Fprintf(out, "   Parallel-safe: %v\n", bundleFile.Bundle.IsParallel)
	fmt.Fprintf(out, "   Total tests: %d\n", len(bundleFile.Bundle.Tests))

	if filteredBundle != nil && len(filteredBundle.Tests) != len(bundleFile.Bundle.Tests) {
		fmt.Fprintf(out, "   Filtered tests: %d\n", len(filteredBundle.Tests))
	}

	fmt.Fprintf(out, "\nTests (with indexes):\n")
	for i, test := range bundleFile.Bundle.Tests {
		status := "✓"
		if filteredBundle != nil {
			// Check if this test is in the filtered bundle
			found := false
			for _, ft := range filteredBundle.Tests {
				if ft.Name == test.Name && ft.PackagePath == test.PackagePath {
					found = true
					break
				}
			}
			if !found {
				status = "✗"
			}
		}

		parallel := "sequential"
		if test.IsParallel {
			parallel = "parallel"
		}

		fmt.Fprintf(out, "  [%d] %s %s (%s) [%s] - %v\n",
			i, status, test.Name, test.PackagePath, parallel, test.EstimatedDuration.Round(time.Second))
	}

	return nil
}
