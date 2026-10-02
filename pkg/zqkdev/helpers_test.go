package zqkdev

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/logging"
)

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m mockDirEntry) Name() string               { return m.name }
func (m mockDirEntry) IsDir() bool                { return m.isDir }
func (m mockDirEntry) Type() fs.FileMode          { return 0 }
func (m mockDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestShouldProcessYAMLDirEntry(t *testing.T) {
	tests := []struct {
		name        string
		entry       os.DirEntry
		walkErr     error
		wantSkipDir bool
		wantProcess bool
	}{
		{
			name:        "walk error returns false, false",
			entry:       mockDirEntry{name: "item.yaml", isDir: false},
			walkErr:     errors.New("permission denied"),
			wantSkipDir: false,
			wantProcess: false,
		},
		{
			name:        "nil entry returns false, false",
			entry:       nil,
			walkErr:     nil,
			wantSkipDir: false,
			wantProcess: false,
		},
		{
			name:        ".git directory triggers skipDir",
			entry:       mockDirEntry{name: ".git", isDir: true},
			walkErr:     nil,
			wantSkipDir: true,
			wantProcess: false,
		},
		{
			name:        "node_modules directory triggers skipDir",
			entry:       mockDirEntry{name: "node_modules", isDir: true},
			walkErr:     nil,
			wantSkipDir: true,
			wantProcess: false,
		},
		{
			name:        "regular directory does not skip and does not process",
			entry:       mockDirEntry{name: "pkg", isDir: true},
			walkErr:     nil,
			wantSkipDir: false,
			wantProcess: false,
		},
		{
			name:        "AppleDouble file is skipped",
			entry:       mockDirEntry{name: "._spec.yaml", isDir: false},
			walkErr:     nil,
			wantSkipDir: false,
			wantProcess: false,
		},
		{
			name:        "non-yaml file is skipped",
			entry:       mockDirEntry{name: "readme.md", isDir: false},
			walkErr:     nil,
			wantSkipDir: false,
			wantProcess: false,
		},
		{
			name:        "_placeholder.yaml is skipped",
			entry:       mockDirEntry{name: "_placeholder.yaml", isDir: false},
			walkErr:     nil,
			wantSkipDir: false,
			wantProcess: false,
		},
		{
			name:        "hashed yaml filename is skipped",
			entry:       mockDirEntry{name: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.yaml", isDir: false},
			walkErr:     nil,
			wantSkipDir: false,
			wantProcess: false,
		},
		{
			name:        "valid .yaml file is processed",
			entry:       mockDirEntry{name: "goal.yaml", isDir: false},
			walkErr:     nil,
			wantSkipDir: false,
			wantProcess: true,
		},
		{
			name:        "valid .yml file is processed",
			entry:       mockDirEntry{name: "spec.yml", isDir: false},
			walkErr:     nil,
			wantSkipDir: false,
			wantProcess: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotSkipDir, gotProcess := ShouldProcessYAMLDirEntry(tc.entry, tc.walkErr)
			if gotSkipDir != tc.wantSkipDir {
				t.Errorf("ShouldProcessYAMLDirEntry() gotSkipDir = %v, want %v", gotSkipDir, tc.wantSkipDir)
			}
			if gotProcess != tc.wantProcess {
				t.Errorf("ShouldProcessYAMLDirEntry() gotProcess = %v, want %v", gotProcess, tc.wantProcess)
			}
		})
	}
}

func TestLogSummaryAndCheckErrors(t *testing.T) {
	logger := logging.GetLoggerFromProfile(SystemProfileHuman)

	t.Run("zero errors returns nil", func(t *testing.T) {
		err := LogSummaryAndCheckErrors(logger, 10, 2, 0)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("internal alias with zero errors returns nil", func(t *testing.T) {
		err := logSummaryAndCheckErrors(logger, 5, 1, 0)
		if err != nil {
			t.Errorf("expected nil error from alias, got %v", err)
		}
	})

	t.Run("positive errors returns wrapped error", func(t *testing.T) {
		err := LogSummaryAndCheckErrors(logger, 5, 0, 3)
		if err == nil {
			t.Fatal("expected error for positive error count, got nil")
		}
		if expected := "generation completed with 3 errors"; err.Error() != expected {
			t.Errorf("error text = %q, want %q", err.Error(), expected)
		}
	})
}

func TestAddBuilderFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test-builder"}
	var sourceDir, outputDir string
	var overwrite bool

	AddBuilderFlags(cmd, &sourceDir, "source-dir", "pkg/specs", "Source directory", &outputDir, "pkg/gen", &overwrite)

	flags := []string{"source-dir", "output-dir", "overwrite"}
	for _, flagName := range flags {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("flag %q was not registered on command", flagName)
		}
	}
}

func TestCmdLogger(t *testing.T) {
	cmd := &cobra.Command{Use: "test-logger"}
	logger := cmdLogger(cmd)
	if logger == nil {
		t.Fatal("cmdLogger returned nil logger")
	}
}
