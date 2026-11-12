package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/interactionpolicy"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

const (
	defaultMarkerFilename = "cap_loop_active"
	steeringSummaryFmt    = "- [%s from %s]: %s"
	steeringNudgeFmt      = " Pending steering inbox (%d unacked message(s)):\n%s."
)

type continuationHookOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

func newContinuationHookCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAgentHookContinuationCommandBuilder(), &cobra.Command{
		RunE: withHookProcessor(func(cmd *cobra.Command, proc *cli.Processor, in io.Reader, out io.Writer, sec *pkgctx.SecurityContext) error {
			flags := cmd.Flags()
			vendor, err := flags.GetString("vendor")
			if err != nil {
				return err
			}
			marker, err := flags.GetString("marker")
			if err != nil {
				return err
			}
			agentID, err := flags.GetString("agent-id")
			if err != nil {
				return err
			}
			personaRef, err := flags.GetString("persona-ref")
			if err != nil {
				return err
			}
			markerPath := marker
			if markerPath == "" {
				markerPath = filepath.Join(proc.ProjectRoot(), paths.ProjectDataDir, "state", defaultMarkerFilename)
			}
			return runContinuationHookWithDeps(proc.OperationContext(), sec, proc.Storage(), proc.ProjectRoot(), vendor, markerPath, agentID, personaRef, in, out)
		}),
	})
	return cmd
}

func runContinuationHookWithDeps(
	ctx context.Context,
	sec *pkgctx.SecurityContext,
	sp storage.ObjectStorageProvider,
	projectRoot string,
	vendor string,
	markerPath string,
	agentID string,
	personaRef string,
	in io.Reader,
	out io.Writer,
) error {
	if _, err := safeDrainInput(in); err != nil {
		return err
	}

	if markerPath == "" && projectRoot != "" {
		markerPath = filepath.Join(projectRoot, paths.ProjectDataDir, "state", defaultMarkerFilename)
	}

	capActive := markerExists(markerPath)
	planInfo := ""
	if sp != nil {
		planInfo = executionFacingPlanInfo(ctx, sec, sp)
		if planInfo != "" {
			capActive = true
		} else if !capActive {
			capActive = hasExecutionFacingPlan(ctx, sec, sp)
		}
	}

	targetAgentID := resolveContinuationSeatID(projectRoot, agentID)
	var steeringNudge string
	inboxN := 0
	outboxAwaitN := 0
	if targetAgentID != "" && projectRoot != "" {
		seat := agentfeed.Seat{
			AgentID:    targetAgentID,
			PersonaRef: strings.TrimSpace(personaRef),
		}
		if snap, err := agentfeed.LoadCorrespondence(projectRoot, seat, 5); err == nil {
			inboxN = len(snap.InboxUnacked)
			outboxAwaitN = len(snap.OutboxAwaitingPeerAck)
			if inboxN > 0 {
				var msgSummaries []string
				for _, item := range snap.InboxUnacked {
					msgSummaries = append(msgSummaries, fmt.Sprintf(steeringSummaryFmt, item.EventType, item.FromAgentID, item.Summary))
				}
				steeringNudge = fmt.Sprintf(steeringNudgeFmt, inboxN, strings.Join(msgSummaries, "\n"))
			}
		}
	}

	// Prefer live whats-next fill_item. Only rewrite the stale "autofix dangling"
	// wording to heal-dangling; do not invent a GhostRef fill when fill_item is nil.
	// Also classify ambience from live backlog counts — hardcoding EventIdle made
	// CAP followups always demand swarm-init even when planned=0 (stratplan_ahead),
	// which burned Cursor stop loop_limit then suspended the loop.
	fillHint := ""
	planned, inProgress, exploring, validated := 0, 0, 0, 0
	planStatus := ""
	planTitle := ""
	if projectRoot != "" {
		amb := whatsnext.LoadKernelAmbience(projectRoot)
		if fill := whatsnext.CompileFillItem(&amb); fill != nil {
			fillHint = strings.TrimSpace(fill.CommandHint)
			if strings.Contains(fillHint, "autofix dangling") {
				fillHint = paths.CLICommandName + " system kernel-integrity heal-dangling --apply"
			}
		}
		if sp != nil {
			wn, wnErr := whatsnext.Execute(ctx, sp, projectRoot, &whatsnext.QueryRequest{
				SkipMeasure:       true,
				SkipPersonaFilter: true,
				AgentID:           targetAgentID,
			})
			if wnErr == nil && wn != nil {
				if wn.BacklogCountsByStatus != nil {
					planned = wn.BacklogCountsByStatus["planned"]
					inProgress = wn.BacklogCountsByStatus["in_progress"]
					exploring = wn.BacklogCountsByStatus["exploring"]
					validated = wn.BacklogCountsByStatus["validated"]
				}
				if wn.PriorityPlan != nil {
					planStatus = strings.TrimSpace(wn.PriorityPlan.Status)
					planTitle = strings.TrimSpace(wn.PriorityPlan.Title)
				}
				if fillHint == "" && wn.FillItem != nil {
					fillHint = strings.TrimSpace(wn.FillItem.CommandHint)
				}
			}
		}
	}
	ev := interactionpolicy.ClassifyAmbience(interactionpolicy.Ambience{
		InboxUnacked: inboxN,
		Planned:      planned,
		InProgress:   inProgress,
		Exploring:    exploring,
		Validated:    validated,
		PlanStatus:   planStatus,
	})
	res := interactionpolicy.Evaluate(nil, ev)
	step := ""
	hint := ""
	pol := ""
	if res.Matched && res.Step != nil {
		step = res.Step.GuidingStep
		hint = res.Step.CommandHint
		pol = res.Step.PolicyID
	}
	gut := interactionpolicy.CompileProductDeltaGutCheck(interactionpolicy.ProductDeltaSignals{
		LeadTitleTheater: interactionpolicy.TitleLooksLikeResidualTheater(planTitle),
		// WindowCommits left 0 unless git signals are available — LeadTitleTheater still bites.
	})
	drive := interactionpolicy.CompileTurnContinue(interactionpolicy.TurnContinueInput{
		InboxUnacked:          inboxN,
		OutboxAwaitingPeerAck: outboxAwaitN,
		GuidingEvent:          ev,
		GuidingStep:           step,
		CommandHint:           hint,
		PolicyID:              pol,
		CapLoopActive:         capActive,
		SteeringNudge:         steeringNudge,
		FillItemCommandHint:   fillHint,
		GutCheck:              gut,
	})
	if !drive.Continue {
		return emitContinuationDecision(out, vendor, "allow", "")
	}
	reason := drive.Message
	if planInfo != "" && !strings.Contains(reason, planInfo) {
		reason = planInfo + ". " + reason
	}
	reason = paths.RewriteCanonicalCLIInvocations(reason)
	return emitContinuationDecision(out, vendor, "continue", reason)
}

func hasExecutionFacingPlan(ctx context.Context, sec *pkgctx.SecurityContext, sp storage.ObjectStorageProvider) bool {
	return executionFacingPlanInfo(ctx, sec, sp) != ""
}

func executionFacingPlanInfo(ctx context.Context, sec *pkgctx.SecurityContext, sp storage.ObjectStorageProvider) string {
	for _, status := range []string{objects.ObjectStatusInProgress, objects.ObjectStatusActive} {
		filter := storage.ListFilter{
			Kind: objects.KindPriorityPlan,
			Filters: map[string]any{
				objects.FieldKeyStatus: status,
			},
		}
		res, err := sp.List(ctx, sec, nil, filter)
		if err != nil || len(res.Objects) == 0 {
			continue
		}
		first := res.Objects[0]
		pID, _ := first[objects.FieldKeyID].(string)
		title, _ := first[objects.FieldKeyTitle].(string)
		pID = strings.TrimSpace(pID)
		title = strings.TrimSpace(title)
		if pID == "" {
			continue
		}
		if title == "" {
			return pID
		}
		return fmt.Sprintf("Active plan in progress: %s (%s)", title, pID)
	}
	return ""
}

func resolveContinuationSeatID(projectRoot, agentID string) string {
	if id := strings.TrimSpace(agentID); id != "" {
		return id
	}
	if id := strings.TrimSpace(os.Getenv("ZQK_AGENT_ID")); id != "" {
		return id
	}
	if id := strings.TrimSpace(os.Getenv("ZQK_SEAT_ID")); id != "" {
		return id
	}
	if projectRoot != "" {
		if id := strings.TrimSpace(agentfeed.CoordinatorSeatID(projectRoot)); id != "" {
			return id
		}
		if id := strings.TrimSpace(agentfeed.WorkerSeatID(projectRoot)); id != "" {
			return id
		}
	}
	return "coordinator"
}

// emitContinuationDecision maps the generic continue/allow decision onto vendor
// host contracts. Cursor stop hooks require followup_message (not decision/message).
func emitContinuationDecision(out io.Writer, vendor string, decision string, reason string) error {
	v := strings.ToLower(strings.TrimSpace(vendor))
	switch v {
	case "claude":
		d := decision
		if d == "continue" {
			d = "block"
		}
		output := map[string]any{"decision": d}
		if reason != "" {
			output["reason"] = reason
		}
		return encodeJSON(out, output)
	case "cursor":
		// Cursor Agent `stop` hook: empty object ends the turn; followup_message
		// is auto-submitted as the next user message (host specialization only).
		if decision != "continue" || strings.TrimSpace(reason) == "" {
			return encodeJSON(out, map[string]any{})
		}
		return encodeJSON(out, map[string]any{
			"followup_message":   reason,
			"additional_context": reason,
		})
	case "windsurf", "cascade":
		output := map[string]any{"decision": decision}
		if reason != "" {
			output["reason"] = reason
			output["explanation"] = reason
		}
		return encodeJSON(out, output)
	case "cline", "roo":
		output := map[string]any{"decision": decision}
		if reason != "" {
			output["reason"] = reason
		}
		return encodeJSON(out, output)
	default:
		output := continuationHookOutput{
			Decision: decision,
			Reason:   reason,
		}
		return encodeJSON(out, output)
	}
}

func encodeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func markerExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := fileutil.Stat(path)
	return err == nil
}
