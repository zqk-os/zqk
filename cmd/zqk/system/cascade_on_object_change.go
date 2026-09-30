package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/execwrap"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// CascadeOnObjectChange performs a TRIGGER/FANOUT cascade for object changes.
// It keeps caches in sync with storage even for background operations that do
// not carry an explicit CacheContext (e.g. compaction/retention flows).
func CascadeOnObjectChange(ctx context.Context, projectRoot, operation, kind, id, sessionID string) {
	if projectRoot == emptyValue || id == emptyValue || kind == emptyValue {
		return
	}

	// Find dependents using the global reverse reference index.
	// Since we are invalidating the validation cache for the modified object,
	// we must also invalidate it for any objects that reference it (dependents)
	// to prevent stale cross-reference validation cache hits.
	dependents := storage.GetGlobalReverseReferenceIndex().GetDependents(id)
	toInvalidate := append([]string{id}, dependents...)

	// Drop pending AUTOFIX batch rows for this id so deferred apply cannot replay
	// snapshotted issues after promote/status repair (stale-batch churn).
	switch operation {
	case storage.OpCreate, storage.OpUpdate, storage.OpDelete:
		EnqueueAutofixPendingPrune(projectRoot, id)
	}

	switch operation {
	case storage.OpCreate, storage.OpUpdate:
		// Shockwave Event: If a global rule is updated, trigger a system-wide shockwave
		// to ensure the graph and all object caches are refreshed properly.
		// contract-change shockwave is async:
		// emit durable outbox at SPEC_ORIGIN_TRIGGER / codegen; on kernel start load +
		// demote shovel_ready|execution_locked that fail new invariants (not mid-build).
		if kind == "validation_rule" || kind == "verification_matrix" || kind == "rule" {
			// Global invalidation shockwave
			storage.InvalidateListCache()
			// Use the global validation cache reset if available, or just clear the known dependents
			// For a true shockwave, we ensure list caches and validation caches drop their state.
			validation.InvalidateObjectsInGlobalValidationCacheWithContext(
				withDefaultCacheMode(ctx),
				projectRoot,
				toInvalidate,
			)

			// Dispatch the shockwave asynchronously to rebuild the caches and re-evaluate the graph
			goroutinelabels.NewGoroutine("shockwave_cache_graph_refresh", "Trigger shockwave to refresh graph and caches after rule update").
				StartSimple(func() {
					// Re-evaluate all objects against the new rules via system check with cache refresh
					cmd := execwrap.Command(paths.CLICommandName, "system", "check", "--refresh-cache")
					zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
					_ = cmd.Run() // Fire and forget the shockwave
				})

			// For the file cache, we also invalidate the CAS cache for all known kinds if we had access,
			// but InvalidateListCache handles the primary memory footprint.
		} else {
			// Normal narrow cascade
			storage.InvalidateListCacheForKind(kind)
			if validation.ShouldCacheValidationState(kind) || len(dependents) > 0 {
				validation.InvalidateObjectsInGlobalValidationCacheWithContext(
					withDefaultCacheMode(ctx),
					projectRoot,
					toInvalidate,
				)
			}
			// Keep object-id-cache on the live CAS hash path after update/promote.
			// CAS post-sync is primary; this covers background paths without CacheContext.
			refreshObjectIDCachePathAfterUpdate(projectRoot, kind, id)
		}
	case storage.OpDelete:
		// Keep object-id-cache aligned with storage by removing the deleted ID.
		InvalidateObjectIDCache(id)

		// Ensure list cache entries for this kind are cleared so subsequent List()
		// calls cannot return the deleted object.
		storage.InvalidateListCacheForKind(kind)

		// Invalidate validation cache entries for the deleted object and its dependents
		// so future system check runs do not report stale validation state.
		if validation.ShouldCacheValidationState(kind) || len(dependents) > 0 {
			validation.InvalidateObjectsInGlobalValidationCacheWithContext(
				withDefaultCacheMode(ctx),
				projectRoot,
				toInvalidate,
			)
		}
	}

	// Agent wake notification for multi-agent collaboration.
	// When work-relevant objects change, notify connected agents via
	// the JSONL chat channel. Uses the shockwave pipeline pattern:
	// CascadeOnObjectChange → async dispatch → agent sessions.
	if isAgentNotifiableKind(kind) {
		capturedOp := operation
		capturedKind := kind
		capturedID := id
		capturedRoot := projectRoot
		capturedSessionID := sessionID
		goroutinelabels.NewGoroutine("agent_wake_notification",
			"Fan out object change to connected MCP agents").
			StartSimple(func() {
				notifyAgentsOfKernelChange(capturedRoot, capturedOp, capturedKind, capturedID, capturedSessionID)
			})
	}
}

// withDefaultCacheMode ensures the cache mode on ctx is suitable for normal
// asynchronous operation; if no cache mode is set, it leaves the context as-is.
func withDefaultCacheMode(ctx context.Context) context.Context {
	if ctx == nil {
		return pkgctx.NewSystemContext()
	}

	// If a cache mode is already present, respect it.
	mode := pkgctx.GetCacheMode(ctx)
	if mode != emptyValue {
		return ctx
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Debug("CascadeOnObjectChange: setting default cache mode for validation cache invalidation").Log()
	return pkgctx.WithCacheMode(ctx, pkgctx.CacheModeDefault)
}

// refreshObjectIDCachePathAfterUpdate re-points object-id-cache at the live CAS
// blob when the listing index already has the new hash (update/promote). No-op
// when the object is missing or not CAS-backed.
func refreshObjectIDCachePathAfterUpdate(projectRoot, kind, id string) {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return
	}
	if storage.IsHighVolumeKindForCache(kind) {
		return
	}
	live, ok := caspkg.ResolveLiveCASFilePath(projectRoot, kind, id)
	if !ok {
		return
	}
	_ = UpdateObjectIDCache(id, kind, live) //nolint:errcheck // best-effort cascade
	storage.ClearObjectIDCachePending(projectRoot, id)
}

// agentNotifiableKinds are object kinds that should trigger agent wake notifications.
// These represent work-relevant objects that agents need to collaborate on.
var agentNotifiableKinds = map[string]struct{}{
	"requirement":          {},
	"backlog_item":         {},
	"goal":                 {},
	"decision":             {},
	"milestone":            {},
	objects.FieldKeyVision: {},
	"mission":              {},
	"workstream":           {},
	"planning":             {},
}

// isAgentNotifiableKind returns true if the object kind should trigger agent wake notifications.
func isAgentNotifiableKind(kind string) bool {
	_, ok := agentNotifiableKinds[kind]
	return ok
}

// agentWakeNotification is the structured payload written to the chat channel JSONL.
type agentWakeNotification struct {
	Type            string   `json:"type"`
	ProjectRoot     string   `json:"project_root"`
	KernelName      string   `json:"kernel_name"`
	Operation       string   `json:"operation"`
	Kind            string   `json:"kind"`
	ObjectID        string   `json:"object_id"`
	Timestamp       string   `json:"timestamp"`
	Message         string   `json:"message"`
	ExcludeSessions []string `json:"exclude_sessions,omitempty"`
}

// notifyAgentsOfKernelChange writes a structured notification to the agent chat channel.
func notifyAgentsOfKernelChange(projectRoot, operation, kind, id, sessionID string) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	kernelName := filepath.Base(projectRoot)
	ts := time.Now().UTC().Format(time.RFC3339)

	notification := agentWakeNotification{
		Type:        "kernel_change",
		ProjectRoot: projectRoot,
		KernelName:  kernelName,
		Operation:   operation,
		Kind:        kind,
		ObjectID:    id,
		Timestamp:   ts,
		Message:     paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("[KERNEL_CHANGE] Project: %s. Object %s %sd: %s (%s). Query: ./bin/zqk object list %s", kernelName, kind, operation, id, kind, kind)),
	}

	if sessionID != "" {
		notification.ExcludeSessions = []string{sessionID}
	}

	// Write to agent chat channel JSONL
	chatPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir,
		paths.IDEHooksLogsSubdir, paths.AgentChatChannelEventsFile)

	if err := fileutil.MkdirAll(filepath.Dir(chatPath), paths.DirPerm755); err != nil {
		logging.Fluent(logger).Warn(fmt.Sprintf("agent_wake: mkdir failed: %v", err)).Log()
		return
	}

	jsonBytes, err := json.Marshal(notification)
	if err != nil {
		logging.Fluent(logger).Warn(fmt.Sprintf("agent_wake: marshal failed: %v", err)).Log()
		return
	}

	f, err := fileutil.OpenFile(chatPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		logging.Fluent(logger).Warn(fmt.Sprintf("agent_wake: open failed: %v", err)).Log()
		return
	}
	_, _ = f.Write(append(jsonBytes, '\n'))
	_ = f.Close()

	logging.Fluent(logger).Info(fmt.Sprintf("agent_wake: notified kind=%s op=%s id=%s", kind, operation, id)).Log()
}
