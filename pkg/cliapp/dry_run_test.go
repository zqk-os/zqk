package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestDryRunMessages(t *testing.T) {
	msg := DryRunMessage("test action")
	if !strings.HasPrefix(msg, "[DRY RUN]") {
		t.Errorf("expected [DRY RUN] prefix, got: %s", msg)
	}

	would := DryRunWould("delete object")
	if would != "[DRY RUN] Would delete object" {
		t.Errorf("unexpected DryRunWould: %s", would)
	}

	wouldRun := DryRunWouldRun("zqk object prune")
	if wouldRun != "[DRY RUN] Would run: zqk object prune" {
		t.Errorf("unexpected DryRunWouldRun: %s", wouldRun)
	}

	wouldRunf := DryRunWouldRunf("zqk object prune --kind %s", "goal")
	if wouldRunf != "[DRY RUN] Would run: zqk object prune --kind goal" {
		t.Errorf("unexpected DryRunWouldRunf: %s", wouldRunf)
	}
}

func TestDryRunHandler_CreateAndUpdate(t *testing.T) {
	logger := logging.GetLogger()
	handler := NewDryRunHandler(logger)

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().Bool("dry-run", false, "dry run")

	// Not dry run
	handled, err := handler.HandleCreateDryRun(cmd, map[string]any{"id": "GOAL-1"}, "goal")
	if handled || err != nil {
		t.Errorf("expected false, nil when dry-run is false")
	}

	// Dry run enabled
	cmd.Flags().Set("dry-run", "true")
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	handled, err = handler.HandleCreateDryRun(cmd, map[string]any{"id": "GOAL-1", "title": "Test Goal"}, "goal")
	if !handled || err != nil {
		t.Fatalf("expected handled=true, err=nil, got %v, %v", handled, err)
	}
	out := buf.String()
	if !strings.Contains(out, "Would create object:") || !strings.Contains(out, "GOAL-1") {
		t.Errorf("expected output to contain object YAML, got: %s", out)
	}

	// Update dry run
	buf.Reset()
	handled, err = handler.HandleUpdateDryRun(cmd, "GOAL-1", map[string]any{"title": "Old"}, map[string]any{"title": "New"})
	if !handled || err != nil {
		t.Fatalf("expected handled=true, err=nil, got %v, %v", handled, err)
	}
	out = buf.String()
	if !strings.Contains(out, "Would update object GOAL-1:") {
		t.Errorf("expected update dry-run output, got: %s", out)
	}
}
