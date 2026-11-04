package object

import (
	"encoding/json"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
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

	goalID2 := "GOAL-99998"
	goalObj2 := createTestObject("goal", goalID2, goalFields, 1)
	goalObj2[objects.FieldKeyStatus] = objectStatusActive

	if err := fs.Create(cliCtx, secCtx, goalObj); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := fs.Create(cliCtx, secCtx, goalObj2); err != nil {
		t.Fatalf("create goal 2: %v", err)
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

	// 4. Test "get --kind <kind> <id>"
	cmd4 := testEnv.CreateCLICommand("object", "get", "--kind", "goal", goalID, "--format", "json")
	out4, err := cmd4.CombinedOutput()
	if err != nil {
		t.Fatalf("get --kind id failed: %v output=%s", err, string(out4))
	}

	// Assert outputs match the created goal
	for idx, out := range [][]byte{out1, out2, out3, out4} {
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("unmarshal output %d: %v; output=%s", idx+1, err, string(out))
		}
		if got[objects.FieldKeyID] != goalID {
			t.Errorf("expected goal id=%s in output %d; got=%v", goalID, idx+1, got[objects.FieldKeyID])
		}
	}

	// 5. Test comma-separated IDs on get and show
	commaIDs := goalID + "," + goalID2
	for _, args := range [][]string{
		{"object", "get", commaIDs, "--format", "json"},
		{"object", "get", "goal", commaIDs, "--format", "json"},
		{"object", "get", "--kind", "goal", commaIDs, "--format", "json"},
		{"object", "show", commaIDs, "--format", "json"},
		{"object", "show", "goal", commaIDs, "--format", "json"},
		{"object", "show", "--kind", "goal", commaIDs, "--format", "json"},
	} {
		cmd := testEnv.CreateCLICommand(args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bulk command %v failed: %v output=%s", args, err, string(out))
		}
		var rawMap map[string]any
		if err := json.Unmarshal(out, &rawMap); err != nil {
			t.Fatalf("unmarshal bulk output for %v: %v output=%s", args, err, string(out))
		}
		if rawMap["total"] != float64(2) || rawMap["success"] != float64(2) {
			t.Fatalf("expected 2 successful results for %v, got %v", args, rawMap)
		}
	}
}
