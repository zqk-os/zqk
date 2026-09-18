package system

import (
	"context"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNotifyLiteFileAgentChatMaterialized_enqueueJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx := context.Background()
	detail := "agent_chat_channel feed_id=AGF-test"
	err := datacell.EnqueueStewardMaintenance(ctx, root, datacell.ProfileLightFile,
		datacell.MaintenanceOp{Name: datacell.MaintenanceOpRefreshSummary, Detail: detail}, log)
	if err != nil {
		t.Fatal(err)
	}
	b, err := fileutil.ReadFile(datacell.StewardEnqueueJSONLPath(root))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, datacell.MaintenanceOpRefreshSummary) || !strings.Contains(s, "AGF-test") {
		t.Fatalf("jsonl: %s", s)
	}
}

func TestMaterializeAgentChatChannelLiteFromObject(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	cfg, err := materializeAgentChatChannelLiteFromObject(map[string]any{
		objects.FieldKeyKind:                    objects.KindAgentFeed,
		objects.FieldKeyID:                      "AGF-test-1",
		objects.FieldKeyEnabled:                 true,
		objects.FieldKeyDeliveryMode:            "off",
		objects.FieldKeyContractSchemaVersion:   "1",
		objects.FieldKeyNote:                    "n",
		objects.FieldKeyEventsJsonlPathOverride: ".zqk/logs/ide-hooks/custom.jsonl",
		objects.FieldKeyProbeToolAllowlist:      []any{"Shell", "run_terminal_cmd"},
		objects.FieldKeyProbeCommandSubstrings:  []any{"zqk"},
	}, at)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FeedID != "AGF-test-1" || !cfg.Enabled || cfg.DeliveryMode != "off" {
		t.Fatalf("cfg %#v", cfg)
	}
	if cfg.MaterializedAt != "2026-04-16T12:00:00Z" {
		t.Fatalf("ts %q", cfg.MaterializedAt)
	}
	if cfg.EventsJSONLPathOverride != ".zqk/logs/ide-hooks/custom.jsonl" {
		t.Fatalf("override %q", cfg.EventsJSONLPathOverride)
	}
	if len(cfg.ProbeToolAllowlist) != 2 || cfg.ProbeToolAllowlist[0] != "Shell" {
		t.Fatalf("probe_tool_allowlist %#v", cfg.ProbeToolAllowlist)
	}
	if len(cfg.ProbeCommandSubstrings) != 1 || cfg.ProbeCommandSubstrings[0] != "zqk" {
		t.Fatalf("probe_command_substrings %#v", cfg.ProbeCommandSubstrings)
	}
}

func TestMaterializeAgentChatChannelLiteFromObject_wrongKind(t *testing.T) {
	t.Parallel()
	_, err := materializeAgentChatChannelLiteFromObject(map[string]any{
		objects.FieldKeyKind: objects.KindBacklogItem,
		objects.FieldKeyID:   "BLI-1",
	}, time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}
