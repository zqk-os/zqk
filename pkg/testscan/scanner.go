package testscan

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// TestFunction represents a discovered test function
type TestFunction struct {
	// Name is the test function name (e.g., "TestMyFeature")
	Name string

	// Package is the package name (e.g., "storage")
	Package string

	// PackagePath is the full package path (e.g., "pkg/storage")
	PackagePath string

	// FilePath is the path to the test file
	FilePath string

	// IsParallel indicates if the test calls t.Parallel()
	IsParallel bool

	// EstimatedDuration is the estimated duration based on timing data
	EstimatedDuration time.Duration

	// RunCount is the number of times this test has been run (from timing data)
	RunCount int
}

// TestBundle represents a group of tests that can run together
type TestBundle struct {
	// ID is a unique identifier for this bundle
	ID string

	// Tests are the test functions in this bundle
	Tests []*TestFunction

	// IsParallel indicates if all tests in this bundle are parallel-safe
	IsParallel bool

	// EstimatedDuration is the total estimated duration for this bundle
	EstimatedDuration time.Duration

	// PackagePath is the package path for this bundle
	PackagePath string

	// CriteriaRefs optionally lists process criteria (CRIT-*) satisfied when this bundle run passes (see scheduler
	// test-bundle metadata and criteria_verification_evidence events).
	CriteriaRefs []string `json:"criteria_refs,omitempty"`

	// TestCaseRefs optionally lists test_case objects (TEST-*) this bundle run evidences.
	TestCaseRefs []string `json:"test_case_refs,omitempty"`
}

// Scanner scans Go test files and identifies test functions
type Scanner struct {
	// ProjectRoot is the root directory of the project
	ProjectRoot string

	// ExcludeDirs are directories to exclude from scanning
	ExcludeDirs []string

	// TimingData provides access to test timing information
	TimingData *TimingDataAccessor
}

// TimingDataAccessor provides access to test timing data
type TimingDataAccessor struct {
	timings map[string]*testkit.TestTiming
}

// NewTimingDataAccessor creates a new timing data accessor
func NewTimingDataAccessor() *TimingDataAccessor {
	// Load timing data from file
	timings := make(map[string]*testkit.TestTiming)
	testkit.LoadTimingsIntoMap(timings)
	return &TimingDataAccessor{timings: timings}
}

// GetTiming returns timing data for a test
func (tda *TimingDataAccessor) GetTiming(testName, packageName string) *testkit.TestTiming {
	key := fmt.Sprintf("%s/%s", testName, packageName)
	return tda.timings[key]
}

// NewScanner creates a new test scanner.
// ProjectRoot is cleaned and symlink-resolved so filepath.Walk can descend into
// Local CI workdirs that are symlinks (trees/<sha>); Walk does not follow symlinks.
// TRACK: BLI-1785723654802038000-b14064bc
func NewScanner(projectRoot string) *Scanner {
	root := projectRoot
	if abs, err := filepath.Abs(projectRoot); err == nil {
		root = abs
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != "" {
		root = resolved
	}
	return &Scanner{
		ProjectRoot: root,
		ExcludeDirs: []string{
			".git",
			"vendor",
			"node_modules",
			paths.ProjectDataDir,
			"docs",
			"scripts",
			"tools",
		},
		TimingData: NewTimingDataAccessor(),
	}
}

// CacheEntry holds the cached parsed tests for a file
type CacheEntry struct {
	ModTime int64           `json:"mod_time"`
	Tests   []*TestFunction `json:"tests"`
}

// ScanCache holds the cache for all files
type ScanCache struct {
	Entries map[string]CacheEntry `json:"entries"`
}

func (s *Scanner) getCachePath() string {
	return filepath.Join(s.ProjectRoot, paths.ProjectDataDir, "testscan_cache.json")
}

func (s *Scanner) loadCache() *ScanCache {
	cache := &ScanCache{Entries: make(map[string]CacheEntry)}
	data, err := fileutil.ReadFile(s.getCachePath())
	if err == nil {
		_ = json.Unmarshal(data, cache)
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]CacheEntry)
	}
	return cache
}

func (s *Scanner) saveCache(cache *ScanCache) {
	dir := filepath.Dir(s.getCachePath())
	_ = fileutil.EnsureDir(dir)
	if data, err := json.Marshal(cache); err == nil {
		_ = fileutil.WriteStandardFile(s.getCachePath(), data)
	}
}

// Scan scans the project for test files and returns all test functions
func (s *Scanner) Scan() ([]*TestFunction, error) {
	var testFunctions []*TestFunction

	cache := s.loadCache()
	cacheUpdated := false

	err := filepath.Walk(s.ProjectRoot, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			baseName := filepath.Base(path)
			for _, excludeDir := range s.ExcludeDirs {
				if baseName == excludeDir {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		modTime := info.ModTime().UnixNano()
		relPath, _ := filepath.Rel(s.ProjectRoot, path)

		var tests []*TestFunction
		entry, hit := cache.Entries[relPath]
		if hit && entry.ModTime == modTime {
			tests = entry.Tests
			// Repopulate dynamic timing data that wasn't properly persisted or could have changed
			for _, t := range tests {
				if timing := s.TimingData.GetTiming(t.Name, t.Package); timing != nil {
					t.EstimatedDuration = timing.AverageDuration
					t.RunCount = timing.RunCount
				} else {
					t.EstimatedDuration = defaultEstimatedDurationForPackage(t.PackagePath)
				}
			}
		} else {
			parsedTests, err := s.parseTestFile(path)
			if err != nil {
				return nil
			}
			tests = parsedTests
			cache.Entries[relPath] = CacheEntry{ModTime: modTime, Tests: tests}
			cacheUpdated = true
		}

		testFunctions = append(testFunctions, tests...)
		return nil
	})

	if cacheUpdated {
		s.saveCache(cache)
	}

	return testFunctions, err
}

// parseTestFile parses a Go test file and extracts test functions
func (s *Scanner) parseTestFile(filePath string) ([]*TestFunction, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, errfmt.Errorf("failed to parse file %s: %w", filePath, err)
	}

	// Extract package name and path
	packageName := node.Name.Name
	packagePath := s.extractPackagePath(filePath)

	var testFunctions []*TestFunction

	// Walk the AST to find test functions
	ast.Inspect(node, func(n ast.Node) bool {
		if x, ok := n.(*ast.FuncDecl); ok {
			// Check if this is a test function
			if strings.HasPrefix(x.Name.Name, "Test") && x.Name.Name != "TestMain" {
				testFunc := &TestFunction{
					Name:        x.Name.Name,
					Package:     packageName,
					PackagePath: packagePath,
					FilePath:    filePath,
				}

				// Check if the function calls t.Parallel()
				testFunc.IsParallel = s.hasParallelCall(x)

				// Get timing data if available
				if timing := s.TimingData.GetTiming(testFunc.Name, packageName); timing != nil {
					testFunc.EstimatedDuration = timing.AverageDuration
					testFunc.RunCount = timing.RunCount
				} else {
					// Default estimate: use package-based default so heavy packages get realistic bundle timeouts
					testFunc.EstimatedDuration = defaultEstimatedDurationForPackage(testFunc.PackagePath)
				}

				testFunctions = append(testFunctions, testFunc)
			}
		}
		return true
	})

	return testFunctions, nil
}

// defaultEstimatedDurationForPackage returns a per-test default when no timing data exists.
// Heavy packages (many CLI or storage tests) get a higher default so bundle/job timeouts are realistic.
func defaultEstimatedDurationForPackage(packagePath string) time.Duration {
	switch {
	case strings.HasPrefix(packagePath, "cmd/zqk/object") || packagePath == "cmd/zqk/object":
		return 90 * time.Second
	case strings.HasPrefix(packagePath, "pkg/storage") || packagePath == "pkg/storage":
		return 90 * time.Second
	default:
		return 30 * time.Second
	}
}

// hasParallelCall checks if a function contains a call to t.Parallel()
func (s *Scanner) hasParallelCall(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		if found {
			return false
		}
		if x, ok := n.(*ast.CallExpr); ok {
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "t" {
					if sel.Sel.Name == "Parallel" {
						found = true
						return false
					}
				}
			}
		}
		return true
	})
	return found
}

// extractPackagePath extracts the package path from a file path
func (s *Scanner) extractPackagePath(filePath string) string {
	relPath, err := filepath.Rel(s.ProjectRoot, filePath)
	if err != nil {
		return ""
	}

	// Remove filename, get directory
	dir := filepath.Dir(relPath)
	return dir
}

// BundleTests groups tests into bundles based on parallel safety and package
func (s *Scanner) BundleTests(tests []*TestFunction, maxBundleSize int) []*TestBundle {
	if maxBundleSize <= 0 {
		maxBundleSize = 10 // Default bundle size
	}

	// Group tests by package and parallel safety
	packageGroups := make(map[string]map[bool][]*TestFunction)
	for _, test := range tests {
		key := test.PackagePath
		if packageGroups[key] == nil {
			packageGroups[key] = make(map[bool][]*TestFunction)
		}
		packageGroups[key][test.IsParallel] = append(packageGroups[key][test.IsParallel], test)
	}

	var bundles []*TestBundle
	bundleID := 0

	// Create bundles for each package/parallel combination
	for packagePath, parallelGroups := range packageGroups {
		for isParallel, testGroup := range parallelGroups {
			// Sort tests by estimated duration (longest first for better scheduling)
			sort.Slice(testGroup, func(i, j int) bool {
				return testGroup[i].EstimatedDuration > testGroup[j].EstimatedDuration
			})

			// Split into bundles if needed
			for i := 0; i < len(testGroup); i += maxBundleSize {
				end := i + maxBundleSize
				if end > len(testGroup) {
					end = len(testGroup)
				}

				bundle := &TestBundle{
					ID:          fmt.Sprintf("bundle-%d", bundleID),
					Tests:       testGroup[i:end],
					IsParallel:  isParallel,
					PackagePath: packagePath,
				}

				// Calculate total estimated duration
				for _, test := range bundle.Tests {
					bundle.EstimatedDuration += test.EstimatedDuration
				}

				bundles = append(bundles, bundle)
				bundleID++
			}
		}
	}

	return bundles
}

// ScheduleBundles creates an optimal schedule for test bundles based on runtime metrics
func (s *Scanner) ScheduleBundles(bundles []*TestBundle, maxParallel int) []*TestBundle {
	// Default to 4 parallel jobs if not specified
	effectiveMaxParallel := maxParallel
	if effectiveMaxParallel <= 0 {
		effectiveMaxParallel = 4
	}

	// Sort bundles by estimated duration (longest first)
	// This ensures we start long-running tests early
	sort.Slice(bundles, func(i, j int) bool {
		return bundles[i].EstimatedDuration > bundles[j].EstimatedDuration
	})

	// Separate parallel-safe and non-parallel bundles
	var parallelBundles []*TestBundle
	var sequentialBundles []*TestBundle

	for _, bundle := range bundles {
		if bundle.IsParallel {
			parallelBundles = append(parallelBundles, bundle)
		} else {
			sequentialBundles = append(sequentialBundles, bundle)
		}
	}

	// Schedule parallel bundles first (can run concurrently)
	// Then schedule sequential bundles (must run one at a time)
	scheduled := make([]*TestBundle, 0, len(bundles))
	scheduled = append(scheduled, parallelBundles...)
	scheduled = append(scheduled, sequentialBundles...)

	// Note: effectiveMaxParallel is calculated but not currently used in scheduling logic
	// This is intentional - the current implementation schedules all parallel bundles first,
	// then sequential bundles. Future enhancements may use effectiveMaxParallel to limit
	// concurrent execution.
	_ = effectiveMaxParallel // Suppress unused variable warning

	return scheduled
}

// SuggestedTimeoutForPackage returns the suggested timeout in seconds for running
// all tests in the given package (e.g. "go test ./cmd/zqk/object"). Uses the same
// logic as scheduler job timeout: sum of estimated durations (from timing data or
// package defaults), 1.5x buffer, package minimums, and cap at 1 hour.
// Package path can be with or without "./" prefix. Returns 0 and non-nil error on failure.
func SuggestedTimeoutForPackage(projectRoot, packagePath string) (seconds int, err error) {
	normalizedPath := strings.TrimPrefix(packagePath, "./")
	scanner := NewScanner(projectRoot)
	allTests, err := scanner.Scan()
	if err != nil {
		return 0, err
	}
	var total time.Duration
	hasHeavyAllKinds := false
	for _, t := range allTests {
		if t.PackagePath != normalizedPath && !strings.HasSuffix(t.PackagePath, "/"+normalizedPath) {
			continue
		}
		total += t.EstimatedDuration
		if t.Name == "TestAllKindsCRUD" || t.Name == "TestAllKindsPagination" {
			hasHeavyAllKinds = true
		}
	}
	timeout := int(total.Seconds() * 1.5)
	if timeout < 300 {
		timeout = 300
	}
	if minPkg := GetMinTimeoutSecondsForPackage(projectRoot, normalizedPath); minPkg > timeout {
		timeout = minPkg
	}
	if hasHeavyAllKinds && timeout < 3600 {
		timeout = 3600
	}
	if timeout > 3600 {
		timeout = 3600
	}
	return timeout, nil
}
