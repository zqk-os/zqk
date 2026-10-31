package logging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type testCLIContext struct {
	Profile string
}

type testFailingFormatter struct{}

func (f *testFailingFormatter) Format(entry *LogEntry) ([]byte, error) {
	return nil, errors.New("simulated format failure")
}

type testFlakyJSONValue struct {
	calls *int
}

func (f testFlakyJSONValue) MarshalJSON() ([]byte, error) {
	*f.calls++
	switch *f.calls {
	case 1:
		// sanitizeForJSON testData marshal
		return []byte(`"ok1"`), nil
	case 2:
		// json.Marshal(data) 1st attempt: fail
		return nil, errors.New("fail on 1st marshal")
	case 3:
		// problematicFields check: fail so it gets replaced
		return nil, errors.New("fail on testData check")
	default:
		return []byte(`"ok"`), nil
	}
}

type testAlwaysFailsSecondMarshal struct {
	calls *int
}

func (f testAlwaysFailsSecondMarshal) MarshalJSON() ([]byte, error) {
	*f.calls++
	switch *f.calls {
	case 1:
		// sanitizeForJSON: pass
		return []byte(`"ok1"`), nil
	case 2:
		// json.Marshal(data) 1st attempt: fail
		return nil, errors.New("fail 1")
	case 3:
		// testData check: pass so it is NOT replaced
		return []byte(`"ok3"`), nil
	case 4:
		// json.Marshal(data) 2nd attempt: fail!
		return nil, errors.New("fail 2")
	default:
		return []byte(`"ok"`), nil
	}
}

type testFailingWriter struct{}

func (f *testFailingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("simulated write error")
}

type testFailingCloser struct {
	io.Writer
}

func (f *testFailingCloser) Close() error {
	return errors.New("simulated close error")
}

type testMockSyncer struct {
	io.Writer
	synced bool
}

func (m *testMockSyncer) Sync() error {
	m.synced = true
	return nil
}

type testFailingProgressFormatter struct{}

func (f *testFailingProgressFormatter) SupportsOverwrite() bool { return false }
func (f *testFailingProgressFormatter) FormatProgress(operationID string, progress int, message string, fields map[string]any) ([]byte, error) {
	return nil, errors.New("simulated progress format error")
}
func (f *testFailingProgressFormatter) FormatStatus(operationID, oldStatus, newStatus string, fields map[string]any) ([]byte, error) {
	return nil, errors.New("simulated status format error")
}
func (f *testFailingProgressFormatter) FormatStreamEvent(eventType string, data any, fields map[string]any) ([]byte, error) {
	return nil, errors.New("simulated stream event error")
}

// TestCycle311_FluentBuilderComprehensive covers all unexercised fluent entry methods and edge cases.
func TestCycle311_FluentBuilderComprehensive(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	f := Fluent(logger)

	// Trace method
	f.Trace("trace message").Log()

	// All missing builder methods
	entry := f.Info("broad metric verification").
		EventsUpdated(12).
		Loaded(42).
		TotalCheckResults(99).
		Recovered(5).
		FailedOps(2).
		SkippedOps(1).
		BuildTime("2026-10-31T00:00:00Z").
		TotalTasks(50).
		EnqueuedObjectIDs(25).
		TasksActuallyEnqueued(25).
		EnqueuedIDs(25).
		TasksPassedToValidator(25).
		CacheHits(100).
		CacheMisses(10).
		ProgressCount(40).
		CachedCount(60).
		TotalAccounted(100).
		TotalEnqueued(100).
		MissingCount(0).
		Tolerance(5).
		ProgressReceived(35).
		AutoFixableCount(3).
		Accounted(90).
		Completed(88).
		ActiveWorkers(4).
		GoroutineID(1234).
		ActiveWorkersRemainingAtTimeout(1).
		WorkerStateLabel("idle").
		DiagnosticDetail("all checks passed").
		WaitTime("50ms").
		TotalDrained(15).
		ValidationDrainCount(15).
		ValidationProgressStatus("in_progress").
		PercentComplete("75.5%").
		TimeSinceProgress("1.5s").
		QueueEmptyDuration("200ms").
		EmptyDuration("100ms").
		StuckDuration("300ms").
		AccountedPercent("95.0%").
		ValidatorCompleted(10).
		ProgressGap(2).
		MaxAllowedGap(5).
		ValidatorQueueSize(3).
		PendingInQueue("a,b").
		DiagNote("healthy").
		ExpectedTotal(50)

	if len(entry.fields) != 46 {
		t.Fatalf("expected 46 fields on entry, got %d", len(entry.fields))
	}
	entry.Log()

	// Nil receiver and nil logger safety
	var nilEntry *fluentEntry
	nilEntry.Log()

	noLoggerEntry := (&fluentEntry{}).WithFields(String("key", "val"))
	noLoggerEntry.Log()

	// Unlevelled entry fallback
	unlevelled := (&FluentRoot{logger: logger}).newEntry("unknown_level_name", "unlevelled message", nil)
	unlevelled.Log()

	// Large fields slice to trigger release cleanup (cap > 64)
	largeEntry := f.Info("large fields")
	for i := 0; i < 70; i++ {
		largeEntry.Int(fmt.Sprintf("k_%d", i), i)
	}
	largeEntry.Log()
}

// TestCycle311_ContextLoggerAndMCPMode covers context extraction, MCP mode profiles, and decision contexts.
func TestCycle311_ContextLoggerAndMCPMode(t *testing.T) {
	// Profile string extraction from context
	baseCtx := context.Background()

	// 1. ctx with direct string "profile"
	pCtx1 := context.WithValue(baseCtx, "profile", "debug")
	l1 := GetLoggerFromContext(pCtx1)
	if l1 == nil {
		t.Fatal("expected logger for string profile")
	}

	// 2. ctx with non-string "profile"
	pCtx2 := context.WithValue(baseCtx, "profile", 12345)
	l2 := GetLoggerFromContext(pCtx2)
	if l2 == nil {
		t.Fatal("expected fallback logger for non-string profile")
	}

	// 3. ctx with cli_context pointer
	pCtx3 := context.WithValue(baseCtx, "cli_context", &testCLIContext{Profile: "ai-agent"})
	l3 := GetLoggerFromContext(pCtx3)
	if l3 == nil {
		t.Fatal("expected logger for ptr cli_context")
	}

	// 4. ctx with cli_context struct value
	pCtx4 := context.WithValue(baseCtx, "cli_context", testCLIContext{Profile: "system"})
	l4 := GetLoggerFromContext(pCtx4)
	if l4 == nil {
		t.Fatal("expected logger for struct cli_context")
	}

	// 5. ctx with invalid cli_context
	pCtx5 := context.WithValue(baseCtx, "cli_context", "not_a_struct")
	l5 := GetLoggerFromContext(pCtx5)
	if l5 == nil {
		t.Fatal("expected fallback logger for invalid cli_context")
	}

	// Profile creation under normal mode
	profiles := []string{
		string(pkgctx.ProfileMCP),
		string(pkgctx.ProfileSystem),
		string(pkgctx.ProfileAIAgent),
		string(pkgctx.ProfileDebug),
		string(pkgctx.ProfileHuman),
		"",
		"unrecognized_custom_profile",
	}

	for _, prof := range profiles {
		lc := createLoggingContextFromProfile(prof)
		if lc == nil {
			t.Fatalf("expected non-nil logging context for profile %q", prof)
		}
		_ = GetLoggerFromProfile(prof)
		_ = GetLoggerFromProfile(prof, context.Background())
		_ = GetLoggerFromLoggingContext(context.Background(), lc)
	}

	// Nil logging context fallback
	_ = GetLoggerFromLoggingContext(context.Background(), nil)

	// Profile creation under simulated MCP mode
	origMCP := zqkenv.MCPAccountID().Get()
	defer zqkenv.MCPAccountID().Set(origMCP)

	zqkenv.MCPAccountID().Set("mcp-worker-test-account")

	for _, prof := range profiles {
		lc := createLoggingContextFromProfile(prof)
		if lc == nil {
			t.Fatalf("expected non-nil logging context for MCP profile %q", prof)
		}
		_ = GetLoggerFromLoggingContext(context.Background(), lc)
	}

	// Decision context logger with and without OperationCtx
	dclNil := GetLoggerFromDecisionContext(nil, "")
	if dclNil == nil {
		t.Fatal("expected logger for nil decision context")
	}

	dctxWithOp := &pkgctx.LoggingDecisionContext{
		OperationCtx: context.WithValue(context.Background(), "op", "sync"),
		LoggingCtx:   pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent),
		Component:    "test_component",
		Operation:    "test_op",
	}
	dclOp := GetLoggerFromDecisionContext(dctxWithOp, "")
	if dclOp == nil {
		t.Fatal("expected logger for decision context with op")
	}

	dctxNoLoggingCtx := &pkgctx.LoggingDecisionContext{
		Component: "comp_only",
	}
	dclNoLC := GetLoggerFromDecisionContext(dctxNoLoggingCtx, "")
	if dclNoLC == nil {
		t.Fatal("expected logger for decision context without logging ctx")
	}
}

// TestCycle311_DecisionContextLoggerBranches covers decision context routing, stdio destinations, format errors, fatal panic.
func TestCycle311_DecisionContextLoggerBranches(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "decision_test.log")

	policy := &pkgctx.RollingPolicy{
		Strategy: "size",
		MaxSize:  1024 * 1024,
		MaxFiles: 3,
	}

	router := NewLogRouter()
	defer router.Close()

	dcl := &decisionContextLogger{
		router:      router,
		projectRoot: tempDir,
		destCache:   make(map[string]*destination),
		decisionCtx: &pkgctx.LoggingDecisionContext{
			Component:  "worker_service",
			Operation:  "process_queue",
			MCPCtx:     pkgctx.GetMCPServerContext(),
			LoggingCtx: pkgctx.NewLoggingContext(pkgctx.ProfileDebug).WithSuppressDebugToStdout(true),
		},
		loggingCtx: pkgctx.NewLoggingContext(pkgctx.ProfileDebug).WithSuppressDebugToStdout(true),
	}

	// Exercise getOrCreateDestination with various configs
	destFile := dcl.getOrCreateDestination("dest_file", &pkgctx.LogDestination{
		Name:          "dest_file",
		FilePath:      logPath,
		Level:         int(DebugLevel),
		Formatter:     "json",
		RollingPolicy: policy,
	})
	if destFile == nil {
		t.Fatal("expected non-nil destFile")
	}

	destStdout := dcl.getOrCreateDestination("dest_stdout", &pkgctx.LogDestination{
		Name:      "dest_stdout",
		Writer:    "stdout",
		Level:     int(InfoLevel),
		Formatter: "text",
	})
	if destStdout == nil {
		t.Fatal("expected non-nil destStdout")
	}

	destStderr := dcl.getOrCreateDestination("dest_stderr", &pkgctx.LogDestination{
		Name:      "dest_stderr",
		Writer:    "stderr",
		Level:     int(WarnLevel),
		Formatter: "compact",
	})
	if destStderr == nil {
		t.Fatal("expected non-nil destStderr")
	}

	destInvalidWriter := dcl.getOrCreateDestination("dest_invalid_writer", &pkgctx.LogDestination{
		Name:      "dest_invalid_writer",
		Writer:    "unknown_writer",
		Level:     int(DebugLevel),
		Formatter: "default",
	})
	if destInvalidWriter != nil {
		t.Fatal("expected nil for invalid writer")
	}

	destEmpty := dcl.getOrCreateDestination("dest_empty", &pkgctx.LogDestination{
		Name: "dest_empty",
	})
	if destEmpty != nil {
		t.Fatal("expected nil for empty dest")
	}

	// Log with component and operation fields explicitly set
	dcl.Debug("debugging decision logger",
		Field{Key: DecisionLogFieldKeyComponent, Value: "custom_comp"},
		Field{Key: DecisionLogFieldKeyOperation, Value: "custom_op"},
	)
	dcl.Info("info message")
	dcl.Warn("warn message")
	dcl.Error("error message", errors.New("simulated error"))
	_ = dcl.WithFields(String("extra", "field"))
	_ = dcl.WithContext(context.Background())
	_ = dcl.WithObjectRef("task", "TASK-1")

	// Fatal panic recovery
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected Fatal to panic")
		}
	}()
	dcl.Fatal("fatal error", errors.New("fatal simulated error"))
}

// TestCycle311_DecisionContextLoggerDestinationEdgeCases covers destination creation failures and close errors.
func TestCycle311_DecisionContextLoggerDestinationEdgeCases(t *testing.T) {
	// Destination formatting fallback
	dest := &destination{
		name:      "failing_formatter_dest",
		writer:    &bytes.Buffer{},
		level:     DebugLevel,
		formatter: &testFailingFormatter{},
	}

	dcl := &decisionContextLogger{
		router:    GetGlobalRouter(),
		destCache: map[string]*destination{"failing": dest},
	}
	dcl.writeToDestination(dest, &LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     InfoLevel,
		Message:   "testing fallback on format error",
	})

	// Close error branch
	failingDest := &destination{
		name:   "failing_close",
		closer: &testFailingCloser{Writer: &bytes.Buffer{}},
	}
	dclWithFailingCloser := &decisionContextLogger{
		destCache: map[string]*destination{"fail": failingDest},
	}
	err := dclWithFailingCloser.Close()
	if err == nil {
		t.Fatal("expected error closing destinations")
	}
}

// TestCycle311_RouterEdgeCasesAndFatal covers router Fatal panic, formatting errors, and fallback writers.
func TestCycle311_RouterEdgeCasesAndFatal(t *testing.T) {
	router := NewLogRouter()
	defer router.Close()

	var outBuf bytes.Buffer
	router.AddDestination("buf_dest", &outBuf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	router.AddStdoutDestination(InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	router.AddStderrDestination(WarnLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	rl := router.GetLogger()
	rl.Debug("debug message")
	rl.Info("info message")
	rl.Warn("warn message")
	rl.Error("error message", errors.New("test error"))
	_ = rl.WithFields(String("field", "val"))
	_ = rl.WithContext(context.Background())
	_ = rl.WithObjectRef("task", "TASK-1")

	// Router logger Fatal recovery
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected routerLogger.Fatal to panic")
			}
		}()
		rl.Fatal("critical router failure", errors.New("fatal panic"))
	}()

	// Router with failing formatter to trigger fallback writer branch
	failingRouter := NewLogRouter()
	defer failingRouter.Close()
	failingRouter.AddDestination("failing_buf", &bytes.Buffer{}, DebugLevel, &testFailingFormatter{})
	failingLogger := failingRouter.GetLogger()
	failingLogger.Info("message that triggers format error fallback")

	// Remove destination
	failingRouter.RemoveDestination("failing_buf")
	failingRouter.RemoveDestination("non_existent")
}

// TestCycle311_RouterInitGlobalMulti covers InitializeGlobalRouter and CloseGlobalRouter.
func TestCycle311_RouterInitGlobalMulti(t *testing.T) {
	// Empty project root skips file logging
	_ = concurrency.WithLock(
		&globalRouterMu,
		"test_reset_global_router",
		func() error {
			globalRouter = nil
			globalRouterOnce = sync.Once{}
			return nil
		},
	)

	err := InitializeGlobalRouter("", NewTextFormatter(pkgctx.NewSystemContext()), "system", InfoLevel)
	if err != nil {
		t.Fatalf("expected no error for empty projectRoot, got %v", err)
	}

	gr := GetGlobalRouter()
	if gr == nil {
		t.Fatal("expected non-nil global router")
	}

	if err := CloseGlobalRouter(); err != nil {
		t.Fatalf("failed to close global router: %v", err)
	}

	// Initialize with temp directory and JSONFormatter
	tempDir := t.TempDir()
	_ = concurrency.WithLock(
		&globalRouterMu,
		"test_reset_global_router_2",
		func() error {
			globalRouter = nil
			globalRouterOnce = sync.Once{}
			return nil
		},
	)

	err = InitializeGlobalRouter(tempDir, NewJSONFormatter(pkgctx.NewSystemContext()), "mcp", DebugLevel)
	if err != nil {
		t.Fatalf("expected no error for mcp json init, got %v", err)
	}
	_ = CloseGlobalRouter()
}

// TestCycle311_EventLoggerComprehensive covers all unexercised EventLogger methods.
func TestCycle311_EventLoggerComprehensive(t *testing.T) {
	ctx := context.Background()
	el := NewEventLogger(ctx)

	// extractProfileFromContext directly
	ctxDirect := context.WithValue(ctx, "profile", "ai-agent")
	if extractProfileFromContext(ctxDirect) != "ai-agent" {
		t.Errorf("expected ai-agent profile")
	}

	ctxStruct := context.WithValue(ctx, "cli_context", &testCLIContext{Profile: "debug"})
	if extractProfileFromContext(ctxStruct) != "debug" {
		t.Errorf("expected debug profile")
	}

	// LogCheckResult
	el.LogCheckResult("goal", 10, 0, nil)
	el.LogCheckResult("goal", 10, 2, map[int]int{1: 2})
	el.LogCheckResult("goal", 10, 2, map[int]int{2: 2})

	// LogObjectCheck
	el.LogObjectCheck("G-1", "goal", []CheckIssue{
		{Category: "naming", Message: "bad name", Tier: 1},
		{Category: "metadata", Message: "missing tag", Tier: 2},
	})
	el.LogObjectCheck("G-2", "goal", []CheckIssue{
		{Category: "metadata", Message: "warning only", Tier: 2},
	})
	el.LogObjectCheck("G-3", "goal", nil)

	// LogFileOperation
	tempDir := t.TempDir()
	existingFile := filepath.Join(tempDir, "existing.txt")
	_ = fileutil.WriteFile(existingFile, []byte("content"), 0600)
	el.LogFileOperation("read", existingFile, nil)
	el.LogFileOperation("write", filepath.Join(tempDir, "non_existent.txt"), errors.New("file not found"))

	// LogHashOperation
	el.LogHashOperation("verify", "bli", "spec.yaml", "abcdef1234567890abcdef", nil)
	el.LogHashOperation("verify", "bli", "spec.yaml", "", errors.New("empty hash"))

	// LogValidationError
	el.LogValidationError(ValidationError{
		Field:       "title",
		MissingItem: "required",
		Message:     "title is empty",
		CriteriaRef: "CRIT-001",
	})
	el.LogValidationError(ValidationError{
		Field:   "summary",
		Message: "summary invalid",
	})

	// LogInfo, LogDebug, LogWarning, LogError
	el.LogInfo("info log", Field{Key: "k1", Value: "v1"})
	el.LogDebug("debug log", Field{Key: "k2", Value: "v2"})
	el.LogWarning("warn log", Field{Key: "k3", Value: "v3"})
	el.LogError("error log", errors.New("sample err"), Field{Key: "k4", Value: "v4"})

	_ = el.Logger()
	_ = el.WithObjectRef("goal", "G-1")
	_ = el.WithFields(String("with", "field"))
}

// TestCycle311_LoggerLevelsAndFormatters covers LogLevel strings, parsing, TryClose, format error fallback, and fatal panic.
func TestCycle311_LoggerLevelsAndFormatters(t *testing.T) {
	// LogLevel.String()
	if FatalLevel.String() != "fatal" {
		t.Errorf("expected fatal, got %s", FatalLevel.String())
	}
	if LogLevel(999).String() != "unknown" {
		t.Errorf("expected unknown, got %s", LogLevel(999).String())
	}

	// ParseLevel
	cases := []struct {
		input    string
		expected LogLevel
		ok       bool
	}{
		{"", InfoLevel, false},
		{"debug", DebugLevel, true},
		{"DEBUG", DebugLevel, true},
		{"info", InfoLevel, true},
		{"warn", WarnLevel, true},
		{"warning", WarnLevel, true},
		{"error", ErrorLevel, true},
		{"fatal", FatalLevel, true},
		{"invalid_level", InfoLevel, false},
	}

	for _, c := range cases {
		lvl, ok := ParseLevel(c.input)
		if lvl != c.expected || ok != c.ok {
			t.Errorf("ParseLevel(%q) = (%v, %v), expected (%v, %v)", c.input, lvl, ok, c.expected, c.ok)
		}
	}

	// TryCloseLoggerDestinations
	if err := TryCloseLoggerDestinations(nil); err != nil {
		t.Errorf("unexpected error for nil: %v", err)
	}

	var buf bytes.Buffer
	stdLogger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	if err := TryCloseLoggerDestinations(stdLogger); err != nil {
		t.Errorf("unexpected error for non-closer: %v", err)
	}

	// Standard logger Fatal panic
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected standard logger Fatal to panic")
			}
		}()
		stdLogger.Fatal("fatal standard error", errors.New("boom"))
	}()

	// Standard logger with failing formatter
	var fallbackBuf bytes.Buffer
	loggerWithFailingFormatter := NewLogger(&fallbackBuf, InfoLevel, &testFailingFormatter{})
	loggerWithFailingFormatter.Info("triggers standard logger fallback write")

	// JSONFormatter with unmarshalable and problematic fields
	jf := NewJSONFormatter(pkgctx.NewSystemContext())
	ch := make(chan int)
	fn := func() {}
	flakyCalls := 0
	entryWithComplexFields := &LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     InfoLevel,
		Message:   "complex fields test",
		Error:     errors.New("sample error"),
		Fields: map[string]any{
			"chan_field": ch,
			"func_field": fn,
			"ptr_func":   &fn,
			"ptr_chan":   &ch,
			"flaky":      testFlakyJSONValue{calls: &flakyCalls},
		},
	}
	bytesOut, err := jf.Format(entryWithComplexFields)
	if err != nil {
		t.Fatalf("expected JSONFormatter to recover from problematic fields, got error: %v", err)
	}
	if len(bytesOut) == 0 {
		t.Fatal("expected non-empty output from JSONFormatter")
	}

	alwaysFailCalls := 0
	entryAlwaysFails := &LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     InfoLevel,
		Message:   "always fail fields",
		Fields: map[string]any{
			"fail_field": testAlwaysFailsSecondMarshal{calls: &alwaysFailCalls},
		},
	}
	_, err = jf.Format(entryAlwaysFails)
	if err == nil {
		t.Fatal("expected error on unrecoverable JSON format failure")
	}

	// LockLoggerAdapter
	lla := NewLockLoggerAdapter(stdLogger)
	lla.Debug("lock debug message")
	lla.Warn("lock warn message")
	lla.Debug("lock debug with fields", concurrency.LockField{Key: "lock_name", Value: "mu1"})
	lla.Warn("lock warn with fields", concurrency.LockField{Key: "lock_name", Value: "mu1"})

	var nilLLA *LockLoggerAdapter
	nilLLA.Debug("nil lla debug")
	nilLLA.Warn("nil lla warn")

	emptyLLA := &LockLoggerAdapter{logger: nil}
	emptyLLA.Debug("empty lla debug")
	emptyLLA.Warn("empty lla warn")

	_ = GetLockLoggerFromProfile("debug")
	_ = NewLockLoggerAdapter(nil)
}

// TestCycle311_ProgressLoggerFailuresAndMCP covers ProgressLogger error handling and fallback paths.
func TestCycle311_ProgressLoggerFailuresAndMCP(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	failingFormatter := &testFailingProgressFormatter{}
	pl := NewProgressLoggerWithWriter(baseLogger, failingFormatter, &buf)

	// Format errors should fall back to baseLogger.Warn without crashing
	pl.Progress("op1", 50, "halfway")
	pl.Status("op1", "starting", "running")
	err := pl.Stream(context.Background(), "stream_event", "sample_data")
	if err == nil {
		t.Errorf("expected Stream to return error on formatter failure")
	}

	// MCP mode redirect in progressLogger.write
	origMCP := zqkenv.MCPAccountID().Get()
	defer zqkenv.MCPAccountID().Set(origMCP)

	zqkenv.MCPAccountID().Set("mcp-account-progress-test")
	plMCP := NewProgressLogger(baseLogger, NewTextProgressFormatter())
	plMCP.Progress("mcp_op", 100, "done")
}

// TestCycle311_RollingAndBufferedWriterEdgeCases covers simpleFileWriter, size rolling writer limits, and buffered writer sync.
func TestCycle311_RollingAndBufferedWriterEdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	// RollingWriterFactory strategies
	factory := NewRollingWriterFactory()
	timePath := filepath.Join(tempDir, "time.log")
	tw, err := factory.CreateWriter(timePath, RollingPolicy{Strategy: "time"})
	if err == nil && tw != nil {
		_ = tw.Close()
	}
	sizeTimePath := filepath.Join(tempDir, "size_time.log")
	stw, err := factory.CreateWriter(sizeTimePath, RollingPolicy{Strategy: "size_and_time"})
	if err == nil && stw != nil {
		_ = stw.Close()
	}
	defPath := filepath.Join(tempDir, "def.log")
	dw, err := factory.CreateWriter(defPath, RollingPolicy{Strategy: "unknown_strategy"})
	if err == nil && dw != nil {
		_ = dw.Close()
	}

	simplePath := filepath.Join(tempDir, "simple.log")
	sw, err := factory.CreateWriter(simplePath, RollingPolicy{Strategy: "none"})
	if err != nil {
		t.Fatalf("failed to create simple writer: %v", err)
	}
	_, err = sw.Write([]byte("simple file entry\n"))
	if err != nil {
		t.Fatalf("failed to write to simple writer: %v", err)
	}
	if err := sw.Sync(); err != nil {
		t.Fatalf("failed to sync simple writer: %v", err)
	}
	if err := sw.Close(); err != nil {
		t.Fatalf("failed to close simple writer: %v", err)
	}
	// Calling Close/Sync on already closed simple writer (nil file)
	_ = sw.Close()
	_ = sw.Sync()

	// NewSizeBasedRollingWriter with non-positive sizes and max files cleanup
	rollPath := filepath.Join(tempDir, "rolling.log")
	rw, err := NewSizeBasedRollingWriter(rollPath, SizeBasedRollingConfig{
		MaxSize:  -1, // should default
		MaxFiles: -1, // should default
	})
	if err != nil {
		t.Fatalf("failed to create rolling writer with defaults: %v", err)
	}
	_ = rw.Close()

	// Rolling writer with very small size to trigger multiple rotations and cleanupOldFilesLocked
	microPath := filepath.Join(tempDir, "micro_roll.log")
	microRW, err := NewSizeBasedRollingWriter(microPath, SizeBasedRollingConfig{
		MaxSize:  20,
		MaxFiles: 2,
	})
	if err != nil {
		t.Fatalf("failed to create micro rolling writer: %v", err)
	}

	for i := 0; i < 10; i++ {
		_, writeErr := microRW.Write([]byte(fmt.Sprintf("rotation_entry_payload_exceeding_max_size_%d\n", i)))
		if writeErr != nil {
			t.Fatalf("failed write during rotation: %v", writeErr)
		}
	}
	_ = microRW.Sync()
	_ = microRW.Close()

	// BufferedWriter with invalid config values
	var memBuf bytes.Buffer
	bw := NewBufferedWriter(&memBuf, BufferedWriterConfig{
		BufferSize:    -1,
		FlushInterval: -1,
	})
	if bw.Available() <= 0 {
		t.Errorf("expected positive available buffer size")
	}

	syncer := &testMockSyncer{Writer: &bytes.Buffer{}}
	bwSyncer := NewBufferedWriter(syncer, DefaultBufferedWriterConfig())
	_, _ = bwSyncer.Write([]byte("sync test\n"))
	if err := bwSyncer.Sync(); err != nil {
		t.Fatalf("failed to sync: %v", err)
	}
	if !syncer.synced {
		t.Errorf("expected underlying syncer to be called")
	}
	_ = bwSyncer.Close()

	// Fault injection: failing writer
	bwFailing := NewBufferedWriter(&testFailingWriter{}, DefaultBufferedWriterConfig())
	_, _ = bwFailing.Write([]byte("overflow data that exceeds buffer..."))
	_ = bwFailing.Flush()
	_ = bwFailing.Close()

	// WriteWithLevel error / fatal flush
	var levelBuf bytes.Buffer
	bwLevel := NewBufferedWriter(&levelBuf, BufferedWriterConfig{
		BufferSize:   1024,
		FlushOnError: true,
		FlushOnFatal: true,
	})
	_, _ = bwLevel.WriteWithLevel([]byte("error line\n"), ErrorLevel)
	if levelBuf.Len() == 0 {
		t.Errorf("expected immediate flush on error")
	}
	_ = bwLevel.Close()
}

// TestCycle311_ConcurrencyAndHygiene runs concurrent writers ensuring zero race conditions.
func TestCycle311_ConcurrencyAndHygiene(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "concurrent_audit.log")

	factory := NewRollingWriterFactory()
	writer, err := factory.CreateWriter(logPath, RollingPolicy{
		Strategy: "size",
		MaxSize:  500,
		MaxFiles: 3,
	})
	if err != nil {
		t.Fatalf("failed to create concurrent writer: %v", err)
	}
	defer writer.Close()

	buffered := NewBufferedWriter(writer, DefaultBufferedWriterConfig())
	defer buffered.Close()

	var wg sync.WaitGroup
	workers := 8
	iterations := 25

	for w := 0; w < workers; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_cycle311_concurrent_writer", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					msg := fmt.Sprintf("[worker-%d] concurrent log payload iteration %d\n", workerID, i)
					_, _ = buffered.Write([]byte(msg))
				}
			})
	}

	wg.Wait()
	_ = buffered.Flush()

	// Verify file was written using fileutil
	exists := fileutil.Exists(logPath)
	if !exists {
		t.Errorf("expected log file %s to exist", logPath)
	}
}

// TestCycle311_CommandOutputWriter covers all GetCommandOutputWriter routing conditions.
func TestCycle311_CommandOutputWriter(t *testing.T) {
	baseCtx := context.Background()

	// 1. Context override
	var customBuf bytes.Buffer
	ctxWithOverride := pkgctx.WithCommandOutputWriter(baseCtx, &customBuf)
	w1 := GetCommandOutputWriter(ctxWithOverride)
	if w1 != &customBuf {
		t.Errorf("expected custom writer from context")
	}

	// 2. Normal CLI usage
	w2 := GetCommandOutputWriter(baseCtx)
	if w2 == nil {
		t.Errorf("expected non-nil default writer")
	}

	// 3. MCP subprocess mode
	origMCP := zqkenv.MCPAccountID().Get()
	defer zqkenv.MCPAccountID().Set(origMCP)

	zqkenv.MCPAccountID().Set("mcp-account-output-test")
	w3 := GetCommandOutputWriter(baseCtx)
	if w3 == nil {
		t.Errorf("expected non-nil MCP subprocess writer")
	}
}
