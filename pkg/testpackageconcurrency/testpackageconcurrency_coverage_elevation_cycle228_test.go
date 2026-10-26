// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testpackageconcurrency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCycle228_NormalizePackagePath_Exhaustive(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"\t\n ", ""},
		{"./", ""},
		{"  ./  ", ""},
		{"./pkg/storage", "pkg/storage"},
		{"  ./pkg/storage  ", "pkg/storage"},
		{"pkg/storage", "pkg/storage"},
		{"  pkg/storage  ", "pkg/storage"},
		{"./cmd/zqk/sub/tool", "cmd/zqk/sub/tool"},
		{".//double/slash", "/double/slash"},
	}

	for _, tc := range testCases {
		actual := NormalizePackagePath(tc.input)
		if actual != tc.expected {
			t.Errorf("NormalizePackagePath(%q) = %q, expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestCycle228_ReadLimitsMap_NilAndMissing(t *testing.T) {
	t.Parallel()

	// Empty projectRoot returns (nil, nil)
	limits, err := ReadLimitsMap("")
	if err != nil {
		t.Fatalf("expected nil error for empty root, got: %v", err)
	}
	if limits != nil {
		t.Fatalf("expected nil map for empty root, got: %+v", limits)
	}

	// Non-existent projectRoot returns (nil, nil)
	tmp := t.TempDir()
	nonExistentRoot := filepath.Join(tmp, "does_not_exist_subpath")
	limits, err = ReadLimitsMap(nonExistentRoot)
	if err != nil {
		t.Fatalf("expected nil error for missing directory, got: %v", err)
	}
	if limits != nil {
		t.Fatalf("expected nil map for missing directory, got: %+v", limits)
	}
}

func TestCycle228_ReadLimitsMap_ReadErrorAndCorruptedJSON(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	bundlesDir := filepath.Join(tmp, paths.ProjectDataDir, paths.TestBundlesDir)
	if err := fileutil.MkdirAll(bundlesDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Case 1: Target limits file path is a directory (causing ReadFile to fail with read error)
	dirAsFile := filepath.Join(bundlesDir, LimitsFileName)
	if err := fileutil.MkdirAll(dirAsFile, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll directory as file failed: %v", err)
	}
	limits, err := ReadLimitsMap(tmp)
	if err == nil {
		t.Fatalf("expected error reading directory as file, got nil error (limits=%+v)", limits)
	}
	if limits != nil {
		t.Fatalf("expected nil limits map on read error, got: %+v", limits)
	}

	// Case 2: Target limits file is corrupted JSON
	if err := fileutil.Remove(dirAsFile); err != nil {
		t.Fatalf("failed removing directory: %v", err)
	}
	corrupted := []byte(`{ "schema_version": 1, "max_concurrent_jobs_by_package": INVALID_JSON...`)
	if err := fileutil.WriteFile(dirAsFile, corrupted, paths.FilePerm600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	limits, err = ReadLimitsMap(tmp)
	if err == nil {
		t.Fatalf("expected parse error on corrupted json, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected error to mention 'parse', got: %v", err)
	}
	if limits != nil {
		t.Fatalf("expected nil limits map on parse error, got: %+v", limits)
	}
}

func TestCycle228_ReadLimitsMap_NilPackageMapAndFilterValidation(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	bundlesDir := filepath.Join(tmp, paths.ProjectDataDir, paths.TestBundlesDir)
	if err := fileutil.MkdirAll(bundlesDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	filePath := filepath.Join(bundlesDir, LimitsFileName)

	// Case 1: MaxConcurrentJobsByPackage is nil in JSON
	payloadNil := []byte(`{"schema_version": 1, "updated_at": "2026-10-25T00:00:00Z"}`)
	if err := fileutil.WriteFile(filePath, payloadNil, paths.FilePerm600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	limits, err := ReadLimitsMap(tmp)
	if err != nil {
		t.Fatalf("ReadLimitsMap failed: %v", err)
	}
	if limits == nil || len(limits) != 0 {
		t.Fatalf("expected empty non-nil map, got: %+v", limits)
	}

	// Case 2: MaxConcurrentJobsByPackage contains empty keys, negative/zero values, >32 values, and valid values
	payloadValid := limitsFile{
		SchemaVersion: 1,
		UpdatedAt:     "2026-10-25T12:00:00Z",
		MaxConcurrentJobsByPackage: map[string]int{
			"":               4,   // empty key after trim -> skipped
			"./":             5,   // trimmed to empty -> skipped
			"   ":            6,   // whitespace only -> skipped
			"pkg/zero":       0,   // v <= 0 -> skipped
			"pkg/negative":   -3,  // v <= 0 -> skipped
			"./pkg/storage":  8,   // valid normalized key
			"pkg/scheduler":  16,  // valid key
			"pkg/extreme":    999, // clamped to 32
			"./pkg/boundary": 32,  // exact boundary 32
			"pkg/min":        1,   // exact min positive 1
		},
	}
	raw, err := json.Marshal(payloadValid)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if err := fileutil.WriteFile(filePath, raw, paths.FilePerm600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	limits, err = ReadLimitsMap(tmp)
	if err != nil {
		t.Fatalf("ReadLimitsMap failed: %v", err)
	}
	expected := map[string]int{
		"pkg/storage":   8,
		"pkg/scheduler": 16,
		"pkg/extreme":   32,
		"pkg/boundary":  32,
		"pkg/min":       1,
	}
	if len(limits) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %+v", len(expected), len(limits), limits)
	}
	for k, expV := range expected {
		if limits[k] != expV {
			t.Errorf("entry %q: expected %d, got %d", k, expV, limits[k])
		}
	}
}

func TestCycle228_MergeLimitMaps_ExhaustiveCombinations(t *testing.T) {
	t.Parallel()

	// Both nil
	mergedNil := MergeLimitMaps(nil, nil)
	if mergedNil == nil || len(mergedNil) != 0 {
		t.Fatalf("expected empty non-nil map, got %+v", mergedNil)
	}

	// Map a with non-positive values
	mapA := map[string]int{
		"pkg/positive": 10,
		"pkg/zero":     0,
		"pkg/neg":      -5,
	}
	mergedAOnly := MergeLimitMaps(mapA, nil)
	if len(mergedAOnly) != 1 || mergedAOnly["pkg/positive"] != 10 {
		t.Fatalf("expected only pkg/positive=10, got: %+v", mergedAOnly)
	}

	// Map b with non-positive values, new keys, lower values, higher values
	mapB := map[string]int{
		"pkg/zero":      0,  // v <= 0 in b -> skipped
		"pkg/neg":       -2, // v <= 0 in b -> skipped
		"pkg/positive":  4,  // 4 < 10 -> lower wins (4)
		"pkg/positive2": 15, // not in a -> added (15)
	}
	mergedAB := MergeLimitMaps(mapA, mapB)
	if mergedAB["pkg/positive"] != 4 {
		t.Fatalf("expected pkg/positive=4, got %d", mergedAB["pkg/positive"])
	}
	if mergedAB["pkg/positive2"] != 15 {
		t.Fatalf("expected pkg/positive2=15, got %d", mergedAB["pkg/positive2"])
	}
	if _, exists := mergedAB["pkg/zero"]; exists {
		t.Fatalf("expected pkg/zero omitted from merged map")
	}

	mapA2 := map[string]int{
		"pkg/positive":  10,
		"pkg/unchanged": 8, // b has higher value -> 8 preserved
	}
	mapB2 := map[string]int{
		"pkg/positive":  4,
		"pkg/unchanged": 20, // 20 > 8 -> 8 remains
		"pkg/new":       6,
	}
	mergedCombo := MergeLimitMaps(mapA2, mapB2)
	expectedCombo := map[string]int{
		"pkg/positive":  4,
		"pkg/unchanged": 8,
		"pkg/new":       6,
	}
	if len(mergedCombo) != len(expectedCombo) {
		t.Fatalf("expected %d keys, got %d: %+v", len(expectedCombo), len(mergedCombo), mergedCombo)
	}
	for k, v := range expectedCombo {
		if mergedCombo[k] != v {
			t.Errorf("expected merged[%q] = %d, got %d", k, v, mergedCombo[k])
		}
	}
}

func simulateAdaptiveLimit(cpus int, memMB int, envOverride string) int {
	if envOverride != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(envOverride)); err == nil && parsed > 0 {
			if parsed > 32 {
				return 32
			}
			return parsed
		}
	}
	if cpus <= 0 {
		cpus = 1
	}
	if memMB <= 0 {
		memMB = 512
	}
	cpuLimit := cpus * 2
	memLimit := memMB / 256
	limit := cpuLimit
	if memLimit < limit {
		limit = memLimit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 32 {
		limit = 32
	}
	return limit
}

func TestCycle228_AdaptiveLimitCalculation_Simulation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cpus        int
		memMB       int
		envOverride string
		expected    int
	}{
		{cpus: 0, memMB: 0, envOverride: "", expected: 2},
		{cpus: -4, memMB: -100, envOverride: "", expected: 2},
		{cpus: 4, memMB: 2048, envOverride: "", expected: 8},
		{cpus: 16, memMB: 1024, envOverride: "", expected: 4},
		{cpus: 64, memMB: 65536, envOverride: "", expected: 32},
		{cpus: 8, memMB: 4096, envOverride: "12", expected: 12},
		{cpus: 8, memMB: 4096, envOverride: "64", expected: 32},
		{cpus: 8, memMB: 4096, envOverride: "-5", expected: 16},
		{cpus: 8, memMB: 4096, envOverride: "invalid", expected: 16},
	}

	for i, tc := range tests {
		actual := simulateAdaptiveLimit(tc.cpus, tc.memMB, tc.envOverride)
		if actual != tc.expected {
			t.Errorf("case %d (cpus=%d, mem=%d, env=%q): expected %d, got %d",
				i, tc.cpus, tc.memMB, tc.envOverride, tc.expected, actual)
		}
	}
}

type simulatedPackageSemaphore struct {
	mu       sync.Mutex
	capacity int
	active   int
}

func newSimulatedPackageSemaphore(cap int) *simulatedPackageSemaphore {
	if cap <= 0 {
		cap = 1
	}
	if cap > 32 {
		cap = 32
	}
	return &simulatedPackageSemaphore{
		capacity: cap,
	}
}

func (s *simulatedPackageSemaphore) acquire(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		if s.active < s.capacity {
			s.active++
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *simulatedPackageSemaphore) release() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active <= 0 {
		return errors.New("underflow: release called on idle semaphore")
	}
	s.active--
	return nil
}

func (s *simulatedPackageSemaphore) inUse() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func TestCycle228_SemaphoreLifecycle_AcquireTimeoutRelease(t *testing.T) {
	t.Parallel()

	sem := newSimulatedPackageSemaphore(2)
	ctx := context.Background()

	// Acquire up to capacity
	if err := sem.acquire(ctx); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if err := sem.acquire(ctx); err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	if count := sem.inUse(); count != 2 {
		t.Fatalf("expected inUse=2, got %d", count)
	}

	// Third acquire times out
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	err := sem.acquire(timeoutCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}

	// Release one slot and re-acquire
	if err := sem.release(); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if count := sem.inUse(); count != 1 {
		t.Fatalf("expected inUse=1, got %d", count)
	}
	if err := sem.acquire(ctx); err != nil {
		t.Fatalf("re-acquire failed: %v", err)
	}

	// Release both
	if err := sem.release(); err != nil {
		t.Fatalf("release 1 failed: %v", err)
	}
	if err := sem.release(); err != nil {
		t.Fatalf("release 2 failed: %v", err)
	}
	// Underflow error check
	if err := sem.release(); err == nil {
		t.Fatalf("expected underflow error on extra release, got nil")
	}
}

func runStressWorker(ctx context.Context, sem *simulatedPackageSemaphore, readRoot string, ops *atomic.Int64, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	for i := 0; i < 20; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := sem.acquire(ctx); err != nil {
			errCh <- fmt.Errorf("worker acquire: %w", err)
			return
		}

		// Perform read and merge operations under semaphore lock
		limits, err := ReadLimitsMap(readRoot)
		if err != nil {
			errCh <- fmt.Errorf("worker ReadLimitsMap: %w", err)
			if relErr := sem.release(); relErr != nil {
				errCh <- fmt.Errorf("worker release error: %w", relErr)
			}
			return
		}
		merged := MergeLimitMaps(limits, map[string]int{"pkg/stress": 2})
		if merged["pkg/stress"] <= 0 {
			errCh <- errors.New("worker merged limit missing or invalid")
			if relErr := sem.release(); relErr != nil {
				errCh <- fmt.Errorf("worker release error: %w", relErr)
			}
			return
		}

		ops.Add(1)
		time.Sleep(1 * time.Millisecond)

		if err := sem.release(); err != nil {
			errCh <- fmt.Errorf("worker release: %w", err)
			return
		}
	}
}

func TestCycle228_HighContentionStress_GoroutineLabels(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	bundlesDir := filepath.Join(tmp, paths.ProjectDataDir, paths.TestBundlesDir)
	if err := fileutil.MkdirAll(bundlesDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	seed := limitsFile{
		SchemaVersion: 1,
		UpdatedAt:     "2026-10-25T12:00:00Z",
		MaxConcurrentJobsByPackage: map[string]int{
			"pkg/stress": 4,
		},
	}
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(bundlesDir, LimitsFileName), raw, paths.FilePerm600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	sem := newSimulatedPackageSemaphore(4)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var ops atomic.Int64
	var wg sync.WaitGroup
	errCh := make(chan error, 50)
	concurrency := 10

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		workerID := i
		builder := goroutinelabels.NewGoroutine("testpackageconcurrency_stress_worker", fmt.Sprintf("worker-%d", workerID))
		builder.StartSimple(func() {
			runStressWorker(ctx, sem, tmp, &ops, &wg, errCh)
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Errorf("stress worker reported error: %v", err)
		}
	}

	if totalOps := ops.Load(); totalOps < int64(concurrency*10) {
		t.Fatalf("expected at least %d completed operations, got %d", concurrency*10, totalOps)
	}
}
