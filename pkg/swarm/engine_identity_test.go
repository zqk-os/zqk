package swarm

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/lanceman/zqk/pkg/zqksession"
)

func TestEngine_correlationFields_preferExplicitIdentity(t *testing.T) {
	t.Parallel()
	e := NewEngine(nil, nil, 0, "eng-1").WithRunIdentity("ZQK-SESS", "ATK-1")
	fields := e.correlationFields(context.Background())
	got := fieldMap(fields)
	if got["engineID"] != "eng-1" {
		t.Fatalf("engineID=%q", got["engineID"])
	}
	if got[objects.FieldKeySessionID] != "ZQK-SESS" {
		t.Fatalf("session_id=%q", got[objects.FieldKeySessionID])
	}
	if got["task_id"] != "ATK-1" {
		t.Fatalf("task_id=%q", got["task_id"])
	}
}

func TestEngine_resolvedSessionID_contextThenEnv(t *testing.T) {
	t.Setenv(zqkenv.SessionID(), "ZQK-FROM-ENV")
	e := NewEngine(nil, nil, 0, "eng-2")
	if got := e.resolvedSessionID(context.Background()); got != "ZQK-FROM-ENV" {
		t.Fatalf("env session=%q", got)
	}
	ctx := zqksession.WithID(context.Background(), "ZQK-FROM-CTX")
	if got := e.resolvedSessionID(ctx); got != "ZQK-FROM-CTX" {
		t.Fatalf("ctx session=%q", got)
	}
}

func fieldMap(fields []logging.Field) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		if f.Key == "" {
			continue
		}
		if s, ok := f.Value.(string); ok {
			out[f.Key] = s
		}
	}
	return out
}
