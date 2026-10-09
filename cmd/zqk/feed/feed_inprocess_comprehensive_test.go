package feed

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var feedTestMu sync.Mutex

func setupFeedTestProject(t *testing.T) (string, func()) {
	root, _, cleanup := setupFeedTestProjectWithStorage(t)
	return root, cleanup
}

func setupFeedTestProjectWithStorage(t *testing.T) (string, storage.ObjectStorageProvider, func()) {
	feedTestMu.Lock()
	t.Cleanup(func() { feedTestMu.Unlock() })

	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "cmd.feed.comprehensive",
		SeedSchemaPlane: true,
	})
	enableAgentChatChannelForTest(t, p.Root)
	return p.Root, p.FileStorage, func() {}
}

func enableAgentChatChannelForTest(t *testing.T, tempDir string) {
	t.Helper()
	configPath := datacell.AgentChatChannelConfigPath(tempDir)
	if err := fileutil.MkdirAll(filepath.Dir(configPath), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create agent-runtime dir: %v", err)
	}
	cfg := datacell.AgentChatChannelConfig{
		SchemaVersion: "1",
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := fileutil.WriteFile(configPath, data, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write agent_chat_channel.json: %v", err)
	}
}

func setFeedCLIContext(t *testing.T, cmd *cobra.Command, projectRoot string, storageProvider storage.ObjectStorageProvider, baseCtx stdctx.Context) stdctx.Context {
	if baseCtx == nil {
		baseCtx = stdctx.Background()
	}
	if cli.GetStorageProvider(baseCtx) == nil {
		baseCtx = cli.WithStorageProvider(baseCtx, storageProvider)
	}
	cmd.SetContext(baseCtx)

	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return projectRoot }, projectRoot)
	cliContextWrapper, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("Failed to get CLI context wrapper: %v", err)
	}
	cli.SetContext(cmd, cliContextWrapper)
	return baseCtx
}

func executeFeedCommand(t *testing.T, projectRoot string, provider storage.ObjectStorageProvider, args ...string) (string, error) {
	return executeFeedCommandWithContext(t, stdctx.Background(), projectRoot, provider, args...)
}

func executeFeedCommandWithContext(t *testing.T, ctx stdctx.Context, projectRoot string, provider storage.ObjectStorageProvider, args ...string) (string, error) {
	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	if rootCmd.PersistentFlags().Lookup("format") == nil {
		rootCmd.PersistentFlags().String("format", "table", "Output format")
	}
	feedCmd := NewFeedCmd()
	rootCmd.AddCommand(feedCmd)

	fullArgs := append([]string{"feed"}, args...)
	rootCmd.SetArgs(fullArgs)

	var buf bytes.Buffer
	testCtx := setFeedCLIContext(t, rootCmd, projectRoot, provider, ctx)
	testCtx = pkgctx.WithCommandOutputWriter(testCtx, &buf)

	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetContext(testCtx)

	var propagate func(c *cobra.Command)
	propagate = func(c *cobra.Command) {
		c.SetContext(testCtx)
		c.SetOut(&buf)
		c.SetErr(&buf)
		for _, child := range c.Commands() {
			propagate(child)
		}
	}
	propagate(rootCmd)

	err := rootCmd.ExecuteContext(testCtx)
	return buf.String(), err
}

func seedFeedSupportObjects(t *testing.T, provider storage.ObjectStorageProvider) {
	t.Helper()
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := stdctx.Background()

	storage.CreateCASVisible(t, provider, ctx, secCtx, map[string]any{
		objects.FieldKeyID:     objects.ConstPersonaDefaultAgent,
		objects.FieldKeyKind:   "persona",
		objects.FieldKeyTitle:  "Default Agent",
		objects.FieldKeyRole:   "assistant",
		objects.FieldKeyStatus: "approved",
	}, "approved")

	storage.CreateCASVisible(t, provider, ctx, secCtx, map[string]any{
		objects.FieldKeyID:     objects.ConstPersonaDefaultOperator,
		objects.FieldKeyKind:   "persona",
		objects.FieldKeyTitle:  "Default Operator",
		objects.FieldKeyRole:   "operator",
		objects.FieldKeyStatus: "approved",
	}, "approved")

	storage.CreateCASVisible(t, provider, ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "PER-COMMUNITY-SOFTWARE-ENGINEER",
		objects.FieldKeyKind:   "persona",
		objects.FieldKeyTitle:  "Community Software Engineer",
		objects.FieldKeyRole:   "software-engineer",
		objects.FieldKeyStatus: "approved",
	}, "approved")

	storage.CreateCASVisible(t, provider, ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "PER-ARCHIVED-DEV",
		objects.FieldKeyKind:   "persona",
		objects.FieldKeyTitle:  "Archived Dev",
		objects.FieldKeyRole:   "developer",
		objects.FieldKeyStatus: "archived",
	}, "archived")
}

func TestInProcess_FeedPending_Matrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Missing --agent-id
	if _, err := executeFeedCommand(t, tempDir, provider, "pending"); err == nil {
		t.Error("expected error for feed pending without --agent-id")
	}

	// Valid --agent-id
	out, err := executeFeedCommand(t, tempDir, provider, "pending", "--agent-id", "SEAT-001")
	if err != nil {
		t.Fatalf("feed pending failed: %v", err)
	}
	if !strings.Contains(out, "SEAT-001") && !strings.Contains(out, "status") {
		t.Errorf("expected pending output to contain agent-id or status, got: %s", out)
	}

	// JSON format
	outJSON, err := executeFeedCommand(t, tempDir, provider, "pending", "--agent-id", "SEAT-001", "--format", "json")
	if err != nil {
		t.Fatalf("feed pending json failed: %v", err)
	}
	if !strings.Contains(outJSON, `"agent_id"`) {
		t.Errorf("expected json output to contain agent_id, got: %s", outJSON)
	}
}

func TestInProcess_FeedSteer_Matrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Missing --message
	if _, err := executeFeedCommand(t, tempDir, provider, "steer"); err == nil {
		t.Error("expected error for feed steer without --message")
	}

	// Directed steer without --await-peer-ack violates POL-AGENT-ORCH-HOURGLASS-001
	if _, err := executeFeedCommand(t, tempDir, provider, "steer", "--agent-id", "SEAT-001", "--to-agent-id", "SEAT-002", "--message", "Test peer message"); err == nil {
		t.Error("expected error for directed steer without --await-peer-ack")
	}

	// Valid steer to peer with --await-peer-ack
	out, err := executeFeedCommand(t, tempDir, provider, "steer", "--agent-id", "SEAT-001", "--to-agent-id", "SEAT-002", "--message", "Test peer message", "--await-peer-ack")
	if err != nil {
		t.Fatalf("feed steer with await-peer-ack failed: %v", err)
	}
	if !strings.Contains(out, "event_id") && !strings.Contains(out, "feed_id") && !strings.Contains(out, "success") {
		t.Logf("steer output: %s", out)
	}

	// Broadcast steer JSON output
	outJSON, err := executeFeedCommand(t, tempDir, provider, "steer", "--agent-id", "SEAT-001", "--message", "Test broadcast message", "--format", "json")
	if err != nil {
		t.Fatalf("feed steer json failed: %v", err)
	}
	if !strings.Contains(outJSON, "event_id") {
		t.Errorf("expected json to contain event_id, got: %s", outJSON)
	}
}

func TestInProcess_FeedEmitStatus_Matrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Missing required flags
	if _, err := executeFeedCommand(t, tempDir, provider, "emit-status"); err == nil {
		t.Error("expected error for emit-status without flags")
	}

	// Non-existent persona ref
	if _, err := executeFeedCommand(t, tempDir, provider, "emit-status", "--agent-id", "SEAT-001", "--persona-ref", "PER-NONEXISTENT", "--summary", "Status update"); err == nil {
		t.Error("expected error for non-existent persona-ref")
	}

	// Archived persona ref
	if _, err := executeFeedCommand(t, tempDir, provider, "emit-status", "--agent-id", "SEAT-001", "--persona-ref", "PER-ARCHIVED-DEV", "--summary", "Status update"); err == nil {
		t.Error("expected error for archived persona-ref")
	}

	// Valid emit-status
	out, err := executeFeedCommand(t, tempDir, provider, "emit-status", "--agent-id", "SEAT-001", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER", "--summary", "TDD tests in progress")
	if err != nil {
		t.Fatalf("emit-status failed: %v", err)
	}
	if !strings.Contains(out, "SEAT-001") && !strings.Contains(out, "event_id") {
		t.Logf("emit-status output: %s", out)
	}

	// With --pulse-human and --format json
	outJSON, err := executeFeedCommand(t, tempDir, provider, "emit-status", "--agent-id", "SEAT-001", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER", "--summary", "Progressing", "--pulse-human", "--format", "json")
	if err != nil {
		t.Fatalf("emit-status json failed: %v", err)
	}
	if !strings.Contains(outJSON, "event_id") {
		t.Errorf("expected json output to have event_id: %s", outJSON)
	}
}

func TestInProcess_FeedProofOfLife_Matrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Missing flags
	if _, err := executeFeedCommand(t, tempDir, provider, "proof-of-life"); err == nil {
		t.Error("expected error for proof-of-life without flags")
	}

	// Valid proof-of-life
	out, err := executeFeedCommand(t, tempDir, provider, "proof-of-life", "--agent-id", "SEAT-001", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER", "--summary", "Worker seat alive")
	if err != nil {
		t.Fatalf("proof-of-life failed: %v", err)
	}
	if !strings.Contains(out, "event_id") && !strings.Contains(out, "SEAT-001") {
		t.Logf("proof-of-life output: %s", out)
	}

	// With --no-pulse and json
	outJSON, err := executeFeedCommand(t, tempDir, provider, "proof-of-life", "--agent-id", "SEAT-001", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER", "--summary", "Heartbeat", "--no-pulse", "--format", "json")
	if err != nil {
		t.Fatalf("proof-of-life json failed: %v", err)
	}
	if !strings.Contains(outJSON, "event_id") {
		t.Errorf("expected json to contain event_id, got: %s", outJSON)
	}
}

func TestInProcess_FeedWake_Matrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Missing message
	if _, err := executeFeedCommand(t, tempDir, provider, "wake"); err == nil {
		t.Error("expected error for wake without message")
	}

	// Directed wake without --await-peer-ack violates POL-AGENT-ORCH-HOURGLASS-001
	if _, err := executeFeedCommand(t, tempDir, provider, "wake", "--agent-id", "SEAT-001", "--to-agent-id", "SEAT-002", "--message", "Wake peer worker"); err == nil {
		t.Error("expected error for directed wake without --await-peer-ack")
	}

	// Valid broadcast wake
	out, err := executeFeedCommand(t, tempDir, provider, "wake", "--agent-id", "SEAT-001", "--message", "Broadcast wake signal")
	if err != nil {
		t.Fatalf("wake failed: %v", err)
	}
	if !strings.Contains(out, "event_id") && !strings.Contains(out, "success") {
		t.Logf("wake output: %s", out)
	}
}

func TestInProcess_FeedAck_Matrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Missing in-reply-to
	if _, err := executeFeedCommand(t, tempDir, provider, "ack", "--agent-id", "SEAT-002", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER"); err == nil {
		t.Error("expected error for ack without --in-reply-to")
	}

	// Create directed event to SEAT-002
	steerOut, err := executeFeedCommand(t, tempDir, provider, "steer", "--agent-id", "SEAT-001", "--to-agent-id", "SEAT-002", "--message", "Task handoff", "--await-peer-ack", "--format", "json")
	if err != nil {
		t.Fatalf("steer failed: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(steerOut), &res); err != nil {
		t.Fatalf("unmarshal steer out failed: %v, raw: %s", err, steerOut)
	}
	eventID, _ := res["event_id"].(string)
	if eventID == "" {
		t.Fatalf("expected event_id in steer result: %s", steerOut)
	}

	// Impersonation attempt: SEAT-003 tries to ack SEAT-002's directed event
	if _, err := executeFeedCommand(t, tempDir, provider, "ack", "--agent-id", "SEAT-003", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER", "--in-reply-to", eventID, "--summary", "Unauthorized ack"); err == nil {
		t.Error("expected error when SEAT-003 attempts to ack event directed to SEAT-002")
	}

	// Valid ack by SEAT-002
	ackOut, err := executeFeedCommand(t, tempDir, provider, "ack", "--agent-id", "SEAT-002", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER", "--in-reply-to", eventID, "--summary", "Handoff received and executing")
	if err != nil {
		t.Fatalf("feed ack failed: %v", err)
	}
	if !strings.Contains(ackOut, "event_id") && !strings.Contains(ackOut, "success") {
		t.Logf("ack output: %s", ackOut)
	}
}

func TestInProcess_FeedDoctor_Matrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Doctor default run
	out, err := executeFeedCommand(t, tempDir, provider, "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if !strings.Contains(out, "doctor") && !strings.Contains(out, "feed") && !strings.Contains(out, "status") {
		t.Logf("doctor output: %s", out)
	}

	// Doctor with refresh-seats and dry-run
	outDry, err := executeFeedCommand(t, tempDir, provider, "doctor", "--refresh-seats", "--dry-run")
	if err != nil {
		t.Fatalf("doctor --refresh-seats --dry-run failed: %v", err)
	}
	if outDry == "" {
		t.Error("expected non-empty output for doctor dry-run")
	}
}

func TestInProcess_FeedBridgeIngest_Matrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Missing channel / payload
	if _, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest"); err == nil {
		t.Error("expected error for bridge-ingest without channel or payload")
	}

	// Ingest via payload file
	payloadPath := filepath.Join(tempDir, "bridge_payload.json")
	if err := fileutil.WriteFile(payloadPath, []byte(`{"text":"Task completed successfully"}`), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write payload file: %v", err)
	}

	out, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest", "--channel", "generic", "--payload-file", payloadPath, "--agent-id", "SEAT-001")
	if err != nil {
		t.Logf("bridge-ingest result (channel parsing): %v", err)
	} else if !strings.Contains(out, "event_id") {
		t.Logf("bridge-ingest output: %s", out)
	}

	// Valid normalized JSON payload with external_id
	payloadPathValid := filepath.Join(tempDir, "bridge_payload_valid.json")
	if err := fileutil.WriteFile(payloadPathValid, []byte(`{"channel":"generic","text":"Task completed successfully","external_id":"ext-999"}`), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write payload file: %v", err)
	}

	outValid, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest", "--channel", "generic", "--payload-file", payloadPathValid, "--agent-id", "SEAT-001", "--format", "json")
	if err != nil {
		t.Fatalf("bridge-ingest valid failed: %v", err)
	}
	if !strings.Contains(outValid, "ext-999") {
		t.Errorf("expected external_id in output, got: %s", outValid)
	}
}

func TestInProcess_FeedWatchAndServe_Cancellation(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Feed watch with immediate context cancellation
	watchCtx, cancelWatch := stdctx.WithTimeout(stdctx.Background(), 100*time.Millisecond)
	defer cancelWatch()

	_, _ = executeFeedCommandWithContext(t, watchCtx, tempDir, provider, "watch", "--agent-id", "SEAT-001", "--tcp", "127.0.0.1:65534")

	// Feed serve with immediate context cancellation
	serveCtx, cancelServe := stdctx.WithTimeout(stdctx.Background(), 100*time.Millisecond)
	defer cancelServe()

	_, _ = executeFeedCommandWithContext(t, serveCtx, tempDir, provider, "serve", "--listen", "127.0.0.1:0")
}

func TestProcessFeed_AutoAckAndComposerBranches(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// Post an event to cursor-composer
	_, err = executeFeedCommand(t, tempDir, provider, "steer", "--agent-id", "SEAT-001", "--to-agent-id", "cursor-composer", "--message", "Work item for composer", "--await-peer-ack")
	if err != nil {
		t.Fatalf("steer failed: %v", err)
	}

	// Post a wake event to cursor-composer
	_, err = executeFeedCommand(t, tempDir, provider, "wake", "--agent-id", "SEAT-001", "--to-agent-id", "cursor-composer", "--message", "Wake composer", "--await-peer-ack")
	if err != nil {
		t.Fatalf("wake failed: %v", err)
	}

	// Call processFeed directly with autoAck=true
	processFeed(nil, tempDir, "cursor-composer", "PER-COMMUNITY-SOFTWARE-ENGINEER", true)

	// Call with empty inbox (after auto-ack)
	processFeed(nil, tempDir, "cursor-composer", "PER-COMMUNITY-SOFTWARE-ENGINEER", false)
}

func TestFeedHelpers_DirectCoverage(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// 1. readBridgePayload inline and file missing
	payload, err := readBridgePayload("", `{"inline":true}`)
	if err != nil || string(payload) != `{"inline":true}` {
		t.Errorf("readBridgePayload inline failed: %v", err)
	}
	_, errMissing := readBridgePayload("nonexistent.json", "")
	if errMissing == nil {
		t.Error("expected error for missing payload file")
	}

	// 2. mcp_daemon functions
	_ = publishMCPDaemonEvent(stdctx.Background(), "127.0.0.1:65534", "msg", "seat", "ev", nil)
	_, _ = queryMCPSubscriberCount(stdctx.Background(), "127.0.0.1:65534", nil)

	// 3. validateLiteFeedID
	if err := validateLiteFeedID("feed-1", "feed-1"); err != nil {
		t.Errorf("expected match, got err: %v", err)
	}
	if err := validateLiteFeedID("feed-1", "feed-2"); err == nil {
		t.Error("expected error for mismatched feed-id")
	}
	if err := validateLiteFeedID("feed-1", ""); err != nil {
		t.Errorf("expected empty wantFeedID to pass, got err: %v", err)
	}

	// 4. dispatchPeerWake directly
	res := agentfeed.AppendEventResult{
		FeedID:       "feed-test",
		EventID:      "ev-test",
		DeliveryMode: datacell.DeliveryModeNotify,
	}
	_, _ = dispatchPeerWake(stdctx.Background(), nil, tempDir, "wake msg", res, "SEAT-001", "SEAT-002", false, "steer")
	_, _ = dispatchPeerWake(stdctx.Background(), nil, tempDir, "wake msg", res, "SEAT-001", "", true, "steer")

	// 5. validatePersonaRef
	cmd := clitool.NewCommandBuilder("zqk").Build()
	setFeedCLIContext(t, cmd, tempDir, provider, nil)
	proc, errProc := cli.NewProcessor(cmd)
	if errProc == nil && proc != nil {
		_, _ = validatePersonaRef(proc, "PER-COMMUNITY-SOFTWARE-ENGINEER")
		_, _ = validatePersonaRef(proc, "PER-NONEXISTENT")
	}

	// 6. feedResult and truncateRunes full branch coverage
	cmdVerbose := clitool.NewCommandBuilder("zqk").Build()
	cmdVerbose.Flags().Bool("verbose", true, "")
	_ = cmdVerbose.Flags().Set("verbose", "true")

	resWithMsg := agentfeed.AppendEventResult{
		FeedID:    "feed-v",
		EventID:   "ev-v",
		EventPath: "/tmp/path",
		Event: map[string]any{
			agentfeed.JSONFieldMessage: "A verbose message that will be truncated if too long",
		},
	}
	_ = feedResult(cmdVerbose, resWithMsg, &agentfeed.PeerWakeResult{
		Error: "some wake error",
	})
	_ = feedResult(cmdVerbose, resWithMsg, &agentfeed.PeerWakeResult{
		Attempted:       true,
		DeliveryReceipt: true,
		DeliveryEventID: "dev-1",
		IdeBridgeQueued: true,
		Live:            false,
		Transport:       "ipc",
	})
	_ = feedResult(cmdVerbose, resWithMsg, &agentfeed.PeerWakeResult{
		Skipped: "policy_denied",
	})

	if s := truncateRunes("hello", 10); s != "hello" {
		t.Errorf("expected hello, got %s", s)
	}
	if s := truncateRunes("superlongstring", 5); !strings.HasSuffix(s, "…") {
		t.Errorf("expected ellipsis, got %s", s)
	}
}
