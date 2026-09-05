package swarminit

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/agentfeed"
)

func TestBindSeats_trajectoryNotFoundFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	env := &Env{
		ProjectRoot: root,
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"peer-agent-1": {
					PID:          os.Getpid(),
					Conversation: "dead-uuid",
					Wake:         agentfeed.WakeMembraneAgentAPI,
				},
			}}, nil
		},
		ProbeConversation: func(_ context.Context, _ string, rec agentfeed.PeerSeatRecord) error {
			return errfmtWrap("trajectory not found: " + rec.Conversation)
		},
	}
	res, err := execBindSeats(context.Background(), env, Stage{
		ID:       "bind_seats",
		Executor: ExecutorBindSeats,
		Pass:     map[string]any{"conversation_live": true},
	})
	if err == nil || !strings.Contains(err.Error(), "trajectory not found") {
		t.Fatalf("want trajectory not found, got res=%+v err=%v", res, err)
	}
	if res.OK {
		t.Fatal("expected fail")
	}
}

func TestBindSeats_liveMetadataPasses(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	probed := false
	env := &Env{
		ProjectRoot: root,
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"peer-agent-1": {
					PID:          os.Getpid(),
					Conversation: "live-uuid",
					Wake:         agentfeed.WakeMembraneAgentAPI,
				},
			}}, nil
		},
		ProbeConversation: func(_ context.Context, seatID string, rec agentfeed.PeerSeatRecord) error {
			probed = true
			if seatID != "peer-agent-1" || rec.Conversation != "live-uuid" {
				t.Fatalf("probe %s %+v", seatID, rec)
			}
			return nil
		},
	}
	res, err := execBindSeats(context.Background(), env, Stage{
		Executor: ExecutorBindSeats,
		Pass:     map[string]any{"conversation_live": true},
	})
	if err != nil || !res.OK {
		t.Fatalf("want pass, got res=%+v err=%v", res, err)
	}
	if !probed {
		t.Fatal("probe not called")
	}
}

func TestBindSeats_refreshSeatsForbidden(t *testing.T) {
	t.Parallel()
	_, err := execBindSeats(context.Background(), &Env{ProjectRoot: t.TempDir()}, Stage{
		Executor: ExecutorBindSeats,
		Config:   map[string]any{"refresh_seats": true},
	})
	if err == nil || !strings.Contains(err.Error(), "refresh-seats") {
		t.Fatalf("want forbid refresh-seats, got %v", err)
	}
}

func TestOrchestratePlan_bodyIsOrchestratePlan(t *testing.T) {
	t.Parallel()
	var got string
	env := &Env{
		ProjectRoot: t.TempDir(),
		PlanID:      "PRI-MESH-SWARM-INIT-001",
		Steer: func(_ context.Context, to, msg string, await bool) (SteerOutcome, error) {
			got = msg
			if to != "peer-agent-1" || !await {
				t.Fatalf("to=%s await=%v", to, await)
			}
			return SteerOutcome{EventID: "AFE-1", AwaitID: "AWA-1", ToAgentID: to}, nil
		},
	}
	res, err := execOrchestratePlan(context.Background(), env, Stage{
		Executor: ExecutorOrchestratePlan,
		Config:   map[string]any{"to_agent_id": "peer-agent-1"},
	})
	if err != nil || !res.OK {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !agentfeed.IsOrchestratePlanBody(got) {
		t.Fatalf("body %q is not ORCHESTRATE_PLAN", got)
	}
}

func TestChatBootstrap_skipsWithoutAllowChat(t *testing.T) {
	t.Parallel()
	res, err := execChatBootstrap(context.Background(), &Env{ProjectRoot: t.TempDir(), AllowChat: false}, Stage{
		Executor: ExecutorChatBootstrap,
		Optional: true,
	})
	if err != nil || !res.Skipped {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestCommsCheck_steersNonce(t *testing.T) {
	t.Parallel()
	var msgs []string
	env := &Env{
		ProjectRoot:        t.TempDir(),
		CoordinatorAgentID: "cursor-composer",
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"peer-agent-1": {Wake: agentfeed.WakeMembraneAgentAPI, PID: 1, Conversation: "c"},
			}}, nil
		},
		Steer: func(_ context.Context, to, msg string, await bool) (SteerOutcome, error) {
			if to != "peer-agent-1" || !await {
				t.Fatalf("to=%s await=%v", to, await)
			}
			msgs = append(msgs, msg)
			return SteerOutcome{EventID: "AFE-c", ToAgentID: to}, nil
		},
	}
	res, err := execCommsCheck(context.Background(), env, Stage{
		Executor: ExecutorCommsCheck,
		Config:   map[string]any{"nonce": "CC-TEST"},
	})
	if err != nil || !res.OK {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(msgs) != 1 || !strings.Contains(msgs[0], agentfeed.CommsCheckPrefix+" CC-TEST") {
		t.Fatalf("msgs=%v", msgs)
	}
}

func errfmtWrap(s string) error { return errString(s) }

type errString string

func (e errString) Error() string { return string(e) }
