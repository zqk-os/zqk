package datacell

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

func TestLoggingCellCoordinator_nilSafe(t *testing.T) {
	t.Parallel()
	var c *LoggingCellCoordinator
	if err := c.Enqueue(context.Background(), MaintenanceOp{Name: MaintenanceOpInvalidateCache}); err != nil {
		t.Fatal(err)
	}
	c = &LoggingCellCoordinator{Logger: nil}
	if err := c.Enqueue(context.Background(), MaintenanceOp{Name: "x"}); err != nil {
		t.Fatal(err)
	}
}

type countingCoordinator struct {
	calls int
}

func (c *countingCoordinator) Enqueue(ctx context.Context, op MaintenanceOp) error {
	c.calls++
	_, _ = ctx, op
	return nil
}

func TestLoggingCellCoordinator_InfoUsesMaintenanceEnqueueLogKey(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := logging.NewLogger(&buf, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	c := &LoggingCellCoordinator{
		Logger:  log,
		Profile: ProfileStream,
	}
	if err := c.Enqueue(context.Background(), MaintenanceOp{
		Name:   MaintenanceOpInvalidateCache,
		Detail: "unit-detail",
	}); err != nil {
		t.Fatal(err)
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry); err != nil {
		t.Fatalf("log JSON: %v body=%q", err, buf.String())
	}
	if got, _ := entry["message"].(string); got != LogEventDataCellMaintenanceEnqueue {
		t.Fatalf("message: got %q want %q", got, LogEventDataCellMaintenanceEnqueue)
	}
	if got, _ := entry["op"].(string); got != MaintenanceOpInvalidateCache {
		t.Fatalf("op field: got %v", got)
	}
	if got, _ := entry["detail"].(string); got != "unit-detail" {
		t.Fatalf("detail field: got %v", got)
	}
	if got, _ := entry["storage_profile"].(string); got != string(ProfileStream) {
		t.Fatalf("storage_profile: got %v", got)
	}
}

func TestLoggingCellCoordinator_delegatesToInner(t *testing.T) {
	t.Parallel()
	inner := &countingCoordinator{}
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	c := &LoggingCellCoordinator{
		Logger:  log,
		Inner:   inner,
		Profile: ProfileStream,
	}
	if err := c.Enqueue(context.Background(), MaintenanceOp{Name: MaintenanceOpRefreshSummary, Detail: "unit"}); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner Enqueue calls: got %d want 1", inner.calls)
	}
}
