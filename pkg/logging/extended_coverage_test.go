package logging

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestEventBuilder_AllMethods(t *testing.T) {
	b := NewEvent("test_event").
		String("str_k", "str_v").
		Int("int_k", 42).
		Bool("bool_k", true).
		Duration("dur_k", 100*time.Millisecond).
		BatchID("BAT-1").
		JobID("JOB-1").
		JobType("sync").
		Category("validation").
		Processed(10).
		Fixed(5).
		Failed(1).
		Skipped(2).
		ChunkFixed(3).
		ChunkSkipped(1).
		ChunkFailed(0).
		File("test.yaml")

	if b.Message() != "test_event" {
		t.Errorf("expected test_event, got %s", b.Message())
	}
	if len(b.Fields()) != 16 {
		t.Errorf("expected 16 fields, got %d", len(b.Fields()))
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	b.Debug(logger)
	b.Info(logger)
	b.Warn(logger)
	b.Error(logger, errors.New("err"))

	out := buf.String()
	if !strings.Contains(out, "test_event") || !strings.Contains(out, "batch_id=BAT-1") {
		t.Errorf("missing expected content in event log: %s", out)
	}
}

func TestBufferedWriter_FlushSyncBufferedAvailable(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultBufferedWriterConfig()
	cfg.BufferSize = 1024
	bw := NewBufferedWriter(&buf, cfg)

	n, err := bw.Write([]byte("hello buffered"))
	if err != nil || n != 14 {
		t.Fatalf("Write failed: n=%d, err=%v", n, err)
	}

	if bw.Buffered() != 14 {
		t.Errorf("expected 14 buffered bytes, got %d", bw.Buffered())
	}
	if bw.Available() != 1024-14 {
		t.Errorf("expected 1010 available bytes, got %d", bw.Available())
	}

	if err := bw.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if bw.Buffered() != 0 {
		t.Errorf("expected 0 buffered bytes after flush, got %d", bw.Buffered())
	}
	if buf.String() != "hello buffered" {
		t.Errorf("expected 'hello buffered', got %q", buf.String())
	}

	if err := bw.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if err := bw.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestSharedBufferedFlusher_SnapshotAndUnregister(t *testing.T) {
	flusher := &sharedBufferedFlusher{
		writers: make(map[*BufferedWriter]struct{}),
	}

	var buf bytes.Buffer
	bw := NewBufferedWriter(&buf, DefaultBufferedWriterConfig())

	flusher.register(bw)
	sn := flusher.snapshot()
	if len(sn) != 1 || sn[0] != bw {
		t.Errorf("expected 1 snapshot entry")
	}

	flusher.unregister(bw)
	sn2 := flusher.snapshot()
	if len(sn2) != 0 {
		t.Errorf("expected 0 snapshot entries after unregister")
	}

	// nil safety
	flusher.register(nil)
	flusher.unregister(nil)
	var nilFlusher *sharedBufferedFlusher
	nilFlusher.register(bw)
	nilFlusher.unregister(bw)
}

func TestCommandOutput_Writers(t *testing.T) {
	// Normal CLI usage
	w1 := GetCommandOutputWriter(context.Background())
	if w1 != os.Stdout {
		t.Errorf("expected os.Stdout for default CLI")
	}

	// Context override
	var customBuf bytes.Buffer
	ctxWithWriter := pkgctx.WithCommandOutputWriter(context.Background(), &customBuf)
	w2 := GetCommandOutputWriter(ctxWithWriter)
	if w2 != &customBuf {
		t.Errorf("expected customBuf from context")
	}
}

func TestContextLogger_ProfilesAndDecisionContext(t *testing.T) {
	tmpDir := t.TempDir()

	// GetLogger deprecated
	l1 := GetLogger()
	if l1 == nil {
		t.Errorf("expected non-nil from GetLogger")
	}

	// GetLoggerFromContext
	ctx := pkgctx.NewSystemContext()
	l2 := GetLoggerFromContext(ctx)
	if l2 == nil {
		t.Errorf("expected non-nil from GetLoggerFromContext")
	}

	// Profiles
	for _, prof := range []string{"system", "human", "debug", "json", "compact", "audit", "metrics"} {
		l := GetLoggerFromProfile(prof)
		if l == nil {
			t.Errorf("expected non-nil logger for profile %s", prof)
		}
	}

	// DecisionContextLogger
	decCtx := pkgctx.NewLoggingDecisionContext().
		WithLoggingContext(pkgctx.NewLoggingContext(pkgctx.ProfileSystem)).
		WithComponent("test_comp").
		WithOperationContext(ctx)

	decLogger := GetLoggerFromDecisionContext(decCtx, tmpDir)
	if decLogger == nil {
		t.Fatalf("expected non-nil from GetLoggerFromDecisionContext")
	}

	decLogger.Debug("test debug msg", String("key", "val"))
	decLogger.Info("test info msg", Int("num", 1))
	decLogger.Warn("test warn msg", Bool("flag", true))
	decLogger.Error("test error msg", errors.New("err detail"))

	decLogger = decLogger.WithFields(String("f1", "v1"))
	decLogger = decLogger.WithContext(context.Background())
	decLogger = decLogger.WithObjectRef("backlog_item", "BLI-1")

	if closer, ok := decLogger.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}
}

func TestFluentBuilder_ExtensiveChaining(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	el := NewEventLogger(pkgctx.NewSystemContext())

	// Test TryFluentEvent
	if root, ok := TryFluentEvent(el); ok {
		root.Info("try_event").String("k1", "v1").Log()
	} else {
		t.Errorf("expected TryFluentEvent to succeed on *EventLogger")
	}
	if _, ok := TryFluentEvent("not an event logger"); ok {
		t.Errorf("expected false for invalid type")
	}

	// Test FluentEvent
	FluentEvent(el).Info("event_name").
		String("k", "v").
		Log()

	// Test full chain of fields
	Fluent(logger).Debug("fluent_extensive").
		JobID("JOB-1").
		MessageID("MSG-1").
		WorkerID(2).
		MetricKind("gauge").
		Kind("backlog_item").
		RuleName("rule_a").
		RuleConfigName("rule_cfg").
		RulesDir("/path/to/rules").
		RuleCount(15).
		ValidationRowField("spec.name").
		ValidationRowMessage("invalid spec").
		LoaderPhase("phase_1").
		KindsCount(5).
		File("file.go").
		Path("/tmp/file.go").
		URL("https://example.com").
		Protocol("https").
		ObjectID("OBJ-100").
		BatchID("BAT-50").
		MetricID("MET-1").
		ProjectRoot("/repo").
		ScheduleID("SCH-1").
		Target("target_node").
		SessionID("SES-1").
		ActiveGoroutines(8).
		WorkerStopTimeout("5s").
		CacheSaveTimeout("2s").
		SemaphoreCapacity(16).
		MaxWorkers(4).
		EngineID("ENG-1").
		Watermark("2026-01-01T00:00:00Z").
		SessionStatus("running").
		Command("zqk run").
		WorkingDirectory("/repo").
		SchedulerJobType("periodic").
		JobCategory("maintenance").
		TestName("TestA").
		PackageName("pkg/test").
		ExpectedTimeout("30s").
		ScriptPath("run.sh").
		StderrSnippet("err").
		StdoutSnippet("out").
		EventType("custom").
		LogLevel("debug").
		MetricsFound(5).
		DeletedCount(2).
		FailedCount(1).
		ProcessedCount(10).
		ExpectedURLCount(3).
		VerifiedURLCount(2).
		FailedURLCount(1).
		Count(100).
		Deleted(4).
		MaxCount(200).
		ToDelete(2).
		BatchSize(50).
		MaxBatches(5).
		CandidateCount(20).
		FilesProcessed(10).
		MetricsCaptured(8).
		FilesDeleted(2).
		ErrorCount(1).
		ObjectCount(50).
		PolicyCount(12).
		PoliciesProcessed(10).
		PoliciesRefreshed(2).
		MessagesSent(15).
		MessagesSucceeded(14).
		MessagesFailed(1).
		MessageIndex(3).
		Profile("human").
		ProfileName("admin").
		Index(0).
		Title("Sample Title").
		OutputDir("/out").
		FilePath("/out/file.txt").
		KindDir("/out/kinds").
		JobIdx(1).
		Entries(50).
		CacheSize(100).
		Invalidated(5).
		Dir("/tmp").
		PanicSummary("recovered").
		Errors(2).
		Written(10).
		Strategy("bulk").
		HashFile("hash.txt").
		TemplatePath("tpl.yaml").
		Message("custom msg").
		TimestampRFC3339Nano("2026-01-01T00:00:00.000000000Z").
		Bytes(1024).
		ExecPath("/usr/bin/git").
		ExecArgs("status").
		ElapsedString("1.2s").
		ErrorSummary("fail").
		DedupeKey("k1").
		AckedBy("agent").
		SkippedCount(1).
		WrittenCount(8).
		EntryCount(12).
		Total(100).
		QueueSize(10).
		QueueName("q_main").
		Enqueued(5).
		Stale(2).
		WithIssues(1).
		KindsWithObjects(4).
		EntriesRemoved(2).
		EntriesBeforeCleanup(10).
		BlockingPublic(1).
		BlockingInternal(2).
		EventsCreated(5).
		Channel("ch1").
		EventID("EV-1").
		PersonaTitle("Dev").
		DeliveryMode("direct").
		ObjectKeyID("OK-1").
		ItemID("IT-1").
		Note("sample note").
		ErrorText("some error").
		Output("command output").
		Format("json").
		Limit(100).
		EventName("event.start").
		Scene(1).
		WithError(errors.New("test err")).
		Log()

	out := buf.String()
	for _, expected := range []string{
		"job_id=JOB-1", "message_id=MSG-1", "worker_id=2", "metric_kind=gauge",
		"rule=rule_a", "rule_name=rule_cfg", "rules_dir=/path/to/rules",
		"rule_count=15", "field=spec.name", "loader=phase_1",
		"kinds=5", "file=file.go", "path=/tmp/file.go", "url=https://example.com",
		"protocol=https", "id=OBJ-100", "batch_id=BAT-50", "metric_id=MET-1",
		"project_root=/repo", "schedule_id=SCH-1", "target=target_node",
		"session_id=SES-1", "active_goroutines=8", "worker_stop_timeout=5s",
		"cache_save_timeout=2s", "semaphore_capacity=16", "max_workers=4",
		"engineID=ENG-1", "watermark=2026-01-01T00:00:00Z", "session_status=running",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("missing expected field %q in %s", expected, out)
		}
	}
}

func TestEventLogger_AdditionalMethods(t *testing.T) {
	ctx := pkgctx.NewSystemContext()
	el := NewEventLogger(ctx)

	// LogDependencyGraph without error
	el.LogDependencyGraph("build", 10, nil)

	// LogDependencyGraph with error
	el.LogDependencyGraph("build", 10, []GraphError{
		{Type: "cycle", Message: "cyclic dependency"},
	})

	// LogValidationError
	el.LogValidationError(ValidationError{
		Field:       "title",
		MissingItem: "item",
		Message:     "must be set",
		CriteriaRef: "CRI-1",
	})

	// LogError, LogInfo, LogDebug, LogWarning
	el.LogError("an error occurred", errors.New("underlying"), String("extra", "val"))
	el.LogInfo("info log", String("info_k", "info_v"))
	el.LogDebug("debug log", Int("count", 42))
	el.LogWarning("warn log", Bool("warned", true))

	// WithFields
	subEl := el.WithFields(String("sub", "logger"))
	if subEl == nil {
		t.Errorf("expected non-nil sub logger")
	}

	// WithObjectRef
	subRef := el.WithObjectRef("backlog_item", "BLI-1")
	if subRef == nil {
		t.Errorf("expected non-nil sub ref logger")
	}
}

func TestCommonFieldsHelpers_AllFunctions(t *testing.T) {
	// Call field helpers
	_ = CodeField(200)
	_ = EventField("test_event")
	_ = StatusIntField(404)
	_ = ActiveInstancesField(3)
	_ = CountField(10)
	_ = ElapsedStringField("100ms")
	_ = LimitField(50)
	_ = PeriodField(5)
	_ = PRNumberField(123)
	_ = SceneField(2)
	_ = AddrField("localhost:8080")
	_ = FileField("test.txt")
	_ = FormatField("yaml")
	_ = OutputField("result")
	_ = PathField("/path")
	_ = ProjectRootField("/root")
	_ = ScriptField("test.sh")
	_ = CommandField("build")
	_ = HandlerField("handler1")
	_ = MethodField("GET")
	_ = ProtocolField("tcp")
	_ = ArgumentsField("arg1 arg2")
	_ = ErrorTextField("err text")
	_ = KindField("kind1")
	_ = NormalizedNameField("norm")
	_ = NoteField("note")
	_ = ToolNameField("tool")

	// truncateLoggedText on large text (> 8KB)
	largeText := strings.Repeat("A", 10*1024)
	truncated := truncateLoggedText(largeText)
	if !strings.HasSuffix(truncated, "…[truncated]") {
		t.Errorf("expected truncated text to have suffix")
	}
	smallText := "small"
	if truncateLoggedText(smallText) != "small" {
		t.Errorf("expected small text unchanged")
	}
}

func TestLockLoggerAdapter_Methods(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	adapter := NewLockLoggerAdapter(logger)
	adapter.Debug("lock debug", concurrency.LockField{Key: "lk", Value: "val"})
	adapter.Warn("lock warn", concurrency.LockField{Key: "lk", Value: "val"})

	out := buf.String()
	if !strings.Contains(out, "lock debug") || !strings.Contains(out, "lock warn") {
		t.Errorf("missing lock output: %s", out)
	}

	adapterNil := NewLockLoggerAdapter(nil)
	if adapterNil != nil {
		t.Errorf("expected nil for NewLockLoggerAdapter(nil)")
	}

	pLogger := GetLockLoggerFromProfile("system")
	if pLogger == nil {
		t.Errorf("expected non-nil from GetLockLoggerFromProfile")
	}
}

func TestFormatters_Extended(t *testing.T) {
	ctx := pkgctx.NewSystemContext()

	// Compact formatter
	cf := NewCompactFormatter(ctx)
	entry := &LogEntry{
		Timestamp: time.Now(),
		Level:     InfoLevel,
		Message:   "compact msg",
		Fields:    map[string]any{"key": "val"},
	}
	bytesCompact, err := cf.Format(entry)
	if err != nil || !strings.Contains(string(bytesCompact), "compact msg") {
		t.Errorf("compact formatter failed: %v, out: %s", err, string(bytesCompact))
	}

	// JSON formatter
	jf := NewJSONFormatter(ctx)
	bytesJSON, err := jf.Format(entry)
	if err != nil || !strings.Contains(string(bytesJSON), `"message":"compact msg"`) {
		t.Errorf("json formatter failed: %v, out: %s", err, string(bytesJSON))
	}
}

func TestFluentBuilder_RemainingMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	Fluent(logger).Info("full_coverage").
		ValidatorCompleted(10).
		ProgressGap(5).
		MaxAllowedGap(2).
		ValidatorQueueSize(3).
		PendingInQueue("a,b").
		DiagNote("note").
		ExpectedTotal(100).
		MissingPercent("5%").
		FilterExpr("state==active").
		KeyID("k-123").
		NotifyType("alert").
		Capped(50).
		KindsSummary("task,bli").
		EmitComponent("orchestrator").
		TraceMarker().
		RPCMethod("tools/call").
		RequestID("req-99").
		RPCMessageType("request").
		ParamsText(`{"a":1}`).
		ParamsData(map[string]any{"a": 1}).
		RPCResult(map[string]any{"ok": true}).
		TraceFile("/tmp/trace.log").
		MCPLogLevel("debug").
		Reason("timeout").
		Op("sync").
		OpDetail("sync detail").
		StorageProfileName("sqlite").
		Success(true).
		Enabled(false).
		Timeout("30s").
		HTTPStatusCode(200).
		IdleDuration("5m").
		RetryAttempt(1).
		RetryMaxAttempts(3).
		RetryDelay("1s").
		BackoffStrategy("exponential").
		MaxBackoffMs(5000).
		DefaultTimeoutMs(1000).
		ValidationErrorCount(2).
		WorkerStopTimeout("10s").
		CacheSaveTimeout("5s").
		AuthType("bearer").
		IDE("vscode").
		Dest("/path/dest").
		Action("run").
		Version("1.0.0").
		Interval("10s").
		Prompt("do this").
		ChangeType("insert").
		SkillID("sk-1").
		EnvelopeID("env-1").
		ToolName("bash").
		Bool("flag", true).
		Int("num", 42).
		String("str", "val").
		WithError(errors.New("custom err")).
		Log()

	out := buf.String()
	if !strings.Contains(out, "full_coverage") || !strings.Contains(out, "validator_completed=10") {
		t.Errorf("missing expected fluent builder log output: %s", out)
	}

	// Direct field helpers
	_ = EnvelopeIDField("env-99")
	_ = SkillIDField("sk-99")
	_ = ToolCallIDField("tc-99")
}

func TestLogger_Helpers(t *testing.T) {
	// ParseLevel
	levels := []struct {
		input    string
		expected LogLevel
		ok       bool
	}{
		{"", InfoLevel, false},
		{"debug", DebugLevel, true},
		{"DEBUG", DebugLevel, true},
		{"info", InfoLevel, true},
		{"INFO", InfoLevel, true},
		{"warn", WarnLevel, true},
		{"warning", WarnLevel, true},
		{"error", ErrorLevel, true},
		{"fatal", FatalLevel, true},
		{"unknown", InfoLevel, false},
	}

	for _, tc := range levels {
		lvl, ok := ParseLevel(tc.input)
		if lvl != tc.expected || ok != tc.ok {
			t.Errorf("ParseLevel(%q) = (%v, %v), expected (%v, %v)", tc.input, lvl, ok, tc.expected, tc.ok)
		}
	}

	// ErrField
	if fields := ErrField(nil); fields != nil {
		t.Errorf("expected nil for ErrField(nil), got %v", fields)
	}
	if fields := ErrField(errors.New("test err")); len(fields) != 1 || fields[0].Key != "error" {
		t.Errorf("expected 1 error field, got %v", fields)
	}

	// Timestamp
	tsField := Timestamp(time.Now())
	if tsField.Key != "timestamp" {
		t.Errorf("expected timestamp key, got %s", tsField.Key)
	}

	// NewBufferedLogger
	var buf bytes.Buffer
	ctx := pkgctx.NewSystemContext()
	formatter := NewTextFormatter(ctx)
	bLogger := NewBufferedLogger(&buf, DebugLevel, formatter)
	bLogger.Info("buffered info")
	_ = TryCloseLoggerDestinations(bLogger)

	stdoutLogger := NewBufferedLogger(os.Stdout, InfoLevel, formatter)
	if stdoutLogger == nil {
		t.Errorf("expected non-nil stdoutLogger")
	}

	// LogSwallowedError
	LogSwallowedError(nil)
	LogSwallowedError(errors.New("intentional error swallowed for test"))
}

func TestFormatters_SanitizeAndNewlines(t *testing.T) {
	// flattenSingleLineFieldValue
	flatStr := flattenSingleLineFieldValue("line1\nline2\rline3")
	if flatStr != "line1 line2 line3" {
		t.Errorf("expected flattened string, got %v", flatStr)
	}
	if notStr := flattenSingleLineFieldValue(123); notStr != 123 {
		t.Errorf("expected non-string unmodified, got %v", notStr)
	}

	// sanitizeForJSON
	if sanitizeForJSON(nil) != nil {
		t.Errorf("expected nil for sanitizeForJSON(nil)")
	}
	fn := func() {}
	if res := sanitizeForJSON(fn); res != "<func>" {
		t.Errorf("expected <func>, got %v", res)
	}
	ch := make(chan int)
	if res := sanitizeForJSON(ch); res != "<chan>" {
		t.Errorf("expected <chan>, got %v", res)
	}
	if res := sanitizeForJSON(&fn); res != "<*func>" {
		t.Errorf("expected <*func>, got %v", res)
	}
	if res := sanitizeForJSON(&ch); res != "<*chan>" {
		t.Errorf("expected <*chan>, got %v", res)
	}
	if res := sanitizeForJSON("valid"); res != "valid" {
		t.Errorf("expected 'valid', got %v", res)
	}

	// JSONFormatter with problematic fields
	ctx := pkgctx.NewSystemContext()
	jf := NewJSONFormatter(ctx)
	entry := &LogEntry{
		Timestamp: time.Now(),
		Level:     ErrorLevel,
		Message:   "json with error and complex types",
		Error:     errors.New("boom"),
		Fields: map[string]any{
			"valid_int":  42,
			"valid_str":  "hello",
			"func_field": fn,
			"chan_field": ch,
		},
	}
	data, err := jf.Format(entry)
	if err != nil {
		t.Fatalf("JSONFormatter.Format failed: %v", err)
	}
	if !strings.Contains(string(data), `"error":"boom"`) || !strings.Contains(string(data), `"valid_int":42`) {
		t.Errorf("unexpected json output: %s", string(data))
	}
}

func TestRollingWriter_FactoryAndRotation(t *testing.T) {
	dir := t.TempDir()
	factory := NewRollingWriterFactory()

	// Simple writer (strategy: none)
	simplePath := filepath.Join(dir, "simple.log")
	simpleWriter, err := factory.CreateWriter(simplePath, RollingPolicy{Strategy: "none"})
	if err != nil {
		t.Fatalf("CreateWriter none failed: %v", err)
	}
	n, err := simpleWriter.Write([]byte("simple test\n"))
	if err != nil || n == 0 {
		t.Fatalf("simpleWriter Write failed: %v", err)
	}
	if err := simpleWriter.Sync(); err != nil {
		t.Fatalf("simpleWriter Sync failed: %v", err)
	}
	if err := simpleWriter.Close(); err != nil {
		t.Fatalf("simpleWriter Close failed: %v", err)
	}

	// Fallback strategies: time and size_and_time
	timeWriter, err := factory.CreateWriter(filepath.Join(dir, "time.log"), RollingPolicy{Strategy: "time"})
	if err != nil {
		t.Fatalf("CreateWriter time failed: %v", err)
	}
	_ = timeWriter.Close()

	bothWriter, err := factory.CreateWriter(filepath.Join(dir, "both.log"), RollingPolicy{Strategy: "size_and_time"})
	if err != nil {
		t.Fatalf("CreateWriter size_and_time failed: %v", err)
	}
	_ = bothWriter.Close()

	defaultWriter, err := factory.CreateWriter(filepath.Join(dir, "default.log"), RollingPolicy{})
	if err != nil {
		t.Fatalf("CreateWriter default failed: %v", err)
	}
	_ = defaultWriter.Close()

	// Size-based rolling writer with rotation
	rollPath := filepath.Join(dir, "roll.log")
	cfg := SizeBasedRollingConfig{
		MaxSize:  30, // low max size to easily trigger rotation
		MaxFiles: 2,
	}
	rollWriter, err := NewSizeBasedRollingWriter(rollPath, cfg)
	if err != nil {
		t.Fatalf("NewSizeBasedRollingWriter failed: %v", err)
	}

	// Write 20 bytes (< 30)
	_, err = rollWriter.Write([]byte("12345678901234567890"))
	if err != nil {
		t.Fatalf("Write 1 failed: %v", err)
	}

	// Write another 20 bytes -> total 40 > 30, triggers rotateLocked before writing
	_, err = rollWriter.Write([]byte("abcdefghijklmnopqrst"))
	if err != nil {
		t.Fatalf("Write 2 failed: %v", err)
	}

	// Write 50 bytes -> triggers rotateLocked after write
	_, err = rollWriter.Write([]byte(strings.Repeat("X", 50)))
	if err != nil {
		t.Fatalf("Write 3 failed: %v", err)
	}

	if err := rollWriter.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if err := rollWriter.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify rotated files exist
	files, _ := os.ReadDir(dir)
	var rotatedCount int
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "roll-") {
			rotatedCount++
		}
	}
	if rotatedCount == 0 {
		t.Errorf("expected rotated files in %s, found %d", dir, rotatedCount)
	}
}

func TestLogRouter_Full(t *testing.T) {
	dir := t.TempDir()
	ctx := pkgctx.NewSystemContext()
	router := NewLogRouter()

	var buf1, buf2 bytes.Buffer
	router.AddDestination("mem1", &buf1, ErrorLevel, NewTextFormatter(ctx))

	// Add same destination with more permissive level (DebugLevel < ErrorLevel) -> triggers update
	router.AddDestination("mem1", &buf1, DebugLevel, NewTextFormatter(ctx))

	// Add same destination with less permissive level -> no-op
	router.AddDestination("mem1", &buf1, WarnLevel, NewTextFormatter(ctx))

	// Add file destinations
	file1Path := filepath.Join(dir, "dest1.log")
	err := router.AddFileDestinationWithPolicy("f1", file1Path, InfoLevel, NewCompactFormatter(ctx), &RollingPolicy{
		Strategy: "size",
		MaxSize:  1024,
		MaxFiles: 2,
	})
	if err != nil {
		t.Fatalf("AddFileDestinationWithPolicy failed: %v", err)
	}

	// Add existing destination f1 again -> early return
	err = router.AddFileDestinationWithPolicy("f1", file1Path, InfoLevel, NewCompactFormatter(ctx), nil)
	if err != nil {
		t.Fatalf("AddFileDestinationWithPolicy repeat failed: %v", err)
	}

	// Add default policy file destination
	file2Path := filepath.Join(dir, "dest2.log")
	err = router.AddFileDestination("f2", file2Path, InfoLevel, NewTextFormatter(ctx))
	if err != nil {
		t.Fatalf("AddFileDestination failed: %v", err)
	}

	// Add second memory destination
	router.AddDestination("mem2", &buf2, InfoLevel, NewJSONFormatter(ctx))

	// Logging methods
	rLog := router.GetLogger()
	rLog.Debug("debug routed")
	rLog.Info("info routed")
	rLog.Warn("warn routed")
	rLog.Error("error routed", errors.New("err routed"))

	sub2 := rLog.WithFields(String("k1", "v1"), Int("k2", 2))
	sub2.Info("sub2 info")

	// Remove destination
	router.RemoveDestination("mem2")
	router.RemoveDestination("non_existent")

	// Close
	if err := router.Close(); err != nil {
		t.Fatalf("router Close failed: %v", err)
	}
	// Double close is safe
	if err := router.Close(); err != nil {
		t.Fatalf("second router Close failed: %v", err)
	}

	// Check buffer content
	out := buf1.String()
	if !strings.Contains(out, "debug routed") || !strings.Contains(out, "error routed") {
		t.Errorf("missing expected output in router destination: %s", out)
	}
}

func TestRouterInit_Global(t *testing.T) {
	ctx := pkgctx.NewSystemContext()

	// Ensure router is closed
	_ = CloseGlobalRouter()

	// Initialize with empty projectRoot (skips file logging)
	err := InitializeGlobalRouter("", NewJSONFormatter(ctx), "test-profile", DebugLevel)
	if err != nil {
		t.Errorf("InitializeGlobalRouter failed: %v", err)
	}

	// Close again
	err = CloseGlobalRouter()
	if err != nil {
		t.Errorf("CloseGlobalRouter failed: %v", err)
	}
}
