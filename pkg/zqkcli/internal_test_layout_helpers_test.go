package internal

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storagetesting"
	"github.com/zqk-os/zqk/pkg/testenvroot"

	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1" // Register instance builders for tests
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"          // Register spec builders for tests
)

func init() {
	// Package-scoped init has no *testing.T, so process env is the only scope available here;
	// the key list itself is shared with the t.Setenv call sites.
	zqkenv.ApplyIsolatedStorageEnv(zqkenv.OSEnvSetter)
}

// internalCompleteTestEnv mirrors pkg/testing.TestEnvironment fields used by CRUD baseline tests.
type internalCompleteTestEnv struct {
	TestRoot        string
	Storage         any
	SpecLoader      *objects.SpecLoader
	LifecycleLoader *objects.LifecycleLoader
	SecurityContext *pkgctx.SecurityContext
	Cleanup         func()
}

type internalTestStorageGlobalsConfigurator = storagetesting.GlobalHookConfigurator

func findInternalProjectRootForCompleteEnv() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)); err == nil {
			return dir
		}
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := fileutil.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
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

func copyInternalSpecFilesToTest(testRoot, projectRoot string) error {
	sourceSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	targetSpecsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := fileutil.Stat(sourceSpecsDir); fileutil.IsNotExist(err) {
		return fmt.Errorf("source specs directory does not exist: %s", sourceSpecsDir)
	}
	if err := fileutil.EnsureDir(targetSpecsDir); err != nil {
		return fmt.Errorf("failed to create target specs directory: %w", err)
	}
	entries, err := fileutil.ReadDir(sourceSpecsDir)
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
		data, err := fileutil.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read source file %s: %w", sourcePath, err)
		}
		if err := fileutil.WriteSecureFile(targetPath, data); err != nil { //nolint:gosec // spec files
			return fmt.Errorf("failed to write target file %s: %w", targetPath, err)
		}
	}
	return nil
}

// findInternalProjectRootForSpecs mirrors pkg/testing.findProjectRoot for CopySpecsToInternalTestRoot.
func findInternalProjectRootForSpecs() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)); err == nil {
			return dir
		}
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := fileutil.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
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

// copySpecsToInternalTestRoot copies object spec YAML from the detected project root into testRoot.
func copySpecsToInternalTestRoot(testRoot string) error {
	projectRoot := findInternalProjectRootForSpecs()
	if projectRoot == "" {
		return fmt.Errorf("cannot copy specs: project root not found")
	}
	return copyInternalSpecFilesToTest(testRoot, projectRoot)
}

// setupCompleteTestEnvironmentInternal mirrors pkg/testing.SetupCompleteTestEnvironment with default options.
func setupCompleteTestEnvironmentInternal(t *testing.T, factory storagetesting.IsolationFactory) *internalCompleteTestEnv {
	t.Helper()
	if factory == nil {
		t.Fatalf("IsolationFactory is required")
	}
	testRoot := t.TempDir()
	originalTestRoot := zqkenv.TestRoot().Get()
	if err := zqkenv.TestRoot().Set(testRoot); err != nil {
		t.Fatalf("Setenv TestRoot: %v", err)
	}
	t.Cleanup(func() {
		if originalTestRoot != "" {
			_ = zqkenv.TestRoot().Set(originalTestRoot)
		} else {
			_ = zqkenv.TestRoot().Unset()
		}
	})
	if _, err := testenvroot.Setup(testRoot); err != nil {
		t.Fatalf("testenvroot.Setup: %v", err)
	}
	projectRoot := findInternalProjectRootForCompleteEnv()
	if projectRoot != "" {
		if err := copyInternalSpecFilesToTest(testRoot, projectRoot); err != nil {
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
	if cfg, ok := factory.(internalTestStorageGlobalsConfigurator); ok {
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
	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
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
	return &internalCompleteTestEnv{
		TestRoot:        testRoot,
		Storage:         storageProvider,
		SpecLoader:      specLoader,
		LifecycleLoader: lifecycleLoader,
		SecurityContext: secCtx,
		Cleanup:         cleanup,
	}
}
