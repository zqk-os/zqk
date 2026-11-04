// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package stampmemo

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestTable_Load_ErrorPropagation(t *testing.T) {
	var tab Table[string]
	expectedErr := errors.New("simulated load failure")
	loadCount := 0

	failingLoad := func() (string, error) {
		loadCount++
		return "", expectedErr
	}

	val, err := tab.Load("key-err", 100, failingLoad)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if val != "" {
		t.Fatalf("expected empty value on error, got %q", val)
	}
	if loadCount != 1 {
		t.Fatalf("expected loadCount=1, got %d", loadCount)
	}

	// Verify entry was not marked loaded and subsequent load retries
	successLoad := func() (string, error) {
		loadCount++
		return "recovered", nil
	}

	val, err = tab.Load("key-err", 100, successLoad)
	if err != nil {
		t.Fatalf("expected success on retry, got %v", err)
	}
	if val != "recovered" {
		t.Fatalf("expected recovered value, got %q", val)
	}
	if loadCount != 2 {
		t.Fatalf("expected loadCount=2 after retry, got %d", loadCount)
	}
}

func TestTable_Delete(t *testing.T) {
	var tab Table[string]
	loads := 0
	load := func() (string, error) {
		loads++
		return "val", nil
	}

	val, err := tab.Load("k1", 1, load)
	if err != nil || val != "val" || loads != 1 {
		t.Fatalf("initial load failed: val=%q loads=%d err=%v", val, loads, err)
	}

	// Delete existing key
	tab.Delete("k1")

	// Verify Peek misses after Delete
	if _, ok := tab.Peek("k1", 1); ok {
		t.Fatal("expected Peek to miss after Delete")
	}

	// Next Load triggers loadFn again
	val, err = tab.Load("k1", 1, load)
	if err != nil || val != "val" || loads != 2 {
		t.Fatalf("subsequent load failed: val=%q loads=%d err=%v", val, loads, err)
	}

	// Delete non-existent key should not panic
	tab.Delete("non-existent-key")
}

func TestTable_Peek_EdgeCases(t *testing.T) {
	var tab Table[string]

	// 1. Peek on missing key
	if _, ok := tab.Peek("missing", 10); ok {
		t.Fatal("expected Peek on missing key to return false")
	}

	// Load an entry with stamp 10
	val, err := tab.Load("item", 10, func() (string, error) {
		return "cached-value", nil
	})
	if err != nil || val != "cached-value" {
		t.Fatalf("unexpected load err=%v val=%q", err, val)
	}

	// 2. Peek with wrong stamp
	if _, ok := tab.Peek("item", 20); ok {
		t.Fatal("expected Peek with wrong stamp to return false")
	}

	// 3. Peek with matching stamp
	peeked, ok := tab.Peek("item", 10)
	if !ok || peeked != "cached-value" {
		t.Fatalf("expected Peek match, got ok=%v peeked=%q", ok, peeked)
	}
}

func TestTable_Peek_InFlightLockContention(t *testing.T) {
	var tab Table[string]
	inFlightStarted := make(chan struct{})
	releaseInFlight := make(chan struct{})

	// Start a blocking load on a separate goroutine
	goroutinelabels.NewGoroutine("test", "in-flight-peek").StartSimple(func() {
		_, loadErr := tab.Load("blocking-key", 50, func() (string, error) {
			close(inFlightStarted)
			<-releaseInFlight
			return "done", nil
		})
		if loadErr != nil {
			panic(loadErr)
		}
	})

	<-inFlightStarted

	// Peek while load is in-flight must return false immediately (TryLock fails)
	if _, ok := tab.Peek("blocking-key", 50); ok {
		t.Fatal("expected Peek to return false while load is in-flight")
	}

	close(releaseInFlight)

	// Allow time for goroutine to finish and verify Peek succeeds
	var peeked string
	var ok bool
	for range 100 {
		peeked, ok = tab.Peek("blocking-key", 50)
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !ok || peeked != "done" {
		t.Fatalf("expected successful peek after completion, got ok=%v peeked=%q", ok, peeked)
	}
}

func TestPaths_OfAll_EdgeCases(t *testing.T) {
	// Empty argument list returns 0
	if stamp := OfAll(); stamp != 0 {
		t.Fatalf("expected 0 for empty OfAll, got %d", stamp)
	}

	dir := t.TempDir()
	p1 := filepath.Join(dir, "f1.txt")
	p2 := filepath.Join(dir, "f2.txt")
	missing := filepath.Join(dir, "missing.txt")

	if err := fileutil.WriteStandardFile(p1, []byte("data1")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := fileutil.WriteStandardFile(p2, []byte("data2")); err != nil {
		t.Fatal(err)
	}

	s1 := Of(p1)
	s2 := Of(p2)
	if s2 <= s1 {
		t.Fatalf("expected s2 > s1, got s1=%d s2=%d", s1, s2)
	}

	// Mixed existing and missing
	combined := OfAll(missing, p1, p2)
	if combined != s2 {
		t.Fatalf("expected OfAll to return max stamp %d, got %d", s2, combined)
	}
}

func TestPaths_FirstExisting_EdgeCases(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "existing.txt")
	missing := filepath.Join(dir, "missing.txt")

	if err := fileutil.WriteStandardFile(p1, []byte("content")); err != nil {
		t.Fatal(err)
	}

	// 1. Empty slice returns ""
	if got := FirstExisting(nil); got != "" {
		t.Fatalf("expected empty string for nil slice, got %q", got)
	}
	if got := FirstExisting([]string{}); got != "" {
		t.Fatalf("expected empty string for empty slice, got %q", got)
	}

	// 2. Slice containing empty string and missing path
	if got := FirstExisting([]string{"", missing}); got != "" {
		t.Fatalf("expected empty string for missing paths, got %q", got)
	}

	// 3. Slice with missing path followed by existing path
	if got := FirstExisting([]string{"", missing, p1}); got != p1 {
		t.Fatalf("expected %q, got %q", p1, got)
	}
}

func TestPaths_Combine_EdgeCases(t *testing.T) {
	// Empty slice returns 0
	if got := Combine(); got != 0 {
		t.Fatalf("expected 0 for empty Combine, got %d", got)
	}

	// All zeros returns 0
	if got := Combine(0, 0, 0); got != 0 {
		t.Fatalf("expected 0 for all zero Combine, got %d", got)
	}

	// Descending, ascending, and negative values
	if got := Combine(10, 50, 30, 20); got != 50 {
		t.Fatalf("expected max 50, got %d", got)
	}
	if got := Combine(-5, 0, -10); got != 0 {
		t.Fatalf("expected max 0, got %d", got)
	}
}

func TestView_SameBacking_EdgeCases(t *testing.T) {
	buf := []byte{'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd'}

	// 1. Different lengths
	if SameBacking(buf[:5], buf[:6]) {
		t.Fatal("expected false for slices of different lengths")
	}

	// 2. Both zero length
	if !SameBacking([]byte{}, []byte{}) {
		t.Fatal("expected true for empty slices")
	}
	if !SameBacking(nil, []byte{}) {
		t.Fatal("expected true for nil and empty slices")
	}

	// 3. Same slice
	if !SameBacking(buf, buf) {
		t.Fatal("expected true for identical slice")
	}

	// 4. Same length but different backing arrays
	otherBuf := make([]byte, len(buf))
	copy(otherBuf, buf)
	if SameBacking(buf, otherBuf) {
		t.Fatal("expected false for different backing arrays with same content")
	}

	// 5. Same length but shifted offset in same underlying array
	if SameBacking(buf[0:4], buf[1:5]) {
		t.Fatal("expected false for different offsets in same backing array")
	}
}

func TestView_Get_Error(t *testing.T) {
	var view View[string]
	expectedErr := errors.New("parse error")
	parseCount := 0

	failingParse := func(b []byte) (string, error) {
		parseCount++
		return "", expectedErr
	}

	raw := []byte("raw data")
	val, err := view.Get("k-err", raw, failingParse)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if val != "" {
		t.Fatalf("expected empty value on error, got %q", val)
	}
	if parseCount != 1 {
		t.Fatalf("expected parseCount=1, got %d", parseCount)
	}

	// Verify retry succeeds and caches
	successParse := func(b []byte) (string, error) {
		parseCount++
		return string(b), nil
	}

	val, err = view.Get("k-err", raw, successParse)
	if err != nil || val != "raw data" {
		t.Fatalf("expected success, got val=%q err=%v", val, err)
	}
	if parseCount != 2 {
		t.Fatalf("expected parseCount=2, got %d", parseCount)
	}

	// Cached hit
	val, err = view.Get("k-err", raw, successParse)
	if err != nil || val != "raw data" {
		t.Fatalf("expected cached hit, got val=%q err=%v", val, err)
	}
	if parseCount != 2 {
		t.Fatalf("expected cached hit without reparse, got parseCount=%d", parseCount)
	}
}

func TestWalkYAML_EdgeCases(t *testing.T) {
	dir := t.TempDir()

	// 1. Missing directory returns error
	missingDir := filepath.Join(dir, "non-existent")
	err := WalkYAML(missingDir, func(fileID, path string, data []byte) {
		t.Fatalf("unexpected call for missing dir: %s", fileID)
	})
	if err == nil {
		t.Fatal("expected error for missing directory")
	}

	// 2. Directory with subdirectories, dotfiles, non-yaml files, and valid yaml files
	subDir := filepath.Join(dir, "nested_dir.yaml")
	if mkErr := fileutil.EnsureDir(subDir); mkErr != nil {
		t.Fatal(mkErr)
	}

	dotYaml := filepath.Join(dir, ".hidden.yaml")
	if wErr := fileutil.WriteStandardFile(dotYaml, []byte("hidden: true\n")); wErr != nil {
		t.Fatal(wErr)
	}

	txtFile := filepath.Join(dir, "document.txt")
	if wErr := fileutil.WriteStandardFile(txtFile, []byte("not yaml\n")); wErr != nil {
		t.Fatal(wErr)
	}

	jsonFile := filepath.Join(dir, "config.json")
	if wErr := fileutil.WriteStandardFile(jsonFile, []byte("{}\n")); wErr != nil {
		t.Fatal(wErr)
	}

	valid1 := filepath.Join(dir, "alpha.yaml")
	if wErr := fileutil.WriteStandardFile(valid1, []byte("name: alpha\n")); wErr != nil {
		t.Fatal(wErr)
	}

	valid2 := filepath.Join(dir, "beta.yaml")
	if wErr := fileutil.WriteStandardFile(valid2, []byte("name: beta\n")); wErr != nil {
		t.Fatal(wErr)
	}

	// Unreadable file branch
	unreadable := filepath.Join(dir, "unreadable.yaml")
	if wErr := fileutil.WriteStandardFile(unreadable, []byte("forbidden: true\n")); wErr != nil {
		t.Fatal(wErr)
	}
	if chErr := fileutil.Chmod(unreadable, 0000); chErr == nil {
		defer func() {
			restoreErr := fileutil.Chmod(unreadable, 0644)
			if restoreErr != nil {
				t.Logf("failed restoring permissions: %v", restoreErr)
			}
		}()
	}

	found := make(map[string]string)
	walkErr := WalkYAML(dir, func(fileID, path string, data []byte) {
		found[fileID] = string(data)
	})
	if walkErr != nil {
		t.Fatalf("unexpected WalkYAML error: %v", walkErr)
	}

	if len(found) != 2 {
		t.Fatalf("expected exactly 2 yaml files, found %d: %v", len(found), found)
	}
	if found["alpha"] != "name: alpha\n" {
		t.Fatalf("unexpected alpha content: %q", found["alpha"])
	}
	if found["beta"] != "name: beta\n" {
		t.Fatalf("unexpected beta content: %q", found["beta"])
	}
}

func TestFingerprints_EdgeCases(t *testing.T) {
	var fp Fingerprints

	// Initial check
	if fp.Unchanged("doc1", "hash1") {
		t.Fatal("expected false for unrecorded key")
	}

	fp.Remember("doc1", "hash1")
	if !fp.Unchanged("doc1", "hash1") {
		t.Fatal("expected true for recorded matching hash")
	}
	if fp.Unchanged("doc1", "hash2") {
		t.Fatal("expected false for mismatched hash")
	}

	// Update fingerprint
	fp.Remember("doc1", "hash2")
	if !fp.Unchanged("doc1", "hash2") {
		t.Fatal("expected true for updated hash")
	}
	if fp.Unchanged("doc1", "hash1") {
		t.Fatal("expected false for previous hash")
	}
}

func TestTable_ConcurrentAccess(t *testing.T) {
	var tab Table[int]
	var wg sync.WaitGroup
	const workers = 16
	const iterations = 50

	for w := range workers {
		wg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine("test", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", workerID%4)
			for i := range iterations {
				stamp := Stamp(i + 1)
				val, err := tab.Load(key, stamp, func() (int, error) {
					return workerID*1000 + i, nil
				})
				if err != nil {
					t.Errorf("worker %d load err: %v", workerID, err)
					return
				}
				if val < 0 {
					t.Errorf("invalid value %d", val)
				}

				peekVal, peekOk := tab.Peek(key, stamp)
				if peekOk && peekVal < 0 {
					t.Errorf("invalid peek value %d", peekVal)
				}
				if i%10 == 0 {
					tab.Delete(key)
				}
				if i%25 == 0 {
					tab.Reset()
				}
			}
		})
	}

	wg.Wait()
}

func TestFingerprints_ConcurrentAccess(t *testing.T) {
	var fp Fingerprints
	var wg sync.WaitGroup
	const workers = 16
	const iterations = 50

	for w := range workers {
		wg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine("test", fmt.Sprintf("fp-worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			key := fmt.Sprintf("fp-key-%d", workerID)
			for i := range iterations {
				hash := fmt.Sprintf("hash-%d-%d", workerID, i)
				fp.Remember(key, hash)
				if !fp.Unchanged(key, hash) {
					t.Errorf("worker %d expected key %s to have hash %s", workerID, key, hash)
				}
			}
		})
	}

	wg.Wait()
}

func TestView_ConcurrentAccess(t *testing.T) {
	var v View[string]
	var wg sync.WaitGroup
	const workers = 16
	const iterations = 50

	for w := range workers {
		wg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine("test", fmt.Sprintf("view-worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			key := fmt.Sprintf("view-key-%d", workerID%4)
			raw := []byte(fmt.Sprintf("raw-%d", workerID))
			for range iterations {
				val, err := v.Get(key, raw, func(b []byte) (string, error) {
					return string(b), nil
				})
				if err != nil || val != string(raw) {
					t.Errorf("view get failed: val=%q err=%v", val, err)
					return
				}
			}
		})
	}

	wg.Wait()
}
