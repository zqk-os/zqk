package storage

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestRegistryShardingIntegration(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "zqk_sharding_test")
	if err != nil {
		t.Fatal(err)
	}
	defer fileutil.RemoveAll(tempDir)

	projectRoot := tempDir
	kind := "testkind"
	id := "BLI-123456"
	loc := "segment::100"

	// Create required directories
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	err = fileutil.EnsureDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	// Write and read via sharded path
	err = AppendStreamLocationToRegistry(projectRoot, kind, id, loc)
	if err != nil {
		t.Fatal(err)
	}

	// Verify read
	foundLoc := getStreamLocationFromPersistentRegistry(projectRoot, kind, id)
	if foundLoc != loc {
		t.Fatalf("expected loc %q, got %q", loc, foundLoc)
	}

	// Verify list
	ids := ListStreamIDsFromPersistentRegistry(projectRoot, kind)
	found := false
	for _, rid := range ids {
		if rid == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ID %q not found in registry list", id)
	}
}
