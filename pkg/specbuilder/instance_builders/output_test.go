package instance_builders

import (
	"os"
	"os/exec"
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
	absPack := filepath.Join(moduleRoot(t), "packs", "work", "instance_builders")
	if got := EnumImportBase(absPack); got != "github.com/zqk-os/zqk/packs/work/bldr_enum_v1" {
		t.Fatalf("absolute pack enum import: %q", got)
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

func TestCollectEnumDefinitions_aliasUsesEnumImportBase(t *testing.T) {
	spec := &objects.Spec{
		Ontology: "widget",
		Fields: map[string]any{
			"tone": map[string]any{
				objects.FieldKeyType: enumCodegenFieldTypeEnum,
				enumCodegenFieldKeyValidation: map[string]any{
					enumCodegenFieldKeyEnum: []any{"low", "high"},
				},
			},
		},
	}
	owners := map[string]string{"tone": "parent"}
	packDir := filepath.Join("packs", "work", "instance_builders")
	defs := collectEnumDefinitions(spec, "widget", owners, nil, packDir)
	got := aliasImportForField(defs, "tone")
	want := "github.com/zqk-os/zqk/packs/work/bldr_enum_v1/parent"
	if got != want {
		t.Fatalf("pack alias import %q", got)
	}
	defs = collectEnumDefinitions(spec, "widget", owners, nil, DefaultInstanceBuilderToolDir)
	got = aliasImportForField(defs, "tone")
	want = "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/parent"
	if got != want {
		t.Fatalf("kernel alias import %q", got)
	}
	owners = map[string]string{"tone": "account"}
	defs = collectEnumDefinitions(spec, "widget", owners, nil, packDir)
	got = aliasImportForField(defs, "tone")
	want = "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/account"
	if got != want {
		t.Fatalf("kernel owner alias import %q", got)
	}
}

func aliasImportForField(defs []enumSpec, field string) string {
	for _, def := range defs {
		if def.FieldName == field {
			return def.AliasImportPkg
		}
	}
	return ""
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
	if !strings.Contains(src, "instance_builders.FieldOrderFromSpec(") {
		t.Fatal("written builder does not take field order from the spec")
	}
	if strings.Contains(src, "buildFieldOrderFromSpec") {
		t.Fatal("written builder depends on a helper that lives only in the kernel package")
	}
}

func TestGeneratedSiblingCompiles(t *testing.T) {
	root := moduleRoot(t)
	packRoot := filepath.Join(root, "packs", "work")
	t.Cleanup(func() { _ = os.RemoveAll(packRoot) })
	specPath := filepath.Join(packRoot, "widget.yaml")
	if err := os.MkdirAll(packRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	specYAML := "schema_version: \"1.0.0\"\nontology: widget\nfields:\n  title:\n    type: string\n"
	if err := os.WriteFile(specPath, []byte(specYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	toolDir := filepath.Join(packRoot, "instance_builders")
	if err := GenerateInstanceBuilderFromSpec(specPath, toolDir, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./packs/work/"+generatedInstancePackage)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated pack builder did not compile: %v\n%s", err, out)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
