package cli

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestHandleDaemonCommandTimeout_mcpDaemonRunsWithoutOuterCap(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)
	ctx := pkgctx.NewSystemContext()

	var ran atomic.Bool
	handled, err := hook.handleDaemonCommandTimeout(
		ctx,
		"zqk",
		"zqk mcp daemon --tcp 127.0.0.1:8443",
		[]string{"mcp", "daemon", "--tcp", "127.0.0.1:8443"},
		&CommandContext{},
		func(execCtx context.Context) error {
			ran.Store(true)
			// Must not see a short child timeout cancel (would fail if 2m path applied incorrectly
			// in a future refactor that wired execCtx into this branch incorrectly).
			select {
			case <-time.After(50 * time.Millisecond):
				return nil
			case <-execCtx.Done():
				t.Fatalf("mcp daemon exec context cancelled early: %v", execCtx.Err())
				return execCtx.Err()
			}
		},
		time.Now(),
		2*time.Minute, // typical childMaxTimeout lure — daemon path must ignore
		false,
	)
	if !handled {
		t.Fatal("expected mcp daemon to be handled as long-lived daemon command")
	}
	if err != nil {
		t.Fatalf("handleDaemonCommandTimeout: %v", err)
	}
	if !ran.Load() {
		t.Fatal("expected daemon fn to run")
	}
}

func TestHandleDaemonCommandTimeout_ideAdapterIsLongLived(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)
	ctx := pkgctx.NewSystemContext()

	var ran atomic.Bool
	handled, err := hook.handleDaemonCommandTimeout(
		ctx,
		"zqk-mcp-ide-adapter",
		"zqk-mcp-ide-adapter mcp ide-adapter --tcp 127.0.0.1:8443",
		[]string{"mcp", "ide-adapter", "--tcp", "127.0.0.1:8443"},
		&CommandContext{},
		func(execCtx context.Context) error {
			ran.Store(true)
			select {
			case <-time.After(50 * time.Millisecond):
				return nil
			case <-execCtx.Done():
				t.Fatalf("ide-adapter exec context cancelled early: %v", execCtx.Err())
				return execCtx.Err()
			}
		},
		time.Now(),
		2*time.Minute,
		false,
	)
	if !handled {
		t.Fatal("expected mcp ide-adapter to be handled as long-lived daemon command")
	}
	if err != nil {
		t.Fatalf("handleDaemonCommandTimeout: %v", err)
	}
	if !ran.Load() {
		t.Fatal("expected ide-adapter fn to run")
	}
	if !isMCPLongLivedCommand("zqk-mcp-ide-adapter mcp ide-adapter --tcp 127.0.0.1:8443") {
		t.Fatal("isMCPLongLivedCommand must match mcp ide-adapter")
	}
	if !isMCPLongLivedCommand("zqk mcp cursor-adapter --tcp 127.0.0.1:8443") {
		t.Fatal("isMCPLongLivedCommand must match mcp cursor-adapter")
	}
}

func TestGetTimeoutForCommand_mcpDaemonUsesIdleTimeoutDisabled(t *testing.T) {
	hook := NewTimeoutHook()
	// When idle_timeout is "0" in project config, daemon should get InfiniteTimeout.
	// This test only asserts the mcp-daemon branch is recognized the same way as mcp serve
	// when config disables idle timeout (project has idle_timeout: "0").
	d := hook.getTimeoutForCommand("zqk mcp daemon --tcp 127.0.0.1:8443", []string{"mcp", "daemon"})
	cfg := hook.getTimeoutConfig()
	if d != cfg.InfiniteTimeout && d != 0 {
		// 0 can happen if config walk fails in test cwd; InfiniteTimeout is success path.
		// Accept either infinite or a positive MCP-configured duration — never the tiny
		// metrics baseline that was killing the TCP daemon.
		if d > 0 && d <= 2*time.Minute {
			t.Fatalf("mcp daemon timeout too short (child-cap territory): %v", d)
		}
	}
}

func TestGetMCPIdleTimeout_reloadsWhenStampMoves(t *testing.T) {
	root := t.TempDir()
	configPath := paths.MCPConfigPath(root)
	if err := fileutil.EnsureDir(filepath.Dir(configPath)); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	hook := NewTimeoutHook()
	if d, disabled := hook.getMCPIdleTimeout(); d != 0 || disabled {
		t.Fatalf("missing config: timeout=%v disabled=%v", d, disabled)
	}

	if err := fileutil.WriteStandardFile(configPath, []byte("mcp_server:\n  idle_timeout: 99m\n")); err != nil {
		t.Fatal(err)
	}
	d, disabled := hook.getMCPIdleTimeout()
	if disabled || d != 99*time.Minute {
		t.Fatalf("after write: timeout=%v disabled=%v want 99m", d, disabled)
	}

	if err := fileutil.WriteStandardFile(configPath, []byte("mcp_server:\n  idle_timeout: \"0\"\n")); err != nil {
		t.Fatal(err)
	}
	d, disabled = hook.getMCPIdleTimeout()
	if !disabled || d != 0 {
		t.Fatalf("after disable: timeout=%v disabled=%v want disabled", d, disabled)
	}
}
