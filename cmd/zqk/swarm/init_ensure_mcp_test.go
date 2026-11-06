package swarm

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSwarmInitCmdFailureDetail_EmptyStderrKeepsCause(t *testing.T) {
	t.Parallel()
	detail := swarmInitCmdFailureDetail(context.Canceled, context.Canceled, "", "")
	if !strings.Contains(detail, "context canceled") {
		t.Fatalf("expected context canceled, got %q", detail)
	}
	if strings.TrimSpace(detail) == "" {
		t.Fatal("empty detail must not collapse")
	}
}

func TestSwarmInitCLIBin_PrefersStableThenTip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := fileutil.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stable := paths.RepoStableBinaryPath(root)
	if err := fileutil.WriteFile(stable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tip := filepath.Join(binDir, "zqk-tip")
	if err := fileutil.WriteFile(tip, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := paths.RepoBinPath(root)
	if err := fileutil.WriteFile(plain, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := swarmInitCLIBin(root)
	if got != stable {
		t.Fatalf("want stable %q, got %q", stable, got)
	}
}
