package mcp

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEnsureMCPIDERoleSymlinksCreatesDaemonAndIDEAdapter(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := fileutil.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(binDir, brand.ExecutableName())
	if err := fileutil.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMCPIDERoleSymlinks(root, target); err != nil {
		t.Fatalf("EnsureMCPIDERoleSymlinks: %v", err)
	}
	for _, role := range IDEMCPRoles {
		link := MCPRoleBinPath(root, role)
		fi, err := fileutil.Lstat(link)
		if err != nil {
			t.Fatalf("role %s missing: %v", role, err)
		}
		if fi.Mode()&fileutil.ModeSymlink == 0 {
			t.Fatalf("role %s not a symlink", role)
		}
	}
	// Idempotent when already present.
	if err := EnsureMCPIDERoleSymlinks(root, target); err != nil {
		t.Fatalf("second EnsureMCPIDERoleSymlinks: %v", err)
	}
}

func TestEnsureMCPRoleSymlinkCreatesProxyLink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := fileutil.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(binDir, brand.ExecutableName())
	if err := fileutil.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	link, err := EnsureMCPRoleSymlink(root, MCPRoleProxy, target)
	if err != nil {
		t.Fatalf("EnsureMCPRoleSymlink: %v", err)
	}
	wantBase := brand.ExecutableName() + "-mcp-proxy"
	if filepath.Base(link) != wantBase {
		t.Fatalf("link basename=%q want %q", filepath.Base(link), wantBase)
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	wantTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != wantTarget {
		t.Fatalf("resolved=%q want %q", resolved, wantTarget)
	}

	// Idempotent when target unchanged.
	link2, err := EnsureMCPRoleSymlink(root, MCPRoleProxy, target)
	if err != nil {
		t.Fatalf("second EnsureMCPRoleSymlink: %v", err)
	}
	if link2 != link {
		t.Fatalf("link path changed: %q vs %q", link2, link)
	}
}

func TestStripBrandSuffixesRoleNames(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"zqk":                 "zqk",
		"zqk-mcp":             "zqk",
		"zqk-mcp-proxy":       "zqk",
		"zqk-mcp-daemon":      "zqk",
		"zqk-mcp-ide-adapter": "zqk",
		"mybrand-stable":      "mybrand",
	}
	for name, want := range cases {
		if got := stripBrandSuffixes(name); got != want {
			t.Fatalf("%s -> %q want %q", name, got, want)
		}
	}
}
