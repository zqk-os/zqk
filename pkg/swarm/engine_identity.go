package swarm

import (
	"context"
	"strings"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqksession"
)

// WithRunIdentity stamps session and task ids onto every run-loop log so concurrent
// engines are distinguishable (session_id, engineID, task_id).
func (e *Engine) WithRunIdentity(sessionID, taskID string) *Engine {
	e.sessionID = strings.TrimSpace(sessionID)
	e.taskID = strings.TrimSpace(taskID)
	return e
}

func (e *Engine) resolvedSessionID(ctx context.Context) string {
	if e.sessionID != "" {
		return e.sessionID
	}
	if id := zqksession.GetIDFromContext(ctx); id != "" {
		return id
	}
	return strings.TrimSpace(zqkenv.SessionID().Get())
}

func (e *Engine) correlationFields(ctx context.Context) []logging.Field {
	out := make([]logging.Field, 0, 3)
	if e.engineID != "" {
		out = append(out, logging.EngineIDField(e.engineID))
	}
	if sid := e.resolvedSessionID(ctx); sid != "" {
		out = append(out, logging.SessionIDField(sid))
	}
	if e.taskID != "" {
		out = append(out, logging.TaskIDField(e.taskID))
	}
	return out
}

func (e *Engine) fields(ctx context.Context, extra ...logging.Field) []logging.Field {
	base := e.correlationFields(ctx)
	out := make([]logging.Field, 0, len(base)+len(extra))
	out = append(out, base...)
	out = append(out, extra...)
	return out
}
