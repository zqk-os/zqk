package swarm

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/internal/bootstrap"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSwarmRunCommand_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	manifestFile := filepath.Join(tmpDir, "swarm.yaml")
	if err := pack.EnsureSampleSwarm(manifestFile); err != nil {
		t.Fatalf("EnsureSampleSwarm failed: %v", err)
	}

	cmd := NewRunCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{manifestFile, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Swarm Package: sample-refactor-swarm") {
		t.Errorf("expected swarm package header, got:\n%s", out)
	}
	if !strings.Contains(out, "Refactor Specialist") {
		t.Errorf("expected Refactor Specialist agent, got:\n%s", out)
	}
	if !strings.Contains(out, "✓ Swarm validation successful (dry-run mode).") {
		t.Errorf("expected dry-run success message, got:\n%s", out)
	}
}

func TestSwarmRunCommand_MissingManifest(t *testing.T) {
	cmd := NewRunCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"/nonexistent/path/swarm.yaml"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing manifest file")
	}
}

func TestSwarmRunCommand_RemotePackageHint(t *testing.T) {
	cmd := NewRunCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"github.com/user/expert-refactor-swarm"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for un-cloned remote package")
	}
	if !strings.Contains(err.Error(), "failed to resolve remote swarm") && !strings.Contains(err.Error(), "decentralized remote package") {
		t.Errorf("expected remote package error, got: %v", err)
	}
}

func TestSwarmRunCommand_CachedRemoteSwarm(t *testing.T) {
	home, err := fileutil.UserHomeDir()
	if err != nil {
		t.Skip("skipping test due to home dir error")
	}
	cacheTarget := filepath.Join(home, paths.ProjectDataDir, "swarms", "cache", "test-org", "cached-swarm")
	if err := fileutil.MkdirAll(cacheTarget, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	defer fileutil.RemoveAll(cacheTarget)

	sampleYAML := `schema_version: 1.0.0
name: cached-swarm
version: 0.1.0
description: A cached remote swarm test
agents:
  - name: reviewer
    role: code_reviewer
`
	if err := fileutil.WriteFile(filepath.Join(cacheTarget, "swarm.yaml"), []byte(sampleYAML), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write swarm.yaml: %v", err)
	}

	cmd := NewRunCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"github.com/test-org/cached-swarm", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error running cached remote swarm: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "cached-swarm v0.1.0") {
		t.Errorf("expected cached swarm output, got:\n%s", out)
	}
}

func TestSwarmRunCommand_LiveIngestionAndPersistence(t *testing.T) {
	root := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, root, nil)
	logger := logging.GetLoggerFromProfile("test")
	if err := bootstrap.ExtractTo(root, logger, true); err != nil {
		t.Fatalf("bootstrap.ExtractTo failed: %v", err)
	}

	manifestFile := filepath.Join(root, "swarm.yaml")
	if err := pack.EnsureSampleSwarm(manifestFile); err != nil {
		t.Fatalf("EnsureSampleSwarm failed: %v", err)
	}

	cmd := NewRunCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{manifestFile, "--stage-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("live execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Swarm Package: sample-refactor-swarm") {
		t.Errorf("expected swarm package header, got:\n%s", out)
	}
	if !strings.Contains(out, "Swarm initialized and dispatch ready") {
		t.Errorf("expected dispatch ready output, got:\n%s", out)
	}
	if !strings.Contains(out, "Active Plan:       PRI-SAMPLE_REFACTOR_SWARM") {
		t.Errorf("expected Active Plan in output, got:\n%s", out)
	}

	// Verify persistence in ObjectStorageProvider
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	if err != nil {
		t.Fatalf("failed to get storage for root %s: %v", root, err)
	}
	testkit.RegisterStorageTestCleanup(t, root, store)

	// Verify Priority Plan
	planObj, err := store.Read(ctx, sec, "PRI-SAMPLE_REFACTOR_SWARM")
	if err != nil {
		t.Fatalf("failed to read persisted priority_plan: %v", err)
	}
	if planObj[objects.FieldKeyStatus] != objects.ObjectStatusActive {
		t.Errorf("expected plan status active, got: %v", planObj[objects.FieldKeyStatus])
	}

	// Verify Backlog Items
	bliObj, err := store.Read(ctx, sec, "BLI-SAMPLE_REFACTOR_SWARM-TASK_REFACTOR")
	if err != nil {
		t.Fatalf("failed to read persisted backlog_item: %v", err)
	}
	if bliObj[objects.FieldKeyPriorityPlanRef] != "PRI-SAMPLE_REFACTOR_SWARM" {
		t.Errorf("expected priority_plan_ref link, got: %v", bliObj[objects.FieldKeyPriorityPlanRef])
	}
	if bliObj[objects.FieldKeyStatus] != objects.ObjectStatusPlanned {
		t.Errorf("expected backlog item status planned, got: %v", bliObj[objects.FieldKeyStatus])
	}

	// Verify Goal and Milestone
	if _, err := store.Read(ctx, sec, "GOAL-SAMPLE_REFACTOR_SWARM"); err != nil {
		t.Errorf("failed to read goal: %v", err)
	}
	if _, err := store.Read(ctx, sec, "MIL-SAMPLE_REFACTOR_SWARM"); err != nil {
		t.Errorf("failed to read milestone: %v", err)
	}
	if _, err := store.Read(ctx, sec, "WS-SAMPLE_REFACTOR_SWARM"); err != nil {
		t.Errorf("failed to read workstream: %v", err)
	}

	// Verify Requirements and Criteria
	if _, err := store.Read(ctx, sec, "REQ-SAMPLE_REFACTOR_SWARM-TASK_REFACTOR"); err != nil {
		t.Errorf("failed to read requirement: %v", err)
	}
	if _, err := store.Read(ctx, sec, "CRIT-SAMPLE_REFACTOR_SWARM-TASK_REFACTOR-INV"); err != nil {
		t.Errorf("failed to read criteria: %v", err)
	}
	_ = store.Shutdown(ctx)
}
