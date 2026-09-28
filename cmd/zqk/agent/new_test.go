package agent_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAgentNew(t *testing.T) {
	root, store := setupOrchestrateTest(t)

	cmd := agent.NewAgentNewCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	cmd.SetArgs([]string{"TestAgent"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent new failed: %v", err)
	}

	t.Logf("DEBUG: directory for agent_skill = %q", objects.GetDirectoryFromKind("agent_skill"))
	t.Logf("DEBUG: directory for persona = %q", objects.GetDirectoryFromKind("persona"))

	t.Logf("DEBUG FILES:")
	_ = filepath.Walk(filepath.Join(root, paths.ProcessDir), func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // debug directory walk in test
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(root, path)
			t.Logf("  FILE: %s", rel)
		}
		return nil
	})

	flushCtx, flushCancel := storage.DurabilityFlushContext()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, store, root, []string{"persona", "agent_skill"}); err != nil {
		flushCancel()
		t.Fatalf("EnsureCLIObjectMutationVisibleForProvider failed: %v", err)
	}
	flushCancel()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Check if objects were created
	// Since we don't know the exact IDs, we can list them by kind
	res, err := store.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: "persona",
	})
	if err != nil {
		t.Fatalf("List persona failed: %v", err)
	}
	if len(res.Objects) < 1 {
		t.Errorf("Expected 1 persona, got %d", len(res.Objects))
	}
	if st := objects.GetString(res.Objects[0], objects.FieldKeyStatus); st != objects.ObjectStatusApproved {
		t.Errorf("persona status=%q want %q", st, objects.ObjectStatusApproved)
	}

	resSkill, err := store.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: "agent_skill",
	})
	if err != nil {
		t.Fatalf("List agent_skill failed: %v", err)
	}
	if len(resSkill.Objects) < 1 {
		t.Errorf("Expected 1 agent_skill, got %d", len(resSkill.Objects))
	}
	if st := objects.GetString(resSkill.Objects[0], objects.FieldKeyStatus); st != objects.ObjectStatusApproved {
		t.Errorf("agent_skill status=%q want %q", st, objects.ObjectStatusApproved)
	}
}
