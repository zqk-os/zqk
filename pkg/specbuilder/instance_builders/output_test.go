package instance_builders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestInstanceBuilderOutput_siblingOfToolDir(t *testing.T) {
	toolDir := DefaultInstanceBuilderToolDir
	dir, packageName := InstanceBuilderOutput(toolDir)
	wantDir := filepath.Join("pkg", "specbuilder", generatedInstancePackage)
	if dir != wantDir || packageName != generatedInstancePackage {
		t.Fatalf("kernel tool dir: dir=%q package=%q", dir, packageName)
	}

	packDir := filepath.Join("packs", "work", "instance_builders")
	dir, packageName = InstanceBuilderOutput(packDir)
	wantDir = filepath.Join("packs", "work", generatedInstancePackage)
	if dir != wantDir || packageName != generatedInstancePackage {
		t.Fatalf("pack tool dir: dir=%q package=%q", dir, packageName)
	}
	if got := EnumOutput(packDir); got != filepath.Join("packs", "work", generatedEnumPackage) {
		t.Fatalf("pack enum root: %q", got)
	}
	if got := EnumImportBase(toolDir); got != "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1" {
		t.Fatalf("kernel enum import: %q", got)
	}
	if got := EnumImportBase(packDir); got != "github.com/zqk-os/zqk/packs/work/bldr_enum_v1" {
		t.Fatalf("pack enum import: %q", got)
	}
	scratch := filepath.Join(string(filepath.Separator), "tmp", "spec-cell", "instance_builders")
	if got := EnumImportBase(scratch); got != "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1" {
		t.Fatalf("scratch enum import: %q", got)
	}
}

func TestGenerateInstanceBuilderCode_packEnumImport(t *testing.T) {
	spec := &objects.Spec{
		Ontology: "widget",
		Fields: map[string]any{
			"tone": map[string]any{
				objects.FieldKeyType: "enum",
				"validation": map[string]any{
					"enum": []any{"low", "high"},
				},
			},
		},
	}
	packDir := filepath.Join("packs", "work", "instance_builders")
	code, err := generateInstanceBuilderCode(spec, "widget", nil, generatedInstancePackage, EnumImportBase(packDir))
	if err != nil {
		t.Fatal(err)
	}
	want := "enumv \"github.com/zqk-os/zqk/packs/work/bldr_enum_v1/widget\""
	if !strings.Contains(code, want) {
		t.Fatalf("missing pack enum import")
	}
	if strings.Contains(code, "RegisterBuilder") {
		t.Fatal("generated builder registers on the kernel singleton")
	}
}

func TestGenerateInstanceBuilderFromSpec_writesSiblingWithoutRegistry(t *testing.T) {
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "widget.yaml")
	specYAML := "schema_version: \"1.0.0\"\nontology: widget\nfields:\n  title:\n    type: string\n"
	if err := os.WriteFile(specPath, []byte(specYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	toolDir := filepath.Join(tmp, "instance_builders")
	if err := GenerateInstanceBuilderFromSpec(specPath, toolDir, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, generatedInstancePackage, "widget_instance_builder.go")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if !strings.Contains(src, "package "+generatedInstancePackage) {
		t.Fatal("written file is not the generated instance package")
	}
	if strings.Contains(src, "RegisterBuilder") {
		t.Fatal("written builder registers on the kernel singleton")
	}
}
