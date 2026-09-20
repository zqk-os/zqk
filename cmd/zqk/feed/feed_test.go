package feed

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
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
