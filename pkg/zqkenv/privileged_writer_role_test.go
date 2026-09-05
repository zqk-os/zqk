package zqkenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivilegedWriterDaemonArgv(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "stable object daemon", args: []string{"/tmp/zqk-stable", "object", "daemon"}, want: true},
		{name: "flags before group", args: []string{"zqk", "--verbose", "object", "daemon"}, want: true},
		{name: "object get", args: []string{"zqk", "object", "get", "BLI-1"}, want: false},
		{name: "empty", args: nil, want: false},
		{name: "daemon token only", args: []string{"zqk", "daemon"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := PrivilegedWriterDaemonArgv(tc.args); got != tc.want {
				t.Fatalf("PrivilegedWriterDaemonArgv(%v)=%v want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestPrivilegedWriterDaemonRole_ArgvWithoutEnv(t *testing.T) {
	t.Setenv(IsDaemon(), "")
	setCommandArgs(t, []string{"zqk-stable", "object", "daemon"})
	if !PrivilegedWriterDaemonRole() {
		t.Fatal("expected role from argv when IS_DAEMON is unset")
	}
}

func TestPrivilegedWriterDaemonRole_Neither(t *testing.T) {
	t.Setenv(IsDaemon(), "")
	setCommandArgs(t, []string{"zqk", "object", "update", "CRIT-1"})
	if PrivilegedWriterDaemonRole() {
		t.Fatal("expected no role for object update without IS_DAEMON")
	}
}

func TestPrivilegedWriterRequiredLaunchEnv(t *testing.T) {
	t.Parallel()
	env := PrivilegedWriterRequiredLaunchEnv()
	if env[IsDaemon()] != enabledFlagValue {
		t.Fatalf("launch env must pin %s=1, got %#v", IsDaemon(), env)
	}
}

func TestPrivilegedWriterLaunchAgentTemplatePinsDaemonEnv(t *testing.T) {
	t.Parallel()
	root := findGoModRoot(t)
	plist := filepath.Join(root, "scripts", "launchd", "com.zqk.privileged-writer.plist")
	data, err := os.ReadFile(plist)
	if err != nil {
		t.Fatalf("read launchd template: %v", err)
	}
	body := string(data)
	key := "<key>" + IsDaemon() + "</key>"
	idx := strings.Index(body, key)
	if idx < 0 {
		t.Fatalf("template must include EnvironmentVariables key %s", IsDaemon())
	}
	window := body[idx:]
	if len(window) > 160 {
		window = window[:160]
	}
	if !strings.Contains(window, "<string>"+enabledFlagValue+"</string>") {
		t.Fatalf("template must set %s to %s immediately after the key", IsDaemon(), enabledFlagValue)
	}
	if !strings.Contains(body, "<string>"+PrivilegedWriterCLIGroup+"</string>") ||
		!strings.Contains(body, "<string>"+PrivilegedWriterCLIVerb+"</string>") {
		t.Fatal("template ProgramArguments must include object daemon")
	}
}

func setCommandArgs(t *testing.T, args []string) {
	t.Helper()
	orig := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = orig })
}

func findGoModRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found")
	return ""
}
