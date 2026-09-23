package swarm

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/swarm/pack"
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
	if !strings.Contains(err.Error(), "decentralized remote package") {
		t.Errorf("expected decentralized remote package error, got: %v", err)
	}
}
