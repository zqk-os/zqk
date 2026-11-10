package mcpkeepalive

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestKeepAliveWriteReadRemove(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ka := New(root, "7100")

	if err := ka.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	ts, err := ka.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if ts.IsZero() {
		t.Fatal("Read returned zero timestamp")
	}
	if since := time.Since(ts); since < 0 || since > 30*time.Second {
		t.Fatalf("timestamp not fresh: %v", since)
	}

	if err := ka.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := ka.Read(); err == nil {
		t.Fatal("Read after Remove should fail")
	}
}

func TestKeepAlivePathNaming(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ka := New(root, "7100")
	wantDir := filepath.Join(root, paths.ProjectDataDir, "mcp")
	if filepath.Dir(ka.Path()) != wantDir {
		t.Fatalf("Path() dir = %q, want %q", filepath.Dir(ka.Path()), wantDir)
	}
	if !strings.Contains(filepath.Base(ka.Path()), "7100") {
		t.Fatalf("Path() = %q, want port 7100 in filename", ka.Path())
	}
}

func TestFreshness(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ka := New(root, "7100")
	if err := ka.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !ka.IsFresh(DefaultStaleAfter) {
		t.Fatal("fresh keep-alive reported stale")
	}
}

func TestLaunchAgentTemplatePinsSupervision(t *testing.T) {
	t.Parallel()
	tpl := LaunchAgentTemplate{
		Label:       "com.zqk.mcp-daemon-7100",
		ProjectRoot: "/tmp/proj",
		TCPAddr:     "127.0.0.1:7100",
	}
	body := tpl.Render()

	if !strings.Contains(body, "<key>KeepAlive</key>\n\t<true/>") {
		t.Fatal("template must pin KeepAlive true so launchd supervises the loop")
	}
	if !strings.Contains(body, "<key>RunAtLoad</key>\n\t<true/>") {
		t.Fatal("template must pin RunAtLoad true")
	}
	if !strings.Contains(body, "<string>supervise</string>") || !strings.Contains(body, "<string>--loop</string>") {
		t.Fatal("template ProgramArguments must run mcp supervise --loop")
	}
	daemonKey := zqkenv.IsDaemon().Name()
	if !strings.Contains(body, "<key>"+daemonKey+"</key>") || !strings.Contains(body, "<string>1</string>") {
		t.Fatalf("template must pin %s=1 for the supervised process", daemonKey)
	}
	if strings.Contains(body, "__ZQK_PROJECT_ROOT__") || strings.Contains(body, "__ZQK_MCP_TCP__") {
		t.Fatal("Render must substitute all template placeholders")
	}
}
