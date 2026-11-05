package newcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestEmitJITGuidanceTip_AllKinds(t *testing.T) {
	kinds := []string{
		objects.KindGoal,
		objects.KindRequirement,
		objects.KindMilestone,
		objects.KindTestCase,
		objects.KindBacklogItem,
		objects.KindPriorityPlan,
		objects.KindEpic,
		objects.KindTechnicalDebt,
		"unknown_kind",
	}

	for _, kind := range kinds {
		var buf bytes.Buffer
		cmd := &cobra.Command{Use: "test"}
		cmd.SetErr(&buf)
		emitJITGuidanceTip(cmd, kind)
		out := buf.String()
		if kind != "unknown_kind" && !strings.Contains(out, "Shift-Left Tip") {
			t.Errorf("kind %s: expected guidance tip, got %q", kind, out)
		}
	}

	// JSON/YAML format suppress tip
	formats := []string{"json", "yaml", "stream"}
	for _, f := range formats {
		var buf bytes.Buffer
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("format", "", "")
		_ = cmd.Flags().Set("format", f)
		cmd.SetErr(&buf)
		emitJITGuidanceTip(cmd, objects.KindGoal)
		if buf.Len() != 0 {
			t.Errorf("format %s: expected empty tip, got %q", f, buf.String())
		}
	}
}

func TestReadTitleAndBody_Paths(t *testing.T) {
	// 1. Direct content parsing
	body, title, err := readTitleAndBody("", "# Direct Title\n\nDirect body content here", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Direct Title" {
		t.Errorf("expected 'Direct Title', got %q", title)
	}
	if !strings.Contains(body, "Direct body content here") {
		t.Errorf("expected body to contain content, got %q", body)
	}

	// 2. Title override
	body, title, err = readTitleAndBody("", "# Old Title\n\nBody", "New Title Override")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "New Title Override" {
		t.Errorf("expected 'New Title Override', got %q", title)
	}

	// 3. File reading
	tmpFile := filepath.Join(t.TempDir(), "draft.md")
	if err := fileutil.WriteFile(tmpFile, []byte("# From File\n\nBody from file."), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	body, title, err = readTitleAndBody(tmpFile, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "From File" {
		t.Errorf("expected 'From File', got %q", title)
	}
	if !strings.Contains(body, "Body from file.") {
		t.Errorf("expected body from file, got %q", body)
	}

	// 4. File reading error
	_, _, err = readTitleAndBody(filepath.Join(t.TempDir(), "nonexistent.md"), "", "")
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}
}

func TestEnqueueMintPromoteJob(t *testing.T) {
	// Empty inputs fail-closed
	_, err := enqueueMintPromoteJob("", "BLI-123")
	if err == nil {
		t.Error("expected error with empty projectRoot")
	}
	_, err = enqueueMintPromoteJob("/tmp/repo", "")
	if err == nil {
		t.Error("expected error with empty objectID")
	}

	// Valid job creation with isolated project
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRoot(); err == nil {
			repoRoot = root
		}
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	t.Cleanup(func() {
		if teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil)); teardownErr != nil {
			t.Logf("teardown error: %v", teardownErr)
		}
	})

	jobID, err := enqueueMintPromoteJob(testRoot, "BLI-TEST-MINT-001")
	if err != nil {
		t.Fatalf("enqueueMintPromoteJob: %v", err)
	}
	if !strings.HasPrefix(jobID, "SCH-mint-promote-") {
		t.Errorf("expected prefix SCH-mint-promote-, got %q", jobID)
	}
}

func TestNewObject_EdgeCases(t *testing.T) {
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRoot(); err == nil {
			repoRoot = root
		}
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	t.Cleanup(func() {
		if teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil)); teardownErr != nil {
			t.Logf("teardown error: %v", teardownErr)
		}
	})

	// Test object command with --file
	tmpFile := filepath.Join(testRoot, "spec.md")
	if err := fileutil.WriteFile(tmpFile, []byte("# Markdown Title\nSpec body"), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "backlog_item", "--file", tmpFile})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("object with --file failed: %v", err)
	}

	// Test object command with --content
	buf.Reset()
	cmd = NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "backlog_item", "--content", "# Inline Title\nInline body"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("object with --content failed: %v", err)
	}
}

func TestNewTestCase_Aliases(t *testing.T) {
	cmd := NewNewCmd()
	var tcCmd *cobra.Command
	for _, sub := range cmd.Commands() {
		if sub.Name() == objects.KindTestCase {
			tcCmd = sub
			break
		}
	}
	if tcCmd == nil {
		t.Fatalf("test_case command not found under new")
	}
	if !slices.Contains(tcCmd.Aliases, "tst") || !slices.Contains(tcCmd.Aliases, "tc") {
		t.Fatalf("expected test_case aliases to contain 'tst' and 'tc', got %v", tcCmd.Aliases)
	}
}

func TestNewCriteria_DefaultCategory(t *testing.T) {
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRoot(); err == nil {
			repoRoot = root
		}
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	t.Cleanup(func() {
		if teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil)); teardownErr != nil {
			t.Logf("teardown error: %v", teardownErr)
		}
	})

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	// 1. Default category via "criteria"
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"criteria", "--title", "Default Category Crit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new criteria failed: %v", err)
	}

	draftFiles, err := storage.ListObjectDraftPlaneIDs(testRoot, objects.KindCriteria)
	if err != nil || len(draftFiles) == 0 {
		t.Fatalf("expected drafted criteria on draft plane, got err=%v count=%d", err, len(draftFiles))
	}
	data, err := fileutil.ReadFile(storage.ObjectDraftPlanePath(testRoot, objects.KindCriteria, draftFiles[0]))
	if err != nil {
		t.Fatalf("failed to read drafted criteria: %v", err)
	}
	var critObj map[string]any
	if err := yaml.Unmarshal(data, &critObj); err != nil {
		t.Fatalf("unmarshal drafted criteria: %v", err)
	}
	if cat, _ := critObj[objects.FieldKeyCategory].(string); cat != "functional" {
		t.Fatalf("expected category 'functional', got %q", cat)
	}

	// 2. Custom category via flag
	buf.Reset()
	cmd = NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"criteria", "--title", "Acceptance Crit", "--category", "acceptance"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new criteria with --category failed: %v", err)
	}
}
