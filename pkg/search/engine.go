package search

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Engine is the central in-process search orchestrator.
type Engine struct {
	projectRoot string
	cachedIndex *TrigramIndex
	indexMu     sync.RWMutex
}

// NewEngine constructs a new search Engine rooted at projectRoot.
func NewEngine(projectRoot string) *Engine {
	return &Engine{
		projectRoot: filepath.Clean(projectRoot),
	}
}

// BuildTrigramIndex explicitly builds and caches an in-memory trigram index for the project.
func (e *Engine) BuildTrigramIndex(opts SearchOptions) error {
	root := e.projectRoot
	if opts.Path != "" {
		if filepath.IsAbs(opts.Path) {
			root = opts.Path
		} else {
			root = filepath.Join(e.projectRoot, opts.Path)
		}
	}

	files, err := CollectFiles(root, opts)
	if err != nil {
		return err
	}

	idx, err := BuildIndex(files)
	if err != nil {
		return err
	}

	e.indexMu.Lock()
	e.cachedIndex = idx
	e.indexMu.Unlock()
	return nil
}

// Search executes an in-process search adhering to the provided options.
func (e *Engine) Search(ctx context.Context, opts SearchOptions) (*SearchResult, error) {
	start := time.Now()

	// Default fallbacks
	if opts.MaxMatches <= 0 {
		opts.MaxMatches = DefaultMaxMatches
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = DefaultMaxTokens
	}

	targetRoot := e.projectRoot
	if opts.Path != "" {
		if filepath.IsAbs(opts.Path) {
			targetRoot = opts.Path
		} else {
			targetRoot = filepath.Join(e.projectRoot, opts.Path)
		}
	}

	// Validate query regex if requested
	var queryRegex *regexp.Regexp
	if opts.Regex && opts.Query != "" {
		pattern := opts.Query
		if opts.CaseInsensitive {
			pattern = "(?i)" + pattern
		}
		if opts.WordMatch {
			pattern = `\b` + pattern + `\b`
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid search regular expression: %w", err)
		}
		queryRegex = re
	}

	if opts.Mode == ModeAST {
		// AST search requires Go files
		if len(opts.FileExtensions) == 0 {
			opts.FileExtensions = []string{".go"}
		}
		return e.searchAST(ctx, targetRoot, opts, start)
	}

	return e.searchText(ctx, targetRoot, opts, queryRegex, start)
}

func (e *Engine) searchText(ctx context.Context, targetRoot string, opts SearchOptions, queryRegex *regexp.Regexp, start time.Time) (*SearchResult, error) {
	files, err := CollectFiles(targetRoot, opts)
	if err != nil {
		return nil, err
	}

	// If trigram index is enabled or cached, narrow candidate files
	if opts.UseIndex && !opts.Regex && len(opts.Query) >= 3 {
		e.indexMu.RLock()
		idx := e.cachedIndex
		e.indexMu.RUnlock()

		if idx != nil {
			candidates := idx.FilterCandidates(opts.Query)
			candidateMap := make(map[string]bool, len(candidates))
			for _, c := range candidates {
				candidateMap[c] = true
			}
			var filtered []string
			for _, f := range files {
				if candidateMap[f] {
					filtered = append(filtered, f)
				}
			}
			files = filtered
		}
	}

	// Construct line matcher
	lowerQuery := []byte(strings.ToLower(opts.Query))
	exactQuery := []byte(opts.Query)

	matcher := func(line []byte) (int, int, bool) {
		if opts.Query == "" {
			return 0, 0, false
		}
		if queryRegex != nil {
			loc := queryRegex.FindIndex(line)
			if loc == nil {
				return 0, 0, false
			}
			return loc[0], loc[1], true
		}

		searchIn := line
		q := exactQuery
		if opts.CaseInsensitive {
			searchIn = bytes.ToLower(line)
			q = lowerQuery
		}

		idx := bytes.Index(searchIn, q)
		if idx == -1 {
			return 0, 0, false
		}

		if opts.WordMatch {
			if !IsWordBoundary(line, idx, idx+len(q)) {
				return 0, 0, false
			}
		}

		return idx, idx + len(q), true
	}

	// Fast pre-filter to reject non-matching files without line-by-line scanning
	var quickFileMatch func(data []byte) bool
	if opts.Query != "" {
		if queryRegex != nil {
			quickFileMatch = queryRegex.Match
		} else if opts.CaseInsensitive {
			pattern := "(?i)" + regexp.QuoteMeta(opts.Query)
			if opts.WordMatch {
				pattern = `\b` + pattern + `\b`
			}
			if re, err := regexp.Compile(pattern); err == nil {
				quickFileMatch = re.Match
			}
		} else if opts.WordMatch {
			pattern := `\b` + regexp.QuoteMeta(opts.Query) + `\b`
			if re, err := regexp.Compile(pattern); err == nil {
				quickFileMatch = re.Match
			}
		} else {
			quickFileMatch = func(data []byte) bool {
				return bytes.Contains(data, exactQuery)
			}
		}
	}

	result := &SearchResult{
		FilesSearched: len(files),
	}

	var matchesMu sync.Mutex
	var totalMatches int32
	var accumulatedTokens int32
	var stopped int32

	numWorkers := runtime.GOMAXPROCS(0) * 2
	if numWorkers > 32 {
		numWorkers = 32
	}
	if numWorkers < 2 {
		numWorkers = 2
	}

	fileCh := make(chan string, 128)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileCh {
				if atomic.LoadInt32(&stopped) != 0 {
					continue
				}

				select {
				case <-ctx.Done():
					continue
				default:
				}

				data, err := fileutil.ReadFile(path)
				if err != nil || IsBinary(data) {
					continue
				}

				if quickFileMatch != nil && !quickFileMatch(data) {
					continue
				}

				relPath, err := filepath.Rel(e.projectRoot, path)
				if err != nil {
					relPath = path
				}

				fileMatches := LineSearch(relPath, data, matcher, opts.ContextLines)
				if len(fileMatches) == 0 {
					continue
				}

				matchesMu.Lock()
				for _, m := range fileMatches {
					mTokens := EstimateMatchTokens(m)
					curTokens := atomic.LoadInt32(&accumulatedTokens)
					curCount := atomic.LoadInt32(&totalMatches)

					if opts.MaxTokens > 0 && int(curTokens)+mTokens > opts.MaxTokens {
						result.Truncated = true
						result.TruncateReason = "max_tokens"
						atomic.StoreInt32(&stopped, 1)
						break
					}

					if opts.MaxMatches > 0 && int(curCount) >= opts.MaxMatches {
						result.Truncated = true
						result.TruncateReason = "max_matches"
						atomic.StoreInt32(&stopped, 1)
						break
					}

					atomic.AddInt32(&accumulatedTokens, int32(mTokens))
					atomic.AddInt32(&totalMatches, 1)
					result.Matches = append(result.Matches, m)
				}
				matchesMu.Unlock()
			}
		}()
	}

	for _, f := range files {
		if atomic.LoadInt32(&stopped) != 0 {
			break
		}
		select {
		case <-ctx.Done():
			break
		case fileCh <- f:
		}
	}
	close(fileCh)
	wg.Wait()

	result.TotalMatches = len(result.Matches)
	result.EstimatedTokens = int(atomic.LoadInt32(&accumulatedTokens))
	result.Duration = time.Since(start)
	result.DurationMs = float64(result.Duration.Microseconds()) / 1000.0

	return result, nil
}

func (e *Engine) searchAST(ctx context.Context, targetRoot string, opts SearchOptions, start time.Time) (*SearchResult, error) {
	files, err := CollectFiles(targetRoot, opts)
	if err != nil {
		return nil, err
	}

	result := &SearchResult{
		FilesSearched: len(files),
	}

	var matchesMu sync.Mutex
	var totalMatches int32
	var accumulatedTokens int32
	var stopped int32

	numWorkers := runtime.GOMAXPROCS(0) * 2
	if numWorkers > 32 {
		numWorkers = 32
	}
	if numWorkers < 2 {
		numWorkers = 2
	}

	fileCh := make(chan string, 64)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileCh {
				if atomic.LoadInt32(&stopped) != 0 {
					continue
				}

				select {
				case <-ctx.Done():
					continue
				default:
				}

				// If searching for a specific symbol or receiver, skip files that don't contain the token
				targetSymbol := opts.Query
				if targetSymbol == "" && opts.ASTReceiver != "" {
					targetSymbol = opts.ASTReceiver
				}
				if targetSymbol != "" {
					data, err := fileutil.ReadFile(path)
					if err != nil || IsBinary(data) || !bytes.Contains(data, []byte(targetSymbol)) {
						continue
					}
				}

				relPath, err := filepath.Rel(e.projectRoot, path)
				if err != nil {
					relPath = path
				}

				astMatches, err := ASTSearchFile(path, nil, opts)
				if err != nil || len(astMatches) == 0 {
					continue
				}

				matchesMu.Lock()
				for _, m := range astMatches {
					m.File = relPath
					mTokens := EstimateMatchTokens(m)
					curTokens := atomic.LoadInt32(&accumulatedTokens)
					curCount := atomic.LoadInt32(&totalMatches)

					if opts.MaxTokens > 0 && int(curTokens)+mTokens > opts.MaxTokens {
						result.Truncated = true
						result.TruncateReason = "max_tokens"
						atomic.StoreInt32(&stopped, 1)
						break
					}

					if opts.MaxMatches > 0 && int(curCount) >= opts.MaxMatches {
						result.Truncated = true
						result.TruncateReason = "max_matches"
						atomic.StoreInt32(&stopped, 1)
						break
					}

					atomic.AddInt32(&accumulatedTokens, int32(mTokens))
					atomic.AddInt32(&totalMatches, 1)
					result.Matches = append(result.Matches, m)
				}
				matchesMu.Unlock()
			}
		}()
	}

	for _, f := range files {
		if atomic.LoadInt32(&stopped) != 0 {
			break
		}
		select {
		case <-ctx.Done():
			break
		case fileCh <- f:
		}
	}
	close(fileCh)
	wg.Wait()

	result.TotalMatches = len(result.Matches)
	result.EstimatedTokens = int(atomic.LoadInt32(&accumulatedTokens))
	result.Duration = time.Since(start)
	result.DurationMs = float64(result.Duration.Microseconds()) / 1000.0

	return result, nil
}
