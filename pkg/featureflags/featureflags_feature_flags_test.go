// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package featureflags

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// setupIsolatedTestProject initializes an isolated workspace for feature flags testing.
func setupIsolatedTestProject(t *testing.T, testKind string) (string, *FeatureFlags) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "pkg.featureflags." + testKind,
	})
	ff := NewFeatureFlags(proj.Root)
	return proj.Root, ff
}

// setupExistingFlagsFile writes predefined JSON content to the feature flags file path.
func setupExistingFlagsFile(t *testing.T, projectRoot string, rawJSON []byte) string {
	targetPath := datacell.FeatureFlagsPath(projectRoot)
	dir := filepath.Dir(targetPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory for feature flags: %v", err)
	}
	if err := fileutil.WriteFile(targetPath, rawJSON, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write feature flags file: %v", err)
	}
	return targetPath
}

func TestFeatureFlags_DefaultInitializationAndConstants(t *testing.T) {
	_, ff := setupIsolatedTestProject(t, "defaults")
	if err := ff.Load(); err != nil {
		t.Fatalf("unexpected Load error: %v", err)
	}

	expectedDefaults := map[string]bool{
		FlagAsyncValidation:         false,
		FlagAsyncValidationParallel: false,
		FlagDeferredHashUpdates:     true,
		FlagStorageOrchestration:    true,
	}

	for flagName, expectedState := range expectedDefaults {
		if got := ff.IsEnabled(flagName); got != expectedState {
			t.Errorf("flag %q: got enabled=%v, want %v", flagName, got, expectedState)
		}
		flag, exists := ff.GetFlag(flagName)
		if !exists || flag == nil {
			t.Errorf("flag %q should exist in registry", flagName)
			continue
		}
		if flag.Enabled != expectedState {
			t.Errorf("flag %q struct: got enabled=%v, want %v", flagName, flag.Enabled, expectedState)
		}
		if flag.Description == "" {
			t.Errorf("flag %q should have non-empty description", flagName)
		}
	}

	all := ff.GetAllFlags()
	if len(all) < len(expectedDefaults) {
		t.Errorf("GetAllFlags returned %d items, want at least %d", len(all), len(expectedDefaults))
	}
}

func TestFeatureFlags_LoadExistingValidFile(t *testing.T) {
	projectRoot, ff := setupIsolatedTestProject(t, "valid_file")
	customPayload := `{
		"flags": [
			{
				"name": "async_validation",
				"enabled": true,
				"description": "overridden async validation"
			},
			{
				"name": "custom_organism_mesh",
				"enabled": true,
				"description": "runtime organism dynamic mesh"
			}
		]
	}`

	_ = setupExistingFlagsFile(t, projectRoot, []byte(customPayload))

	if err := ff.Load(); err != nil {
		t.Fatalf("Load should succeed on valid JSON: %v", err)
	}

	if !ff.IsEnabled("async_validation") {
		t.Errorf("async_validation should be true from file override")
	}
	if !ff.IsEnabled("custom_organism_mesh") {
		t.Errorf("custom_organism_mesh should be true from file")
	}
	if !ff.IsEnabled(FlagDeferredHashUpdates) {
		t.Errorf("deferred_hash_updates default should remain initialized")
	}

	flag, ok := ff.GetFlag("custom_organism_mesh")
	if !ok || flag == nil || flag.Description != "runtime organism dynamic mesh" {
		t.Errorf("expected custom_organism_mesh flag metadata preserved")
	}
}

func TestFeatureFlags_UnreadableFileFallback(t *testing.T) {
	projectRoot, ff := setupIsolatedTestProject(t, "unreadable_file")
	targetPath := datacell.FeatureFlagsPath(projectRoot)

	// Create a directory where the file was expected, causing ReadFile to return an error
	if err := fileutil.MkdirAll(targetPath, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory collision: %v", err)
	}

	if err := ff.Load(); err != nil {
		t.Fatalf("Load should fallback to defaults gracefully on read error: %v", err)
	}

	if !ff.IsEnabled(FlagDeferredHashUpdates) {
		t.Errorf("defaults should be initialized despite unreadable file")
	}
}

func TestFeatureFlags_FilesystemErrorBoundaries(t *testing.T) {
	tempDir := t.TempDir()
	blockingFilePath := filepath.Join(tempDir, "blocked_as_file")
	if err := fileutil.WriteFile(blockingFilePath, []byte("file-content"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}

	// filePath inside a regular file path causes MkdirAll to fail
	impossiblePath := filepath.Join(blockingFilePath, "subdir", "feature_flags.json")
	ffBadDir := &FeatureFlags{
		flags:    make(map[string]*FeatureFlag),
		filePath: impossiblePath,
	}

	if err := ffBadDir.Load(); err == nil {
		t.Error("Load should fail when MkdirAll encounters regular file ancestor")
	}

	if err := ffBadDir.Save(); err == nil {
		t.Error("Save should fail when MkdirAll encounters regular file ancestor")
	}

	if err := ffBadDir.SetEnabled("test_fail", true); err == nil {
		t.Error("SetEnabled should fail when saving to impossible path")
	}
}

func TestFeatureFlags_WriteFileError(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "feature_flags.json")
	if err := fileutil.MkdirAll(targetPath, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory at target path: %v", err)
	}

	ff := &FeatureFlags{
		flags:    make(map[string]*FeatureFlag),
		filePath: targetPath,
	}

	if err := ff.Save(); err == nil {
		t.Error("Save should fail when WriteFile encounters existing directory target")
	}
}

func TestFeatureFlags_SetEnabledAndMutation(t *testing.T) {
	_, ff := setupIsolatedTestProject(t, "mutate")
	if err := ff.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Toggle existing default
	if err := ff.SetEnabled(FlagDeferredHashUpdates, false); err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}
	if ff.IsEnabled(FlagDeferredHashUpdates) {
		t.Errorf("deferred_hash_updates should be false after disable")
	}

	// Add brand new custom flag
	customFlag := "progressive_rollout_switch"
	if err := ff.SetEnabled(customFlag, true); err != nil {
		t.Fatalf("SetEnabled failed for new flag: %v", err)
	}
	if !ff.IsEnabled(customFlag) {
		t.Errorf("new flag should be enabled")
	}

	// Toggle back to false
	if err := ff.SetEnabled(customFlag, false); err != nil {
		t.Fatalf("SetEnabled toggle failed: %v", err)
	}
	if ff.IsEnabled(customFlag) {
		t.Errorf("new flag should be disabled")
	}

	// Verify deep copy isolation from GetAllFlags
	all := ff.GetAllFlags()
	copiedFlag, exists := all[customFlag]
	if !exists || copiedFlag == nil {
		t.Fatalf("flag %q should be in GetAllFlags", customFlag)
	}
	copiedFlag.Enabled = true // mutate copy
	if ff.IsEnabled(customFlag) {
		t.Errorf("modifying map from GetAllFlags should not mutate internal state")
	}
}

func TestFeatureFlags_GlobalSingletonMemoization(t *testing.T) {
	proj1 := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "pkg.featureflags.memo1",
	})
	proj2 := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "pkg.featureflags.memo2",
	})

	inst1A := GetGlobalFeatureFlags(proj1.Root)
	inst1B := GetGlobalFeatureFlags(proj1.Root)
	inst2 := GetGlobalFeatureFlags(proj2.Root)

	if inst1A == nil || inst1B == nil || inst2 == nil {
		t.Fatal("GetGlobalFeatureFlags must not return nil")
	}
	if inst1A != inst1B {
		t.Errorf("same project root should return identical memoized instance")
	}
	if inst1A == inst2 {
		t.Errorf("different project roots must return separate instances")
	}
}

func TestFeatureFlags_ConcurrentStress(t *testing.T) {
	_, ff := setupIsolatedTestProject(t, "concurrency")
	if err := ff.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	workers := 10
	iterations := 50
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("featureflags_stress", fmt.Sprintf("worker_%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				flagName := fmt.Sprintf("dynamic_flag_%d", j%5)
				shouldEnable := (j+workerID)%2 == 0

				setErr := ff.SetEnabled(flagName, shouldEnable)
				if setErr != nil {
					t.Errorf("concurrent SetEnabled failed: %v", setErr)
				}

				_ = ff.IsEnabled(flagName)
				_, _ = ff.GetFlag(flagName)
				_ = ff.GetAllFlags()
			}
		})
	}

	wg.Wait()
}

// SubscriberEvent represents an emitted state change event for dynamic toggles.
type SubscriberEvent struct {
	FlagName string
	Enabled  bool
}

// ObservableFeatureFlags wraps FeatureFlags with a listener subscription mechanism.
type ObservableFeatureFlags struct {
	mu        sync.RWMutex
	ff        *FeatureFlags
	listeners []chan SubscriberEvent
}

func newObservableFeatureFlags(ff *FeatureFlags) *ObservableFeatureFlags {
	return &ObservableFeatureFlags{
		ff: ff,
	}
}

func (o *ObservableFeatureFlags) Subscribe() chan SubscriberEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	ch := make(chan SubscriberEvent, 10)
	o.listeners = append(o.listeners, ch)
	return ch
}

func (o *ObservableFeatureFlags) Unsubscribe(target chan SubscriberEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i, ch := range o.listeners {
		if ch == target {
			o.listeners = append(o.listeners[:i], o.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

func (o *ObservableFeatureFlags) SetEnabled(name string, enabled bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ff.SetEnabled(name, enabled); err != nil {
		return err
	}
	evt := SubscriberEvent{FlagName: name, Enabled: enabled}
	for _, ch := range o.listeners {
		select {
		case ch <- evt:
		default:
		}
	}
	return nil
}

func TestFeatureFlags_SubscriberEventDispatch(t *testing.T) {
	_, ff := setupIsolatedTestProject(t, "subscriber")
	if err := ff.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	observable := newObservableFeatureFlags(ff)
	sub1 := observable.Subscribe()
	sub2 := observable.Subscribe()

	testFlag := "event_driven_toggle"
	if err := observable.SetEnabled(testFlag, true); err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}

	select {
	case evt := <-sub1:
		if evt.FlagName != testFlag || !evt.Enabled {
			t.Errorf("sub1 received invalid event: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub1 timed out waiting for subscriber event")
	}

	select {
	case evt := <-sub2:
		if evt.FlagName != testFlag || !evt.Enabled {
			t.Errorf("sub2 received invalid event: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub2 timed out waiting for subscriber event")
	}

	observable.Unsubscribe(sub1)
	if err := observable.SetEnabled(testFlag, false); err != nil {
		t.Fatalf("SetEnabled disable failed: %v", err)
	}

	select {
	case evt := <-sub2:
		if evt.FlagName != testFlag || evt.Enabled {
			t.Errorf("sub2 received invalid disable event: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub2 timed out waiting for disable event")
	}

	observable.Unsubscribe(sub2)
}

func parseEnvFlagOverride(rawEnvValue string, defaultValue bool) bool {
	norm := strings.TrimSpace(strings.ToLower(rawEnvValue))
	switch norm {
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return defaultValue
	}
}

func applyEnvironmentFlagOverrides(ff *FeatureFlags, envPrefix string) error {
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		if strings.HasPrefix(key, envPrefix) {
			flagKey := strings.ToLower(strings.TrimPrefix(key, envPrefix))
			current := ff.IsEnabled(flagKey)
			overridden := parseEnvFlagOverride(val, current)
			if err := ff.SetEnabled(flagKey, overridden); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestFeatureFlags_EnvironmentOverridesAndNormalization(t *testing.T) {
	_, ff := setupIsolatedTestProject(t, "env_overrides")
	if err := ff.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	t.Setenv("ZQK_TEST_FEATURE_ASYNC_VALIDATION", "true")
	t.Setenv("ZQK_TEST_FEATURE_DEFERRED_HASH_UPDATES", "off")
	t.Setenv("ZQK_TEST_FEATURE_NEW_ORGANISM_TOGGLE", "1")
	t.Setenv("ZQK_TEST_FEATURE_INVALID_FALLBACK", "not_a_boolean")

	if err := applyEnvironmentFlagOverrides(ff, "ZQK_TEST_FEATURE_"); err != nil {
		t.Fatalf("applyEnvironmentFlagOverrides failed: %v", err)
	}

	if !ff.IsEnabled("async_validation") {
		t.Errorf("async_validation should be enabled by env override")
	}
	if ff.IsEnabled("deferred_hash_updates") {
		t.Errorf("deferred_hash_updates should be disabled by 'off' env override")
	}
	if !ff.IsEnabled("new_organism_toggle") {
		t.Errorf("new_organism_toggle should be enabled by '1' env override")
	}
	if ff.IsEnabled("invalid_fallback") {
		t.Errorf("invalid boolean should fallback to default (false)")
	}
}

// VariantEvaluation simulates multi-variant flag evaluation (string/number/percentage rollout).
type VariantEvaluation struct {
	VariantKey   string
	NumericQuota int
}

func evaluateVariantFlag(ff *FeatureFlags, flagName string, rawVariant string) VariantEvaluation {
	if !ff.IsEnabled(flagName) {
		return VariantEvaluation{VariantKey: "control", NumericQuota: 0}
	}
	parts := strings.Split(rawVariant, ":")
	if len(parts) == 2 {
		num, err := strconv.Atoi(parts[1])
		if err == nil {
			return VariantEvaluation{VariantKey: parts[0], NumericQuota: num}
		}
	}
	return VariantEvaluation{VariantKey: rawVariant, NumericQuota: 100}
}

func TestFeatureFlags_VariantAndDynamicEvaluation(t *testing.T) {
	_, ff := setupIsolatedTestProject(t, "variants")
	if err := ff.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	flagName := "experimental_scheduler_v2"

	// Control when disabled
	evalControl := evaluateVariantFlag(ff, flagName, "canary:25")
	if evalControl.VariantKey != "control" || evalControl.NumericQuota != 0 {
		t.Errorf("disabled flag must evaluate to control variant, got: %+v", evalControl)
	}

	// Enabled with percentage variant
	if err := ff.SetEnabled(flagName, true); err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}

	evalCanary := evaluateVariantFlag(ff, flagName, "canary:25")
	if evalCanary.VariantKey != "canary" || evalCanary.NumericQuota != 25 {
		t.Errorf("enabled flag must evaluate to specified variant, got: %+v", evalCanary)
	}

	// Simple variant without numeric weight
	evalTreatment := evaluateVariantFlag(ff, flagName, "treatment_b")
	if evalTreatment.VariantKey != "treatment_b" || evalTreatment.NumericQuota != 100 {
		t.Errorf("variant without weight should default to 100 quota, got: %+v", evalTreatment)
	}
}

func TestFeatureFlags_LockOperationConstants(t *testing.T) {
	lockNames := []string{
		LockNameFeatureFlagsGetAll,
		LockNameFeatureFlagsGetFlag,
		LockNameFeatureFlagsIsEnabled,
		LockNameFeatureFlagsLoad,
		LockNameFeatureFlagsSave,
		LockNameFeatureFlagsSetEnabled,
	}

	seen := make(map[string]bool)
	for _, name := range lockNames {
		if strings.TrimSpace(name) == "" {
			t.Errorf("lock name must not be empty")
		}
		if seen[name] {
			t.Errorf("duplicate lock name detected: %q", name)
		}
		seen[name] = true
	}
}

func TestFeatureFlags_EmptyAndUnknownLookups(t *testing.T) {
	_, ff := setupIsolatedTestProject(t, "empty_lookups")
	if err := ff.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if ff.IsEnabled("") {
		t.Errorf("empty flag name must evaluate to false")
	}

	flag, exists := ff.GetFlag("")
	if exists || flag != nil {
		t.Errorf("empty flag name must return exists=false and nil flag")
	}

	// Save empty name and verify it behaves predictably
	if err := ff.SetEnabled("", true); err != nil {
		t.Fatalf("SetEnabled with empty name should not panic: %v", err)
	}

	if !ff.IsEnabled("") {
		t.Errorf("empty flag name set to true should return true")
	}
}
