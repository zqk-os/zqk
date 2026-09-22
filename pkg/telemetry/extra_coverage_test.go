// BLI-STARTER-COMMUNITY-067 / PRI-STARTER-COMMUNITY-067 coverage elevation
package telemetry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

type extraHook struct {
	starts, ends, metrics int
}

func (h *extraHook) OnSpanStart(ctx context.Context, operation string, tags map[string]string) context.Context {
	h.starts++
	_ = operation
	_ = tags
	return ctx
}
func (h *extraHook) OnSpanEnd(ctx context.Context, err error) { h.ends++; _ = err }
func (h *extraHook) RecordMetric(ctx context.Context, name string, value float64, tags map[string]string) {
	h.metrics++
	_, _, _ = name, value, tags
}

func TestExtraTelemetryHooksTrackerAndAnonymizer(t *testing.T) {
	ctx := context.Background()
	m := NewManager()
	h := &extraHook{}
	m.RegisterHook(h)
	_, done := m.StartSpan(nil, "op", map[string]string{"k": "v"})
	done(fmt.Errorf("span err"))
	m.RecordMetric(ctx, "n", 1, nil)
	gm := GlobalManager()
	gm.RegisterHook(h)
	_, done2 := gm.StartSpan(ctx, "g", nil)
	done2(nil)

	logger := logging.GetLoggerFromProfile("system")
	tr := NewTracker(logger)
	tr.RecordExecution(ctx, "zqk", []string{"a"}, time.Millisecond, 1, fmt.Errorf("boom"))
	tr.RecordExecution(ctx, "zqk", nil, time.Millisecond, 0, nil)
	tr.RecordRateLimitExceeded(ctx, "zqk")
	tr.RecordCacheHit(ctx, "c1")
	tr.RecordCacheMiss(ctx, "c1")
	tr.RecordCacheMiss(ctx, "c2")
	_ = tr.GetCacheHitRatios()
	tr.RecordIPCLatency(ctx, "ipc", time.Millisecond)
	tr.RecordAgentTokenUsage(ctx, "agt", 3, 7)
	tr.RecordGhostDriftMTTR(ctx, "ghost", time.Second)
	tr.RecordPersonaSkillInvocation(ctx, "p", "sk", time.Millisecond, true)
	NewTracker(nil).RecordExecution(ctx, "x", nil, 0, 0, nil)

	_ = SanitizePayload("")
	scrubbed := SanitizePayload("user@example.com 10.0.0.1 bearer abcdefghijklmnop\npassword=secret\napi_key: val\nnosep")
	if scrubbed == "" {
		t.Fatal("sanitize empty")
	}
	_ = sanitizeLine("plain")
	_ = sanitizeLine("token=abc")
	_ = sanitizeLine("note: value")

	SetOptIn(true)
	t.Cleanup(func() { SetOptIn(false) })
	c := NewDiagnosticMetricsCollector(WithCollectorEndpoint("http://127.0.0.1:1"), WithMaxBuffered(2))
	_, _ = c.RecordEvent("", nil)
	ev, err := c.RecordEvent("boot", map[string]any{"email": "a@b.co", "n": 1})
	if err != nil {
		t.Fatal(err)
	}
	_ = ev
	c.Close()
	_, _ = c.RecordEvent("after", nil)
	_ = ExportToFile(filepath.Join(t.TempDir(), "t.json"), []MetricEvent{ev})
	doneCh := make(chan struct{})
	c2 := NewDiagnosticMetricsCollector()
	SetOptIn(true)
	_, _ = c2.RecordEvent("e", map[string]any{"k": "v"})
	c2.AsyncExport(ctx, func([]MetricEvent, error) { close(doneCh) })
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("async export")
	}
	empty := NewDiagnosticMetricsCollector()
	empty.AsyncExport(ctx, nil)

	cap := NewMemoryCap(1<<40, false)
	t.Cleanup(cap.Close)
	_ = cap.GetUsage()
	_ = cap.IsExceeded()
	_ = cap.percentUsed(0)
	_ = cap.percentUsed(cap.limit)
	_, _ = cap.rssForPID(os.Getpid())
	st := NewOOMSafeStreamer(cap)
	_ = st.ProcessChunk("ok")
	cap.simulateAtCap()
	_ = st.ProcessChunk("drop")
	_ = st.HasOOOCurred()
	_ = st.FailedCount()
	_ = st.StreamedCount()
	_ = NewOOMSafeStreamer(nil).ProcessChunk(1)
}
