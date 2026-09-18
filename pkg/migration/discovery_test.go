package migration

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestListSpecs_NoMigrationsDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	specs, err := ListSpecs(root)
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("expected no specs, got %d", len(specs))
	}
}

func TestListSpecs_EmptyDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(datacell.CellCASPrimaryDir(root, "_internal"), MigrationsDirName)
	if err := fileutil.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	specs, err := ListSpecs(root)
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("expected no specs, got %d", len(specs))
	}
}

func TestListSpecs_ValidSpec(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(datacell.CellCASPrimaryDir(root, "_internal"), MigrationsDirName)
	if err := fileutil.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "minimal.yaml")
	const yaml = `
id: minimal-mig
name: Minimal Migration
description: Minimal valid spec for discovery
from:
  state: a
to:
  state: b
steps:
  - id: s1
    type: scan_files
    config:
      path: docs
`
	if err := fileutil.WriteSecureFile(path, []byte(yaml)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	specs, err := ListSpecs(root)
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	s := specs[0]
	if s.ID != "minimal-mig" || s.Name != "Minimal Migration" || s.Description != "Minimal valid spec for discovery" {
		t.Errorf("spec: id=%q name=%q desc=%q", s.ID, s.Name, s.Description)
	}
	if s.Path != path {
		t.Errorf("path: got %q", s.Path)
	}
}
