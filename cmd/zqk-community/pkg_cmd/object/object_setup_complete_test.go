package object

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/storagetesting"
	"github.com/lanceman/zqk/pkg/zqkenv"

	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1" // Register instance builders for tests
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"          // Register spec builders for tests
)

// objectCompleteTestEnv mirrors pkg/testing.TestEnvironment fields used by move tests.
type objectCompleteTestEnv struct {
	TestRoot string
	Storage  any
	Cleanup  func()
}

type objectTestStorageGlobalsConfigurator = storagetesting.GlobalHookConfigurator

func objectFindProjectRootForCompleteEnv() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func objectCopySpecFilesToTest(testRoot, projectRoot string) error {
	sourceSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	targetSpecsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := os.Stat(sourceSpecsDir); os.IsNotExist(err) {
		return fmt.Errorf("source specs directory does not exist: %s", sourceSpecsDir)
	}
	if err := fileutil.EnsureDir(targetSpecsDir); err != nil {
		return fmt.Errorf("failed to create target specs directory: %w", err)
	}
	entries, err := os.ReadDir(sourceSpecsDir)
	if err != nil {
		return fmt.Errorf("failed to read source specs directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		sourcePath := filepath.Join(sourceSpecsDir, entry.Name())
		targetPath := filepath.Join(targetSpecsDir, entry.Name())
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read source file %s: %w", sourcePath, err)
		}
		if err := fileutil.WriteSecureFile(targetPath, data); err != nil { //nolint:gosec // spec files
			return fmt.Errorf("failed to write target file %s: %w", targetPath, err)
		}
	}
	return nil
}

// setupCompleteTestEnvironmentObject mirrors pkg/testing.SetupCompleteTestEnvironment with default options.
func setupCompleteTestEnvironmentObject(t *testing.T, factory storagetesting.IsolationFactory) *objectCompleteTestEnv {
	t.Helper()
	if factory == nil {
		t.Fatalf("IsolationFactory is required")
	}
	testRoot := t.TempDir()
	originalTestRoot := os.Getenv(zqkenv.TestRoot())
	if err := os.Setenv(zqkenv.TestRoot(), testRoot); err != nil {
		t.Fatalf("Setenv TestRoot: %v", err)
	}
	t.Cleanup(func() {
		if originalTestRoot != "" {
			_ = os.Setenv(zqkenv.TestRoot(), originalTestRoot)
		} else {
			_ = os.Unsetenv(zqkenv.TestRoot())
		}
	})
	if _, err := setupObjectTestEnvironmentRoot(testRoot); err != nil {
		t.Fatalf("setupObjectTestEnvironmentRoot: %v", err)
	}
	projectRoot := objectFindProjectRootForCompleteEnv()
	if projectRoot != "" {
		if err := objectCopySpecFilesToTest(testRoot, projectRoot); err != nil {
			t.Logf("Warning: Failed to copy spec files (tests may still work): %v", err)
		}
	}
	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.EnsureDir(specsDir); err != nil {
		t.Fatalf("Failed to create specs directory: %v", err)
	}
	if err := builders.NewSpecGenerator(specsDir).GenerateAllSpecs(); err != nil {
		t.Logf("Warning: Failed to generate specs (tests may still work): %v", err)
	}
	if cfg, ok := factory.(objectTestStorageGlobalsConfigurator); ok {
		cfg.ConfigureTestGlobals(t, &storagetesting.GlobalHookOptions{
			NoopStorageMetrics: true,
		})
	}
	buffer := factory.GetAuditBuffer()
	if buffer != nil {
		if buf, ok := buffer.(interface {
			SetEnabled(enabled bool)
			Flush() error
		}); ok {
			buf.SetEnabled(false)
			if err := buf.Flush(); err != nil {
				t.Logf("Warning: Failed to flush buffer before test: %v", err)
			}
		}
	}
	storageProvider, err := factory.CreateFileStorage(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage provider: %v", err)
	}
	_ = objects.NewSpecLoader(testRoot)
	_ = objects.NewLifecycleLoader(testRoot)
	cleanup := func() {
		if storageProvider == nil {
			return
		}
		if s, ok := storageProvider.(interface{ GetTestCleanup() func() }); ok {
			if fn := s.GetTestCleanup(); fn != nil {
				fn()
				return
			}
		}
		if s, ok := storageProvider.(interface{ Shutdown(context.Context) error }); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.Shutdown(ctx)
		}
	}
	t.Cleanup(cleanup)
	return &objectCompleteTestEnv{
		TestRoot: testRoot,
		Storage:  storageProvider,
		Cleanup:  cleanup,
	}
}
