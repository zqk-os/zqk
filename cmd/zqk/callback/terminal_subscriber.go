package callback

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// SubscriberNameTerminalProgress is the canonical identifier for the terminal progress subscriber.
	SubscriberNameTerminalProgress = "terminal_progress"

	// AnsiCarriageReturnClear is the ANSI escape sequence to return cursor to column 0 and clear the line.
	AnsiCarriageReturnClear = "\r\033[K"
)

var (
	// ErrTerminalSubscriberClosed indicates an operation was attempted on a closed TerminalProgressSubscriber.
	ErrTerminalSubscriberClosed = errfmt.Errorf("terminal subscriber is closed")

	// ErrTerminalSubscriberNil indicates the receiver was nil.
	ErrTerminalSubscriberNil = errfmt.Errorf("terminal subscriber is nil")

	// ansiColorRegex matches ANSI color and style escape sequences.
	ansiColorRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

var (
	_ CallbackSubscriber = (*TerminalProgressSubscriber)(nil)
	_ Subscriber         = (*TerminalProgressSubscriber)(nil)
)

// TerminalSubscriberConfig holds configuration options for TerminalProgressSubscriber.
type TerminalSubscriberConfig struct {
	Writer      io.Writer
	AnsiEnabled bool
	StripColors bool
	Prefix      string
	BufferSize  int
	DropOnFull  bool
}

// TerminalProgressSubscriber listens to callback entries and formats live updates to an io.Writer.
// It supports ANSI stream invalidation for zero-latency in-place redrawing.
type TerminalProgressSubscriber struct {
	cfg          TerminalSubscriberConfig
	writer       io.Writer
	mu           sync.Mutex
	eventCh      chan *CallbackEntry
	workerWg     sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	closed       atomic.Bool
	droppedCount atomic.Int64
	lastHadLF    bool
}

// NewTerminalProgressSubscriber creates a new terminal progress subscriber.
func NewTerminalProgressSubscriber(cfg TerminalSubscriberConfig) *TerminalProgressSubscriber {
	writer := cfg.Writer
	if writer == nil {
		writer = os.Stderr
	}

	sub := &TerminalProgressSubscriber{
		cfg:       cfg,
		writer:    writer,
		lastHadLF: true,
	}

	if cfg.BufferSize > 0 {
		sub.eventCh = make(chan *CallbackEntry, cfg.BufferSize)
		sub.ctx, sub.cancel = context.WithCancel(context.Background())
		sub.workerWg.Add(1)
		goroutinelabels.NewGoroutine("terminal_subscriber_worker", "async terminal progress worker").StartSimple(sub.runWorker)
	}

	return sub
}

// Name returns the canonical identifier for the terminal progress subscriber.
func (s *TerminalProgressSubscriber) Name() string {
	return SubscriberNameTerminalProgress
}

// IsClosed returns whether the subscriber has been terminated.
func (s *TerminalProgressSubscriber) IsClosed() bool {
	return s.closed.Load()
}

// IsAsync returns whether asynchronous dispatch mode is configured.
func (s *TerminalProgressSubscriber) IsAsync() bool {
	return s.cfg.BufferSize > 0
}

// DroppedCount returns the count of dropped events due to buffer saturation.
func (s *TerminalProgressSubscriber) DroppedCount() int64 {
	return s.droppedCount.Load()
}

// Notify formats and writes callback entries to the configured writer.
func (s *TerminalProgressSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	if s == nil {
		return ErrTerminalSubscriberNil
	}
	if s.closed.Load() {
		return ErrTerminalSubscriberClosed
	}
	if entry == nil || entry.Payload == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	if s.cfg.BufferSize > 0 {
		return s.enqueueAsync(ctx, entry)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeEntry(entry)
}

func (s *TerminalProgressSubscriber) enqueueAsync(ctx context.Context, entry *CallbackEntry) error {
	if s.cfg.DropOnFull {
		select {
		case s.eventCh <- entry:
			return nil
		default:
			s.droppedCount.Add(1)
			return nil
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case s.eventCh <- entry:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *TerminalProgressSubscriber) runWorker() {
	defer s.workerWg.Done()
	for {
		select {
		case <-s.ctx.Done():
			s.drainPendingEvents()
			return
		case entry, ok := <-s.eventCh:
			if !ok {
				return
			}
			s.processWorkerEntry(entry)
		}
	}
}

func (s *TerminalProgressSubscriber) drainPendingEvents() {
	for {
		select {
		case entry, ok := <-s.eventCh:
			if !ok {
				return
			}
			s.processWorkerEntry(entry)
		default:
			return
		}
	}
}

func (s *TerminalProgressSubscriber) processWorkerEntry(entry *CallbackEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeEntry(entry); err != nil {
		return
	}
}

func (s *TerminalProgressSubscriber) writeEntry(entry *CallbackEntry) error {
	line := s.formatEntry(entry)
	if _, err := io.WriteString(s.writer, line); err != nil {
		return errfmt.Newf("failed to write terminal progress update").Wrap(err)
	}
	s.lastHadLF = strings.HasSuffix(line, "\n")
	return nil
}

type flusherWithErr interface {
	Flush() error
}

type flusherWithoutErr interface {
	Flush()
}

type syncerWithErr interface {
	Sync() error
}

// Flush flushes any pending writes or underlying flusher/sync interfaces.
func (s *TerminalProgressSubscriber) Flush() error {
	if s == nil {
		return nil
	}
	return s.flushUnderLock()
}

func (s *TerminalProgressSubscriber) flushUnderLock() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if f, ok := s.writer.(flusherWithErr); ok {
		if err := f.Flush(); err != nil {
			return errfmt.Newf("failed to flush writer").Wrap(err)
		}
		return nil
	}
	if f, ok := s.writer.(flusherWithoutErr); ok {
		f.Flush()
		return nil
	}
	if syncable, ok := s.writer.(syncerWithErr); ok {
		if err := syncable.Sync(); err != nil {
			return errfmt.Newf("failed to sync writer").Wrap(err)
		}
		return nil
	}
	return nil
}

// Close gracefully terminates worker goroutines, flushes output, and cleans up resources.
func (s *TerminalProgressSubscriber) Close() error {
	if s == nil {
		return nil
	}
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	if s.cfg.BufferSize > 0 {
		if s.cancel != nil {
			s.cancel()
		}
		s.workerWg.Wait()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.AnsiEnabled && !s.lastHadLF {
		if _, err := io.WriteString(s.writer, "\n"); err != nil {
			return errfmt.Newf("failed to finalize terminal line on close").Wrap(err)
		}
		s.lastHadLF = true
	}
	return nil
}

type entryFields struct {
	jobID       string
	cbType      string
	status      string
	message     string
	step        int64
	total       int64
	hasSteps    bool
	percent     float64
	hasPercent  bool
	duration    float64
	hasDuration bool
	errMsg      string
	kind        string
	objectID    string
}

func extractEntryFields(entry *CallbackEntry) entryFields {
	payload := entry.Payload
	var fields entryFields

	fields.jobID = extractJobID(entry)
	fields.cbType = extractCallbackType(payload)
	fields.status = extractStatus(payload)
	fields.message = extractString(payload, "message", "msg", "detail")
	fields.errMsg = extractString(payload, "error", "err")
	fields.kind = extractString(payload, objects.FieldKeyKind, "kind")
	fields.objectID = extractString(payload, "object_id", "id")

	fields.percent, fields.hasPercent = extractPercent(payload)
	fields.step, fields.total, fields.hasSteps = extractSteps(payload)
	fields.duration, fields.hasDuration = extractDuration(payload)

	return fields
}

func extractJobID(entry *CallbackEntry) string {
	if entry.Payload != nil {
		for _, key := range []string{"job_id", "id", "object_id"} {
			if id := objects.GetString(entry.Payload, key); id != "" {
				return id
			}
		}
	}
	return entry.JobID
}

func extractCallbackType(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{objects.FieldKeyCallbackType, "callback_type", "type", "event"} {
		if t := objects.GetString(payload, key); t != "" {
			return t
		}
	}
	return ""
}

func extractStatus(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if s := objects.GetString(payload, "status"); s != "" {
		return s
	}
	if success, ok := payload["success"].(bool); ok {
		if success {
			return "completed"
		}
		return "failed"
	}
	if errStr := objects.GetString(payload, "error"); errStr != "" {
		return "failed"
	}
	return ""
}

func extractDuration(payload map[string]any) (float64, bool) {
	if payload == nil {
		return 0, false
	}
	if d, ok := payload["duration"]; ok {
		return extractFloat(d)
	}
	return 0, false
}

func extractString(payload map[string]any, keys ...string) string {
	if payload == nil {
		return ""
	}
	for _, k := range keys {
		if val := objects.GetString(payload, k); val != "" {
			return val
		}
	}
	return ""
}

func extractFloat(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	default:
		return 0, false
	}
}

func extractInt(val any) (int64, bool) {
	switch v := val.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	default:
		return 0, false
	}
}

func extractPercent(payload map[string]any) (float64, bool) {
	if val, ok := payload["percent"]; ok {
		if f, ok := extractFloat(val); ok {
			return f, true
		}
	}
	if val, ok := payload["progress"]; ok {
		if f, ok := extractFloat(val); ok {
			if f <= 1.0 && f >= 0.0 {
				return f * 100, true
			}
			return f, true
		}
	}
	return 0, false
}

func extractSteps(payload map[string]any) (int64, int64, bool) {
	stepVal, hasStep := payload["step"]
	totalVal, hasTotal := payload["total"]
	if !hasStep && !hasTotal {
		return 0, 0, false
	}
	step, okStep := extractInt(stepVal)
	total, okTotal := extractInt(totalVal)
	if !okStep && !okTotal {
		return 0, 0, false
	}
	return step, total, true
}

func stripAnsiColors(s string) string {
	return ansiColorRegex.ReplaceAllString(s, "")
}

func isWakerEvent(f entryFields) bool {
	return f.cbType == "waker" || f.cbType == "task_waker" || f.status == "waking" || f.status == "woken"
}

func isProgressEvent(f entryFields) bool {
	return f.cbType == "progress" || f.hasPercent || f.hasSteps
}

func isCompletionEvent(f entryFields) bool {
	switch strings.ToLower(f.cbType) {
	case "completion", "error", "done", "finished":
		return true
	}
	switch strings.ToLower(f.status) {
	case "completed", "success", "failed", "error":
		return true
	}
	return false
}

func appendStatusAndMessage(parts []string, status, message string) []string {
	if status != "" {
		parts = append(parts, fmt.Sprintf("Status: %s", status))
	}
	if message != "" {
		parts = append(parts, message)
	}
	return parts
}

func formatWakerEvent(f entryFields) (string, []string) {
	tag := "[WAKER]"
	parts := make([]string, 0, 4)
	if f.objectID != "" && f.kind != "" {
		parts = append(parts, fmt.Sprintf("%s %s", f.kind, f.objectID))
	} else if f.jobID != "" {
		parts = append(parts, fmt.Sprintf("Job: %s", f.jobID))
	}
	return tag, appendStatusAndMessage(parts, f.status, f.message)
}

func formatProgressEvent(f entryFields) (string, []string) {
	tag := "[PROGRESS]"
	parts := make([]string, 0, 5)
	if f.jobID != "" {
		parts = append(parts, fmt.Sprintf("Job: %s", f.jobID))
	}
	if f.hasPercent {
		parts = append(parts, fmt.Sprintf("%.1f%%", f.percent))
	}
	if f.hasSteps {
		parts = append(parts, fmt.Sprintf("step %d/%d", f.step, f.total))
	}
	if f.status != "" && f.status != "running" && f.status != "in_progress" {
		parts = append(parts, fmt.Sprintf("Status: %s", f.status))
	}
	if f.message != "" {
		parts = append(parts, f.message)
	}
	return tag, parts
}

func formatCompletionEvent(f entryFields) (string, []string) {
	isSuccess := f.status == "completed" || f.status == "success"
	if isSuccess {
		tag := "[COMPLETE]"
		parts := make([]string, 0, 4)
		if f.jobID != "" {
			parts = append(parts, fmt.Sprintf("Job: %s", f.jobID))
		}
		parts = append(parts, "Status: completed")
		if f.hasDuration {
			parts = append(parts, fmt.Sprintf("Duration: %.2fs", f.duration))
		}
		if f.message != "" {
			parts = append(parts, f.message)
		}
		return tag, parts
	}

	tag := "[FAILED]"
	parts := make([]string, 0, 4)
	if f.jobID != "" {
		parts = append(parts, fmt.Sprintf("Job: %s", f.jobID))
	}
	parts = append(parts, "Status: failed")
	if f.errMsg != "" {
		parts = append(parts, fmt.Sprintf("Error: %s", f.errMsg))
	}
	if f.message != "" && f.message != f.errMsg {
		parts = append(parts, f.message)
	}
	return tag, parts
}

func formatDefaultEvent(f entryFields) (string, []string) {
	tag := "[STATUS]"
	parts := make([]string, 0, 4)
	if f.jobID != "" {
		parts = append(parts, fmt.Sprintf("Job: %s", f.jobID))
	}
	return tag, appendStatusAndMessage(parts, f.status, f.message)
}

func composeStatusLine(fields entryFields) (string, []string) {
	switch {
	case isWakerEvent(fields):
		return formatWakerEvent(fields)
	case isProgressEvent(fields):
		return formatProgressEvent(fields)
	case isCompletionEvent(fields):
		return formatCompletionEvent(fields)
	default:
		return formatDefaultEvent(fields)
	}
}

func (s *TerminalProgressSubscriber) formatEntry(entry *CallbackEntry) string {
	fields := extractEntryFields(entry)
	tag, parts := composeStatusLine(fields)

	var text string
	if len(parts) > 0 {
		text = tag + " " + strings.Join(parts, " | ")
	} else {
		text = tag
	}

	if s.cfg.Prefix != "" {
		text = s.cfg.Prefix + " " + text
	}

	if s.cfg.StripColors {
		text = stripAnsiColors(text)
	}

	if s.cfg.AnsiEnabled {
		text = AnsiCarriageReturnClear + text
		if isCompletionEvent(fields) {
			text = text + "\n"
		}
		return text
	}

	return text + "\n"
}
