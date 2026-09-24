package system

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"bufio"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/git"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Compiled once at package load to avoid per-call cost.
var (
	hashFilePattern = regexp.MustCompile(`^[a-f0-9]{64}\.yaml$`)
	// Top-level object id only (column 0). Indented "id:" (nested maps) must not be
	// treated as the object id — that false-grouped hash duplicates and let cleanup
	// delete sole real objects.
	idLineRegex = regexp.MustCompile(`(?m)^id\s*:\s*(.+)$`)
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

	// Bypass/system-generated kinds delete by default (quarantine would grow forever).
	// All other kinds quarantine unless --delete-hash-duplicates.
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
	f, err := fileutil.Open(filePath)
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
	data, err := fileutil.ReadFile(path)
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

	err := filepath.Walk(dirPath, func(path string, info fileutil.FileInfo, err error) error {
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

// quarantineOrDeleteDuplicateHashFile moves a hash-duplicate into QuarantineDir (default),
// or deletes when --delete-hash-duplicates / bypass kind.
func quarantineOrDeleteDuplicateHashFile(ctx *CleanupDuplicatesContext, kind, objectID, filePath string) (bool, error) {
	deleteIt := ctx.DeleteHashDuplicates ||
		(ctx.DeleteHashDuplicatesForKinds != nil && ctx.DeleteHashDuplicatesForKinds[kind])
	if ctx.DryRun {
		if deleteIt {
			logging.Fluent(ctx.Logger).Info("Would delete hash-duplicate").File(filePath).Kind(kind).ObjectID(objectID).Log()
		} else {
			logging.Fluent(ctx.Logger).Info("Would quarantine hash-duplicate").File(filePath).Kind(kind).ObjectID(objectID).
				String("quarantine_dir", ctx.QuarantineDir).Log()
		}
		return true, nil
	}
	if deleteIt {
		if err := fileutil.RemoveFile(filePath); err != nil && !fileutil.IsNotExist(err) {
			return false, err
		}
		return true, nil
	}
	qDir := ctx.QuarantineDir
	if qDir == emptyValue {
		qDir = caspkg.DefaultCASDuplicateQuarantineDir(ctx.ProjectRoot)
	}
	if err := caspkg.QuarantineCASHashFile(filePath, qDir, kind); err != nil {
		if fileutil.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}

type gitFileState int

const (
	gitStateUnknown gitFileState = iota
	gitStateCleanTracked
	gitStateModifiedTracked
	gitStateDeletedTracked
	gitStateUntracked
)

// inspectGitStatusForPaths inspects the git tracking status of given file paths relative to projectRoot.
func inspectGitStatusForPaths(projectRoot string, filePaths []string) map[string]gitFileState {
	states := make(map[string]gitFileState, len(filePaths))
	if projectRoot == emptyValue || len(filePaths) == 0 {
		return states
	}
	gitDir := filepath.Join(projectRoot, ".git")
	if _, err := fileutil.Stat(gitDir); err != nil {
		return states
	}

	relPaths := make([]string, 0, len(filePaths))
	pathToClean := make(map[string]string, len(filePaths))
	for _, p := range filePaths {
		clean := filepath.Clean(p)
		rel, err := filepath.Rel(projectRoot, clean)
		if err != nil {
			rel = clean
		}
		relPaths = append(relPaths, rel)
		pathToClean[rel] = clean
	}

	g := git.NewFacade(projectRoot)
	outLs, err := g.LSFiles(append([]string{"--"}, relPaths...)...)
	if err != nil {
		return states
	}
	trackedRels := make(map[string]bool)
	for _, line := range strings.Split(string(outLs), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != emptyValue {
			trackedRels[trimmed] = true
		}
	}

	outSt, _ := g.StatusPorcelain(append([]string{"-uall", "--"}, relPaths...)...)
	statusMap := make(map[string]string)
	for _, line := range strings.Split(string(outSt), "\n") {
		if len(line) >= 4 {
			code := line[:2]
			p := strings.TrimSpace(line[3:])
			statusMap[p] = code
		}
	}

	for _, rel := range relPaths {
		orig := pathToClean[rel]
		if trackedRels[rel] {
			if code, exists := statusMap[rel]; exists {
				if strings.Contains(code, "D") {
					states[orig] = gitStateDeletedTracked
				} else {
					states[orig] = gitStateModifiedTracked
				}
			} else {
				states[orig] = gitStateCleanTracked
			}
		} else {
			states[orig] = gitStateUntracked
		}
	}

	return states
}

// selectKeeperCASPath chooses the authoritative keeper file among duplicate CAS paths for an object ID.
//
// Resolution Priority:
//  1. Git-reconciled index alignment:
//     If the CAS index maps objectID to an existing path, and projectRoot is a Git repository:
//     - If the indexed path is tracked by Git and clean (unmodified in worktree), while sibling
//     paths are untracked (e.g. branch transition remnants), the indexed path is chosen.
//     - If the indexed path was deleted in the Git worktree and an untracked sibling exists,
//     the untracked sibling is chosen as an active mutation in progress.
//  2. Index alignment:
//     If the CAS index maps to one of the surviving paths, prefer that path over unindexed duplicates.
//  3. Fallback:
//     Newest file modification time (mtime).
func selectKeeperCASPath(paths []string, objectID string, cas *storage.ContentAddressableStorage, projectRoot string) string {
	if len(paths) == 0 {
		return emptyValue
	}
	if len(paths) == 1 {
		return paths[0]
	}

	var indexedPath string
	if cas != nil {
		if idx := cas.GetIndex(); idx != nil && idx.Mappings != nil {
			if h, ok := idx.Mappings[objectID]; ok && h != emptyValue {
				for _, p := range paths {
					base := filepath.Base(p)
					if strings.TrimSuffix(base, filepath.Ext(base)) == h {
						if _, err := fileutil.Stat(p); err == nil {
							indexedPath = p
							break
						}
					}
				}
			}
		}
	}

	gitStates := inspectGitStatusForPaths(projectRoot, paths)
	if len(gitStates) > 0 {
		if indexedPath != emptyValue {
			st := gitStates[indexedPath]
			if st == gitStateCleanTracked {
				return indexedPath
			}
			if st == gitStateDeletedTracked {
				var bestNew string
				var bestMTime time.Time
				for _, p := range paths {
					if gitStates[p] == gitStateUntracked {
						if fi, err := fileutil.Stat(p); err == nil {
							if bestNew == emptyValue || fi.ModTime().After(bestMTime) {
								bestNew = p
								bestMTime = fi.ModTime()
							}
						}
					}
				}
				if bestNew != emptyValue {
					return bestNew
				}
			}
		}

		var cleanTracked []string
		for _, p := range paths {
			if gitStates[p] == gitStateCleanTracked {
				cleanTracked = append(cleanTracked, p)
			}
		}
		if len(cleanTracked) == 1 {
			return cleanTracked[0]
		}
	}

	if indexedPath != emptyValue {
		return indexedPath
	}

	keep := emptyValue
	var keepMTime time.Time
	for _, p := range paths {
		fi, err := fileutil.Stat(p)
		if err != nil {
			continue
		}
		if keep == emptyValue || fi.ModTime().After(keepMTime) {
			keepMTime = fi.ModTime()
			keep = p
		}
	}
	return keep
}

// reconcileCASIndexMapping updates the CAS index to point objectID at the keeper hash file.
// Caller must pass the same cas instance used for this kind to avoid creating one per object.
func reconcileCASIndexMapping(cas *storage.ContentAddressableStorage, kind, objectID, keepFilePath string) error {
	base := filepath.Base(keepFilePath)
	if !strings.HasSuffix(base, ".yaml") || len(base) != 69 {
		return errfmt.Errorf("cannot extract hash from filename: %s", base)
	}
	hash := base[:64]

	if cas != nil {
		if idx := cas.GetIndex(); idx != nil {
			kindDir := filepath.Dir(idx.FilePath)
			relDir, relErr := filepath.Rel(kindDir, filepath.Dir(keepFilePath))
			var bucketKey string
			if relErr == nil && relDir != "." && relDir != "" {
				bucketKey = relDir
			}
			if bucketKey != "" {
				_ = idx.SetMapping(objectID, hash, bucketKey)
			} else {
				_ = idx.SetMapping(objectID, hash)
			}
		}
	}

	writeQueue := caspkg.GetGlobalListingIndexWriteQueue()
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
		if c, ok := getCachedCASForKind(context.Background(), ctx.ProjectRoot, kind); ok { // Background: request-or-shutdown derived
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

		keep := selectKeeperCASPath(paths, objectID, cas, ctx.ProjectRoot)
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
			// Refuse to delete if this path is not actually the same object id (misgrouped
			// by nested id: lines) or if it is the last surviving file for this id on disk.
			trueID, idErr := objectIDFromHashFile(p, ctx.Verbose, ctx.Logger)
			if idErr != nil || trueID == emptyValue || trueID != objectID {
				logging.Fluent(ctx.Logger).Warn("Skipping hash-duplicate delete: id mismatch or unreadable").
					File(p).ObjectID(objectID).String("parsed_id", trueID).WithError(idErr).Log()
				skipped++
				continue
			}
			surviving := 0
			for _, cand := range paths {
				if _, stErr := fileutil.Stat(cand); stErr == nil {
					surviving++
				}
			}
			if surviving <= 1 {
				logging.Fluent(ctx.Logger).Warn("Skipping hash-duplicate delete: would remove sole surviving CAS file").
					File(p).ObjectID(objectID).Log()
				skipped++
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
	data, err := fileutil.ReadFile(filePath)
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

	if err := fileutil.RemoveFile(filePath); err != nil {
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
	if _, statErr := fileutil.Stat(hashFilePath); statErr != nil {
		logging.Fluent(ctx.Logger).Warn("Refusing to delete traditional duplicate; hash-based equivalent missing").
			File(filePath).ObjectID(objectID).WithError(statErr).Log()
		return false, true, nil
	}

	wasDeleted, err := handleDuplicateFile(filePath, hashFilePath, ctx)
	if err != nil {
		return false, true, err
	}
	return wasDeleted, false, nil
}

// scanTraditionalFiles walks dirPath and removes traditional YAML files that duplicate hash-based objects.
func scanTraditionalFiles(dirPath string, hashFileIDs map[string]string, ctx *CleanupDuplicatesContext) (deleted, skipped int, errors []error) {
	err := filepath.Walk(dirPath, func(path string, info fileutil.FileInfo, err error) error {
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
	if _, err := fileutil.Stat(dirPath); fileutil.IsNotExist(err) {
		return KindCleanupResult{Kind: kind}
	}

	var hashFileIDs map[string]string
	var hashDuplicates map[string][]string

	if !ctx.HashDuplicates {
		if primary, _ := primaryFromCASIndex(context.Background(), dirPath, kind, ctx.ProjectRoot); len(primary) > 0 { // Background: request-or-shutdown derived
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
	poolCtx := context.Background() // Background: request-or-shutdown derived

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

	if ctx.ProjectRoot != emptyValue && totalDeleted > 0 {
		ClearCASDuplicateIDInventoryCache(ctx.ProjectRoot)
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
