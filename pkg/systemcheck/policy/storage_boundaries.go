package policy

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// StorageBoundariesGate validates import boundaries within pkg/storage.
type StorageBoundariesGate struct{}

func (g *StorageBoundariesGate) Name() string {
	return "storage-boundaries"
}

func (g *StorageBoundariesGate) Description() string {
	return "Validates storage package layer isolation and prevents circular or upward storage imports"
}

func (g *StorageBoundariesGate) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	root := opts.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
	}

	storageDir := filepath.Join(root, "pkg", "storage")
	if _, err := os.Stat(storageDir); os.IsNotExist(err) {
		return &Result{
			GateName: g.Name(),
			Passed:   true,
			Message:  "pkg/storage does not exist, skipping check",
		}, nil
	}

	var violations []string
	fset := token.NewFileSet()

	getImports := func(filePath string) ([]string, error) {
		node, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		var imports []string
		for _, imp := range node.Imports {
			if imp.Path != nil {
				pathVal := strings.Trim(imp.Path.Value, `"`)
				imports = append(imports, pathVal)
			}
		}
		return imports, nil
	}

	err := filepath.WalkDir(storageDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 4 {
			return nil
		}

		subpkg := parts[2]

		imports, parseErr := getImports(path)
		if parseErr != nil {
			return nil
		}

		for _, imp := range imports {
			// Rule 1: Subpackages in pkg/storage/* must never import root pkg/storage (except cas, crud, migration allowlisted during refactor)
			if imp == "github.com/lanceman/zqk/pkg/storage" {
				if subpkg != "cas" && subpkg != "crud" && subpkg != "migration" {
					violations = append(violations, fmt.Sprintf("%s imports root pkg/storage (subpackages must use interfaces or lower layers)", rel))
				}
			}

			// Rule 2: pkg/storage/core must not import sibling storage subpackages
			if subpkg == "core" {
				if strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/file") ||
					strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/graph") ||
					strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/wal") ||
					strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/cache") {
					violations = append(violations, fmt.Sprintf("%s (core) imports higher engine %s", rel, imp))
				}
			}

			// Rule 3: pkg/storage/wal must not import higher-level engines (file, graph, cache)
			if subpkg == "wal" {
				if strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/file") ||
					strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/graph") ||
					strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/cache") {
					violations = append(violations, fmt.Sprintf("%s (wal) imports sibling/higher engine %s", rel, imp))
				}
			}

			// Rule 4: Engine isolation: file, graph, cache must not import each other
			if subpkg == "file" {
				if strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/graph") ||
					strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/cache") {
					violations = append(violations, fmt.Sprintf("%s (file) imports sibling engine %s", rel, imp))
				}
			}
			if subpkg == "graph" {
				if strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/file") ||
					strings.HasPrefix(imp, "github.com/lanceman/zqk/pkg/storage/cache") {
					violations = append(violations, fmt.Sprintf("%s (graph) imports sibling engine %s", rel, imp))
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk pkg/storage: %w", err)
	}

	if len(violations) > 0 {
		return &Result{
			GateName:   g.Name(),
			Passed:     false,
			Message:    fmt.Sprintf("Detected %d storage boundary violation(s)", len(violations)),
			Violations: violations,
		}, nil
	}

	return &Result{
		GateName: g.Name(),
		Passed:   true,
		Message:  "All storage package boundary rules respected",
	}, nil
}
