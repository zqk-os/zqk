package system

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestNewMigrateCasCmd_FlagsAndHelp(t *testing.T) {
	t.Parallel()

	cmd := NewMigrateCasCmd()
	if cmd == nil {
		t.Fatalf("NewMigrateCasCmd returned nil")
	}

	if cmd.Use != "migrate-cas" {
		t.Errorf("cmd.Use = %q, want 'migrate-cas'", cmd.Use)
	}

	flag := cmd.Flags().Lookup("remove-old-files")
	if flag == nil {
		t.Fatalf("remove-old-files flag missing")
	}
	if flag.DefValue != "true" {
		t.Errorf("remove-old-files default = %q, want 'true'", flag.DefValue)
	}
}

func TestRunMigrateCas_EmptyProjectRoot(t *testing.T) {
	// No t.Parallel: uses t.Setenv (Go forbids Parallel + Setenv).

	// In an empty temp directory without project marker, runMigrateCas returns project root error.
	dir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), dir)

	cmd := NewMigrateCasCmd()
	cmd.SetContext(context.Background())

	err := runMigrateCas(cmd, true)
	if err == nil {
		t.Errorf("runMigrateCas in empty dir expected error, got nil")
	}
}

func TestRunMigrateCas_WithTestEnvironment(t *testing.T) {
	// No t.Parallel: uses t.Setenv (Go forbids Parallel + Setenv).

	projectRoot := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), projectRoot)
	if _, err := setupSystemTestEnvironmentRoot(t, projectRoot); err != nil {
		t.Fatalf("setupSystemTestEnvironmentRoot: %v", err)
	}

	// Create storage factory setup (requires .zqk/process layout from setup above)
	factory, err := storage.NewStorageFactory(context.Background(), projectRoot)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	if factory == nil {
		t.Fatalf("StorageFactory is nil")
	}

	cmd := NewMigrateCasCmd()
	cmd.SetContext(context.Background())

	err = runMigrateCas(cmd, true)
	if err != nil {
		t.Logf("runMigrateCas output: %v", err)
	}
}
