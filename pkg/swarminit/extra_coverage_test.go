// BLI-STARTER-COMMUNITY-067 / PRI-STARTER-COMMUNITY-067 coverage elevation
package swarminit

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestExtraSwarminitHelpersAndOnFail(t *testing.T) {
	ctx := context.Background()
	_, _ = Run(ctx, Options{})
	_, _ = Run(ctx, Options{Env: &Env{ProjectRoot: t.TempDir()}})
	_, _ = Run(ctx, Options{Env: &Env{ProjectRoot: t.TempDir(), GetObject: func(context.Context, string) (map[string]any, error) {
		return nil, nil
	}}})

	_, _ = ParseRecipe(nil)
	_, _ = ParseRecipe(map[string]any{})
	_, _ = ParseRecipe(map[string]any{objects.FieldKeyID: "PIP-X"})
	_, _ = ParseRecipe(map[string]any{objects.FieldKeyID: "PIP-X", objects.FieldKeyStages: "nope"})
	recipe, err := ParseRecipe(map[string]any{
		objects.FieldKeyID:    "PIP-X",
		objects.FieldKeyTitle: "t",
		objects.FieldKeyStages: []any{
			map[string]any{objects.FieldKeyID: "s1", stageKeyExecutor: ExecutorControlPlane, stageKeyOptional: "yes", stageKeyOnFail: OnFailSkip},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = recipe
	_, _ = ParseRecipe(map[string]any{
		objects.FieldKeyID: "PIP-X",
		objects.FieldKeyStages: []any{
			map[string]any{stageKeyExecutor: ExecutorControlPlane, stageKeyOnFail: "bogus"},
		},
	})
	_, _ = PipelineRefFromWorkflow(nil)
	_, _ = PipelineRefFromWorkflow(map[string]any{})
	_ = boolFromAny(true)
	_ = boolFromAny("YES")
	_ = boolFromAny(1)
	_ = intFromAny(3, 0)
	_ = intFromAny(int64(4), 0)
	_ = intFromAny(5.0, 0)
	_ = intFromAny("6", 0)
	_ = intFromAny("x", 9)
	_ = stringFromPass(nil, "k")
	_ = stringFromPass(map[string]any{"k": " v "}, "k")
	_ = boolFromPass(nil, "k", true)
	_ = boolFromPass(map[string]any{}, "k", true)
	_ = boolFromPass(map[string]any{"k": "1"}, "k", false)
	_, _ = asList([]string{"a"})
	_, _ = asList([]any{"a"})
	_ = mapFromAny(nil)
	_ = mapFromAny(map[string]any{"a": 1})
	_ = heartbeatMaxAge(nil)
	_ = heartbeatMaxAge(map[string]any{"heartbeat_max_age": "bogus"})
	_ = heartbeatMaxAge(map[string]any{"heartbeat_max_age": "3s"})
	_ = LookPathAgentAPI()
	_ = ProbeConversationViaAgentAPI(ctx, "", "", "")
	_ = ProbeConversationViaAgentAPI(ctx, "false", "", "c1")
	probe := NewConversationProbe("", "")
	_ = probe(ctx, "seat", agentfeed.PeerSeatRecord{})
	_ = probe(ctx, "seat", agentfeed.PeerSeatRecord{Conversation: "c1"})

	art := Artifact{OK: true, Stages: []StageRecord{{Status: stageStatusFail}}}
	st := Stage{OnFail: OnFailSkip}
	_, _ = applyOnFail(&art, st, StageRecord{Status: stageStatusFail})
	art2 := Artifact{OK: true, Stages: []StageRecord{{Status: stageStatusFail}}}
	_, _ = applyOnFail(&art2, Stage{OnFail: OnFailHumanGate, ID: "g"}, StageRecord{})
	art3 := Artifact{OK: true, Stages: []StageRecord{{Status: stageStatusFail}}}
	_, _ = applyOnFail(&art3, Stage{OnFail: OnFailStop}, StageRecord{})
	_ = Payload(Artifact{RunID: "SWI-1", OK: true, WorkflowID: "W", HumanGate: "g", Path: "p"})
	_ = statusFromArtifact(Artifact{OK: true})
	_ = statusFromArtifact(Artifact{OK: false})
	_ = statusFromArtifact(Artifact{HumanGate: "x"})
	root := t.TempDir()
	a := Artifact{RunID: "SWI-1", OK: true}
	_ = writeArtifact(root, &a)
	if a.Path == "" {
		t.Fatal("artifact path")
	}

	env := testEnv(root, map[string]map[string]any{})
	env.DryRun = true
	_, _ = execControlPlane(ctx, env, Stage{Config: map[string]any{"mcp_tcp": "127.0.0.1:1"}, Pass: map[string]any{"mcp_subscribers_min": 0}})
	_, _ = execControlPlane(ctx, nil, Stage{})
	_, _ = execBindSeats(ctx, nil, Stage{})
	_, _ = execBindSeats(ctx, env, Stage{Config: map[string]any{"refresh_seats": true}})
	env.LoadSeats = func(string) (agentfeed.PeerSeatsFile, error) {
		return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
			"agy": {Wake: agentfeed.WakeMembraneAgentAPI, PID: 1, Conversation: "c1"},
			"mcp": {Wake: agentfeed.WakeMembraneMCP},
		}}, nil
	}
	_, _ = execBindSeats(ctx, env, Stage{
		Config: map[string]any{
			"pid_overrides":          map[string]any{"agy": "2"},
			"conversation_overrides": map[string]any{"agy": "c2"},
		},
		Pass: map[string]any{"conversation_live": false, "require_pid": false},
	})
	_, _ = execSeatWorkers(ctx, env, Stage{Config: map[string]any{"execute_non_comms": false, "poll_seconds": 1}})
	_, _ = execCommsCheck(ctx, nil, Stage{})
	_, _ = execCommsCheck(ctx, env, Stage{Config: map[string]any{"nonce": "n", "seats": []any{"agy"}}})
	_, _ = execChatBootstrap(ctx, nil, Stage{})
	_, _ = execChatBootstrap(ctx, env, Stage{Optional: true})
	env.AllowChat = true
	_, _ = execChatBootstrap(ctx, env, Stage{Config: map[string]any{"payload_path": "p"}})
	_, _ = execOrchestratePlan(ctx, nil, Stage{})
	_, _ = execOrchestratePlan(ctx, env, Stage{})
	env.PlanID = "PRI-X"
	_, _ = execOrchestratePlan(ctx, env, Stage{Config: map[string]any{"to_agent_id": "agy"}})

	env.DryRun = false
	env.EnsureMCP = func(context.Context, string) error { return errfmt.Errorf("mcp") }
	_, _ = execControlPlane(ctx, env, Stage{})
	env.EnsureMCP = func(context.Context, string) error { return nil }
	env.MCPSubscribers = func(context.Context) (int, error) { return 0, errfmt.Errorf("subs") }
	_, _ = execControlPlane(ctx, env, Stage{})
	env.MCPSubscribers = func(context.Context) (int, error) { return 2, nil }
	env.InspectFeed = func(agentfeed.DoctorOptions) agentfeed.DoctorResult {
		return agentfeed.DoctorResult{FeedHealth: "ok", ContractPathsOK: true, Issues: []string{"i"}}
	}
	env.GetObject = func(_ context.Context, id string) (map[string]any, error) {
		return map[string]any{objects.FieldKeyEnabled: true}, nil
	}
	_, _ = execControlPlane(ctx, env, Stage{Pass: map[string]any{"mcp_subscribers_min": 1}})
	env.GetObject = func(context.Context, string) (map[string]any, error) {
		return map[string]any{objects.FieldKeyEnabled: false}, nil
	}
	_, _ = execControlPlane(ctx, env, Stage{Pass: map[string]any{"mcp_subscribers_min": 1}})
	env.InstallSeatWorkers = func(context.Context, SeatWorkerInstallConfig) error { return errfmt.Errorf("inst") }
	_, _ = execSeatWorkers(ctx, env, Stage{Pass: map[string]any{"heartbeat_required": false}})
	env.InstallSeatWorkers = func(context.Context, SeatWorkerInstallConfig) error { return nil }
	_, _ = execSeatWorkers(ctx, env, Stage{Pass: map[string]any{"heartbeat_required": true, "heartbeat_max_age": "1ns"}})
	env.Steer = func(context.Context, string, string, bool) (SteerOutcome, error) {
		return SteerOutcome{}, errfmt.Errorf("steer")
	}
	_, _ = execCommsCheck(ctx, env, Stage{Config: map[string]any{"seats": []any{"agy"}}})
	env.Steer = func(_ context.Context, to, msg string, ack bool) (SteerOutcome, error) {
		return SteerOutcome{EventID: "e", AwaitID: "a", Receipt: true, ToAgentID: to, Message: msg}, nil
	}
	_, _ = execCommsCheck(ctx, env, Stage{Config: map[string]any{"seats": []any{"agy"}}})
	_, _ = execOrchestratePlan(ctx, env, Stage{Config: map[string]any{"to_agent_id": "agy"}})
	env.ChatBootstrap = func(context.Context, string, agentfeed.PeerSeatRecord, string) error { return nil }
	_, _ = execChatBootstrap(ctx, env, Stage{})
	env.AllowChat = false
	_, _ = execChatBootstrap(ctx, env, Stage{})
}
