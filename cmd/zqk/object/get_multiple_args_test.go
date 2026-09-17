package object

import (
	"encoding/json"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestGetMultipleArgs(t *testing.T) {
	testEnv := SetupTestEnvironment(t)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	goalFields, err := fieldRegistry.GetFieldsForKind("goal")
	if err != nil {
		t.Fatalf("failed to get goal fields: %v", err)
	}

	fs, err := storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testEnv.GetTestRoot(), fs)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	goalID := "GOAL-99999"
	goalObj := createTestObject("goal", goalID, goalFields, 0)
	goalObj[objects.FieldKeyStatus] = objectStatusActive

	if err := fs.Create(cliCtx, secCtx, goalObj); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"goal"}); err != nil {
		t.Fatalf("ensure referenced objects visible: %v", err)
	}

	// 1. Test standard "get <id>"
	cmd1 := testEnv.CreateCLICommand("object", "get", goalID, "--format", "json")
	out1, err := cmd1.CombinedOutput()
	if err != nil {
		t.Fatalf("standard get failed: %v output=%s", err, string(out1))
	}

	// 2. Test "get <kind> <id>"
	cmd2 := testEnv.CreateCLICommand("object", "get", "goal", goalID, "--format", "json")
	out2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("get kind id failed: %v output=%s", err, string(out2))
	}

	// 3. Test "show <kind> <id>"
	cmd3 := testEnv.CreateCLICommand("object", "show", "goal", goalID, "--format", "json")
	out3, err := cmd3.CombinedOutput()
	if err != nil {
		t.Fatalf("show kind id failed: %v output=%s", err, string(out3))
	}

	// Assert outputs match the created goal
	for idx, out := range [][]byte{out1, out2, out3} {
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("unmarshal output %d: %v; output=%s", idx+1, err, string(out))
		}
		if got[objects.FieldKeyID] != goalID {
			t.Errorf("expected goal id=%s in output %d; got=%v", goalID, idx+1, got[objects.FieldKeyID])
		}
	}
}
