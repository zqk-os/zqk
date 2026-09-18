package codegen_test

// CodegenBoundaryIsolation enforces compilation and AST quarantine.
// Reference: BLI-CELLULAR-CODEGEN-QUARANTINE-004

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/internal/codegen/ast"
	"github.com/zqk-os/zqk/internal/codegen/generators"
	"github.com/zqk-os/zqk/internal/codegen/macro"
	"github.com/zqk-os/zqk/pkg/dna"
)

func TestASTInspectionAndEmbedding(t *testing.T) {
	sampleSource := `package sample

import "github.com/zqk-os/zqk/pkg/dna"

type SampleItem struct {
	dna.BaseObject
	dna.Auditable
	dna.Lifecycle
	Title string
}
`
	inspector := ast.NewASTInspector()
	node, err := inspector.ParseSource("sample.go", []byte(sampleSource))
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	structs := inspector.FindStructNames(node)
	if len(structs) != 1 || structs[0] != "SampleItem" {
		t.Fatalf("expected struct SampleItem, got: %v", structs)
	}

	if !inspector.HasEmbeddedType(node, "SampleItem", "BaseObject") {
		t.Errorf("expected BaseObject to be embedded")
	}
	if !inspector.HasEmbeddedType(node, "SampleItem", "Auditable") {
		t.Errorf("expected Auditable to be embedded")
	}
	if !inspector.HasEmbeddedType(node, "SampleItem", "Lifecycle") {
		t.Errorf("expected Lifecycle to be embedded")
	}
	if inspector.HasEmbeddedType(node, "SampleItem", "NonExistent") {
		t.Errorf("expected NonExistent not to be embedded")
	}
}

func TestEntityGeneratorAndMacroExpander(t *testing.T) {
	schema := &dna.MetaSchema{
		TargetKind: "incident_ticket",
		Fields: map[string]dna.FieldMetaSpec{
			"severity": {
				Name:     "severity",
				Type:     dna.TypeString,
				Required: true,
			},
			"affected_urn": {
				Name:     "affected_urn",
				Type:     dna.TypeURN,
				Required: true,
			},
		},
	}

	expander := macro.NewSchemaMacroExpander()
	spec, err := expander.ExpandMetaSchema("incident", schema)
	if err != nil {
		t.Fatalf("ExpandMetaSchema failed: %v", err)
	}

	gen := generators.NewEntityGenerator()
	code, err := gen.GenerateEntityCode(*spec)
	if err != nil {
		t.Fatalf("GenerateEntityCode failed: %v", err)
	}

	codeStr := string(code)
	if !strings.Contains(codeStr, "type IncidentTicket struct") {
		t.Errorf("expected struct declaration in generated code")
	}
	if !strings.Contains(codeStr, "dna.BaseObject") {
		t.Errorf("expected dna.BaseObject embedded")
	}
	if !strings.Contains(codeStr, "func NewIncidentTicket") {
		t.Errorf("expected constructor in generated code")
	}

	// Verify the generated code is parseable by AST inspector
	inspector := ast.NewASTInspector()
	node, err := inspector.ParseSource("incident.go", code)
	if err != nil {
		t.Fatalf("generated code failed to parse: %v", err)
	}
	if !inspector.HasEmbeddedType(node, "IncidentTicket", "BaseObject") {
		t.Errorf("generated code does not embed BaseObject")
	}
}

func TestQuarantineImportBanForDNAAndKernel(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../.."))

	bannedImports := []string{
		"github.com/zqk-os/zqk/internal/codegen",
		"go/ast",
		"go/parser",
		"go/token",
	}

	targetDirs := []string{
		filepath.Join(repoRoot, "pkg/dna"),
		filepath.Join(repoRoot, "pkg/kernel"),
	}

	fset := token.NewFileSet()

	for _, dir := range targetDirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}

			// Don't ban testing imports in test files unless checking internal/codegen
			isTestFile := strings.HasSuffix(path, "_test.go")

			f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}

			for _, imp := range f.Imports {
				impPath := strings.Trim(imp.Path.Value, `"`)
				for _, banned := range bannedImports {
					// Pure runtime packages must NEVER import codegen
					if strings.HasPrefix(impPath, "github.com/zqk-os/zqk/internal/codegen") {
						t.Errorf("QUARANTINE VIOLATION: %s imports banned package %s", path, impPath)
					}
					// Non-test files must never import compiler internals
					if !isTestFile && (banned == "go/ast" || banned == "go/parser" || banned == "go/token") {
						if impPath == banned {
							t.Errorf("QUARANTINE VIOLATION: runtime file %s imports compiler internals %s", path, impPath)
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir failed on %s: %v", dir, err)
		}
	}
}

func TestCodegenBoundaryIsolation(t *testing.T) {
	TestQuarantineImportBanForDNAAndKernel(t)
}

