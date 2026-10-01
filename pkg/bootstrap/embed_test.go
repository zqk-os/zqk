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

	// Test non-existent file
	_, err = ReadEmbeddedFile("non_existent_file_xyz.yaml")
	if err == nil {
		t.Fatal("expected error reading non-existent file")
	}
}
