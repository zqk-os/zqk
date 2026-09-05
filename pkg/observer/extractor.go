package observer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// Extractor parses source files and extracts structured entities (functions, types, etc.).
// Implementations are language-specific (e.g. Go via go/ast, others via tree-sitter later).
type Extractor interface {
	// Language returns the language code this extractor handles (e.g. "go").
	Language() string
	// ExtractFile parses a single file and returns entities and any parse errors.
	ExtractFile(ctx context.Context, path string, content []byte) ([]Entity, error)
	// FileSuffix returns the file suffix this extractor handles (e.g. ".go").
	FileSuffix() string
}

// ExtractFromDir walks dir (using fs.FS) and runs the appropriate extractor for each file,
// collecting all entities and errors. It uses extractors to dispatch by language.
func ExtractFromDir(ctx context.Context, fsys fs.FS, dir string, extractors []Extractor) (*ExtractResult, error) {
	if fsys == nil {
		fsys = os.DirFS(dir)
		dir = "."
	}

	suffixToExtractor := make(map[string]Extractor)
	for _, ex := range extractors {
		suffixToExtractor[ex.FileSuffix()] = ex
	}

	var files []string
	err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if _, ok := suffixToExtractor[ext]; ok {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // 10 concurrent extractors

	var processed int32
	total := int32(len(files))

	// Use lock-free index-based aggregation arrays to avoid Mutex contention
	entitiesResults := make([][]Entity, len(files))
	errorsResults := make([][]ExtractError, len(files))

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var processErr error
	var errOnce sync.Once

	for i, path := range files {
		i, path := i, path

		select {
		case <-execCtx.Done():
			break
		default:
		}

		sem <- struct{}{}

		goroutinelabels.NewGoroutine("extractor", "parsing AST").
			WithContext(execCtx).
			WithWaitGroup(&wg).
			WithPreCleanup(func() {
				<-sem
			}).
			WithErrorHandler(func(err error) {
				if err != nil {
					errOnce.Do(func() {
						processErr = err
						cancel()
					})
				}
			}).
			WithPanicHandler(func(r interface{}) {
				errOnce.Do(func() {
					processErr = fmt.Errorf("panic: %v", r)
					cancel()
				})
			}).
			StartWithContext(execCtx, func(ctx context.Context) error {
				content, readErr := fs.ReadFile(fsys, path)
				if readErr != nil {
					errorsResults[i] = []ExtractError{{File: path, Msg: readErr.Error()}}
				} else {
					ex := suffixToExtractor[filepath.Ext(path)]
					fileEntities, parseErr := ex.ExtractFile(ctx, path, content)
					if parseErr != nil {
						errorsResults[i] = []ExtractError{{File: path, Msg: parseErr.Error()}}
					} else {
						entitiesResults[i] = fileEntities
					}
				}

				p := atomic.AddInt32(&processed, 1)
				if p%50 == 0 || p == total {
					logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("Extracting AST: %d/%d files", p, total)).Log()
				}
				return nil
			})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("wait_group", "waiting for extractors").
		WithCleanup(func() { close(waitDone) }).
		WithPanicHandler(func(r interface{}) {}).
		StartSimple(func() { wg.Wait() })

	select {
	case <-waitDone:
	case <-time.After(15 * time.Minute):
		return nil, fmt.Errorf("AST extraction timeout")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if processErr != nil {
		return nil, processErr
	}

	out := &ExtractResult{}
	// Flatten lock-free arrays back onto the output struct
	for _, entities := range entitiesResults {
		if len(entities) > 0 {
			out.Entities = append(out.Entities, entities...)
		}
	}
	for _, errs := range errorsResults {
		if len(errs) > 0 {
			out.Errors = append(out.Errors, errs...)
		}
	}
	return out, nil
}
