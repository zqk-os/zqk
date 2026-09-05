package featureflags

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestFeatureFlags_EnableDisable(t *testing.T) {
	// Not t.Parallel(): isolated temp project uses t.Setenv(ZQK_TEST_ROOT).
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "pkg.featureflags.enable_disable"})
	projectRoot := proj.Root
	flags := NewFeatureFlags(projectRoot)

	// Load flags
	if err := flags.Load(); err != nil {
		t.Fatalf("Failed to load flags: %v", err)
	}

	// Test enable with timeout
	done := make(chan error, 1)
	goroutinelabels.StartTestGoroutine("test_enable_flag", "enabling feature flag in test", func() {
		err := flags.SetEnabled("async_validation", true)
		done <- err
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SetEnabled failed: %v", err)
		}
		if !flags.IsEnabled("async_validation") {
			t.Error("Flag should be enabled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SetEnabled hung - timed out after 5 seconds")
	}

	// Test disable with timeout
	goroutinelabels.StartTestGoroutine("test_disable_flag", "disabling feature flag in test", func() {
		err := flags.SetEnabled("async_validation", false)
		done <- err
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SetEnabled failed: %v", err)
		}
		if flags.IsEnabled("async_validation") {
			t.Error("Flag should be disabled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SetEnabled hung - timed out after 5 seconds")
	}
}

func TestFeatureFlags_Save(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "pkg.featureflags.save"})
	projectRoot := proj.Root
	flags := NewFeatureFlags(projectRoot)
	_ = flags.Load() //nolint:errcheck // Test setup - load errors use default flags

	// Test Save with timeout
	done := make(chan error, 1)
	goroutinelabels.StartTestGoroutine("test_save_flags", "saving feature flags in test", func() {
		err := flags.Save()
		done <- err
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		// Verify file was created (use same path constants as implementation)
		filePath := datacell.FeatureFlagsPath(projectRoot)
		if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
			t.Errorf("Feature flags file was not created at expected path: %s", filePath)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Save hung - timed out after 5 seconds")
	}
}

func TestFeatureFlags_Load(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "pkg.featureflags.load"})
	projectRoot := proj.Root
	flags := NewFeatureFlags(projectRoot)

	// Test Load with timeout
	done := make(chan error, 1)
	goroutinelabels.NewGoroutine("featureflags_test", "load flags").StartSimple(func() {
		err := flags.Load()
		done <- err
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load hung - timed out after 5 seconds")
	}
}

func TestGetGlobalFeatureFlags(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "pkg.featureflags.global"})
	projectRoot := proj.Root

	// Test GetGlobalFeatureFlags with timeout
	done := make(chan *FeatureFlags, 1)
	goroutinelabels.NewGoroutine("featureflags_test", "get global flags").StartSimple(func() {
		flags := GetGlobalFeatureFlags(projectRoot)
		done <- flags
	})

	select {
	case flags := <-done:
		if flags == nil {
			t.Fatal("GetGlobalFeatureFlags returned nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GetGlobalFeatureFlags hung - timed out after 5 seconds")
	}
}
