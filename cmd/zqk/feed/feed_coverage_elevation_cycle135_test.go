package feed

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupCycle135IsolatedTestEnv(t *testing.T) (string, storage.ObjectStorageProvider, func()) {
	t.Helper()
	tempDir, provider, cleanup := initFeedTestHarness(t)
	t.Setenv(zqkenv.ProjectRoot().Name(), tempDir)
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)
	return tempDir, provider, cleanup
}

func TestCycle135_AckEdgeCases(t *testing.T) {
	tempDir, provider, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("missing in-reply-to flag", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "ack", "--agent-id", "SEAT-1", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--in-reply-to is required")
	})

	t.Run("missing agent-id flag", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "ack", "--in-reply-to", "EVT-TEST-1", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--agent-id is required")
	})

	t.Run("missing persona-ref flag", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "ack", "--in-reply-to", "EVT-TEST-1", "--agent-id", "SEAT-1")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--persona-ref is required")
	})

	t.Run("non-existent persona-ref", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "ack", "--in-reply-to", "EVT-TEST-1", "--agent-id", "SEAT-1", "--persona-ref", "PER-NON-EXISTENT")
		require.Error(t, err)
		require.Contains(t, err.Error(), "resolve persona")
	})

	t.Run("persona wrong kind", func(t *testing.T) {
		secCtx := pkgctx.NewSystemSecurityContext()
		bgCtx := context.Background()
		storage.CreateCASVisible(t, provider, bgCtx, secCtx, map[string]any{
			objects.FieldKeyID:     "GOAL-WRONG-KIND",
			objects.FieldKeyKind:   "goal",
			objects.FieldKeyTitle:  "Wrong Kind Goal",
			objects.FieldKeyStatus: "approved",
		}, "approved")

		_, err := executeFeedCommand(t, tempDir, provider, "ack", "--in-reply-to", "EVT-TEST-1", "--agent-id", "SEAT-1", "--persona-ref", "GOAL-WRONG-KIND")
		require.Error(t, err)
		require.Contains(t, err.Error(), "want persona")
	})

	t.Run("execution with await completion and unauthorized ack", func(t *testing.T) {
		steerOut, err := executeFeedCommand(t, tempDir, provider, "steer",
			"--agent-id", "SEAT-SENDER",
			"--to-agent-id", "SEAT-ACK-TARGET",
			"--await-peer-ack",
			"--message", "Require prompt review",
			"--format", "json")
		require.NoError(t, err)

		var steerMap map[string]any
		err = json.Unmarshal([]byte(steerOut), &steerMap)
		require.NoError(t, err)
		evtID, ok := steerMap[agentfeed.JSONFieldEventID].(string)
		require.True(t, ok)
		require.NotEmpty(t, evtID)

		// Unauthorized ack by different agent
		_, err = executeFeedCommand(t, tempDir, provider, "ack",
			"--in-reply-to", evtID,
			"--agent-id", "SEAT-UNAUTHORIZED",
			"--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER",
			"--summary", "Attempted ack")
		require.Error(t, err)

		// Authorized ack by intended target
		ackOut, err := executeFeedCommand(t, tempDir, provider, "ack",
			"--in-reply-to", evtID,
			"--agent-id", "SEAT-ACK-TARGET",
			"--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER",
			"--summary", "Completed peer review",
			"--format", "json")
		require.NoError(t, err)
		require.Contains(t, ackOut, "peer_ack")
		require.Contains(t, ackOut, "in_reply_to")
		require.Contains(t, ackOut, "success")
	})
}

func TestCycle135_BridgeIngestEdgeCases(t *testing.T) {
	tempDir, provider, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("missing both payload and payload-file", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest", "--channel", "generic")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--payload-file or --payload")
	})

	t.Run("non-existent payload-file", func(t *testing.T) {
		nonExistent := filepath.Join(tempDir, "missing.json")
		_, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest", "--channel", "generic", "--payload-file", nonExistent)
		require.Error(t, err)
	})

	t.Run("valid payload-file execution", func(t *testing.T) {
		payloadPath := filepath.Join(tempDir, "payload.json")
		err := fileutil.WriteFile(payloadPath, []byte(`{"text":"Payload from file"}`), paths.FilePerm644)
		require.NoError(t, err)

		out, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest",
			"--channel", "generic",
			"--payload-file", payloadPath,
			"--agent-id", "SEAT-BRIDGE-FILE",
			"--format", "json")
		require.NoError(t, err)
		require.Contains(t, out, "success")
	})

	t.Run("invalid json payload parse error", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest",
			"--channel", "generic",
			"--payload", "{invalid json syntax}")
		require.Error(t, err)
		require.Contains(t, err.Error(), "feed bridge-ingest parse")
	})

	t.Run("feed-id mismatch error", func(t *testing.T) {
		configPath := datacell.AgentChatChannelConfigPath(tempDir)
		cfgData, rErr := fileutil.ReadFile(configPath)
		require.NoError(t, rErr)
		var cfg datacell.AgentChatChannelConfig
		rErr = json.Unmarshal(cfgData, &cfg)
		require.NoError(t, rErr)
		cfg.FeedID = "FEED-EXPECTED-123"
		cfgBytes, mErr := json.Marshal(cfg)
		require.NoError(t, mErr)
		wErr := fileutil.WriteFile(configPath, cfgBytes, paths.FilePerm644)
		require.NoError(t, wErr)

		_, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest",
			"--channel", "generic",
			"--payload", `{"text":"test mismatch"}`,
			"--feed-id", "FEED-MISMATCH-456")
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not match --feed-id")
	})

	t.Run("overrides and no-ack no-wake flags", func(t *testing.T) {
		out, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest",
			"--channel", "generic",
			"--payload", `{"text":"test overrides"}`,
			"--agent-id", "SEAT-OVERRIDE",
			"--to-agent-id", "SEAT-DEST",
			"--no-ack",
			"--no-wake",
			"--format", "json")
		require.NoError(t, err)
		require.Contains(t, out, "success")
	})
}

func TestCycle135_DoctorSeatRefreshAndDefaultSeating(t *testing.T) {
	tempDir, provider, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("doctor with refresh-seats and dry-run", func(t *testing.T) {
		out, err := executeFeedCommand(t, tempDir, provider, "doctor",
			"--refresh-seats",
			"--dry-run",
			"--format", "json")
		require.NoError(t, err)

		var res map[string]any
		err = json.Unmarshal([]byte(out), &res)
		require.NoError(t, err)
		require.True(t, res["default_seating_ok"].(bool))
		require.Contains(t, res, "refresh_seats")
	})

	t.Run("doctor detects missing default seating", func(t *testing.T) {
		p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
			Kind:            "cmd.feed.cycle135.missing_seating",
			SeedSchemaPlane: true,
		})
		enableAgentChatChannelForTest(t, p.Root)
		t.Setenv(zqkenv.ProjectRoot().Name(), p.Root)
		t.Setenv(zqkenv.TestRoot().Name(), p.Root)

		out, err := executeFeedCommand(t, p.Root, p.FileStorage, "doctor", "--format", "json")
		require.NoError(t, err)

		var res map[string]any
		err = json.Unmarshal([]byte(out), &res)
		require.NoError(t, err)
		require.False(t, res["default_seating_ok"].(bool))
		require.Contains(t, res, "missing_default_seating")
	})
}

func TestCycle135_EmitStatusEdgeCases(t *testing.T) {
	tempDir, provider, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("missing persona-ref", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "emit-status", "--agent-id", "SEAT-1", "--summary", "done")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--persona-ref is required")
	})

	t.Run("missing agent-id", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "emit-status", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER", "--summary", "done")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--agent-id is required")
	})

	t.Run("agent-id identical to persona-ref", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "emit-status",
			"--agent-id", "PER-COMMUNITY-SOFTWARE-ENGINEER",
			"--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER",
			"--summary", "done")
		require.Error(t, err)
		require.Contains(t, err.Error(), "must differ")
	})

	t.Run("archived persona status", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "emit-status",
			"--agent-id", "SEAT-ARCHIVED",
			"--persona-ref", "PER-ARCHIVED-DEV",
			"--summary", "attempt")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not usable for mesh stamp")
	})

	t.Run("successful emit-status with pulse-human", func(t *testing.T) {
		out, err := executeFeedCommand(t, tempDir, provider, "emit-status",
			"--agent-id", "SEAT-ACTIVE",
			"--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER",
			"--summary", "Completed milestone phase",
			"--pulse-human",
			"--format", "json")
		require.NoError(t, err)
		require.Contains(t, out, "success")
	})
}

func TestCycle135_PendingAndProofOfLifeEdgeCases(t *testing.T) {
	tempDir, provider, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("pending without agent-id", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "pending")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--agent-id is required")
	})

	t.Run("pending with agent-id and limit", func(t *testing.T) {
		out, err := executeFeedCommand(t, tempDir, provider, "pending",
			"--agent-id", "SEAT-PENDING-CHECK",
			"--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER",
			"--limit", "10",
			"--format", "json")
		require.NoError(t, err)
		require.Contains(t, out, "success")
	})

	t.Run("proof-of-life missing agent-id", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "proof-of-life", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--agent-id is required")
	})

	t.Run("proof-of-life missing persona-ref", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "proof-of-life", "--agent-id", "SEAT-1")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--persona-ref is required")
	})

	t.Run("proof-of-life agent-id identical to persona-ref", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "proof-of-life",
			"--agent-id", "PER-COMMUNITY-SOFTWARE-ENGINEER",
			"--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER")
		require.Error(t, err)
		require.Contains(t, err.Error(), "must differ")
	})

	t.Run("proof-of-life valid execution", func(t *testing.T) {
		out, err := executeFeedCommand(t, tempDir, provider, "proof-of-life",
			"--agent-id", "SEAT-POL-ALIVE",
			"--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER",
			"--summary", "alive and well",
			"--no-pulse",
			"--format", "json")
		require.NoError(t, err)
		require.Contains(t, out, "success")
	})
}

func TestCycle135_ServeLifecycleAndValidation(t *testing.T) {
	tempDir, provider, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("serve rejects unauthenticated plain non-loopback listen address", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "serve", "--listen", "192.168.1.1:8787")
		require.Error(t, err)
		require.Contains(t, err.Error(), "refusing to bind unauthenticated plain HTTP feed server")
	})

	t.Run("serve graceful shutdown with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := executeFeedCommandWithContext(t, ctx, tempDir, provider, "serve", "--listen", "127.0.0.1:0")
		require.NoError(t, err)
	})
}

func TestCycle135_SteerAndWakeEdgeCases(t *testing.T) {
	tempDir, provider, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("steer missing message", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "steer")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing required parameter: message")
	})

	t.Run("steer directed hourglass error", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "steer",
			"--message", "Awaiting peer ack without target",
			"--to-agent-id", "SEAT-TARGET")
		require.Error(t, err)
		require.Contains(t, err.Error(), "POL-AGENT-ORCH-HOURGLASS-001")
	})

	t.Run("steer non-interrupting and reprime flags", func(t *testing.T) {
		out, err := executeFeedCommand(t, tempDir, provider, "steer",
			"--message", "Background task update",
			"--non-interrupting",
			"--reprime",
			"--format", "json")
		require.NoError(t, err)
		require.Contains(t, out, "success")
	})

	t.Run("wake missing message", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "wake")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing required parameter: message")
	})

	t.Run("wake directed hourglass error", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "wake",
			"--message", "Wake target without await",
			"--to-agent-id", "SEAT-TARGET")
		require.Error(t, err)
		require.Contains(t, err.Error(), "POL-AGENT-ORCH-HOURGLASS-001")
	})

	t.Run("wake valid execution", func(t *testing.T) {
		out, err := executeFeedCommand(t, tempDir, provider, "wake",
			"--agent-id", "SEAT-1",
			"--message", "Wake agent",
			"--format", "json")
		require.NoError(t, err)
		require.Contains(t, out, "success")
	})
}

func TestCycle135_WatchProcessFeedMatrix(t *testing.T) {
	tempDir, provider, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("watch missing agent-id", func(t *testing.T) {
		_, err := executeFeedCommand(t, tempDir, provider, "watch")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--agent-id is required")
	})

	t.Run("watch loop termination with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := executeFeedCommandWithContext(t, ctx, tempDir, provider, "watch",
			"--agent-id", "SEAT-WATCHER",
			"--tcp", "127.0.0.1:1")
		require.NoError(t, err)
	})

	t.Run("processFeed direct invocation with various inboxes", func(t *testing.T) {
		logger := logging.GetLoggerFromProfile("")

		// Case 1: Empty inbox
		processFeed(logger, tempDir, "SEAT-EMPTY-INBOX", "PER-COMMUNITY-SOFTWARE-ENGINEER", false)

		// Case 2: Populated inbox with standard steering message
		_, err := executeFeedCommand(t, tempDir, provider, "steer",
			"--agent-id", "SEAT-SENDER",
			"--to-agent-id", "SEAT-POPULATED",
			"--await-peer-ack",
			"--message", "Review task progress")
		require.NoError(t, err)

		processFeed(logger, tempDir, "SEAT-POPULATED", "PER-COMMUNITY-SOFTWARE-ENGINEER", false)

		// Case 3: Cursor composer attention queue branch
		_, err = executeFeedCommand(t, tempDir, provider, "steer",
			"--agent-id", "SEAT-SENDER",
			"--to-agent-id", "cursor-composer",
			"--await-peer-ack",
			"--message", "Critical guidance item")
		require.NoError(t, err)

		processFeed(logger, tempDir, "cursor-composer", "PER-COMMUNITY-SOFTWARE-ENGINEER", false)

		// Case 4: Auto-ack branch
		_, err = executeFeedCommand(t, tempDir, provider, "steer",
			"--agent-id", "SEAT-SENDER",
			"--to-agent-id", "SEAT-AUTO-ACK",
			"--await-peer-ack",
			"--message", "Message for auto ack")
		require.NoError(t, err)

		processFeed(logger, tempDir, "SEAT-AUTO-ACK", "PER-COMMUNITY-SOFTWARE-ENGINEER", true)

		// Case 5: Wake stub event prefix branch
		_, err = agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: tempDir,
			AgentID:     "SEAT-WAKER",
			ToAgentID:   "SEAT-WAKE-RECV",
			EventType:   agentfeed.FeedEventTypeWake,
			Message:     "Agent woke up",
		})
		require.NoError(t, err)

		processFeed(logger, tempDir, "SEAT-WAKE-RECV", "PER-COMMUNITY-SOFTWARE-ENGINEER", false)
	})
}

func TestCycle135_HelpersValidation(t *testing.T) {
	_, _, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("validateLiteFeedID matching and mismatching", func(t *testing.T) {
		err := validateLiteFeedID("FEED-123", "FEED-123")
		require.NoError(t, err)

		err = validateLiteFeedID("FEED-123", "")
		require.NoError(t, err)

		err = validateLiteFeedID("FEED-123", "FEED-456")
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not match --feed-id")
	})

	t.Run("validateUsablePersonaStatus matrix", func(t *testing.T) {
		require.NoError(t, validateUsablePersonaStatus(map[string]any{objects.FieldKeyStatus: "approved"}, "PER-1"))
		require.NoError(t, validateUsablePersonaStatus(map[string]any{objects.FieldKeyStatus: "active"}, "PER-1"))
		require.NoError(t, validateUsablePersonaStatus(map[string]any{objects.FieldKeyStatus: "implemented"}, "PER-1"))
		require.NoError(t, validateUsablePersonaStatus(map[string]any{objects.FieldKeyStatus: "draft"}, "PER-1"))
		require.Error(t, validateUsablePersonaStatus(map[string]any{objects.FieldKeyStatus: ""}, "PER-1"))
		require.Error(t, validateUsablePersonaStatus(map[string]any{objects.FieldKeyStatus: "archived"}, "PER-1"))
		require.Error(t, validateUsablePersonaStatus(map[string]any{objects.FieldKeyStatus: "rejected"}, "PER-1"))
	})

	t.Run("formatPeerWakeOutput detail fields", func(t *testing.T) {
		cmd := &cobra.Command{}
		res := agentfeed.AppendEventResult{
			FeedID:          "FEED-1",
			EventID:         "EVT-1",
			DeliveryMode:    datacell.DeliveryModeLog,
			InFlightRePrime: true,
		}
		wakePtr := &agentfeed.PeerWakeResult{
			Transport:       "mock-transport",
			NonInterrupting: true,
		}

		err := formatPeerWakeOutput(cmd, res, wakePtr, "AWAIT-100", 3)
		require.NoError(t, err)
	})

	t.Run("requireProjectRoot processor validation", func(t *testing.T) {
		_, err := requireProjectRoot(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "processor is nil")
	})
}

func TestCycle135_ResultAndMCPHelpers(t *testing.T) {
	tempDir, _, cleanup := setupCycle135IsolatedTestEnv(t)
	defer cleanup()

	t.Run("dispatchPeerWake non-interrupting branch", func(t *testing.T) {
		logger := logging.GetLoggerFromProfile("")
		res := agentfeed.AppendEventResult{
			EventID:         "EVT-NON-INT",
			DeliveryMode:    datacell.DeliveryModeLog,
			NonInterrupting: true,
		}

		wakePtr, subs := dispatchPeerWake(context.Background(), logger, tempDir, "Message", res, "SEAT-SENDER", "SEAT-RECEIVER", false, "test")
		require.NotNil(t, wakePtr)
		require.Equal(t, -1, subs)
	})

	t.Run("truncateRunes formatting", func(t *testing.T) {
		shortText := "Short text"
		require.Equal(t, shortText, truncateRunes(shortText, 50))

		longText := strings.Repeat("A", 150)
		truncated := truncateRunes(longText, 100)
		require.True(t, strings.HasSuffix(truncated, "…"))
		require.Equal(t, 101, len([]rune(truncated)))
	})

	t.Run("MCP daemon helper functions", func(t *testing.T) {
		tcp := resolveFeedMCPTCP("127.0.0.1:9099")
		require.Equal(t, "127.0.0.1:9099", tcp)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		logger := logging.GetLoggerFromProfile("")
		_, err := queryMCPSubscriberCount(ctx, "127.0.0.1:1", logger)
		require.Error(t, err)

		err = publishMCPDaemonEvent(ctx, "127.0.0.1:1", "test-msg", "SEAT-1", "EVT-1", logger)
		require.Error(t, err)
	})

	t.Run("NewFeedCmd subcommands wiring", func(t *testing.T) {
		cmd := NewFeedCmd()
		require.Equal(t, "feed", cmd.Use)
		require.NotEmpty(t, cmd.Commands())
	})
}
