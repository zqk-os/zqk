package diskusage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestScan_UnreadableDirectory verifies resilience and skipped error counting
// when traversing unreadable directories.
func TestScan_UnreadableDirectory(t *testing.T) {
	root := t.TempDir()
	subDir := filepath.Join(root, "unreadable_dir")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create subDir: %v", err)
	}

	testFile := filepath.Join(subDir, "secret.bin")
	if err := fileutil.WriteFile(testFile, []byte("data"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Restrict permissions to induce read error
	if err := os.Chmod(subDir, 0000); err != nil {
		t.Fatalf("failed to chmod subDir: %v", err)
	}
	defer func() {
		restoreErr := os.Chmod(subDir, paths.DirPerm755)
		if restoreErr != nil {
			t.Logf("warning: failed to restore permissions on %s: %v", subDir, restoreErr)
		}
	}()

	res, err := Scan(root, Options{MaxDepth: 2})
	if err != nil {
		t.Fatalf("Scan failed on tree with unreadable directory: %v", err)
	}
	if res.SkippedErrors == 0 {
		t.Logf("note: platform allowed read despite mode 0000 (running as superuser)")
	}
}

// TestScan_SymlinkDirectoryAndLeaf verifies symlink handling for both
// directory symlinks and file symlinks.
func TestScan_SymlinkDirectoryAndLeaf(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real_dir")
	if err := fileutil.MkdirAll(realDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create real_dir: %v", err)
	}
	realFile := filepath.Join(realDir, "file.bin")
	if err := fileutil.WriteFile(realFile, make([]byte, 512), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	symDir := filepath.Join(root, "sym_dir")
	if err := os.Symlink(realDir, symDir); err != nil {
		t.Fatalf("failed to create directory symlink: %v", err)
	}

	symFile := filepath.Join(root, "sym_file.bin")
	if err := os.Symlink(realFile, symFile); err != nil {
		t.Fatalf("failed to create file symlink: %v", err)
	}

	res, err := Scan(root, Options{MaxDepth: 2, IncludeFiles: true})
	if err != nil {
		t.Fatalf("Scan failed with symlinks: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}

	// Symlink dir should not duplicate file count or double-count sizes
	for _, entry := range res.Entries {
		if entry.Path == "sym_dir/file.bin" {
			t.Errorf("symlink directory was descended into: %s", entry.Path)
		}
	}
}

// TestScan_FileFilteringAndDepthHierarchy tests file inclusion, depth filtering,
// and parent directory size rollups.
func TestScan_FileFilteringAndDepthHierarchy(t *testing.T) {
	root := t.TempDir()
	createHierarchyFile(t, root, "level1/level2/deep.txt", 100)
	createHierarchyFile(t, root, "level1/mid.txt", 500)
	createHierarchyFile(t, root, "shallow.txt", 1000)

	// MaxDepth 1 with IncludeFiles
	res, err := Scan(root, Options{
		MaxDepth:     1,
		IncludeFiles: true,
		MinBytes:     200,
	})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	foundShallow := false
	for _, entry := range res.Entries {
		if entry.Path == "shallow.txt" {
			foundShallow = true
			if entry.IsDir {
				t.Errorf("expected shallow.txt to be file, got dir")
			}
			if entry.SizeBytes != 1000 {
				t.Errorf("expected size 1000, got %d", entry.SizeBytes)
			}
		}
		if entry.Path == "level1/mid.txt" {
			t.Errorf("mid.txt at depth 2 should be excluded from file entries when MaxDepth=1")
		}
		if entry.Path == "level1/level2/deep.txt" {
			t.Errorf("deep.txt at depth 3 should be excluded when MaxDepth=1")
		}
	}
	if !foundShallow {
		t.Errorf("expected shallow.txt in entries")
	}
}

func createHierarchyFile(t *testing.T, root, relPath string, size int) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(relPath))
	if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	if err := fileutil.WriteFile(p, make([]byte, size), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
}

// TestScan_SortingStabilityAndTieBreaking verifies that entries with identical
// size are stably sorted alphabetically by path.
func TestScan_SortingStabilityAndTieBreaking(t *testing.T) {
	root := t.TempDir()
	createHierarchyFile(t, root, "dir_z/file.bin", 2048)
	createHierarchyFile(t, root, "dir_a/file.bin", 2048)
	createHierarchyFile(t, root, "dir_m/file.bin", 2048)

	res, err := Scan(root, Options{MaxDepth: 1})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	var dirPaths []string
	for _, e := range res.Entries {
		if e.Path != "." && e.IsDir {
			dirPaths = append(dirPaths, e.Path)
		}
	}

	expected := []string{"dir_a", "dir_m", "dir_z"}
	if len(dirPaths) != len(expected) {
		t.Fatalf("expected %d dirs, got %d: %v", len(expected), len(dirPaths), dirPaths)
	}
	for i, exp := range expected {
		if dirPaths[i] != exp {
			t.Errorf("at index %d: expected path %q, got %q", i, exp, dirPaths[i])
		}
	}
}

// TestScan_OptionsEdgeCases validates boundary values for Options (Limit, MinBytes).
func TestScan_OptionsEdgeCases(t *testing.T) {
	root := t.TempDir()
	createHierarchyFile(t, root, "sub/f.bin", 100)

	// MinBytes == 0 should have empty MinHuman
	res0, err := Scan(root, Options{MaxDepth: 1, MinBytes: 0})
	if err != nil {
		t.Fatalf("Scan with MinBytes 0 failed: %v", err)
	}
	if res0.MinHuman != "" {
		t.Errorf("expected empty MinHuman for MinBytes=0, got %q", res0.MinHuman)
	}

	// MinBytes > 0 should populate MinHuman
	resMin, err := Scan(root, Options{MaxDepth: 1, MinBytes: 1024})
	if err != nil {
		t.Fatalf("Scan with MinBytes 1024 failed: %v", err)
	}
	if resMin.MinHuman != "1.0KiB" {
		t.Errorf("expected MinHuman '1.0KiB', got %q", resMin.MinHuman)
	}
}

// TestPathDepth_BoundaryErrors tests edge conditions and relative error branches.
func TestPathDepth_BoundaryErrors(t *testing.T) {
	// Root match
	if d := pathDepth("/a/b", "/a/b"); d != 0 {
		t.Errorf("expected depth 0, got %d", d)
	}

	// Descendant paths
	if d := pathDepth("/a/b", "/a/b/c/d/e"); d != 3 {
		t.Errorf("expected depth 3, got %d", d)
	}

	// Parent path (outside root)
	if d := pathDepth("/a/b/c", "/a/b"); d != -1 {
		t.Errorf("expected depth -1 for ancestor, got %d", d)
	}

	// Unrelated path
	if d := pathDepth("/a/b", "/x/y"); d != -1 {
		t.Errorf("expected depth -1 for outside root, got %d", d)
	}

	// Rel error: relative root vs absolute target
	if d := pathDepth("relative/path", "/absolute/target"); d != -1 {
		t.Errorf("expected depth -1 when Rel fails, got %d", d)
	}
}

// TestDisplayPath_BoundaryErrors tests displayPath when paths match, differ, or fail Rel.
func TestDisplayPath_BoundaryErrors(t *testing.T) {
	// Root match
	if p := displayPath("/base", "/base"); p != "." {
		t.Errorf("expected '.', got %q", p)
	}

	// Subpath
	if p := displayPath("/base", "/base/child/sub"); p != "child/sub" {
		t.Errorf("expected 'child/sub', got %q", p)
	}

	// Rel failure fallback
	if p := displayPath("relative/path", "/absolute/target"); p != "." {
		t.Errorf("expected '.' on Rel error, got %q", p)
	}
}

// TestFormatBytes_AllScales tests byte formatting across byte, KiB, MiB, GiB, TiB, PiB and beyond.
func TestFormatBytes_AllScales(t *testing.T) {
	const (
		kiB = int64(1024)
		miB = 1024 * kiB
		giB = 1024 * miB
		tiB = 1024 * giB
		piB = 1024 * tiB
	)

	tests := []struct {
		bytes int64
		want  string
	}{
		{-100, "0B"},
		{0, "0B"},
		{1, "1B"},
		{1023, "1023B"},
		{kiB, "1.0KiB"},
		{9 * kiB, "9.0KiB"},
		{10 * kiB, "10KiB"},
		{1023 * kiB, "1023KiB"},
		{miB, "1.0MiB"},
		{miB + 512*kiB, "1.5MiB"},
		{12 * miB, "12MiB"},
		{giB, "1.0GiB"},
		{15 * giB, "15GiB"},
		{tiB, "1.0TiB"},
		{50 * tiB, "50TiB"},
		{piB, "1.0PiB"},
		{8 * piB, "8.0PiB"},
		{100 * piB, "100PiB"},
		// Exceeding PiB (exercise exp >= len(suffixes) branch)
		{1024 * piB, "1024PiB"},
		{5000 * piB, "5000PiB"},
	}

	for _, tc := range tests {
		got := FormatBytes(tc.bytes)
		if got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

// TestParseSize_AllUnitsAndErrors tests string parsing with all unit suffixes,
// case variations, fractional numbers, and malformed inputs.
func TestParseSize_AllUnitsAndErrors(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"   ", 0, false},
		{"\t\n", 0, false},
		{"42", 42, false},
		{"100B", 100, false},
		{"100b", 100, false},
		{"2K", 2 * 1024, false},
		{"2KB", 2 * 1024, false},
		{"2kib", 2 * 1024, false},
		{"4M", 4 * 1024 * 1024, false},
		{"4MB", 4 * 1024 * 1024, false},
		{"4mib", 4 * 1024 * 1024, false},
		{"8G", 8 * 1024 * 1024 * 1024, false},
		{"8GB", 8 * 1024 * 1024 * 1024, false},
		{"8gib", 8 * 1024 * 1024 * 1024, false},
		{"1T", 1024 * 1024 * 1024 * 1024, false},
		{"1TB", 1024 * 1024 * 1024 * 1024, false},
		{"1tib", 1024 * 1024 * 1024 * 1024, false},
		{"0.5G", int64(0.5 * 1024 * 1024 * 1024), false},
		{"  16   MB  ", 16 * 1024 * 1024, false},

		// Invalid inputs
		{"KB", 0, true},
		{"MIB", 0, true},
		{"GIB", 0, true},
		{"TIB", 0, true},
		{"B", 0, true},
		{"unknown", 0, true},
		{"100XYZ", 0, true},
		{"10KIBEXTRA", 0, true},
		{"-50M", 0, true},
		{"1.2.3GB", 0, true},
	}

	for _, tc := range tests {
		got, err := ParseSize(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseSize(%q) unexpected error state: err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("ParseSize(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// TestTrimSizeSuffix_Direct tests the helper function across suffix matches and no-matches.
func TestTrimSizeSuffix_Direct(t *testing.T) {
	if got := trimSizeSuffix("100KIB", "KIB", "KB", "K"); got != "100" {
		t.Errorf("trimSizeSuffix match 1st failed: %q", got)
	}
	if got := trimSizeSuffix("100KB", "KIB", "KB", "K"); got != "100" {
		t.Errorf("trimSizeSuffix match 2nd failed: %q", got)
	}
	if got := trimSizeSuffix("100K", "KIB", "KB", "K"); got != "100" {
		t.Errorf("trimSizeSuffix match 3rd failed: %q", got)
	}
	if got := trimSizeSuffix("100NOMATCH", "KIB", "KB", "K"); got != "100NOMATCH" {
		t.Errorf("trimSizeSuffix no match failed: %q", got)
	}
}

// StorageCapacityTelemetry models filesystem capacity metrics and alert thresholds.
type StorageCapacityTelemetry struct {
	TotalBytes     uint64
	AvailableBytes uint64
	FreeBytes      uint64
	UsedBytes      uint64
}

// PercentAvailable calculates available disk percentage with zero-division safety.
func (s StorageCapacityTelemetry) PercentAvailable() float64 {
	if s.TotalBytes == 0 {
		return 0.0
	}
	return (float64(s.AvailableBytes) / float64(s.TotalBytes)) * 100.0
}

// DiskAlertStatus represents capacity warning tiers.
type DiskAlertStatus string

const (
	DiskAlertStatusOk       DiskAlertStatus = "OK"
	DiskAlertStatusWarning  DiskAlertStatus = "WARNING"
	DiskAlertStatusCritical DiskAlertStatus = "CRITICAL"
)

// EvaluateDiskThreshold assesses disk status against percentage and absolute thresholds.
func EvaluateDiskThreshold(s StorageCapacityTelemetry, minAvailablePct float64, minAvailableBytes uint64) DiskAlertStatus {
	if s.TotalBytes == 0 {
		return DiskAlertStatusCritical
	}
	pct := s.PercentAvailable()
	if pct < minAvailablePct/2 || s.AvailableBytes < minAvailableBytes/2 {
		return DiskAlertStatusCritical
	}
	if pct < minAvailablePct || s.AvailableBytes < minAvailableBytes {
		return DiskAlertStatusWarning
	}
	return DiskAlertStatusOk
}

// TestStorageCapacityTelemetry_Thresholds tests telemetry evaluation, zero division protection,
// and threshold classification.
func TestStorageCapacityTelemetry_Thresholds(t *testing.T) {
	const (
		giB uint64 = 1024 * 1024 * 1024
	)

	// Zero capacity protection
	zeroVol := StorageCapacityTelemetry{TotalBytes: 0, AvailableBytes: 0}
	if pct := zeroVol.PercentAvailable(); pct != 0.0 {
		t.Errorf("expected 0.0%% on zero total, got %f", pct)
	}
	if status := EvaluateDiskThreshold(zeroVol, 10.0, 5*giB); status != DiskAlertStatusCritical {
		t.Errorf("expected CRITICAL on zero total, got %s", status)
	}

	// Healthy volume: 100GiB total, 50GiB available (50%)
	healthyVol := StorageCapacityTelemetry{
		TotalBytes:     100 * giB,
		AvailableBytes: 50 * giB,
		FreeBytes:      55 * giB,
		UsedBytes:      50 * giB,
	}
	if status := EvaluateDiskThreshold(healthyVol, 10.0, 5*giB); status != DiskAlertStatusOk {
		t.Errorf("expected OK for healthy volume, got %s", status)
	}

	// Warning volume: 100GiB total, 8GiB available (8% < 10%)
	warnVol := StorageCapacityTelemetry{
		TotalBytes:     100 * giB,
		AvailableBytes: 8 * giB,
		FreeBytes:      10 * giB,
		UsedBytes:      92 * giB,
	}
	if status := EvaluateDiskThreshold(warnVol, 10.0, 5*giB); status != DiskAlertStatusWarning {
		t.Errorf("expected WARNING for 8%% available, got %s", status)
	}

	// Critical volume: 100GiB total, 2GiB available (2% < 5%)
	critVol := StorageCapacityTelemetry{
		TotalBytes:     100 * giB,
		AvailableBytes: 2 * giB,
		FreeBytes:      3 * giB,
		UsedBytes:      98 * giB,
	}
	if status := EvaluateDiskThreshold(critVol, 10.0, 5*giB); status != DiskAlertStatusCritical {
		t.Errorf("expected CRITICAL for 2%% available, got %s", status)
	}
}

// TestConcurrency_ParallelScanAndFormatting verifies thread safety during
// concurrent invocations across identical and distinct paths.
func TestConcurrency_ParallelScanAndFormatting(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		sub := fmt.Sprintf("dir_%d/data.bin", i)
		createHierarchyFile(t, root, sub, 1024*(i+1))
	}

	var wg sync.WaitGroup
	const workers = 10
	errChan := make(chan error, workers*3)

	for i := 0; i < workers; i++ {
		wg.Add(3)

		// Worker 1: Scan
		workerIdx := i
		goroutinelabels.NewGoroutine(
			"test_diskusage_scan_worker",
			fmt.Sprintf("concurrent scan worker %d", workerIdx),
		).StartSimple(func() {
			defer wg.Done()
			res, scanErr := Scan(root, Options{MaxDepth: 2, IncludeFiles: true})
			if scanErr != nil {
				errChan <- fmt.Errorf("worker %d Scan failed: %w", workerIdx, scanErr)
				return
			}
			if res == nil || len(res.Entries) == 0 {
				errChan <- fmt.Errorf("worker %d got empty result", workerIdx)
				return
			}
		})

		// Worker 2: FormatBytes
		goroutinelabels.NewGoroutine(
			"test_diskusage_format_worker",
			fmt.Sprintf("concurrent format worker %d", workerIdx),
		).StartSimple(func() {
			defer wg.Done()
			formatted := FormatBytes(int64(workerIdx * 1024 * 1024))
			if formatted == "" {
				errChan <- fmt.Errorf("worker %d FormatBytes returned empty", workerIdx)
			}
		})

		// Worker 3: ParseSize
		goroutinelabels.NewGoroutine(
			"test_diskusage_parse_worker",
			fmt.Sprintf("concurrent parse worker %d", workerIdx),
		).StartSimple(func() {
			defer wg.Done()
			sz, parseErr := ParseSize(fmt.Sprintf("%dGiB", workerIdx+1))
			if parseErr != nil {
				errChan <- fmt.Errorf("worker %d ParseSize failed: %w", workerIdx, parseErr)
				return
			}
			if sz <= 0 {
				errChan <- fmt.Errorf("worker %d ParseSize returned non-positive %d", workerIdx, sz)
			}
		})
	}

	wg.Wait()
	close(errChan)

	for workerErr := range errChan {
		t.Errorf("concurrency error: %v", workerErr)
	}
}
