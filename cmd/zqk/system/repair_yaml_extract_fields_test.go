package system

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestExtractFieldsFromCorruptedYAML_BodyBlock_MetadataOK(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "bad.yaml")
	input := `kind: policy
id: POL-EXAMPLE-001
schema_version: "` + objects.DefaultSchemaVersion + `"
body:
  |
    line1
    line2
other_field: should_stop
`
	if err := fileutil.WriteFile(p, []byte(input), paths.FilePerm644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	instance, extractedKind, err := extractFieldsFromCorruptedYAML(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if extractedKind != "policy" {
		t.Fatalf("unexpected extractedKind=%q", extractedKind)
	}
	if instance[objects.FieldKeyID] != "POL-EXAMPLE-001" {
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
	input := `id: POL-EXAMPLE-001
kind: policy
schema_version: "` + objects.DefaultSchemaVersion + `"
bad: [unclosed
body:
  |
    x
    y
next: Z
`
	if err := fileutil.WriteFile(p, []byte(input), paths.FilePerm644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	instance, extractedKind, err := extractFieldsFromCorruptedYAML(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if extractedKind != "policy" {
		t.Fatalf("unexpected extractedKind=%q", extractedKind)
	}
	if instance[objects.FieldKeyID] != "POL-EXAMPLE-001" {
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
	input := `kind: policy
id: POL-EXAMPLE-001
schema_version: "` + objects.DefaultSchemaVersion + `"
`
	p := testkit.WriteTestObjectStandalone(t, tmp, input)

	instance, extractedKind, err := extractFieldsFromCorruptedYAML(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if extractedKind != "policy" {
		t.Fatalf("unexpected extractedKind=%q", extractedKind)
	}
	if instance[objects.FieldKeyID] != "POL-EXAMPLE-001" {
		t.Fatalf("unexpected id=%v", instance[objects.FieldKeyID])
	}
}

func TestExtractFieldsFromCorruptedYAML_NoBody_GitConflictMarkers(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "conflict.yaml")
	input := `id: BLI-CEF-LIST-REFS-FILTER-001
kind: backlog_item
schema_version: "` + objects.DefaultSchemaVersion + `"
status: validated
title: List filters
<<<<<<<< HEAD:.zqk/process/backlog/old.yaml
updated_at: "2026-08-30T09:21:20Z"
========
updated_at: "2026-08-30T10:16:38Z"
>>>>>>>> ee5c8939ce:.zqk/process/backlog/new.yaml
updated_by: ACC-TEST
`
	if err := fileutil.WriteFile(p, []byte(input), paths.FilePerm644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	instance, extractedKind, err := extractFieldsFromCorruptedYAML(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if extractedKind != "backlog_item" {
		t.Fatalf("unexpected extractedKind=%q", extractedKind)
	}
	if instance[objects.FieldKeyID] != "BLI-CEF-LIST-REFS-FILTER-001" {
		t.Fatalf("unexpected id=%v", instance[objects.FieldKeyID])
	}
	got, _ := instance[objects.FieldKeyUpdatedAt].(string)
	if got != "2026-08-30T10:16:38Z" {
		t.Fatalf("expected later conflict side updated_at, got %q", got)
	}
}
