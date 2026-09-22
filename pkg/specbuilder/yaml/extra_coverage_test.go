// BLI-STARTER-COMMUNITY-052 / PRI-STARTER-COMMUNITY-052 coverage elevation
package yaml

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type extraSpec struct {
	Name string `yaml:"name"`
	Fail bool   `yaml:"fail,omitempty"`
}

func (s extraSpec) Validate() error {
	if s.Fail {
		return errFail("fail")
	}
	if s.Name == "" {
		return errFail("empty")
	}
	return nil
}
func (s extraSpec) GetName() string { return s.Name }

type errFail string

func (e errFail) Error() string { return string(e) }

func TestExtraWriterLoaderAndFind(t *testing.T) {
	w := NewYAMLWriter[extraSpec]()
	w.SetIndent(4)
	var buf bytes.Buffer
	if err := w.Write(extraSpec{Name: "one"}, &buf); err != nil {
		t.Fatal(err)
	}
	s, err := w.WriteToString(extraSpec{Name: "two"})
	if err != nil || s == "" {
		t.Fatal(err)
	}
	b, err := w.WriteToBytes(extraSpec{Name: "three"})
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fp := filepath.Join(dir, "nested", "spec.yaml")
	if err := w.WriteToFile(extraSpec{Name: "file"}, fp); err != nil {
		t.Fatal(err)
	}

	loader := NewYAMLSpecLoader[extraSpec](dir).WithSchemaValidation(false)
	got, err := loader.LoadSpec(fp)
	if err != nil || got.Name != "file" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := loader.LoadSpec(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("missing")
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := fileutil.WriteFile(bad, []byte(": :"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.LoadSpec(bad); err == nil {
		t.Fatal("parse")
	}
	failPath := filepath.Join(dir, "fail.yaml")
	if err := fileutil.WriteFile(failPath, []byte("name: x\nfail: true\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.LoadSpec(failPath); err == nil {
		t.Fatal("validate")
	}

	listPath := filepath.Join(dir, "list.yaml")
	if err := fileutil.WriteFile(listPath, []byte("- name: a\n- name: b\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	specs, err := loader.LoadSpecs(listPath)
	if err != nil || len(specs) != 2 {
		t.Fatalf("%v %v", specs, err)
	}
	if _, err := loader.LoadSpecs(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("missing list")
	}
	failList := filepath.Join(dir, "faillist.yaml")
	if err := fileutil.WriteFile(failList, []byte("- name: a\n  fail: true\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.LoadSpecs(failList); err == nil {
		t.Fatal("list validate")
	}

	wrap := filepath.Join(dir, "wrap.yaml")
	if err := fileutil.WriteFile(wrap, []byte("specs:\n  - name: a\n  - name: b\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	wrapped, err := loader.LoadSpecList(wrap, "specs")
	if err != nil || len(wrapped) != 2 {
		t.Fatalf("%v %v", wrapped, err)
	}
	if _, err := loader.LoadSpecList(wrap, "missing"); err == nil {
		t.Fatal("key")
	}
	if _, err := loader.LoadSpecList(filepath.Join(dir, "missing.yaml"), "specs"); err == nil {
		t.Fatal("missing wrap")
	}
	if _, err := loader.LoadSpecList(bad, "specs"); err == nil {
		t.Fatal("bad wrap")
	}

	_, _ = LoadYAMLSpec[extraSpec](fp)
	_, _ = LoadYAMLSpecs[extraSpec](listPath)
	_, _ = LoadYAMLSpecList[extraSpec](wrap, "specs")

	loader.WithSchemaValidation(true).WithSchemaValidator(NewSchemaValidator(dir))
	_, _ = loader.LoadSpec(fp)
	_, _ = loader.LoadSpecs(listPath)
	_, _ = loader.LoadSpecList(wrap, "specs")
	_ = NewYAMLSpecLoader[extraSpec](dir).getSchemaValidator()
	_ = NewYAMLSpecLoader[extraSpec]("").getSchemaValidator()

	found, err := FindSpecFiles(dir, "")
	if err != nil || len(found) == 0 {
		t.Fatal(err)
	}
	_, _ = FindSpecFiles(dir, "*.yaml")
	_, _ = FindSpecFiles(filepath.Join(dir, "nope"), "*.yaml")
	if err := fileutil.WriteFile(filepath.Join(dir, "._skip.yaml"), []byte("name: x\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_, _ = FindSpecFiles(dir, "*.yaml")

	sv := NewSchemaValidator(dir)
	_ = sv.ValidateYAMLWithAutoSchema(fp)
	_ = sv.ValidateYAML(fp, "")
	_ = sv.ValidateYAML(filepath.Join(dir, "missing.yaml"), "")
	schemaJSON := []byte(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"name":{"type":"string"}}}`)
	schema := filepath.Join(dir, "s.json")
	if err := fileutil.WriteFile(schema, schemaJSON, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	named := filepath.Join(dir, "object_spec.schema.json")
	if err := fileutil.WriteFile(named, schemaJSON, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := sv.ValidateYAML(fp, schema); err != nil {
		t.Fatal(err)
	}
	_, _ = sv.resolveSchemaPath(schema)
	_, _ = sv.resolveSchemaPath("https://zqk.dev/schemas/object_spec.schema.json")
	_, _ = sv.resolveSchemaPath("http://zqk.dev/schemas/missing.schema.json")
	_, _ = sv.resolveSchemaPath("object_spec.schema.json")
	_, _ = sv.resolveSchemaPath("nested/nope.json")
	_, _ = sv.loadSchema(schema)
	_, _ = sv.loadSchema(schema)
	_, _ = sv.loadSchema(filepath.Join(dir, "missing.json"))
	auto := filepath.Join(dir, "auto.yaml")
	if err := fileutil.WriteFile(auto, []byte("$schema: object_spec.schema.json\nname: x\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_ = sv.ValidateYAMLWithAutoSchema(auto)
	if err := fileutil.WriteFile(filepath.Join(dir, "name_def.json"), []byte(`{"type":"string"}`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	refSchema := filepath.Join(dir, "ref.json")
	if err := fileutil.WriteFile(refSchema, []byte(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"name":{"$ref":"name_def.json"}}}`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_ = sv.ValidateYAML(fp, refSchema)
	_ = sv.ValidateYAML(fp, "https://example.com/nope.json")
	badSchema := filepath.Join(dir, "bad.json")
	if err := fileutil.WriteFile(badSchema, []byte(`{`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_, _ = sv.loadSchema(badSchema)
}
