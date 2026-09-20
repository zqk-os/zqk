package swarminit

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

func execControlPlane(ctx context.Context, env *Env, stage Stage) (Result, error) {
	tcp := strings.TrimSpace(objects.GetString(stage.Config, "mcp_tcp"))
	if tcp == "" {
		tcp = "127.0.0.1:8443"
	}
	minSub := intFromAny(stage.Pass["mcp_subscribers_min"], 1)
	ev := map[string]any{
		"mcp_tcp":              tcp,
		"forbid_refresh_seats": true,
		"dry_run":              env != nil && env.DryRun,
	}
	if env != nil && env.DryRun {
		ev["would"] = "ensure mcp; refuse --refresh-seats; leave night-duty disabled"
		return Result{OK: true, Evidence: ev}, nil
	}
	if env != nil && env.EnsureMCP != nil {
		if err := env.EnsureMCP(ctx, tcp); err != nil {
			return Result{Evidence: ev}, errfmt.Newf("mcp ensure").Wrap(err)
		}
		ev["mcp_ensure"] = "ok"
	}
	subs := -1
	if env != nil && env.MCPSubscribers != nil {
		n, err := env.MCPSubscribers(ctx)
		if err != nil {
			return Result{Evidence: ev}, errfmt.Newf("mcp subscribers").Wrap(err)
		}
		subs = n
	}
	ev["mcp_subscribers"] = subs
	if env != nil && env.InspectFeed != nil {
		doc := env.InspectFeed(agentfeed.DoctorOptions{
			ProjectRoot:    env.ProjectRoot,
			MCPSubscribers: subs,
		})
		ev["feed_health"] = doc.FeedHealth
		ev["contract_paths_ok"] = doc.ContractPathsOK
		if len(doc.Issues) > 0 {
			ev["doctor_issues"] = doc.Issues
		}
	}
	if env != nil && env.GetObject != nil {
		job, err := env.GetObject(ctx, objects.JobIDCapNightDuty)
		if err == nil {
			enabled := boolFromAny(job[objects.FieldKeyEnabled])
			ev["night_duty_enabled"] = enabled
			if enabled {
				return Result{OK: false, Evidence: ev}, errfmt.Errorf("%s is enabled; swarm-init will not turn it on and refuses to continue", objects.JobIDCapNightDuty)
			}
		}
	}
	ok := subs < 0 || subs >= minSub
	return Result{OK: ok, Evidence: ev}, nil
}

func execBindSeats(ctx context.Context, env *Env, stage Stage) (Result, error) {
	if boolFromAny(stage.Config["refresh_seats"]) || boolFromAny(stage.Config["allow_refresh_seats"]) {
		return Result{}, errfmt.Errorf("bind_seats: --refresh-seats is forbidden")
	}
	ev := map[string]any{"forbid_refresh_seats": true, "dry_run": env != nil && env.DryRun}
	if env == nil {
		return Result{Evidence: ev}, errfmt.Errorf("env required")
	}
	seats, err := env.loadSeats()
	if err != nil {
		return Result{Evidence: ev}, err
	}
	pidOver := mapFromAny(stage.Config["pid_overrides"])
	convOver := mapFromAny(stage.Config["conversation_overrides"])
	changed := false
	for seatID, rec := range seats.Seats {
		if v := strings.TrimSpace(objects.GetString(pidOver, seatID)); v != "" {
			if n, convErr := strconv.Atoi(v); convErr == nil && n > 0 {
				rec.PID = n
				changed = true
			}
		}
		if v := strings.TrimSpace(objects.GetString(convOver, seatID)); v != "" {
			rec.Conversation = v
			changed = true
		}
		seats.Seats[seatID] = rec
	}
	if changed && !env.DryRun {
		if err := env.saveSeats(seats); err != nil {
			return Result{Evidence: ev}, err
		}
		ev["seats_written"] = true
	}
	live := boolFromPass(stage.Pass, "conversation_live", true)
	needPID := boolFromPass(stage.Pass, "require_pid", true)
	issues := []string{}
	checked := 0
	for seatID, rec := range seats.Seats {
		wake := agentfeed.NormalizeWakeMembrane(rec.Wake)
		if wake != "" && wake != agentfeed.WakeMembraneAgentAPI {
			continue
		}
		checked++
		if needPID && rec.PID <= 0 {
			issues = append(issues, seatID+": missing pid")
			continue
		}
		if strings.TrimSpace(rec.Conversation) == "" {
			issues = append(issues, seatID+": missing conversation")
			continue
		}
		if env.DryRun {
			continue
		}
		if rec.PID > 0 && !agentfeed.IsPIDAlive(rec.PID) {
			issues = append(issues, seatID+": pid not alive")
		}
		if live && env.ProbeConversation != nil {
			if err := env.ProbeConversation(ctx, seatID, rec); err != nil {
				issues = append(issues, seatID+": "+err.Error())
			}
		}
	}
	ev["agentapi_seats_checked"] = checked
	if len(issues) > 0 {
		ev["issues"] = issues
		return Result{OK: false, Evidence: ev}, errfmt.Errorf("bind_seats failed: %s", strings.Join(issues, "; "))
	}
	return Result{OK: true, Evidence: ev}, nil
}

func execSeatWorkers(ctx context.Context, env *Env, stage Stage) (Result, error) {
	execNC := true
	if _, ok := stage.Config["execute_non_comms"]; ok {
		execNC = boolFromAny(stage.Config["execute_non_comms"])
	}
	poll := intFromAny(stage.Config["poll_seconds"], 30)
	ev := map[string]any{
		"execute_non_comms": execNC,
		"poll_seconds":      poll,
		"dry_run":           env != nil && env.DryRun,
	}
	if env != nil && env.DryRun {
		ev["would"] = "install seat-workers with execute_non_comms from recipe"
		return Result{OK: true, Evidence: ev}, nil
	}
	seats, err := env.loadSeats()
	if err != nil {
		return Result{Evidence: ev}, err
	}
	var ids []string
	for id, rec := range seats.Seats {
		wake := agentfeed.NormalizeWakeMembrane(rec.Wake)
		if wake == agentfeed.WakeMembraneAgentAPI || wake == agentfeed.WakeMembraneMCP {
			ids = append(ids, id)
		}
	}
	ev["seats"] = ids
	if env.InstallSeatWorkers != nil {
		if err := env.InstallSeatWorkers(ctx, SeatWorkerInstallConfig{
			ExecuteNonComms: execNC,
			PollSeconds:     poll,
			Seats:           ids,
		}); err != nil {
			return Result{Evidence: ev}, err
		}
		ev["installed"] = true
	}
	maxAge := heartbeatMaxAge(stage.Pass)
	stale := []string{}
	for _, id := range ids {
		ok, ts, err := agentfeed.SeatWorkerAlive(env.ProjectRoot, id, maxAge)
		if err != nil || !ok {
			stale = append(stale, id)
			continue
		}
		_ = ts
	}
	if len(stale) > 0 && env.InstallSeatWorkers == nil {
		// No installer and no heartbeat: still fail closed when pass asks for heartbeat.
		if boolFromPass(stage.Pass, "heartbeat_required", true) {
			ev["stale_heartbeats"] = stale
			return Result{OK: false, Evidence: ev}, errfmt.Errorf("seat_workers: stale/missing heartbeat for %s", strings.Join(stale, ","))
		}
	}
	if len(stale) > 0 && env.InstallSeatWorkers != nil && boolFromPass(stage.Pass, "heartbeat_required", false) {
		ev["stale_heartbeats"] = stale
		return Result{OK: false, Evidence: ev}, errfmt.Errorf("seat_workers: heartbeat missing for %s", strings.Join(stale, ","))
	}
	return Result{OK: true, Evidence: ev}, nil
}

func heartbeatMaxAge(pass map[string]any) time.Duration {
	s := stringFromPass(pass, "heartbeat_max_age")
	if s == "" {
		return 2 * time.Minute
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 2 * time.Minute
	}
	return d
}

func execCommsCheck(ctx context.Context, env *Env, stage Stage) (Result, error) {
	if env == nil {
		return Result{}, errfmt.Errorf("env required")
	}
	nonce := strings.TrimSpace(objects.GetString(stage.Config, "nonce"))
	if nonce == "" {
		nonce = agentfeed.CommsNoncePrefix + env.now().UTC().Format("20060102T150405Z")
	}
	from := strings.TrimSpace(env.CoordinatorAgentID)
	if from == "" {
		from = agentfeed.CoordinatorSeatID(env.ProjectRoot)
	}
	seats, err := env.loadSeats()
	if err != nil {
		return Result{}, err
	}
	var targets []string
	cfgTargets, _ := asList(stage.Config["seats"])
	for _, v := range cfgTargets {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			targets = append(targets, strings.TrimSpace(s))
		}
	}
	if len(targets) == 0 {
		for id, rec := range seats.Seats {
			if agentfeed.NormalizeWakeMembrane(rec.Wake) == agentfeed.WakeMembraneAgentAPI {
				targets = append(targets, id)
			}
		}
	}
	ev := map[string]any{"nonce": nonce, "from": from, "targets": targets, "dry_run": env.DryRun}
	if env.DryRun {
		ev["would"] = "COMMS-CHECK steer --await-peer-ack per target"
		return Result{OK: true, Evidence: ev}, nil
	}
	if env.Steer == nil {
		return Result{Evidence: ev}, errfmt.Errorf("comms_check: steer hook required")
	}
	var steers []map[string]any
	for _, to := range targets {
		msg := agentfeed.CommsCheckPrefix + " " + nonce + ": ack as " + to + "; echo nonce; whats-next probe; emit-status/steer back"
		out, err := env.Steer(ctx, to, msg, true)
		if err != nil {
			return Result{Evidence: ev}, errfmt.Newf("steer %s", to).Wrap(err)
		}
		steers = append(steers, map[string]any{
			"to": out.ToAgentID, "event_id": out.EventID, "await_id": out.AwaitID, "receipt": out.Receipt,
		})
	}
	ev["steers"] = steers
	return Result{OK: true, Evidence: ev}, nil
}

func execChatBootstrap(ctx context.Context, env *Env, stage Stage) (Result, error) {
	if env == nil {
		return Result{}, errfmt.Errorf("env required")
	}
	ev := map[string]any{"dry_run": env.DryRun, "allow_chat": env.AllowChat}
	if !env.AllowChat {
		if stage.Optional {
			return Result{Skipped: true, SkipWhy: "chat_bootstrap requires --allow-chat", Evidence: ev}, nil //nolint:gosec
		}
		return Result{Skipped: true, SkipWhy: "chat_bootstrap requires --allow-chat", Human: "allow-chat", Evidence: ev}, nil //nolint:gosec
	}
	payload := strings.TrimSpace(objects.GetString(stage.Config, "payload_path"))
	ev["payload_path"] = payload
	if env.DryRun {
		ev["would"] = "full chat-turn onboard per agentapi seat"
		return Result{OK: true, Evidence: ev}, nil
	}
	if env.ChatBootstrap == nil {
		return Result{Evidence: ev}, errfmt.Errorf("chat_bootstrap hook not wired")
	}
	seats, err := env.loadSeats()
	if err != nil {
		return Result{Evidence: ev}, err
	}
	n := 0
	for id, rec := range seats.Seats {
		if agentfeed.NormalizeWakeMembrane(rec.Wake) != agentfeed.WakeMembraneAgentAPI {
			continue
		}
		if err := env.ChatBootstrap(ctx, id, rec, payload); err != nil {
			return Result{Evidence: ev}, errfmt.Newf("chat bootstrap %s", id).Wrap(err)
		}
		n++
	}
	ev["bootstrapped"] = n
	return Result{OK: true, Evidence: ev}, nil
}

func execOrchestratePlan(ctx context.Context, env *Env, stage Stage) (Result, error) {
	if env == nil {
		return Result{}, errfmt.Errorf("env required")
	}
	planID := strings.TrimSpace(env.PlanID)
	if planID == "" {
		planID = strings.TrimSpace(objects.GetString(stage.Config, "plan_id"))
	}
	to := strings.TrimSpace(objects.GetString(stage.Config, "to_agent_id"))
	if to == "" {
		to = strings.TrimSpace(objects.GetString(stage.Config, "persona_ref"))
		if to != "" {
			to = agentfeed.WorkerSeatForPersona(env.ProjectRoot, to)
		}
	}
	ev := map[string]any{"plan_id": planID, "to": to, "dry_run": env.DryRun}
	if planID == "" {
		return Result{Evidence: ev}, errfmt.Errorf("orchestrate_plan: plan_id required (recipe config or --plan-id)")
	}
	msg := agentfeed.ComposeOrchestratePlanMessage(planID, objects.GetString(stage.Config, "extra"))
	if !agentfeed.IsOrchestratePlanBody(msg) {
		return Result{Evidence: ev}, errfmt.Errorf("orchestrate_plan: composed body is not ORCHESTRATE_PLAN")
	}
	ev["message"] = msg
	if env.DryRun {
		return Result{OK: true, Evidence: ev}, nil
	}
	if env.Steer == nil {
		return Result{Evidence: ev}, errfmt.Errorf("orchestrate_plan: steer hook required")
	}
	if to == "" {
		return Result{Evidence: ev}, errfmt.Errorf("orchestrate_plan: to_agent_id required")
	}
	out, err := env.Steer(ctx, to, msg, true)
	if err != nil {
		return Result{Evidence: ev}, err
	}
	ev["event_id"] = out.EventID
	ev["await_id"] = out.AwaitID
	return Result{OK: true, Evidence: ev}, nil
}
