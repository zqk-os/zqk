package sessionpaging

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBuildHandoffPacket_NonGitDirectoryDegradesGracefully(t *testing.T) {
	dir := t.TempDir()
	pkt, err := BuildHandoffPacket(HandoffOptions{
		ProjectRoot: dir,
		Note:        "Switching from Claude Code to Gemini 2.5 Pro at Turn 12",
		ActiveGoals: []HandoffObjectRef{
			{ID: "GOAL-001", Title: "Ship Portable Context Handoff", Status: "in_progress"},
		},
		ActivePlans: []HandoffObjectRef{
			{ID: "PRI-001", Title: "Study 2 Token-Cap Escape Hatch", Status: "in_progress"},
		},
		ActiveBacklogItems: []HandoffObjectRef{
			{ID: "BLI-001", Title: "Implement BuildHandoffPacket", Status: "in_progress"},
		},
	})
	if err != nil {
		t.Fatalf("BuildHandoffPacket failed: %v", err)
	}
	if pkt.Schema != HandoffSchemaVersion {
		t.Fatalf("unexpected schema: got %q, want %q", pkt.Schema, HandoffSchemaVersion)
	}
	if pkt.MaxTokens != DefaultHandoffMaxTokens {
		t.Fatalf("expected default max tokens %d, got %d", DefaultHandoffMaxTokens, pkt.MaxTokens)
	}
	if pkt.Truncated {
		t.Fatalf("expected small handoff packet not to be truncated")
	}
	if pkt.EstimatedTokens <= 0 || pkt.EstimatedTokens > pkt.MaxTokens {
		t.Fatalf("expected estimated tokens in (0, %d], got %d", pkt.MaxTokens, pkt.EstimatedTokens)
	}

	md := FormatHandoffMarkdown(pkt)
	for _, want := range []string{"GOAL-001", "PRI-001", "BLI-001", "Switching from Claude Code", "zqk workflow whats-next"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestBuildHandoffPacket_GitRepoAndStrictTokenBudget(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := testkit.ManagedCommand(t, context.Background(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@zqk.dev")
	run("config", "user.name", "ZQK Test")

	baseFile := filepath.Join(dir, "main.go")
	if err := fileutil.WriteStandardFile(baseFile, []byte("package main\nfunc main() {}\n")); err != nil {
		t.Fatalf("write baseFile: %v", err)
	}
	run("add", "main.go")
	run("commit", "-m", "initial")

	// Mutate tracked file and add many untracked files to exercise truncation
	if err := fileutil.WriteStandardFile(baseFile, []byte("package main\nfunc main() { println(\"handoff\") }\n")); err != nil {
		t.Fatalf("update baseFile: %v", err)
	}
	for i := 0; i < 40; i++ {
		f := filepath.Join(dir, fmt.Sprintf("extra_module_file_%02d.go", i))
		if err := fileutil.WriteStandardFile(f, []byte("package main\n")); err != nil {
			t.Fatalf("write extra file: %v", err)
		}
	}

	pkt, err := BuildHandoffPacket(HandoffOptions{
		ProjectRoot: dir,
		MaxTokens:   180,
		IncludeDiff: true,
	})
	if err != nil {
		t.Fatalf("BuildHandoffPacket failed: %v", err)
	}
	if pkt.GitBranch != "main" {
		t.Errorf("expected git branch main, got %q", pkt.GitBranch)
	}
	if pkt.GitHeadSHA == "" {
		t.Errorf("expected non-empty GitHeadSHA")
	}
	if !pkt.Truncated {
		t.Errorf("expected strict 180-token budget to trigger truncation")
	}
	if pkt.EstimatedTokens > pkt.MaxTokens {
		t.Errorf("expected EstimatedTokens (%d) <= MaxTokens (%d)", pkt.EstimatedTokens, pkt.MaxTokens)
	}
}
