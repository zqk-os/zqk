package sync

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func isolateSyncTest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)
	zqkenv.ApplyIsolatedStorageEnv(t.Setenv)
}

func TestNewSyncCmd_Structure(t *testing.T) {
	cmd := NewSyncCmd()
	if cmd.Use != "sync [command]" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	for _, expected := range []string{"github", "linear", "status"} {
		if !subcommands[expected] {
			t.Errorf("expected subcommand %s to be registered", expected)
		}
	}
}

func TestSyncCmd_DryRunGitHub(t *testing.T) {
	isolateSyncTest(t)
	cmd := NewSyncCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"github", "--repo", "zqk-os/zqk", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing dry-run: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "DRY-RUN") || !strings.Contains(output, "zqk-os/zqk") {
		t.Errorf("expected dry-run output mentioning repo, got: %s", output)
	}
}

func TestSyncCmd_DryRunLinear(t *testing.T) {
	isolateSyncTest(t)
	cmd := NewSyncCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"linear", "--team", "ENG", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing dry-run: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "DRY-RUN") || !strings.Contains(output, "ENG") {
		t.Errorf("expected dry-run output mentioning team, got: %s", output)
	}
}

