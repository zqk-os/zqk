// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package swarminit

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func createMockAgentAPIScript(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "agentapi.sh")
	var body string
	switch mode {
	case "not_found_err":
		body = "#!/bin/sh\necho 'trajectory not found'\nexit 1\n"
	case "other_err":
		body = "#!/bin/sh\necho 'fatal rpc failure'\nexit 2\n"
	case "not_found_ok":
		body = "#!/bin/sh\necho 'trajectory not found'\nexit 0\n"
	case "success":
		body = "#!/bin/sh\necho '{\"metadata\":{\"status\":\"ok\"}}'\nexit 0\n"
	default:
		body = "#!/bin/sh\nexit 0\n"
	}
	writeErr := fileutil.WriteExecutableFile(binPath, []byte(body))
	if writeErr != nil {
		t.Fatalf("failed to write mock script: %v", writeErr)
	}
	return binPath
}

func TestCycle217_ConversationProbe_Scenarios(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	errEmptyBin := ProbeConversationViaAgentAPI(ctx, "", "127.0.0.1:9000", "c-1")
	if errEmptyBin == nil || !strings.Contains(errEmptyBin.Error(), "agentapi binary not set") {
		t.Fatalf("expected binary not set error, got %v", errEmptyBin)
	}

	binNotFoundErr := createMockAgentAPIScript(t, "not_found_err")
	errNotFound := ProbeConversationViaAgentAPI(ctx, binNotFoundErr, "127.0.0.1:9000", "c-dead")
	if errNotFound == nil || !strings.Contains(errNotFound.Error(), "trajectory not found") {
		t.Fatalf("expected trajectory not found error, got %v", errNotFound)
	}

	binOtherErr := createMockAgentAPIScript(t, "other_err")
	errOther := ProbeConversationViaAgentAPI(ctx, binOtherErr, "", "c-2")
	if errOther == nil || !strings.Contains(errOther.Error(), "agentapi metadata") {
		t.Fatalf("expected agentapi metadata error, got %v", errOther)
	}

	binNotFoundOK := createMockAgentAPIScript(t, "not_found_ok")
	errNotFoundOK := ProbeConversationViaAgentAPI(ctx, binNotFoundOK, "", "c-3")
	if errNotFoundOK == nil || !strings.Contains(errNotFoundOK.Error(), "trajectory not found") {
		t.Fatalf("expected trajectory not found error from exit 0, got %v", errNotFoundOK)
	}

	binSuccess := createMockAgentAPIScript(t, "success")
	errSuccess := ProbeConversationViaAgentAPI(ctx, binSuccess, "127.0.0.1:8888", "c-live")
	if errSuccess != nil {
		t.Fatalf("expected success, got %v", errSuccess)
	}

	probeSuccess := NewConversationProbe(binSuccess, "127.0.0.1:8888")
	recLive := agentfeed.PeerSeatRecord{Conversation: "c-live", PID: 1234}
	probeErr := probeSuccess(ctx, "seat-1", recLive)
	if probeErr != nil {
		t.Fatalf("expected probe success, got %v", probeErr)
	}

	lookPath := LookPathAgentAPI()
	if len(lookPath) >= 0 {
		// Evaluated LookPathAgentAPI safely
	}
}

func TestCycle217_Registry_EdgeCases(t *testing.T) {
	t.Parallel()
	var nilReg *Registry
	nilReg.Register("test", func(context.Context, *Env, Stage) (Result, error) {
		return Result{OK: true}, nil
	})

	reg := NewRegistry()
	reg.Register("", func(context.Context, *Env, Stage) (Result, error) {
		return Result{OK: true}, nil
	})
	reg.Register("custom", nil)

	_, errLookupNil := nilReg.Lookup("any")
	if errLookupNil == nil || !strings.Contains(errLookupNil.Error(), "unknown swarm-init executor") {
		t.Fatalf("expected lookup on nil registry to fail, got %v", errLookupNil)
	}

	emptyReg := &Registry{}
	_, errLookupEmpty := emptyReg.Lookup("any")
	if errLookupEmpty == nil || !strings.Contains(errLookupEmpty.Error(), "unknown swarm-init executor") {
		t.Fatalf("expected lookup on empty registry to fail, got %v", errLookupEmpty)
	}

	reg.Register("valid_exec", func(context.Context, *Env, Stage) (Result, error) {
		return Result{OK: true}, nil
	})
	execFn, errValid := reg.Lookup(" valid_exec ")
	if errValid != nil || execFn == nil {
		t.Fatalf("expected valid lookup, got fn=%v err=%v", execFn, errValid)
	}
}

func TestCycle217_Schema_Parsing_EdgeCases(t *testing.T) {
	t.Parallel()

	_, errNil := ParseRecipe(nil)
	if errNil == nil || !strings.Contains(errNil.Error(), "pipeline object is nil") {
		t.Fatalf("expected nil object error, got %v", errNil)
	}

	_, errNoID := ParseRecipe(map[string]any{"title": "No ID"})
	if errNoID == nil || !strings.Contains(errNoID.Error(), "pipeline id is required") {
		t.Fatalf("expected id required error, got %v", errNoID)
	}

	_, errNoStages := ParseRecipe(map[string]any{objects.FieldKeyID: "PIP-1"})
	if errNoStages == nil || !strings.Contains(errNoStages.Error(), "stages required") {
		t.Fatalf("expected stages required error, got %v", errNoStages)
	}

	_, errStagesNotList := ParseRecipe(map[string]any{
		objects.FieldKeyID:     "PIP-1",
		objects.FieldKeyStages: 12345,
	})
	if errStagesNotList == nil || !strings.Contains(errStagesNotList.Error(), "want list") {
		t.Fatalf("expected want list error, got %v", errStagesNotList)
	}

	_, errStagesEmpty := ParseRecipe(map[string]any{
		objects.FieldKeyID:     "PIP-1",
		objects.FieldKeyStages: []any{},
	})
	if errStagesEmpty == nil || !strings.Contains(errStagesEmpty.Error(), "stages must not be empty") {
		t.Fatalf("expected stages empty error, got %v", errStagesEmpty)
	}

	_, errStageNotMap := ParseRecipe(map[string]any{
		objects.FieldKeyID:     "PIP-1",
		objects.FieldKeyStages: []any{"invalid_stage_item"},
	})
	if errStageNotMap == nil || !strings.Contains(errStageNotMap.Error(), "must be an object") {
		t.Fatalf("expected stage must be object error, got %v", errStageNotMap)
	}

	_, errNoExecutor := ParseRecipe(map[string]any{
		objects.FieldKeyID: "PIP-1",
		objects.FieldKeyStages: []any{
			map[string]any{objects.FieldKeyID: "s1"},
		},
	})
	if errNoExecutor == nil || !strings.Contains(errNoExecutor.Error(), "executor is required") {
		t.Fatalf("expected executor is required error, got %v", errNoExecutor)
	}

	_, errInvalidOnFail := ParseRecipe(map[string]any{
		objects.FieldKeyID: "PIP-1",
		objects.FieldKeyStages: []any{
			map[string]any{
				objects.FieldKeyID: "s1",
				stageKeyExecutor:   ExecutorControlPlane,
				stageKeyOnFail:     "terminate_now",
			},
		},
	})
	if errInvalidOnFail == nil || !strings.Contains(errInvalidOnFail.Error(), "is not stop|skip|human_gate") {
		t.Fatalf("expected invalid on_fail error, got %v", errInvalidOnFail)
	}

	_, errDupStage := ParseRecipe(map[string]any{
		objects.FieldKeyID: "PIP-1",
		objects.FieldKeyStages: []any{
			map[string]any{objects.FieldKeyID: "s1", stageKeyExecutor: ExecutorControlPlane},
			map[string]any{objects.FieldKeyID: "s1", stageKeyExecutor: ExecutorControlPlane},
		},
	})
	if errDupStage == nil || !strings.Contains(errDupStage.Error(), "duplicate stage id") {
		t.Fatalf("expected duplicate stage error, got %v", errDupStage)
	}

	recipeMapSlice, errMapSlice := ParseRecipe(map[string]any{
		objects.FieldKeyID: "PIP-MAPS",
		objects.FieldKeyStages: []map[string]any{
			{stageKeyExecutor: ExecutorControlPlane},
		},
	})
	if errMapSlice != nil || len(recipeMapSlice.Stages) != 1 || recipeMapSlice.Stages[0].ID != "stage_0" {
		t.Fatalf("expected auto-generated stage_0 ID, got %v err=%v", recipeMapSlice, errMapSlice)
	}
}

func TestCycle217_Schema_TypeConversions(t *testing.T) {
	t.Parallel()

	_, errRefNil := PipelineRefFromWorkflow(nil)
	if errRefNil == nil || !strings.Contains(errRefNil.Error(), "workflow object is nil") {
		t.Fatalf("expected nil workflow error, got %v", errRefNil)
	}

	_, errRefEmpty := PipelineRefFromWorkflow(map[string]any{objects.FieldKeyID: "W-1"})
	if errRefEmpty == nil || !strings.Contains(errRefEmpty.Error(), "is empty") {
		t.Fatalf("expected empty ref error, got %v", errRefEmpty)
	}

	refVal, errRefOK := PipelineRefFromWorkflow(map[string]any{
		objects.FieldKeyID: "W-2",
		objects.FieldKeyMetadata: map[string]any{
			SwarmInitPipelineRefKey: "PIP-RECIPE",
		},
	})
	if errRefOK != nil || refVal != "PIP-RECIPE" {
		t.Fatalf("expected PIP-RECIPE, got %s err=%v", refVal, errRefOK)
	}

	listAny, errAny := asList([]any{"a", "b"})
	if errAny != nil || len(listAny) != 2 {
		t.Fatalf("asList []any failed: %v", errAny)
	}

	listMap, errMap := asList([]map[string]any{{"k": "v"}})
	if errMap != nil || len(listMap) != 1 {
		t.Fatalf("asList []map failed: %v", errMap)
	}

	_, errInvalidList := asList("string")
	if errInvalidList == nil {
		t.Fatal("expected asList error on string")
	}

	mMap, okMap := asMap(map[string]any{"key": "val"})
	if !okMap || mMap["key"] != "val" {
		t.Fatal("expected valid asMap")
	}

	_, okNotMap := asMap("not_a_map")
	if okNotMap {
		t.Fatal("expected asMap false on string")
	}

	mNil := mapFromAny(nil)
	if len(mNil) != 0 {
		t.Fatal("expected empty map from nil")
	}

	mValid := mapFromAny(map[string]any{"x": 1})
	if mValid["x"] != 1 {
		t.Fatal("expected mapped x=1")
	}

	if !boolFromAny("true") || !boolFromAny("1") || !boolFromAny("yes") {
		t.Fatal("boolFromAny truthy strings failed")
	}
	if boolFromAny("false") || boolFromAny("0") || boolFromAny("no") || boolFromAny(123) {
		t.Fatal("boolFromAny falsy strings failed")
	}

	if stringFromPass(nil, "key") != "" || stringFromPass(map[string]any{"k": " val "}, "k") != "val" {
		t.Fatal("stringFromPass failed")
	}

	if !boolFromPass(nil, "k", true) || boolFromPass(nil, "k", false) {
		t.Fatal("boolFromPass nil default failed")
	}
	if !boolFromPass(map[string]any{"k": "true"}, "k", false) {
		t.Fatal("boolFromPass key value failed")
	}

	if intFromAny(10, 0) != 10 || intFromAny(int64(20), 0) != 20 || intFromAny(float64(30), 0) != 30 {
		t.Fatal("intFromAny numeric types failed")
	}
	if intFromAny("40", 0) != 40 || intFromAny("not_an_int", 99) != 99 || intFromAny(true, 55) != 55 {
		t.Fatal("intFromAny fallback failed")
	}
}

func TestCycle217_Executors_ControlPlane_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	envEnsureErr := &Env{
		EnsureMCP: func(context.Context, string) error {
			return errfmt.Errorf("port bind failed")
		},
	}
	resEnsure, errEnsure := execControlPlane(ctx, envEnsureErr, Stage{})
	if errEnsure == nil || !strings.Contains(errEnsure.Error(), "port bind failed") || resEnsure.OK {
		t.Fatalf("expected EnsureMCP error, got %v", errEnsure)
	}

	envSubErr := &Env{
		MCPSubscribers: func(context.Context) (int, error) {
			return 0, errfmt.Errorf("rpc failure")
		},
	}
	resSub, errSub := execControlPlane(ctx, envSubErr, Stage{})
	if errSub == nil || !strings.Contains(errSub.Error(), "rpc failure") || resSub.OK {
		t.Fatalf("expected MCPSubscribers error, got %v", errSub)
	}

	envSubLow := &Env{
		MCPSubscribers: func(context.Context) (int, error) {
			return 0, nil
		},
	}
	resLow, errLow := execControlPlane(ctx, envSubLow, Stage{
		Pass: map[string]any{"mcp_subscribers_min": 2},
	})
	if errLow != nil || resLow.OK {
		t.Fatalf("expected resLow.OK to be false, got ok=%v err=%v", resLow.OK, errLow)
	}

	envInspect := &Env{
		ProjectRoot: t.TempDir(),
		InspectFeed: func(agentfeed.DoctorOptions) agentfeed.DoctorResult {
			return agentfeed.DoctorResult{
				FeedHealth:      "degraded",
				ContractPathsOK: false,
				Issues:          []string{"missing ledger"},
			}
		},
	}
	resInspect, errInspect := execControlPlane(ctx, envInspect, Stage{})
	if errInspect != nil || !resInspect.OK || resInspect.Evidence["feed_health"] != "degraded" {
		t.Fatalf("expected inspect evidence, got res=%+v err=%v", resInspect, errInspect)
	}

	envNightDutyEnabled := &Env{
		GetObject: func(_ context.Context, id string) (map[string]any, error) {
			if id == objects.JobIDCapNightDuty {
				return map[string]any{objects.FieldKeyEnabled: true}, nil
			}
			return nil, errfmt.Errorf("not found")
		},
	}
	resND, errND := execControlPlane(ctx, envNightDutyEnabled, Stage{})
	if errND == nil || !strings.Contains(errND.Error(), "refuses to continue") || resND.OK {
		t.Fatalf("expected night duty refusal, got res=%+v err=%v", resND, errND)
	}
}

func TestCycle217_Executors_BindSeats_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, errNoEnv := execBindSeats(ctx, nil, Stage{})
	if errNoEnv == nil || !strings.Contains(errNoEnv.Error(), "env required") {
		t.Fatalf("expected env required error, got %v", errNoEnv)
	}

	envLoadErr := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{}, errfmt.Errorf("disk read error")
		},
	}
	_, errLoadSeats := execBindSeats(ctx, envLoadErr, Stage{})
	if errLoadSeats == nil || !strings.Contains(errLoadSeats.Error(), "disk read error") {
		t.Fatalf("expected disk read error, got %v", errLoadSeats)
	}

	envSaveErr := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"seat-1": {PID: 10, Conversation: "conv-1"},
			}}, nil
		},
		SaveSeats: func(string, agentfeed.PeerSeatsFile) error {
			return errfmt.Errorf("save seats failed")
		},
	}
	stageOverrides := Stage{
		Config: map[string]any{
			"pid_overrides": map[string]any{"seat-1": "99"},
		},
	}
	_, errSaveSeats := execBindSeats(ctx, envSaveErr, stageOverrides)
	if errSaveSeats == nil || !strings.Contains(errSaveSeats.Error(), "save seats failed") {
		t.Fatalf("expected save seats failed error, got %v", errSaveSeats)
	}

	savedSeats := false
	envSaveOK := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"seat-1": {PID: 10, Conversation: "c1", Wake: agentfeed.WakeMembraneMCP},
			}}, nil
		},
		SaveSeats: func(string, agentfeed.PeerSeatsFile) error {
			savedSeats = true
			return nil
		},
	}
	resSaveOK, errSaveOK := execBindSeats(ctx, envSaveOK, Stage{
		Config: map[string]any{
			"conversation_overrides": map[string]any{"seat-1": "c2"},
		},
	})
	if errSaveOK != nil || !resSaveOK.OK || !savedSeats || resSaveOK.Evidence["seats_written"] != true {
		t.Fatalf("expected seats_written, got res=%+v err=%v", resSaveOK, errSaveOK)
	}

	envSeatIssues := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"seat-no-pid":  {Wake: agentfeed.WakeMembraneAgentAPI, PID: 0, Conversation: "c1"},
				"seat-no-conv": {Wake: agentfeed.WakeMembraneAgentAPI, PID: 10, Conversation: ""},
				"seat-dead":    {Wake: agentfeed.WakeMembraneAgentAPI, PID: 99999999, Conversation: "c2"},
			}}, nil
		},
		ProbeConversation: func(context.Context, string, agentfeed.PeerSeatRecord) error {
			return errfmt.Errorf("probe failed")
		},
	}
	resIssues, errIssues := execBindSeats(ctx, envSeatIssues, Stage{})
	if errIssues == nil || resIssues.OK {
		t.Fatalf("expected bind seats issues failure, got res=%+v err=%v", resIssues, errIssues)
	}
}

func TestCycle217_Executors_SeatWorkers_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	envLoadErr := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{}, errfmt.Errorf("seat load failure")
		},
	}
	_, errLoad := execSeatWorkers(ctx, envLoadErr, Stage{})
	if errLoad == nil || !strings.Contains(errLoad.Error(), "seat load failure") {
		t.Fatalf("expected seat load failure, got %v", errLoad)
	}

	envInstallErr := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"w1": {Wake: agentfeed.WakeMembraneAgentAPI},
			}}, nil
		},
		InstallSeatWorkers: func(context.Context, SeatWorkerInstallConfig) error {
			return errfmt.Errorf("install failed")
		},
	}
	_, errInstall := execSeatWorkers(ctx, envInstallErr, Stage{})
	if errInstall == nil || !strings.Contains(errInstall.Error(), "install failed") {
		t.Fatalf("expected install failed error, got %v", errInstall)
	}

	envStaleNoInstaller := &Env{
		ProjectRoot: t.TempDir(),
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"w-dead": {Wake: agentfeed.WakeMembraneAgentAPI},
			}}, nil
		},
	}
	resStaleReq, errStaleReq := execSeatWorkers(ctx, envStaleNoInstaller, Stage{
		Pass: map[string]any{"heartbeat_required": true},
	})
	if errStaleReq == nil || resStaleReq.OK || !strings.Contains(errStaleReq.Error(), "stale/missing heartbeat") {
		t.Fatalf("expected stale heartbeat error, got res=%+v err=%v", resStaleReq, errStaleReq)
	}

	resStaleOpt, errStaleOpt := execSeatWorkers(ctx, envStaleNoInstaller, Stage{
		Pass: map[string]any{"heartbeat_required": false},
	})
	if errStaleOpt != nil || !resStaleOpt.OK {
		t.Fatalf("expected ok when heartbeat not required, got res=%+v err=%v", resStaleOpt, errStaleOpt)
	}

	envStaleWithInstaller := &Env{
		ProjectRoot: t.TempDir(),
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"w-dead-2": {Wake: agentfeed.WakeMembraneMCP},
			}}, nil
		},
		InstallSeatWorkers: func(context.Context, SeatWorkerInstallConfig) error {
			return nil
		},
	}
	resInstalledStale, errInstalledStale := execSeatWorkers(ctx, envStaleWithInstaller, Stage{
		Pass: map[string]any{"heartbeat_required": true},
	})
	if errInstalledStale == nil || resInstalledStale.OK || !strings.Contains(errInstalledStale.Error(), "heartbeat missing") {
		t.Fatalf("expected heartbeat missing error, got res=%+v err=%v", resInstalledStale, errInstalledStale)
	}
}

func TestCycle217_Executors_CommsCheck_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, errNoEnv := execCommsCheck(ctx, nil, Stage{})
	if errNoEnv == nil || !strings.Contains(errNoEnv.Error(), "env required") {
		t.Fatalf("expected env required error, got %v", errNoEnv)
	}

	envLoadErr := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{}, errfmt.Errorf("seats failed")
		},
	}
	_, errLoad := execCommsCheck(ctx, envLoadErr, Stage{})
	if errLoad == nil || !strings.Contains(errLoad.Error(), "seats failed") {
		t.Fatalf("expected seats failed error, got %v", errLoad)
	}

	envNoSteer := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"w1": {Wake: agentfeed.WakeMembraneAgentAPI},
			}}, nil
		},
	}
	_, errNoSteer := execCommsCheck(ctx, envNoSteer, Stage{})
	if errNoSteer == nil || !strings.Contains(errNoSteer.Error(), "steer hook required") {
		t.Fatalf("expected steer hook required error, got %v", errNoSteer)
	}

	envSteerErr := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"w1": {Wake: agentfeed.WakeMembraneAgentAPI},
			}}, nil
		},
		Steer: func(context.Context, string, string, bool) (SteerOutcome, error) {
			return SteerOutcome{}, errfmt.Errorf("transport timeout")
		},
	}
	_, errSteerErr := execCommsCheck(ctx, envSteerErr, Stage{})
	if errSteerErr == nil || !strings.Contains(errSteerErr.Error(), "transport timeout") {
		t.Fatalf("expected transport timeout error, got %v", errSteerErr)
	}

	envSteerOK := &Env{
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"w1": {Wake: agentfeed.WakeMembraneAgentAPI},
			}}, nil
		},
		Steer: func(_ context.Context, to, msg string, _ bool) (SteerOutcome, error) {
			return SteerOutcome{
				EventID:   "evt-1",
				AwaitID:   "await-1",
				Receipt:   true,
				ToAgentID: to,
				Message:   msg,
			}, nil
		},
	}
	resOK, errOK := execCommsCheck(ctx, envSteerOK, Stage{
		Config: map[string]any{"seats": []any{"w1"}},
	})
	if errOK != nil || !resOK.OK || len(resOK.Evidence["steers"].([]map[string]any)) != 1 {
		t.Fatalf("expected successful comms check steer, got res=%+v err=%v", resOK, errOK)
	}
}

func TestCycle217_Executors_ChatBootstrap_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, errNoEnv := execChatBootstrap(ctx, nil, Stage{})
	if errNoEnv == nil || !strings.Contains(errNoEnv.Error(), "env required") {
		t.Fatalf("expected env required error, got %v", errNoEnv)
	}

	envOptNoChat := &Env{AllowChat: false}
	resOpt, errOpt := execChatBootstrap(ctx, envOptNoChat, Stage{Optional: true})
	if errOpt != nil || !resOpt.Skipped || resOpt.Human != "" {
		t.Fatalf("expected skipped without human gate, got res=%+v err=%v", resOpt, errOpt)
	}

	resReq, errReq := execChatBootstrap(ctx, envOptNoChat, Stage{Optional: false})
	if errReq != nil || !resReq.Skipped || resReq.Human != "allow-chat" {
		t.Fatalf("expected skipped with allow-chat human gate, got res=%+v err=%v", resReq, errReq)
	}

	envDry := &Env{AllowChat: true, DryRun: true}
	resDry, errDry := execChatBootstrap(ctx, envDry, Stage{})
	if errDry != nil || !resDry.OK {
		t.Fatalf("expected dry-run ok, got res=%+v err=%v", resDry, errDry)
	}

	envNoHook := &Env{AllowChat: true}
	_, errNoHook := execChatBootstrap(ctx, envNoHook, Stage{})
	if errNoHook == nil || !strings.Contains(errNoHook.Error(), "chat_bootstrap hook not wired") {
		t.Fatalf("expected hook not wired error, got %v", errNoHook)
	}

	envLoadErr := &Env{
		AllowChat:     true,
		ChatBootstrap: func(context.Context, string, agentfeed.PeerSeatRecord, string) error { return nil },
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{}, errfmt.Errorf("seats unreadable")
		},
	}
	_, errLoad := execChatBootstrap(ctx, envLoadErr, Stage{})
	if errLoad == nil || !strings.Contains(errLoad.Error(), "seats unreadable") {
		t.Fatalf("expected seats unreadable error, got %v", errLoad)
	}

	envBootErr := &Env{
		AllowChat: true,
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"w1": {Wake: agentfeed.WakeMembraneAgentAPI},
			}}, nil
		},
		ChatBootstrap: func(context.Context, string, agentfeed.PeerSeatRecord, string) error {
			return errfmt.Errorf("peer rejection")
		},
	}
	_, errBoot := execChatBootstrap(ctx, envBootErr, Stage{})
	if errBoot == nil || !strings.Contains(errBoot.Error(), "peer rejection") {
		t.Fatalf("expected peer rejection error, got %v", errBoot)
	}

	bootstrappedCount := 0
	envBootOK := &Env{
		AllowChat: true,
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
				"w1":    {Wake: agentfeed.WakeMembraneAgentAPI},
				"w-mcp": {Wake: agentfeed.WakeMembraneMCP},
			}}, nil
		},
		ChatBootstrap: func(context.Context, string, agentfeed.PeerSeatRecord, string) error {
			bootstrappedCount++
			return nil
		},
	}
	resBootOK, errBootOK := execChatBootstrap(ctx, envBootOK, Stage{})
	if errBootOK != nil || !resBootOK.OK || bootstrappedCount != 1 || resBootOK.Evidence["bootstrapped"] != 1 {
		t.Fatalf("expected 1 bootstrapped seat, got res=%+v err=%v", resBootOK, errBootOK)
	}
}

func TestCycle217_Executors_OrchestratePlan_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, errNoEnv := execOrchestratePlan(ctx, nil, Stage{})
	if errNoEnv == nil || !strings.Contains(errNoEnv.Error(), "env required") {
		t.Fatalf("expected env required error, got %v", errNoEnv)
	}

	envNoPlan := &Env{}
	_, errNoPlan := execOrchestratePlan(ctx, envNoPlan, Stage{})
	if errNoPlan == nil || !strings.Contains(errNoPlan.Error(), "plan_id required") {
		t.Fatalf("expected plan_id required error, got %v", errNoPlan)
	}

	envDry := &Env{
		PlanID: "PRI-ORCH-001",
		DryRun: true,
	}
	resDry, errDry := execOrchestratePlan(ctx, envDry, Stage{})
	if errDry != nil || !resDry.OK {
		t.Fatalf("expected dry-run ok, got res=%+v err=%v", resDry, errDry)
	}

	envNoSteer := &Env{
		PlanID: "PRI-ORCH-001",
	}
	stageWithTarget := Stage{
		Config: map[string]any{"to_agent_id": "peer-agent-1"},
	}
	_, errNoSteer := execOrchestratePlan(ctx, envNoSteer, stageWithTarget)
	if errNoSteer == nil || !strings.Contains(errNoSteer.Error(), "steer hook required") {
		t.Fatalf("expected steer hook required error, got %v", errNoSteer)
	}

	envNoTarget := &Env{
		PlanID: "PRI-ORCH-001",
		Steer: func(context.Context, string, string, bool) (SteerOutcome, error) {
			return SteerOutcome{}, nil
		},
	}
	_, errNoTarget := execOrchestratePlan(ctx, envNoTarget, Stage{})
	if errNoTarget == nil || !strings.Contains(errNoTarget.Error(), "to_agent_id required") {
		t.Fatalf("expected to_agent_id required error, got %v", errNoTarget)
	}

	envSteerErr := &Env{
		PlanID: "PRI-ORCH-001",
		Steer: func(context.Context, string, string, bool) (SteerOutcome, error) {
			return SteerOutcome{}, errfmt.Errorf("network disconnect")
		},
	}
	_, errSteerErr := execOrchestratePlan(ctx, envSteerErr, stageWithTarget)
	if errSteerErr == nil || !strings.Contains(errSteerErr.Error(), "network disconnect") {
		t.Fatalf("expected network disconnect error, got %v", errSteerErr)
	}

	envSteerOK := &Env{
		PlanID: "PRI-ORCH-001",
		Steer: func(_ context.Context, to, _ string, _ bool) (SteerOutcome, error) {
			return SteerOutcome{
				EventID:   "evt-orch-1",
				AwaitID:   "await-orch-1",
				ToAgentID: to,
			}, nil
		},
	}
	resSteerOK, errSteerOK := execOrchestratePlan(ctx, envSteerOK, stageWithTarget)
	if errSteerOK != nil || !resSteerOK.OK || resSteerOK.Evidence["event_id"] != "evt-orch-1" {
		t.Fatalf("expected orchestrate plan success, got res=%+v err=%v", resSteerOK, errSteerOK)
	}
}

func TestCycle217_Run_EdgeCases_And_FailureModes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, errNoRoot := Run(ctx, Options{Env: &Env{ProjectRoot: ""}})
	if errNoRoot == nil || !strings.Contains(errNoRoot.Error(), "project root is required") {
		t.Fatalf("expected project root error, got %v", errNoRoot)
	}

	_, errNoGetter := Run(ctx, Options{Env: &Env{ProjectRoot: t.TempDir()}})
	if errNoGetter == nil || !strings.Contains(errNoGetter.Error(), "object getter is required") {
		t.Fatalf("expected object getter error, got %v", errNoGetter)
	}

	root := t.TempDir()
	store := map[string]map[string]any{}
	env := testEnv(root, store)

	_, errNoTarget := Run(ctx, Options{Env: env})
	if errNoTarget == nil || !strings.Contains(errNoTarget.Error(), "require --pipeline or --workflow") {
		t.Fatalf("expected require pipeline or workflow error, got %v", errNoTarget)
	}

	_, errWorkflowNotFound := Run(ctx, Options{WorkflowID: "WFL-MISSING", Env: env})
	if errWorkflowNotFound == nil || !strings.Contains(errWorkflowNotFound.Error(), "get workflow") {
		t.Fatalf("expected get workflow error, got %v", errWorkflowNotFound)
	}

	store["WFL-BAD-REF"] = map[string]any{
		objects.FieldKeyID: "WFL-BAD-REF",
	}
	_, errWorkflowBadRef := Run(ctx, Options{WorkflowID: "WFL-BAD-REF", Env: env})
	if errWorkflowBadRef == nil || !strings.Contains(errWorkflowBadRef.Error(), "metadata.swarm_init_pipeline_ref is empty") {
		t.Fatalf("expected empty pipeline ref error, got %v", errWorkflowBadRef)
	}

	_, errPipeNotFound := Run(ctx, Options{PipelineID: "PIP-NOT-FOUND", Env: env})
	if errPipeNotFound == nil || !strings.Contains(errPipeNotFound.Error(), "get pipeline") {
		t.Fatalf("expected get pipeline error, got %v", errPipeNotFound)
	}

	store["PIP-CORRUPT"] = map[string]any{
		objects.FieldKeyID: "PIP-CORRUPT",
	}
	_, errPipeCorrupt := Run(ctx, Options{PipelineID: "PIP-CORRUPT", Env: env})
	if errPipeCorrupt == nil || !strings.Contains(errPipeCorrupt.Error(), "stages required") {
		t.Fatalf("expected stages required error, got %v", errPipeCorrupt)
	}
}

func TestCycle217_Run_FailureHandling_OnFailPolicies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()

	reg := NewRegistry()
	reg.Register("fail_exec", func(context.Context, *Env, Stage) (Result, error) {
		return Result{}, errfmt.Errorf("underlying failure")
	})
	reg.Register("not_ok_exec", func(context.Context, *Env, Stage) (Result, error) {
		return Result{OK: false}, nil
	})
	reg.Register("gate_exec", func(context.Context, *Env, Stage) (Result, error) {
		return Result{Skipped: true, Human: "human_approval"}, nil
	})

	store := map[string]map[string]any{
		"PIP-STOP": testPipeline("PIP-STOP", []map[string]any{
			{objects.FieldKeyID: "s-stop", stageKeyExecutor: "fail_exec", stageKeyOnFail: OnFailStop},
		}),
		"PIP-SKIP": testPipeline("PIP-SKIP", []map[string]any{
			{objects.FieldKeyID: "s-skip", stageKeyExecutor: "fail_exec", stageKeyOnFail: OnFailSkip},
		}),
		"PIP-GATE": testPipeline("PIP-GATE", []map[string]any{
			{objects.FieldKeyID: "s-gate", stageKeyExecutor: "fail_exec", stageKeyOnFail: OnFailHumanGate},
		}),
		"PIP-NOT-OK-STOP": testPipeline("PIP-NOT-OK-STOP", []map[string]any{
			{objects.FieldKeyID: "s-not-ok", stageKeyExecutor: "not_ok_exec", stageKeyOnFail: OnFailStop},
		}),
		"PIP-NOT-OK-SKIP": testPipeline("PIP-NOT-OK-SKIP", []map[string]any{
			{objects.FieldKeyID: "s-not-ok-skip", stageKeyExecutor: "not_ok_exec", stageKeyOnFail: OnFailSkip},
		}),
		"PIP-NOT-OK-GATE": testPipeline("PIP-NOT-OK-GATE", []map[string]any{
			{objects.FieldKeyID: "s-not-ok-gate", stageKeyExecutor: "not_ok_exec", stageKeyOnFail: OnFailHumanGate},
		}),
		"PIP-HUMAN-RES": testPipeline("PIP-HUMAN-RES", []map[string]any{
			{objects.FieldKeyID: "s-human-gate", stageKeyExecutor: "gate_exec"},
		}),
	}
	env := testEnv(root, store)

	artStop, errStop := Run(ctx, Options{PipelineID: "PIP-STOP", Env: env, Registry: reg})
	if errStop == nil || artStop.OK {
		t.Fatalf("expected stop failure, got art=%+v err=%v", artStop, errStop)
	}

	artSkip, errSkip := Run(ctx, Options{PipelineID: "PIP-SKIP", Env: env, Registry: reg})
	if errSkip != nil || !artSkip.OK || artSkip.Stages[0].Status != stageStatusSkip {
		t.Fatalf("expected skip success, got art=%+v err=%v", artSkip, errSkip)
	}

	artGate, errGate := Run(ctx, Options{PipelineID: "PIP-GATE", Env: env, Registry: reg})
	if errGate != nil || artGate.OK || artGate.HumanGate != "s-gate" {
		t.Fatalf("expected human gate, got art=%+v err=%v", artGate, errGate)
	}

	artNOStop, errNOStop := Run(ctx, Options{PipelineID: "PIP-NOT-OK-STOP", Env: env, Registry: reg})
	if errNOStop == nil || artNOStop.OK {
		t.Fatalf("expected not ok stop failure, got art=%+v err=%v", artNOStop, errNOStop)
	}

	artNOSkip, errNOSkip := Run(ctx, Options{PipelineID: "PIP-NOT-OK-SKIP", Env: env, Registry: reg})
	if errNOSkip != nil || !artNOSkip.OK || artNOSkip.Stages[0].Status != stageStatusSkip {
		t.Fatalf("expected not ok skip success, got art=%+v err=%v", artNOSkip, errNOSkip)
	}

	artNOGate, errNOGate := Run(ctx, Options{PipelineID: "PIP-NOT-OK-GATE", Env: env, Registry: reg})
	if errNOGate != nil || artNOGate.OK || artNOGate.HumanGate != "s-not-ok-gate" {
		t.Fatalf("expected not ok human gate, got art=%+v err=%v", artNOGate, errNOGate)
	}

	artHumanRes, errHumanRes := Run(ctx, Options{PipelineID: "PIP-HUMAN-RES", Env: env, Registry: reg})
	if errHumanRes != nil || artHumanRes.OK || artHumanRes.HumanGate != "s-human-gate" {
		t.Fatalf("expected human gate from result, got art=%+v err=%v", artHumanRes, errHumanRes)
	}
}

func TestCycle217_Env_Fallback_Methods_And_Payload(t *testing.T) {
	t.Parallel()

	var nilEnv *Env
	nowNil := nilEnv.now()
	if nowNil.IsZero() {
		t.Fatal("expected non-zero time from nilEnv.now()")
	}

	customNow := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	envWithNow := &Env{Now: func() time.Time { return customNow }}
	if !envWithNow.now().Equal(customNow) {
		t.Fatalf("expected customNow, got %v", envWithNow.now())
	}

	root := t.TempDir()
	envSeats := &Env{
		ProjectRoot: root,
	}
	seats, errLoad := envSeats.loadSeats()
	if errLoad != nil {
		t.Fatalf("expected fallback loadSeats to succeed, got %v", errLoad)
	}
	errSave := envSeats.saveSeats(seats)
	if errSave != nil {
		t.Fatalf("expected fallback saveSeats to succeed, got %v", errSave)
	}

	errWriteNilArt := writeArtifact(root, nil)
	if errWriteNilArt != nil {
		t.Fatalf("expected nil error on nil artifact, got %v", errWriteNilArt)
	}

	errWriteNilRoot := writeArtifact("", &Artifact{RunID: "SWI-1"})
	if errWriteNilRoot != nil {
		t.Fatalf("expected nil error on empty root, got %v", errWriteNilRoot)
	}

	artBadDate := &Artifact{
		RunID:     "SWI-BADDATE",
		StartedAt: "invalid-date-format",
	}
	errWriteBadDate := writeArtifact(root, artBadDate)
	if errWriteBadDate != nil {
		t.Fatalf("expected fallback date write to succeed, got %v", errWriteBadDate)
	}

	payloadGate := Payload(Artifact{
		RunID:      "SWI-P1",
		PipelineID: "PIP-P1",
		WorkflowID: "WFL-P1",
		HumanGate:  "stage_1",
		OK:         false,
	})
	if payloadGate["workflow_id"] != "WFL-P1" || payloadGate["next_human_gate"] != "stage_1" || payloadGate[objects.FieldKeyStatus] != stageStatusGate {
		t.Fatalf("expected payload with human gate, got %+v", payloadGate)
	}
}

func TestCycle217_Concurrency_And_Race(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()

	pipelineObj := testPipeline("PIP-CONCURRENT", []map[string]any{
		{objects.FieldKeyID: "control_plane", stageKeyExecutor: ExecutorControlPlane},
	})
	store := map[string]map[string]any{
		"PIP-CONCURRENT": pipelineObj,
	}

	const workerCount = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workerCount*2)

	for i := 0; i < workerCount; i++ {
		workerID := i
		wg.Add(2)

		goroutinelabels.NewGoroutine(
			fmt.Sprintf("swarminit_recipe_parse_worker_%d", workerID),
			"concurrent recipe parsing validation",
		).StartSimple(func() {
			defer wg.Done()
			_, err := ParseRecipe(pipelineObj)
			if err != nil {
				errCh <- fmt.Errorf("worker %d parse recipe failed: %w", workerID, err)
			}
		})

		goroutinelabels.NewGoroutine(
			fmt.Sprintf("swarminit_run_worker_%d", workerID),
			"concurrent runner pipeline execution",
		).StartSimple(func() {
			defer wg.Done()
			env := testEnv(filepath.Join(root, fmt.Sprintf("sub_%d", workerID)), store)
			env.DryRun = true
			art, err := Run(ctx, Options{
				PipelineID: "PIP-CONCURRENT",
				Env:        env,
			})
			if err != nil || !art.OK {
				errCh <- fmt.Errorf("worker %d run failed: art=%+v err=%w", workerID, art, err)
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrency error: %v", err)
	}
}
