package feed

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/datacell"
)

func TestNewFeedCmd_wiresSteerAndEmitStatus(t *testing.T) {
	cmd := NewFeedCmd()
	if cmd.Use != "feed" {
		t.Fatalf("use=%q", cmd.Use)
	}
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	if !names["steer"] || !names["emit-status"] || !names["proof-of-life"] || !names["ack"] || !names["pending"] || !names["bridge-ingest"] || !names["serve"] || !names["doctor"] || !names["watch"] {
		t.Fatalf("subcommands=%v", names)
	}
	doctor := NewDoctorCmd()
	if doctor.Use == "" || doctor.RunE == nil {
		t.Fatal("doctor must wire Use + RunE from DNA builder")
	}
	watch := NewWatchCmd()
	if watch.Flags().Lookup("tcp") == nil {
		t.Fatal("watch missing --tcp")
	}
	if watch.Flags().Lookup("agent-id") == nil {
		t.Fatal("watch missing --agent-id")
	}
	if watch.RunE == nil {
		t.Fatal("watch must wire RunE")
	}
	serve := bldr_cli_cmd_v1.NewServeCommandBuilder()
	if serve.Flags().Lookup("listen") == nil || serve.Flags().Lookup("token") == nil {
		t.Fatal("serve missing --listen / --token")
	}
	steer := bldr_cli_cmd_v1.NewSteerCommandBuilder()
	if steer.Flags().Lookup("message") == nil {
		t.Fatal("steer missing --message")
	}
	if steer.Flags().Lookup("verbose") == nil {
		t.Fatal("steer must expose --verbose (POL-CODE-015; FormatOutput detail)")
	}
	if steer.Flags().Lookup("format") == nil {
		t.Fatal("steer must expose --format (POL-CODE-015)")
	}
	if steer.Flags().Lookup("non-interrupting") == nil {
		t.Fatal("steer missing --non-interrupting")
	}
	if steer.Flags().Lookup("reprime") == nil {
		t.Fatal("steer missing --reprime")
	}
	agentID := steer.Flags().Lookup("agent-id")
	if agentID == nil {
		t.Fatal("steer missing --agent-id")
	}
	if strings.Contains(agentID.Usage, "default") {
		t.Fatalf("agent-id usage must not duplicate default: %q", agentID.Usage)
	}
	if steer.Flags().Lookup("to-agent-id") == nil {
		t.Fatal("steer missing --to-agent-id")
	}
	emit := bldr_cli_cmd_v1.NewFeedEmitStatusCommandBuilder()
	if emit.Flags().Lookup("persona-ref") == nil || emit.Flags().Lookup("summary") == nil {
		t.Fatal("emit-status missing flags")
	}
	if emit.Flags().Lookup("agent-id") == nil {
		t.Fatal("emit-status must require --agent-id for swarm disambiguation")
	}
	if emit.Flags().Lookup("role") != nil {
		t.Fatal("emit-status must not invent --role; use --persona-ref")
	}
	if emit.Flags().Lookup("verbose") == nil {
		t.Fatal("emit-status must expose --verbose")
	}
	if emit.Flags().Lookup("pulse-human") == nil {
		t.Fatal("emit-status missing --pulse-human")
	}
	pol := bldr_cli_cmd_v1.NewFeedProofOfLifeCommandBuilder()
	if pol.Flags().Lookup("agent-id") == nil || pol.Flags().Lookup("persona-ref") == nil {
		t.Fatal("proof-of-life missing identity flags")
	}
	ack := bldr_cli_cmd_v1.NewAckCommandBuilder()
	if ack.Flags().Lookup("in-reply-to") == nil {
		t.Fatal("ack missing --in-reply-to")
	}
	pending := bldr_cli_cmd_v1.NewPendingCommandBuilder()
	if pending.Flags().Lookup("agent-id") == nil {
		t.Fatal("pending missing --agent-id")
	}
	ingest := bldr_cli_cmd_v1.NewBridgeIngestCommandBuilder()
	if ingest.Flags().Lookup("channel") == nil || ingest.Flags().Lookup("payload-file") == nil {
		t.Fatal("bridge-ingest missing --channel / --payload-file")
	}
	if ingest.Flags().Lookup("verbose") == nil || ingest.Flags().Lookup("format") == nil {
		t.Fatal("bridge-ingest must expose --verbose and --format")
	}
}

func TestFeedSteer_NonInterruptingAndReprime(t *testing.T) {
	tempDir, provider, cleanup := setupFeedTestProjectWithStorage(t)
	defer cleanup()
	seedFeedSupportObjects(t, provider)

	out, err := executeFeedCommand(
		t,
		tempDir,
		provider,
		"steer",
		"--agent-id", "SEAT-001",
		"--to-agent-id", "SEAT-002",
		"--message", "Focus on boundary verification and race safety",
		"--await-peer-ack",
		"--non-interrupting",
		"--reprime",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("feed steer --non-interrupting --reprime failed: %v, out: %s", err, out)
	}

	var steerMap map[string]any
	if err := json.Unmarshal([]byte(out), &steerMap); err != nil {
		t.Fatalf("failed to unmarshal steer output: %v, raw: %s", err, out)
	}

	eventID, _ := steerMap["event_id"].(string)
	if eventID == "" {
		t.Fatalf("expected non-empty event_id in output: %v", steerMap)
	}
	if nonInt, ok := steerMap["non_interrupting"].(bool); !ok || !nonInt {
		t.Fatalf("expected non_interrupting=true in steer output: %v", steerMap)
	}
	if reprime, ok := steerMap["in_flight_reprime"].(bool); !ok || !reprime {
		t.Fatalf("expected in_flight_reprime=true in steer output: %v", steerMap)
	}
	if transport, _ := steerMap["peer_wake_transport"].(string); transport != agentfeed.TransportNonInterrupting {
		t.Fatalf("expected peer_wake_transport=%q, got %q", agentfeed.TransportNonInterrupting, transport)
	}

	if pw, _ := steerMap["peer_wake"].(string); pw != "attempted" {
		t.Fatalf("expected peer_wake=attempted, got: %v", steerMap["peer_wake"])
	}
	if live, _ := steerMap["peer_wake_live"].(bool); !live {
		t.Fatalf("expected peer_wake_live=true, got: %v", steerMap["peer_wake_live"])
	}
	if dr, _ := steerMap["delivery_receipt"].(bool); !dr {
		t.Fatalf("expected delivery_receipt=true, got: %v", steerMap["delivery_receipt"])
	}

	// Verify in-flight context re-priming on receiving agent
	baseContext := "Active task in progress: implement verification gates."
	reprimeRes, err := agentfeed.RePrimeAgentContext(agentfeed.RePrimeInput{
		ProjectRoot:    tempDir,
		AgentID:        "SEAT-002",
		PersonaRef:     "PER-COMMUNITY-SOFTWARE-ENGINEER",
		CurrentContext: baseContext,
		AutoAck:        true,
	})
	if err != nil {
		t.Fatalf("RePrimeAgentContext failed: %v", err)
	}
	if !reprimeRes.Applied {
		t.Fatal("expected reprimeRes.Applied = true")
	}
	if !strings.Contains(reprimeRes.RePrimedContext, baseContext) {
		t.Fatalf("expected base context preserved in: %s", reprimeRes.RePrimedContext)
	}
	if !strings.Contains(reprimeRes.RePrimedContext, "Focus on boundary verification and race safety") {
		t.Fatalf("expected in-flight steering guidance in reprimed context: %s", reprimeRes.RePrimedContext)
	}
	if len(reprimeRes.AckedEventIDs) != 1 || reprimeRes.AckedEventIDs[0] != eventID {
		t.Fatalf("expected event %s acked, got: %v", eventID, reprimeRes.AckedEventIDs)
	}

	// Verify persistent in-flight state alignment
	state, err := agentfeed.LoadAgentInFlightState(tempDir, "SEAT-002")
	if err != nil {
		t.Fatalf("LoadAgentInFlightState failed: %v", err)
	}
	if state.Status != agentfeed.InFlightStatusAligned {
		t.Fatalf("expected state status=%q, got %q", agentfeed.InFlightStatusAligned, state.Status)
	}
	if len(state.ActiveGuidance) != 1 {
		t.Fatalf("expected 1 active guidance item in persistent state, got %d", len(state.ActiveGuidance))
	}
}

func TestFeedSteer_NonInterrupting_Boundary(t *testing.T) {
	tempDir, provider, cleanup := setupFeedTestProjectWithStorage(t)
	defer cleanup()
	seedFeedSupportObjects(t, provider)

	// Explicitly configure chat channel with known feed_id
	cfg := datacell.AgentChatChannelConfig{
		Enabled:      true,
		DeliveryMode: datacell.DeliveryModeNotify,
		FeedID:       "TEST-FEED-LITE-01",
	}
	if err := datacell.WriteAgentChatChannelConfig(tempDir, cfg); err != nil {
		t.Fatalf("failed to write channel config: %v", err)
	}

	// Missing --message
	if _, err := executeFeedCommand(t, tempDir, provider, "steer", "--non-interrupting"); err == nil {
		t.Fatal("expected error for steer without --message")
	}

	// Directed steer without --await-peer-ack must fail closed (POL-AGENT-ORCH-HOURGLASS-001)
	if _, err := executeFeedCommand(t, tempDir, provider, "steer", "--agent-id", "SEAT-001", "--to-agent-id", "SEAT-002", "--message", "Boundary test", "--non-interrupting"); err == nil {
		t.Fatal("expected error for directed steer without --await-peer-ack")
	}

	// Mismatched feed-id
	if _, err := executeFeedCommand(t, tempDir, provider, "steer", "--agent-id", "SEAT-001", "--message", "Test", "--feed-id", "WRONG-FEED-ID", "--non-interrupting"); err == nil {
		t.Fatal("expected error for mismatched feed-id")
	}
}
