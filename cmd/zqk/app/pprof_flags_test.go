package app

import (
	"os"
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPprofPersistentFlagsRegistration(t *testing.T) {
	rootCmd := NewRootCommand()

	flags := []string{"cpuprofile", "cpu-profile", "goroutine-profile", "goroutines-profile"}
	for _, flagName := range flags {
		f := rootCmd.PersistentFlags().Lookup(flagName)
		if f == nil {
			t.Fatalf("expected persistent flag --%s to be registered on rootCmd", flagName)
		}
		if !f.Hidden {
			t.Errorf("expected persistent flag --%s to be marked hidden per POL-CODE-015", flagName)
		}
	}

	// Verify inheritance by subcommands
	ensureCommandsRegistered()
	for _, child := range rootCmd.Commands() {
		for _, flagName := range flags {
			f := child.Flag(flagName)
			if f == nil {
				t.Errorf("expected subcommand %s to inherit persistent flag --%s", child.Name(), flagName)
			}
		}
	}
}

func TestWriteGoroutineProfile(t *testing.T) {
	tmpDir := t.TempDir()
	profPath := filepath.Join(tmpDir, "test_goroutine.pprof")

	// Empty filename should be a clean no-op
	if err := writeGoroutineProfile(""); err != nil {
		t.Errorf("expected empty filename to return nil error, got: %v", err)
	}

	// Valid profile dump
	if err := writeGoroutineProfile(profPath); err != nil {
		t.Fatalf("writeGoroutineProfile failed: %v", err)
	}

	info, err := os.Stat(profPath)
	if err != nil {
		t.Fatalf("stat on dumped profile failed: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("expected goroutine profile to be non-empty")
	}

	// Invalid target path should fail
	invalidPath := filepath.Join(tmpDir, "non_existent_dir", "sub", "test.pprof")
	if err := writeGoroutineProfile(invalidPath); err == nil {
		t.Errorf("expected writeGoroutineProfile to fail on invalid directory path, but it succeeded")
	}

	_ = fileutil.Remove(profPath)
}
