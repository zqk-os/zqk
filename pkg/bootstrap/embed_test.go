package bootstrap

import (
	"testing"
)

func TestReadEmbeddedFile(t *testing.T) {
	t.Parallel()

	// Test reading a scheduler job template
	data, err := ReadEmbeddedFile("scripts/scheduler_jobs/maintenance_wal_trigger.yaml")
	if err != nil {
		t.Fatalf("expected to read maintenance_wal_trigger.yaml from archive: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty data for maintenance_wal_trigger.yaml")
	}

	// Test reading with alternative path formats
	data2, err := ReadEmbeddedFile("scheduler_jobs/maintenance_wal_trigger.yaml")
	if err != nil {
		t.Fatalf("expected to read scheduler_jobs/maintenance_wal_trigger.yaml from archive: %v", err)
	}
	if len(data2) == 0 {
		t.Fatal("expected non-empty data for scheduler_jobs/maintenance_wal_trigger.yaml")
	}

	// Test reading with dot-slash prefix
	data3, err := ReadEmbeddedFile("./scripts/scheduler_jobs/maintenance_wal_trigger.yaml")
	if err != nil || len(data3) == 0 {
		t.Fatalf("expected to read with ./ prefix: %v", err)
	}

	// Test reading by suffix only
	data4, err := ReadEmbeddedFile("maintenance_wal_trigger.yaml")
	if err != nil || len(data4) == 0 {
		t.Fatalf("expected to read by suffix: %v", err)
	}

	// Test non-existent file
	_, err = ReadEmbeddedFile("non_existent_file_xyz.yaml")
	if err == nil {
		t.Fatal("expected error reading non-existent file")
	}

	// Test reading directory entry (should not match as regular file)
	_, err = ReadEmbeddedFile("scripts")
	if err == nil {
		t.Fatal("expected error reading directory entry as file")
	}

	// Test reading empty path
	_, err = ReadEmbeddedFile("")
	if err == nil {
		t.Fatal("expected error reading empty path")
	}
}

func TestManifestPaths_Properties(t *testing.T) {
	t.Parallel()
	paths := ManifestPaths()
	if paths == nil {
		t.Skip("no embedded manifest")
	}
	if len(paths) == 0 {
		t.Fatal("manifest paths should not be empty")
	}
	for _, p := range paths {
		if p == "" {
			t.Fatal("manifest should not contain empty entries")
		}
	}
}
