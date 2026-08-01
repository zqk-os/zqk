package scheduler

import (
	"context"
	"os"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
)

func TestStewardEnqueueCoordinator_Enqueue_writesJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	c, err := NewStreamStewardEnqueueCoordinator(root, log)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	op := datacell.MaintenanceOp{Name: datacell.MaintenanceOpRefreshSummary, Detail: "test"}
	if err := c.Enqueue(ctx, op); err != nil {
		t.Fatal(err)
	}
	path := datacell.StewardEnqueueJSONLPath(root)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), datacell.MaintenanceOpRefreshSummary) {
		t.Fatalf("file content: %s", string(b))
	}
	if !strings.Contains(string(b), "stream") {
		t.Fatalf("want stream profile in %s", string(b))
	}
}

func TestNewStewardEnqueueCoordinator_unknownProfile(t *testing.T) {
	t.Parallel()
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_, err := NewStewardEnqueueCoordinator(t.TempDir(), datacell.StorageProfile("unknown"), log)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewCASEntityStewardEnqueueCoordinator_acceptsProfile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	cIface, err := NewCASEntityStewardEnqueueCoordinator(root, log)
	if err != nil {
		t.Fatal(err)
	}
	c := cIface.(*StewardEnqueueCoordinator)
	if c.Profile != datacell.ProfileCASEntity {
		t.Fatalf("profile=%q", c.Profile)
	}
}
