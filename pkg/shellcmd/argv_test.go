package shellcmd

import (
	"slices"
	"testing"
)

func TestArgv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{
			name:    "blank command yields nil",
			command: "   ",
			want:    nil,
		},
		{
			name:    "plain command splits on whitespace",
			command: "tee -a /tmp/callback.jsonl",
			want:    []string{"tee", "-a", "/tmp/callback.jsonl"},
		},
		{
			name:    "redirect and chain run through a shell",
			command: `tee -a "log.jsonl" >/dev/null && zqk feed steer -m "a b c"`,
			want:    []string{ShellPath, ShellFlagC, `tee -a "log.jsonl" >/dev/null && zqk feed steer -m "a b c"`},
		},
		{
			name:    "quoted argument with spaces runs through a shell",
			command: `notify --message "two words"`,
			want:    []string{ShellPath, ShellFlagC, `notify --message "two words"`},
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

func TestNeedsShell(t *testing.T) {
	t.Parallel()

	shellish := []string{
		"a && b",
		"a | b",
		"a > out",
		"a; b",
		"echo $HOME",
		`say "hi there"`,
		"rm *.log",
	}
	for _, cmd := range shellish {
		if !NeedsShell(cmd) {
			t.Errorf("NeedsShell(%q) = false, want true", cmd)
		}
	}

	plain := []string{"tee -a out.jsonl", "zqk feed doctor", "true"}
	for _, cmd := range plain {
		if NeedsShell(cmd) {
			t.Errorf("NeedsShell(%q) = true, want false", cmd)
		}
	}
}

func TestResolveShell(t *testing.T) {
	t.Parallel()

	// POSIX test
	path, flag := ResolveShell("darwin", "")
	if path != ShellPathPOSIX || flag != ShellFlagCPOSIX {
		t.Errorf("ResolveShell(darwin) = (%s, %s), want (%s, %s)", path, flag, ShellPathPOSIX, ShellFlagCPOSIX)
	}

	path, flag = ResolveShell("linux", "")
	if path != ShellPathPOSIX || flag != ShellFlagCPOSIX {
		t.Errorf("ResolveShell(linux) = (%s, %s), want (%s, %s)", path, flag, ShellPathPOSIX, ShellFlagCPOSIX)
	}

	// Windows fallback (no COMSPEC)
	path, flag = ResolveShell("windows", "")
	if path != ShellPathWindows || flag != ShellFlagCWindows {
		t.Errorf("ResolveShell(windows, empty) = (%s, %s), want (%s, %s)", path, flag, ShellPathWindows, ShellFlagCWindows)
	}

	// Windows with COMSPEC
	path, flag = ResolveShell("windows", `C:\Windows\System32\cmd.exe`)
	if path != `C:\Windows\System32\cmd.exe` || flag != ShellFlagCWindows {
		t.Errorf("ResolveShell(windows, comspec) = (%s, %s), want (C:\\Windows\\System32\\cmd.exe, %s)", path, flag, ShellFlagCWindows)
	}
}

