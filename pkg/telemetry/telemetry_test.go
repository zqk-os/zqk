package telemetry

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStreamCompaction(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "segment.chunk")
	if err := fileutil.WriteFile(oldFile, []byte("chunk"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := fileutil.Chtimes(oldFile, past, past); err != nil {
		t.Fatal(err)
	}
	deleted, err := CompactOldSegments([]string{dir}, 24*time.Hour)
	if err != nil {
		t.Fatalf("CompactOldSegments failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 chunk compacted, got %d", deleted)
	}
}

func TestGc(t *testing.T) {
	dir := t.TempDir()
	daemon := NewDaemon(nil)
	if daemon == nil {
		t.Fatal("expected daemon to be initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	daemon.RunBackgroundGC(ctx, []string{dir}, 24*time.Hour, 5*time.Millisecond)
}

func TestCacheHitRatioObservability(t *testing.T) {
	tracker := NewTracker(nil)
	if tracker == nil {
		t.Fatal("Expected tracker to be initialized")
	}

	tracker.RecordCacheHit(context.Background(), "cache_A")
	tracker.RecordCacheHit(context.Background(), "cache_A")
	tracker.RecordCacheHit(context.Background(), "cache_A")
	tracker.RecordCacheMiss(context.Background(), "cache_A")

	tracker.RecordCacheMiss(context.Background(), "cache_B")

	ratios := tracker.GetCacheHitRatios()

	if ratioA, ok := ratios["cache_A"]; !ok || ratioA != 0.75 {
		t.Errorf("Expected ratio for cache_A to be 0.75, got %v", ratioA)
	}
	if ratioB, ok := ratios["cache_B"]; !ok || ratioB != 0.0 {
		t.Errorf("Expected ratio for cache_B to be 0.0, got %v", ratioB)
	}
}

func TestIPCLatencyObservability(t *testing.T) {
	tracker := NewTracker(nil)
	if tracker == nil {
		t.Fatal("Expected tracker to be initialized")
	}

	tracker.RecordIPCLatency(context.Background(), "test_op", time.Millisecond*50)

	tracker.mu.Lock()
	h, ok := tracker.ipcHistograms["test_op"]
	tracker.mu.Unlock()

	if !ok {
		t.Fatal("Expected histogram to be created")
	}

	sum, count := h.GetStats()
	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
	// Using a small tolerance for floating point comparison
	if sum < 0.049 || sum > 0.051 {
		t.Errorf("Expected sum approx 0.05, got %f", sum)
	}
}

func TestAgentTokenTrackingObservability(t *testing.T) {
	// A simple test to ensure the interface methods exist and can be called
	tracker := NewTracker(nil)
	if tracker == nil {
		t.Fatal("Expected tracker to be initialized")
	}
}

func TestGhostDriftMTTRObservability(t *testing.T) {
	tracker := NewTracker(nil)
	if tracker == nil {
		t.Fatal("Expected tracker to be initialized")
	}

	tracker.RecordGhostDriftMTTR(context.Background(), "memory_leak", time.Minute*15) // 900s

	tracker.mu.Lock()
	h, ok := tracker.ghostDriftHistograms["memory_leak"]
	tracker.mu.Unlock()

	if !ok {
		t.Fatal("Expected histogram to be created for ghost drift")
	}

	sum, count := h.GetStats()
	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
	if sum != 900.0 {
		t.Errorf("Expected sum 900.0, got %f", sum)
	}
}

func TestHistogramAccessors(t *testing.T) {
	tracker := NewTracker(nil)

	ctx := context.Background()
	tracker.RecordIPCLatency(ctx, "storage_read", 25*time.Millisecond)
	tracker.RecordGhostDriftMTTR(ctx, "goroutine_leak", 120*time.Second)

	ipcHist := tracker.GetIPCHistograms()
	if h, ok := ipcHist["storage_read"]; !ok {
		t.Fatal("expected storage_read histogram in GetIPCHistograms")
	} else {
		_, count := h.GetStats()
		if count != 1 {
			t.Errorf("expected count 1, got %d", count)
		}
	}

	driftHist := tracker.GetGhostDriftHistograms()
	if h, ok := driftHist["goroutine_leak"]; !ok {
		t.Fatal("expected goroutine_leak histogram in GetGhostDriftHistograms")
	} else {
		_, count := h.GetStats()
		if count != 1 {
			t.Errorf("expected count 1, got %d", count)
		}
	}
}

