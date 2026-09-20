package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestMigrateStreamSegmentFilenamesToCanonical_IdempotentWhenNoLegacy(t *testing.T) {
	dir := t.TempDir()
	streamsDir := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir)
	if err := fileutil.EnsureDir(filepath.Join(streamsDir, "audit_event")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Already canonical: 2030-03-01_stream.json
	if err := fileutil.WriteSecureFile(filepath.Join(streamsDir, "audit_event", "2030-03-01_stream.json"), []byte(`{}`+"\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	moved, kinds, err := MigrateStreamSegmentFilenamesToCanonical(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if moved != 0 || len(kinds) != 0 {
		t.Errorf("expected no-op when no legacy names: moved=%d kinds=%v", moved, kinds)
	}
}

func TestMigrateStreamSegmentFilenamesToCanonical_RenamesAndRewritesRegistry(t *testing.T) {
	dir := t.TempDir()
	streamsDir := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir)
	stateDir := filepath.Join(dir, paths.ProjectDataDir, paths.StateDir)
	kind := "scheduler_job"
	kindDir := filepath.Join(streamsDir, kind)
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatalf("mkdir kind: %v", err)
	}
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	legacyFile := filepath.Join(kindDir, "scheduler_job_2030-03-01.jsonl")
	if err := fileutil.WriteSecureFile(legacyFile, []byte(`{"id":"SCH-1"}`+"\n")); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	legacyAbs, _ := filepath.Abs(legacyFile)
	registryPath := filepath.Join(stateDir, streamRegistryPrefix+kind+streamRegistrySuffix)
	line := `{"id":"SCH-1","loc":"` + legacyAbs + `::0"}` + "\n"
	if err := fileutil.WriteSecureFile(registryPath, []byte(line)); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	moved, kindsUpdated, err := MigrateStreamSegmentFilenamesToCanonical(dir)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if moved != 1 {
		t.Errorf("expected moved=1, got %d", moved)
	}
	if len(kindsUpdated) != 1 || kindsUpdated[0] != kind {
		t.Errorf("expected kinds_updated=[scheduler_job], got %v", kindsUpdated)
	}

	canonicalFile := filepath.Join(kindDir, "2030-03-01_stream.json")
	if _, err := fileutil.Stat(canonicalFile); err != nil {
		t.Errorf("canonical segment file missing: %v", err)
	}
	if _, err := fileutil.Stat(legacyFile); err == nil {
		t.Error("legacy file should be removed")
	}
	// Registry should now point at canonical path
	data, _ := fileutil.ReadFile(registryPath)
	if len(data) == 0 {
		t.Fatal("registry empty")
	}
	if !strings.Contains(string(data), "2030-03-01_stream.json") {
		t.Errorf("registry should contain new path; got %s", string(data))
	}
}
