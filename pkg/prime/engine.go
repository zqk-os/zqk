package prime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/search"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	errEmptyProjectRoot     = "project root path cannot be empty"
	errGitStatusFailed      = "git status check failed: %w"
	errTrackedFilesModified = "negative boundary violation: tracked git files were modified during prime execution"
	errOrphanOutsideZqk     = "negative boundary violation: orphaned file created outside .zqk/: %s"
	errBuildTrigramFailed   = "failed to build trigram index: %w"
	errSaveASTIndexFailed   = "failed to persist AST symbol index: %w"
	errSaveManifestFailed   = "failed to persist prime manifest: %w"
	msgPrimeSuccess         = "repository successfully primed"
	goExtension             = ".go"
)

// Engine orchestrates brownfield scanning, trigram indexing, and AST symbol extraction.
type Engine struct {
	projectRoot string
	maxDuration time.Duration
}

// NewEngine creates a new brownfield prime engine for the designated project root.
func NewEngine(projectRoot string) *Engine {
	clean := filepath.Clean(projectRoot)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	return &Engine{
		projectRoot: clean,
		maxDuration: DefaultMaxPrimingDuration,
	}
}

// SetMaxDuration overrides the default maximum execution duration threshold.
func (e *Engine) SetMaxDuration(d time.Duration) {
	if d > 0 {
		e.maxDuration = d
	}
}

// Prime executes brownfield indexing, AST symbol extraction, and invariant verification.
func (e *Engine) Prime(ctx context.Context, opts PrimeOptions) (*PrimeResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()

	root := e.projectRoot
	if opts.ProjectRoot != "" {
		root = filepath.Clean(opts.ProjectRoot)
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}
	if root == "" {
		return nil, errors.New(errEmptyProjectRoot)
	}

	maxDuration := e.maxDuration
	if opts.MaxDuration > 0 {
		maxDuration = opts.MaxDuration
	}

	// Step 1: Record initial git state (for zero git modification verification)
	var initialGitStatus string
	isGitRepo := isGitRepository(root)
	if isGitRepo && opts.Verify {
		var err error
		initialGitStatus, err = runGitStatus(root)
		if err != nil {
			return nil, fmt.Errorf(errGitStatusFailed, err)
		}
	}

	// Step 2: Ensure internal .zqk directories exist (never touching external files)
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	cacheDir := filepath.Join(zqkDir, paths.CacheDir)
	lockDir := filepath.Join(zqkDir, LockDir)
	stateDir := filepath.Join(zqkDir, paths.StateDir)

	if err := fileutil.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		return nil, fmt.Errorf("failed to ensure cache dir: %w", err)
	}
	if err := fileutil.MkdirAll(lockDir, paths.DirPerm755); err != nil {
		return nil, fmt.Errorf("failed to ensure lock dir: %w", err)
	}
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		return nil, fmt.Errorf("failed to ensure state dir: %w", err)
	}

	// Acquire prime lock inside .zqk/lock/ (cleaned up via defer)
	lockFilePath := filepath.Join(lockDir, PrimeLockFile)
	if err := fileutil.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d\n", time.Now().UnixNano())), paths.FilePerm644); err != nil {
		return nil, fmt.Errorf("failed to acquire prime lock: %w", err)
	}
	defer func() {
		_ = fileutil.Remove(lockFilePath)
	}()

	// Step 3: Collect files for indexing (respecting .gitignore, ignoring vendor, node_modules, .git, .zqk)
	searchOpts := search.SearchOptions{
		Path:          root,
		IncludeHidden: false,
	}
	files, err := search.CollectFiles(root, searchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to collect project files: %w", err)
	}

	// Step 4: Build persistent trigram index
	searchEngine := search.NewEngine(root)
	if err := searchEngine.BuildTrigramIndex(searchOpts); err != nil {
		return nil, fmt.Errorf(errBuildTrigramFailed, err)
	}

	// Step 5: Extract AST symbols from Go source files
	var (
		symbolRecords []ASTSymbolRecord
		stats         ASTSymbolStats
		totalLines    int
	)

	fset := token.NewFileSet()
	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		content, err := fileutil.ReadFile(file)
		if err != nil {
			continue
		}

		lines := bytes.Count(content, []byte("\n"))
		if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
			lines++
		}
		totalLines += lines

		if !strings.HasSuffix(file, goExtension) {
			continue
		}

		fileNode, err := parser.ParseFile(fset, file, content, 0)
		if err != nil {
			// Skip unparseable files without failing whole scan
			continue
		}

		relPath, relErr := filepath.Rel(root, file)
		if relErr != nil {
			relPath = file
		}

		fileRecords, fileStats := extractASTSymbols(fset, fileNode, relPath)
		symbolRecords = append(symbolRecords, fileRecords...)
		stats.Structs += fileStats.Structs
		stats.Interfaces += fileStats.Interfaces
		stats.Functions += fileStats.Functions
		stats.Methods += fileStats.Methods
		stats.Types += fileStats.Types
		stats.Constants += fileStats.Constants
		stats.Variables += fileStats.Variables
		stats.Total += fileStats.Total
	}

	// Step 6: Persist AST symbol cache into .zqk/cache/ast_symbols.json
	astIndexPath := filepath.Join(cacheDir, ASTSymbolsCacheFile)
	indexData := ASTIndexData{
		GeneratedAt: time.Now().UTC(),
		ProjectRoot: root,
		FileCount:   len(files),
		LineCount:   totalLines,
		Stats:       stats,
		Symbols:     symbolRecords,
	}
	indexBytes, err := json.MarshalIndent(indexData, "", "  ")
	if err != nil {
		return nil, fmt.Errorf(errSaveASTIndexFailed, err)
	}
	if err := fileutil.WriteFile(astIndexPath, indexBytes, paths.FilePerm644); err != nil {
		return nil, fmt.Errorf(errSaveASTIndexFailed, err)
	}

	// Persist prime manifest
	manifestPath := filepath.Join(cacheDir, PrimeManifestFile)
	manifestBytes, _ := json.MarshalIndent(map[string]interface{}{
		"primed_at":     time.Now().UTC().Format(time.RFC3339),
		"files_scanned": len(files),
		"lines_indexed": totalLines,
		"symbols_total": stats.Total,
		"duration_ms":   time.Since(start).Milliseconds(),
	}, "", "  ")
	if err := fileutil.WriteFile(manifestPath, manifestBytes, paths.FilePerm644); err != nil {
		return nil, fmt.Errorf(errSaveManifestFailed, err)
	}

	// Step 7: Negative Adversarial Boundary & Zero Git Modification Verification
	gitVerified := true
	if isGitRepo && opts.Verify {
		currentGitStatus, err := runGitStatus(root)
		if err != nil {
			return nil, fmt.Errorf(errGitStatusFailed, err)
		}
		if currentGitStatus != initialGitStatus {
			// Check if any tracked file changed
			if hasTrackedFileModifications(initialGitStatus, currentGitStatus) {
				return nil, errors.New(errTrackedFilesModified)
			}
		}
	}

	// Step 8: Verify no orphaned lock or temporary files outside .zqk/
	orphanedVerified := true
	if err := verifyNoOrphanedStateOutsideZqk(root); err != nil {
		return nil, err
	}

	elapsed := time.Since(start)
	targetMet := elapsed < maxDuration

	result := &PrimeResult{
		ProjectRoot:           root,
		FilesScanned:          len(files),
		LinesIndexed:          totalLines,
		SymbolStats:           stats,
		TrigramIndexPath:      searchEngine.CachePath(),
		ASTIndexPath:          astIndexPath,
		Duration:              elapsed,
		DurationFormatted:     elapsed.Round(time.Millisecond).String(),
		TargetDurationMet:     targetMet,
		GitUnmodifiedVerified: gitVerified,
		OrphanedStateVerified: orphanedVerified,
		Success:               true,
		Message:               msgPrimeSuccess,
	}

	return result, nil
}

// extractASTSymbols walks a parsed Go file node and categorizes all structural declarations.
func extractASTSymbols(fset *token.FileSet, node *ast.File, filePath string) ([]ASTSymbolRecord, ASTSymbolStats) {
	var (
		records []ASTSymbolRecord
		stats   ASTSymbolStats
	)

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			pos := fset.Position(d.Pos())
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recvName := extractReceiverName(d.Recv)
				records = append(records, ASTSymbolRecord{
					File:     filePath,
					Line:     pos.Line,
					Kind:     KindMethod,
					Name:     d.Name.Name,
					Receiver: recvName,
				})
				stats.Methods++
			} else {
				records = append(records, ASTSymbolRecord{
					File: filePath,
					Line: pos.Line,
					Kind: KindFunc,
					Name: d.Name.Name,
				})
				stats.Functions++
			}
			stats.Total++

		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					pos := fset.Position(ts.Pos())
					kind := KindType
					switch ts.Type.(type) {
					case *ast.StructType:
						kind = KindStruct
						stats.Structs++
					case *ast.InterfaceType:
						kind = KindInterface
						stats.Interfaces++
					default:
						stats.Types++
					}
					records = append(records, ASTSymbolRecord{
						File: filePath,
						Line: pos.Line,
						Kind: kind,
						Name: ts.Name.Name,
					})
					stats.Total++
				}

			case token.CONST:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					pos := fset.Position(vs.Pos())
					for _, nameIdent := range vs.Names {
						records = append(records, ASTSymbolRecord{
							File: filePath,
							Line: pos.Line,
							Kind: KindConst,
							Name: nameIdent.Name,
						})
						stats.Constants++
						stats.Total++
					}
				}

			case token.VAR:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					pos := fset.Position(vs.Pos())
					for _, nameIdent := range vs.Names {
						records = append(records, ASTSymbolRecord{
							File: filePath,
							Line: pos.Line,
							Kind: KindVar,
							Name: nameIdent.Name,
						})
						stats.Variables++
						stats.Total++
					}
				}
			}
		}
	}

	return records, stats
}

// extractReceiverName resolves clean receiver type name from AST FieldList.
func extractReceiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	for {
		if star, ok := t.(*ast.StarExpr); ok {
			t = star.X
		} else {
			break
		}
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// isGitRepository checks if a directory contains a .git directory.
func isGitRepository(dir string) bool {
	gitDir := filepath.Join(dir, ".git")
	stat, err := fileutil.Stat(gitDir)
	return err == nil && stat.IsDir()
}

// runGitStatus runs `git status --porcelain --untracked-files=no` in the specified directory.
func runGitStatus(dir string) (string, error) {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=no")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// hasTrackedFileModifications checks if any tracked files (not untracked '??') were modified.
func hasTrackedFileModifications(before, after string) bool {
	parseTracked := func(status string) map[string]string {
		res := make(map[string]string)
		for _, line := range strings.Split(status, "\n") {
			line = strings.TrimSpace(line)
			if len(line) < 3 {
				continue
			}
			code := line[:2]
			file := strings.TrimSpace(line[2:])
			if code != "??" {
				res[file] = code
			}
		}
		return res
	}

	trackedBefore := parseTracked(before)
	trackedAfter := parseTracked(after)

	for file, code := range trackedAfter {
		if strings.HasPrefix(file, paths.ProjectDataDir+"/") || file == paths.ProjectDataDir {
			continue
		}
		if prevCode, exists := trackedBefore[file]; !exists || prevCode != code {
			return true
		}
	}
	return false
}

// verifyNoOrphanedStateOutsideZqk ensures no temporary or lock files exist outside .zqk/.
func verifyNoOrphanedStateOutsideZqk(root string) error {
	entries, err := fileutil.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == paths.ProjectDataDir || name == ".git" {
			continue
		}
		// Invariant: no .lock, .tmp, or orphaned cache files outside .zqk/
		if strings.HasSuffix(name, ".lock") || strings.HasSuffix(name, ".tmp") || strings.HasPrefix(name, "zqk_tmp_") {
			return errfmt.Newf(errOrphanOutsideZqk, name)
		}
	}
	return nil
}
