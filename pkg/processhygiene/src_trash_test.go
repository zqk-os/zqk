package processhygiene_test

import (
	"go/scanner"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestNoSrcTrash(t *testing.T) {
	repoRoot := findRepoRoot(t)
	roots := []string{
		filepath.Join(repoRoot, "pkg"),
		filepath.Join(repoRoot, "cmd"),
		filepath.Join(repoRoot, "internal"),
	}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case paths.GitWorktreeMetadataEntry, "vendor", "node_modules",
					paths.StudioNestedWorktreesDir, paths.ProjectDataDir:
					return fs.SkipDir
				}
				return nil
			}
			if !isPossibleGoSrcTrash(d.Name()) {
				return nil
			}

			src, err := fileutil.ReadFile(path)
			if err != nil {
				t.Errorf("ReadFile %s: %v", path, err)
				return nil
			}

			fset := token.NewFileSet()
			file := fset.AddFile(path, fset.Base(), len(src))
			var s scanner.Scanner
			s.Init(file, src, nil, scanner.ScanComments)

			for {
				_, tok, _ := s.Scan()
				if tok == token.EOF {
					break
				}
				if tok == token.COMMENT {
					continue
				}
				if tok == token.PACKAGE {
					t.Errorf("found Go source code in non-.go file (possible src trash): %s", path)
				}
				break
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir failed on %s: %v", root, err)
		}
	}
}

func isPossibleGoSrcTrash(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".go") {
		return false
	}
	for _, ext := range []string{".bak", ".orig", ".rej", ".txt", ".patch"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return strings.Contains(lower, ".go.")
}
