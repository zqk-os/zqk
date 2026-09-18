package object

import (
	"context"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestObjectUpdate_FileWithAddRef(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	tmpDir, cliBinary := setupCLITestEnvironment(t)

	// Create milestone targets
	mil1 := "MIL-TEST-1"
	mil2 := "MIL-TEST-2"

	for _, id := range []string{mil1, mil2} {
		mFile := filepath.Join(tmpDir, id+".yaml")
		mData, _ := yaml.Marshal(map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "milestone",
			objects.FieldKeyTitle:         "Test milestone " + id,
			objects.FieldKeyStatus:        objectStatusNotStarted,
			objects.FieldKeySchemaVersion: "2.0",
		})
		_ = fileutil.WriteFile(mFile, mData, paths.FilePerm644)
		execCommand(t, cliBinary, tmpDir, "object", "create", "milestone", "--file", mFile)
	}

	// Create a backlog item
	bliID := "BLI-FILE-REF-TEST"
	bliFile := filepath.Join(tmpDir, "bli.yaml")
	bliData, _ := yaml.Marshal(map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Test item",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: "2.0",
	})
	_ = fileutil.WriteFile(bliFile, bliData, paths.FilePerm644)
	execCommand(t, cliBinary, tmpDir, "object", "create", pplanKindBacklogItem, "--file", bliFile)

	// Create an update file
	updFile := filepath.Join(tmpDir, "update.yaml")
	updData, _ := yaml.Marshal(map[string]any{
		objects.FieldKeyMilestoneRefs: []string{mil1},
	})
	_ = fileutil.WriteFile(updFile, updData, paths.FilePerm644)

	// Update with file AND add-ref
	out := execCommand(t, cliBinary, tmpDir, "object", "update", bliID, "--file", updFile, "--add-ref", "milestone_refs="+mil2)
	if out != "" {
		t.Fatalf("update failed: %s", out)
	}

	// Verify it has BOTH milestone refs
	st, _ := storage.NewFileObjectStorage(tmpDir)
	testkit.RegisterStorageTestCleanup(t, tmpDir, st)
	obj, _ := st.Read(context.Background(), pkgctx.NewSystemSecurityContext(), bliID)

	refs, _ := obj[objects.FieldKeyMilestoneRefs].([]any)
	if len(refs) != 2 {
		t.Errorf("Expected 2 refs, got %d: %v", len(refs), refs)
	}
}
