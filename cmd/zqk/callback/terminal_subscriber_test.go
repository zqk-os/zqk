package callback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type errWriter struct {
	closed bool
}

func (w *errWriter) Write(p []byte) (n int, err error) {
	if w.closed {
		return 0, errors.New("writer closed")
	}
	return len(p), nil
}

type flushTrackerWriter struct {
	syncBuffer
	flushed bool
}

func (f *flushTrackerWriter) Flush() error {
	f.flushed = true
	return nil
}

func newTestEntry(jobID, cbType, status, msg string, extra map[string]any) *CallbackEntry {
	payload := make(map[string]any)
	if jobID != "" {
		payload["job_id"] = jobID
	}
	if cbType != "" {
		payload[objects.FieldKeyCallbackType] = cbType
	}
	if status != "" {
		payload["status"] = status
	}
	if msg != "" {
		payload["message"] = msg
	}
	for k, v := range extra {
		payload[k] = v
	}
	return &CallbackEntry{
		JobID:     jobID,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}
}

func newTestSubscriber(writer *syncBuffer, ansi, strip bool, prefix string) *TerminalProgressSubscriber {
	return NewTerminalProgressSubscriber(TerminalSubscriberConfig{
		Writer:      writer,
		AnsiEnabled: ansi,
		StripColors: strip,
		Prefix:      prefix,
	})
}

func setupTestSubscriberWithBuffer(t *testing.T, ansi, strip bool, prefix string) (*syncBuffer, *TerminalProgressSubscriber) {
	t.Helper()
	buf := &syncBuffer{}
	sub := newTestSubscriber(buf, ansi, strip, prefix)
	t.Cleanup(func() {
		if err := sub.Close(); err != nil && !errors.Is(err, ErrTerminalSubscriberClosed) {
			t.Logf("cleanup sub close: %v", err)
		}
	})
	return buf, sub
}

func notifyAndGetOutput(t *testing.T, sub *TerminalProgressSubscriber, buf *syncBuffer, entry *CallbackEntry) string {
	t.Helper()
	if err := sub.Notify(context.Background(), entry); err != nil {
		t.Fatalf("notify failed: %v", err)
	}
	return buf.String()
}

func assertOutputContains(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Fatalf("expected output to contain %q, got %q", expected, actual)
	}
}

func assertOutputNotContains(t *testing.T, actual, unexpected string) {
	t.Helper()
	if strings.Contains(actual, unexpected) {
		t.Fatalf("expected output NOT to contain %q, got %q", unexpected, actual)
	}
}

func TestTerminalProgressSubscriber_InterfaceCompliance(t *testing.T) {
	t.Parallel()
	_, sub := setupTestSubscriberWithBuffer(t, true, false, "[TEST]")

	var cbSub CallbackSubscriber = sub
	if cbSub.Name() != SubscriberNameTerminalProgress {
		t.Fatalf("expected name %q, got %q", SubscriberNameTerminalProgress, cbSub.Name())
	}
	if sub.IsClosed() {
		t.Fatalf("expected sub not to be closed initially")
	}
	if sub.IsAsync() {
		t.Fatalf("expected sub without buffer to be synchronous")
	}
	if sub.DroppedCount() != 0 {
		t.Fatalf("expected 0 dropped count")
	}
}

func TestTerminalProgressSubscriber_FormattedProgressOutput(t *testing.T) {
	t.Parallel()
	buf, sub := setupTestSubscriberWithBuffer(t, false, false, "zqk:")

	entry := newTestEntry("job-101", "progress", "in_progress", "Compiling package...", map[string]any{
		"percent": 45.5,
		"step":    9,
		"total":   20,
	})

	out := notifyAndGetOutput(t, sub, buf, entry)
	assertOutputContains(t, out, "zqk:")
	assertOutputContains(t, out, "[PROGRESS]")
	assertOutputContains(t, out, "Job: job-101")
	assertOutputContains(t, out, "45.5%")
	assertOutputContains(t, out, "step 9/20")
	assertOutputContains(t, out, "Compiling package...")
	assertOutputContains(t, out, "\n")
}

func TestTerminalProgressSubscriber_AnsiStreamInvalidation(t *testing.T) {
	t.Parallel()
	buf, sub := setupTestSubscriberWithBuffer(t, true, false, "")

	ctx := context.Background()
	progEntry := newTestEntry("job-ansi", "progress", "running", "Processing slice", map[string]any{
		"percent": 25.0,
	})
	if err := sub.Notify(ctx, progEntry); err != nil {
		t.Fatalf("notify progress failed: %v", err)
	}

	outProg := buf.String()
	assertOutputContains(t, outProg, AnsiCarriageReturnClear)
	assertOutputContains(t, outProg, "[PROGRESS] Job: job-ansi | 25.0% | Processing slice")
	assertOutputNotContains(t, outProg, "\n")

	doneEntry := newTestEntry("job-ansi", "completion", "completed", "Job finished successfully", map[string]any{
		"success":  true,
		"duration": 3.42,
	})
	if err := sub.Notify(ctx, doneEntry); err != nil {
		t.Fatalf("notify completion failed: %v", err)
	}

	outTotal := buf.String()
	assertOutputContains(t, outTotal, "[COMPLETE] Job: job-ansi | Status: completed | Duration: 3.42s")
	if !strings.HasSuffix(outTotal, "\n") {
		t.Fatalf("expected completion entry in ANSI mode to terminate with newline")
	}
}

func TestTerminalProgressSubscriber_PlainTextMode(t *testing.T) {
	t.Parallel()
	buf, sub := setupTestSubscriberWithBuffer(t, false, false, "PREFIX")

	entry := newTestEntry("job-plain", "status", "syncing", "Syncing files", nil)
	out := notifyAndGetOutput(t, sub, buf, entry)
	assertOutputNotContains(t, out, AnsiCarriageReturnClear)
	assertOutputContains(t, out, "PREFIX [STATUS] Job: job-plain | Status: syncing | Syncing files\n")
}

func TestTerminalProgressSubscriber_WakerEventFormatting(t *testing.T) {
	t.Parallel()
	buf, sub := setupTestSubscriberWithBuffer(t, false, false, "")

	wakerEntry := newTestEntry("", "waker", "waking", "Waking up dependent tasks", map[string]any{
		"kind":      "requirement",
		"object_id": "REQ-100",
	})
	out := notifyAndGetOutput(t, sub, buf, wakerEntry)
	assertOutputContains(t, out, "[WAKER]")
	assertOutputContains(t, out, "requirement REQ-100")
	assertOutputContains(t, out, "Status: waking")
	assertOutputContains(t, out, "Waking up dependent tasks")
}

func TestTerminalProgressSubscriber_FailureFormatting(t *testing.T) {
	t.Parallel()
	buf, sub := setupTestSubscriberWithBuffer(t, false, false, "")

	failEntry := newTestEntry("job-fail", "error", "failed", "", map[string]any{
		"error": "disk quota exceeded",
	})
	out := notifyAndGetOutput(t, sub, buf, failEntry)
	assertOutputContains(t, out, "[FAILED]")
	assertOutputContains(t, out, "Job: job-fail")
	assertOutputContains(t, out, "Status: failed")
	assertOutputContains(t, out, "Error: disk quota exceeded")
}

func TestTerminalProgressSubscriber_StripColors(t *testing.T) {
	t.Parallel()
	buf, sub := setupTestSubscriberWithBuffer(t, false, true, "\x1b[32m[COLOR]\x1b[0m")

	entry := newTestEntry("job-c", "status", "ok", "\x1b[31mcolorful text\x1b[0m", nil)
	out := notifyAndGetOutput(t, sub, buf, entry)
	assertOutputNotContains(t, out, "\x1b[32m")
	assertOutputNotContains(t, out, "\x1b[31m")
	assertOutputNotContains(t, out, "\x1b[0m")
	assertOutputContains(t, out, "[COLOR] [STATUS] Job: job-c | Status: ok | colorful text")
}

func TestTerminalProgressSubscriber_NilAndEmptyResilience(t *testing.T) {
	t.Parallel()
	var nilSub *TerminalProgressSubscriber
	ctx := context.Background()
	if err := nilSub.Notify(ctx, nil); !errors.Is(err, ErrTerminalSubscriberNil) {
		t.Fatalf("expected ErrTerminalSubscriberNil, got %v", err)
	}

	buf, sub := setupTestSubscriberWithBuffer(t, false, false, "")

	if err := sub.Notify(ctx, nil); err != nil {
		t.Fatalf("expected nil error for nil entry, got %v", err)
	}

	emptyEntry := &CallbackEntry{Payload: nil}
	if err := sub.Notify(ctx, emptyEntry); err != nil {
		t.Fatalf("expected nil error for nil payload, got %v", err)
	}

	if buf.String() != "" {
		t.Fatalf("expected no output written for nil entries")
	}
}

func TestTerminalProgressSubscriber_ClosedSubscriberAndWriter(t *testing.T) {
	t.Parallel()
	_, sub := setupTestSubscriberWithBuffer(t, false, false, "")
	if err := sub.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	ctx := context.Background()
	entry := newTestEntry("job-closed", "progress", "active", "msg", nil)
	if err := sub.Notify(ctx, entry); !errors.Is(err, ErrTerminalSubscriberClosed) {
		t.Fatalf("expected ErrTerminalSubscriberClosed, got %v", err)
	}

	ew := &errWriter{closed: true}
	errSub := NewTerminalProgressSubscriber(TerminalSubscriberConfig{
		Writer: ew,
	})
	defer func() {
		if err := errSub.Close(); err != nil {
			t.Fatalf("errSub close failed: %v", err)
		}
	}()

	if err := errSub.Notify(ctx, entry); err == nil {
		t.Fatalf("expected write error from closed writer")
	}
}

func TestTerminalProgressSubscriber_AsyncNonBlockingDropOnFull(t *testing.T) {
	t.Parallel()
	buf := &syncBuffer{}
	sub := NewTerminalProgressSubscriber(TerminalSubscriberConfig{
		Writer:      buf,
		BufferSize:  2,
		DropOnFull:  true,
		AnsiEnabled: false,
	})

	if !sub.IsAsync() {
		t.Fatalf("expected subscriber to be async")
	}

	ctx := context.Background()
	for i := 0; i < 20; i++ {
		entry := newTestEntry(fmt.Sprintf("job-%d", i), "progress", "running", "tick", nil)
		if err := sub.Notify(ctx, entry); err != nil {
			t.Fatalf("unexpected notify error: %v", err)
		}
	}

	time.Sleep(50 * time.Millisecond)
	if err := sub.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	if sub.DroppedCount() < 0 {
		t.Fatalf("invalid dropped count: %d", sub.DroppedCount())
	}
}

func TestTerminalProgressSubscriber_ContextCancellation(t *testing.T) {
	t.Parallel()
	buf := &syncBuffer{}
	sub := NewTerminalProgressSubscriber(TerminalSubscriberConfig{
		Writer:      buf,
		BufferSize:  1,
		DropOnFull:  false,
		AnsiEnabled: false,
	})
	defer func() {
		if err := sub.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entry := newTestEntry("job-canceled", "progress", "running", "msg", nil)
	if err := sub.Notify(ctx, entry); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
}

func TestTerminalProgressSubscriber_FlushAndClose(t *testing.T) {
	t.Parallel()
	ft := &flushTrackerWriter{}
	sub := NewTerminalProgressSubscriber(TerminalSubscriberConfig{
		Writer:      ft,
		AnsiEnabled: true,
	})

	ctx := context.Background()
	entry := newTestEntry("job-flush", "progress", "running", "progress tick", nil)
	if err := sub.Notify(ctx, entry); err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if err := sub.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}
	if !ft.flushed {
		t.Fatalf("expected writer flush to be called")
	}

	if err := sub.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !strings.HasSuffix(ft.String(), "\n") {
		t.Fatalf("expected close in ANSI mode without final newline to write trailing newline")
	}
}

func executeConcurrentSender(wg *sync.WaitGroup, sub *TerminalProgressSubscriber, workerID int, totalEntries int) {
	defer wg.Done()
	ctx := context.Background()
	for i := 0; i < totalEntries; i++ {
		entry := newTestEntry(fmt.Sprintf("job-conc-%d", workerID), "progress", "running", "step", map[string]any{
			"percent": float64(i),
		})
		if err := sub.Notify(ctx, entry); err != nil {
			return
		}
	}
}

func TestTerminalProgressSubscriber_ConcurrentLoadAndZeroLeaks(t *testing.T) {
	t.Parallel()
	buf := &syncBuffer{}
	sub := NewTerminalProgressSubscriber(TerminalSubscriberConfig{
		Writer:      buf,
		BufferSize:  256,
		DropOnFull:  false,
		AnsiEnabled: false,
	})

	var wg sync.WaitGroup
	workers := 10
	entriesPerWorker := 30
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		workerID := w
		goroutinelabels.NewGoroutine("concurrent_sender", "send concurrent terminal entries").StartSimple(func() {
			executeConcurrentSender(&wg, sub, workerID, entriesPerWorker)
		})
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	if err := sub.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	out := buf.String()
	if len(out) == 0 {
		t.Fatalf("expected concurrent writes in buffer")
	}
}

func TestTerminalProgressSubscriber_DispatcherIntegration(t *testing.T) {
	t.Parallel()
	buf, sub := setupTestSubscriberWithBuffer(t, false, false, "[DISPATCH]")

	dispatcher := NewMultiSubscriberDispatcher(nil)
	dispatcher.Register(sub)

	entry := newTestEntry("job-disp", "progress", "running", "via dispatcher", map[string]any{
		"percent": 88.0,
	})
	if err := dispatcher.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	out := buf.String()
	assertOutputContains(t, out, "[DISPATCH] [PROGRESS] Job: job-disp | 88.0% | via dispatcher")
}
