package scheduler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// Log events for object_validation handler (POL-CODE-007 stable keys).
const (
	LogEventObjectValidationSkipBatchItem = JobTypeObjectValidation + "_skip_batch_item"
	LogEventObjectValidationProcessing    = JobTypeObjectValidation + "_processing"
)

// ObjectValidationHandler runs background validation for a single object.
// It is triggered by TriggerJobByEvent("object_validation", kind, eventData)
// with eventData containing kind, id, operation. No new scheduler_job is created—
// the same job is reused, avoiding recursion and one-off job proliferation.
type ObjectValidationHandler struct {
	projectRoot string
	logger      logging.Logger
	runCommand  objectValidationCommandRunner
}

type objectValidationCommandRunner func(ctx context.Context, exe string, args, env []string, dir string) error

func runObjectValidationCommand(ctx context.Context, exe string, args, env []string, dir string) error {
	cmd := execwrap.CommandContext(ctx, exe, args...)
	cmd.Env = env
	if dir != emptyValue {
		cmd.Dir = dir
	}
	return cmd.Run()
}

// NewObjectValidationHandler creates a handler that runs one targeted system check per event batch.
func NewObjectValidationHandler(projectRoot string, logger logging.Logger) ObjectValidationHandlerInterface {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &ObjectValidationHandler{
		projectRoot: projectRoot,
		logger:      logger,
		runCommand:  runObjectValidationCommand,
	}
}

// Execute runs validation for the object(s) in event data.
// Event data may be:
//   - Single: {"kind": "<kind>", "id": "<id>", "operation": "<op>"}
//   - Batch:  {"items": [{"kind":"...","id":"...","operation":"..."}, ...]}
func (h *ObjectValidationHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunObjectValidationViaPipeline(ctx, h, job)
}

// executeObjectValidationCore runs validation for the object(s) in event data.
// Called from RunObjectValidationViaPipeline NORMALIZE stage.
func (h *ObjectValidationHandler) executeObjectValidationCore(ctx context.Context, job *ScheduledJob) error {
	eventData := ctx.Value(evtDataKey{})
	if eventData == nil {
		return errfmt.Errorf("no event data provided for object_validation job")
	}
	eventDataMap, ok := eventData.(map[string]any)
	if !ok {
		return errfmt.Errorf("invalid event data format for object_validation job")
	}

	var items []map[string]any
	if rawItems, ok := eventDataMap["items"].([]any); ok && len(rawItems) > 0 {
		for _, r := range rawItems {
			if m, ok := r.(map[string]any); ok {
				items = append(items, m)
			}
		}
	}
	if len(items) == 0 {
		// Single-item format
		kind, _ := eventDataMap[objects.FieldKeyKind].(string)
		id, _ := eventDataMap[objects.FieldKeyID].(string)
		if kind != emptyValue && id != emptyValue {
			items = []map[string]any{{
				objects.FieldKeyKind:      kind,
				objects.FieldKeyID:        id,
				objects.FieldKeyOperation: eventDataMap[objects.FieldKeyOperation],
			}}
		}
	}
	if len(items) == 0 {
		return errfmt.Errorf("event data must include kind and id, or non-empty items array")
	}

	// Use resolved CLI binary so validation works when daemon has minimal PATH and avoids unstable wrappers.
	exe := resolveSchedulerCLIBinary(h.projectRoot)
	if exe == emptyValue {
		exe = brand.ExecutableName()
		if exe == emptyValue {
			exe = "zqk"
		}
	}

	ids := make([]string, 0, len(items))
	seenIDs := make(map[string]struct{}, len(items))
	for _, it := range items {
		kind, _ := it[objects.FieldKeyKind].(string)
		id, _ := it[objects.FieldKeyID].(string)
		if kind == emptyValue || id == emptyValue {
			continue
		}
		if ShouldSkipBackgroundValidationBatchForKind(kind) {
			SLog(h.logger).Debug(LogEventObjectValidationSkipBatchItem).
				JobID(job.ID).
				Kind(kind).
				String("id", id).
				Log()
			continue
		}
		if _, exists := seenIDs[id]; exists {
			continue
		}
		seenIDs[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	// A check with IDs only infers each kind from its ID. Sending the entire event batch
	// through one process avoids rebuilding and warming all system-check caches per object.
	args := append([]string{"system", "check"}, ids...)
	args = append(args, "--background")
	SLog(h.logger).Info(LogEventObjectValidationProcessing).
		JobID(job.ID).
		Int("batch_size", len(ids)).
		String("command", strings.Join(append([]string{exe}, args...), " ")).
		Log()

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	runner := h.runCommand
	if runner == nil {
		runner = runObjectValidationCommand
	}
	// Ensure `zqk system check --background` switches to the async "follow" path
	// (so the check actually runs to completion inside the scheduler job budget).
	// `run_wrapper` sets this env var too; for object_validation we inject it here.
	if err := runner(runCtx, exe, args, withZQKJobIDEnv(os.Environ(), job.ID), h.projectRoot); err != nil {
		return errfmt.Errorf("validation command failed for %d object(s): %w", len(ids), err)
	}
	return nil
}

func withZQKJobIDEnv(baseEnv []string, jobID string) []string {
	env := append([]string(nil), baseEnv...)
	if jobID == emptyValue {
		return env
	}
	env = append(env, fmt.Sprintf("%s=%s", zqkenv.JobID().Name(), jobID))
	return env
}
