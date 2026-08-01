package system

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Compiled once at package load to avoid per-call cost.
var (
	hashFilePattern = regexp.MustCompile(`^[a-f0-9]{64}\.yaml$`)
	// idLineRegex matches "id: value" at start of line (YAML); value may be quoted or unquoted.
	idLineRegex = regexp.MustCompile(`(?m)^\s*id\s*:\s*(.+)$`)
)

// KindProgressFunc is an optional callback invoked when a kind completes during hash-duplicates cleanup.
// completedIndex is 1-based; total is the number of kinds being cleaned.
type KindProgressFunc func(kind string, completedIndex, total int)

// CleanupDuplicatesContext groups state for cleanup operation
type CleanupDuplicatesContext struct {
	ProjectRoot                  string
	ProcessDir                   string
	Kinds                        []string
	DryRun                       bool
	Verbose                      bool
	HashDuplicates               bool
	DeleteHashDuplicates         bool
	DeleteHashDuplicatesForKinds map[string]bool // when set, delete (not quarantine) for these kinds (e.g. system-generated)
	QuarantineDir                string
	Logger                       logging.Logger
	// OnKindProgress, when set, is called from the collector as each kind completes (optional progress for UI).
	OnKindProgress KindProgressFunc
}

// initializeCleanupContext sets up the cleanup context
func initializeCleanupContext(cmd *cobra.Command, args []string) (*CleanupDuplicatesContext, error) {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root not found")
	}

	processDir := datacell.ProcessPrimaryDir(projectRoot)
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")
	hashDupes, _ := cmd.Flags().GetBool("hash-duplicates")
	deleteHashDupes, _ := cmd.Flags().GetBool("delete-hash-duplicates")
	quarantineDir, _ := cmd.Flags().GetString("quarantine-dir")
	if quarantineDir == emptyValue {
		quarantineDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, paths.QuarantineDir, "hash-duplicates")
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	var kinds []string
	if len(args) > 0 {
		if k, ok := cli.KindCanonicalFromPRERun(cli.KindAnnotKeysSystem, cmd); ok {
			kinds = []string{k}
		} else {
			k, err := objects.ResolveAndValidateKindForProject(projectRoot, args[0])
			if err != nil {
				return nil, err
			}
			kinds = []string{k}
		}
	} else {
		kinds = discoverObjectKinds(processDir)
	}

	// Hash-duplicates are always deleted (no quarantine). deleteForKindsMap kept for API/compat.
	var deleteForKindsMap map[string]bool
	if hashDupes {
		if cfg := storage.GetGlobalBlockingCheckConfig(); cfg != nil {
			if bypass := cfg.GetBypassKinds(); len(bypass) > 0 {
				deleteForKindsMap = make(map[string]bool, len(bypass))
				for _, k := range bypass {
					deleteForKindsMap[k] = true
				}
			}
		}
	}

	return &CleanupDuplicatesContext{
		ProjectRoot:                  projectRoot,
		ProcessDir:                   processDir,
		Kinds:                        kinds,
		DryRun:                       dryRun,
		Verbose:                      verbose,
		HashDuplicates:               hashDupes,
		DeleteHashDuplicates:         deleteHashDupes,
		DeleteHashDuplicatesForKinds: deleteForKindsMap,
		QuarantineDir:                quarantineDir,
		Logger:                       logger,
	}, nil
}

// KindCleanupResult contains results for a single kind
type KindCleanupResult struct {
	Kind    string
	Deleted int
	Skipped int
	Errors  []error
}

// primaryFromCASIndex builds objectID -> one hash file path from the CAS index (no disk read of hash files).
// Returns (primary, nil) when the index has entries; (nil, nil) when index is empty so caller can fall back to scanHashFiles.
// When projectRoot is non-empty, uses cached CAS to avoid loading the full index per kind (OOM risk).
func primaryFromCASIndex(ctx context.Context, dirPath, kind, projectRoot string) (primary map[string]string, _ error) {
	var cas *storage.ContentAddressableStorage
	if projectRoot != emptyValue {
		if c, ok := getCachedCASForKind(ctx, projectRoot, kind); ok {
			cas = c
		}
	}
	if cas == nil {
		cas = storage.NewContentAddressableStorage(dirPath, kind)
	}
	ids, err := cas.ListIDs()
	if err != nil || len(ids) == 0 {
		return nil, nil
	}
	mappings, err := cas.GetAllMappings()
	if err != nil || len(mappings) == 0 {
		return nil, nil
	}
	primary = make(map[string]string, len(ids))
	idx := cas.GetIndex()
	for _, objectID := range ids {
		hash, ok := mappings[objectID]
		if !ok || hash == emptyValue {
			continue
		}
		primary[objectID] = buildHashFilePath(dirPath, hash, idx.GetBucketKey(objectID))
	}
	return primary, nil
}

// buildHashFilePath returns the filesystem path for a hash file (kindDir or kindDir/bucketKey).
func buildHashFilePath(dirPath, hash, bucketKey string) string {
	if bucketKey != emptyValue {
		return filepath.Join(dirPath, bucketKey, hash+".yaml")
	}
	return filepath.Join(dirPath, hash+".yaml")
}

// isTraditionalYAMLFile returns true if name looks like a traditional YAML object file (not a hash-named CAS file).
func isTraditionalYAMLFile(name string, isDir bool) bool {
	if isDir {
		return false
	}
	if hashFilePattern.MatchString(name) || strings.HasPrefix(name, ".") {
		return false
	}
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

// extractObjectIDFromYAMLHead reads up to maxBytes from the file and returns the first "id:" value.
// Avoids full file read and full YAML parse when id is near the top (common case).
func extractObjectIDFromYAMLHead(filePath string, maxBytes int) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var buf []byte
	for len(buf) < maxBytes {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			buf = append(buf, line...)
		}
		if err != nil {
			break
		}
	}
	if len(buf) > maxBytes {
		buf = buf[:maxBytes]
	}
	matches := idLineRegex.FindSubmatch(buf)
	if len(matches) < 2 {
		return "", nil
	}
	val := strings.TrimSpace(string(matches[1]))
	val = strings.Trim(val, `"'`)
	return val, nil
}

const idHeadBytes = 4096

// objectIDFromHashFile reads a hash file and returns its object ID (lightweight head-first, then full parse).
func objectIDFromHashFile(path string, verbose bool, logger logging.Logger) (objectID string, err error) {
	objectID, err = extractObjectIDFromYAMLHead(path, idHeadBytes)
	if err != nil {
		return "", err
	}
	if objectID != emptyValue {
		return objectID, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if verbose {
			logging.Fluent(logger).Warn("Failed to read hash file").File(path).WithError(err).Log()
		}
		return "", err
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		if verbose {
			logging.Fluent(logger).Warn("Failed to parse hash file").File(path).WithError(err).Log()
		}
		return "", err
	}
	id, _ := obj[objects.FieldKeyID].(string)
	return id, nil
}

// scanHashFiles scans a directory recursively for hash-based files and extracts object IDs.
// Returns primary (one path per object ID) and duplicates (object IDs with multiple hash files).
// Handles bucketed storage. Uses lightweight ID extraction when possible.
func scanHashFiles(dirPath string, verbose bool, logger logging.Logger) (primary map[string]string, duplicates map[string][]string) {
	primary = make(map[string]string)
	duplicates = make(map[string][]string)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !hashFilePattern.MatchString(info.Name()) {
			return nil
		}

		objectID, readErr := objectIDFromHashFile(path, verbose, logger)
		if readErr != nil || objectID == emptyValue {
			if objectID == emptyValue && verbose {
				logging.Fluent(logger).Debug("Hash file missing object ID").File(path).Log()
			}
			return nil
		}

		if _, exists := primary[objectID]; !exists {
			primary[objectID] = path
		} else {
			if len(duplicates[objectID]) == 0 {
				duplicates[objectID] = append(duplicates[objectID], primary[objectID])
			}
			duplicates[objectID] = append(duplicates[objectID], path)
		}
		return nil
	})

	if err != nil {
		logging.Fluent(logger).Warn("Error walking directory").Dir(dirPath).WithError(err).Log()
	}
	return primary, duplicates
}

// quarantineOrDeleteDuplicateHashFile removes a hash-duplicate file. We always delete (no quarantine);
// quarantine was unused for recovery and only added bloat.
func quarantineOrDeleteDuplicateHashFile(ctx *CleanupDuplicatesContext, kind, objectID, filePath string) (bool, error) {
	if ctx.DryRun {
		logging.Fluent(ctx.Logger).Info("Would delete hash-duplicate").File(filePath).Kind(kind).ObjectID(objectID).Log()
		return true, nil
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

// reconcileCASIndexMapping updates the CAS index to point objectID at the keeper hash file.
// Caller must pass the same cas instance used for this kind to avoid creating one per object.
func reconcileCASIndexMapping(cas *storage.ContentAddressableStorage, kind, objectID, keepFilePath string) error {
	base := filepath.Base(keepFilePath)
	if !strings.HasSuffix(base, ".yaml") || len(base) != 69 {
		return errfmt.Errorf("cannot extract hash from filename: %s", base)
	}
	hash := base[:64]

	writeQueue := storage.GetGlobalListingIndexWriteQueue()
	done, err := writeQueue.EnqueueUpdateWithCallback(kind, objectID, hash, cas)
	if err != nil {
		return err
	}
	if updateErr := <-done; updateErr != nil {
		return updateErr
	}
	_ = storage.FlushListingIndexForKind(kind)
	return nil
}

func cleanupHashDuplicatesForKind(ctx *CleanupDuplicatesContext, kind, kindDir string, duplicates map[string][]string) (deleted int, skipped int, errors []error) {
	if !ctx.HashDuplicates || len(duplicates) == 0 {
		return 0, 0, nil
	}

	var cas *storage.ContentAddressableStorage
	if ctx.ProjectRoot != emptyValue {
		if c, ok := getCachedCASForKind(context.Background(), ctx.ProjectRoot, kind); ok {
			cas = c
		}
	}
	if cas == nil {
		cas = storage.NewContentAddressableStorage(kindDir, kind)
	}

	for objectID, paths := range duplicates {
		if len(paths) <= 1 {
			continue
		}

		// Choose keeper: newest mtime wins. Skip paths that no longer exist (index/cache stale or already deleted).
		keep := ""
		var keepMTime time.Time
		for _, p := range paths {
			fi, err := os.Stat(p)
			if err != nil {
				if os.IsNotExist(err) {
					continue // file already gone; no need to report
				}
				errors = append(errors, err)
				continue
			}
			if keep == emptyValue || fi.ModTime().After(keepMTime) {
				keepMTime = fi.ModTime()
				keep = p
			}
		}
		if keep == emptyValue {
			continue // all paths missing (e.g. object deleted); nothing to reconcile
		}

		if err := reconcileCASIndexMapping(cas, kind, objectID, keep); err != nil {
			errors = append(errors, errfmt.Errorf("failed to reconcile CAS index for %s/%s: %w", kind, objectID, err))
		}

		for _, p := range paths {
			if p == keep {
				continue
			}
			ok, err := quarantineOrDeleteDuplicateHashFile(ctx, kind, objectID, p)
			if err != nil {
				errors = append(errors, err)
				skipped++
				continue
			}
			if ok {
				deleted++
			}
		}
	}

	return deleted, skipped, errors
}

// readAndParseTraditionalFile reads and parses a traditional file (full read + YAML parse).
func readAndParseTraditionalFile(filePath string, ctx *CleanupDuplicatesContext) (map[string]any, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if ctx.Verbose {
			logging.Fluent(ctx.Logger).Warn("Failed to read traditional file").File(filePath).WithError(err).Log()
		}
		return nil, err
	}

	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		if ctx.Verbose {
			logging.Fluent(ctx.Logger).Warn("Failed to parse traditional file").File(filePath).WithError(err).Log()
		}
		return nil, err
	}

	return obj, nil
}

// handleDuplicateFile handles deletion of a duplicate file
func handleDuplicateFile(filePath string, hashFilePath string, ctx *CleanupDuplicatesContext) (deleted bool, err error) {
	if ctx.DryRun {
		logging.Fluent(ctx.Logger).Info("Would delete duplicate").File(filePath).HashFile(filepath.Base(hashFilePath)).Log()
		return true, nil
	}

	if err := os.Remove(filePath); err != nil {
		logging.Fluent(ctx.Logger).Error("Failed to delete duplicate file", err).File(filePath).Log()
		return false, err
	}

	if ctx.Verbose {
		logging.Fluent(ctx.Logger).Info("Deleted duplicate").File(filePath).HashFile(filepath.Base(hashFilePath)).Log()
	}
	return true, nil
}

// processTraditionalFile processes a single traditional file for duplicate detection.
// Uses lightweight ID extraction (first 4KB) when possible to avoid full read and YAML parse.
func processTraditionalFile(
	filePath string,
	hashFileIDs map[string]string,
	ctx *CleanupDuplicatesContext,
) (deleted bool, skipped bool, err error) {
	objectID, extractErr := extractObjectIDFromYAMLHead(filePath, idHeadBytes)
	if extractErr != nil {
		return false, true, extractErr
	}
	if objectID == emptyValue {
		// id not in first 4KB; fall back to full read/parse
		obj, parseErr := readAndParseTraditionalFile(filePath, ctx)
		if parseErr != nil {
			return false, true, parseErr
		}
		var ok bool
		objectID, ok = obj[objects.FieldKeyID].(string)
		if !ok || objectID == emptyValue {
			return false, false, nil
		}
	}

	hashFilePath, exists := hashFileIDs[objectID]
	if !exists {
		return false, false, nil
	}

	wasDeleted, err := handleDuplicateFile(filePath, hashFilePath, ctx)
	if err != nil {
		return false, true, err
	}
	return wasDeleted, false, nil
}

// scanTraditionalFiles walks dirPath and removes traditional YAML files that duplicate hash-based objects.
func scanTraditionalFiles(dirPath string, hashFileIDs map[string]string, ctx *CleanupDuplicatesContext) (deleted, skipped int, errors []error) {
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !isTraditionalYAMLFile(info.Name(), info.IsDir()) {
			return nil
		}

		wasDeleted, wasSkipped, fileErr := processTraditionalFile(path, hashFileIDs, ctx)
		if fileErr != nil {
			errors = append(errors, fileErr)
		}
		if wasSkipped {
			skipped++
		} else if wasDeleted {
			deleted++
		}
		return nil
	})

	if err != nil {
		errors = append(errors, err)
	}
	return deleted, skipped, errors
}

// cleanupKind processes a single kind for duplicate cleanup
func cleanupKind(ctx *CleanupDuplicatesContext, kind string) KindCleanupResult {
	kindDir := objects.GetDirectoryFromKind(kind)
	if kindDir == emptyValue {
		return KindCleanupResult{Kind: kind}
	}

	dirPath := filepath.Join(ctx.ProcessDir, kindDir)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return KindCleanupResult{Kind: kind}
	}

	var hashFileIDs map[string]string
	var hashDuplicates map[string][]string

	if !ctx.HashDuplicates {
		if primary, _ := primaryFromCASIndex(context.Background(), dirPath, kind, ctx.ProjectRoot); len(primary) > 0 {
			hashFileIDs = primary
			hashDuplicates = nil
		}
	}
	if hashFileIDs == nil {
		hashFileIDs, hashDuplicates = scanHashFiles(dirPath, ctx.Verbose, ctx.Logger)
	}

	deleted, skipped, errors := scanTraditionalFiles(dirPath, hashFileIDs, ctx)

	// Optional: handle duplicate hash-based files (CAS-version duplicates) by quarantining/deleting older versions.
	if ctx.HashDuplicates && len(hashDuplicates) > 0 {
		dd, ss, errs := cleanupHashDuplicatesForKind(ctx, kind, dirPath, hashDuplicates)
		deleted += dd
		skipped += ss
		errors = append(errors, errs...)
	}

	return KindCleanupResult{
		Kind:    kind,
		Deleted: deleted,
		Skipped: skipped,
		Errors:  errors,
	}
}

// cleanupKindsInParallel processes all kinds in parallel.
// When a goroutine budget is set, uses a budgeted pool; otherwise starts one goroutine per kind.
func cleanupKindsInParallel(ctx *CleanupDuplicatesContext) (int, int, []error) {
	type result struct {
		kindResult KindCleanupResult
	}

	results := make(chan result, len(ctx.Kinds))
	var wg sync.WaitGroup
	nk := len(ctx.Kinds)
	poolCtx := context.Background()

	workerCount := min(16, nk)
	if workerCount < 1 {
		workerCount = 1
	}
	queueSize := min(nk, 256)
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "cleanup_duplicates_worker", "cleaning up duplicates", workerCount, queueSize)
	pool.Start(poolCtx)
	defer pool.Stop()
	for _, kind := range ctx.Kinds {
		k := kind
		wg.Add(1)
		_ = pool.Submit(poolCtx, func(taskCtx context.Context) error {
			defer wg.Done()
			kindResult := cleanupKind(ctx, k)
			results <- result{kindResult: kindResult}
			return nil
		})
	}
	collectorBud := goroutinelabels.DefaultBudget()
	collectorBuilder := goroutinelabels.NewGoroutine("cleanup_duplicates_collector", "collecting duplicate cleanup results").
		WithCleanup(func() { close(results) })
	if collectorBud != nil {
		collectorBuilder = collectorBuilder.WithBudget(collectorBud)
	}
	collectorBuilder.StartSimple(func() { wg.Wait() })

	totalDeleted := 0
	totalSkipped := 0
	var allErrors []error
	totalKinds := nk
	completedIndex := 0

	for r := range results {
		totalDeleted += r.kindResult.Deleted
		totalSkipped += r.kindResult.Skipped
		allErrors = append(allErrors, r.kindResult.Errors...)
		completedIndex++
		if ctx.OnKindProgress != nil {
			ctx.OnKindProgress(r.kindResult.Kind, completedIndex, totalKinds)
		}
	}

	return totalDeleted, totalSkipped, allErrors
}

// RunHashDuplicatesCleanupForKinds runs hash-duplicates cleanup for the given kinds
// without going through the cobra command. Used by check --auto-fix to resolve
// "Stale CAS version" issues in the same run (integrity resolution plan).
// Duplicate hash-based files are always deleted (no quarantine).
// onKindProgress, if non-nil, is called as each kind completes (completedIndex 1-based, total = len(kinds)).
// Returns total number of files deleted and any errors.
func RunHashDuplicatesCleanupForKinds(projectRoot string, kinds []string, dryRun, deleteHashDuplicates bool, logger logging.Logger, onKindProgress KindProgressFunc, deleteForKinds []string) (totalDeleted int, errs []error) {
	if projectRoot == emptyValue || len(kinds) == 0 {
		return 0, nil
	}
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	quarantineDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, paths.QuarantineDir, "hash-duplicates")
	deleteForKindsMap := make(map[string]bool)
	for _, k := range deleteForKinds {
		deleteForKindsMap[k] = true
	}
	ctx := &CleanupDuplicatesContext{
		ProjectRoot:                  projectRoot,
		ProcessDir:                   processDir,
		Kinds:                        kinds,
		DryRun:                       dryRun,
		Verbose:                      false,
		HashDuplicates:               true,
		DeleteHashDuplicates:         deleteHashDuplicates,
		DeleteHashDuplicatesForKinds: deleteForKindsMap,
		QuarantineDir:                quarantineDir,
		Logger:                       logger,
		OnKindProgress:               onKindProgress,
	}
	totalDeleted, _, allErrors := cleanupKindsInParallel(ctx)
	return totalDeleted, allErrors
}
