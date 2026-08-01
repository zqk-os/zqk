package testscan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

// BundleStorage handles saving and loading test bundles
type BundleStorage struct {
	storageDir string
}

// NewBundleStorage creates a new bundle storage
func NewBundleStorage(projectRoot string) *BundleStorage {
	storageDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.TestBundlesDir)
	return &BundleStorage{
		storageDir: storageDir,
	}
}

// SaveBundle saves a test bundle to disk
func (bs *BundleStorage) SaveBundle(bundle *TestBundle, name string) error {
	return bs.SaveBundleWithOptions(bundle, name, SaveOptions{})
}

// SaveOptions controls how bundles are saved
type SaveOptions struct {
	// Overwrite forces replacement of existing bundle
	Overwrite bool
	// Merge merges new tests with existing bundle (adds new, keeps existing)
	Merge bool
	// RemoveMissing removes tests from bundle that are no longer in the new bundle
	RemoveMissing bool
}

// SaveBundleWithOptions saves a test bundle with options
func (bs *BundleStorage) SaveBundleWithOptions(bundle *TestBundle, name string, options SaveOptions) error {
	// Ensure directory exists
	if err := os.MkdirAll(bs.storageDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create bundle storage directory").Wrap(err)
	}

	filePath := filepath.Join(bs.storageDir, fmt.Sprintf("%s.json", name))

	// Check if bundle exists
	existingBundle, err := bs.LoadBundle(name)
	if err == nil && !options.Overwrite {
		// Bundle exists - handle merge or return error
		if options.Merge {
			// Merge new tests with existing
			bundle = bs.mergeBundles(existingBundle.Bundle, bundle, options.RemoveMissing)
		} else {
			return errfmt.Errorf("bundle %s already exists (use --overwrite to replace or --merge to update)", name)
		}
	}

	// Create bundle file with metadata
	bundleFile := &BundleFile{
		Name:        name,
		CreatedAt:   time.Now().UTC(),
		Bundle:      bundle,
		TestIndexes: bs.createTestIndexes(bundle),
	}

	// If merging and bundle existed, preserve original creation date
	if existingBundle != nil && options.Merge {
		bundleFile.CreatedAt = existingBundle.CreatedAt
		bundleFile.UpdatedAt = time.Now().UTC()
	}

	// Save as JSON
	data, err := json.MarshalIndent(bundleFile, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal bundle").Wrap(err)
	}

	//nolint:gosec // G306: 0600 is acceptable for bundle files (readable by all)
	if err := os.WriteFile(filePath, data, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write bundle file").Wrap(err)
	}

	return nil
}

// mergeBundles merges two bundles, adding new tests and optionally removing missing ones
func (bs *BundleStorage) mergeBundles(existing, newBundle *TestBundle, removeMissing bool) *TestBundle {
	if existing == nil {
		return newBundle
	}
	if newBundle == nil {
		return existing
	}

	// Create a map of existing tests by name+package for quick lookup
	existingMap := make(map[string]*TestFunction)
	for _, test := range existing.Tests {
		key := fmt.Sprintf("%s:%s", test.PackagePath, test.Name)
		existingMap[key] = test
	}

	// Build merged test list
	var mergedTests []*TestFunction
	var totalDuration time.Duration

	// Add all existing tests first (if not removing missing)
	if !removeMissing {
		mergedTests = append(mergedTests, existing.Tests...)
		totalDuration = existing.EstimatedDuration
	}

	// Add new tests (avoid duplicates)
	for _, test := range newBundle.Tests {
		key := fmt.Sprintf("%s:%s", test.PackagePath, test.Name)
		if _, exists := existingMap[key]; !exists {
			// New test - add it
			mergedTests = append(mergedTests, test)
			totalDuration += test.EstimatedDuration
		} else if removeMissing {
			// Test still exists - keep it
			mergedTests = append(mergedTests, test)
			totalDuration += test.EstimatedDuration
		}
	}

	var mergedCrit []string
	if len(newBundle.CriteriaRefs) > 0 {
		mergedCrit = append([]string(nil), newBundle.CriteriaRefs...)
	} else {
		mergedCrit = append([]string(nil), existing.CriteriaRefs...)
	}
	var mergedTC []string
	if len(newBundle.TestCaseRefs) > 0 {
		mergedTC = append([]string(nil), newBundle.TestCaseRefs...)
	} else {
		mergedTC = append([]string(nil), existing.TestCaseRefs...)
	}

	// Create merged bundle
	mergedBundle := &TestBundle{
		ID:                existing.ID,
		Tests:             mergedTests,
		IsParallel:        existing.IsParallel && newBundle.IsParallel, // Both must be parallel
		EstimatedDuration: totalDuration,
		PackagePath:       existing.PackagePath, // Keep original package path
		CriteriaRefs:      mergedCrit,
		TestCaseRefs:      mergedTC,
	}

	return mergedBundle
}

// LoadBundle loads a test bundle from disk
func (bs *BundleStorage) LoadBundle(name string) (*BundleFile, error) {
	filePath := filepath.Join(bs.storageDir, fmt.Sprintf("%s.json", name))

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read bundle file").Wrap(err)
	}

	var bundleFile BundleFile
	if err := json.Unmarshal(data, &bundleFile); err != nil {
		return nil, errfmt.Newf("failed to unmarshal bundle").Wrap(err)
	}

	return &bundleFile, nil
}

// ResolveBundleNames expands a bundle name into one or more concrete bundle file names.
// Use this for intuitive names like "storage-all" or "scheduler-all":
//   - If a bundle with the exact name exists (e.g. storage-all.json), returns [name].
//   - Otherwise, looks for name-0.json, name-1.json, ... and returns them in numeric order.
//   - Non-numeric suffixes (e.g. storage-id_generation) are ignored when expanding.
//
// Create "storage-all" by running:
//
//	zqk scheduler scan-tests --package ./pkg/storage --save-bundle storage-all
//
// Then run all storage bundles with: --load-bundles storage-all
func (bs *BundleStorage) ResolveBundleNames(name string) ([]string, error) {
	exactPath := filepath.Join(bs.storageDir, fmt.Sprintf("%s.json", name))
	if _, err := os.Stat(exactPath); err == nil {
		return []string{name}, nil
	}

	entries, err := os.ReadDir(bs.storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errfmt.Errorf("no bundle(s) found for %q (directory does not exist)", name)
		}
		return nil, errfmt.Newf("failed to list bundle directory").Wrap(err)
	}

	prefix := name + "-"
	var indexed []struct {
		i    int
		name string
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		base := entry.Name()[:len(entry.Name())-5]
		if !strings.HasPrefix(base, prefix) {
			continue
		}
		suffix := base[len(prefix):]
		n, err := strconv.Atoi(suffix)
		if err != nil {
			continue // skip non-numeric suffixes (e.g. pkg-storage-id_generation)
		}
		indexed = append(indexed, struct {
			i    int
			name string
		}{n, base})
	}
	if len(indexed) == 0 {
		return nil, errfmt.Errorf("no bundle(s) found for %q (expected %s-0.json, %s-1.json, ...)", name, name, name)
	}
	sort.Slice(indexed, func(i, j int) bool { return indexed[i].i < indexed[j].i })
	out := make([]string, len(indexed))
	for i, v := range indexed {
		out[i] = v.name
	}
	return out, nil
}

// ListBundles lists all saved bundles
func (bs *BundleStorage) ListBundles() ([]string, error) {
	if _, err := os.Stat(bs.storageDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(bs.storageDir)
	if err != nil {
		return nil, errfmt.Newf("failed to read bundle directory").Wrap(err)
	}

	var bundles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			name := entry.Name()[:len(entry.Name())-5] // Remove .json extension
			bundles = append(bundles, name)
		}
	}

	return bundles, nil
}

// DeleteBundle deletes a saved bundle
func (bs *BundleStorage) DeleteBundle(name string) error {
	filePath := filepath.Join(bs.storageDir, fmt.Sprintf("%s.json", name))
	return os.Remove(filePath)
}

// createTestIndexes creates index mapping for tests in a bundle
func (bs *BundleStorage) createTestIndexes(bundle *TestBundle) map[int]*TestFunction {
	indexes := make(map[int]*TestFunction)
	for i, test := range bundle.Tests {
		indexes[i] = test
	}
	return indexes
}

// BundleFile represents a saved test bundle with metadata
type BundleFile struct {
	Name        string                `json:"name"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at,omitempty"`
	Bundle      *TestBundle           `json:"bundle"`
	TestIndexes map[int]*TestFunction `json:"test_indexes"`
}

// FilterTests filters tests in a bundle based on indexes
func (bf *BundleFile) FilterTests(onlyIndexes, skipIndexes []int) *TestBundle {
	if bf.Bundle == nil {
		return nil
	}

	// Create index sets for fast lookup
	onlySet := make(map[int]bool)
	for _, idx := range onlyIndexes {
		onlySet[idx] = true
	}

	skipSet := make(map[int]bool)
	for _, idx := range skipIndexes {
		skipSet[idx] = true
	}

	// Filter tests
	var filteredTests []*TestFunction
	var totalDuration time.Duration

	for i, test := range bf.Bundle.Tests {
		// Skip if in skip list
		if skipSet[i] {
			continue
		}

		// If only list is specified, only include if in list
		if len(onlyIndexes) > 0 && !onlySet[i] {
			continue
		}

		filteredTests = append(filteredTests, test)
		totalDuration += test.EstimatedDuration
	}

	// Create filtered bundle
	filteredBundle := &TestBundle{
		ID:                bf.Bundle.ID,
		Tests:             filteredTests,
		IsParallel:        bf.Bundle.IsParallel,
		EstimatedDuration: totalDuration,
		PackagePath:       bf.Bundle.PackagePath,
		CriteriaRefs:      append([]string(nil), bf.Bundle.CriteriaRefs...),
		TestCaseRefs:      append([]string(nil), bf.Bundle.TestCaseRefs...),
	}

	return filteredBundle
}

// GetTestByIndex gets a test by its index
func (bf *BundleFile) GetTestByIndex(index int) (*TestFunction, error) {
	if bf.Bundle == nil || index < 0 || index >= len(bf.Bundle.Tests) {
		return nil, errfmt.Errorf("test index %d out of range (0-%d)", index, len(bf.Bundle.Tests)-1)
	}
	return bf.Bundle.Tests[index], nil
}

// ListTestsWithIndexes returns a list of tests with their indexes
func (bf *BundleFile) ListTestsWithIndexes() []TestWithIndex {
	if bf.Bundle == nil {
		return []TestWithIndex{}
	}

	var result []TestWithIndex
	for i, test := range bf.Bundle.Tests {
		result = append(result, TestWithIndex{
			Index: i,
			Test:  test,
		})
	}
	return result
}

// TestWithIndex represents a test with its index in the bundle
type TestWithIndex struct {
	Index int           `json:"index"`
	Test  *TestFunction `json:"test"`
}
