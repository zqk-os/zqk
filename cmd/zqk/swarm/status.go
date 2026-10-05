package swarm

import (
	"context"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	capOrchestratorJobID = objects.JobIDCapOrchestrator
	swarmStatusMaxSample = 12
)

// NewStatusCmd creates swarm status.
func NewStatusCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSwarmStatusCommandBuilder(), &cobra.Command{
		RunE: runSwarmStatus,
	})
	cli.AddCommonFlags(cmd)
	return cmd
}

func runSwarmStatus(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		sec := pkgctx.NewSystemSecurityContext()
		sp := proc.Storage()
		if sp == nil {
			return errfmt.Errorf("storage unavailable")
		}

		payload, err := BuildSwarmStatus(ctx, sp, sec)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("swarm status failed", err).Log()
			return err
		}
		return cli.FormatOutput(cmd, payload)
	})(cmd, args)
}

// BuildSwarmStatus compiles the swarm throughput, agent tasks, instructions, and orchestration status.
func BuildSwarmStatus(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) (map[string]any, error) {
	atkByStatus, atkTotal, err := countByStatus(ctx, sp, sec, objects.KindAgentTask)
	if err != nil {
		return nil, err
	}
	agiByStatus, agiTotal, err := countByStatus(ctx, sp, sec, objects.KindAgentInstruction)
	if err != nil {
		return nil, err
	}

	activePlans, err := countWithFilter(ctx, sp, sec, objects.KindPriorityPlan, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	})
	if err != nil {
		logging.FluentEvent(logging.GetLogger()).Debug("swarm status: priority_plan count").WithError(err).Log()
		activePlans = -1
	}

	personaReady := summarizePersonaSkillBound(ctx, sp, sec)

	capPresent := false
	if _, err := sp.Read(ctx, sec, capOrchestratorJobID); err == nil {
		capPresent = true
	}

	executing := 0
	for _, st := range []string{
		objects.ObjectStatusInProgress,
		objects.ObjectStatusExecuting,
		objects.ObjectStatusPendingVerification,
		"routed",
	} {
		executing += atkByStatus[st]
	}

	throughputHint := "idle"
	switch {
	case executing > 0:
		throughputHint = "executing"
	case atkByStatus[objects.ObjectStatusProposed] > 0 || atkByStatus[objects.ObjectStatusApproved] > 0:
		throughputHint = "queued"
	case agiTotal > 0 && agiByStatus[objects.ObjectStatusProposed] > 0:
		throughputHint = "instructions_pending"
	}

	return map[string]any{
		"swarm": map[string]any{
			"throughput_hint":              throughputHint,
			"executing_agent_tasks":        executing,
			"agent_tasks_total":            atkTotal,
			"agent_tasks_by_status":        atkByStatus,
			"agent_instructions_total":     agiTotal,
			"agent_instructions_by_status": agiByStatus,
			"active_priority_plans":        activePlans,
			"persona_skill_bound":          personaReady,
			"cap_orchestrator_job": map[string]any{
				objects.FieldKeyID: capOrchestratorJobID,
				"present":          capPresent,
				"history":          paths.CLIUsage("scheduler", "history", "--job-id", capOrchestratorJobID),
			},
			"related_commands": []string{
				paths.CLIUsage("agent", "status"),
				paths.CLIUsage("feed", "pending"),
				paths.CLIUsage("workflow", "whats-next", "--format", "json"),
				paths.CLIUsage("scheduler", "history", "--job-id", capOrchestratorJobID),
			},
		},
	}, nil
}

func countByStatus(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, kind string) (map[string]int, int, error) {
	res, err := sp.List(ctx, sec, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind:    kind,
		GroupBy: objects.FieldKeyStatus,
		Limit:   0,
	})
	if err != nil {
		res, err = sp.List(ctx, sec, pkgctx.NewStorageContext(), storage.ListFilter{Kind: kind, Limit: 0})
		if err != nil {
			return nil, 0, err
		}
	}
	by := map[string]int{}
	total := 0
	if res != nil && len(res.Groups) > 0 {
		for key, objs := range res.Groups {
			k := strings.TrimSpace(key)
			if k == "" {
				k = "(empty)"
			}
			by[k] = len(objs)
			total += len(objs)
		}
		return by, total, nil
	}
	if res == nil {
		return by, 0, nil
	}
	for _, o := range res.Objects {
		st, _ := o[objects.FieldKeyStatus].(string)
		st = strings.TrimSpace(st)
		if st == "" {
			st = "(empty)"
		}
		by[st]++
		total++
	}
	return by, total, nil
}

func countWithFilter(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, kind string, filters map[string]any) (int, error) {
	return sp.Count(ctx, sec, storage.ListFilter{Kind: kind, Filters: filters})
}

func summarizePersonaSkillBound(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) map[string]any {
	// Structural dual-read (agent_skill_refs + related ASK-*). Full resolve gate lands with
	// CRI-PERSONA-SKILL-BOUND enforce PR; status still surfaces bind rate for throughput ops.
	res, err := sp.List(ctx, sec, pkgctx.NewStorageContext(), storage.ListFilter{Kind: objects.KindPersona, Limit: 0})
	out := map[string]any{
		"total":   0,
		"bound":   0,
		"unbound": 0,
	}
	if err != nil || res == nil {
		out["error"] = "list_failed"
		return out
	}
	unboundSample := []string{}
	bound, unbound := 0, 0
	for _, p := range res.Objects {
		if len(personaASKRefs(p)) > 0 {
			bound++
			continue
		}
		unbound++
		if len(unboundSample) < swarmStatusMaxSample {
			id, _ := p[objects.FieldKeyID].(string)
			unboundSample = append(unboundSample, id)
		}
	}
	sort.Strings(unboundSample)
	out["total"] = len(res.Objects)
	out["bound"] = bound
	out["unbound"] = unbound
	out["unbound_sample"] = unboundSample
	return out
}

func personaASKRefs(persona map[string]any) []string {
	seen := map[string]struct{}{}
	var out []string
	var appendASK func(raw any)
	appendASK = func(raw any) {
		switch t := raw.(type) {
		case string:
			for _, part := range strings.Split(t, ",") {
				id := strings.TrimSpace(part)
				if !strings.HasPrefix(strings.ToUpper(id), "ASK-") {
					continue
				}
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				out = append(out, id)
			}
		case []string:
			for _, s := range t {
				appendASK(s)
			}
		case []any:
			for _, item := range t {
				appendASK(item)
			}
		}
	}
	appendASK(persona[objects.FieldKeyRelatedObjectRefs])
	appendASK(persona[objects.FieldKeyAgentSkillRefs])
	return out
}
