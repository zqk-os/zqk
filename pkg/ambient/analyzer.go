package ambient

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Prediction represents an anticipated need based on a filesystem event.
type Prediction struct {
	FilePath    string
	Type        string // e.g., "missing_test"
	Description string
}

// ASTAnalyzer inspects Go source files to determine contextual intent.
type ASTAnalyzer struct{}

// NewASTAnalyzer creates a new analyzer.
func NewASTAnalyzer() *ASTAnalyzer {
	return &ASTAnalyzer{}
}

// Analyze receives a file path (typically from fsnotify) and attempts to predict
// what the user will need next based on the file's Abstract Syntax Tree.
func (a *ASTAnalyzer) Analyze(filePath string) ([]Prediction, error) {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		// We currently only predict off of primary Go source files.
		return nil, nil
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		// If the file doesn't parse (e.g., user is halfway through typing),
		// we just silently ignore rather than flooding logs.
		return nil, nil
	}

	var predictions []Prediction

	// 1. Check for missing tests
	testFilePath := strings.TrimSuffix(filePath, ".go") + "_test.go"
	if _, err := fileutil.Stat(testFilePath); fileutil.IsNotExist(err) {
		// Only predict a missing test if there are actually functions to test
		hasFuncs := false
		ast.Inspect(node, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncDecl); ok {
				hasFuncs = true
				return false // stop traversing this branch
			}
			return true
		})

		if hasFuncs {
			predictions = append(predictions, Prediction{
				FilePath:    testFilePath,
				Type:        "missing_test",
				Description: "Predicted need for test file due to new functions in " + filepath.Base(filePath),
			})
		}
	}

	return predictions, nil
}
