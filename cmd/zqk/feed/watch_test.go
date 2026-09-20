package feed

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestProcessFeed_RendersFullMessageAndLabelsWakeStubs(t *testing.T) {
	root := t.TempDir()
	agentID := "test-agent"
	personaRef := "PER-TEST"

	cfg := datacell.AgentChatChannelConfig{
		SchemaVersion:         datacell.AgentChatChannelSchemaVersion,
		Enabled:               true,
		DeliveryMode:          datacell.DeliveryModeNotify,
		FeedID:                "AGF-test",
		ContractSchemaVersion: "1",
	}
	if err := datacell.WriteAgentChatChannelConfig(root, cfg); err != nil {
		t.Fatalf("WriteAgentChatChannelConfig: %v", err)
	}

	eventsPath := datacell.EffectiveAgentChatChannelEventsJSONLPath(root, cfg)
	_ = fileutil.MkdirAll(filepath.Dir(eventsPath), paths.DirPerm755)

	f, err := fileutil.OpenFile(eventsPath, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, paths.FilePerm644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	const (
		longSteerMessage = "This is a very long message that exceeds 120 characters to ensure that the full message body is indeed rendered in the watch output and not truncated."
		wakeStubMessage  = "ATTN PEER — wake [xyz] — zqk workflow whats-next"
		peerAgentID      = "other-agent"
	)

	steerEvent := map[string]interface{}{
		agentfeed.JSONFieldEventID:     "AFE-steer",
		agentfeed.JSONFieldTimestamp:   time.Now().Format(time.RFC3339),
		objects.FieldKeyEventType:      agentfeed.FeedEventTypeSteering,
		agentfeed.JSONFieldMessage:     longSteerMessage,
		objects.FieldKeySummary:        "Short summary",
		agentfeed.JSONFieldFromAgentID: peerAgentID,
		agentfeed.JSONFieldToAgentID:   agentID,
	}
	b1, _ := json.Marshal(steerEvent)
	_, _ = f.Write(append(b1, '\n'))

	wakeEvent := map[string]interface{}{
		agentfeed.JSONFieldEventID:     "AFE-wake",
		agentfeed.JSONFieldTimestamp:   time.Now().Format(time.RFC3339),
		objects.FieldKeyEventType:      agentfeed.FeedEventTypeWake,
		agentfeed.JSONFieldMessage:     wakeStubMessage,
		objects.FieldKeySummary:        wakeStubMessage,
		agentfeed.JSONFieldFromAgentID: peerAgentID,
		agentfeed.JSONFieldToAgentID:   agentID,
	}
	b2, _ := json.Marshal(wakeEvent)
	_, _ = f.Write(append(b2, '\n'))
	_ = f.Close()

	var buf bytes.Buffer
	logger := logging.NewLogger(&buf, logging.InfoLevel, &logging.JSONFormatter{})

	processFeed(logger, root, agentID, personaRef, false)

	output := buf.String()
	if !strings.Contains(output, "This is a very long message that exceeds 120 characters") {
		t.Errorf("expected full message body to be rendered, got: %s", output)
	}
	if !strings.Contains(output, "[wake/notify]") {
		t.Errorf("expected wake stub to be labeled with [wake/notify], got: %s", output)
	}
}
