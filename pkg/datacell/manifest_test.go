package datacell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestReadRuntimeManifest_MissingOK(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	zqk := filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir)
	if err := os.MkdirAll(zqk, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	m, err := ReadRuntimeManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if m.ProtocolVersion != "" {
		t.Fatalf("expected empty manifest, got %#v", m)
	}
	if got := EffectiveProtocolVersion(m); got != ProtocolVersion {
		t.Fatalf("EffectiveProtocolVersion = %q want %q", got, ProtocolVersion)
	}
}

func TestReadRuntimeManifest_WithFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	zqk := filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir)
	if err := os.MkdirAll(zqk, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(zqk, paths.DataCellRuntimeManifestFile)
	if err := fileutil.WriteSecureFile(p, []byte(`{"protocol_version":"9"}`)); err != nil {
		t.Fatal(err)
	}
	m, err := ReadRuntimeManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if m.ProtocolVersion != "9" {
		t.Fatalf("ProtocolVersion = %q", m.ProtocolVersion)
	}
	if got := EffectiveProtocolVersion(m); got != "9" {
		t.Fatalf("EffectiveProtocolVersion = %q", got)
	}
}

func TestReadRuntimeManifest_InvalidJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	zqk := filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir)
	if err := os.MkdirAll(zqk, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(zqk, paths.DataCellRuntimeManifestFile)
	if err := fileutil.WriteSecureFile(p, []byte(`not json`)); err != nil {
		t.Fatal(err)
	}
	_, err := ReadRuntimeManifest(root)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEffectiveProtocolVersion_EmptyStringUsesConstant(t *testing.T) {
	t.Parallel()
	if got := EffectiveProtocolVersion(RuntimeManifest{ProtocolVersion: ""}); got != ProtocolVersion {
		t.Fatalf("got %q want %q", got, ProtocolVersion)
	}
}
