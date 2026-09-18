package mcp

import (
	"testing"

	pkgmcp "github.com/zqk-os/zqk/pkg/mcp"
)

func TestNewMCPCmd_wiresEnsureSuperviseDaemonProxy(t *testing.T) {
	cmd := NewMCPCmd()
	if cmd.Use != "mcp" {
		t.Fatalf("use=%q", cmd.Use)
	}
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"ensure", "supervise", "daemon", "ide-adapter", "cursor-adapter", "proxy", "serve", "install", "list-tools"} {
		if !names[want] {
			t.Fatalf("missing subcommand %q in %v", want, names)
		}
	}

	ensure := NewEnsureCmd()
	if f := ensure.Flags().Lookup("tcp"); f == nil || f.DefValue != pkgmcp.DefaultDaemonTCP {
		t.Fatalf("ensure --tcp default=%v", f)
	}
	sup := NewSuperviseCmd()
	for _, name := range []string{"tcp", "status", "stop", "loop"} {
		if sup.Flags().Lookup(name) == nil {
			t.Fatalf("supervise missing --%s", name)
		}
	}
	proxy := NewProxyCmd()
	if f := proxy.Flags().Lookup("tcp"); f == nil || f.DefValue != pkgmcp.DefaultDaemonTCP {
		t.Fatalf("proxy --tcp default=%v", f)
	}
}

func TestDefaultDaemonTCPShared(t *testing.T) {
	t.Parallel()
	if DefaultMCPDaemonTCP != pkgmcp.DefaultDaemonTCP {
		t.Fatalf("cli alias %q != pkg %q", DefaultMCPDaemonTCP, pkgmcp.DefaultDaemonTCP)
	}
}

func TestServeCommandFlags(t *testing.T) {
	cmd := NewServeCmd()
	flags := []string{"tcp", "tls-cert", "tls-key", "tls-ca"}
	for _, flagName := range flags {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Fatalf("serve command missing flag --%s", flagName)
		}
	}
}
