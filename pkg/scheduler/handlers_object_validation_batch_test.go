package scheduler

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestObjectValidationHandlerRunsBatchInOneCheck(t *testing.T) {
	handler := NewObjectValidationHandler(t.TempDir(), nil).(*ObjectValidationHandler)
	var calls int
	var gotArgs, gotEnv []string
	handler.runCommand = func(_ context.Context, _ string, args, env []string, _ string) error {
		calls++
		gotArgs = append([]string(nil), args...)
		gotEnv = append([]string(nil), env...)
		return nil
	}

	eventData := map[string]any{
		"items": []any{
			map[string]any{objects.FieldKeyKind: "criteria", objects.FieldKeyID: "CRIT-1"},
			map[string]any{objects.FieldKeyKind: "backlog_item", objects.FieldKeyID: "BLI-1"},
			map[string]any{objects.FieldKeyKind: "criteria", objects.FieldKeyID: "CRIT-1"},
			map[string]any{objects.FieldKeyKind: objects.KindSchedulerJob, objects.FieldKeyID: "SCH-1"},
			map[string]any{objects.FieldKeyKind: "", objects.FieldKeyID: "INVALID-1"},
		},
	}
	ctx := context.WithValue(pkgctx.NewSystemContext(), evtDataKey{}, eventData)
	job := &ScheduledJob{ID: BackgroundObjectValidationJobID, JobType: JobTypeObjectValidation}

	if err := handler.executeObjectValidationCore(ctx, job); err != nil {
		t.Fatalf("executeObjectValidationCore() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("command calls = %d, want 1", calls)
	}
	wantArgs := []string{"system", "check", "CRIT-1", "BLI-1", "--background"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command args = %v, want %v", gotArgs, wantArgs)
	}
	wantJobEnv := zqkenv.JobID().Name() + "=" + BackgroundObjectValidationJobID
	if !containsString(gotEnv, wantJobEnv) {
		t.Fatalf("command environment does not contain %q", wantJobEnv)
	}
}

func TestObjectValidationHandlerBatchErrorCoversEveryID(t *testing.T) {
	handler := NewObjectValidationHandler(t.TempDir(), nil).(*ObjectValidationHandler)
	sentinel := errors.New("check failed")
	var gotArgs []string
	handler.runCommand = func(_ context.Context, _ string, args, _ []string, _ string) error {
		gotArgs = append([]string(nil), args...)
		return sentinel
	}

	eventData := map[string]any{
		"items": []any{
			map[string]any{objects.FieldKeyKind: "criteria", objects.FieldKeyID: "CRIT-1"},
			map[string]any{objects.FieldKeyKind: "criteria", objects.FieldKeyID: "CRIT-2"},
		},
	}
	ctx := context.WithValue(pkgctx.NewSystemContext(), evtDataKey{}, eventData)
	job := &ScheduledJob{ID: BackgroundObjectValidationJobID, JobType: JobTypeObjectValidation}

	err := handler.executeObjectValidationCore(ctx, job)
	if !errors.Is(err, sentinel) {
		t.Fatalf("executeObjectValidationCore() error = %v, want wrapped sentinel", err)
	}
	for _, id := range []string{"CRIT-1", "CRIT-2"} {
		if !containsString(gotArgs, id) {
			t.Errorf("failed batch command does not include %s: %v", id, gotArgs)
		}
	}
	if !strings.Contains(err.Error(), "2 object(s)") {
		t.Fatalf("error = %q, want batch size", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
