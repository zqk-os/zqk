package telemetry

import (
	"testing"
)

// msgTestFmt is a standard test assertion template.
const errMsgLimFmt = "limit = %d; want %d"
const errMsgRSS = "rss out of bounds (%f)"
const errMsgExceed = "should not exceed before simulate"
const errMsgCapBefore = "exceeded before any pressure"
const errMsgLimB = "LimitBytes = %d; want %d"
const errMsgRSSRange = "RSS field out of expected range: found %.0f (cap %.0f)"
const errMsgNilChunk = "nil chunk under valid rss budget"
const errMsgPayload = "payload mismatch type=%T value=%v"
const errMsgOOMGuarded = "expected oom guard to be true"
const errMsgZeroTS = "zero timestamp on emitted chunk %d"
const errMsgStreamCount = "StreamedCount = %d; want %d"
const errMsgSeqFmt = "sequence = %d; want %d"
const errMsgNilSecond = "nil second chunk under valid rss"
const errMsgSecondSeq = "second sequence = %d; want %d"
const errMsgCleanExceed = "exceeded during clean streaming"
const errMsgZeroTSStream = "zero timestamp on streamer emitted chunk"
const errMsgPayloadFmt = "payload not string 'hello': type=%T value=%v"
const errMsgSKStrFmt = "String = %%q; want %%%q"
const errMsgUnknownSourceLen = "unknown source kind must still return a string"
const errMsgInvalidFmt = "IsValid(%%d) = false; want true"
const errMsgCollectorZeroMetrics = "collector returned zero metrics from kernel sources"
const errMsgEmptyTag = "empty tag for metric %+v, source=%s"
const errMsgMetricZeroTS = "zero timestamp on metric %q (source=%s)"
const errMsgRSSCountFmt = "rss_bytes count = %d; want %d"
const errMsgNilCapMetrics = "nil cap should still produce non-empty metrics (cpu/net)"
const errMsgDeterminism = "determinism: limit mismatch %d vs %d"

// ---- MemoryCap tests (RSS cap enforcement) ----

func TestMemoryCap_Monitoring(t *testing.T) {
	cap := NewMemoryCap(1024*1024*300, true) // 300MB budget
	defer cap.Close()
	if int(cap.limit) != 300*1024*1024 {
		t.Fatalf(errMsgLimFmt, cap.limit, 300*1024*1024)
	}
}

func TestMemoryCap_Enforcement(t *testing.T) {
	cap := NewMemoryCap(1024*1024*300, true) // 300MB
	defer cap.Close()
	cap.simulateAtCap()
	if got := cap.IsExceeded(); !got {
		t.Fatal("expected IsExceeded = true after simulate at cap")
	}
}

func TestMemoryCap_SubLimit(t *testing.T) {
	cap := NewMemoryCap(1024*1024*300, true)
	defer cap.Close()
	info := cap.GetUsage()
	if info.RSS < -1 || info.RSS > float64(cap.limit)+1e9 {
		t.Fatalf(errMsgRSS, info.RSS)
	}
	if cap.IsExceeded() {
		t.Fatal(errMsgExceed)
	}
}

func TestRSSUsage_Structure(t *testing.T) {
	cap := NewMemoryCap(300*1024*1024, true)
	defer cap.Close()
	info := cap.GetUsage()
	if cap.IsExceeded() {
		t.Fatal(errMsgCapBefore)
	}
	if info.LimitBytes != 300*1024*1024 {
		t.Fatalf(errMsgLimB, info.LimitBytes, 300*1024*1024)
	}
	if info.RSS < -1 || info.RSS > float64(cap.limit)+1e9 {
		t.Fatalf(errMsgRSSRange, info.RSS, float64(cap.limit))
	}
}

// ---- OOMSafeStreamer tests ----

func TestOOMSafeStreamer_Streaming(t *testing.T) {
	cap := NewMemoryCap(1024*1024*300, true)
	defer cap.Close()
	streamer := NewOOMSafeStreamer(cap)
	data := []string{"item-a", "item-b", "item-c"}
	for i, item := range data {
		chunk := streamer.ProcessChunk(item)
		if chunk == nil {
			t.Fatalf(errMsgNilChunk + " chunk[%d]", i)
		}
		v, ok := chunk.Payload.(string)
		if !ok || v != item {
			t.Fatalf(errMsgPayload, chunk.Payload, v)
		}
		if !chunk.OOMGuarded {
			t.Fatalf(errMsgOOMGuarded + " on healthy flow")
		}
		if chunk.Timestamp.IsZero() {
			t.Fatalf(errMsgZeroTS, i)
		}
	}
	got := streamer.StreamedCount()
	want := len(data)
	if got != want {
		t.Fatalf(errMsgStreamCount, got, want)
	}
}

func TestOOMSafeStreamer_Sequence(t *testing.T) {
	cap := NewMemoryCap(1024*1024*300, true)
	defer cap.Close()
	streamer := NewOOMSafeStreamer(cap)

	chunkA := streamer.ProcessChunk("first")
	if chunkA == nil || int(chunkA.Sequence) != 0 {
		t.Fatalf(errMsgSeqFmt, chunkA.Sequence, 0)
	}

	chunkB := streamer.ProcessChunk("second")
	if chunkB == nil {
		t.Fatal(errMsgNilSecond)
	}
	if int(chunkB.Sequence) != 1 {
		t.Fatalf(errMsgSecondSeq, chunkB.Sequence, 1)
	}
	if cap.IsExceeded() {
		t.Fatal(errMsgCleanExceed)
	}
}

// TestStreamChunk_struct verifies stream chunks emitted by OOMSafeStreamer
func TestStreamChunk_struct(t *testing.T) {
	cap := NewMemoryCap(300*1024*1024, true)
	defer cap.Close()
	streamer := NewOOMSafeStreamer(cap)
	chunk := streamer.ProcessChunk("hello")

	if chunk.Timestamp.IsZero() {
		t.Fatal(errMsgZeroTSStream)
	}

	v, ok := chunk.Payload.(string)
	if !ok || v != "hello" {
		t.Fatalf(errMsgPayloadFmt, chunk.Payload, v)
	}
}

// ---- SourceKind tests ----

func TestSourceKind_String(t *testing.T) {
	want := map[SourceKind]string{
		SourceRSS:  "rss",
		SourceNet:  "net",
		SourceCPU:  "cpu",
		SourceSwap: "swap",
	}
	for k, v := range want {
		got := k.String()
		if got != v {
			t.Fatalf("SK string mismatch: got=%q wanted=%q (id=%d)", got, v, int(k))
		}
	}

	bogus := SourceKind(99)
	sname := bogus.String()
	if len(sname) < 1 {
		t.Fatal(errMsgUnknownSourceLen)
	}
}

func TestSourceKind_IsValid(t *testing.T) {
	validSources := []SourceKind{SourceRSS, SourceNet, SourceCPU, SourceSwap}
	for _, s := range validSources {
		if !s.IsValid() {
			t.Fatalf("IsValid(%d) = false; want true", int(s))
		}
	}
}

// TelemetryCollector tests (kernel telemetry, not synthetic metrics)

func TestTelemetryCollector_ObjectiveMetrics(t *testing.T) {
	cap := NewMemoryCap(1024*1024*300, true)
	defer cap.Close()
	reporter := NewTelemetryCollector(cap)
	metrics := reporter.CollectObjectiveKernelMetrics()
	if len(metrics) == 0 {
		t.Fatal(errMsgCollectorZeroMetrics)
	}
	for _, m := range metrics {
		if m.Tag == "" {
			t.Errorf(errMsgEmptyTag, m, m.Source.String())
		}
		if m.Timestamp.IsZero() {
			t.Fatalf(errMsgMetricZeroTS, m.Tag, m.Source.String())
		}
	}
}

func TestTelemetryCollector_MemoryMetricsSource(t *testing.T) {
	cap := NewMemoryCap(1024*1024*300, true)
	defer cap.Close()
	reporter := NewTelemetryCollector(cap)

	metrics := reporter.CollectObjectiveKernelMetrics()
	gotCount := 0
	for _, m := range metrics {
		if m.Tag == "rss_bytes" && m.Source == SourceRSS {
			gotCount++
		}
	}
	want1 := 1
	if gotCount != want1 {
		t.Fatalf(errMsgRSSCountFmt, gotCount, want1)
	}
}

func TestTelemetryCollector_nilCap(t *testing.T) {
	reporter := NewTelemetryCollector(nil)
	metrics := reporter.CollectObjectiveKernelMetrics()
	if len(metrics) == 0 {
		t.Fatal(errMsgNilCapMetrics)
	}
	foundNet := false
	for _, m2 := range metrics {
		if m2.Tag != "" && m2.Unit != "" {
			foundNet = true
		}
	}
	if !foundNet {
		t.Error("expected at least one metric with tag and unit")
	}
}

func TestRSSCap_Determinism(t *testing.T) {
	info1 := NewMemoryCap(300*1024*1024, true).GetUsage()
	info2 := NewMemoryCap(300*1024*1024, true).GetUsage()
	if info1.LimitBytes != info2.LimitBytes {
		t.Fatalf(errMsgDeterminism, info1.LimitBytes, info2.LimitBytes)
	}
}
