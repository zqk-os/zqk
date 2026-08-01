package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

const testCASHash64 = "abc1230000000000000000000000000000000000000000000000000000000000"

// TestRecoverCASKind_ReturnsWhenContextCanceled verifies that RecoverCASKind
// returns ctx.Err() when the context is cancelled mid-loop (SCH-002 fix: recovery
// must respect context so job timeout aborts long runs).
func TestRecoverCASKind_ReturnsWhenContextCanceled(t *testing.T) {
	projectRoot := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	dirName := objects.GetDirectoryFromKind("audit_event")
	if dirName == emptyValue {
		dirName = "audit"
	}
	kindDir := filepath.Join(processDir, dirName)
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	// Create an index with 1001 mappings so the loop hits the abort check at n=1000.
	indexPath := filepath.Join(kindDir, ".audit_event.index")
	mappings := make(map[string]string, 1001)
	for i := 1; i <= 1001; i++ {
		mappings[fmt.Sprintf("AUD-%d", i)] = testCASHash64
	}
	indexData := map[string]any{
		objects.FieldKeyVersion: "1.0",
		objects.FieldKeyKind:    "audit_event",
		"mappings":              mappings,
	}
	data, err := json.MarshalIndent(indexData, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, data, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// Create dummy hash file once since all 1001 entries share the same testCASHash64
	dummyHashFile := filepath.Join(kindDir, testCASHash64+".yaml")
	if err := os.WriteFile(dummyHashFile, []byte("{}"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled so first abort check at n=1000 returns
	results, err := RecoverCASKind(ctx, projectRoot, "audit_event", DefaultCASRecoveryOptions(nil), nil)
	if err != context.Canceled {
		// With 1001 entries we hit n%1000==0 at n=1000 and return ctx.Err()
		t.Errorf("RecoverCASKind with cancelled context: want err=context.Canceled, got %v (results=%d)", err, len(results))
	}
	_ = results
}
