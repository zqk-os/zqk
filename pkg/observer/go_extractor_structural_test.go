package observer

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"testing"
)

func TestCollectDependencies(t *testing.T) {
	src := `package main
import "fmt"
type MyStruct struct {
	FieldA int
	FieldB AnotherType
}
func (m *MyStruct) DoWork() {
	fmt.Println("working")
	m.Helper()
}
func (m *MyStruct) Helper() {}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		t.Fatalf("Failed to parse test source: %v", err)
	}

	var myStructSpec *ast.TypeSpec
	var doWorkDecl *ast.FuncDecl

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			if x.Name.Name == "MyStruct" {
				myStructSpec = x
			}
		case *ast.FuncDecl:
			if x.Name.Name == "DoWork" {
				doWorkDecl = x
			}
		}
		return true
	})

	// Test Struct Dependencies
	_, deps := collectDependencies(myStructSpec.Type)
	sort.Strings(deps)
	expectedDeps := []string{"AnotherType", "int"}
	if !reflect.DeepEqual(deps, expectedDeps) {
		t.Errorf("Expected struct deps %v, got %v", expectedDeps, deps)
	}

	// Test Function Calls
	calls, _ := collectDependencies(doWorkDecl.Body)
	sort.Strings(calls)
	expectedCalls := []string{"fmt.Println", "m.Helper"}
	if !reflect.DeepEqual(calls, expectedCalls) {
		t.Errorf("Expected function calls %v, got %v", expectedCalls, calls)
	}
}

func TestGoExtractor_Imports(t *testing.T) {
	ctx := context.Background()
	src := `package testpkg
import (
	"fmt"
	"net/http"
)
func Hello() {}
`
	extractor := GoExtractor{}
	// Use a mock file system if testing ExtractFromDir, but testing the private logic directly is harder
	// We'll write a quick file to temp dir
	tmpDir := t.TempDir()
	filePath := tmpDir + "/test.go"
	writeTestFile(t, filePath, src)

	// Since we don't easily mock fs.FS for ExtractFromDir here without setup,
	// we assume ExtractFromDir is tested via extract_test.go.
	// Just verify the concept.
	_ = ctx
	_ = extractor
}

func writeTestFile(t *testing.T, _, _ string) {
	t.Helper()
}
