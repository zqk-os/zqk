package cas_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// writeCASBlob writes content into kindDir under its own content hash, the way CAS names files.
func writeCASBlob(t *testing.T, kindDir, content string) {
	t.Helper()
	sum := sha256.Sum256([]byte(content))
	name := hex.EncodeToString(sum[:]) + ".yaml"
	if err := fileutil.WriteFile(filepath.Join(kindDir, name), []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestInventoryCASDuplicateIDs_reportsRegisteredKindNotDirectoryName covers the wiring the
// command-rendering guard in cmd/zqk/system cannot reach.
//
// The inventory is directory-driven: it walks process/<dir> and previously assigned the
// directory name to the hit's Kind field. Consumers then interpolated that into a remediation
// command whose kind argument only accepts registered kinds, producing
// `unknown kind "scheduler_jobs"` when an operator copy-pasted the suggested fix.
//
// This asserts on the inventory's own output, so reverting the resolution fails here even though the
// renderer stays correct in isolation.
func TestInventoryCASDuplicateIDs_reportsRegisteredKindNotDirectoryName(t *testing.T) {
	root := t.TempDir()
	// scheduler_jobs is chosen because its kind (scheduler_job) differs from the directory name, and
	// because it is the directory that produced the reported failure.
	const dir = "scheduler_jobs"
	kindDir := filepath.Join(root, paths.ProcessDir, dir)
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	const objectID = "SCH-run-test-duplicate-kind-probe"
	// Two distinct contents carrying the same id: that is exactly a POL-CODE-004 dual blob.
	writeCASBlob(t, kindDir, "id: "+objectID+"\nkind: scheduler_job\nstatus: active\n")
	writeCASBlob(t, kindDir, "id: "+objectID+"\nkind: scheduler_job\nstatus: archived\n")

	inv := caspkg.InventoryCASDuplicateIDs(context.Background(), root)
	if inv.DuplicateCount == 0 {
		t.Fatalf("expected a duplicate hit for %s; inventory found none", objectID)
	}

	var hit *caspkg.CASDuplicateIDHit
	for i := range inv.Hits {
		if inv.Hits[i].ObjectID == objectID {
			hit = &inv.Hits[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("duplicate count %d but no hit for %s", inv.DuplicateCount, objectID)
	}

	if hit.Dir != dir {
		t.Errorf("Dir = %q, want the scanned directory %q", hit.Dir, dir)
	}
	if hit.Kind == dir {
		t.Errorf("Kind = %q, which is the directory name, not a registered kind; "+
			"consumers interpolate Kind into a command that rejects directory names", hit.Kind)
	}
	if want := "scheduler_job"; hit.Kind != want {
		t.Errorf("Kind = %q, want %q", hit.Kind, want)
	}
}
