package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"

	"github.com/zqk-os/zqk/pkg/config"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentclaim"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/swarm"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqksession"
)

const (
	seatWorkerOutputFieldStatus = "status"
	seatWorkerOutputStatusOK    = "ok"
	orchestratePlanDirective    = agentfeed.OrchestratePlanPrefix
	localLLMAPIKey              = "ollama"
	// seatWorkerMaxEventAttempts caps AgentX retries for one ATK (or one feed
	// event when the steer has no ATK). A reminted AFE for the same ATK must
	// not reset the budget.
	seatWorkerMaxEventAttempts = 3
	// seatWorkerAgentXRunTimeout bounds one cognitive run independently of the
	// 24-hour daemon lifetime. Without it, maxSteps × per-request LLM timeout
	// can let one event monopolize a seat for hours and starve its inbox.
	seatWorkerAgentXRunTimeout = 10 * time.Minute
	// planOrchSubmitCooldown avoids stacking agent-orchestrate jobs when the
	// coordinator remints ORCHESTRATE_PLAN on an empty inbox.
	planOrchSubmitCooldown = 45 * time.Minute
)

// Night-duty / CAP steers name the ATK to claim; AgentX must load that object
// rather than improvising from the short wake text alone.
var seatWorkerATKIDPattern = regexp.MustCompile(`\bATK-[0-9]+-[0-9a-fA-F]+\b`)

type seatWorkerPromptBundle struct {
	System           string
	User             string
	CognitionPersona string
	TaskID           string
	MaxSteps         int
	WorkClass        agentprompt.WorkClass
	ExecRoot         string
}

type seatWorkerPriorityProbe func(context.Context) (string, error)

func resolveSeatWorkerPriority(ctx context.Context, inboxCount int, probe seatWorkerPriorityProbe) (string, error) {
	if inboxCount == 0 || probe == nil {
		return "", nil
	}
	return probe(ctx)
}

// NewSeatWorkerCmd creates zqk agent seat-worker.
func NewSeatWorkerCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentSeatWorkerCommandBuilder()
	cmd.RunE = runAgentSeatWorker
	return cmd
}

func runAgentSeatWorker(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root := proc.ProjectRoot()
		if root == "" {
			if envRoot := zqkenv.ProjectRoot().Get(); envRoot != "" && paths.IsAgentWorktreePath(envRoot) {
				return errfmt.Errorf("seat-worker cannot run with ZQK_PROJECT_ROOT set to agent worktree %s without seated kernel configuration", envRoot)
			}
			return errfmt.Errorf("project root not found")
		}
		if paths.IsAgentWorktreePath(root) {
			return errfmt.Errorf("seat-worker cannot run with project root inside agent worktree %s", root)
		}
		var flags clipkg.FlagBag
		agentID := strings.TrimSpace(flags.String(cmd, "agent-id"))
		personaRef := strings.TrimSpace(flags.String(cmd, "persona-ref"))
		replyTo := strings.TrimSpace(flags.String(cmd, "reply-to-agent-id"))
		pollSec := flags.Int(cmd, "poll-seconds")
		once := flags.Bool(cmd, "once")
		executeNonComms := flags.Bool(cmd, "execute-non-comms")
		if err := flags.Err(); err != nil {
			return err
		}
		if agentID == "" {
			return errfmt.Errorf("--agent-id is required")
		}
		if personaRef == "" {
			personaRef = objects.ConstPersonaDefaultOperator
		}
		if pollSec <= 0 {
			pollSec = 5
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		sp := proc.GetStorageProvider()
		secCtx := proc.SecurityContext()
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		accountID := strings.TrimSpace(secCtx.AccountID)
		if accountID == "" {
			accountID = pkgctx.SystemAccountID
		}

		// One fresh zqk_session per worker process.
		llmCfg := llm.DefaultConfig(cmd.Context())
		parentSessionID := zqksession.GetIDFromContext(cmd.Context())
		workerSessionID := ""
		sessionEnded := false
		endWorkerSession := func(status string) {
			if sessionEnded || workerSessionID == "" || sp == nil {
				return
			}
			sessionEnded = true
			zqksession.End(cmd.Context(), root, workerSessionID, status, accountID, sp)
		}
		if sp != nil {
			sid, sessErr := zqksession.StartWorkerSession(cmd.Context(), zqksession.WorkerSessionInput{
				ProjectRoot:     root,
				AccountID:       accountID,
				Title:           "seat-worker " + seatWorkerLane(personaRef, agentID),
				ParentSessionID: parentSessionID,
				AgentID:         agentID,
				PersonaRef:      personaRef,
				ExecutorType:    zqksession.ExecutorTypeAgentX,
				Provider:        llmCfg.Provider,
				ModelID:         llmCfg.ChatModel,
			}, sp)
			if sessErr != nil {
				return errfmt.Errorf("start worker session: %w", sessErr)
			}
			workerSessionID = sid
		}
		defer endWorkerSession(zqksession.StatusCompleted)

		logging.Fluent(logger).Info("seat-worker starting").
			String("agent_id", agentID).
			String("persona_ref", personaRef).
			String(objects.FieldKeySessionID, workerSessionID).
			String(objects.FieldKeyProvider, llmCfg.Provider).
			String(objects.FieldKeyModelID, llmCfg.ChatModel).
			Bool("once", once).
			Bool("execute_non_comms", executeNonComms).
			Log()

		runOnce := func(ctx context.Context) (map[string]any, error) {
			snap, err := agentfeed.LoadCorrespondence(root, agentfeed.Seat{
				AgentID:    agentID,
				PersonaRef: personaRef,
			}, 100)
			if err != nil {
				return nil, errfmt.Errorf("load correspondence: %w", err)
			}
			inboxCount := len(snap.InboxUnacked)
			if inboxCount == 0 {
				dispatch := map[string]any{}
				if executeNonComms && agentfeed.IsCoordinatorDutySeat(root, agentID, personaRef) {
					if d, dErr := dispatchLeadPlanFromWhatsNext(ctx, logger, sp, root, agentID, personaRef); dErr != nil {
						logging.Fluent(logger).Warn("seat-worker lead-plan dispatch failed").
							WithError(errfmt.Newf("dispatch").Wrap(dErr)).
							Log()
					} else if d.EventID != "" || d.Skipped != "" || d.FillKind != "" {
						dispatch["lead_plan_event_id"] = d.EventID
						dispatch["lead_plan_skipped"] = d.Skipped
						dispatch["fill_kind"] = d.FillKind
						dispatch["fill_command"] = d.FillHint
						dispatch["fill_submitted"] = d.FillSubmitted
					}
				}
				alivePath, writeErr := agentfeed.WriteSeatWorkerAlive(root, agentID, workerSessionID, time.Time{})
				if writeErr != nil {
					return nil, writeErr
				}
				out := map[string]any{
					seatWorkerOutputFieldStatus: seatWorkerOutputStatusOK,
					objects.FieldKeyAgentID:     agentID,
					objects.FieldKeySessionID:   workerSessionID,
					"alive_path":                alivePath,
					"inbox":                     0,
				}
				for k, v := range dispatch {
					if s, ok := v.(string); ok && s != "" {
						out[k] = v
					}
				}
				return out, nil
			}

			priorityID, probeErr := resolveSeatWorkerPriority(ctx, inboxCount, func(ctx context.Context) (string, error) {
				if sp == nil {
					return "", nil
				}
				out, probeErr := whatsnext.Execute(ctx, sp, root, &whatsnext.QueryRequest{
					SkipMeasure: true,
					PersonaID:   personaRef,
					AgentID:     agentID,
				})
				if probeErr != nil {
					return "", probeErr
				}
				if out == nil || out.PriorityPlan == nil {
					return "", nil
				}
				return out.PriorityPlan.ID, nil
			})
			if probeErr != nil {
				logging.Fluent(logger).Warn("whats-next probe failed").
					WithError(errfmt.Newf("whats-next").Wrap(probeErr)).
					Log()
			}

			opts := agentfeed.SeatWorkerOptions{
				ProjectRoot:    root,
				AgentID:        agentID,
				PersonaRef:     personaRef,
				ReplyToAgentID: replyTo,
				SessionID:      workerSessionID,
				PriorityID:     priorityID,
				InboxCount:     inboxCount,
			}
			// Named-ATK hourglasses run even when --execute-non-comms is off.
			opts.OnNamedATK = func(ctx context.Context, item agentfeed.CorrespondenceItem, body string) error {
				return handleNonCommsWithAgentX(ctx, logger, sp, secCtx, root, agentID, personaRef, workerSessionID, item, body)
			}
			if executeNonComms {
				opts.OnNonComms = func(ctx context.Context, item agentfeed.CorrespondenceItem, body string) error {
					return handleNonCommsWithAgentX(ctx, logger, sp, secCtx, root, agentID, personaRef, workerSessionID, item, body)
				}
			}

			res, err := agentfeed.ProcessSeatInbox(ctx, opts)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{
				seatWorkerOutputFieldStatus: seatWorkerOutputStatusOK,
				objects.FieldKeyAgentID:     agentID,
				objects.FieldKeySessionID:   workerSessionID,
				"processed_comms":           res.ProcessedCOMMS,
				objects.FieldKeySkipped:     res.Skipped,
				"non_comms_event_ids":       res.NonComms,
				"event_ids":                 res.EventIDs,
				"alive_path":                res.AlivePath,
				"priority_id":               priorityID,
				"inbox":                     inboxCount,
			}
			if len(res.Errors) > 0 {
				payload["errors"] = res.Errors
			}
			logging.Fluent(logger).Info("seat-worker pass").
				Int("processed_comms", res.ProcessedCOMMS).
				Int("skipped", res.Skipped).
				Int("non_comms", len(res.NonComms)).
				Int("inbox", inboxCount).
				Log()
			return payload, nil
		}

		if once {
			payload, err := runOnce(cmd.Context())
			if err != nil {
				endWorkerSession(zqksession.StatusError)
				return err
			}
			return cli.FormatOutput(cmd, payload)
		}

		for {
			if _, err := runOnce(cmd.Context()); err != nil {
				logging.Fluent(logger).Warn("seat-worker pass failed").WithError(errfmt.Newf("pass").Wrap(err)).Log()
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case <-time.After(time.Duration(pollSec) * time.Second):
			}
		}
	})(cmd, nil)
}

// seatWorkerMCPServePath prefers the running binary so MCP tool names and
// mutation-evidence prefixes stay on the same executable. bin/zqk is last
// resort when this process has no resolvable path (tests).
func seatWorkerMCPServePath(root string) string {
	if self, err := fileutil.Executable(); err == nil {
		if trimmed := strings.TrimSpace(self); trimmed != "" {
			return trimmed + " mcp serve"
		}
	}
	if binPath := strings.TrimSpace(zqkenv.Bin().Get()); binPath != "" {
		return binPath + " mcp serve"
	}
	stable := filepath.Join(root, "bin", "zqk-stable")
	if _, err := fileutil.Stat(stable); err == nil {
		return stable + " mcp serve"
	}
	return filepath.Join(root, "bin", "zqk") + " mcp serve"
}

// handleNonCommsWithAgentX peer-acks the steer then runs a bounded swarm.Engine pass
// on the feed body (non-COMMS path).
//
// Cognition uses the ATK assignee persona when the steer names an ATK; mesh ack
// permanently keeps the seat persona so feed identity and task expertise remain
// distinct when night-duty assigns coder work to PER-ORCH-* seats.
func handleNonCommsWithAgentX(
	ctx context.Context,
	logger logging.Logger,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	root, agentID, personaRef, sessionID string,
	item agentfeed.CorrespondenceItem,
	body string,
) error {
	if planID, ok, err := parseOrchestratePlanDirective(body); ok {
		if err != nil {
			return err
		}
		result, submitErr := triggerPlanOrchestration(ctx, sp, secCtx, root, planID)
		writeSeatWorkerResult(root, seatWorkerEngineID(personaRef, agentID), result)
		if submitErr != nil {
			if strings.Contains(submitErr.Error(), "orchestrator can only operate on active or in_progress priority plans") {
				return ackSeatWorkerSkip(root, agentID, personaRef, sessionID, item.EventID, submitErr.Error())
			}
			return submitErr
		}
		kind := "SEAT_WORKER_ORCHESTRATE_SUBMITTED"
		if strings.HasPrefix(result, "no_live_atk") || strings.HasPrefix(result, "already_submitted") {
			kind = "SEAT_WORKER_ORCHESTRATE_SKIPPED"
		}
		summary := fmt.Sprintf("%s %s — %s", kind, item.EventID, planID)
		if _, err := agentfeed.AppendPeerAckWithSession(root, agentID, personaRef, sessionID, item.EventID, summary); err != nil {
			return errfmt.Errorf("orchestration ack: %w", err)
		}
		return nil
	}

	taskID := extractATKIDFromSteer(body)
	if taskID == "" {
		return ackSeatWorkerSkip(root, agentID, personaRef, sessionID, item.EventID,
			"refuse cold AgentX: steer has no ATK")
	}

	// Occupancy before cognition — in_progress without claimed_by is not exclusive work.
	if sp != nil && strings.TrimSpace(agentID) != "" {
		claimRes, claimErr := agentclaim.TryClaim(ctx, sp, secCtx, taskID, agentID, agentclaim.ClaimOptions{
			ProjectRoot:    root,
			HourglassOn:    true,
			CheckinCadence: agentclaim.DefaultCheckinCadence,
		})
		if claimErr != nil {
			return ackSeatWorkerSkip(root, agentID, personaRef, sessionID, item.EventID,
				fmt.Sprintf("refuse AgentX: claim %s failed (%s): %v", taskID, claimRes.Reason, claimErr))
		}
		logging.Fluent(logger).Info("seat-worker claimed ATK").
			String("task_id", taskID).
			String(objects.FieldKeyClaimedBy, claimRes.ClaimedBy).
			String("reason", claimRes.Reason).
			Log()
	}

	attemptKey := agentfeed.SeatAttemptKey(item.EventID, taskID)
	if n := agentfeed.SeatEventAttemptCount(root, agentID, attemptKey); n >= seatWorkerMaxEventAttempts {
		parked := fmt.Sprintf("SEAT_WORKER_FAILED %s — already at %d attempts for %s; not re-run", item.EventID, n, attemptKey)
		if _, err := agentfeed.AppendPeerAckWithSession(root, agentID, personaRef, sessionID, item.EventID, parked); err != nil {
			return errfmt.Errorf("non-comms exhausted ack: %w", err)
		}
		logging.Fluent(logger).Warn("seat-worker AgentX event parked").
			String("event_id", item.EventID).
			Int("attempts", n).
			Log()
		return nil
	}

	// Coding ATKs isolate into a git worktree. Docs-eval / CEF ATKs must not:
	// worktrees reset to origin/main and hide untracked cef-runs + studio docs.
	execRoot := root
	workClass := agentprompt.ClassifyWorkClass(body)
	if sp != nil {
		if task, readErr := sp.Read(ctx, secCtx, taskID); readErr == nil {
			desc, _ := task[objects.FieldKeyDescription].(string)
			title, _ := task[objects.FieldKeyTitle].(string)
			workClass = agentprompt.ClassifyWorkClass(body, desc, title)
		}
	}
	if workClass.IsDocsEval() {
		logging.Fluent(logger).Info("seat-worker AgentX using studio exec root").
			String("event_id", item.EventID).
			String("task_id", taskID).
			String("work_class", string(workClass)).
			Log()
	} else if wt, wtErr := ensureOrchestrationWorktree(ctx, root, taskID); wtErr != nil {
		return errfmt.Errorf("isolate ATK worktree: %w", wtErr)
	} else if seedErr := seedAgentWorktreeRuntime(root, wt); seedErr != nil {
		return errfmt.Errorf("seed ATK worktree: %w", seedErr)
	} else {
		execRoot = wt
	}

	summary := fmt.Sprintf("SEAT_WORKER_RECEIVED %s — AgentX", item.EventID)
	runCtx, cancelRun := context.WithTimeout(ctx, seatWorkerAgentXRunTimeout)
	defer cancelRun()

	prompts, err := buildSeatWorkerAgentXPrompts(runCtx, logger, sp, secCtx, execRoot, agentID, personaRef, item.EventID, body)
	if err != nil {
		if isPreparedContextRefusal(err) {
			return ackSeatWorkerSkip(root, agentID, personaRef, sessionID, item.EventID,
				"refuse cold AgentX: "+err.Error())
		}
		return errfmt.Errorf("non-comms AgentX prompt: %w", err)
	}

	mcpPath := seatWorkerMCPServePath(root)

	executor, err := swarm.NewMCPExecutorAt(runCtx, mcpPath, execRoot)
	if err != nil {
		return errfmt.Errorf("non-comms AgentX mcp: %w", err)
	}
	defer executor.Close()

	config := llm.DefaultConfig(runCtx)
	if llm.IsCodeDraftModel(config.ChatModel) && config.Temperature == nil {
		zero := 0.0
		config.Temperature = &zero
	}
	client := llm.NewClient(runCtx, config)
	engineID := seatWorkerEngineID(personaRef, agentID)
	// Count alone is too weak: live seats "completed" after system_status /
	// object_list. Coding ATKs require a successful write_code or write_file,
	// and that write must actually build and pass its package tests.
	var writtenGoFiles []string
	engine := swarm.NewEngine(client, executor, swarm.PreserveToolSchemas, engineID).
		WithRunIdentity(sessionID, prompts.TaskID).
		WithMaxSteps(prompts.MaxSteps).
		RequireSuccessfulToolCalls(1).
		RequireAnySuccessfulTools(swarm.MutationEvidenceTools()...).
		VerifyCompletionWith(func(ctx context.Context, history []swarm.ToolCallRecord) (string, error) {
			if prompts.WorkClass.IsDocsEval() {
				// Docs-eval / CEF tasks write evaluation JSONL/markdown under docs/quality/, not Go source code.
				return "", nil
			}
			writtenGoFiles = goFilesWritten(history, execRoot)
			return verifyGoWorkAsCompletion(ctx, execRoot, history)
		}, seatWorkerMaxRepairs)
	if swarm.ApplyCodeDraftHarness(engine, config.ChatModel) {
		logging.Fluent(logger).Warn("seat-worker using code-draft harness (write-only tools)").
			String(objects.FieldKeyModelID, config.ChatModel).
			Log()
	}

	logging.Fluent(logger).Info("seat-worker AgentX starting").
		String("event_id", item.EventID).
		String("engine_id", engineID).
		String(objects.FieldKeySessionID, sessionID).
		String("task_id", prompts.TaskID).
		String("cognition_persona", prompts.CognitionPersona).
		String(objects.FieldKeyModelID, config.ChatModel).
		Log()

	result, runErr := engine.Run(runCtx, prompts.System, prompts.User)
	writeSeatWorkerResult(root, engineID, result)
	if runErr == nil && llm.IsMockFallback(result) {
		runErr = errfmt.Errorf("AgentX unavailable: %s", result)
	}
	if runErr != nil {
		attempts, ledgerErr := agentfeed.RecordSeatEventAttempt(root, agentID, attemptKey)
		if ledgerErr != nil {
			logging.Fluent(logger).Warn("seat-worker attempt ledger failed").
				String("event_id", item.EventID).
				WithError(errfmt.Newf("ledger").Wrap(ledgerErr)).
				Log()
			return errfmt.Errorf("non-comms AgentX run: %w", runErr)
		}
		if attempts < seatWorkerMaxEventAttempts {
			return errfmt.Errorf("non-comms AgentX run (attempt %d/%d): %w", attempts, seatWorkerMaxEventAttempts, runErr)
		}
		// The run is being abandoned, so its half-finished code must not stay in
		// the tree for the next agent or human to trip over.
		if len(writtenGoFiles) > 0 {
			if revertErr := revertSeatWrites(ctx, root, writtenGoFiles); revertErr != nil {
				logging.Fluent(logger).Warn("seat-worker revert failed").
					String("event_id", item.EventID).
					WithError(errfmt.Newf("revert").Wrap(revertErr)).
					Log()
			} else {
				logging.Fluent(logger).Info("seat-worker reverted rejected writes").
					String("event_id", item.EventID).
					Int("files", len(writtenGoFiles)).
					Log()
			}
		}
		parked := fmt.Sprintf("SEAT_WORKER_FAILED %s — AgentX gave up after %d attempts: %v", item.EventID, attempts, runErr)
		if _, err := agentfeed.AppendPeerAckWithSession(root, agentID, personaRef, sessionID, item.EventID, parked); err != nil {
			return errfmt.Errorf("non-comms failure ack: %w", err)
		}
		logging.Fluent(logger).Warn("seat-worker AgentX event parked").
			String("event_id", item.EventID).
			String("engine_id", engineID).
			Int("attempts", attempts).
			WithError(errfmt.Newf("run").Wrap(runErr)).
			Log()
		return nil
	}
	if err := agentfeed.ClearSeatEventAttempts(root, agentID, attemptKey); err != nil {
		logging.Fluent(logger).Warn("seat-worker attempt ledger clear failed").
			String("event_id", item.EventID).
			WithError(errfmt.Newf("ledger").Wrap(err)).
			Log()
	}
	if _, err := agentfeed.AppendPeerAckWithSession(root, agentID, personaRef, sessionID, item.EventID, summary); err != nil {
		return errfmt.Errorf("non-comms ack: %w", err)
	}
	logging.Fluent(logger).Info("seat-worker AgentX finished").
		String("event_id", item.EventID).
		String("engine_id", engineID).
		String("task_id", prompts.TaskID).
		Log()
	return nil
}

func extractATKIDFromSteer(body string) string {
	if id := agentfeed.NamedATKID(body); id != "" {
		return id
	}
	return seatWorkerATKIDPattern.FindString(body)
}

func ackSeatWorkerSkip(root, agentID, personaRef, sessionID, eventID, reason string) error {
	summary := fmt.Sprintf("SEAT_WORKER_SKIPPED %s — %s", eventID, reason)
	if _, err := agentfeed.AppendPeerAckWithSession(root, agentID, personaRef, sessionID, eventID, summary); err != nil {
		return errfmt.Errorf("seat-worker skip ack: %w", err)
	}
	return nil
}

// seatWorkerWorkClass classifies AgentX cognition/exec-root from steer + ATK text.
func seatWorkerWorkClass(steerBody string) agentprompt.WorkClass {
	return agentprompt.ClassifyWorkClass(steerBody)
}

func buildSeatWorkerAgentXPrompts(
	ctx context.Context,
	logger logging.Logger,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	root, agentID, seatPersona, eventID, body string,
) (seatWorkerPromptBundle, error) {
	out := seatWorkerPromptBundle{CognitionPersona: strings.TrimSpace(seatPersona)}
	steer := strings.TrimSpace(body)
	taskID := extractATKIDFromSteer(steer)
	out.TaskID = taskID
	if taskID == "" {
		return out, errPreparedContextRequired
	}

	prepared, err := AssemblePreparedContext(ctx, secCtx, sp, PreparedContextInput{
		TaskID:      taskID,
		Persona:     out.CognitionPersona,
		ProjectRoot: root,
		Depth:       1,
		IncludeTDD:  true, // AssemblePreparedContext clears TDD for docs_eval
	})
	if err != nil {
		return out, err
	}
	if prepared.Persona != "" {
		out.CognitionPersona = prepared.Persona
	}
	if out.CognitionPersona == "" {
		out.CognitionPersona = strings.TrimSpace(seatPersona)
	}

	workClass := agentprompt.ClassifyWorkClass(steer, prepared.Title, prepared.Prompt)
	out.WorkClass = workClass
	out.ExecRoot = root
	caps := []string{"coding", "review"}
	if workClass.IsDocsEval() {
		caps = []string{"docs_eval", "cef"}
	}
	systemPrompt, err := swarm.RenderSystemPrompt(swarm.QwenSystemData{
		WorkerID:     agentID,
		Capabilities: caps,
		WorkClass:    string(workClass),
	})
	if err != nil {
		return out, err
	}
	out.System = systemPrompt
	clippedSteer := clipSeatWorkerPromptPart(steer, seatWorkerSteerCap, "STEER", workClass)
	clippedPrepared := clipSeatWorkerPromptPart(prepared.Prompt, seatWorkerPreparedPromptCap, "PREPARED CONTEXT", workClass)
	out.User = fmt.Sprintf(
		"Seat %s / feed event %s\nwork_class=%s\nSteer:\n%s\n\n## Prepared kernel context\n%s\n\nRequired: work this ATK from the prepared context; do not invent tools absent from Available Tools; when finished, summarize with NO further tool calls.",
		agentID, eventID, workClass, clippedSteer, clippedPrepared,
	)
	if logger != nil {
		logging.Fluent(logger).Info("seat-worker prepared context").
			String("task_id", taskID).
			String("cognition_persona", out.CognitionPersona).
			Int("dependency_count", prepared.DependencyCount).
			Int("steer_bytes", len(steer)).
			Int("prepared_bytes", len(prepared.Prompt)).
			Int("user_prompt_bytes", len(out.User)).
			Log()
	}
	out.MaxSteps = 30
	if stepsRaw, ok := prepared.Task["task_steps"].([]any); ok && len(stepsRaw) > 0 {
		out.MaxSteps = 20 + (len(stepsRaw) * 15)
	}
	if effort, ok := prepared.Task["estimated_effort"].(string); ok {
		if strings.Contains(effort, "d") {
			out.MaxSteps += 20
		}
	}
	if out.MaxSteps > 150 {
		out.MaxSteps = 150
	}

	return out, nil
}

func dispatchLeadPlanFromWhatsNext(ctx context.Context, logger logging.Logger, sp storage.ObjectStorageProvider, root, agentID, _ string) (leadPlanDispatch, error) {
	if sp == nil {
		return leadPlanDispatch{DispatchOrchestratePlanResult: agentfeed.DispatchOrchestratePlanResult{Skipped: "no_storage"}}, nil
	}
	// Unfiltered Gantt lead — OPERATOR columns must not hide plan primaries,
	// and core must not enumerate PER-ORCH-* IDs.
	out, err := whatsnext.Execute(ctx, sp, root, &whatsnext.QueryRequest{
		SkipMeasure:       true,
		SkipPersonaFilter: true,
		AgentID:           agentID,
	})
	if err != nil {
		return leadPlanDispatch{}, err
	}
	if out == nil || out.PriorityPlan == nil || strings.TrimSpace(out.PriorityPlan.ID) == "" {
		return leadPlanDispatch{DispatchOrchestratePlanResult: agentfeed.DispatchOrchestratePlanResult{Skipped: "no_lead_plan"}}, nil
	}
	if out.PriorityPlan.Status != objects.ObjectStatusActive && out.PriorityPlan.Status != objects.ObjectStatusInProgress {
		return leadPlanDispatch{DispatchOrchestratePlanResult: agentfeed.DispatchOrchestratePlanResult{Skipped: "plan_status_not_executable"}}, nil
	}
	planned := 0
	if out.BacklogCountsByStatus != nil {
		planned = out.BacklogCountsByStatus["planned"]
	}
	if planned <= 0 {
		return leadPlanDispatch{DispatchOrchestratePlanResult: agentfeed.DispatchOrchestratePlanResult{Skipped: "no_planned"}}, nil
	}
	sec := pkgctx.GetSecurityContext(ctx)
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	if tasks, listErr := listOrchestrationTasksForPlan(ctx, sp, sec, out.PriorityPlan.ID); listErr == nil {
		if reason := orchDispatchSkipReason(tasks); reason != "" {
			d := leadPlanDispatch{DispatchOrchestratePlanResult: agentfeed.DispatchOrchestratePlanResult{Skipped: reason}}
			if reason == "no_live_atk" {
				fill := out.FillItem
				if fill == nil {
					fill = whatsnext.CompileFillItem(out.KernelAmbience)
				}
				if fill != nil {
					d.FillKind = fill.Kind
					d.FillHint = fill.CommandHint
					submitted, subErr := submitKernelFill(ctx, root, fill)
					if subErr != nil {
						if logger != nil {
							logging.Fluent(logger).Warn("seat-worker kernel fill submit failed").
								WithError(errfmt.Newf("fill").Wrap(subErr)).
								Log()
						}
					} else {
						d.FillSubmitted = submitted
					}
				}
			}
			return d, nil
		}
	}
	planObj, readErr := sp.Read(ctx, sec, out.PriorityPlan.ID)
	if readErr != nil {
		return leadPlanDispatch{}, readErr
	}
	if kind, _ := planObj[objects.FieldKeyKind].(string); kind == objects.KindPriorityPlan {
		st, _ := planObj[objects.FieldKeyStatus].(string)
		if st != objects.ObjectStatusActive && st != objects.ObjectStatusInProgress {
			return leadPlanDispatch{DispatchOrchestratePlanResult: agentfeed.DispatchOrchestratePlanResult{Skipped: "plan_status_not_executable"}}, nil
		}
	}
	persona := agentfeed.FirstPersonaRef(planObj)
	if persona == "" {
		return leadPlanDispatch{DispatchOrchestratePlanResult: agentfeed.DispatchOrchestratePlanResult{Skipped: "no_plan_persona"}}, nil
	}
	seat := agentfeed.WorkerSeatForPersona(root, persona)
	if seat == "" {
		return leadPlanDispatch{DispatchOrchestratePlanResult: agentfeed.DispatchOrchestratePlanResult{Skipped: "no_worker_seat"}}, nil
	}
	res, dispErr := agentfeed.DispatchOrchestratePlan(agentfeed.DispatchOrchestratePlanInput{
		ProjectRoot: root,
		FromAgentID: agentID,
		ToAgentID:   seat,
		PlanID:      out.PriorityPlan.ID,
		Extra:       "self-serve whats-next; do not remint",
	})
	if dispErr != nil {
		return leadPlanDispatch{DispatchOrchestratePlanResult: res}, dispErr
	}
	if logger != nil && res.EventID != "" {
		logging.Fluent(logger).Info("seat-worker dispatched ORCHESTRATE_PLAN").
			String("plan_id", out.PriorityPlan.ID).
			String("to_agent_id", seat).
			String("event_id", res.EventID).
			Log()
	}
	return leadPlanDispatch{DispatchOrchestratePlanResult: res}, nil
}

func parseOrchestratePlanDirective(body string) (string, bool, error) {
	firstLine := strings.TrimSpace(strings.SplitN(body, "\n", 2)[0])
	if !strings.HasPrefix(firstLine, orchestratePlanDirective) {
		return "", false, nil
	}
	planID := strings.TrimSpace(strings.TrimPrefix(firstLine, orchestratePlanDirective))
	if !validPlanDirectiveID(planID) {
		return "", true, errfmt.Errorf("invalid ORCHESTRATE_PLAN id %q", planID)
	}
	return planID, true, nil
}

func validPlanDirectiveID(planID string) bool {
	if !strings.HasPrefix(planID, "PRI-") {
		return false
	}
	for _, char := range planID {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '-' {
			return false
		}
	}
	return true
}

func triggerPlanOrchestration(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, root, planID string) (string, error) {
	if sp == nil && root != "" {
		if sf, fErr := storage.NewStorageFactory(ctx, root); fErr == nil && sf != nil {
			sp = sf.GetStorage()
		}
	}
	if sp != nil {
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		if planObj, err := sp.Read(ctx, secCtx, planID); err == nil && planObj != nil {
			kind, _ := planObj[objects.FieldKeyKind].(string)
			if kind == objects.KindPriorityPlan {
				st, _ := planObj[objects.FieldKeyStatus].(string)
				if st != objects.ObjectStatusActive && st != objects.ObjectStatusInProgress {
					return "", errfmt.Errorf("orchestrator can only operate on active or in_progress priority plans (got '%s')", st)
				}
			}
		}
	}
	if skip, msg := recentPlanOrchSubmit(root, planID); skip {
		return msg, nil
	}
	// Explicit ORCHESTRATE_PLAN must run even when the plan surface is
	// terminal-only. Per-item orch skips implemented ATKs and mints
	// replacements for error debris instead of reusing dead IDs.
	// Idle auto-dispatch is gated separately in dispatchLeadPlanFromWhatsNext.
	zqkPath := filepath.Join(root, "bin", "zqk")
	// Do not pass --persona-id: that filter drops BLIs assigned to coder/reviewer
	// seats and the job exits routed with dispatched_items=0.
	orchCmd := zqkPath + " agent orchestrate " + planID
	logDir := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, "agent-ops")
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
		return "", errfmt.Errorf("orchestrate callback dir: %w", err)
	}
	cb := filepath.Join(logDir, "orch-"+safePlanFileName(planID)+".callback.json")
	// tee-only is not a seat wake.
	hourglass := filepath.Join(root, "scripts", "agent-ops", "scheduler-job-callback.py") +
		" --log " + cb
	cmd := execwrap.CommandContext(ctx, zqkPath,
		"scheduler", "submit", orchCmd,
		"--title", "ORCHESTRATE "+planID,
		"--max-runtime", "7200",
		"--workdir", root,
		"--callback-completion", hourglass,
		"--callback-failure", hourglass,
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), errfmt.Errorf("submit agent orchestrate for %s: %w", planID, err)
	}
	if markErr := writePlanOrchSubmitMark(root, planID, strings.TrimSpace(string(output))); markErr != nil {
		return string(output), markErr
	}
	return string(output), nil
}

func planOrchSubmitMarkPath(root, planID string) string {
	return filepath.Join(root, paths.ProjectDataDir, paths.StateDir, "mesh", "seat_workers",
		"plan-orch-"+safePlanFileName(planID)+".json")
}

func safePlanFileName(planID string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '_'
	}, planID)
}

func recentPlanOrchSubmit(root, planID string) (bool, string) {
	raw, err := fileutil.ReadFile(planOrchSubmitMarkPath(root, planID))
	if err != nil {
		return false, ""
	}
	var mark struct {
		DispatchedAt string `json:"dispatched_at"`
	}
	if json.Unmarshal(raw, &mark) != nil {
		return false, ""
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(mark.DispatchedAt))
	if err != nil {
		return false, ""
	}
	if time.Since(ts) >= planOrchSubmitCooldown {
		return false, ""
	}
	return true, "already_submitted " + planID
}

func writePlanOrchSubmitMark(root, planID, detail string) error {
	path := planOrchSubmitMarkPath(root, planID)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return errfmt.Errorf("plan orch mark dir: %w", err)
	}
	payload, err := json.Marshal(map[string]string{
		"schema":        "zqk_plan_orch_submit_v1",
		"plan_id":       planID,
		"dispatched_at": time.Now().UTC().Format(time.RFC3339),
		"detail":        detail,
	})
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, append(payload, '\n'), paths.FilePerm600)
}

// withLocalLLMEnv ensures local LLM settings from the process environment (or the
// loopback API-key default) are present in a child env block.
func withLocalLLMEnv(environ []string) []string {
	out := append([]string(nil), environ...)
	baseURL := zqkenv.LLMBaseURL().Get()
	if baseURL == "" {
		baseURL = zqkenv.Get("LLM_BASE_URL").OrDefault("")
	}
	out = withEnvValue(out, zqkenv.LLMProvider().Name(), zqkenv.LLMProvider().Get())
	out = withEnvValue(out, zqkenv.LLMBaseURL().Name(), baseURL)
	chatModel := zqkenv.LLMChatModel().Get()
	if chatModel == "" {
		chatModel = zqkenv.Get("LLM_CHAT_MODEL").OrDefault("")
	}
	if chatModel == "" && strings.Contains(baseURL, "11434") {
		chatModel = "qwen3.8:latest"
	}
	out = withEnvValue(out, zqkenv.LLMChatModel().Name(), chatModel)
	timeout := zqkenv.Get("LLM_TIMEOUT").OrDefault("")
	if timeout == "" && strings.Contains(baseURL, "11434") {
		timeout = "900"
	}
	out = withEnvValue(out, "LLM_TIMEOUT", timeout)
	out = withEnvValue(out, zqkenv.LLMTimeout().Name(), timeout)
	if apiKey := localLLMAPIKeyValue(); apiKey != "" {
		out = withEnvValue(out, zqkenv.LLMAPIKey().Name(), apiKey)
	} else {
		out = withEnvValue(out, zqkenv.LLMAPIKey().Name(), zqkenv.LLMAPIKey().Get())
	}
	return out
}

func withEnvValue(environ []string, key, value string) []string {
	if strings.TrimSpace(value) == "" {
		return environ
	}
	out := append([]string(nil), environ...)
	prefix := key + "="
	for i, entry := range out {
		if strings.HasPrefix(entry, prefix) {
			out[i] = prefix + value
			return out
		}
	}
	return append(out, prefix+value)
}

func localLLMAPIKeyValue() string {
	if existing := strings.TrimSpace(zqkenv.LLMAPIKey().Get()); existing != "" {
		return existing
	}
	baseURL := strings.TrimSpace(config.LLMBaseURL().OrDefault(""))
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	host := parsedURL.Hostname()
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return localLLMAPIKey
	}
	return ""
}

func writeSeatWorkerResult(root, engineID, result string) {
	logDir := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, "agent-seat-worker")
	_ = fileutil.MkdirAll(logDir, paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(logDir, engineID+".log"), []byte(result), paths.FilePerm644)
}
