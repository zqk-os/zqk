package swarm

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
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
