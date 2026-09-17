package storage

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestMigrateAuditStreamToCanonicalLocation_IdempotentWhenNoLegacy(t *testing.T) {
	dir := t.TempDir()
	moved, updated, err := MigrateAuditStreamToCanonicalLocation(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if moved != 0 || updated {
		t.Errorf("expected no-op when legacy dir missing: moved=%d updated=%v", moved, updated)
	}
}

func TestMigrateAuditStreamToCanonicalLocation_MovesAndRewritesRegistry(t *testing.T) {
	dir := t.TempDir()
	legacyDir := filepath.Join(dir, paths.ProjectDataDir, paths.AuditStreamsDir)
	stateDir := filepath.Join(dir, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.EnsureDir(legacyDir); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	// Create one legacy segment file
	legacyFile := filepath.Join(legacyDir, "audit_stream_2030-03-01.jsonl")
	if err := fileutil.WriteSecureFile(legacyFile, []byte(`{"id":"AUD-1"}`+"\n")); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	// Create registry with one entry pointing at legacy path
	registryPath := filepath.Join(stateDir, streamRegistryPrefix+auditEventKind+streamRegistrySuffix)
	legacyAbs, _ := filepath.Abs(legacyFile)
	line := `{"id":"AUD-1","loc":"` + legacyAbs + `::0"}` + "\n"
	if err := fileutil.WriteSecureFile(registryPath, []byte(line)); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	moved, registryUpdated, err := MigrateAuditStreamToCanonicalLocation(dir)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if moved != 1 {
		t.Errorf("expected moved=1, got %d", moved)
	}
	if !registryUpdated {
		t.Error("expected registry_updated=true")
	}

	canonicalDir := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, auditEventKind)
	canonicalFile := filepath.Join(canonicalDir, "2030-03-01_stream.json")
	if _, err := fileutil.Stat(canonicalFile); err != nil {
		t.Errorf("canonical segment file missing: %v", err)
	}
	if _, err := fileutil.Stat(legacyFile); err == nil {
		t.Error("legacy file should be removed")
	}
}
