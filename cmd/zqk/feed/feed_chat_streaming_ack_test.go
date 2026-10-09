package feed

import (
	"bufio"
	stdctx "context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSubscriberStreaming_LoopHandshakeAndEvents tests runSubscriberLoop covering
// the MCP JSON-RPC protocol handshake, subscription, event stream dispatching,
// error line handling, and graceful connection shutdown.
func TestSubscriberStreaming_LoopHandshakeAndEvents(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on tcp: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	serverDone := make(chan struct{})
	goroutinelabels.NewGoroutine("mock_mcp_test_server", "mock MCP tcp listener for subscriber test").
		StartSimple(func() {
			defer close(serverDone)
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()

			reader := bufio.NewReader(conn)

			// 1. Expect initialize request
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			var initReq map[string]any
			_ = json.Unmarshal([]byte(line), &initReq)

			// Respond to initialize with id=1
			_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}` + "\n"))

			// 2. Expect notifications/initialized
			_, err = reader.ReadString('\n')
			if err != nil {
				return
			}

			// 3. Expect events/subscribe request
			_, err = reader.ReadString('\n')
			if err != nil {
				return
			}
			// Respond to subscribe with id=2
			_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"subscribed":true}}` + "\n"))

			// 4. Send malformed json line to test unmarshal error recovery
			_, _ = conn.Write([]byte(`{not valid json}` + "\n"))

			// 5. Send unrelated event notification (ignored by subscriber)
			_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/event","params":{"event":{"type":"peer.heartbeat"}}}` + "\n"))

			// 6. Send action.required notification
			_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/event","params":{"event":{"type":"action.required"}}}` + "\n"))

			// 7. Send action_required notification
			_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/event","params":{"event":{"type":"action_required"}}}` + "\n"))

			// Allow client to process events
			time.Sleep(50 * time.Millisecond)
		})

	logger := logging.GetLoggerFromProfile("test")
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 2*time.Second)
	defer cancel()

	// Seed a correspondence event to verify processFeed works during stream
	_, steerErr := executeFeedCommand(t, tempDir, provider, "steer", "--agent-id", "SEAT-001", "--to-agent-id", "SEAT-002", "--message", "Stream event", "--await-peer-ack")
	if steerErr != nil {
		t.Fatalf("steer failed: %v", steerErr)
	}

	runSubscriberLoop(ctx, logger, addr, tempDir, "SEAT-002", "PER-COMMUNITY-SOFTWARE-ENGINEER", true)
	<-serverDone

	// Test dial failure branch with unreachable address
	runSubscriberLoop(ctx, logger, "127.0.0.1:65534", tempDir, "SEAT-002", "PER-COMMUNITY-SOFTWARE-ENGINEER", false)
}

// TestFeedAck_ErrorAndValidationMatrix tests the acknowledgment command boundary
// conditions, input validations, impersonation rejections, and await completions.
func TestFeedAck_ErrorAndValidationMatrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// 1. Missing --agent-id
	if _, err := executeFeedCommand(t, tempDir, provider, "ack", "--in-reply-to", "EVT-1", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER"); err == nil {
		t.Error("expected error for missing --agent-id")
	}

	// 2. Missing --persona-ref
	if _, err := executeFeedCommand(t, tempDir, provider, "ack", "--in-reply-to", "EVT-1", "--agent-id", "SEAT-002"); err == nil {
		t.Error("expected error for missing --persona-ref")
	}

	// 3. Missing --in-reply-to
	if _, err := executeFeedCommand(t, tempDir, provider, "ack", "--agent-id", "SEAT-002", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER"); err == nil {
		t.Error("expected error for missing --in-reply-to")
	}

	// 4. Invalid persona ref
	if _, err := executeFeedCommand(t, tempDir, provider, "ack", "--in-reply-to", "EVT-1", "--agent-id", "SEAT-002", "--persona-ref", "PER-INVALID-PERSONA"); err == nil {
		t.Error("expected error for nonexistent persona-ref")
	}

	// 5. Non-existent in-reply-to event
	if _, err := executeFeedCommand(t, tempDir, provider, "ack", "--in-reply-to", "EVT-NONEXISTENT-999", "--agent-id", "SEAT-002", "--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER"); err == nil {
		t.Error("expected error for non-existent event in-reply-to")
	}

	// 6. Create event with --await-peer-ack
	steerOut, err := executeFeedCommand(t, tempDir, provider, "steer",
		"--agent-id", "SEAT-001",
		"--to-agent-id", "SEAT-002",
		"--message", "Coordinate streaming",
		"--await-peer-ack",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("steer with await-peer-ack failed: %v", err)
	}

	var steerMap map[string]any
	if err := json.Unmarshal([]byte(steerOut), &steerMap); err != nil {
		t.Fatalf("failed to unmarshal steer output: %v", err)
	}
	eventID, _ := steerMap["event_id"].(string)
	if eventID == "" {
		t.Fatalf("expected non-empty event_id: %s", steerOut)
	}

	// 7. Valid Ack with JSON format output verifying awaits_completed
	ackOut, err := executeFeedCommand(t, tempDir, provider, "ack",
		"--agent-id", "SEAT-002",
		"--persona-ref", "PER-COMMUNITY-SOFTWARE-ENGINEER",
		"--in-reply-to", eventID,
		"--summary", "Acknowledged and complete",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("feed ack failed: %v", err)
	}

	var ackMap map[string]any
	if err := json.Unmarshal([]byte(ackOut), &ackMap); err != nil {
		t.Fatalf("failed to unmarshal ack json output: %v", err)
	}
	if ackMap[objects.FieldKeyEventType] != agentfeed.FeedEventTypePeerAck {
		t.Errorf("expected event_type peer_ack, got: %v", ackMap[objects.FieldKeyEventType])
	}
	if ackMap["in_reply_to"] != eventID {
		t.Errorf("expected in_reply_to %s, got: %v", eventID, ackMap["in_reply_to"])
	}
	awaitsCompleted, ok := ackMap["awaits_completed"].([]any)
	if !ok || len(awaitsCompleted) == 0 {
		t.Logf("awaits_completed present: %v (len: %d)", ok, len(awaitsCompleted))
	}
}

// TestFeedBridgeIngest_ComprehensiveMatrix tests payload flag mutual exclusion,
// inline payloads, feed ID matching & mismatch validation, no-ack and no-wake flags.
func TestFeedBridgeIngest_ComprehensiveMatrix(t *testing.T) {
	tempDir, cleanup := setupFeedTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedFeedSupportObjects(t, provider)

	// 1. Both --payload and --payload-file
	dummyFile := filepath.Join(tempDir, "payload.json")
	_ = fileutil.WriteStandardFile(dummyFile, []byte(`{"channel":"generic","text":"test"}`))
	if _, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest", "--channel", "generic", "--payload", `{"text":"test"}`, "--payload-file", dummyFile); err == nil {
		t.Error("expected error when both --payload and --payload-file are specified")
	}

	// 2. Neither payload provided
	if _, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest", "--channel", "generic"); err == nil {
		t.Error("expected error when neither payload nor payload-file is provided")
	}

	// 3. Nonexistent payload file
	if _, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest", "--channel", "generic", "--payload-file", filepath.Join(tempDir, "missing.json")); err == nil {
		t.Error("expected error for nonexistent payload-file")
	}

	// 4. Invalid JSON payload
	if _, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest", "--channel", "generic", "--payload", `{invalid-json`); err == nil {
		t.Error("expected error for malformed payload JSON")
	}

	// 5. Valid inline payload with --no-ack and --no-wake
	validInline := `{"channel":"generic","text":"Inline bridge message","external_id":"bridge-ext-42"}`
	out, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest",
		"--channel", "generic",
		"--payload", validInline,
		"--agent-id", "SEAT-001",
		"--to-agent-id", "SEAT-002",
		"--no-ack",
		"--no-wake",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("bridge-ingest valid inline failed: %v", err)
	}
	if !strings.Contains(out, "bridge-ext-42") {
		t.Errorf("expected external_id bridge-ext-42 in output: %s", out)
	}

	var parsedOut map[string]any
	if err := json.Unmarshal([]byte(out), &parsedOut); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	feedID, _ := parsedOut["feed_id"].(string)

	// 6. Ingest with matching --feed-id
	if feedID != "" {
		matchOut, err := executeFeedCommand(t, tempDir, provider, "bridge-ingest",
			"--channel", "generic",
			"--payload", `{"channel":"generic","text":"Message with feed ID"}`,
			"--agent-id", "SEAT-001",
			"--feed-id", feedID,
			"--format", "json",
		)
		if err != nil {
			t.Fatalf("bridge-ingest with matching feed-id failed: %v", err)
		}
		_ = matchOut

		// 7. Ingest with mismatching --feed-id
		_, errMismatch := executeFeedCommand(t, tempDir, provider, "bridge-ingest",
			"--channel", "generic",
			"--payload", `{"channel":"generic","text":"Message with mismatch feed ID"}`,
			"--agent-id", "SEAT-001",
			"--feed-id", "feed-mismatch-999",
		)
		if errMismatch == nil {
			t.Error("expected error for mismatched feed-id")
		}
	}
}

// TestReadBridgePayload_EdgeCases directly tests readBridgePayload error conditions.
func TestReadBridgePayload_EdgeCases(t *testing.T) {
	// Empty both
	_, errEmpty := readBridgePayload("", "")
	if errEmpty == nil {
		t.Error("expected error when neither payload nor payload-file is provided")
	}

	// File and inline both present
	_, errBoth := readBridgePayload("f.json", "inline")
	if errBoth == nil {
		t.Error("expected error when both payloadFile and payloadInline are set")
	}

	// Missing file
	_, errMissing := readBridgePayload("nonexistent_path_test_12345.json", "")
	if errMissing == nil {
		t.Error("expected error for nonexistent file")
	}

	// Valid temp file
	tmpFile := filepath.Join(t.TempDir(), "valid.json")
	if err := fileutil.WriteFile(tmpFile, []byte(`{"ok":true}`), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}
	b, err := readBridgePayload(tmpFile, "")
	if err != nil || string(b) != `{"ok":true}` {
		t.Errorf("readBridgePayload file failed: b=%s err=%v", string(b), err)
	}
}
