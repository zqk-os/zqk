package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// persistSessionOutcome records --persist-session results for JSON/YAML and agent-prompt copy.
type persistSessionOutcome struct {
	Requested             bool `json:"requested,omitempty"`
	Applied               bool `json:"applied,omitempty"`
	SkippedNoNewWatermark bool `json:"skipped_no_new_watermark,omitempty"`
	// AuditAppended is set when only an activity_log line was written (duplicate health watermark).
	AuditAppended bool `json:"audit_appended,omitempty"`
}

const agentPromptAutonomyBlock = `Autonomy (routine convergence work):
- Advance the session toward **desired_end_state** under **hypothesis**; treat **predictions** as measurable checkpoints.
- **Do not** treat **ready_for_session_completion** or **measurement_implied_phase: c6_exit** as proof that **desired_end_state** is fully satisfied — those signals are **test-bundle health only** unless you have also verified gates, matrix, drift, and other contract items.
- Infer scope from the repo, CVS fields, and suggested reruns; do not ask to confirm facts already in this prompt or prior thread context.
- **Anti-Idle Mandate**: Do not sit idle awaiting human input that is redundant because the kernel already absorbed the knowledge in an earlier interaction. Stop only for hard blockers (missing secrets, destructive ops, or a genuine product/architecture fork). Otherwise implement, measure, and iterate.
- **The Knowledge Flywheel**: This is a cycle that feeds itself perpetually. Your role is to provide the simple 'pong' of agent exchanges that provide the pumping action to tip the scale towards or away from some initial value or state. Where momentum wanes, store enough potential energy to set the flywheel back in motion so things remain in near perfect equilibrium.
- Verification: follow project rules — targeted **zqk scheduler scan-tests** / bundles and log files for package gates; avoid long foreground **go test** on heavy trees.
`

func runTestFailuresConvergence(cliCtx *cli.Context, cmd *cobra.Command) error {
	// Single ResolveProjectRoot(".") per invocation; all downstream I/O uses this value (health.jsonl,
	// rollup JSONL, agent_prompt_runs.jsonl, agent-prompt pipeline). Do not use cliCtx.ProjectRoot —
	// merged config can point at the real repo and bypass ZQK_TEST_ROOT in subprocess tests.
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		limit = 500
	}

	sessionID, _ := cmd.Flags().GetString("session-id") //nolint:errcheck // optional flag
	currentPhase, _ := cmd.Flags().GetString("current-phase")
	flowVariant, _ := cmd.Flags().GetString("flow-variant")
	persistSession, _ := cmd.Flags().GetBool("persist-session") //nolint:errcheck // optional
	skipSessionCtx, _ := cmd.Flags().GetBool("skip-session-context")
	if persistSession && skipSessionCtx {
		return errfmt.Errorf("--persist-session requires reading the convergence_session object; omit --skip-session-context")
	}
	stampTombstone, _ := cmd.Flags().GetBool("stamp-tombstone")
	finalizeDebrief, _ := cmd.Flags().GetBool("finalize-debrief")
	debriefNotesFlag, _ := cmd.Flags().GetString("debrief-notes")
	debriefNotesFile, _ := cmd.Flags().GetString("debrief-notes-file")
	debriefNotes, err := resolveDebriefNotesFromFlags(debriefNotesFlag, debriefNotesFile)
	if err != nil {
		return err
	}
	if debriefNotes != emptyValue && strings.TrimSpace(sessionID) == emptyValue {
		return errfmt.Errorf("--debrief-notes / --debrief-notes-file require --session-id")
	}

	if persistSession && strings.TrimSpace(sessionID) == emptyValue {
		return errfmt.Errorf("--persist-session requires --session-id")
	}

	// Use GetFormat(cmd) so --format on this command (or chain) wins; cliCtx.Format alone can stay "table".
	format := strings.ToLower(string(cli.GetFormat(cmd)))
	if format == emptyValue {
		format = schedulerFormatTable
	}
	skipRollupGates, _ := cmd.Flags().GetBool("skip-rollup-gates") //nolint:errcheck // optional
	pasteIDE, _ := cmd.Flags().GetBool("paste-ide")
	copyClip, _ := cmd.Flags().GetBool("copy")
	attentionModeRaw, _ := cmd.Flags().GetString("attention-mode")

	var attentionMode string
	// Short-circuit before health.jsonl I/O when agent-prompt flags contradict (same rules as pipeline INGEST).
	if format == schedulerFormatAgentPrompt {
		if err := agentPromptValidateIngestConstraints(&agentPromptPipelineInput{
			sessionID: sessionID, skipSessionCtx: skipSessionCtx,
			pasteIDE: pasteIDE, copyClip: copyClip,
		}); err != nil {
			return err
		}
		var aerr error
		attentionMode, aerr = normalizeAgentPromptAttentionMode(attentionModeRaw)
		if aerr != nil {
			return aerr
		}
	}

	var effCurrentPhase, effFlowVariant string
	var sessionRoutingMeta map[string]any
	var beforeStateSnapshot map[string]any
	var sessionPredictions map[string]any
	var sessionThresholds map[string]any
	if strings.TrimSpace(sessionID) != emptyValue {
		var rerr error
		effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, sessionPredictions, sessionThresholds, rerr = resolveConvergenceRoutingForCLI(cmd, sessionID, currentPhase, flowVariant, skipSessionCtx)
		if rerr != nil {
			return rerr
		}
		surfaceID := schedpkg.EvaluationSurfaceIDFromRoutingMeta(sessionRoutingMeta)
		if surfaceID == schedpkg.EvaluationSurfaceCEFDiamondScorecard {
			// Pluggable surface: do not read health.jsonl or twin SCH-cvs ticks.
			return runCEFDiamondConvergenceMeasure(cliCtx, cmd, projectRoot, sessionID, persistSession, effCurrentPhase, effFlowVariant, sessionRoutingMeta, sessionThresholds)
		}
	} else {
		effCurrentPhase, effFlowVariant = currentPhase, flowVariant
	}

	var lines []map[string]any
	if format == schedulerFormatAgentPrompt {
		lines, err = readTestBundleHealthTailForAgentPrompt(projectRoot, limit)
		if err != nil {
			return errfmt.Newf("read health log").Wrap(err)
		}
	} else {
		lines, err = readTestBundleHealthTailForCLI(cmd, projectRoot, limit)
		if err != nil {
			if errors.Is(err, errTestBundleHealthFileHandled) {
				return nil
			}
			return err
		}
	}

	snap := schedpkg.BuildTestBundleConvergenceSnapshot(projectRoot, lines)

	var persistOut *persistSessionOutcome
	if persistSession && strings.TrimSpace(sessionID) != emptyValue {
		var perr error
		persistOut, perr = runConvergenceSessionPersist(cliCtx, cmd, sessionID, snap, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds)
		if perr != nil {
			return perr
		}
	}

	if format == schedulerFormatAgentPrompt {
		rollupCore, rerr := buildRollupStatusCore(cmd, projectRoot, skipSessionCtx, sessionID, snap, skipRollupGates)
		if rerr != nil {
			return rerr
		}
		schedpkg.AppendRollupStatusCoreEvent(projectRoot, sessionID, rollupCore)
		md, footer, err := runConvergenceAgentPromptPipeline(cmd, projectRoot, snap, sessionID, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, skipSessionCtx, pasteIDE, copyClip, attentionMode, persistOut, sessionThresholds, rollupCore)
		if err != nil {
			return err
		}
		deliverMode := agentPromptDeliverNone
		if pasteIDE {
			deliverMode = agentPromptDeliverPasteIDE
		} else if copyClip {
			deliverMode = agentPromptDeliverClipboard
		}
		httpURL := effectiveAgentPromptHTTPDeliveryURL(cmd)
		return deliverAgentPromptMarkdownOutput(cmd, projectRoot, md, footer, sessionID, deliverMode, attentionMode, httpURL)
	}
	rollupCore, rerr := buildRollupStatusCore(cmd, projectRoot, skipSessionCtx, sessionID, snap, skipRollupGates)
	if rerr != nil {
		return rerr
	}
	schedpkg.AppendRollupStatusCoreEvent(projectRoot, sessionID, rollupCore)

	switch format {
	case schedulerFormatJSON:
		m, err := buildConvergenceOutputMap(snap, sessionID, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds, persistOut, rollupCore)
		if err != nil {
			return errfmt.Newf("build convergence output").Wrap(err)
		}
		return cli.FormatOutputAs(cmd, cli.FormatJSON, m)
	case schedulerFormatYAML:
		m, err := buildConvergenceOutputMap(snap, sessionID, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds, persistOut, rollupCore)
		if err != nil {
			return errfmt.Newf("build convergence output").Wrap(err)
		}
		return cli.FormatOutputAs(cmd, cli.FormatYAML, m)
	default:
		var b strings.Builder
		b.WriteString("Test-bundle convergence measure (from health.jsonl)\n\n")
		fmt.Fprintf(&b, "Lines in window: %d\n", snap.LinesInWindow)
		fmt.Fprintf(&b, "Health watermark: %s\n", snap.HealthWatermarkRFC3339)
		fmt.Fprintf(&b, "Delta assessment: %s\n", snap.DeltaAssessment)
		if snap.PrimaryMeasurementOutcome != emptyValue {
			fmt.Fprintf(&b, "Primary measurement outcome: %s (%s)\n", snap.PrimaryMeasurementOutcome, snap.PrimaryMeasurementOutcomeDetail)
			if snap.MeasurementOutcomeSchemaVersion != emptyValue {
				fmt.Fprintf(&b, "measurement_outcome_schema_version: %s\n", snap.MeasurementOutcomeSchemaVersion)
			}
		}
		fmt.Fprintf(&b, "Had failure in window: %v\n", snap.HadFailureInWindow)
		fmt.Fprintf(&b, "Failing fingerprints (latest): %d\n", len(snap.FailingFingerprintsNow))
		for _, fp := range snap.FailingFingerprintsNow {
			fmt.Fprintf(&b, "  - %s\n", fp)
		}
		if snap.Heartbeat != nil {
			fmt.Fprintf(&b, "Heartbeat age: %.0fs (stale=%v after %.0fs)\n",
				snap.Heartbeat.MeasurementAgeSeconds, snap.Heartbeat.Stale, snap.Heartbeat.StaleAfterSeconds)
		}
		fmt.Fprintf(&b, "Trigger queue pending: %d\n", snap.TriggerQueuePending)
		fmt.Fprintf(&b, "Ready for session completion (bundle-health gates only): %v\n", snap.ReadyForSessionCompletion)
		b.WriteString("(Does not evaluate desired_end_state, vetting matrix, or repo script gates — see agent-prompt Scope section or CONVERGENCE_PREDICATES_AND_GATES.md.)\n")
		if len(snap.SessionCompletionBlockedReasons) > 0 {
			b.WriteString("Session completion blocked:\n")
			for _, r := range snap.SessionCompletionBlockedReasons {
				fmt.Fprintf(&b, "  - %s\n", r)
			}
		}
		if snap.SessionCompletionNote != emptyValue {
			fmt.Fprintf(&b, "Note: %s\n", snap.SessionCompletionNote)
		}
		fmt.Fprintf(&b, "\nNext action: %s\n", snap.NextActionHint)
		b.WriteString("\n---\nrollup_status_core (pkg/convergerollup)\n")
		if rs, ok := rollupCore["rollup_status"].(string); ok {
			fmt.Fprintf(&b, "rollup_status: %s\n", rs)
		}
		if rfp, ok := rollupCore["ready_for_parent_completion"].(bool); ok {
			fmt.Fprintf(&b, "ready_for_parent_completion: %v\n", rfp)
		}
		if fk, ok := rollupCore["field_key_literals_gate"].(map[string]any); ok {
			fmt.Fprintf(&b, "field_key_literals_gate: exit_code=%v script=%v\n", fk["exit_code"], fk["script"])
		}
		if ze, ok := rollupCore["zqk_env_literals_gate"].(map[string]any); ok {
			fmt.Fprintf(&b, "zqk_env_literals_gate: exit_code=%v script=%v\n", ze["exit_code"], ze["script"])
		}
		if skip, ok := rollupCore["rollup_gates_skipped"].(bool); ok && skip {
			b.WriteString("rollup_gates_skipped: true\n")
		}
		if note, ok := rollupCore["evaluation_note"].(string); ok && note != emptyValue {
			fmt.Fprintf(&b, "evaluation_note: %s\n", note)
		}
		if persistOut != nil && persistOut.Requested {
			switch {
			case persistOut.Applied:
				b.WriteString("\n--persist-session: applied object_update_body to the CVS via storage.\n")
			case persistOut.AuditAppended:
				b.WriteString("\n--persist-session: no new health watermark — appended activity_log audit only (snapshot fields unchanged).\n")
			case persistOut.SkippedNoNewWatermark:
				b.WriteString("\n--persist-session: skipped (no new health watermark vs CVS last_measurement_at).\n")
			}
		}
		if sessionID != emptyValue {
			b.WriteString("\n---\n")
			fmt.Fprintf(&b, "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("Session %s — paste into zqk object update (see suggested_convergence_session_fields with --format json --session-id):\n", sessionID)))
			summary, err := summarizeSuggestedSessionFields(snap, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds)
			if err != nil {
				return errfmt.Newf("suggested fields").Wrap(err)
			}
			b.WriteString(summary)
		}
		return cli.WriteOutput(cmd, []byte(b.String()))
	}
}

// runConvergenceSessionPersist applies suggested object_update_body to storage (same payload as
// convergence_session_tick). When the health watermark matches CVS last_measurement_at, applies an
// activity_log append-only audit (no duplicate snapshot fields).
func runConvergenceSessionPersist(
	cliCtx *cli.Context,
	cmd *cobra.Command,
	sessionID string,
	snap *schedpkg.TestBundleConvergenceSnapshot,
	effCurrentPhase, effFlowVariant string,
	sessionRoutingMeta map[string]any,
	beforeStateSnapshot map[string]any,
	stampTombstone bool,
	sessionPredictions map[string]any,
	finalizeDebrief bool,
	debriefNotes string,
	sessionThresholds map[string]any,
) (*persistSessionOutcome, error) {
	_ = cliCtx
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, err
	}
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	obj, err := proc.Storage().Read(ctx, sec, sessionID)
	if err != nil {
		return nil, errfmt.Newf("persist-session: read convergence_session").Wrap(err)
	}
	out := &persistSessionOutcome{Requested: true}
	if schedpkg.ShouldSkipConvergencePersistForDuplicateWatermark(obj, snap) {
		out.SkippedNoNewWatermark = true
		audit := schedpkg.ConvergenceDuplicateWatermarkAuditUpdate(obj, snap, time.Now())
		if err := proc.Storage().Update(ctx, sec, sessionID, audit); err != nil {
			return nil, errfmt.Newf("persist-session: duplicate-watermark audit append").Wrap(err)
		}
		out.AuditAppended = true
		logging.Fluent(logging.GetLoggerFromProfile(cliCtx.Profile)).Info(
			"convergence: --persist-session appended activity_log (no new health watermark vs CVS last_measurement_at)").
			Log()
		return out, nil
	}
	sug, err := schedpkg.BuildSuggestedConvergenceSessionFields(snap, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds)
	if err != nil {
		return nil, err
	}
	body, ok := sug["object_update_body"].(map[string]any)
	if !ok || len(body) == 0 {
		return nil, errfmt.Errorf("persist-session: empty object_update_body")
	}
	schedpkg.MergeConvergencePersistObjectUpdateBody(obj, body, snap)
	if err := proc.Storage().Update(ctx, sec, sessionID, body); err != nil {
		return nil, errfmt.Newf("persist-session").Wrap(err)
	}
	out.Applied = true
	return out, nil
}

// buildConvergenceOutputMap builds the JSON/YAML document for scheduler convergence measure.
// When sessionID is non-empty, adds convergence_session_id and suggested_convergence_session_fields.
func buildConvergenceOutputMap(snap *schedpkg.TestBundleConvergenceSnapshot, sessionID, effCurrentPhase, effFlowVariant string, sessionRoutingMeta map[string]any, beforeStateSnapshot map[string]any, stampTombstone bool, sessionPredictions map[string]any, finalizeDebrief bool, debriefNotes string, sessionThresholds map[string]any, persist *persistSessionOutcome, rollupCore map[string]any) (map[string]any, error) {
	if sessionID == emptyValue {
		raw, err := json.Marshal(snap)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		if rollupCore != nil {
			m["rollup_status_core"] = rollupCore
		}
		return m, nil
	}
	sug, err := schedpkg.BuildSuggestedConvergenceSessionFields(snap, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["convergence_session_id"] = sessionID
	m["suggested_convergence_session_fields"] = sug
	if persist != nil && persist.Requested {
		m["persist_session"] = persist
	}
	if rollupCore != nil {
		m["rollup_status_core"] = rollupCore
	}
	return m, nil
}

// marshalConvergenceOutputJSON encodes the snapshot for tests and callers that need raw bytes.
func marshalConvergenceOutputJSON(snap *schedpkg.TestBundleConvergenceSnapshot, sessionID, effCurrentPhase, effFlowVariant string, sessionRoutingMeta map[string]any, beforeStateSnapshot map[string]any, stampTombstone bool, sessionPredictions map[string]any, finalizeDebrief bool, debriefNotes string, sessionThresholds map[string]any, persist *persistSessionOutcome, rollupCore map[string]any) ([]byte, error) {
	m, err := buildConvergenceOutputMap(snap, sessionID, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds, persist, rollupCore)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

func summarizeSuggestedSessionFields(snap *schedpkg.TestBundleConvergenceSnapshot, effCurrentPhase, effFlowVariant string, sessionRoutingMeta map[string]any, beforeStateSnapshot map[string]any, stampTombstone bool, sessionPredictions map[string]any, finalizeDebrief bool, debriefNotes string, sessionThresholds map[string]any) (string, error) {
	sug, err := schedpkg.BuildSuggestedConvergenceSessionFields(snap, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "delta_assessment=%v\n", sug[objects.FieldKeyDeltaAssessment])
	fmt.Fprintf(&b, "last_measurement_at=%v\n", sug[objects.FieldKeyLastMeasurementAt])
	fmt.Fprintf(&b, "next_action (first line): %s\n", firstLine(sug[objects.FieldKeyNextAction]))
	if cp, ok := sug[objects.FieldKeyCurrentPhase].(string); ok && cp != emptyValue {
		fmt.Fprintf(&b, "current_phase (suggested): %s\n", cp)
	}
	if td, ok := sug["tombstone_disparity"].(map[string]any); ok {
		fmt.Fprintf(&b, "tombstone_disparity.active_tombstone=%v summary=%v\n", td["active_tombstone"], td["disparity_summary"])
	}
	if stampTombstone {
		b.WriteString("stamp_tombstone: object_update_body will include before_state_snapshot\n")
	}
	if finalizeDebrief {
		b.WriteString("finalize_debrief: object_update_body will include predictions with retrospective\n")
	}
	if pd, ok := sug["prediction_debrief"].(map[string]any); ok {
		fmt.Fprintf(&b, "prediction_debrief.signal_alignment=%v\n", pd["signal_alignment"])
		fmt.Fprintf(&b, "prediction_debrief.debrief_summary (first line): %s\n", firstLine(pd["debrief_summary"]))
	}
	if ou, ok := sug["object_update_body"].(map[string]any); ok {
		if _, has := ou[objects.FieldKeyDebriefNotes]; has {
			b.WriteString("object_update_body will include debrief_notes (handoff for next session)\n")
		}
	}
	if pr, ok := sug["phase_router"].(map[string]any); ok {
		fmt.Fprintf(&b, "phase_router.measurement_implied_phase=%v\n", pr["measurement_implied_phase"])
		if cp := pr[objects.FieldKeyCurrentPhase]; cp != nil && cp != emptyValue {
			fmt.Fprintf(&b, "phase_router.phase_alignment=%v\n", pr["phase_alignment"])
		}
	}
	if sr, ok := sug["session_routing_context"].(map[string]any); ok {
		fmt.Fprintf(&b, "session_routing_context.current_phase_source=%v flow_variant_source=%v\n",
			sr["current_phase_source"], sr["flow_variant_source"])
	}
	return b.String(), nil
}

func firstLine(v any) string {
	s, _ := v.(string)
	if s == emptyValue {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func runTestFailuresConvergenceFromCmd(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return runTestFailuresConvergence(ctx, cmd)
}
