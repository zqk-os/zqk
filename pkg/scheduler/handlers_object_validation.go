package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// Log events for object_validation handler (POLICY-CODE-007 stable keys).
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
}

// NewObjectValidationHandler creates a handler that runs "zqk system check <kind> <id> --background".
// NewObjectValidationHandler creates a new object validation handler
func NewObjectValidationHandler(projectRoot string, logger logging.Logger) ObjectValidationHandlerInterface {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &ObjectValidationHandler{
		projectRoot: projectRoot,
		logger:      logger,
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

	for i, it := range items {
		kind, _ := it[objects.FieldKeyKind].(string)
		id, _ := it[objects.FieldKeyID].(string)
		operation, _ := it[objects.FieldKeyOperation].(string)
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
		submitCmd := fmt.Sprintf("%s system check %s %s --background", exe, kind, id)
		args := []string{"-c", submitCmd}
		cmdStr := strings.Join(append([]string{"bash"}, args...), " ")
		SLog(h.logger).Info(LogEventObjectValidationProcessing).
			JobID(job.ID).
			Int("batch_index", i+1).
			Int("batch_size", len(items)).
			Kind(kind).
			String("id", id).
			String("operation", operation).
			String("command", cmdStr).
			Log()

		runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		cmd := exec.CommandContext(runCtx, "bash", args...)
		if h.projectRoot != emptyValue {
			cmd.Dir = h.projectRoot
		}
		// Ensure `zqk system check --background` switches to the async "follow" path
		// (so the check actually runs to completion inside the scheduler job budget).
		// `run_wrapper` sets this env var too; for object_validation we inject it here.
		cmd.Env = withZQKJobIDEnv(os.Environ(), job.ID)
		err := cmd.Run()
		cancel()
		if err != nil {
			return errfmt.Errorf("validation command failed for %s %s: %w", kind, id, err)
		}
	}
	return nil
}

func withZQKJobIDEnv(baseEnv []string, jobID string) []string {
	env := append([]string(nil), baseEnv...)
	if jobID == emptyValue {
		return env
	}
	env = append(env, fmt.Sprintf("%s=%s", zqkenv.JobID(), jobID))
	return env
}
