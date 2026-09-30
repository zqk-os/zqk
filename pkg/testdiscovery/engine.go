package testdiscovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Engine manages test discovery across multiple languages.
type Engine struct {
	discoverers []Discoverer
}

// NewEngine creates a test discovery engine with default language discoverers.
func NewEngine() *Engine {
	return &Engine{
		discoverers: []Discoverer{
			NewGoDiscoverer(),
			NewPythonDiscoverer(),
			NewTypeScriptDiscoverer(),
			NewCargoDiscoverer(),
		},
	}
}

// RegisterDiscoverer adds a custom language discoverer.
func (e *Engine) RegisterDiscoverer(d Discoverer) {
	e.discoverers = append(e.discoverers, d)
}

// Discover scans the project according to the provided options.
func (e *Engine) Discover(ctx context.Context, opts DiscoveryOptions) ([]DiscoveredTarget, error) {
	cleanRoot := filepath.Clean(opts.ProjectRoot)
	if cleanRoot == "" {
		var err error
		cleanRoot, err = os.Getwd()
		if err != nil {
			return nil, errfmt.Newf("failed to determine project root").Wrap(err)
		}
	}

	// Resolve symlinks on projectRoot to have canonical comparison
	canonicalRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		canonicalRoot = cleanRoot
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
		if workers < 2 {
			workers = 2
		}
	}

	var cache *DiscoveryCache
	if opts.Incremental {
		cachePath := opts.CachePath
		if cachePath == "" {
			cachePath = filepath.Join(cleanRoot, paths.ProjectDataDir, paths.CacheDir, "test_discovery.json")
		}
		cache = LoadCache(cachePath)
	}

	// 1. Gather all candidate files
	candidates, err := e.collectCandidateFiles(cleanRoot, canonicalRoot, opts)
	if err != nil {
		return nil, err
	}

	// 2. Process files through worker pool
	taskCh := make(chan fileCandidate, len(candidates))
	for _, c := range candidates {
		taskCh <- c
	}
	close(taskCh)

	var (
		mu         sync.Mutex
		allTargets []DiscoveredTarget
		wg         sync.WaitGroup
		errOnce    sync.Once
		firstErr   error
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("testdiscovery.worker", "discover test targets in files").StartSimple(func() {
			defer wg.Done()
			for t := range taskCh {
				select {
				case <-ctx.Done():
					errOnce.Do(func() { firstErr = ctx.Err() })
					return
				default:
				}

				// Check cache
				if cache != nil {
					if targets, ok := cache.Get(t.relPath, t.info.ModTime(), nil); ok {
						mu.Lock()
						allTargets = append(allTargets, targets...)
						mu.Unlock()
						continue
					}
				}

				// Read file statically
				content, err := fileutil.ReadFile(t.absPath)
				if err != nil {
					continue
				}

				// Dispatch to matching discoverer
				var fileTargets []DiscoveredTarget
				for _, d := range e.discoverers {
					if len(opts.Languages) > 0 && !containsString(opts.Languages, d.Language()) {
						continue
					}
					if d.CanHandle(t.relPath) {
						found, err := d.Discover(ctx, cleanRoot, t.relPath, content)
						if err != nil {
							continue
						}
						fileTargets = append(fileTargets, found...)
					}
				}

				if cache != nil {
					cache.Set(t.relPath, t.info.ModTime(), content, fileTargets)
				}

				mu.Lock()
				allTargets = append(allTargets, fileTargets...)
				mu.Unlock()
			}
		})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("testdiscovery.wait", "wait for discovery workers").StartSimple(func() {
		wg.Wait()
		close(waitDone)
	})
	select {
	case <-waitDone:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if firstErr != nil {
		return nil, firstErr
	}

	if cache != nil {
		_ = cache.Save()
	}

	return allTargets, nil
}

type fileCandidate struct {
	relPath string
	absPath string
	info    os.FileInfo
}

func (e *Engine) collectCandidateFiles(cleanRoot, canonicalRoot string, opts DiscoveryOptions) ([]fileCandidate, error) {
	var candidates []fileCandidate

	// Standard ignore directories
	ignoreDirs := map[string]bool{
		".git":               true,
		"node_modules":      true,
		"vendor":            true,
		paths.ProjectDataDir: true,
		"dist":              true,
		"build":             true,
		"target":            true,
	}
	for _, p := range opts.ExcludePaths {
		ignoreDirs[p] = true
	}

	scanPaths := opts.Paths
	if len(scanPaths) == 0 {
		scanPaths = []string{"."}
	}

	for _, sub := range scanPaths {
		targetDir := filepath.Join(cleanRoot, sub)
		err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Security check: Verify path stays strictly within projectRoot (reject symlink escapes)
			canonicalPath, err := filepath.EvalSymlinks(path)
			if err == nil {
				if !strings.HasPrefix(canonicalPath, canonicalRoot) {
					// Outside project root boundary! Skip or reject
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}

			name := info.Name()
			if info.IsDir() {
				if ignoreDirs[name] || strings.HasPrefix(name, ".") && name != "." {
					return filepath.SkipDir
				}
				return nil
			}

			relPath, err := filepath.Rel(cleanRoot, path)
			if err != nil {
				return nil
			}

			// Check if any discoverer handles this file
			handles := false
			for _, d := range e.discoverers {
				if len(opts.Languages) > 0 && !containsString(opts.Languages, d.Language()) {
					continue
				}
				if d.CanHandle(relPath) {
					handles = true
					break
				}
			}

			if handles {
				candidates = append(candidates, fileCandidate{
					relPath: relPath,
					absPath: path,
					info:    info,
				})
			}
			return nil
		})
		if err != nil {
			return nil, errfmt.Newf("error walking path %s", sub).Wrap(err)
		}
	}

	return candidates, nil
}

// GenerateTestCaseObject transforms a DiscoveredTarget into a compliant test_case kernel object.
func (e *Engine) GenerateTestCaseObject(target DiscoveredTarget, projectRoot string) (map[string]any, error) {
	rawID := fmt.Sprintf("%s:%s:%s", target.Language, target.Path, target.Function)
	h := sha256.Sum256([]byte(rawID))
	tcID := fmt.Sprintf("TST-%s-%s", strings.ToUpper(target.Language), hex.EncodeToString(h[:4]))

	category := "unit"
	for _, tag := range target.Tags {
		lower := strings.ToLower(tag)
		if strings.Contains(lower, "integration") {
			category = "integration"
			break
		} else if strings.Contains(lower, "e2e") {
			category = "e2e"
			break
		} else if strings.Contains(lower, "benchmark") || strings.Contains(lower, "perf") {
			category = "performance"
			break
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tcObj := map[string]any{
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyID:              tcID,
		objects.FieldKeyTitle:           fmt.Sprintf("[%s] %s (%s)", strings.ToUpper(target.Language), target.Function, target.Path),
		objects.FieldKeyDescription:     fmt.Sprintf("Discovered automated test in %s at %s:%d", target.Language, target.Path, target.Line),
		objects.FieldKeyStatus:          objects.ObjectStatusOriginated,
		objects.FieldKeyCategory:        category,
		objects.FieldKeyPathOrID:        target.Path,
		objects.FieldKeyCriteriaRefs:    target.CriteriaRefs,
		objects.FieldKeyRequirementRefs: target.RequirementRefs,
		objects.FieldKeyCreatedAt:       now,
		objects.FieldKeyUpdatedAt:       now,
	}

	if target.ExecutionCommand != "" {
		tcObj[objects.FieldKeyVerificationSuites] = []string{target.ExecutionCommand}
	}

	return tcObj, nil
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}
