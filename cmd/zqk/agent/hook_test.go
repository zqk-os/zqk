package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestPreSubagentHook_EnrichesSubagentPrompt(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "hook_test",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage
	secCtx := pkgctx.NewSystemSecurityContext()

	// Seed a backlog item
	bli := map[string]any{
		objects.FieldKeyID:            "BLI-HOOK-TEST-001",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Implement High Throughput Worker",
		objects.FieldKeyDescription:   "Scale up worker throughput using off-host execution",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	_ = fs.Create(context.Background(), secCtx, bli)

	// Build mock Antigravity hook input
	hookInput := antigravityHookInput{
		ConversationID: "conv-123",
		WorkspacePaths: []string{root},
	}
	hookInput.ToolCall.Name = "invoke_subagent"
	hookInput.ToolCall.Args = map[string]any{
		"Subagents": []any{
			map[string]any{
				"TypeName": "self",
				"Role":     "Worker",
				"Prompt":   "Work on BLI-HOOK-TEST-001",
			},
		},
	}

	inputBytes, err := json.Marshal(hookInput)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	inReader := bytes.NewReader(inputBytes)
	var outBuf bytes.Buffer

	err = runPreSubagentHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", inReader, &outBuf)
	if err != nil {
		t.Fatalf("hook returned error: %v", err)
	}

	var hookOut antigravityHookOutput
	if err := json.Unmarshal(outBuf.Bytes(), &hookOut); err != nil {
		t.Fatalf("failed to unmarshal hook output: %v, raw: %s", err, outBuf.String())
	}

	if hookOut.Decision != "allow" {
		t.Fatalf("expected decision allow, got %s", hookOut.Decision)
	}

	subagentsAny, ok := hookOut.Overwrite["Subagents"]
	if !ok {
		t.Fatal("expected Subagents in overwrite")
	}

	subagentsSlice, ok := subagentsAny.([]any)
	if !ok || len(subagentsSlice) == 0 {
		t.Fatalf("invalid Subagents structure: %v", subagentsAny)
	}

	firstSub, ok := subagentsSlice[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid first subagent: %v", subagentsSlice[0])
	}

	enrichedPrompt, _ := firstSub["Prompt"].(string)
	if !strings.Contains(enrichedPrompt, "BLI-HOOK-TEST-001") {
		t.Fatalf("expected enriched prompt to contain task context, got: %s", enrichedPrompt)
	}
	if !strings.Contains(enrichedPrompt, "Standing mandates") && !strings.Contains(enrichedPrompt, "Orchestration Context") {
		t.Fatalf("expected enriched prompt to contain standing mandates, got: %s", enrichedPrompt)
	}
}
