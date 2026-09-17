package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestFailureKindAndReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		exitCode       int
		stderr         string
		isTimeout      bool
		timeoutSeconds int
		wantKind       string
		wantReasonSub  string
	}{
		{
			name:           "timeout",
			isTimeout:      true,
			timeoutSeconds: 60,
			wantKind:       "timeout",
			wantReasonSub:  "timed out after 60 seconds",
		},
		{
			name:          "exit 126 shell_exec",
			exitCode:      126,
			stderr:        "/bin/sh: /bin/sh: cannot execute binary file\n",
			wantKind:      "shell_exec",
			wantReasonSub: "exit 126",
		},
		{
			name:          "exit 137 resource exhausted",
			exitCode:      137,
			wantKind:      "resource_exhausted",
			wantReasonSub: "memory budget",
		},
		{
			name:          "oom stderr resource exhausted",
			exitCode:      1,
			stderr:        "runtime: out of memory\n",
			wantKind:      "resource_exhausted",
			wantReasonSub: "out of memory",
		},
		{
			name:          "exit 1 script_exit",
			exitCode:      1,
			stderr:        "grep: no match\n",
			wantKind:      "script_exit",
			wantReasonSub: "exited with code 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, reason := failureKindAndReason(tt.exitCode, tt.stderr, tt.isTimeout, tt.timeoutSeconds)
			if kind != tt.wantKind {
				t.Errorf("failureKindAndReason() kind = %v, want %v", kind, tt.wantKind)
			}
			if tt.wantReasonSub != emptyValue && !strings.Contains(reason, tt.wantReasonSub) {
				t.Errorf("failureKindAndReason() reason = %q, want substring %q", reason, tt.wantReasonSub)
			}
		})
	}
}

func TestIsShellWithInlineScript(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		command string
		args    []string
		want    bool
	}{
		{
			name:    "bin_sh with same path in args",
			command: "/bin/sh",
			args:    []string{"/bin/sh", "-c", "echo ok"},
			want:    true,
		},
		{
			name:    "sh with bin_sh in args (path variation)",
			command: "sh",
			args:    []string{"/bin/sh", "-c", "echo ok"},
			want:    true,
		},
		{
			name:    "bash with same path",
			command: "/usr/bin/bash",
			args:    []string{"/usr/bin/bash", "-c", "echo ok"},
			want:    true,
		},
		{
			name:    "too few args",
			command: "/bin/sh",
			args:    []string{"/bin/sh", "-c"},
			want:    false,
		},
		{
			name:    "args not -c",
			command: "/bin/sh",
			args:    []string{"/bin/sh", "-e", "script"},
			want:    false,
		},
		{
			name:    "non-shell command",
			command: "/usr/bin/echo",
			args:    []string{"/usr/bin/echo", "-c", "x"},
			want:    false,
		},
		{
			name:    "empty args",
			command: "/bin/sh",
			args:    nil,
			want:    false,
		},
		{
			name:    "two-arg form (command=/bin/sh, args=[-c, script])",
			command: "/bin/sh",
			args:    []string{"-c", "go test ./pkg/... > out.log 2>&1"},
			want:    true,
		},
		{
			name:    "two-arg form with bash",
			command: "/usr/bin/bash",
			args:    []string{"-c", "echo hi"},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isShellWithInlineScript(tt.command, tt.args)
			if got != tt.want {
				t.Errorf("isShellWithInlineScript(%q, %v) = %v, want %v", tt.command, tt.args, got, tt.want)
			}
		})
	}
}

func TestIsShellScriptPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		command string
		want    bool
	}{
		{"./scripts/pre-commit-integrity.sh", true},
		{"/usr/local/foo.sh", true},
		{"bar.BASH", true},
		{"echo hello", false},
		{"/bin/sh", false},
		{"", false},
		{"script with spaces.sh", false},
		{"script.sh\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := isShellScriptPath(tt.command)
			if got != tt.want {
				t.Errorf("isShellScriptPath(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestResolveScriptPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "myscript.sh")
	if err := fileutil.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveScriptPath("myscript.sh", dir, "")
	if err != nil {
		t.Errorf("resolveScriptPath(myscript.sh, %q, \"\") err = %v", dir, err)
	}
	if got != scriptPath {
		t.Errorf("resolveScriptPath() = %q, want %q", got, scriptPath)
	}

	// Absolute path
	got, err = resolveScriptPath(scriptPath, dir, "")
	if err != nil {
		t.Errorf("resolveScriptPath(abs) err = %v", err)
	}
	if got != scriptPath {
		t.Errorf("resolveScriptPath(abs) = %q, want %q", got, scriptPath)
	}

	// Non-existent
	_, err = resolveScriptPath("nonexistent.sh", dir, "")
	if err == nil {
		t.Error("resolveScriptPath(nonexistent) wanted error")
	}
}

func TestValidateShellScriptSyntax(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.sh")
	if err := fileutil.WriteFile(validPath, []byte("#!/bin/sh\necho ok\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(dir, "invalid.sh")
	// Use a clear syntax error so sh -n fails on all platforms (unexpected ")")
	if err := fileutil.WriteFile(invalidPath, []byte("#!/bin/sh\n)\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	stderr, err := validateShellScriptSyntax(ctx, &NativeExecutor{}, validPath, dir)
	if err != nil {
		t.Errorf("validateShellScriptSyntax(valid) err = %v", err)
	}
	if stderr != "" {
		t.Errorf("validateShellScriptSyntax(valid) stderr = %q", stderr)
	}

	stderr, err = validateShellScriptSyntax(ctx, &NativeExecutor{}, invalidPath, dir)
	if err == nil {
		t.Error("validateShellScriptSyntax(invalid) wanted error")
	}
	if stderr == emptyValue {
		t.Error("validateShellScriptSyntax(invalid) wanted non-empty stderr")
	}
}

// TestEnsureEnvHasSchedulerTools verifies that all run-wrapper jobs receive a PATH,
// including policy jobs running under a minimal launchd environment.
func TestEnsureEnvHasSchedulerTools(t *testing.T) {
	t.Parallel()
	env := os.Environ()
	got := ensureEnvHasSchedulerTools(env)
	if got == nil {
		t.Fatal("ensureEnvHasSchedulerTools returned nil")
	}
	if len(got) < len(env) {
		t.Errorf("ensureEnvHasSchedulerTools returned fewer entries (%d) than input (%d)", len(got), len(env))
	}
	hasPATH := false
	for _, e := range got {
		if strings.HasPrefix(e, zqkenv.OSPath().Name() + "=") {
			hasPATH = true
			break
		}
	}
	if !hasPATH {
		t.Error("ensureEnvHasSchedulerTools result must contain a PATH= entry")
	}
}

func TestEffectiveRunWrapperTimeoutSeconds(t *testing.T) {
	t.Parallel()
	job := &ScheduledJob{MaxRuntimeSeconds: 90}
	if g := effectiveRunWrapperTimeoutSeconds(30, job); g != 30 {
		t.Fatalf("explicit max: got %d want 30", g)
	}
	if g := effectiveRunWrapperTimeoutSeconds(0, job); g != 90 {
		t.Fatalf("inherit job max: got %d want 90", g)
	}
	if g := effectiveRunWrapperTimeoutSeconds(0, &ScheduledJob{}); g != DefaultMaxRuntimeSeconds {
		t.Fatalf("default: got %d want %d", g, DefaultMaxRuntimeSeconds)
	}
}
