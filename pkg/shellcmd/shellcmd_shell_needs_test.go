// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package shellcmd

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestResolveShell_ComprehensiveMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		goos     string
		comspec  string
		wantPath string
		wantFlag string
	}{
		{
			name:     "darwin standard posix",
			goos:     "darwin",
			comspec:  "",
			wantPath: ShellPathPOSIX,
			wantFlag: ShellFlagCPOSIX,
		},
		{
			name:     "linux standard posix",
			goos:     "linux",
			comspec:  "",
			wantPath: ShellPathPOSIX,
			wantFlag: ShellFlagCPOSIX,
		},
		{
			name:     "freebsd standard posix",
			goos:     "freebsd",
			comspec:  "",
			wantPath: ShellPathPOSIX,
			wantFlag: ShellFlagCPOSIX,
		},
		{
			name:     "openbsd standard posix",
			goos:     "openbsd",
			comspec:  "",
			wantPath: ShellPathPOSIX,
			wantFlag: ShellFlagCPOSIX,
		},
		{
			name:     "netbsd standard posix",
			goos:     "netbsd",
			comspec:  "",
			wantPath: ShellPathPOSIX,
			wantFlag: ShellFlagCPOSIX,
		},
		{
			name:     "solaris standard posix",
			goos:     "solaris",
			comspec:  "",
			wantPath: ShellPathPOSIX,
			wantFlag: ShellFlagCPOSIX,
		},
		{
			name:     "posix ignores comspec parameter",
			goos:     "linux",
			comspec:  `C:\Windows\System32\cmd.exe`,
			wantPath: ShellPathPOSIX,
			wantFlag: ShellFlagCPOSIX,
		},
		{
			name:     "windows fallback with empty comspec",
			goos:     "windows",
			comspec:  "",
			wantPath: ShellPathWindows,
			wantFlag: ShellFlagCWindows,
		},
		{
			name:     "windows custom comspec path",
			goos:     "windows",
			comspec:  `C:\Windows\System32\cmd.exe`,
			wantPath: `C:\Windows\System32\cmd.exe`,
			wantFlag: ShellFlagCWindows,
		},
		{
			name:     "windows alternative comspec path",
			goos:     "windows",
			comspec:  `D:\Tools\powershell.exe`,
			wantPath: `D:\Tools\powershell.exe`,
			wantFlag: ShellFlagCWindows,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotPath, gotFlag := ResolveShell(tt.goos, tt.comspec)
			if gotPath != tt.wantPath || gotFlag != tt.wantFlag {
				t.Fatalf("ResolveShell(%q, %q) = (%q, %q), want (%q, %q)",
					tt.goos, tt.comspec, gotPath, gotFlag, tt.wantPath, tt.wantFlag)
			}
		})
	}
}

func TestNeedsShell_EveryMetacharacter(t *testing.T) {
	t.Parallel()

	// Verify each individual character in shellMetaChars triggers NeedsShell.
	for i, r := range shellMetaChars {
		charStr := string(r)
		name := fmt.Sprintf("char_%d_%q", i, charStr)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd := "echo " + charStr
			if !NeedsShell(cmd) {
				t.Fatalf("NeedsShell(%q) = false, want true for metachar %q", cmd, charStr)
			}
		})
	}
}

func TestNeedsShell_SyntaxPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		command   string
		wantShell bool
	}{
		// Clean commands (no shell required)
		{name: "simple binary", command: "ls", wantShell: false},
		{name: "binary with path", command: "/usr/local/bin/zqk", wantShell: false},
		{name: "subcommand arguments", command: "zqk workflow whats-next", wantShell: false},
		{name: "flag arguments", command: "grep --line-number --color", wantShell: false},
		{name: "hyphen underscore dot slash", command: "app --file=/tmp/path_to_file.log", wantShell: false},
		{name: "colon in argument", command: "curl http://localhost:8080/health", wantShell: false},

		// Shell syntax required
		{name: "pipeline", command: "cat file.txt | grep target", wantShell: true},
		{name: "and chain", command: "make build && make test", wantShell: true},
		{name: "or chain", command: "test -f file || touch file", wantShell: true},
		{name: "semicolon separator", command: "echo step1; echo step2", wantShell: true},
		{name: "output redirection write", command: "echo hi > file.txt", wantShell: true},
		{name: "output redirection append", command: "echo hi >> file.txt", wantShell: true},
		{name: "input redirection", command: "cat < input.dat", wantShell: true},
		{name: "subshell grouping", command: "(cd dir && pwd)", wantShell: true},
		{name: "posix variable substitution", command: "echo $HOME", wantShell: true},
		{name: "posix braced variable substitution", command: "echo ${BUILD_DIR}/out", wantShell: true},
		{name: "backtick substitution", command: "echo `date`", wantShell: true},
		{name: "backslash escape", command: "echo hello\\ world", wantShell: true},
		{name: "double quotes", command: `echo "hello world"`, wantShell: true},
		{name: "single quotes", command: "echo 'hello world'", wantShell: true},
		{name: "glob wildcard asterisk", command: "rm -f *.log", wantShell: true},
		{name: "glob wildcard question", command: "ls file?.txt", wantShell: true},
		{name: "glob brackets", command: "ls [a-z]*.go", wantShell: true},
		{name: "shell comment hash", command: "run.sh # trailing comment", wantShell: true},
		{name: "embedded newline", command: "echo line1\necho line2", wantShell: true},
		{name: "windows env variable", command: "echo %USERPROFILE%", wantShell: true},
		{name: "windows caret escape", command: "echo hello^&world", wantShell: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NeedsShell(tt.command)
			if got != tt.wantShell {
				t.Fatalf("NeedsShell(%q) = %v, want %v", tt.command, got, tt.wantShell)
			}
		})
	}
}

func TestArgv_EmptyAndWhitespaceBoundary(t *testing.T) {
	t.Parallel()

	boundaryInputs := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "single space", input: " "},
		{name: "multiple spaces", input: "     "},
		{name: "tab", input: "\t"},
		{name: "multiple tabs", input: "\t\t\t"},
		{name: "newline", input: "\n"},
		{name: "carriage return newline", input: "\r\n"},
		{name: "mixed whitespace tabs spaces newlines", input: "  \t \r\n  \t  \n "},
	}

	for _, bi := range boundaryInputs {
		t.Run(bi.name, func(t *testing.T) {
			t.Parallel()
			got := Argv(bi.input)
			if got != nil {
				t.Fatalf("Argv(%q) = %#v, want nil", bi.input, got)
			}
		})
	}
}

func TestArgv_PlainAndShellDelegation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{
			name:    "single word",
			command: "true",
			want:    []string{"true"},
		},
		{
			name:    "binary with multiple arguments",
			command: "zqk workflow whats-next --json",
			want:    []string{"zqk", "workflow", "whats-next", "--json"},
		},
		{
			name:    "arguments with extra internal whitespace",
			command: "git    status   --short",
			want:    []string{"git", "status", "--short"},
		},
		{
			name:    "pipeline shell wrapping",
			command: "cat /var/log/syslog | grep error",
			want:    []string{ShellPath, ShellFlagC, "cat /var/log/syslog | grep error"},
		},
		{
			name:    "chain with redirection and quotes",
			command: `tee -a log.jsonl >/dev/null && zqk feed steer`,
			want:    []string{ShellPath, ShellFlagC, `tee -a log.jsonl >/dev/null && zqk feed steer`},
		},
		{
			name:    "windows environment variable expansion command",
			command: "echo %COMSPEC%",
			want:    []string{ShellPath, ShellFlagC, "echo %COMSPEC%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Argv(tt.command)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Argv(%q) = %#v, want %#v", tt.command, got, tt.want)
			}
		})
	}
}

func TestConcurrency_ShellcmdRoutines(t *testing.T) {
	t.Parallel()

	const workerCount = 10
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, workerCount*iterations)

	for w := 0; w < workerCount; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("shellcmd_concurrency_worker", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				runConcurrentShellcmdWorker(workerID, iterations, errCh)
			})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrency worker failure: %v", err)
		}
	}
}

func runConcurrentShellcmdWorker(workerID, iterations int, errCh chan<- error) {
	for i := 0; i < iterations; i++ {
		// Test ResolveShell
		posixPath, posixFlag := ResolveShell("linux", "")
		if posixPath != ShellPathPOSIX || posixFlag != ShellFlagCPOSIX {
			errCh <- fmt.Errorf("worker %d: unexpected posix shell (%s, %s)", workerID, posixPath, posixFlag)
			return
		}

		winPath, winFlag := ResolveShell("windows", `C:\cmd.exe`)
		if winPath != `C:\cmd.exe` || winFlag != ShellFlagCWindows {
			errCh <- fmt.Errorf("worker %d: unexpected win shell (%s, %s)", workerID, winPath, winFlag)
			return
		}

		// Test NeedsShell
		if !NeedsShell("cat foo | grep bar") {
			errCh <- fmt.Errorf("worker %d: expected NeedsShell to be true", workerID)
			return
		}
		if NeedsShell("ls -la /tmp") {
			errCh <- fmt.Errorf("worker %d: expected NeedsShell to be false", workerID)
			return
		}

		// Test Argv
		nilArgv := Argv("   \t\n")
		if nilArgv != nil {
			errCh <- fmt.Errorf("worker %d: expected nil argv for blank", workerID)
			return
		}

		plainArgv := Argv("git status")
		if !slices.Equal(plainArgv, []string{"git", "status"}) {
			errCh <- fmt.Errorf("worker %d: plainArgv mismatch: %#v", workerID, plainArgv)
			return
		}

		shellArgv := Argv("echo hello > out.txt")
		expectedShell := []string{ShellPath, ShellFlagC, "echo hello > out.txt"}
		if !slices.Equal(shellArgv, expectedShell) {
			errCh <- fmt.Errorf("worker %d: shellArgv mismatch: %#v", workerID, shellArgv)
			return
		}
	}
}
