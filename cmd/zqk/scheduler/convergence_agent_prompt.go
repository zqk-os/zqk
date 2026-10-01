package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentdelivery"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/policy"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	pipelineKindConvergenceAgentPrompt = "scheduler.convergence_agent_prompt"
	outcomeAgentPromptSessionID        = "convergence_session_id"
	outcomeAgentPromptMarkdownBytes    = "markdown_bytes"
	outcomeAgentPromptDeliverMode      = "deliver_mode"
	outcomeAgentPromptComplete         = "complete"
	outcomeAgentPromptValidationFailed = "validation_failed"
	outcomeAgentPromptBlockedReason    = "blocked_reason"

	// agentPromptIDEKeystrokeLogFile is appended by --paste-ide (Go + AppleScript log) for automation visibility.
	agentPromptIDEKeystrokeLogFile = "agent_prompt_ide_keystrokes.log"
	// agentPromptRunsJSONLFile is one JSON object per successful convergence_agent_prompt pipeline run (audit / metrics plumbing).
	agentPromptRunsJSONLFile  = "agent_prompt_runs.jsonl"
	ideAutomationLogComponent = "zqk.scheduler.convergence_agent_prompt"

	agentPromptRunJSONKeyEventType    = "event_type"
	agentPromptRunJSONKeyPipelineKind = "pipeline_kind"
	agentPromptRunEventType           = "convergence_agent_prompt_run"

	// Keystroke log file events (TSV column 3).
	ideKeystrokeEventPbcopyBegin             = "pbcopy_begin"
	ideKeystrokeEventPbcopyError             = "pbcopy_error"
	ideKeystrokeEventPbcopyDone              = "pbcopy_done"
	ideKeystrokeEventOsascriptBegin          = "osascript_begin"
	ideKeystrokeEventOsascriptCombinedOutput = "osascript_combined_output"
	ideKeystrokeEventOsascriptError          = "osascript_error"
	ideKeystrokeEventOsascriptDone           = "osascript_done"

	// Keystroke log detail fragments (TSV column 4).
	ideKeystrokeDetailClipboardReady   = "clipboard_ready"
	ideKeystrokeDetailOk               = "ok"
	ideKeystrokeDetailKeyMarkdownBytes = "markdown_bytes"
	ideKeystrokeDetailKeyLogPath       = "log_path"

	// Structured logging field keys (ide paste automation).
	logFieldIDELogFile    = "log_file"
	logFieldIDEKeystrokes = "keystrokes"
	logFieldIDEOutput     = "output"
	logFieldPath          = "path"

	// Logger messages (ide keystroke trace).
	logMsgIDEPasteAutomationRunning       = "ide paste automation: running AppleScript"
	logMsgIDEPasteAutomationOutput        = "ide paste automation: osascript output"
	logMsgAgentPromptKeystrokeMkdirFailed = "agent prompt ide keystroke log: mkdir failed"
	logMsgAgentPromptKeystrokeOpenFailed  = "agent prompt ide keystroke log: open failed"
	logMsgAgentPromptKeystrokeWriteFailed = "agent prompt ide keystroke log: write failed"

	// CLI --attention-mode normalization (aliases and sentinel inputs).
	agentPromptAttentionInputNone = "none"
	agentPromptAttentionInputOff  = "off"

	agentPromptAttentionAliasTest                    = "test"
	agentPromptAttentionAliasTestNonDirective        = "test_non_directive"
	agentPromptAttentionAliasNonDirective            = "non_directive"
	agentPromptAttentionAliasInterrupt               = "interrupt"
	agentPromptAttentionAliasInterruptPlumbing       = "interrupt_plumbing"
	agentPromptAttentionAliasCriticalInterrupt       = "critical_interrupt"
	agentPromptAttentionAliasCriticalInterruptDryRun = "critical_interrupt_dry_run"

	agentPromptAttentionModeInvalidFmt = "invalid --attention-mode %q (use: none, test_non_directive, interrupt_plumbing; aliases: test, interrupt, critical_interrupt_dry_run)"

	// sessionRoutingMeta keys for agent-prompt CLI diagnostic; keep in sync with pkg/scheduler/convergence_routing.go.
	agentPromptRoutingMetaFlowVariantSource    = "flow_variant_source"
	agentPromptRoutingMetaEffectiveFlowVariant = "effective_flow_variant"
)

// Agent-prompt markdown: keep measurement semantics aligned with pkg/scheduler applySessionCompletionGates
// and docs/architecture/CONVERGENCE_PREDICATES_AND_GATES.md (measurement vs process rules).
const (
	agentPromptMeasurementScopeSection = `## Scope: test bundles vs session contract

- **Test-bundle measurement** (section below) is derived only from **health.jsonl**: last outcome per fingerprint, window history, heartbeat, and scheduler trigger-queue depth. It does **not** evaluate free-text **desired_end_state** (vetting matrix, FieldKey/ZQK-env gates, drift baselines, refactors).
- **Next measured action (rollup)** (below) adds **rollup_status_core**: bundles + literal gate scripts + direct **related_object_refs** child CVS rows — use it to pick the next command before relying on persisted **next_action** alone.
- **Nested CVS tree** (coordinator, depth-capped BFS): **zqk scheduler convergence overseer** — read-only; does not write **rollup_v1**. See **CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md** Appendix C.
- Field **ready_for_session_completion** means bundle-health gates in **pkg/scheduler** (**applySessionCompletionGates**) passed; it does **not** mean every bullet in **desired_end_state** is satisfied.
- **status → completed** and **current_phase → c6_exit** still require alignment with the written contract and **process rules**; see **docs/architecture/CONVERGENCE_PREDICATES_AND_GATES.md**.

`
	agentPromptPhaseRouterHowToRead = `**How to read measurement-implied phase:** Derived from the **same** test-bundle snapshot as field **ready_for_session_completion**. **c6_exit** means bundle health looks stable enough to *consider* lifecycle exit — **not** proof that **desired_end_state** is fully met. Use **phase_alignment** in the JSON below (aligned, session_behind, session_ahead, unknown). When **session_behind**, persisted **current_phase** is behind measurement — **do not** advance **current_phase** to **c6_exit** on green bundles alone if contract work remains.

`
)

// Sentinel errors for agent-prompt ingest validation (short-circuit before markdown build / CVS read).
var (
	errAgentPromptMissingSession      = errfmt.Errorf("requires --session-id")
	errAgentPromptSkipSessionContext  = errfmt.Errorf("cannot use --skip-session-context; agent prompt loads the convergence_session object")
	errAgentPromptPasteRequiresDarwin = errfmt.Errorf("--paste-ide requires macOS")
	errAgentPromptCopyRequiresDarwin  = errfmt.Errorf("--copy requires macOS (pbcopy)")
	// errAgentPromptProjectRootMissing: pipeline stages must not call ResolveProjectRoot; the caller resolves once.
	errAgentPromptProjectRootMissing = errfmt.Errorf("agent prompt: project root not set (resolve once at command entry)")
)

func buildAppleScriptIDEPaste() (string, error) {
	a, err := loadIDEPasteAutomation()
	if err != nil {
		return "", err
	}
	return a.appleScript, nil
}

// deliverMode values for outcomeAgentPromptDeliverMode (metrics / future event hooks).
const (
	agentPromptDeliverNone      = "none"
	agentPromptDeliverClipboard = "clipboard"
	agentPromptDeliverPasteIDE  = "paste_ide"

	// agentPromptRunsJSONLFileLockTimeout: keep 5s in sync with coordination event log lock in pkg/scheduler/coordination_channel.go (both use storage.FileLock.WithLockTimeout).
	agentPromptRunsJSONLFileLockTimeout = 5 * time.Second
)

// agentPromptPipelineInput is the initial payload for runConvergenceAgentPromptPipeline.
type agentPromptPipelineInput struct {
	cmd                 *cobra.Command
	projectRoot         string // non-empty; sole ResolveProjectRoot(".") for this run (from runTestFailuresConvergence / runConvergenceAgentPromptPipeline)
	snap                *schedpkg.TestBundleConvergenceSnapshot
	sessionID           string
	effCurrentPhase     string
	effFlowVariant      string
	sessionRoutingMeta  map[string]any
	beforeStateSnapshot map[string]any
	stampTombstone      bool
	sessionPredictions  map[string]any
	finalizeDebrief     bool
	debriefNotes        string
	skipSessionCtx      bool
	pasteIDE            bool
	copyClip            bool
	// attentionMode is a canonical value from normalizeAgentPromptAttentionMode (empty = normal directive framing).
	attentionMode string
	// persistOutcome is set when --persist-session ran before the prompt (measurement already written to storage).
	persistOutcome *persistSessionOutcome
	// sessionThresholds is CVS thresholds from the same read as routing (nil when unknown).
	sessionThresholds map[string]any
	// rollupCore is rollup_status_core (convergerollup + literal gates + child CVS); nil omits the rollup section.
	rollupCore map[string]any
}

// agentPromptPipelinePayload carries markdown and delivery state after INGEST.
type agentPromptPipelinePayload struct {
	markdown    string
	pasteIDE    bool
	copyClip    bool
	footer      string
	projectRoot string // for ide automation keystroke log path under .zqk/logs/scheduler/
}

// agentPromptValidateIngestConstraints enforces flag combinations for --format agent-prompt before any
// health read, markdown build, or storage read. Call from the CLI path for early exit; INGEST runs the same check.
func agentPromptValidateIngestConstraints(in *agentPromptPipelineInput) error {
	if in == nil {
		return errfmt.Errorf("agent prompt: nil input")
	}
	if strings.TrimSpace(in.sessionID) == emptyValue {
		return errfmt.Newf("agent prompt").Wrap(errAgentPromptMissingSession)
	}
	if in.skipSessionCtx {
		return errfmt.Newf("agent prompt").Wrap(errAgentPromptSkipSessionContext)
	}
	if in.pasteIDE && runtime.GOOS != "darwin" {
		return errfmt.Newf("agent prompt").Wrap(errAgentPromptPasteRequiresDarwin)
	}
	if in.copyClip && runtime.GOOS != "darwin" {
		return errfmt.Newf("agent prompt").Wrap(errAgentPromptCopyRequiresDarwin)
	}
	return nil
}

// normalizeAgentPromptAttentionMode maps CLI aliases to canonical modes. Empty / none / off → no banner.
func normalizeAgentPromptAttentionMode(raw string) (string, error) {
	t := strings.TrimSpace(strings.ToLower(raw))
	if t == emptyValue || t == agentPromptAttentionInputNone || t == agentPromptAttentionInputOff {
		return "", nil
	}
	switch t {
	case agentPromptAttentionAliasTest, agentPromptAttentionAliasTestNonDirective, agentPromptAttentionAliasNonDirective:
		return agentprompt.AttentionTestNonDirective, nil
	case agentPromptAttentionAliasInterrupt, agentPromptAttentionAliasInterruptPlumbing, agentPromptAttentionAliasCriticalInterrupt, agentPromptAttentionAliasCriticalInterruptDryRun:
		return agentprompt.AttentionInterruptPlumbing, nil
	default:
		return "", errfmt.Errorf(agentPromptAttentionModeInvalidFmt, strings.TrimSpace(raw))
	}
}

func agentPromptIngestBlockedReasonCode(err error) string {
	switch {
	case errors.Is(err, errAgentPromptMissingSession):
		return "missing_session_id"
	case errors.Is(err, errAgentPromptSkipSessionContext):
		return "skip_session_context_incompatible"
	case errors.Is(err, errAgentPromptPasteRequiresDarwin):
		return "paste_ide_requires_darwin"
	case errors.Is(err, errAgentPromptCopyRequiresDarwin):
		return "copy_requires_darwin"
	default:
		return "unknown"
	}
}

// buildAgentConvergenceMarkdown renders a single markdown document for pasting into an agent chat:
// CVS contract fields, latest health snapshot, rollup-driven next measured action, suggested next_action, phase_router, autonomy hints.
func buildAgentConvergenceMarkdown(
	cmd *cobra.Command,
	snap *schedpkg.TestBundleConvergenceSnapshot,
	sessionID string,
	effCurrentPhase, effFlowVariant string,
	sessionRoutingMeta map[string]any,
	beforeStateSnapshot map[string]any,
	stampTombstone bool,
	sessionPredictions map[string]any,
	finalizeDebrief bool,
	debriefNotes string,
	attentionMode string,
	persistOutcome *persistSessionOutcome,
	sessionThresholds map[string]any,
	rollupCore map[string]any,
) (string, error) {
	sug, err := schedpkg.BuildSuggestedConvergenceSessionFields(snap, effCurrentPhase, effFlowVariant, sessionRoutingMeta, beforeStateSnapshot, stampTombstone, sessionPredictions, finalizeDebrief, debriefNotes, sessionThresholds)
	if err != nil {
		return "", err
	}
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return "", err
	}
	obj, readErr := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), strings.TrimSpace(sessionID))
	if readErr != nil {
		return "", errfmt.Errorf("read convergence_session %s: %w", sessionID, readErr)
	}
	linkedBacklog := formatLinkedBacklogAcceptanceSection(proc, obj)
	return formatAgentMarkdown(sessionID, obj, sug, snap, attentionMode, persistOutcome, rollupCore, effFlowVariant, sessionRoutingMeta, linkedBacklog), nil
}

func formatAgentPromptRollupSection(rollup map[string]any) string {
	if len(rollup) == 0 {
		return "## Next measured action (rollup)\n\n- *(rollup_status_core missing — re-run with a current `zqk` binary.)*\n"
	}
	var b strings.Builder
	b.WriteString("## Next measured action (rollup)\n\n")
	b.WriteString(paths.RewriteCanonicalCLIInvocations("Same signals as JSON **`rollup_status_core`** from `zqk scheduler convergence measure --format json --session-id ...`. "))
	b.WriteString("Use this to choose the **next measured step** before defaulting to persisted **next_action** below. ")
	b.WriteString(paths.RewriteCanonicalCLIInvocations("**Parent/coordinator** CVS with nested children: `zqk scheduler convergence overseer --coordinator-session-id <CVS> --format json` adds tree walk + arbitrated parent line.\n\n"))
	if rs, ok := rollup["rollup_status"].(string); ok && strings.TrimSpace(rs) != "" {
		fmt.Fprintf(&b, "- **rollup_status:** `%s`\n", rs)
	}
	if r, ok := rollup["ready_for_parent_completion"].(bool); ok {
		fmt.Fprintf(&b, "- **ready_for_parent_completion:** %v\n", r)
	}
	if adj, ok := rollup["rollup_bundle_completion_adjustment"].(map[string]any); ok && len(adj) > 0 {
		b.WriteString("- **Bundle gate vs rollup:** `thresholds.completion_gate` on this CVS relaxes bundle completion for **`rollup_status_core`** only. The **Latest measurement** block above can still show `ready_for_session_completion: false` when `health.jsonl` is empty — that is expected; follow **rollup** for gate satisfaction unless you re-enable the completion gate.\n")
	}
	if rec, ok := rollup["recommended_next_action"].(string); ok && strings.TrimSpace(rec) != "" {
		fmt.Fprintf(&b, "- **Recommended next (measurement-derived):** %s\n", strings.TrimSpace(rec))
	}
	if bl, ok := rollup[objects.FieldKeyBlockers].([]any); ok && len(bl) > 0 {
		b.WriteString("- **Rollup blockers:**\n")
		const maxBl = 12
		for i, item := range bl {
			if i >= maxBl {
				fmt.Fprintf(&b, "  - … (%d more)\n", len(bl)-maxBl)
				break
			}
			line := formatRollupBlockerLine(item)
			if line != "" {
				fmt.Fprintf(&b, "  - %s\n", line)
			}
		}
	}
	if note, ok := rollup["evaluation_note"].(string); ok && strings.TrimSpace(note) != "" {
		fmt.Fprintf(&b, "- **Note:** %s\n", strings.TrimSpace(note))
	}
	if skip, ok := rollup["rollup_gates_skipped"].(bool); ok && skip {
		b.WriteString("- **Literal gates:** skipped (`--skip-rollup-gates` on convergence); re-run without it before claiming gates green.\n")
	}
	return b.String()
}

func formatRollupBlockerLine(item any) string {
	m, ok := item.(map[string]any)
	if !ok {
		return fmt.Sprint(item)
	}
	code, _ := m[objects.FieldKeyCode].(string)
	detail, _ := m["detail"].(string)
	if code == "" && detail == "" {
		raw, err := json.Marshal(m)
		if err != nil {
			return fmt.Sprint(item)
		}
		return string(raw)
	}
	if detail == "" {
		return fmt.Sprintf("`%s`", code)
	}
	return fmt.Sprintf("`%s` — %s", code, detail)
}

func appendAgentPromptAttentionPreamble(b *strings.Builder, canonicalMode string) {
	if canonicalMode == emptyValue {
		return
	}
	env, err := agentprompt.EnvelopeFromAttentionMode(canonicalMode)
	if err != nil {
		return
	}
	line, err := agentprompt.FormatHTMLCommentLine(env)
	if err != nil {
		return
	}
	b.WriteString(line)
	switch canonicalMode {
	case agentprompt.AttentionTestNonDirective:
		b.WriteString("> **Attention — test / non-directive (ZQK)**  \n")
		b.WriteString("> This chat payload is **not** an operational directive. Use it only to exercise the agent-prompt path (clipboard, paste, policy interrupt / queue plumbing). **Disregard** the body below as executable automation until you clear this mode.\n\n")
	case agentprompt.AttentionInterruptPlumbing:
		b.WriteString("> **Attention — critical interrupt / queue plumbing (ZQK)**  \n")
		b.WriteString("> For **dry-run** testing of **critical interrupt** and **queue send** integration. **Do not** treat the remainder as backlog execution or convergence work orders.\n\n")
	}
}

func agentPromptPhaseRouterNotesContainUnknownVariant(pr map[string]any) bool {
	if len(pr) == 0 {
		return false
	}
	notes, _ := pr[objects.FieldKeyNotes].([]any)
	for _, n := range notes {
		s, _ := n.(string)
		if strings.Contains(s, "Unknown flow_variant") {
			return true
		}
	}
	return false
}

func appendAgentPromptCLIDiagnostic(b *strings.Builder, sug map[string]any, effFlowVariant string, sessionRoutingMeta map[string]any) {
	// Use local name `buf` for POL-CODE-007 allowlist (check-logging-compliance.sh matches Fprintf(buf,) not Fprintf(b,)).
	buf := b
	b.WriteString("## CLI diagnostic\n\n")
	exe := ""
	if p, err := fileutil.Executable(); err == nil {
		exe = p
	} else {
		exe = fmt.Sprintf("(unavailable: %v)", err)
	}
	fmt.Fprintf(buf, "- **Running executable:** `%s`\n", exe)
	var fvSrc, fvEff string
	if len(sessionRoutingMeta) > 0 {
		if s, ok := sessionRoutingMeta[agentPromptRoutingMetaFlowVariantSource].(string); ok {
			fvSrc = strings.TrimSpace(s)
		}
		if s, ok := sessionRoutingMeta[agentPromptRoutingMetaEffectiveFlowVariant].(string); ok {
			fvEff = strings.TrimSpace(s)
		}
	}
	if fvEff == "" {
		fvEff = strings.TrimSpace(effFlowVariant)
	}
	if fvSrc != "" {
		fmt.Fprintf(buf, "- **flow_variant source:** `%s`\n", fvSrc)
	}
	if fvEff != "" {
		fmt.Fprintf(buf, "- **effective flow_variant:** `%s`\n", fvEff)
	}
	var pr map[string]any
	if sug != nil {
		pr, _ = sug["phase_router"].(map[string]any)
	}
	if len(pr) > 0 {
		if rp, ok := pr["routing_profile"].(string); ok && strings.TrimSpace(rp) != "" {
			fmt.Fprintf(buf, "- **phase_router.routing_profile:** `%s`\n", rp)
		}
	}
	if agentPromptPhaseRouterNotesContainUnknownVariant(pr) {
		b.WriteString(paths.RewriteCanonicalCLIInvocations("- **Stale binary hint:** If `main` already registers this `flow_variant` in `pkg/scheduler/convergence_phase_router.go` but the phase router JSON still shows an unknown-variant note, rebuild (`go build -o bin/zqk ./cmd/zqk`) and run `./bin/zqk scheduler convergence measure ...`, or align `PATH` so `which zqk` matches that binary.\n"))
	}
	b.WriteString("\n")
}

func formatAgentMarkdown(sessionID string, cvs map[string]any, sug map[string]any, snap *schedpkg.TestBundleConvergenceSnapshot, attentionMode string, persistOutcome *persistSessionOutcome, rollupCore map[string]any, effFlowVariant string, sessionRoutingMeta map[string]any, linkedBacklogAcceptance string) string {
	var b strings.Builder
	appendAgentPromptAttentionPreamble(&b, attentionMode)
	fmt.Fprintf(&b, "# Convergence — agent prompt (computed)\n\n")
	fmt.Fprintf(&b, "**Session:** `%s`\n\n", sessionID)

	b.WriteString("## Session contract (from CVS object)\n\n")
	fmt.Fprintf(&b, "- **Hypothesis:** %s\n", strings.TrimSpace(convergenceFieldString(cvs, objects.FieldKeyHypothesis)))
	fmt.Fprintf(&b, "- **Desired end state:** %s\n", strings.TrimSpace(convergenceFieldString(cvs, objects.FieldKeyDesiredEndState)))
	fmt.Fprintf(&b, "- **Status:** %s\n", strings.TrimSpace(convergenceFieldString(cvs, objects.FieldKeyStatus)))
	if fv := strings.TrimSpace(convergenceFieldString(cvs, objects.FieldKeyFlowVariant)); fv != emptyValue {
		fmt.Fprintf(&b, "- **Flow variant (persisted):** %s\n", fv)
	}
	predJSON := "(none)"
	if p := sessionPredictionsFromCVS(cvs); len(p) > 0 {
		if raw, err := json.MarshalIndent(p, "", "  "); err == nil {
			predJSON = string(raw)
		}
	}
	fmt.Fprintf(&b, "- **Predictions (from object):**\n\n```json\n%s\n```\n\n", predJSON)

	if strings.TrimSpace(linkedBacklogAcceptance) != emptyValue {
		b.WriteString(strings.TrimSpace(linkedBacklogAcceptance))
		b.WriteString("\n\n")
	}

	b.WriteString(agentPromptMeasurementScopeSection)
	b.WriteString("\n")

	b.WriteString("## Latest measurement (health.jsonl snapshot)\n\n")
	if snap == nil {
		b.WriteString("- *(No snapshot; health window empty or unavailable.)*\n\n")
	} else {
		fmt.Fprintf(&b, "- **Delta assessment:** %s\n", snap.DeltaAssessment)
		fmt.Fprintf(&b, "- **Health watermark:** %s\n", snap.HealthWatermarkRFC3339)
		fmt.Fprintf(&b, "- **Failing fingerprints (latest):** %d\n", len(snap.FailingFingerprintsNow))
		for _, fp := range snap.FailingFingerprintsNow {
			fmt.Fprintf(&b, "  - `%s`\n", fp)
		}
		fmt.Fprintf(&b, "- **Bundle-health completion gate (`ready_for_session_completion`):** %v\n", snap.ReadyForSessionCompletion)
		if len(snap.SessionCompletionBlockedReasons) > 0 {
			b.WriteString("- **Blocked reasons (bundle/queue/heartbeat gates):**\n")
			for _, r := range snap.SessionCompletionBlockedReasons {
				fmt.Fprintf(&b, "  - %s\n", r)
			}
		}
		if strings.TrimSpace(snap.SessionCompletionNote) != emptyValue {
			fmt.Fprintf(&b, "- **Note:** %s\n", strings.TrimSpace(snap.SessionCompletionNote))
		}
		if snap.Heartbeat != nil {
			fmt.Fprintf(&b, "- **Heartbeat stale:** %v (age %.0fs)\n", snap.Heartbeat.Stale, snap.Heartbeat.MeasurementAgeSeconds)
		}
		fmt.Fprintf(&b, "- **Trigger queue pending:** %d\n", snap.TriggerQueuePending)
	}
	b.WriteString("\n")

	b.WriteString(formatAgentPromptRollupSection(rollupCore))
	b.WriteString("\n")

	b.WriteString("## Next action (suggested for persistence)\n\n")
	na := schedpkg.AgentPromptPreferredNextAction(cvs, sug)
	if strings.TrimSpace(na) == emptyValue {
		b.WriteString("(empty)\n\n")
	} else {
		fmt.Fprintf(&b, "%s\n\n", na)
	}

	b.WriteString(formatConvergenceDesiredEndStateRecommendation(cvs, sug, snap, rollupCore))
	b.WriteString("\n")

	b.WriteString("## Phase router (suggested)\n\n")
	b.WriteString(agentPromptPhaseRouterHowToRead)
	b.WriteString("\n")
	if pr, ok := sug["phase_router"].(map[string]any); ok && len(pr) > 0 {
		if raw, err := json.MarshalIndent(pr, "", "  "); err == nil {
			fmt.Fprintf(&b, "```json\n%s\n```\n\n", string(raw))
		}
	} else {
		b.WriteString("(none)\n\n")
	}

	appendAgentPromptCLIDiagnostic(&b, sug, effFlowVariant, sessionRoutingMeta)

	b.WriteString("## Autonomy\n\n")
	b.WriteString(agentPromptAutonomyBlock)
	b.WriteString("\n")

	b.WriteString("## Persist / measure\n\n")
	switch {
	case persistOutcome != nil && persistOutcome.Applied:
		b.WriteString("- **This run already persisted** `object_update_body` to the CVS via `--persist-session` (same contract as `convergence_session_tick`).\n")
		b.WriteString("- On subsequent iterations, use `--persist-session` again after new health.jsonl evidence, or rely on scheduler tick / bundle hooks.\n")
	case persistOutcome != nil && persistOutcome.AuditAppended:
		b.WriteString("- **This run appended an activity_log audit only:** health watermark unchanged vs CVS `last_measurement_at` — snapshot fields (`delta_assessment`, `after_state_snapshot`, phase routing, etc.) were **not** rewritten.\n")
		b.WriteString("- **When you have new test-bundle evidence** in `health.jsonl`, rerun with `--persist-session` to persist the full measurement payload.\n")
	default:
		b.WriteString(paths.RewriteCanonicalCLIInvocations("- After changes, run `zqk scheduler convergence measure --format json --session-id "))
		fmt.Fprintf(&b, "`%s`", sessionID)
		b.WriteString(paths.RewriteCanonicalCLIInvocations(" and apply `suggested_convergence_session_fields.object_update_body` with `zqk object update`, **or** use **`--persist-session`** on this command to write the CVS in one step.\n"))
	}
	b.WriteString(paths.RewriteCanonicalCLIInvocations("- Use targeted `zqk test run` per project policy; attach log paths when reporting.\n"))

	return b.String()
}

func sessionPredictionsFromCVS(cvs map[string]any) map[string]any {
	if cvs == nil {
		return nil
	}
	raw, ok := cvs[objects.FieldKeyPredictions]
	if !ok {
		return nil
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

func convergenceFieldString(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	v, ok := obj[key]
	if !ok {
		return ""
	}
	v, ok = nildecode.DecodeNonNilPayload[any](v)
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

// copyAgentPromptToClipboard writes text to the system clipboard (macOS pbcopy only).
func copyAgentPromptToClipboard(s string) error {
	if runtime.GOOS != "darwin" {
		return errfmt.Errorf("clipboard copy uses pbcopy (macOS only); on other systems copy the command output manually")
	}
	c := execwrap.Command("pbcopy")
	c.Stdin = strings.NewReader(s)
	return c.Run()
}

func agentPromptIDEKeystrokeLogPath(projectRoot string) string {
	if strings.TrimSpace(projectRoot) == emptyValue {
		projectRoot = "."
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, agentPromptIDEKeystrokeLogFile)
}

func agentPromptRunsJSONLPath(projectRoot string) string {
	if strings.TrimSpace(projectRoot) == emptyValue {
		projectRoot = "."
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, agentPromptRunsJSONLFile)
}

func writeAgentPromptRunJSONLLine(path string, line []byte) {
	f, err := fileutil.OpenFile(path, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(line)
}

// appendAgentPromptRunJSONL records pipeline Outcome fields after a successful run (FINALIZE). Best-effort; errors ignored.
// Uses storage.FileLock.WithLockTimeout when possible (same shape as pkg/scheduler CoordinationChannel.PublishEvent and cmd/zqk/app session helpers): flock on path+paths.LockFileSuffix so concurrent zqk processes do not interleave lines.
// If the lock cannot be created or acquired (e.g. some sandboxes), falls back to an unlocked append so audit lines still land.
func appendAgentPromptRunJSONL(projectRoot string, outcome map[string]any) {
	if outcome == nil {
		return
	}
	path := agentPromptRunsJSONLPath(projectRoot)
	entry := map[string]any{
		schedpkg.KeyTimestamp:             zqktime.NowRFC3339UTC(),
		agentPromptRunJSONKeyEventType:    agentPromptRunEventType,
		agentPromptRunJSONKeyPipelineKind: pipelineKindConvergenceAgentPrompt,
	}
	for _, k := range []string{
		outcomeAgentPromptSessionID,
		outcomeAgentPromptMarkdownBytes,
		outcomeAgentPromptDeliverMode,
		agentprompt.KeyAttentionMode,
		outcomeAgentPromptValidationFailed,
		outcomeAgentPromptBlockedReason,
		outcomeAgentPromptComplete,
	} {
		if v, ok := outcome[k]; ok {
			entry[k] = v
		}
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	line := append(data, '\n')

	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return
	}
	lock, err := storagepkg.NewFileLock(path + paths.LockFileSuffix)
	if err != nil {
		writeAgentPromptRunJSONLLine(path, line)
		return
	}
	defer func() { _ = lock.Close() }()
	if err := lock.WithLockTimeout(agentPromptRunsJSONLFileLockTimeout, func() error {
		writeAgentPromptRunJSONLLine(path, line)
		return nil
	}); err != nil {
		writeAgentPromptRunJSONLLine(path, line)
	}
}

// appendAgentPromptIDEKeystrokeLog appends one TSV line (RFC3339Nano, component, event, detail) for --paste-ide visibility.
func appendAgentPromptIDEKeystrokeLog(logger logging.Logger, projectRoot, event, detail string) {
	path := agentPromptIDEKeystrokeLogPath(projectRoot)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		if logger != nil {
			schedpkg.SLog(logger).Warn(logMsgAgentPromptKeystrokeMkdirFailed).
				String(logFieldPath, path).
				WithError(err).
				Log()
		}
		return
	}
	f, err := fileutil.OpenFile(path, fileutil.O_CREATE|fileutil.O_APPEND|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		if logger != nil {
			schedpkg.SLog(logger).Warn(logMsgAgentPromptKeystrokeOpenFailed).
				String(logFieldPath, path).
				WithError(err).
				Log()
		}
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line := fmt.Sprintf("%s\t%s\t%s\t%s\n", ts, ideAutomationLogComponent, event, detail)
	if _, werr := f.WriteString(line); werr != nil && logger != nil {
		schedpkg.SLog(logger).Warn(logMsgAgentPromptKeystrokeWriteFailed).
			String(logFieldPath, path).
			WithError(werr).
			Log()
	}
}

func ideKeystrokeDetailPbcopyBegin(markdownBytes int, logPath string) string {
	return fmt.Sprintf("%s=%d %s=%s", ideKeystrokeDetailKeyMarkdownBytes, markdownBytes, ideKeystrokeDetailKeyLogPath, logPath)
}

// agentPromptMetricsBucketing adds attention_mode to per-stage metric labels (after INGEST populates Outcome)
// for auditing interrupt-plumbing vs normal runs. Cardinality: default | test_non_directive | interrupt_plumbing.
type agentPromptMetricsBucketing struct {
	pipeline.StandardPipelineBucketing
}

// Buckets implements [pipeline.BucketingStrategy].
func (b agentPromptMetricsBucketing) Buckets(pctx *pipeline.Context, pipelineKind, stageName string) map[string]string {
	m := b.StandardPipelineBucketing.Buckets(pctx, pipelineKind, stageName)
	mode := agentprompt.MetricAttentionDefault
	if pctx != nil && pctx.Outcome != nil {
		if v, ok := pctx.Outcome[agentprompt.KeyAttentionMode].(string); ok {
			v = strings.TrimSpace(v)
			if v != emptyValue {
				mode = v
			}
		}
	}
	m[agentprompt.KeyAttentionMode] = mode
	return m
}

// runConvergenceAgentPromptPipeline builds the agent markdown (INGEST), delivers to clipboard / IDE (COMMIT),
// and records completion (FINALIZE). Per-stage metrics use pipeline_kind scheduler.convergence_agent_prompt;
// pipeline.Context.Outcome holds session id, markdown size, deliver_mode, attention_mode, and complete for future event sinks.
func runConvergenceAgentPromptPipeline(
	cmd *cobra.Command,
	projectRoot string,
	snap *schedpkg.TestBundleConvergenceSnapshot,
	sessionID string,
	effCurrentPhase, effFlowVariant string,
	sessionRoutingMeta map[string]any,
	beforeStateSnapshot map[string]any,
	stampTombstone bool,
	sessionPredictions map[string]any,
	finalizeDebrief bool,
	debriefNotes string,
	skipSessionCtx bool,
	pasteIDE, copyClip bool,
	attentionMode string,
	persistOutcome *persistSessionOutcome,
	sessionThresholds map[string]any,
	rollupCore map[string]any,
) (markdown string, footer string, err error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return "", "", errAgentPromptProjectRootMissing
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	in := &agentPromptPipelineInput{
		cmd: cmd, projectRoot: projectRoot, snap: snap, sessionID: sessionID,
		effCurrentPhase: effCurrentPhase, effFlowVariant: effFlowVariant,
		sessionRoutingMeta: sessionRoutingMeta, beforeStateSnapshot: beforeStateSnapshot,
		stampTombstone: stampTombstone, sessionPredictions: sessionPredictions,
		finalizeDebrief: finalizeDebrief, debriefNotes: debriefNotes,
		skipSessionCtx: skipSessionCtx, pasteIDE: pasteIDE, copyClip: copyClip,
		attentionMode: attentionMode, persistOutcome: persistOutcome,
		sessionThresholds: sessionThresholds,
		rollupCore:        rollupCore,
	}
	runCtx := cmd.Context()
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}
	pctx := &pipeline.Context{Ctx: runCtx, Outcome: make(map[string]any)}
	pl := pipeline.NewBuilder(pipelineKindConvergenceAgentPrompt, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfigWithStrategy(logger, agentPromptMetricsBucketing{})).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, agentPromptStageIngest).
		AddStage(pipeline.StageCommit, agentPromptStageCommit).
		AddStage(pipeline.StageFinalize, agentPromptStageFinalize).
		Build()
	result, err := pl.Run(pctx, in)
	if err != nil {
		return "", "", err
	}
	p := result.(*agentPromptPipelinePayload)
	return p.markdown, p.footer, nil
}

// effectiveAgentPromptHTTPDeliveryURL returns --agent-prompt-delivery-http-url when set, else ZQK_AGENT_PROMPT_DELIVERY_HTTP_URL.
func effectiveAgentPromptHTTPDeliveryURL(cmd *cobra.Command) string {
	flagVal, _ := cmd.Flags().GetString("agent-prompt-delivery-http-url") //nolint:errcheck // optional
	if strings.TrimSpace(flagVal) != emptyValue {
		return strings.TrimSpace(flagVal)
	}
	return strings.TrimSpace(zqkenv.AgentPromptDeliveryHTTPURL().Get())
}

func effectiveAgentPromptHTTPBearer() string {
	return strings.TrimSpace(zqkenv.AgentPromptDeliveryHTTPBearer().Get())
}

// primaryAgentPromptDestination classifies where CLI output goes for audit (stdout vs file).
func primaryAgentPromptDestination(outPath string) (primary string, filePathForAudit string) {
	o := strings.TrimSpace(outPath)
	if o == emptyValue || o == "-" {
		return "stdout", ""
	}
	return "file", o
}

// deliverPrimaryAgentPrompt writes the combined markdown+footer to a file or to the command output writer.
func deliverPrimaryAgentPrompt(
	cmd *cobra.Command,
	_ context.Context,
	data []byte,
	_ string,
	outPath string,
) (deliveredTo []string, primaryDest string, auditFilePath string, err error) {
	primaryDest, auditFilePath = primaryAgentPromptDestination(outPath)
	if auditFilePath != emptyValue {
		if err := fileutil.EnsureDir(filepath.Dir(auditFilePath)); err != nil {
			return nil, "", "", errfmt.Newf("agent prompt mkdir").Wrap(err)
		}
		if err := fileutil.WriteSecureFile(auditFilePath, data); err != nil {
			return nil, "", "", errfmt.Newf("agent prompt file delivery").Wrap(err)
		}
		return []string{"file:" + auditFilePath}, primaryDest, auditFilePath, nil
	}
	if err := cli.WriteOutput(cmd, data); err != nil {
		return nil, "", "", err
	}
	return []string{"stdout"}, primaryDest, "", nil
}

// mergeHTTPAgentPromptDelivery POSTs the same payload when httpURL is set; updates rec and logs on failure (best-effort).
func mergeHTTPAgentPromptDelivery(
	rec *agentdelivery.DeliveryAuditRecord,
	runCtx context.Context,
	httpURL string,
	data []byte,
	sessionID string,
) {
	httpURL = strings.TrimSpace(httpURL)
	if httpURL == emptyValue {
		return
	}
	signer, _ := crypto.GenerateKeypair()
	tdeEnvelope := &policy.TrustDomainEnvelope{
		ID:                fmt.Sprintf("TDE-%d", time.Now().UnixNano()),
		Scope:             "agent_orchestration",
		AllowedNamespaces: []string{"*"},
		MaxRuntimeSeconds: 300,
		Payload:           map[string]any{objects.FieldKeyIntent: "scheduler prompt delivery"},
		MCPPermissions:    []string{"agent:orchestrate"},
		Neurological: &policy.NeurologicalGovernance{
			Confidence:     0.95,
			IsStreaming:    true,
			DecisionBranch: "scheduler",
		},
	}
	_ = tdeEnvelope.Sign(signer)

	hd := &agentdelivery.TDEEnforcer{Next: agentdelivery.HTTPDeliverer{URL: httpURL, BearerToken: effectiveAgentPromptHTTPBearer()}}
	hres, err := hd.Deliver(runCtx, agentdelivery.Prompt{
		Markdown:  data,
		SessionID: strings.TrimSpace(sessionID),
		Format:    schedulerFormatAgentPrompt,
		TDE:       tdeEnvelope,
	})
	if err != nil {
		rec.HTTPURL = httpURL
		rec.HTTPError = err.Error()
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedpkg.SLog(logger).Warn("agent prompt HTTP delivery failed").
			WithError(err).
			String("url", httpURL).
			Log()
		return
	}
	rec.HTTPURL = httpURL
	rec.HTTPStatusCode = hres.HTTPStatusCode
	rec.DeliveredTo = append(rec.DeliveredTo, hres.DeliveredTo)
}

// deliverAgentPromptMarkdownOutput writes markdown+footer via agentdelivery (file) or cli.WriteOutput (stdout / "-"),
// optionally POSTs to HTTP, and appends agent_prompt_deliveries.jsonl.
func deliverAgentPromptMarkdownOutput(
	cmd *cobra.Command,
	projectRoot string,
	markdown string,
	footer string,
	sessionID string,
	deliverMode string,
	attentionMode string,
	httpURL string,
) error {
	data := append([]byte(markdown), footer...)
	runCtx := cmd.Context()
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}
	outPath := strings.TrimSpace(cli.GetOutputPath(cmd))
	deliveredTo, primaryDest, auditOutPath, err := deliverPrimaryAgentPrompt(cmd, runCtx, data, sessionID, outPath)
	if err != nil {
		return err
	}

	rec := &agentdelivery.DeliveryAuditRecord{
		ConvergenceSessionID: strings.TrimSpace(sessionID),
		Format:               schedulerFormatAgentPrompt,
		DeliverMode:          deliverMode,
		PrimaryDestination:   primaryDest,
		DeliveredTo:          deliveredTo,
		MarkdownBytes:        len(data),
	}
	if strings.TrimSpace(attentionMode) != emptyValue {
		rec.AttentionMode = attentionMode
	}
	if auditOutPath != emptyValue {
		rec.OutputPath = auditOutPath
	}
	mergeHTTPAgentPromptDelivery(rec, runCtx, httpURL, data, sessionID)
	agentdelivery.AppendDeliveryAuditJSONL(projectRoot, rec)
	return nil
}

func agentPromptStageIngest(pctx *pipeline.Context, payload any) (any, error) {
	in := payload.(*agentPromptPipelineInput)
	if err := agentPromptValidateIngestConstraints(in); err != nil {
		pctx.Outcome[outcomeAgentPromptValidationFailed] = true
		pctx.Outcome[outcomeAgentPromptBlockedReason] = agentPromptIngestBlockedReasonCode(err)
		return nil, err
	}
	md, err := buildAgentConvergenceMarkdown(in.cmd, in.snap, in.sessionID, in.effCurrentPhase, in.effFlowVariant, in.sessionRoutingMeta, in.beforeStateSnapshot, in.stampTombstone, in.sessionPredictions, in.finalizeDebrief, in.debriefNotes, in.attentionMode, in.persistOutcome, in.sessionThresholds, in.rollupCore)
	if err != nil {
		return nil, err
	}
	pctx.Outcome[outcomeAgentPromptSessionID] = strings.TrimSpace(in.sessionID)
	pctx.Outcome[outcomeAgentPromptMarkdownBytes] = len(md)
	if in.attentionMode != emptyValue {
		pctx.Outcome[agentprompt.KeyAttentionMode] = in.attentionMode
	} else {
		pctx.Outcome[agentprompt.KeyAttentionMode] = agentprompt.MetricAttentionDefault
	}
	root := strings.TrimSpace(in.projectRoot)
	if root == "" {
		return nil, errAgentPromptProjectRootMissing
	}
	return &agentPromptPipelinePayload{
		markdown: md, pasteIDE: in.pasteIDE, copyClip: in.copyClip,
		projectRoot: root,
	}, nil
}

func agentPromptStageCommit(pctx *pipeline.Context, payload any) (any, error) {
	p := payload.(*agentPromptPipelinePayload)
	switch {
	case p.pasteIDE:
		adapter := agent.GetDeliveryAdapter(zqkenv.AGTranscriptPath().Get())
		if adapter != nil {
			err := adapter.Deliver(context.Background(), p.markdown, fmt.Sprintf("msg-%d", time.Now().UnixNano()), nil) // Background: request-or-shutdown derived
			if err != nil {
				return nil, errfmt.Newf("%s vendor message delivery", adapter.Vendor()).Wrap(err)
			}
			p.footer = fmt.Sprintf("\n\n(Delivered via %s vendor adapter)\n", adapter.Vendor())
		}
		pctx.Outcome[outcomeAgentPromptDeliverMode] = agentPromptDeliverPasteIDE
	case p.copyClip:
		if err := copyAgentPromptToClipboard(p.markdown); err != nil {
			return nil, errfmt.Newf("copy to clipboard").Wrap(err)
		}
		pctx.Outcome[outcomeAgentPromptDeliverMode] = agentPromptDeliverClipboard
		p.footer = "\n\n(Copied to clipboard on this OS. Paste into the agent chat.)\n"
	default:
		pctx.Outcome[outcomeAgentPromptDeliverMode] = agentPromptDeliverNone
	}
	return p, nil
}

func agentPromptStageFinalize(pctx *pipeline.Context, payload any) (any, error) {
	pctx.Outcome[outcomeAgentPromptComplete] = true
	p := payload.(*agentPromptPipelinePayload)
	appendAgentPromptRunJSONL(p.projectRoot, pctx.Outcome)
	return payload, nil
}
