package logging

import (
	"sync"
)

// Wire keys aligned with pkg/objects FieldKey* for scheduler log fields — kept as literals here to
// avoid importing pkg/objects (import cycle: objects → … → logging).
const (
	logFieldKeyCommand  = "command"
	logFieldKeyJobType  = "job_type"
	logFieldKeyCategory = "category"
)

// Fluent starts a pooled fluent log entry builder for structured logs (POL-CODE-007).
// Prefer Fluent(...) over variadic Logger.Info/Warn/Error/Debug calls when attaching multiple
// fields so naming stays consistent and backing slices reuse pool storage.
//
// Canonical conventions (entry points, domains, interfaces): see docs/best-practices/coding/FLUENT_LOGGING.md.
func Fluent(logger Logger) *FluentRoot {
	root := &FluentRoot{logger: logger}
	root.entryPool.New = func() any {
		return &fluentEntry{}
	}
	return root
}

// FluentEvent starts the same pooled fluent builder as [Fluent], using the backing [Logger]
// from an [EventLogger]. Prefer this over EventLogger.LogInfo(..., logging.String(...), ...) at
// call sites that attach multiple structured fields to one message.
func FluentEvent(el *EventLogger) *FluentRoot {
	return Fluent(el.Logger())
}

// TryFluentEvent returns [FluentEvent] when v is a *EventLogger; otherwise ok is false.
// Use at boundaries that accept a CLI/logging facade interface implemented by *EventLogger.
func TryFluentEvent(v any) (root *FluentRoot, ok bool) {
	el, ok := v.(*EventLogger)
	if !ok {
		return nil, false
	}
	return FluentEvent(el), true
}

// FluentRoot creates level-specific log entries.
type FluentRoot struct {
	logger    Logger
	entryPool sync.Pool
}

// Debug starts a debug-level log entry.
func (r *FluentRoot) Debug(msg string) *fluentEntry {
	return r.newEntry("debug", msg, nil)
}

// Info starts an info-level log entry.
func (r *FluentRoot) Info(msg string) *fluentEntry {
	return r.newEntry("info", msg, nil)
}

// Warn starts a warn-level log entry.
func (r *FluentRoot) Warn(msg string) *fluentEntry {
	return r.newEntry("warn", msg, nil)
}

// Error starts an error-level log entry.
func (r *FluentRoot) Error(msg string, err error) *fluentEntry {
	return r.newEntry("error", msg, err)
}

// Trace starts a trace-like entry. The current logging interface has no Trace level,
// so this maps to debug output until a trace level is introduced in pkg/logging.
func (r *FluentRoot) Trace(msg string) *fluentEntry {
	return r.newEntry("trace", msg, nil)
}

func (r *FluentRoot) newEntry(level, msg string, err error) *fluentEntry {
	entry, _ := r.entryPool.Get().(*fluentEntry)
	if entry == nil {
		entry = &fluentEntry{}
	}
	entry.root = r
	entry.logger = r.logger
	entry.level = level
	entry.msg = msg
	entry.err = err
	entry.fields = entry.fields[:0]
	return entry
}

// fluentEntry accumulates structured fields before writing.
type fluentEntry struct {
	root   *FluentRoot
	logger Logger
	level  string
	msg    string
	err    error
	fields []Field
}

// FluentEntry is the exported fluent chain. STRUCTURE unexported the concrete type;
// failclosed validation-cache logging passes entries into helpers.
// TRACK: REDACTED — keep in sync with pkg/validation semaphore_full logs.
type FluentEntry = fluentEntry

func (e *fluentEntry) JobID(id string) *fluentEntry {
	e.fields = append(e.fields, JobIDField(id))
	return e
}

// MessageID logs a correlation message identifier (wire key "message_id").
func (e *fluentEntry) MessageID(id string) *fluentEntry {
	e.fields = append(e.fields, String("message_id", id))
	return e
}

// WorkerID logs a worker goroutine index (wire key "worker_id").
func (e *fluentEntry) WorkerID(workerID int) *fluentEntry {
	e.fields = append(e.fields, Int("worker_id", workerID))
	return e
}

func (e *fluentEntry) MetricKind(kind string) *fluentEntry {
	e.fields = append(e.fields, String("metric_kind", kind))
	return e
}

func (e *fluentEntry) Kind(kind string) *fluentEntry {
	e.fields = append(e.fields, KindField(kind))
	return e
}

// RuleName logs a transceiver routing rule name (wire key "rule").
func (e *fluentEntry) RuleName(name string) *fluentEntry {
	e.fields = append(e.fields, String("rule", name))
	return e
}

// RuleConfigName logs a routing rule name from YAML config during load (wire key "rule_name").
func (e *fluentEntry) RuleConfigName(name string) *fluentEntry {
	e.fields = append(e.fields, String("rule_name", name))
	return e
}

// RulesDir logs the routing rules directory path (wire key "rules_dir").
func (e *fluentEntry) RulesDir(dir string) *fluentEntry {
	e.fields = append(e.fields, String("rules_dir", dir))
	return e
}

// RuleCount logs how many routing rules were loaded or evaluated (wire key "rule_count").
func (e *fluentEntry) RuleCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("rule_count", n))
	return e
}

// ValidationRowField logs a routing rule validation field path (wire key "field").
func (e *fluentEntry) ValidationRowField(path string) *fluentEntry {
	e.fields = append(e.fields, String("field", path))
	return e
}

// ValidationRowMessage logs a routing rule validation detail (wire key "message").
func (e *fluentEntry) ValidationRowMessage(text string) *fluentEntry {
	e.fields = append(e.fields, String("message", text))
	return e
}

// LoaderPhase logs which setup or check loader phase was involved (wire key "loader").
func (e *fluentEntry) LoaderPhase(name string) *fluentEntry {
	e.fields = append(e.fields, String("loader", name))
	return e
}

// KindsCount logs a kind cardinality (wire key "kinds").
func (e *fluentEntry) KindsCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("kinds", n))
	return e
}

func (e *fluentEntry) File(name string) *fluentEntry {
	e.fields = append(e.fields, FileField(name))
	return e
}

// Path logs a filesystem path (wire key "path").
func (e *fluentEntry) Path(p string) *fluentEntry {
	e.fields = append(e.fields, PathField(p))
	return e
}

// URL logs a URL or HTTP endpoint string (wire key "url").
func (e *fluentEntry) URL(u string) *fluentEntry {
	e.fields = append(e.fields, String("url", u))
	return e
}

// Protocol logs an adapter or action protocol name (wire key "protocol").
func (e *fluentEntry) Protocol(p string) *fluentEntry {
	e.fields = append(e.fields, ProtocolField(p))
	return e
}

// ObjectID logs an object identifier (wire key "id").
func (e *fluentEntry) ObjectID(id string) *fluentEntry {
	e.fields = append(e.fields, IDField(id))
	return e
}

func (e *fluentEntry) BatchID(id string) *fluentEntry {
	e.fields = append(e.fields, String("batch_id", id))
	return e
}

func (e *fluentEntry) MetricID(id string) *fluentEntry {
	e.fields = append(e.fields, String("metric_id", id))
	return e
}

func (e *fluentEntry) ProjectRoot(root string) *fluentEntry {
	e.fields = append(e.fields, ProjectRootField(root))
	return e
}

func (e *fluentEntry) ScheduleID(id string) *fluentEntry {
	e.fields = append(e.fields, String("schedule_id", id))
	return e
}

func (e *fluentEntry) Target(target string) *fluentEntry {
	e.fields = append(e.fields, String("target", target))
	return e
}

// SessionID logs canonical CVS / session correlation (convergence_session_tick, rollup).
func (e *fluentEntry) SessionID(id string) *fluentEntry {
	e.fields = append(e.fields, SessionIDField(id))
	return e
}

// EngineID logs a swarm engine instance id (wire key "engineID").
func (e *fluentEntry) EngineID(id string) *fluentEntry {
	e.fields = append(e.fields, EngineIDField(id))
	return e
}

// Watermark logs health.jsonl / measurement watermark RFC3339 fields.
func (e *fluentEntry) Watermark(w string) *fluentEntry {
	e.fields = append(e.fields, String("watermark", w))
	return e
}

// SessionStatus logs convergence_session.status (string wire).
func (e *fluentEntry) SessionStatus(s string) *fluentEntry {
	e.fields = append(e.fields, String("session_status", s))
	return e
}

// Command logs command (aligned with objects.FieldKeyCommand; wire key "command").
func (e *fluentEntry) Command(cmd string) *fluentEntry {
	e.fields = append(e.fields, CommandField(cmd))
	return e
}

// WorkingDirectory logs the subprocess working directory.
func (e *fluentEntry) WorkingDirectory(dir string) *fluentEntry {
	e.fields = append(e.fields, String("working_directory", dir))
	return e
}

// SchedulerJobType logs [KeyJobType] without colliding with the scheduled job struct field name.
func (e *fluentEntry) SchedulerJobType(jobType string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyJobType, jobType))
	return e
}

// JobCategory logs [KeyCategory].
func (e *fluentEntry) JobCategory(category string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyCategory, category))
	return e
}

// TestName logs zqk test -run scope / bundle test name fragments.
func (e *fluentEntry) TestName(name string) *fluentEntry {
	e.fields = append(e.fields, String("test_name", name))
	return e
}

// PackageName logs Go package path for test timing / bundles.
func (e *fluentEntry) PackageName(pkg string) *fluentEntry {
	e.fields = append(e.fields, String("package", pkg))
	return e
}

// ExpectedTimeout logs resolved duration string for dynamic test timeouts.
func (e *fluentEntry) ExpectedTimeout(s string) *fluentEntry {
	e.fields = append(e.fields, String("expected_timeout", s))
	return e
}

// ScriptPath logs resolved on-disk script path (syntax check, orchestrate).
func (e *fluentEntry) ScriptPath(p string) *fluentEntry {
	e.fields = append(e.fields, String("script_path", p))
	return e
}

// StderrSnippet logs stderr preview (shell -n, subprocess).
func (e *fluentEntry) StderrSnippet(s string) *fluentEntry {
	e.fields = append(e.fields, String("stderr", s))
	return e
}

// StdoutSnippet logs stdout preview (subprocess output).
func (e *fluentEntry) StdoutSnippet(s string) *fluentEntry {
	e.fields = append(e.fields, String("stdout", s))
	return e
}

func (e *fluentEntry) EventType(v string) *fluentEntry {
	e.fields = append(e.fields, String("event_type", v))
	return e
}

func (e *fluentEntry) LogLevel(v string) *fluentEntry {
	e.fields = append(e.fields, String("log_level", v))
	return e
}

func (e *fluentEntry) MetricsFound(n int) *fluentEntry {
	e.fields = append(e.fields, Int("metrics_found", n))
	return e
}

func (e *fluentEntry) DeletedCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("deleted_count", n))
	return e
}

func (e *fluentEntry) FailedCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("failed_count", n))
	return e
}

// ProcessedCount logs items processed in a worker or batch (wire key "processed_count").
func (e *fluentEntry) ProcessedCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("processed_count", n))
	return e
}

// ExpectedURLCount logs how many endpoints should receive a message (wire key "expected_urls").
func (e *fluentEntry) ExpectedURLCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("expected_urls", n))
	return e
}

// VerifiedURLCount logs verified delivery endpoint count (wire key "verified").
func (e *fluentEntry) VerifiedURLCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("verified", n))
	return e
}

// FailedURLCount logs failed delivery endpoint count (wire key "failed").
func (e *fluentEntry) FailedURLCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("failed", n))
	return e
}

func (e *fluentEntry) Count(n int) *fluentEntry {
	e.fields = append(e.fields, CountField(n))
	return e
}

func (e *fluentEntry) Deleted(n int) *fluentEntry {
	e.fields = append(e.fields, Int("deleted", n))
	return e
}

func (e *fluentEntry) MaxCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("max_count", n))
	return e
}

func (e *fluentEntry) ToDelete(n int) *fluentEntry {
	e.fields = append(e.fields, Int("to_delete", n))
	return e
}

func (e *fluentEntry) BatchSize(n int) *fluentEntry {
	e.fields = append(e.fields, Int("batch_size", n))
	return e
}

func (e *fluentEntry) MaxBatches(n int) *fluentEntry {
	e.fields = append(e.fields, Int("max_batches", n))
	return e
}

func (e *fluentEntry) CandidateCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("candidate_count", n))
	return e
}

func (e *fluentEntry) FilesProcessed(n int) *fluentEntry {
	e.fields = append(e.fields, Int("files_processed", n))
	return e
}

func (e *fluentEntry) MetricsCaptured(n int) *fluentEntry {
	e.fields = append(e.fields, Int("metrics_captured", n))
	return e
}

func (e *fluentEntry) FilesDeleted(n int) *fluentEntry {
	e.fields = append(e.fields, Int("files_deleted", n))
	return e
}

func (e *fluentEntry) ErrorCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("error_count", n))
	return e
}

func (e *fluentEntry) ObjectCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("object_count", n))
	return e
}

func (e *fluentEntry) PolicyCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("policy_count", n))
	return e
}

func (e *fluentEntry) PoliciesProcessed(n int) *fluentEntry {
	e.fields = append(e.fields, Int("policies_processed", n))
	return e
}

func (e *fluentEntry) PoliciesRefreshed(n int) *fluentEntry {
	e.fields = append(e.fields, Int("policies_refreshed", n))
	return e
}

func (e *fluentEntry) MessagesSent(n int) *fluentEntry {
	e.fields = append(e.fields, Int("messages_sent", n))
	return e
}

func (e *fluentEntry) MessagesSucceeded(n int) *fluentEntry {
	e.fields = append(e.fields, Int("messages_succeeded", n))
	return e
}

func (e *fluentEntry) MessagesFailed(n int) *fluentEntry {
	e.fields = append(e.fields, Int("messages_failed", n))
	return e
}

func (e *fluentEntry) MessageIndex(n int) *fluentEntry {
	e.fields = append(e.fields, Int("message_index", n))
	return e
}

// Profile logs CLI/profile name (wire key "profile").
func (e *fluentEntry) Profile(name string) *fluentEntry {
	e.fields = append(e.fields, String("profile", name))
	return e
}

// ProfileName logs a named profile identifier (wire key "profile_name").
func (e *fluentEntry) ProfileName(name string) *fluentEntry {
	e.fields = append(e.fields, String("profile_name", name))
	return e
}

// Index logs a zero-based or display index (wire key "index").
func (e *fluentEntry) Index(n int) *fluentEntry {
	e.fields = append(e.fields, Int("index", n))
	return e
}

// Title logs a display title (wire key "title").
func (e *fluentEntry) Title(s string) *fluentEntry {
	e.fields = append(e.fields, String("title", s))
	return e
}

// OutputDir logs an output directory path (wire key "output_dir").
func (e *fluentEntry) OutputDir(s string) *fluentEntry {
	e.fields = append(e.fields, String("output_dir", s))
	return e
}

// FilePath logs a full file path (wire key "file_path"); use [fluentEntry.File] for a short "file" name.
func (e *fluentEntry) FilePath(s string) *fluentEntry {
	e.fields = append(e.fields, String("file_path", s))
	return e
}

// KindDir logs a per-kind output directory (wire key "kind_dir").
func (e *fluentEntry) KindDir(s string) *fluentEntry {
	e.fields = append(e.fields, String("kind_dir", s))
	return e
}

// JobIdx logs a job index within a worker pool (wire key "job_idx").
func (e *fluentEntry) JobIdx(n int) *fluentEntry {
	e.fields = append(e.fields, Int("job_idx", n))
	return e
}

// Entries logs a count of entries (e.g. cache size as a count; wire key "entries").
func (e *fluentEntry) Entries(n int) *fluentEntry {
	e.fields = append(e.fields, Int("entries", n))
	return e
}

// CacheSize logs cache_size (wire key "cache_size").
func (e *fluentEntry) CacheSize(n int) *fluentEntry {
	e.fields = append(e.fields, Int("cache_size", n))
	return e
}

// Invalidated logs a count of invalidated records (wire key "invalidated").
func (e *fluentEntry) Invalidated(n int) *fluentEntry {
	e.fields = append(e.fields, Int("invalidated", n))
	return e
}

// Dir logs a directory path when "dir" is the stable wire key (distinct from [fluentEntry.Path]).
func (e *fluentEntry) Dir(s string) *fluentEntry {
	e.fields = append(e.fields, String("dir", s))
	return e
}

// PanicSummary logs a panic value string (wire key "panic").
func (e *fluentEntry) PanicSummary(s string) *fluentEntry {
	e.fields = append(e.fields, String("panic", s))
	return e
}

// Errors logs an error count (wire key "errors").
func (e *fluentEntry) Errors(n int) *fluentEntry {
	e.fields = append(e.fields, Int("errors", n))
	return e
}

// Written logs objects or bytes written count (wire key "written").
func (e *fluentEntry) Written(n int) *fluentEntry {
	e.fields = append(e.fields, Int("written", n))
	return e
}

// Strategy logs a strategy or handler name (wire key "strategy").
func (e *fluentEntry) Strategy(s string) *fluentEntry {
	e.fields = append(e.fields, String("strategy", s))
	return e
}

// HashFile logs a hash-side filename (wire key "hash_file").
func (e *fluentEntry) HashFile(s string) *fluentEntry {
	e.fields = append(e.fields, String("hash_file", s))
	return e
}

// TemplatePath logs a template file path (wire key "template_path").
func (e *fluentEntry) TemplatePath(s string) *fluentEntry {
	e.fields = append(e.fields, String("template_path", s))
	return e
}

// Message logs a short human/detail message (wire key "msg").
func (e *fluentEntry) Message(s string) *fluentEntry {
	e.fields = append(e.fields, String("msg", s))
	return e
}

// TimestampRFC3339Nano logs an RFC3339Nano timestamp string (wire key "timestamp").
func (e *fluentEntry) TimestampRFC3339Nano(s string) *fluentEntry {
	e.fields = append(e.fields, String("timestamp", s))
	return e
}

// Bytes logs a byte length (wire key "bytes").
func (e *fluentEntry) Bytes(n int) *fluentEntry {
	e.fields = append(e.fields, Int("bytes", n))
	return e
}

// ExecPath logs a subprocess executable path (wire key "exec_path").
func (e *fluentEntry) ExecPath(p string) *fluentEntry {
	e.fields = append(e.fields, String("exec_path", p))
	return e
}

// ExecArgs logs joined argv (wire key "args").
func (e *fluentEntry) ExecArgs(joined string) *fluentEntry {
	e.fields = append(e.fields, String("args", joined))
	return e
}

// ElapsedString logs a human duration (e.g. [time.Duration.String]) (wire key "duration").
func (e *fluentEntry) ElapsedString(s string) *fluentEntry {
	e.fields = append(e.fields, ElapsedStringField(s))
	return e
}

// ErrorSummary logs a free-form error string when [WithError] is not used (wire key "error").
func (e *fluentEntry) ErrorSummary(s string) *fluentEntry {
	e.fields = append(e.fields, String("error", s))
	return e
}

// DedupeKey logs a deduplication key (wire key "dedupe_key").
func (e *fluentEntry) DedupeKey(s string) *fluentEntry {
	e.fields = append(e.fields, String("dedupe_key", s))
	return e
}

// AckedBy logs an ack actor (wire key "acked_by").
func (e *fluentEntry) AckedBy(s string) *fluentEntry {
	e.fields = append(e.fields, String("acked_by", s))
	return e
}

// SkippedCount logs skipped items (wire key "skipped_count").
func (e *fluentEntry) SkippedCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("skipped_count", n))
	return e
}

// WrittenCount logs a written-items count distinct from [fluentEntry.Written] (wire key "written_count").
func (e *fluentEntry) WrittenCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("written_count", n))
	return e
}

// EntryCount logs collection/cache entry counts (wire key "entry_count").
func (e *fluentEntry) EntryCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("entry_count", n))
	return e
}

// Total logs a sum or cardinality when "total" is the stable wire key (distinct from [fluentEntry.Count]).
func (e *fluentEntry) Total(n int) *fluentEntry {
	e.fields = append(e.fields, Int("total", n))
	return e
}

// QueueSize logs validator or work-queue depth (wire key "queue_size").
func (e *fluentEntry) QueueSize(n int) *fluentEntry {
	e.fields = append(e.fields, Int("queue_size", n))
	return e
}

// QueueName logs a logical queue registration name (wire key "queue_name").
func (e *fluentEntry) QueueName(name string) *fluentEntry {
	e.fields = append(e.fields, String("queue_name", name))
	return e
}

// MaxWorkers logs configured worker pool ceiling (wire key "max_workers").
func (e *fluentEntry) MaxWorkers(n int) *fluentEntry {
	e.fields = append(e.fields, Int("max_workers", n))
	return e
}

// Enqueued logs enqueue counts (wire key "enqueued").
func (e *fluentEntry) Enqueued(n int) *fluentEntry {
	e.fields = append(e.fields, Int("enqueued", n))
	return e
}

// Stale logs stale queue/cache counts (wire key "stale").
func (e *fluentEntry) Stale(n int) *fluentEntry {
	e.fields = append(e.fields, Int("stale", n))
	return e
}

// WithIssues logs items with issues (wire key "with_issues").
func (e *fluentEntry) WithIssues(n int) *fluentEntry {
	e.fields = append(e.fields, Int("with_issues", n))
	return e
}

// KindsWithObjects logs distinct kinds that had objects (wire key "kinds_with_objects").
func (e *fluentEntry) KindsWithObjects(n int) *fluentEntry {
	e.fields = append(e.fields, Int("kinds_with_objects", n))
	return e
}

// EntriesRemoved logs stale cleanup counts (wire key "entries_removed").
func (e *fluentEntry) EntriesRemoved(n int) *fluentEntry {
	e.fields = append(e.fields, Int("entries_removed", n))
	return e
}

// EntriesBeforeCleanup logs size before stale cleanup (wire key "entries_before_cleanup").
func (e *fluentEntry) EntriesBeforeCleanup(n int) *fluentEntry {
	e.fields = append(e.fields, Int("entries_before_cleanup", n))
	return e
}

// BlockingPublic logs Tier-1 public blocking counts (wire key "public").
func (e *fluentEntry) BlockingPublic(n int) *fluentEntry {
	e.fields = append(e.fields, Int("public", n))
	return e
}

// BlockingInternal logs Tier-1 internal blocking counts (wire key "internal").
func (e *fluentEntry) BlockingInternal(n int) *fluentEntry {
	e.fields = append(e.fields, Int("internal", n))
	return e
}

// EventsCreated logs created row/event counts (wire key "created").
func (e *fluentEntry) EventsCreated(n int) *fluentEntry {
	e.fields = append(e.fields, Int("created", n))
	return e
}

// EventsUpdated logs updated row/event counts (wire key "updated").
func (e *fluentEntry) EventsUpdated(n int) *fluentEntry {
	e.fields = append(e.fields, Int("updated", n))
	return e
}

// Loaded logs loaded item counts during bulk load (wire key "loaded").
func (e *fluentEntry) Loaded(n int) *fluentEntry {
	e.fields = append(e.fields, Int("loaded", n))
	return e
}

// TotalCheckResults logs expanded check-result cardinality (wire key "total_check_results").
func (e *fluentEntry) TotalCheckResults(n int) *fluentEntry {
	e.fields = append(e.fields, Int("total_check_results", n))
	return e
}

// Recovered logs recovered-item counts (wire key "recovered").
func (e *fluentEntry) Recovered(n int) *fluentEntry {
	e.fields = append(e.fields, Int("recovered", n))
	return e
}

// FailedOps logs failed-operation counts where the wire key is "failed" (distinct from [fluentEntry.FailedCount] "failed_count").
func (e *fluentEntry) FailedOps(n int) *fluentEntry {
	e.fields = append(e.fields, Int("failed", n))
	return e
}

// SkippedOps logs skipped-operation counts (wire key "skipped").
func (e *fluentEntry) SkippedOps(n int) *fluentEntry {
	e.fields = append(e.fields, Int("skipped", n))
	return e
}

// BuildTime logs an RFC3339 build timestamp string (wire key "build_time").
func (e *fluentEntry) BuildTime(s string) *fluentEntry {
	e.fields = append(e.fields, String("build_time", s))
	return e
}

// TotalTasks logs discovery/validation task cardinality (wire key "total_tasks").
func (e *fluentEntry) TotalTasks(n int) *fluentEntry {
	e.fields = append(e.fields, Int("total_tasks", n))
	return e
}

// EnqueuedObjectIDs logs tracked enqueued-ID count diagnostics (wire key "enqueued_object_ids").
func (e *fluentEntry) EnqueuedObjectIDs(n int) *fluentEntry {
	e.fields = append(e.fields, Int("enqueued_object_ids", n))
	return e
}

// TasksActuallyEnqueued logs tasks submitted to the validator batch (wire key "tasks_actually_enqueued").
func (e *fluentEntry) TasksActuallyEnqueued(n int) *fluentEntry {
	e.fields = append(e.fields, Int("tasks_actually_enqueued", n))
	return e
}

// EnqueuedIDs logs summary enqueued-ID count (wire key "enqueued_ids").
func (e *fluentEntry) EnqueuedIDs(n int) *fluentEntry {
	e.fields = append(e.fields, Int("enqueued_ids", n))
	return e
}

// TasksPassedToValidator logs tasks handed off to the validator (wire key "tasks_passed_to_validator").
func (e *fluentEntry) TasksPassedToValidator(n int) *fluentEntry {
	e.fields = append(e.fields, Int("tasks_passed_to_validator", n))
	return e
}

// CacheHits logs object-ID cache hits during enqueue (wire key "cache_hits").
func (e *fluentEntry) CacheHits(n int) *fluentEntry {
	e.fields = append(e.fields, Int("cache_hits", n))
	return e
}

// CacheMisses logs object-ID cache misses during enqueue (wire key "cache_misses").
func (e *fluentEntry) CacheMisses(n int) *fluentEntry {
	e.fields = append(e.fields, Int("cache_misses", n))
	return e
}

// ProgressCount logs validator progress counts (wire key "progress_count").
func (e *fluentEntry) ProgressCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("progress_count", n))
	return e
}

// CachedCount logs cached-state counts (wire key "cached_count").
func (e *fluentEntry) CachedCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("cached_count", n))
	return e
}

// TotalAccounted logs combined accounted progress (wire key "total_accounted").
func (e *fluentEntry) TotalAccounted(n int) *fluentEntry {
	e.fields = append(e.fields, Int("total_accounted", n))
	return e
}

// TotalEnqueued logs enqueued cardinality for validation runs (wire key "total_enqueued").
func (e *fluentEntry) TotalEnqueued(n int) *fluentEntry {
	e.fields = append(e.fields, Int("total_enqueued", n))
	return e
}

// MissingCount logs missing-object counts (wire key "missing_count").
func (e *fluentEntry) MissingCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("missing_count", n))
	return e
}

// Tolerance logs completion tolerance thresholds (wire key "tolerance").
func (e *fluentEntry) Tolerance(n int) *fluentEntry {
	e.fields = append(e.fields, Int("tolerance", n))
	return e
}

// ProgressReceived logs progress channel receipt counts (wire key "progress_received").
func (e *fluentEntry) ProgressReceived(n int) *fluentEntry {
	e.fields = append(e.fields, Int("progress_received", n))
	return e
}

// AutoFixableCount logs auto-fix batch sizing (wire key "auto_fixable_count").
func (e *fluentEntry) AutoFixableCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("auto_fixable_count", n))
	return e
}

// Accounted logs accounted counts where the wire key is "accounted".
func (e *fluentEntry) Accounted(n int) *fluentEntry {
	e.fields = append(e.fields, Int("accounted", n))
	return e
}

// Completed logs completed counts (wire key "completed").
func (e *fluentEntry) Completed(n int) *fluentEntry {
	e.fields = append(e.fields, Int("completed", n))
	return e
}

// ActiveWorkers logs active worker goroutines (wire key "active_workers").
func (e *fluentEntry) ActiveWorkers(n int) *fluentEntry {
	e.fields = append(e.fields, Int("active_workers", n))
	return e
}

// GoroutineID logs goroutine correlation id from diagnostics (wire key "goroutine_id").
func (e *fluentEntry) GoroutineID(id int) *fluentEntry {
	e.fields = append(e.fields, Int("goroutine_id", id))
	return e
}

// ActiveGoroutines logs active goroutine instrumentation counts (wire key "active_goroutines").
func (e *fluentEntry) ActiveGoroutines(n int) *fluentEntry {
	e.fields = append(e.fields, Int("active_goroutines", n))
	return e
}

// SemaphoreCapacity logs semaphore slot capacity (wire key "semaphore_capacity").
func (e *fluentEntry) SemaphoreCapacity(n int) *fluentEntry {
	e.fields = append(e.fields, Int("semaphore_capacity", n))
	return e
}

// ActiveWorkersRemainingAtTimeout logs workers still live when shutdown timed out (wire key "active_workers_at_timeout").
func (e *fluentEntry) ActiveWorkersRemainingAtTimeout(n int) *fluentEntry {
	e.fields = append(e.fields, Int("active_workers_at_timeout", n))
	return e
}

// WorkerStateLabel logs worker state machine labels (wire key "state").
func (e *fluentEntry) WorkerStateLabel(s string) *fluentEntry {
	e.fields = append(e.fields, String("state", s))
	return e
}

// DiagnosticDetail logs long-form diagnostic hints on warnings (wire key "diagnostic").
func (e *fluentEntry) DiagnosticDetail(s string) *fluentEntry {
	e.fields = append(e.fields, String("diagnostic", s))
	return e
}

// WaitTime logs a measured or configured wait duration string (wire key "wait_time").
func (e *fluentEntry) WaitTime(s string) *fluentEntry {
	e.fields = append(e.fields, String("wait_time", s))
	return e
}

// TotalDrained logs items drained from the validation progress channel (wire key "total_drained").
func (e *fluentEntry) TotalDrained(n int) *fluentEntry {
	e.fields = append(e.fields, Int("total_drained", n))
	return e
}

// ValidationDrainCount logs per-update drain counter (wire key "drain_count").
func (e *fluentEntry) ValidationDrainCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("drain_count", n))
	return e
}

// ValidationProgressStatus logs validation progress phase string (wire key "status").
func (e *fluentEntry) ValidationProgressStatus(s string) *fluentEntry {
	e.fields = append(e.fields, String("status", s))
	return e
}

// PercentComplete logs a formatted percent string (wire key "percent_complete").
func (e *fluentEntry) PercentComplete(s string) *fluentEntry {
	e.fields = append(e.fields, String("percent_complete", s))
	return e
}

// TimeSinceProgress logs a duration string since last progress (wire key "time_since_progress").
func (e *fluentEntry) TimeSinceProgress(s string) *fluentEntry {
	e.fields = append(e.fields, String("time_since_progress", s))
	return e
}

// QueueEmptyDuration logs how long the queue stayed empty (wire key "queue_empty_duration").
func (e *fluentEntry) QueueEmptyDuration(s string) *fluentEntry {
	e.fields = append(e.fields, String("queue_empty_duration", s))
	return e
}

// EmptyDuration logs empty-queue duration strings (wire key "empty_duration").
func (e *fluentEntry) EmptyDuration(s string) *fluentEntry {
	e.fields = append(e.fields, String("empty_duration", s))
	return e
}

// StuckDuration logs stuck-detection duration strings (wire key "stuck_duration").
func (e *fluentEntry) StuckDuration(s string) *fluentEntry {
	e.fields = append(e.fields, String("stuck_duration", s))
	return e
}

// AccountedPercent logs formatted accounted percent (wire key "accounted_percent").
func (e *fluentEntry) AccountedPercent(s string) *fluentEntry {
	e.fields = append(e.fields, String("accounted_percent", s))
	return e
}

// ValidatorCompleted logs validator-side completed totals (wire key "validator_completed").
func (e *fluentEntry) ValidatorCompleted(n int) *fluentEntry {
	e.fields = append(e.fields, Int("validator_completed", n))
	return e
}

// ProgressGap logs gaps between validator completion and progress receipts (wire key "progress_gap").
func (e *fluentEntry) ProgressGap(n int) *fluentEntry {
	e.fields = append(e.fields, Int("progress_gap", n))
	return e
}

// MaxAllowedGap logs allowed progress gaps (wire key "max_allowed_gap").
func (e *fluentEntry) MaxAllowedGap(n int) *fluentEntry {
	e.fields = append(e.fields, Int("max_allowed_gap", n))
	return e
}

// ValidatorQueueSize logs internal validator queue depth (wire key "validator_queue_size").
func (e *fluentEntry) ValidatorQueueSize(n int) *fluentEntry {
	e.fields = append(e.fields, Int("validator_queue_size", n))
	return e
}

// PendingInQueue logs a summary of pending object IDs (wire key "pending_in_queue").
func (e *fluentEntry) PendingInQueue(s string) *fluentEntry {
	e.fields = append(e.fields, String("pending_in_queue", s))
	return e
}

// DiagNote logs a diagnostic note string (wire key "note").
func (e *fluentEntry) DiagNote(s string) *fluentEntry {
	e.fields = append(e.fields, String("note", s))
	return e
}

// ExpectedTotal logs expected totals for mismatch diagnostics (wire key "expected_total").
func (e *fluentEntry) ExpectedTotal(n int) *fluentEntry {
	e.fields = append(e.fields, Int("expected_total", n))
	return e
}

// MissingPercent logs a formatted percent of missing cache states (wire key "missing_percent").
func (e *fluentEntry) MissingPercent(s string) *fluentEntry {
	e.fields = append(e.fields, String("missing_percent", s))
	return e
}

// FilterExpr logs a CLI/API filter expression (wire key "filter").
func (e *fluentEntry) FilterExpr(s string) *fluentEntry {
	e.fields = append(e.fields, String("filter", s))
	return e
}

// KeyID logs keystore/crypto key identifiers (wire key "key_id").
func (e *fluentEntry) KeyID(id string) *fluentEntry {
	e.fields = append(e.fields, String("key_id", id))
	return e
}

// NotifyType logs notification routing/presentation labels (wire key "notification_type").
func (e *fluentEntry) NotifyType(s string) *fluentEntry {
	e.fields = append(e.fields, String("notification_type", s))
	return e
}

// Capped logs capped-size limits (e.g. retention caps; wire key "capped").
func (e *fluentEntry) Capped(n int) *fluentEntry {
	e.fields = append(e.fields, Int("capped", n))
	return e
}

// KindsSummary logs serialized kind lists for diagnostics (wire key "kinds").
func (e *fluentEntry) KindsSummary(s string) *fluentEntry {
	e.fields = append(e.fields, String("kinds", s))
	return e
}

// EmitComponent logs the emitting subsystem name (wire key "component").
func (e *fluentEntry) EmitComponent(name string) *fluentEntry {
	e.fields = append(e.fields, String("component", name))
	return e
}

// TraceMarker logs trace=true when verbose trace correlation is enabled (wire key "trace").
func (e *fluentEntry) TraceMarker() *fluentEntry {
	e.fields = append(e.fields, String("trace", "true"))
	return e
}

// RPCMethod logs a JSON-RPC method name (wire key "method").
func (e *fluentEntry) RPCMethod(name string) *fluentEntry {
	e.fields = append(e.fields, MethodField(name))
	return e
}

// RequestID logs a JSON-RPC request id when present (wire key "request_id").
func (e *fluentEntry) RequestID(id string) *fluentEntry {
	e.fields = append(e.fields, String("request_id", id))
	return e
}

// RPCMessageType logs a JSON-RPC envelope type (wire key "type"), e.g. "notification".
func (e *fluentEntry) RPCMessageType(t string) *fluentEntry {
	e.fields = append(e.fields, String("type", t))
	return e
}

// ParamsText logs JSON-RPC params when serialized as text (wire key "params").
func (e *fluentEntry) ParamsText(serialized string) *fluentEntry {
	e.fields = append(e.fields, String("params", serialized))
	return e
}

// ParamsData logs JSON-RPC params as structured data (wire key "params").
func (e *fluentEntry) ParamsData(v any) *fluentEntry {
	e.fields = append(e.fields, Field{Key: "params", Value: v})
	return e
}

// RPCResult logs a JSON-RPC success result payload (wire key "result").
func (e *fluentEntry) RPCResult(v any) *fluentEntry {
	e.fields = append(e.fields, Field{Key: "result", Value: v})
	return e
}

// TraceFile logs MCP trace file paths (wire key "trace_file").
func (e *fluentEntry) TraceFile(path string) *fluentEntry {
	e.fields = append(e.fields, String("trace_file", path))
	return e
}

// MCPLogLevel logs MCP channel log level labels (wire key "mcp_log_level").
func (e *fluentEntry) MCPLogLevel(level string) *fluentEntry {
	e.fields = append(e.fields, String("mcp_log_level", level))
	return e
}

// Reason logs a short machine-readable reason tag (wire key "reason").
func (e *fluentEntry) Reason(s string) *fluentEntry {
	e.fields = append(e.fields, String("reason", s))
	return e
}

// Op logs a short operation label (wire key "op").
func (e *fluentEntry) Op(name string) *fluentEntry {
	e.fields = append(e.fields, String("op", name))
	return e
}

// OpDetail logs supplementary operation detail (wire key "detail").
func (e *fluentEntry) OpDetail(s string) *fluentEntry {
	e.fields = append(e.fields, String("detail", s))
	return e
}

// StorageProfileName logs storage profile identifier (wire key "storage_profile").
func (e *fluentEntry) StorageProfileName(name string) *fluentEntry {
	e.fields = append(e.fields, String("storage_profile", name))
	return e
}

// Success logs a boolean outcome (wire key "success").
func (e *fluentEntry) Success(ok bool) *fluentEntry {
	e.fields = append(e.fields, Bool("success", ok))
	return e
}

// Enabled logs a boolean enabled flag (wire key "enabled").
func (e *fluentEntry) Enabled(on bool) *fluentEntry {
	e.fields = append(e.fields, Bool("enabled", on))
	return e
}

// Timeout logs a human-readable timeout duration (wire key "timeout").
func (e *fluentEntry) Timeout(s string) *fluentEntry {
	e.fields = append(e.fields, String("timeout", s))
	return e
}

// HTTPStatusCode logs an HTTP response status (wire key "status_code").
func (e *fluentEntry) HTTPStatusCode(code int) *fluentEntry {
	e.fields = append(e.fields, Int("status_code", code))
	return e
}

// IdleDuration logs human-readable idle wait before shutdown or idle diagnostics (wire key "idle_duration").
func (e *fluentEntry) IdleDuration(s string) *fluentEntry {
	e.fields = append(e.fields, String("idle_duration", s))
	return e
}

// RetryAttempt logs a 1-based retry attempt number (wire key "attempt").
func (e *fluentEntry) RetryAttempt(n int) *fluentEntry {
	e.fields = append(e.fields, Int("attempt", n))
	return e
}

// RetryMaxAttempts logs configured retry ceiling (wire key "max_attempts").
func (e *fluentEntry) RetryMaxAttempts(n int) *fluentEntry {
	e.fields = append(e.fields, Int("max_attempts", n))
	return e
}

// RetryDelay logs backoff delay before the next attempt (wire key "delay").
func (e *fluentEntry) RetryDelay(s string) *fluentEntry {
	e.fields = append(e.fields, String("delay", s))
	return e
}

// BackoffStrategy logs retry backoff strategy label (wire key "backoff_strategy").
func (e *fluentEntry) BackoffStrategy(s string) *fluentEntry {
	e.fields = append(e.fields, String("backoff_strategy", s))
	return e
}

// MaxBackoffMs logs maximum backoff in milliseconds (wire key "max_backoff_ms").
func (e *fluentEntry) MaxBackoffMs(ms int) *fluentEntry {
	e.fields = append(e.fields, Int("max_backoff_ms", ms))
	return e
}

// DefaultTimeoutMs logs default action timeout in milliseconds (wire key "default_timeout_ms").
func (e *fluentEntry) DefaultTimeoutMs(ms int) *fluentEntry {
	e.fields = append(e.fields, Int("default_timeout_ms", ms))
	return e
}

// ValidationErrorCount logs routing rule validation error cardinality (wire key "validation_errors").
func (e *fluentEntry) ValidationErrorCount(n int) *fluentEntry {
	e.fields = append(e.fields, Int("validation_errors", n))
	return e
}

// WorkerStopTimeout logs configured worker shutdown wait duration text (wire key "worker_stop_timeout").
func (e *fluentEntry) WorkerStopTimeout(s string) *fluentEntry {
	e.fields = append(e.fields, String("worker_stop_timeout", s))
	return e
}

// CacheSaveTimeout logs configured cache persistence wait duration text (wire key "cache_save_timeout").
func (e *fluentEntry) CacheSaveTimeout(s string) *fluentEntry {
	e.fields = append(e.fields, String("cache_save_timeout", s))
	return e
}

// AuthType logs HTTP adapter auth scheme labels (wire key "auth_type").
func (e *fluentEntry) AuthType(t string) *fluentEntry {
	e.fields = append(e.fields, String("auth_type", t))
	return e
}

func (e *fluentEntry) Int(key string, n int) *fluentEntry {
	e.fields = append(e.fields, Int(key, n))
	return e
}

func (e *fluentEntry) String(key, value string) *fluentEntry {
	e.fields = append(e.fields, String(key, value))
	return e
}

func (e *fluentEntry) Bool(key string, value bool) *fluentEntry {
	e.fields = append(e.fields, Bool(key, value))
	return e
}

func (e *fluentEntry) WithError(err error) *fluentEntry {
	e.fields = append(e.fields, Error(err))
	return e
}

func (e *fluentEntry) WithFields(fields ...Field) *fluentEntry {
	e.fields = append(e.fields, fields...)
	return e
}

// Log emits the built entry.
func (e *fluentEntry) Log() {
	// CRIT-CEF-FLUENT-LEVEL-REQUIRED-001: Enforce explicit level requirement
	// CRIT-CEF-FLUENT-LEVEL-REQUIRED-001: Enforce explicit level requirement
	defer e.release()

	switch e.level {
	case "error":
		e.logger.Error(e.msg, e.err, e.fields...)
	case "debug", "trace":
		e.logger.Debug(e.msg, e.fields...)
	case "warn":
		e.logger.Warn(e.msg, e.fields...)
	case "info":
		e.logger.Info(e.msg, e.fields...)
	default:
		panic("unlevelled log entry: explicit level required (CRIT-CEF-FLUENT-LEVEL-REQUIRED-001)")
	}
}

func (e *fluentEntry) release() {
	root := e.root
	e.root = nil
	e.logger = nil
	e.level = ""
	e.msg = ""
	e.err = nil
	// Drop unusually large backing arrays to avoid long-lived memory spikes.
	if cap(e.fields) > 64 {
		e.fields = nil
	} else {
		e.fields = e.fields[:0]
	}
	if root != nil {
		root.entryPool.Put(e)
	}
}
