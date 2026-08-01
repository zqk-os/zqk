package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestExtractFieldsFromCorruptedYAML_BodyBlock_MetadataOK(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "bad.yaml")
	input := `kind: policy
id: POLICY-CODE-009
schema_version: "` + objects.DefaultSchemaVersion + `"
body:
  |
    line1
    line2
other_field: should_stop
`
	if err := os.WriteFile(p, []byte(input), paths.FilePerm644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	instance, extractedKind, err := extractFieldsFromCorruptedYAML(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if extractedKind != "policy" {
		t.Fatalf("unexpected extractedKind=%q", extractedKind)
	}
	if instance[objects.FieldKeyID] != "POLICY-CODE-009" {
		t.Fatalf("unexpected id=%v", instance[objects.FieldKeyID])
	}
	if instance[objects.FieldKeySchemaVersion] != objects.DefaultSchemaVersion {
		t.Fatalf("unexpected schema_version=%v", instance[objects.FieldKeySchemaVersion])
	}

	wantBody := "    line1\n    line2"
	if instance[objects.FieldKeyBody] != wantBody {
		t.Fatalf("unexpected body=%q want %q", instance[objects.FieldKeyBody], wantBody)
	}
}

func TestExtractFieldsFromCorruptedYAML_BodyBlock_MetadataFallback(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "bad.yaml")
	input := `id: POLICY-CODE-009
kind: policy
schema_version: "` + objects.DefaultSchemaVersion + `"
bad: [unclosed
body:
  |
    x
    y
next: Z
`
	if err := os.WriteFile(p, []byte(input), paths.FilePerm644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	instance, extractedKind, err := extractFieldsFromCorruptedYAML(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if extractedKind != "policy" {
		t.Fatalf("unexpected extractedKind=%q", extractedKind)
	}
	if instance[objects.FieldKeyID] != "POLICY-CODE-009" {
		t.Fatalf("unexpected id=%v", instance[objects.FieldKeyID])
	}
	if instance[objects.FieldKeySchemaVersion] != objects.DefaultSchemaVersion {
		t.Fatalf("unexpected schema_version=%v", instance[objects.FieldKeySchemaVersion])
	}
	wantBody := "    x\n    y"
	if instance[objects.FieldKeyBody] != wantBody {
		t.Fatalf("unexpected body=%q want %q", instance[objects.FieldKeyBody], wantBody)
	}
}

func TestExtractFieldsFromCorruptedYAML_NoBody_ParsesNormally(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "ok-but-calls-fn.yaml")
	input := `kind: policy
id: POLICY-CODE-009
schema_version: "` + objects.DefaultSchemaVersion + `"
`
	if err := os.WriteFile(p, []byte(input), paths.FilePerm644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	instance, extractedKind, err := extractFieldsFromCorruptedYAML(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if extractedKind != "policy" {
		t.Fatalf("unexpected extractedKind=%q", extractedKind)
	}
	if instance[objects.FieldKeyID] != "POLICY-CODE-009" {
		t.Fatalf("unexpected id=%v", instance[objects.FieldKeyID])
	}
}
