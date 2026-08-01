package stewardbase

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

// StewardEnqueueDetailMaxBytes caps optional MaintenanceOp.Detail on the steward JSONL queue.
const StewardEnqueueDetailMaxBytes = 512

// Log events (POLICY-CODE-007 stable keys for dashboards).
const LogEventStewardEnqueue = "datacell_steward_enqueue"

// EnqueueStewardMaintenance persists one steward enqueue JSONL record.
func EnqueueStewardMaintenance(ctx context.Context, projectRoot string, profile datacell.StorageProfile, op datacell.MaintenanceOp, logger logging.Logger) error {
	if !profile.IsKnown() {
		return errfmt.Errorf("enqueue steward maintenance: unknown storage_profile %q", profile)
	}

	detail := op.Detail
	if len(detail) > StewardEnqueueDetailMaxBytes {
		detail = detail[:StewardEnqueueDetailMaxBytes]
	}

	rec := map[string]any{
		objects.FieldKeySchemaVersion: "steward_enqueue_v1",
		"storage_profile":             string(profile),
		"op":                          op.Name,
		"detail":                      detail,
		"enqueued_at_rfc3339":         time.Now().UTC().Format(time.RFC3339),
	}

	if err := AppendStewardEnqueueRecord(projectRoot, rec); err != nil {
		return err
	}

	if logger != nil {
		logging.Fluent(logger).Info(LogEventStewardEnqueue).
			String("storage_profile", string(profile)).
			String("op", op.Name).
			String("detail", detail).
			Log()
	}
	return nil
}

// AppendStewardEnqueueRecord appends one JSON line to the steward enqueue JSONL file.
func AppendStewardEnqueueRecord(projectRoot string, rec map[string]any) error {
	root := filepath.Clean(projectRoot)
	if root == "" || root == "." {
		return errfmt.Errorf("append steward enqueue: project root required")
	}

	// Fallback path calculation to avoid datacell dependency
	path := filepath.Join(root, paths.ProjectDataDir, "logs", "datacell", "steward_enqueue.jsonl")

	line, err := json.Marshal(rec)
	if err != nil {
		return errfmt.Newf("append steward enqueue: marshal").Wrap(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return errfmt.Newf("append steward enqueue: mkdir").Wrap(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf("append steward enqueue: open").Wrap(err)
	}
	_, wErr := f.Write(append(line, '\n'))
	cErr := f.Close()
	if wErr != nil {
		return errfmt.Newf("append steward enqueue: write").Wrap(wErr)
	}
	return cErr
}
