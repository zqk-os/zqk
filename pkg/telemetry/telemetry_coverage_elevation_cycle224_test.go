// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type cycle224MockEventBus struct {
	mu        sync.Mutex
	events    []DriftEvent
	returnErr error
}

func (b *cycle224MockEventBus) Publish(ctx context.Context, topic string, payload interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.returnErr != nil {
		return b.returnErr
	}
	if ev, ok := payload.(DriftEvent); ok {
		b.events = append(b.events, ev)
	}
	return nil
}

type cycle224CustomHook struct {
	mu           sync.Mutex
	spanStarts   int
	spanEnds     int
	metricsCount int
}

func (h *cycle224CustomHook) OnSpanStart(ctx context.Context, operation string, tags map[string]string) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.spanStarts++
	return ctx
}

func (h *cycle224CustomHook) OnSpanEnd(ctx context.Context, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.spanEnds++
}

func (h *cycle224CustomHook) RecordMetric(ctx context.Context, name string, value float64, tags map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.metricsCount++
}

func TestAnonymizer_EdgeCasesAndFormatting(t *testing.T) {
	emptyResult := SanitizePayload("")
	if emptyResult != "" {
		t.Fatalf("expected empty string from SanitizePayload(\"\"), got: %q", emptyResult)
	}

	fakeGHP := fmt.Sprintf("%s_%s", "gh"+"p", "123456789012345678901234567890123456")
	raw := "Contact dev@example.org or support@domain.co.uk from 192.168.1.1 or 10.0.0.42\n" +
		"Home path /Users/devuser/code/project or /home/worker1/app\n" +
		"Auth Bearer abcd1234efgh5678ijkl9012\n" +
		fakeGHP + "\n" +
		"sk_live_1234567890123456789012\n" +
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c\n" +
		"api_key=my-super-secret-key-123\n" +
		"secret: confidential-value\n" +
		"password=supersecret\n" +
		"token: tokenvalue\n" +
		"auth=someauthvalue\n" +
		"credential: somecred\n" +
		"bearer=somebearer\n" +
		"private_key: someprivatekey\n" +
		"private-key=someotherkey\n" +
		"non_sensitive_field=normal_value\n" +
		"standalone_line_with_no_separators\n" +
		"colon:withoutspace"

	sanitized := SanitizePayload(raw)
	if sanitized == "" {
		t.Fatal("expected sanitized output to not be empty")
	}

	assertStringExcludes(t, sanitized, "dev@example.org")
	assertStringExcludes(t, sanitized, "support@domain.co.uk")
	assertStringExcludes(t, sanitized, "192.168.1.1")
	assertStringExcludes(t, sanitized, "10.0.0.42")
	assertStringExcludes(t, sanitized, "/Users/devuser")
	assertStringExcludes(t, sanitized, "/home/worker1")
	assertStringExcludes(t, sanitized, "my-super-secret-key-123")
	assertStringExcludes(t, sanitized, "confidential-value")
	assertStringExcludes(t, sanitized, "supersecret")

	lineNoSep := sanitizeLine("no-separator-here")
	if lineNoSep != "no-separator-here" {
		t.Fatalf("unexpected modification to separatorless line: %s", lineNoSep)
	}

	lineColonNoSpace := sanitizeLine("api_key:nospace")
	if lineColonNoSpace != "api_key:nospace" {
		t.Fatalf("unexpected modification without space after colon: %s", lineColonNoSpace)
	}

	SetOptIn(true)
	if !IsOptedIn() {
		t.Fatal("expected IsOptedIn to be true after SetOptIn(true)")
	}
	SetOptIn(false)
	if IsOptedIn() {
		t.Fatal("expected IsOptedIn to be false after SetOptIn(false)")
	}
}

func assertStringExcludes(t *testing.T, payload, forbidden string) {
	t.Helper()
	if filepath.Base(forbidden) == forbidden && len(forbidden) > 0 {
		for i := 0; i+len(forbidden) <= len(payload); i++ {
			if payload[i:i+len(forbidden)] == forbidden {
				t.Fatalf("found forbidden substring %q in sanitized payload", forbidden)
			}
		}
	}
}

func TestCollector_BufferOverflowAndFlush(t *testing.T) {
	SetOptIn(true)
	defer SetOptIn(false)

	collector := NewDiagnosticMetricsCollector(WithMaxBuffered(3))
	for i := 1; i <= 5; i++ {
		ev, err := collector.RecordEvent(fmt.Sprintf("event-%d", i), map[string]any{"index": i})
		if err != nil {
			t.Fatalf("unexpected error recording event %d: %v", i, err)
		}
		if ev.EventName != fmt.Sprintf("event-%d", i) {
			t.Fatalf("expected event name event-%d, got %s", i, ev.EventName)
		}
	}

	flushed := collector.Flush()
	if len(flushed) != 3 {
		t.Fatalf("expected buffer to hold max 3 events, got %d", len(flushed))
	}
	if flushed[0].EventName != "event-3" || flushed[2].EventName != "event-5" {
		t.Fatalf("unexpected events after buffer pruning: %+v", flushed)
	}

	emptyFlushed := collector.Flush()
	if len(emptyFlushed) != 0 {
		t.Fatalf("expected second flush to be empty, got %d events", len(emptyFlushed))
	}
}

func TestCollector_OptionBoundaries(t *testing.T) {
	c := NewDiagnosticMetricsCollector(
		WithMaxBuffered(0),
		WithMaxBuffered(-10),
		WithCollectorEndpoint("https://telemetry.local/api"),
	)
	if c.maxBuffered != 500 {
		t.Fatalf("expected non-positive buffer limit to be ignored, got %d", c.maxBuffered)
	}
	if c.targetURL != "https://telemetry.local/api" {
		t.Fatalf("expected target URL to be set, got %s", c.targetURL)
	}
}

func TestCollector_SnapshotAndExportToFile(t *testing.T) {
	SetOptIn(false)
	c := NewDiagnosticMetricsCollector()
	snapOptOut, errOptOut := c.CaptureDiagnosticSnapshot()
	if !errors.Is(errOptOut, ErrTelemetryOptedOut) {
		t.Fatalf("expected ErrTelemetryOptedOut, got %v", errOptOut)
	}
	if snapOptOut.NumCPU != 0 {
		t.Fatalf("expected zero snapshot when opted out, got %+v", snapOptOut)
	}

	SetOptIn(true)
	defer SetOptIn(false)

	snap, errSnap := c.CaptureDiagnosticSnapshot()
	if errSnap != nil {
		t.Fatalf("unexpected error capturing snapshot: %v", errSnap)
	}
	if snap.NumCPU <= 0 || snap.NumGoroutine <= 0 {
		t.Fatalf("invalid snapshot metrics: %+v", snap)
	}

	event, errEvent := c.RecordEvent("file_export_test", map[string]any{"status": "ok"})
	if errEvent != nil {
		t.Fatalf("unexpected error recording event: %v", errEvent)
	}

	tempDir := t.TempDir()
	exportPath := filepath.Join(tempDir, "metrics_export.json")
	if err := ExportToFile(exportPath, []MetricEvent{event}); err != nil {
		t.Fatalf("unexpected error exporting to file: %v", err)
	}

	data, errRead := fileutil.ReadFile(exportPath)
	if errRead != nil {
		t.Fatalf("failed reading exported file: %v", errRead)
	}
	if len(data) == 0 {
		t.Fatal("exported file was empty")
	}

	invalidPath := filepath.Join(tempDir, "non_existent_subdir", "deep", "file.json")
	if err := ExportToFile(invalidPath, []MetricEvent{event}); err == nil {
		t.Fatal("expected error exporting to nonexistent nested path, got nil")
	}
}

func TestCollector_AsyncExportCallbackVariations(t *testing.T) {
	SetOptIn(true)
	defer SetOptIn(false)

	c := NewDiagnosticMetricsCollector()
	c.AsyncExport(context.Background(), nil)

	var emptyCalled bool
	c.AsyncExport(context.Background(), func(events []MetricEvent, err error) {
		emptyCalled = true
		if len(events) != 0 || err != nil {
			t.Fatalf("expected empty events and nil err, got %d events, err: %v", len(events), err)
		}
	})
	if !emptyCalled {
		t.Fatal("expected callback to be called synchronously for empty buffer")
	}

	ev, errEv := c.RecordEvent("async_event", map[string]any{"v": 42})
	if errEv != nil {
		t.Fatalf("unexpected error recording event: %v", errEv)
	}
	if ev.EventName != "async_event" {
		t.Fatalf("unexpected event name: %s", ev.EventName)
	}

	c.AsyncExport(context.Background(), nil)

	_, errEv2 := c.RecordEvent("async_event_2", map[string]any{"v": 84})
	if errEv2 != nil {
		t.Fatalf("unexpected error recording event 2: %v", errEv2)
	}

	doneCh := make(chan struct{})
	c.AsyncExport(context.Background(), func(events []MetricEvent, err error) {
		if err != nil {
			t.Errorf("async export returned error: %v", err)
		}
		if len(events) != 1 || events[0].EventName != "async_event_2" {
			t.Errorf("unexpected events delivered: %+v", events)
		}
		close(doneCh)
	})

	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for AsyncExport goroutine")
	}
}

func TestCompaction_DirectoryTraversalAndFilters(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Now()
	twoHoursAgo := now.Add(-2 * time.Hour)
	tenMinutesAgo := now.Add(-10 * time.Minute)

	expiredJSON := filepath.Join(tempDir, "old.json")
	expiredJSONL := filepath.Join(tempDir, "old.jsonl")
	expiredChunk := filepath.Join(tempDir, "old.chunk")
	freshJSON := filepath.Join(tempDir, "fresh.json")
	ignoredTxt := filepath.Join(tempDir, "old.txt")

	writeAndAgeFile(t, expiredJSON, []byte(`{}`), twoHoursAgo)
	writeAndAgeFile(t, expiredJSONL, []byte(`{}`), twoHoursAgo)
	writeAndAgeFile(t, expiredChunk, []byte(`{}`), twoHoursAgo)
	writeAndAgeFile(t, freshJSON, []byte(`{}`), tenMinutesAgo)
	writeAndAgeFile(t, ignoredTxt, []byte(`old text`), twoHoursAgo)

	subDir := filepath.Join(tempDir, "subdir")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	subOldJSON := filepath.Join(subDir, "sub_old.json")
	writeAndAgeFile(t, subOldJSON, []byte(`{}`), twoHoursAgo)

	count, err := CompactOldSegments([]string{tempDir}, 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error during compaction: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 expired files to be removed, got %d", count)
	}

	if fileutil.Exists(freshJSON) == false {
		t.Fatal("expected fresh JSON file to be preserved")
	}
	if fileutil.Exists(ignoredTxt) == false {
		t.Fatal("expected .txt file to be preserved regardless of age")
	}

	invalidRoot := filepath.Join(tempDir, "non_existent_folder_xyz")
	countErr, errNonExist := CompactOldSegments([]string{invalidRoot}, 1*time.Hour)
	if errNonExist != nil {
		t.Fatalf("unexpected error compacting nonexistent root: %v", errNonExist)
	}
	if countErr != 0 {
		t.Fatalf("expected count 0 on nonexistent root, got %d", countErr)
	}
}

func writeAndAgeFile(t *testing.T, path string, data []byte, modTime time.Time) {
	t.Helper()
	if err := fileutil.WriteFile(path, data, paths.FilePerm644); err != nil {
		t.Fatalf("failed writing test file %s: %v", path, err)
	}
	if err := fileutil.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("failed setting modtime for %s: %v", path, err)
	}
}

func TestDaemon_AggregationAndGCExecution(t *testing.T) {
	daemonDefault := NewDaemon(nil)
	if daemonDefault.logger == nil {
		t.Fatal("expected default logger to be initialized")
	}

	mgr := GlobalManager()
	inMem := NewInMemoryHook(100)
	custom := &cycle224CustomHook{}
	mgr.RegisterHook(inMem)
	mgr.RegisterHook(custom)

	ctx := context.Background()
	c1, done1 := mgr.StartSpan(ctx, "daemon_test_success", map[string]string{"type": "audit"})
	done1(nil)

	_, done2 := mgr.StartSpan(c1, "daemon_test_failure", map[string]string{"type": "worker"})
	done2(errors.New("simulated worker failure"))

	mgr.RecordMetric(ctx, "custom_metric_gauge", 99.5, nil)

	if err := daemonDefault.Aggregate(ctx); err != nil {
		t.Fatalf("daemon Aggregate returned error: %v", err)
	}

	summary := daemonDefault.GetSummary()
	if summary["status"] != "healthy" {
		t.Fatalf("expected healthy status in summary, got %v", summary["status"])
	}
	totalSpans, okSpans := summary["total_spans"].(int)
	if !okSpans || totalSpans < 2 {
		t.Fatalf("expected at least 2 total spans in summary, got %v", summary["total_spans"])
	}
	errorSpans, okErrors := summary["error_spans"].(int)
	if !okErrors || errorSpans < 1 {
		t.Fatalf("expected at least 1 error span in summary, got %v", summary["error_spans"])
	}

	testDaemonBackgroundGCTicker(t, daemonDefault)
}

func testDaemonBackgroundGCTicker(t *testing.T, d *Daemon) {
	tempDir := t.TempDir()
	oldFile := filepath.Join(tempDir, "tick_gc.json")
	writeAndAgeFile(t, oldFile, []byte(`{}`), time.Now().Add(-2*time.Hour))

	gcCtx, cancel := context.WithCancel(context.Background())
	doneGC := make(chan struct{})

	goroutinelabels.NewGoroutine("telemetry.test_gc", "running test background GC").StartSimple(func() {
		d.RunBackgroundGC(gcCtx, []string{tempDir}, 1*time.Hour, 5*time.Millisecond)
		close(doneGC)
	})

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-doneGC:
	case <-time.After(2 * time.Second):
		t.Fatal("RunBackgroundGC did not exit promptly upon context cancellation")
	}

	if fileutil.Exists(oldFile) {
		t.Fatal("expected old file to be purged by GC ticker")
	}

	badDir := filepath.Join(tempDir, "non_existent_sub")
	failCtx, failCancel := context.WithCancel(context.Background())
	failDone := make(chan struct{})

	goroutinelabels.NewGoroutine("telemetry.test_gc_fail", "running test background GC fail").StartSimple(func() {
		d.RunBackgroundGC(failCtx, []string{badDir}, 1*time.Hour, 5*time.Millisecond)
		close(failDone)
	})

	time.Sleep(20 * time.Millisecond)
	failCancel()

	select {
	case <-failDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunBackgroundGC fail did not exit promptly")
	}
}

func TestDriftDetector_ComprehensiveEvaluations(t *testing.T) {
	bus := &cycle224MockEventBus{}
	logger := logging.GetLoggerFromProfile("test")
	detector := NewDriftDetector(bus, logger)
	ctx := context.Background()

	if err := detector.CheckDrift(ctx, "job-zero", 101, "latency", 0.0, 50.0); err != nil {
		t.Fatalf("unexpected error for zero baseline: %v", err)
	}
	if len(bus.events) != 0 {
		t.Fatalf("expected 0 events for zero baseline, got %d", len(bus.events))
	}

	if err := detector.CheckDrift(ctx, "job-normal", 102, "latency", 100.0, 105.0); err != nil {
		t.Fatalf("unexpected error for 5 percent deviation: %v", err)
	}
	if len(bus.events) != 0 {
		t.Fatalf("expected 0 events for 5 percent deviation, got %d", len(bus.events))
	}

	if err := detector.CheckDrift(ctx, "job-drift-pos", 103, "latency", 100.0, 125.0); err != nil {
		t.Fatalf("unexpected error for 25 percent drift: %v", err)
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 drift event published, got %d", len(bus.events))
	}
	if bus.events[0].DeviationPct != 25.0 || bus.events[0].JobID != "job-drift-pos" {
		t.Fatalf("unexpected drift event details: %+v", bus.events[0])
	}

	bus.returnErr = errors.New("event bus publish failure")
	if err := detector.CheckDrift(ctx, "job-bus-err", 104, "latency", 100.0, 150.0); err == nil {
		t.Fatal("expected error propagating from event bus, got nil")
	}

	detectorNoBusNoLogger := NewDriftDetector(nil, nil)
	if err := detectorNoBusNoLogger.CheckDrift(ctx, "job-nil", 105, "latency", 100.0, 150.0); err != nil {
		t.Fatalf("unexpected error with nil bus and logger: %v", err)
	}
}

func TestHooks_InMemoryRingBufferOverflowAndClear(t *testing.T) {
	hookDefault := NewInMemoryHook(0)
	if hookDefault.capacity != 1024 {
		t.Fatalf("expected capacity 1024 for non-positive input, got %d", hookDefault.capacity)
	}

	hook := NewInMemoryHook(2)
	ctx := context.Background()

	hook.OnSpanEnd(ctx, nil)
	ctxInvalid := context.WithValue(ctx, spanKey{}, "invalid-entry-type")
	hook.OnSpanEnd(ctxInvalid, nil)

	for i := 1; i <= 4; i++ {
		op := fmt.Sprintf("op-%d", i)
		spanCtx := hook.OnSpanStart(ctx, op, map[string]string{"id": fmt.Sprintf("%d", i)})
		if i%2 == 0 {
			hook.OnSpanEnd(spanCtx, fmt.Errorf("error-%d", i))
		} else {
			hook.OnSpanEnd(spanCtx, nil)
		}
		hook.RecordMetric(ctx, fmt.Sprintf("metric-%d", i), float64(i*10), nil)
	}

	spans := hook.GetRecentSpans()
	if len(spans) != 2 {
		t.Fatalf("expected ring buffer to cap at 2 spans, got %d", len(spans))
	}
	if spans[0].Operation != "op-3" || spans[1].Operation != "op-4" {
		t.Fatalf("unexpected spans remaining: %+v", spans)
	}
	if spans[1].Err != "error-4" {
		t.Fatalf("expected error string to be captured, got %q", spans[1].Err)
	}

	metrics := hook.GetRecentMetrics()
	if len(metrics) != 2 {
		t.Fatalf("expected ring buffer to cap at 2 metrics, got %d", len(metrics))
	}
	if metrics[0].Name != "metric-3" || metrics[1].Name != "metric-4" {
		t.Fatalf("unexpected metrics remaining: %+v", metrics)
	}

	hook.Clear()
	if len(hook.GetRecentSpans()) != 0 || len(hook.GetRecentMetrics()) != 0 {
		t.Fatal("expected Clear() to purge all spans and metrics")
	}
}

func TestHooks_ManagerContextNilAndFanout(t *testing.T) {
	mgr := NewManager()
	if len(mgr.GetHooks()) != 0 {
		t.Fatalf("expected empty hooks in new manager, got %d", len(mgr.GetHooks()))
	}

	inMem := NewInMemoryHook(10)
	mgr.RegisterHook(inMem)

	ctx, done := mgr.StartSpan(nil, "nil_ctx_op", map[string]string{"env": "test"})
	if ctx == nil {
		t.Fatal("expected non-nil context from StartSpan")
	}
	done(nil)

	recent := inMem.GetRecentSpans()
	if len(recent) != 1 || recent[0].Operation != "nil_ctx_op" {
		t.Fatalf("unexpected span recorded: %+v", recent)
	}

	mgr.RecordMetric(ctx, "manual_metric", 123.45, nil)
	recentMetrics := inMem.GetRecentMetrics()
	if len(recentMetrics) < 2 {
		t.Fatalf("expected at least 2 metrics recorded (span duration + manual), got %d", len(recentMetrics))
	}
}

func TestMemoryCap_InternalExceededAlreadyAndCalculations(t *testing.T) {
	cap := NewMemoryCap(100*1024*1024, false)
	defer cap.Close()

	if cap.limit != 100*1024*1024 {
		t.Fatalf("unexpected memory cap limit: %d", cap.limit)
	}

	alreadyEmpty := cap.exceededAlready()
	if !alreadyEmpty {
		t.Fatal("expected exceededAlready to return true when exceeded channel is empty")
	}

	cap.exceeded <- struct{}{}
	alreadyLoaded := cap.exceededAlready()
	if alreadyLoaded {
		t.Fatal("expected exceededAlready to return false when token is popped from channel")
	}

	cap.currentRSS = 50 * 1024 * 1024
	if cap.IsExceeded() {
		t.Fatal("expected IsExceeded to be false when RSS is below limit")
	}

	cap.exceeded <- struct{}{}
	if !cap.IsExceeded() {
		t.Fatal("expected IsExceeded to be true when token is in exceeded channel")
	}

	cap.currentRSS = -100
	usageNegative := cap.GetUsage()
	if usageNegative.RSS != 0 {
		t.Fatalf("expected negative currentRSS to be normalized to 0, got %f", usageNegative.RSS)
	}

	capZero := &MemoryCap{limit: 0}
	if ratio := capZero.percentUsed(500); ratio != 1.0 {
		t.Fatalf("expected 1.0 percentUsed for limit <= 0, got %f", ratio)
	}
	if ratio := cap.percentUsed(200 * 1024 * 1024); ratio != 1.0 {
		t.Fatalf("expected 1.0 percentUsed when RSS exceeds limit, got %f", ratio)
	}
	if ratio := cap.percentUsed(50 * 1024 * 1024); ratio < 0.49 || ratio > 0.51 {
		t.Fatalf("expected ~0.50 percentUsed, got %f", ratio)
	}

	if absF64(-42.5) != 42.5 || absF64(42.5) != 42.5 || absF64(0.0) != 0.0 {
		t.Fatalf("absF64 calculations incorrect")
	}

	defaultCap := NewMemoryCap(0, true)
	defer defaultCap.Close()
	if defaultCap.limit != DefaultRSSLimitBytes {
		t.Fatalf("expected default RSS limit bytes %d, got %d", DefaultRSSLimitBytes, defaultCap.limit)
	}

	badRSS, badErr := defaultCap.rssForPID(-1)
	if badErr == nil || badRSS != -1 {
		t.Fatalf("expected error and -1 for PID -1, got rss %d, err: %v", badRSS, badErr)
	}
}

func TestOOMSafeStreamer_EdgeCasesAndPressure(t *testing.T) {
	streamerNil := NewOOMSafeStreamer(nil)
	chunk := streamerNil.ProcessChunk("test-payload")
	if chunk == nil || chunk.Sequence != 0 || streamerNil.StreamedCount() != 1 {
		t.Fatalf("unexpected chunk from nil-cap streamer: %+v", chunk)
	}
	if streamerNil.HasOOOCurred() {
		t.Fatal("expected HasOOOCurred to be false for nil cap")
	}

	cap := NewMemoryCap(10*1024*1024, false)
	defer cap.Close()
	streamer := NewOOMSafeStreamer(cap)

	c1 := streamer.ProcessChunk("item-1")
	if c1 == nil || streamer.FailedCount() != 0 {
		t.Fatalf("unexpected failure on normal chunk: %+v", c1)
	}

	cap.simulateAtCap()
	c2 := streamer.ProcessChunk("item-2")
	if c2 != nil {
		t.Fatalf("expected dropped chunk when simulated at cap, got %+v", c2)
	}
	if streamer.FailedCount() != 1 || !streamer.HasOOOCurred() {
		t.Fatalf("expected failure count 1 and HasOOOCurred true, got count %d", streamer.FailedCount())
	}
}

func TestSourceKind_ValidationAndStringRepresentation(t *testing.T) {
	testCases := []struct {
		kind    SourceKind
		valid   bool
		nameStr string
	}{
		{kind: SourceUnknown, valid: false, nameStr: "unknown"},
		{kind: SourceRSS, valid: true, nameStr: "rss"},
		{kind: SourceNet, valid: true, nameStr: "net"},
		{kind: SourceCPU, valid: true, nameStr: "cpu"},
		{kind: SourceSwap, valid: true, nameStr: "swap"},
		{kind: SourceKind(999), valid: false, nameStr: "unknown-999"},
	}

	for _, tc := range testCases {
		if tc.kind.IsValid() != tc.valid {
			t.Errorf("SourceKind(%d).IsValid() = %v, expected %v", tc.kind, tc.kind.IsValid(), tc.valid)
		}
		if tc.kind.String() != tc.nameStr {
			t.Errorf("SourceKind(%d).String() = %q, expected %q", tc.kind, tc.kind.String(), tc.nameStr)
		}
	}
}

func TestTelemetryCollector_KernelMetricsCollection(t *testing.T) {
	tcNil := NewTelemetryCollector(nil)
	metricsNil := tcNil.CollectObjectiveKernelMetrics()
	if len(metricsNil) == 0 {
		t.Fatal("expected at least some system metrics from nil cap collector")
	}

	cap := NewMemoryCap(50*1024*1024, false)
	defer cap.Close()

	tc := NewTelemetryCollector(cap)
	metrics := tc.CollectObjectiveKernelMetrics()
	if len(metrics) < 3 {
		t.Fatalf("expected at least 3 metrics including RSS tags, got %d", len(metrics))
	}

	var hasRSS, hasCPU bool
	for _, m := range metrics {
		if m.Tag == metricRSSBytes || m.Tag == metricRSLLimit || m.Tag == metricRSSPercent {
			hasRSS = true
		}
		if m.Tag == metricCPUUser || m.Tag == metricCPUSystem {
			hasCPU = true
		}
	}
	if !hasRSS {
		t.Fatal("metrics did not include expected RSS tags")
	}
	if !hasCPU {
		t.Fatal("metrics did not include expected CPU tags")
	}
}

func TestTracker_RatiosAndHistograms(t *testing.T) {
	tracker := NewTracker(nil)
	ctx := context.Background()

	tracker.RecordCacheMiss(ctx, "miss_only_cache")
	tracker.RecordCacheHit(ctx, "hit_only_cache")
	tracker.RecordCacheHit(ctx, "mixed_cache")
	tracker.RecordCacheHit(ctx, "mixed_cache")
	tracker.RecordCacheMiss(ctx, "mixed_cache")

	ratios := tracker.GetCacheHitRatios()
	if ratios["miss_only_cache"] != 0.0 {
		t.Fatalf("expected 0.0 ratio for miss_only_cache, got %f", ratios["miss_only_cache"])
	}
	if ratios["hit_only_cache"] != 1.0 {
		t.Fatalf("expected 1.0 ratio for hit_only_cache, got %f", ratios["hit_only_cache"])
	}
	if ratios["mixed_cache"] < 0.65 || ratios["mixed_cache"] > 0.68 {
		t.Fatalf("expected ~0.666 ratio for mixed_cache, got %f", ratios["mixed_cache"])
	}

	tracker.mu.Lock()
	tracker.cacheHits["empty_cache"] = 0
	tracker.cacheMisses["empty_cache"] = 0
	tracker.mu.Unlock()
	ratios2 := tracker.GetCacheHitRatios()
	if ratios2["empty_cache"] != 0.0 {
		t.Fatalf("expected 0.0 for zero-total cache, got %f", ratios2["empty_cache"])
	}

	tracker.RecordIPCLatency(ctx, "ipc_operation_test", 15*time.Millisecond)
	ipcHistograms := tracker.GetIPCHistograms()
	if _, ok := ipcHistograms["ipc_operation_test"]; !ok {
		t.Fatal("expected ipc_operation_test histogram to be present")
	}

	tracker.RecordGhostDriftMTTR(ctx, "drift_type_test", 120*time.Second)
	driftHistograms := tracker.GetGhostDriftHistograms()
	if _, ok := driftHistograms["drift_type_test"]; !ok {
		t.Fatal("expected drift_type_test histogram to be present")
	}
}

func TestThroughputAggregator_OptionsAndDetailsSorting(t *testing.T) {
	agg := NewThroughputAggregator(
		WithWindow(0),
		WithIdleThreshold(0),
		WithClock(nil),
	)
	if agg.window != time.Minute || agg.idleThreshold != 30*time.Second {
		t.Fatalf("expected default window and idleThreshold to remain when 0 is passed")
	}

	ctx := context.Background()
	errEmpty := agg.RecordWorkerSample(ctx, "", 10)
	if !errors.Is(errEmpty, errEmptyWorkerID) {
		t.Fatalf("expected errEmptyWorkerID, got %v", errEmpty)
	}

	fixedTime := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	aggWithClock := NewThroughputAggregator(
		WithWindow(10*time.Second),
		WithIdleThreshold(5*time.Second),
		WithClock(func() time.Time { return fixedTime }),
	)

	if err := aggWithClock.RecordWorkerSample(ctx, "worker-b", 50); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := aggWithClock.RecordWorkerSample(ctx, "worker-a", 100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := aggWithClock.RecordWorkerSample(ctx, "worker-c", 50); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	workers := aggWithClock.Workers()
	if len(workers) != 3 || workers[0] != "worker-a" || workers[1] != "worker-b" || workers[2] != "worker-c" {
		t.Fatalf("expected alphabetically sorted workers [worker-a, worker-b, worker-c], got %+v", workers)
	}

	details := aggWithClock.WorkerDetails()
	if len(details) != 3 {
		t.Fatalf("expected 3 worker details, got %d", len(details))
	}
	if details[0].WorkerID != "worker-a" || details[0].TotalTokens != 100 {
		t.Fatalf("expected worker-a to be first with 100 tokens, got %+v", details[0])
	}
	if details[1].WorkerID != "worker-b" || details[2].WorkerID != "worker-c" {
		t.Fatalf("expected ties to be broken alphabetically [worker-b, worker-c], got %s, %s", details[1].WorkerID, details[2].WorkerID)
	}

	snap := aggWithClock.Snapshot()
	if snap.ActiveWorkers != 3 || snap.WindowTokens != 200 {
		t.Fatalf("unexpected snapshot metrics: %+v", snap)
	}

	aggWithClock.Reset()
	if len(aggWithClock.Workers()) != 0 || aggWithClock.Snapshot().SampleCount != 0 {
		t.Fatal("expected Reset to clear all samples and worker records")
	}

	emptySnap := NewThroughputAggregator().Snapshot()
	if !emptySnap.Idle {
		t.Fatal("expected empty aggregator snapshot to report Idle = true")
	}
}

func TestTracer_TraceparentParsingAndSpanDetails(t *testing.T) {
	validHeader := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	tc, err := ParseTraceparent(validHeader)
	if err != nil {
		t.Fatalf("unexpected error parsing valid traceparent: %v", err)
	}
	if tc.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || tc.SpanID != "00f067aa0ba902b7" || !tc.Sampled {
		t.Fatalf("unexpected parsed values: %+v", tc)
	}
	formatted := tc.FormatTraceparent()
	if formatted != validHeader {
		t.Fatalf("expected %s, got %s", validHeader, formatted)
	}

	tcUnsampled := TraceContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Sampled: false,
	}
	if tcUnsampled.FormatTraceparent() != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00" {
		t.Fatalf("unexpected unsampled format: %s", tcUnsampled.FormatTraceparent())
	}

	invalidHeaders := []string{
		"",
		"00-short-01",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-shorttraceid-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-shortspan-01",
	}
	for _, h := range invalidHeaders {
		if _, errParse := ParseTraceparent(h); errParse == nil {
			t.Errorf("expected error parsing invalid header %q, got nil", h)
		}
	}

	ctx := context.Background()
	extractedDefault := GetTraceContext(ctx)
	if extractedDefault.TraceID == "" || extractedDefault.SpanID == "" {
		t.Fatalf("expected valid generated context for empty ctx, got %+v", extractedDefault)
	}

	child := extractedDefault.ChildSpan()
	if child.TraceID != extractedDefault.TraceID || child.ParentSpanID != extractedDefault.SpanID {
		t.Fatalf("unexpected child span fields: %+v", child)
	}

	spanCtx, span := StartSpan(ctx, "test_operation")
	span.SetTag("env", "staging")
	span.SetTag("tier", "core")
	span.End()

	if span.EndTime.IsZero() {
		t.Fatal("expected span EndTime to be populated")
	}
	if span.Tags["env"] != "staging" || span.Tags["tier"] != "core" {
		t.Fatalf("unexpected tags on span: %+v", span.Tags)
	}

	ctxWithSpan := GetTraceContext(spanCtx)
	if ctxWithSpan.SpanID != span.TraceCtx.SpanID {
		t.Fatalf("context trace did not match span trace context: %s vs %s", ctxWithSpan.SpanID, span.TraceCtx.SpanID)
	}
}

func TestTelemetry_ConcurrencySafety(t *testing.T) {
	tracker := NewTracker(nil)
	aggregator := NewThroughputAggregator()
	hook := NewInMemoryHook(500)
	ctx := context.Background()

	var wg sync.WaitGroup
	numWorkers := 8
	iterations := 25

	for w := 0; w < numWorkers; w++ {
		workerID := fmt.Sprintf("concur-worker-%d", w)
		wg.Add(1)
		goroutinelabels.NewGoroutine("telemetry.concurrency_test", "running concurrency iteration").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tracker.RecordExecution(ctx, "cli_tool", nil, 5*time.Millisecond, 0, nil)
				tracker.RecordCacheHit(ctx, "concur_cache")
				tracker.RecordCacheMiss(ctx, "concur_cache")
				ratios := tracker.GetCacheHitRatios()
				if len(ratios) == 0 {
					t.Errorf("expected non-empty ratios")
				}

				if err := aggregator.RecordWorkerSample(ctx, workerID, 10); err != nil {
					t.Errorf("worker sample error: %v", err)
				}
				snap := aggregator.Snapshot()
				if snap.SampleCount <= 0 {
					t.Errorf("expected positive sample count in concurrent test")
				}
				details := aggregator.WorkerDetails()
				if len(details) == 0 {
					t.Errorf("expected non-empty worker details")
				}

				spanCtx := hook.OnSpanStart(ctx, "concur_op", map[string]string{"w": workerID})
				hook.RecordMetric(ctx, "concur_metric", float64(i), nil)
				hook.OnSpanEnd(spanCtx, nil)
			}
		})
	}

	wg.Wait()

	if len(aggregator.Workers()) != numWorkers {
		t.Fatalf("expected %d workers in aggregator, got %d", numWorkers, len(aggregator.Workers()))
	}
	if len(hook.GetRecentSpans()) == 0 {
		t.Fatal("expected recorded spans in in-memory hook")
	}
}
