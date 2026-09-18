package interactionpolicy

// Step is the kernel pong: a coherent next action for the seated persona.
type Step struct {
	Event       string `json:"event"`
	PolicyID    string `json:"policy_id"`
	GuidingStep string `json:"guiding_step"`
	CommandHint string `json:"command_hint,omitempty"`
}

type catalogEntry struct {
	events []string
	step   Step
}

// catalog is the fast ping-pong reflex when CAS is unavailable.
// CLI overlays GuidingStep from the policy body (OverlayFromPolicy).
// TRACK: BLI-1787035087372193000-c022117d
var catalog = []catalogEntry{
	{
		events: []string{EventGoTest},
		step: Step{
			PolicyID:    PolicyAdminMembrane,
			GuidingStep: "You ran tests. Persist SCH id + log path on the ATK via zqk object update. Do not set status=implemented from chat. Worker must not call zqk agent next (coordinator owns it).",
			CommandHint: "zqk object update <ATK-id> --field evidence_refs=...",
		},
	},
	{
		events: []string{EventGitCommit},
		step: Step{
			PolicyID:    PolicyAdminMembrane,
			GuidingStep: "After commit in an ATK worktree, merge to integration/pri-* then tear down the worktree before zqk agent next.",
			CommandHint: "git worktree remove <path>",
		},
	},
	{
		events: []string{EventGitWorktreeAdd},
		step: Step{
			PolicyID:    PolicyAdminMembrane,
			GuidingStep: "ATK worktrees must not be under the studio project (POL-AGENT-WORKTREE-ISOLATION-001). Use paths.AgentWorktreeDir / $TMPDIR/zqk-worktrees.",
			CommandHint: "zqk agent orchestrate (kernel sets isolated worktree path)",
		},
	},
	{
		events: []string{EventAgentOrchestrate},
		step: Step{
			PolicyID:    PolicyAdminMembrane,
			GuidingStep: "Run zqk agent prepare-context with --persona-ref before orchestrate (WFL-SUBAGENT-DISPATCH).",
			CommandHint: "zqk agent prepare-context --persona-ref PER-* <ATK-id>",
		},
	},
	{
		events: []string{EventAgentExecute},
		step: Step{
			PolicyID:    PolicyAdminMembrane,
			GuidingStep: "execute is the coder. Do not zqk agent next from this worktree; coordinator transitions after merge-up.",
		},
	},
	{
		// Idle with planned>0 is not an empty priority_plan. Do not share GROOM-AHEAD
		// text — OverlayFromPolicy would tell TPM to groom when the PRI already has planned BLIs.
		// TRACK: BLI-1787035087372193000-c022117d
		events: []string{EventIdle},
		step: Step{
			PolicyID:    PolicyTPMProcessAdmin,
			GuidingStep: "Lead priority_plan has planned BLIs. Do not remint ORCHESTRATE_PLAN unless the plan has a live ATK (pending/in_progress/approved/proposed). Terminal-only ATKs (implemented/archived/error) are inventory, not a dispatch signal. If fill_item is present, do that kernel fill instead of shutdown. Chat is not the scheduler. Do not claim orch-bound work. Do not re-groom items already planned.",
			CommandHint: "",
		},
	},
	{
		events: []string{EventInboxUnacked},
		step: Step{
			PolicyID:    PolicyTPMProcessAdmin,
			GuidingStep: "Inbox has unacked swarm mail. feed ack as this seat. Hourglass only when the body names a live ATK or a COMMS-CHECK nonce. Do not hourglass park, skip, peer_ack, or scheduler-callback wakes.",
			CommandHint: "zqk feed ack --agent-id <seat> --persona-ref <persona> --in-reply-to <AFE>",
		},
	},
	{
		events: []string{EventPushAhead},
		step: Step{
			PolicyID:    PolicyTPMProcessAdmin,
			GuidingStep: "Seated plan trunk is ahead of its upstream. Push origin HEAD (integration/pri-*) before groom-ahead or align. Do not absorb peer orchestrate. Public-push gates still apply.",
			CommandHint: HintPushAhead,
		},
	},
	{
		events: []string{EventShovelReadyEmpty},
		step: Step{
			PolicyID:    PolicyTPMGroomAhead,
			GuidingStep: "Unsealed intake only: seated priority_plan (PRI-*, Gantt matrix column) is not execution-locked and has exploring/validated BLIs. Promote those to planned. Do not mint onto in_progress/complete priority_plans. TPM must not claim orch-bound work.",
			CommandHint: "zqk object list backlog_item --filter priority_plan_ref=<PRI>",
		},
	},
	{
		events: []string{EventStratplanAhead},
		step: Step{
			PolicyID:    PolicyTPMGroomAhead,
			GuidingStep: "Lead priority_plan (PRI-*, Gantt matrix column) planned count is 0 because that PRI is executing or already done — not because intake needs restuffing. Do not break the seal (no mint, no demote to planned). If align-latest.json is fresh (<30m), do not re-run align; shape the next unlocked priority_plan still in grooming and classify draft plane. Else persist a new align cache. Keep active_order honest to mission/vision/strategic_plan. Objectify via gen-trace-pipeline (POL-AGENT-TPM-TRACE-PIPELINE-001): do not hand-mint 1:1 REQ→CRIT. " + HintTracePipeline,
			CommandHint: HintAlignRefresh,
		},
	},
	{
		events: []string{EventCommsFail},
		step: Step{
			PolicyID:    PolicyCommsRemedy,
			GuidingStep: "Channel is sick — not an idle-coder problem. Stop ATK completion on that seat. Repair seating and re-run COMMS-CHECK until life and work pass.",
			CommandHint: "zqk feed doctor --refresh-seats",
		},
	},
	{
		events: []string{EventLifecycleBlocked},
		step: Step{
			PolicyID:    PolicyInteractionMeta,
			GuidingStep: "Promote/transition was blocked. Satisfy the printed preconditions (fields, refs, percent_complete) then re-run zqk object promote. That message is the guiding step.",
			CommandHint: "zqk object promote <id>",
		},
	},
}

func stepsForEvent(event string) []Step {
	var out []Step
	for _, e := range catalog {
		for _, ev := range e.events {
			if ev == event {
				s := e.step
				s.Event = event
				out = append(out, s)
				break
			}
		}
	}
	return out
}
