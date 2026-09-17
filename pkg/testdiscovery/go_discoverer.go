package testdiscovery

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// GoDiscoverer statically discovers tests in Go source files using AST parsing.
type GoDiscoverer struct{}

func NewGoDiscoverer() *GoDiscoverer {
	return &GoDiscoverer{}
}

func (d *GoDiscoverer) Language() string {
	return "go"
}

func (d *GoDiscoverer) CanHandle(relPath string) bool {
	return strings.HasSuffix(relPath, "_test.go")
}

var (
	critRegex = regexp.MustCompile(`(?i)(?:criteria|validates|crit):\s*([A-Za-z0-9_\-,\s]+)`)
	reqRegex  = regexp.MustCompile(`(?i)(?:requirement|req):\s*([A-Za-z0-9_\-,\s]+)`)
	tagRegex  = regexp.MustCompile(`(?i)(?:scenario|tag):\s*([^\n\r]+)`)
)

func (d *GoDiscoverer) Discover(ctx context.Context, projectRoot, relPath string, content []byte) ([]DiscoveredTarget, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filepath.Base(relPath), content, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// 1. Extract build tags from file comments
	var fileTags []string
	for _, commentGroup := range node.Comments {
		for _, c := range commentGroup.List {
			text := strings.TrimSpace(c.Text)
			if strings.HasPrefix(text, "//go:build ") {
				tags := strings.TrimSpace(strings.TrimPrefix(text, "//go:build "))
				fileTags = append(fileTags, strings.Fields(tags)...)
			} else if strings.HasPrefix(text, "// +build ") {
				tags := strings.TrimSpace(strings.TrimPrefix(text, "// +build "))
				fileTags = append(fileTags, strings.Fields(tags)...)
			}
		}
	}

	pkgName := node.Name.Name
	relDir := filepath.Dir(relPath)
	if relDir == "." {
		relDir = ""
	}

	var targets []DiscoveredTarget

	// 2. Iterate AST declarations
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}

		name := fn.Name.Name
		isTest := strings.HasPrefix(name, "Test")
		isBenchmark := strings.HasPrefix(name, "Benchmark")
		isExample := strings.HasPrefix(name, "Example")

		if !isTest && !isBenchmark && !isExample {
			continue
		}

		line := fset.Position(fn.Pos()).Line

		// Extract doc comment metadata
		var criteriaRefs []string
		var reqRefs []string
		var fnTags []string
		fnTags = append(fnTags, fileTags...)

		if fn.Doc != nil {
			for _, comment := range fn.Doc.List {
				txt := comment.Text
				if m := critRegex.FindStringSubmatch(txt); len(m) > 1 {
					parts := strings.Split(m[1], ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							criteriaRefs = append(criteriaRefs, p)
						}
					}
				}
				if m := reqRegex.FindStringSubmatch(txt); len(m) > 1 {
					parts := strings.Split(m[1], ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							reqRefs = append(reqRefs, p)
						}
					}
				}
				if m := tagRegex.FindStringSubmatch(txt); len(m) > 1 {
					fnTags = append(fnTags, strings.TrimSpace(m[1]))
				}
			}
		}

		pkgTarget := "./" + relDir
		if relDir == "" {
			pkgTarget = "."
		}

		execCmd := fmt.Sprintf("go test -run '^%s$' %s", name, pkgTarget)
		if isBenchmark {
			execCmd = fmt.Sprintf("go test -bench '^%s$' -run '^$' %s", name, pkgTarget)
		}

		target := DiscoveredTarget{
			Path:             relPath,
			Language:         "go",
			Package:          pkgName,
			Function:         name,
			Line:             line,
			Tags:             fnTags,
			CriteriaRefs:     criteriaRefs,
			RequirementRefs:  reqRefs,
			ExecutionCommand: execCmd,
		}

		targets = append(targets, target)
	}

	return targets, nil
}
