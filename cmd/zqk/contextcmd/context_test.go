package contextcmd

import (
	"bytes"
	"testing"

	"github.com/zqk-os/zqk/pkg/sessionpaging"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TRACK: https://github.com/zqk-os/zqk/pull/1215 — cold episodic CLI
func TestContextSnapshotRecallRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Key, root)

	hot, err := sessionpaging.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	early := []byte("early fact: go concurrency patterns")
	if err := hot.Put("agent-a", "page-0", early); err != nil {
		t.Fatal(err)
	}

	snap := newSnapshotCmd()
	snap.SetArgs([]string{"--session-id", "agent-a", "--page-id", "page-0", "--note", "phase-1"})
	var snapOut bytes.Buffer
	snap.SetOut(&snapOut)
	if err := snap.Execute(); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !bytes.Contains(snapOut.Bytes(), []byte("snapshot ok")) {
		t.Fatalf("snapshot out=%q", snapOut.String())
	}

	recall := newRecallCmd()
	recall.SetArgs([]string{"--session-id", "agent-a", "--episode-id", "page-0"})
	var recallOut bytes.Buffer
	recall.SetOut(&recallOut)
	if err := recall.Execute(); err != nil {
		t.Fatalf("recall: %v", err)
	}
	if !bytes.Contains(recallOut.Bytes(), early) {
		t.Fatalf("recall missing early fact: %q", recallOut.String())
	}
	if !bytes.Contains(recallOut.Bytes(), []byte("phase-1")) {
		t.Fatalf("recall missing note: %q", recallOut.String())
	}

	// Hot page should be gone after snapshot eviction.
	if _, err := hot.Get("agent-a", "page-0"); err == nil {
		t.Fatal("expected hot page removed")
	}
}

func TestNewContextCmdWiresSubcommands(t *testing.T) {
	t.Parallel()
	cmd := NewContextCmd()
	if cmd.Name() != "context" {
		t.Fatalf("name=%q", cmd.Name())
	}
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	if !names["snapshot"] || !names["recall"] {
		t.Fatalf("subcommands=%v", names)
	}
}

func TestContextSnapshotRequiresFlags(t *testing.T) {
	t.Parallel()
	cmd := newSnapshotCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing required flags")
	}
}

func TestContextRecallMissingEpisode(t *testing.T) {
	root := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Key, root)
	cmd := newRecallCmd()
	cmd.SetArgs([]string{"--session-id", "s1", "--episode-id", "missing"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected ErrEpisodeNotFound")
	}
}
