package object

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestGetObjectExportFormats(t *testing.T) {
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

	goalID := "GOAL-77777"
	goalObj := createTestObject("goal", goalID, goalFields, 0)
	goalObj[objects.FieldKeyStatus] = objectStatusActive
	goalObj["title"] = "Build Sovereign Agent Kernel"
	goalObj["description"] = "Construct an **operating system** for autonomous AI swarms\\nwith sub-15ms AST search\\u2014eliminating context amnesia."

	if err := fs.Create(cliCtx, secCtx, goalObj); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"goal"}); err != nil {
		t.Fatalf("ensure referenced objects visible: %v", err)
	}

	// 1. Test get with --format raw (unescaped prose)
	cmdRaw := testEnv.CreateCLICommand("object", "get", goalID, "--format", "raw")
	outRaw, err := cmdRaw.CombinedOutput()
	if err != nil {
		t.Fatalf("get --format raw failed: %v output=%s", err, string(outRaw))
	}
	rawStr := string(outRaw)
	if !strings.Contains(rawStr, "with sub-15ms AST search—eliminating context amnesia.") {
		t.Errorf("raw format missing unescaped em-dash: %s", rawStr)
	}
	if !strings.Contains(rawStr, "for autonomous AI swarms\nwith sub-15ms") {
		t.Errorf("raw format missing unescaped newline: %s", rawStr)
	}

	// 2. Test get with --format md (markdown)
	cmdMD := testEnv.CreateCLICommand("object", "get", goalID, "--format", "md")
	outMD, err := cmdMD.CombinedOutput()
	if err != nil {
		t.Fatalf("get --format md failed: %v output=%s", err, string(outMD))
	}
	mdStr := string(outMD)
	if !strings.HasPrefix(mdStr, "# Build Sovereign Agent Kernel") {
		t.Errorf("md format missing # title: %s", mdStr)
	}

	// 3. Test get with --format html (HTML)
	cmdHTML := testEnv.CreateCLICommand("object", "get", goalID, "--format", "html")
	outHTML, err := cmdHTML.CombinedOutput()
	if err != nil {
		t.Fatalf("get --format html failed: %v output=%s", err, string(outHTML))
	}
	htmlStr := string(outHTML)
	if !strings.Contains(htmlStr, "<h1>Build Sovereign Agent Kernel</h1>") {
		t.Errorf("html format missing h1 title: %s", htmlStr)
	}
	if !strings.Contains(htmlStr, "<strong>operating system</strong>") {
		t.Errorf("html format missing strong tag: %s", htmlStr)
	}

	// 4. Test fallback file resolution with staged_drafts.json
	stagedDir := filepath.Join(testEnv.GetTestRoot(), paths.ProjectDataDir, "state")
	_ = fileutil.MkdirAll(stagedDir, paths.DirPerm755)
	draftData := map[string]any{
		"campaigns": map[string]any{
			"launch-t0": map[string]any{
				"title": "Launch Post",
				"substack": map[string]any{
					"body": "Substack content\\nSecond line\\u2014dash",
				},
			},
		},
	}
	b, _ := json.Marshal(draftData)
	_ = fileutil.WriteFile(filepath.Join(stagedDir, "staged_drafts.json"), b, 0644)

	cmdDraft := testEnv.CreateCLICommand("object", "get", "launch-t0", "--fields", "substack.body", "--format", "raw")
	outDraft, err := cmdDraft.CombinedOutput()
	if err != nil {
		t.Fatalf("get staged draft failed: %v output=%s", err, string(outDraft))
	}
	draftStr := string(outDraft)
	if !strings.Contains(draftStr, "Substack content\nSecond line—dash") {
		t.Errorf("staged draft extraction failed, got: %s", draftStr)
	}
}
